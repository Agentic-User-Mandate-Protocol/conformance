package conformance

import (
	"fmt"
	"strings"
	"time"
)

var commitmentActions = map[string]bool{
	"accept_deal":                true,
	"complete_checkout":          true,
	"place_order":                true,
	"create_ap2_payment_mandate": true,
}

type semanticResult struct {
	Valid       bool
	ReasonCodes []string
	Paths       []string
}

func validateMandateSemantics(mandate map[string]any, now time.Time) semanticResult {
	reasons := []string{}
	paths := []string{}

	if getString(mandate, "status") != "active" {
		reasons = append(reasons, "mandate_inactive")
		paths = append(paths, "$.status")
	}

	if expiresAt := getString(mandate, "expires_at"); expiresAt != "" {
		parsed, err := parseDateTime(expiresAt)
		if err != nil {
			reasons = append(reasons, "schema_invalid")
			paths = append(paths, "$.expires_at")
		} else if !parsed.After(now) {
			reasons = append(reasons, "mandate_expired")
			paths = append(paths, "$.expires_at")
		}
	}

	authority := getObj(mandate, "authority")
	if getString(authority, "mode") == "delegated" && !hasObjectiveBound(mandate) {
		reasons = append(reasons, "scope_violation")
		paths = append(paths, "$.authority")
	}

	return semanticResult{Valid: len(reasons) == 0, ReasonCodes: reasons, Paths: paths}
}

func evaluateAction(mandate map[string]any, action map[string]any, now time.Time, context map[string]any) map[string]any {
	if context == nil {
		context = map[string]any{}
	}
	deniedReasons := []string{}
	deniedPaths := []string{}
	escalationReasons := []string{}
	escalationPaths := []string{}

	mandateResult := validateMandateSemantics(mandate, now)
	if !mandateResult.Valid {
		deniedReasons = append(deniedReasons, mandateResult.ReasonCodes...)
		deniedPaths = append(deniedPaths, mandateResult.Paths...)
	}

	authority := getObj(mandate, "authority")
	actionType := getString(action, "type")
	permissions := stringSet(authority["permissions"])
	if !permissions[actionType] {
		deniedReasons = append(deniedReasons, "scope_violation")
		deniedPaths = append(deniedPaths, "$.authority.permissions")
	}

	prohibited := stringSet(authority["prohibited_actions"])
	if prohibited[actionType] {
		deniedReasons = append(deniedReasons, "scope_violation")
		deniedPaths = append(deniedPaths, "$.authority.prohibited_actions")
	}

	if actionType == "accept_deal" && prohibited["accept_deal_without_approval"] && !getBool(context, "approval") {
		escalationReasons = append(escalationReasons, "escalation_required")
		escalationPaths = append(escalationPaths, "$.authority.prohibited_actions")
	}

	evaluateAmount(mandate, action, &deniedReasons, &deniedPaths)
	evaluateDisclosures(mandate, action, &deniedReasons, &deniedPaths)
	evaluateEscalation(mandate, action, context, &escalationReasons, &escalationPaths)

	decision := "allowed"
	reasonCodes := []string{}
	paths := []string{}
	if len(deniedReasons) > 0 {
		decision = "denied"
		reasonCodes = stableUnique(deniedReasons)
		paths = stableUnique(deniedPaths)
	} else if len(escalationReasons) > 0 {
		decision = "requires_escalation"
		reasonCodes = stableUnique(escalationReasons)
		paths = stableUnique(escalationPaths)
	}

	version := getString(getObj(mandate, "aump"), "version")
	if version == "" {
		version = "0.1.0"
	}
	return map[string]any{
		"aump": map[string]any{
			"version": version,
			"type":    "action_evaluation_response",
		},
		"mandate_ref": map[string]any{
			"id":      getString(mandate, "id"),
			"version": version,
		},
		"decision":     decision,
		"reason_codes": reasonCodes,
		"paths":        paths,
		"summary":      summary(decision, reasonCodes),
	}
}

func hasObjectiveBound(mandate map[string]any) bool {
	authority := getObj(mandate, "authority")
	purpose := getObj(mandate, "purpose")
	return len(getObj(authority, "budget")) > 0 ||
		len(stringSlice(authority["prohibited_actions"])) > 0 ||
		len(stringSlice(purpose["allowed_categories"])) > 0 ||
		len(stringSlice(purpose["allowed_counterparties"])) > 0 ||
		len(stringSlice(purpose["excluded_counterparties"])) > 0
}

func evaluateAmount(mandate map[string]any, action map[string]any, deniedReasons *[]string, deniedPaths *[]string) {
	amountRaw, ok := action["amount"].(map[string]any)
	if !ok {
		return
	}

	budgetRaw, ok := getObj(mandate, "authority")["budget"].(map[string]any)
	if !ok {
		*deniedReasons = append(*deniedReasons, "scope_violation")
		*deniedPaths = append(*deniedPaths, "$.authority.budget")
		return
	}

	if getString(amountRaw, "currency") != getString(budgetRaw, "currency") {
		*deniedReasons = append(*deniedReasons, "hard_constraint_violation", "currency_mismatch")
		*deniedPaths = append(*deniedPaths, "$.authority.budget.currency")
	}

	totalMinor, hasTotal := getFloat(amountRaw, "total_minor")
	maxTotalMinor, hasMaxTotal := getFloat(budgetRaw, "max_total_minor")
	if hasTotal && hasMaxTotal && totalMinor > maxTotalMinor {
		*deniedReasons = append(*deniedReasons, "hard_constraint_violation", "price_above_budget")
		*deniedPaths = append(*deniedPaths, "$.authority.budget.max_total_minor")
	}

	itemMinor, hasItem := getFloat(amountRaw, "item_minor")
	maxItemMinor, hasMaxItem := getFloat(budgetRaw, "max_item_minor")
	if hasItem && hasMaxItem && itemMinor > maxItemMinor {
		*deniedReasons = append(*deniedReasons, "hard_constraint_violation", "price_above_budget")
		*deniedPaths = append(*deniedPaths, "$.authority.budget.max_item_minor")
	}
}

func evaluateDisclosures(mandate map[string]any, action map[string]any, deniedReasons *[]string, deniedPaths *[]string) {
	disclosures, ok := action["disclosures"].([]any)
	if !ok || len(disclosures) == 0 {
		return
	}

	policy := getObj(mandate, "disclosure")
	allowed := map[string]bool{}
	for _, raw := range disclosureRules(policy["allowed"]) {
		allowed[getString(raw, "field")] = true
	}
	prohibited := map[string]bool{}
	for _, raw := range disclosureRules(policy["prohibited"]) {
		prohibited[getString(raw, "field")] = true
	}
	defaultPolicy := getString(policy, "default")
	if defaultPolicy == "" {
		defaultPolicy = "deny"
	}

	for index, raw := range disclosures {
		disclosure, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		field := getString(disclosure, "field")
		shouldDeny := prohibited[field] || isProtectedField(mandate, field) || (defaultPolicy == "deny" && !allowed[field])
		if shouldDeny {
			*deniedReasons = append(*deniedReasons, "disclosure_denied")
			*deniedPaths = append(*deniedPaths, fmt.Sprintf("$.proposed_action.disclosures[%d].field", index))
		}
	}
}

func disclosureRules(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := []map[string]any{}
	for _, item := range items {
		if rule, ok := item.(map[string]any); ok {
			result = append(result, rule)
		}
	}
	return result
}

func evaluateEscalation(mandate map[string]any, action map[string]any, context map[string]any, reasons *[]string, paths *[]string) {
	escalation := getObj(mandate, "escalation")
	required := stringSet(escalation["required_conditions"])
	active := stringSet(context["conditions"])
	for condition := range required {
		if active[condition] {
			*reasons = append(*reasons, "escalation_required")
			*paths = append(*paths, "$.escalation.required_conditions")
			break
		}
	}

	threshold, hasThreshold := getFloat(escalation, "confidence_threshold")
	confidence, hasConfidence := getFloat(context, "confidence")
	if hasThreshold && hasConfidence && confidence < threshold {
		*reasons = append(*reasons, "escalation_required", "confidence_below_threshold")
		*paths = append(*paths, "$.escalation.confidence_threshold")
	}

	authority := getObj(mandate, "authority")
	actionType := getString(action, "type")
	isCommitment := getBool(action, "commitment") || commitmentActions[actionType]
	if getString(authority, "mode") == "supervised" && isCommitment {
		*reasons = append(*reasons, "escalation_required")
		*paths = append(*paths, "$.authority.mode")
	}

	if getBool(authority, "requires_trusted_ui_for_commitment") && isCommitment && !getBool(context, "trusted_ui_approved") {
		*reasons = append(*reasons, "escalation_required")
		*paths = append(*paths, "$.authority.requires_trusted_ui_for_commitment")
	}
}

func isProtectedField(mandate map[string]any, field string) bool {
	if field == "" {
		return false
	}
	negotiation := getObj(mandate, "negotiation")
	for _, protected := range stringSlice(negotiation["protected_fields"]) {
		if field == protected || strings.HasSuffix(field, "."+protected) {
			return true
		}
	}
	return field == "preferences.private_notes" || strings.HasSuffix(field, ".private_notes")
}

func summary(decision string, reasonCodes []string) string {
	switch decision {
	case "allowed":
		return "The proposed action is allowed by the active mandate."
	case "requires_escalation":
		return "The proposed action requires trusted review before continuing."
	default:
		return "The proposed action is denied by mandate policy: " + strings.Join(reasonCodes, ", ")
	}
}
