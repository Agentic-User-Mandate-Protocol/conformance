package conformance

import (
	"crypto/sha256"
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
	case "evidence":
		return runEvidenceCase(c, root, schemas, now)
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

func runEvidenceCase(c Case, root string, schemas *SchemaRegistry, now time.Time) CaseResult {
	mandate, mandateErr := loadJSONObject(filepath.Join(root, c.Mandate))
	event, eventErr := loadJSONObject(filepath.Join(root, c.Path))
	schemaErrors := []string{}
	if mandateErr != nil {
		schemaErrors = append(schemaErrors, mandateErr.Error())
	}
	if eventErr != nil {
		schemaErrors = append(schemaErrors, eventErr.Error())
	}
	if len(schemaErrors) == 0 {
		schemaErrors = append(schemaErrors, schemas.Validate("mandate", mandate)...)
		schemaErrors = append(schemaErrors, schemas.Validate("evidence-event", event)...)
	}

	actual := "valid"
	reasons := []string{}
	paths := []string{}
	message := joinMessages(schemaErrors)
	if len(schemaErrors) > 0 {
		actual = "invalid"
		reasons = []string{"schema_invalid"}
	} else {
		valid, evidenceReasons, evidencePaths := validateEvidenceSemantics(mandate, event, now)
		if !valid {
			actual = "invalid"
		}
		reasons = evidenceReasons
		paths = evidencePaths
	}

	expected := map[string]any{"validity": c.Expect, "reason_codes": c.ReasonCodes}
	actualPayload := map[string]any{"validity": actual, "reason_codes": reasons}
	return CaseResult{
		ID:          c.ID,
		Category:    "evidence",
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

func validateEvidenceSemantics(mandate map[string]any, event map[string]any, now time.Time) (bool, []string, []string) {
	reasons := []string{}
	paths := []string{}

	if mandateResult := validateMandateSemantics(mandate, now); !mandateResult.Valid {
		reasons = append(reasons, mandateResult.ReasonCodes...)
		paths = append(paths, mandateResult.Paths...)
	}

	mandateRef := getObj(event, "mandate_ref")
	mandateIDMatches := getString(mandateRef, "id") == getString(mandate, "id")
	if !mandateIDMatches {
		reasons = append(reasons, "evidence_mandate_mismatch")
		paths = append(paths, "$.mandate_ref.id")
	}
	if hash := getString(mandateRef, "hash"); mandateIDMatches && hash != "" && hash != hashPayload(mandate) {
		reasons = append(reasons, "evidence_mandate_hash_mismatch")
		paths = append(paths, "$.mandate_ref.hash")
	}

	evidencePolicy := getObj(mandate, "evidence")
	requiredEvents := stringSet(evidencePolicy["events_required"])
	if len(requiredEvents) > 0 && !requiredEvents[getString(event, "event_type")] {
		reasons = append(reasons, "evidence_event_type_not_required")
		paths = append(paths, "$.event_type")
	}

	retention := getString(evidencePolicy, "retention")
	privacy := getObj(event, "privacy")
	if eventRetention := getString(privacy, "retention"); retention != "" && eventRetention != retention {
		reasons = append(reasons, "evidence_retention_mismatch")
		paths = append(paths, "$.privacy.retention")
	}
	if retention != "full_transcript" && getBool(privacy, "contains_private_fields") {
		reasons = append(reasons, "evidence_private_field_leak")
		paths = append(paths, "$.privacy.contains_private_fields")
	}

	reasons = stableUnique(reasons)
	paths = stableUnique(paths)
	return len(reasons) == 0, reasons, paths
}

func hashPayload(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256-%x", sum)
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
