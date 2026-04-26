package conformance

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

func RunSuite(target string, nowOverride *time.Time) (SuiteReport, error) {
	manifestPath := target
	if statPathIsDir(target) {
		manifestPath = filepath.Join(target, "manifest.json")
	}

	manifestPayload, err := loadJSONObject(manifestPath)
	if err != nil {
		return SuiteReport{}, err
	}
	var manifest manifestFile
	data, err := json.Marshal(manifestPayload)
	if err != nil {
		return SuiteReport{}, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return SuiteReport{}, err
	}

	root := filepath.Dir(manifestPath)
	defaultNow, err := parseDateTime(manifest.Defaults.Now)
	if err != nil {
		return SuiteReport{}, err
	}
	runNow := defaultNow
	if nowOverride != nil {
		runNow = *nowOverride
	}

	schemas, err := LoadSchemas(root)
	if err != nil {
		return SuiteReport{}, err
	}

	results := make([]CaseResult, 0, len(manifest.Cases))
	for _, c := range manifest.Cases {
		results = append(results, runCase(c, root, schemas, runNow))
	}

	name := manifest.Suite.Name
	if name == "" {
		name = "AUMP conformance"
	}
	version := manifest.Suite.Version
	if version == "" {
		version = "0.1.0"
	}
	specVersion := manifest.Suite.SpecVersion
	if specVersion == "" {
		specVersion = "0.1.0"
	}

	return SuiteReport{
		Suite:       name,
		Version:     version,
		SpecVersion: specVersion,
		Results:     results,
	}, nil
}

func runCase(c Case, root string, schemas *SchemaRegistry, now time.Time) CaseResult {
	switch c.Category {
	case "schema":
		return runSchemaCase(c, root, schemas)
	case "mandate":
		return runMandateCase(c, root, schemas, now)
	case "action":
		return runActionCase(c, root, schemas, now)
	case "bridge":
		return runBridgeCase(c, root)
	default:
		return CaseResult{
			ID:       c.ID,
			Category: c.Category,
			Title:    c.Title,
			Passed:   false,
			Expected: "known category",
			Actual:   c.Category,
			Message:  fmt.Sprintf("unknown case category %q", c.Category),
			Details:  map[string]any{},
		}
	}
}

func runSchemaCase(c Case, root string, schemas *SchemaRegistry) CaseResult {
	payload, err := loadJSON(filepath.Join(root, c.Path))
	errors := []string{}
	if err != nil {
		errors = append(errors, err.Error())
	} else {
		errors = schemas.Validate(c.Schema, payload)
	}
	actual := "valid"
	if len(errors) > 0 {
		actual = "invalid"
	}
	return CaseResult{
		ID:          c.ID,
		Category:    "schema",
		Title:       c.Title,
		Passed:      actual == c.Expect,
		Expected:    c.Expect,
		Actual:      actual,
		Message:     joinMessages(errors),
		ReasonCodes: reasonCodesForErrors(errors, "schema_invalid"),
		Details:     map[string]any{},
	}
}

func runMandateCase(c Case, root string, schemas *SchemaRegistry, now time.Time) CaseResult {
	mandate, err := loadJSONObject(filepath.Join(root, c.Path))
	schemaErrors := []string{}
	if err != nil {
		schemaErrors = append(schemaErrors, err.Error())
	} else {
		schemaErrors = schemas.Validate("mandate", mandate)
	}

	actual := "valid"
	reasons := []string{}
	paths := []string{}
	message := joinMessages(schemaErrors)
	if len(schemaErrors) > 0 {
		actual = "invalid"
		reasons = []string{"schema_invalid"}
	} else {
		result := validateMandateSemantics(mandate, now)
		if !result.Valid {
			actual = "invalid"
		}
		reasons = result.ReasonCodes
		paths = result.Paths
	}

	expected := map[string]any{"validity": c.Expect, "reason_codes": c.ReasonCodes}
	actualPayload := map[string]any{"validity": actual, "reason_codes": reasons}
	return CaseResult{
		ID:          c.ID,
		Category:    "mandate",
		Title:       c.Title,
		Passed:      actual == c.Expect && equalStringSlices(reasons, c.ReasonCodes),
		Expected:    expected,
		Actual:      actualPayload,
		Message:     message,
		ReasonCodes: reasons,
		Paths:       paths,
		Details:     map[string]any{},
	}
}

func runActionCase(c Case, root string, schemas *SchemaRegistry, now time.Time) CaseResult {
	mandate, mandateErr := loadJSONObject(filepath.Join(root, c.Mandate))
	action, actionErr := loadJSONObject(filepath.Join(root, c.Action))
	schemaErrors := []string{}
	if mandateErr != nil {
		schemaErrors = append(schemaErrors, mandateErr.Error())
	}
	if actionErr != nil {
		schemaErrors = append(schemaErrors, actionErr.Error())
	}
	if len(schemaErrors) == 0 {
		schemaErrors = append(schemaErrors, schemas.Validate("mandate", mandate)...)
		context := c.Context
		if context == nil {
			context = map[string]any{}
		}
		version := getString(getObj(mandate, "aump"), "version")
		if version == "" {
			version = "0.1.0"
		}
		request := map[string]any{
			"aump": map[string]any{
				"version": version,
				"type":    "action_evaluation_request",
			},
			"mandate_ref": map[string]any{
				"id":      getString(mandate, "id"),
				"version": version,
			},
			"proposed_action": action,
			"context":         context,
		}
		schemaErrors = append(schemaErrors, schemas.Validate("action-evaluation", request)...)
	}

	actualDecision := "denied"
	actualReasons := []string{"schema_invalid"}
	actualPaths := []string{}
	message := joinMessages(schemaErrors)
	if len(schemaErrors) == 0 {
		context := c.Context
		if context == nil {
			context = map[string]any{}
		}
		response := evaluateAction(mandate, action, now, context)
		responseErrors := schemas.Validate("action-evaluation", response)
		actualDecision = getString(response, "decision")
		actualReasons = stringSlice(response["reason_codes"])
		actualPaths = stringSlice(response["paths"])
		message = joinMessages(responseErrors)
		if len(responseErrors) > 0 {
			actualReasons = append(actualReasons, "schema_invalid")
		}
	}

	expected := map[string]any{"decision": c.Expect, "reason_codes": c.ReasonCodes}
	actual := map[string]any{"decision": actualDecision, "reason_codes": actualReasons}
	passed := actualDecision == c.Expect && equalStringSlices(actualReasons, c.ReasonCodes) && message == ""
	return CaseResult{
		ID:          c.ID,
		Category:    "action",
		Title:       c.Title,
		Passed:      passed,
		Expected:    expected,
		Actual:      actual,
		Message:     message,
		ReasonCodes: actualReasons,
		Paths:       actualPaths,
		Details:     map[string]any{},
	}
}

func runBridgeCase(c Case, root string) CaseResult {
	payload, err := loadJSONObject(filepath.Join(root, c.Path))
	errors := []string{}
	if err != nil {
		errors = append(errors, err.Error())
	} else {
		var valid bool
		valid, errors = validateBridge(payload, c.BridgeType)
		if valid {
			errors = nil
		}
	}
	actual := "valid"
	if len(errors) > 0 {
		actual = "invalid"
	}
	return CaseResult{
		ID:          c.ID,
		Category:    "bridge",
		Title:       c.Title,
		Passed:      actual == c.Expect,
		Expected:    c.Expect,
		Actual:      actual,
		Message:     joinMessages(errors),
		ReasonCodes: reasonCodesForErrors(errors, "bridge_invalid"),
		Details:     map[string]any{},
	}
}

func reasonCodesForErrors(errors []string, reason string) []string {
	if len(errors) == 0 {
		return []string{}
	}
	return []string{reason}
}
