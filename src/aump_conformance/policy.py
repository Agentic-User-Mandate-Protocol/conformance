"""Deterministic v0.1 AUMP policy checks used by the conformance oracle."""

from __future__ import annotations

from datetime import UTC, datetime
from typing import Any

COMMITMENT_ACTIONS = {
    "accept_deal",
    "complete_checkout",
    "place_order",
    "create_ap2_payment_mandate",
}


def parse_datetime(value: str) -> datetime:
    """Parse an RFC 3339-ish UTC datetime used by the fixtures."""
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    parsed = datetime.fromisoformat(value)
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=UTC)
    return parsed.astimezone(UTC)


def validate_mandate_semantics(
    mandate: dict[str, Any],
    *,
    now: datetime,
) -> tuple[bool, list[str], list[str]]:
    """Validate behavioral mandate requirements not fully encoded in schema."""
    reason_codes: list[str] = []
    paths: list[str] = []

    status = mandate.get("status")
    if status != "active":
        reason_codes.append("mandate_inactive")
        paths.append("$.status")

    expires_at = mandate.get("expires_at")
    if isinstance(expires_at, str):
        try:
            if parse_datetime(expires_at) <= now:
                reason_codes.append("mandate_expired")
                paths.append("$.expires_at")
        except ValueError:
            reason_codes.append("schema_invalid")
            paths.append("$.expires_at")

    authority = mandate.get("authority", {})
    if authority.get("mode") == "delegated" and not _has_objective_bound(mandate):
        reason_codes.append("scope_violation")
        paths.append("$.authority")

    return not reason_codes, reason_codes, paths


def evaluate_action(
    mandate: dict[str, Any],
    action: dict[str, Any],
    *,
    now: datetime,
    context: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Evaluate a proposed action against an AUMP mandate."""
    context = context or {}
    denied_reasons: list[str] = []
    denied_paths: list[str] = []
    escalation_reasons: list[str] = []
    escalation_paths: list[str] = []

    mandate_ok, mandate_reasons, mandate_paths = validate_mandate_semantics(
        mandate,
        now=now,
    )
    if not mandate_ok:
        denied_reasons.extend(mandate_reasons)
        denied_paths.extend(mandate_paths)

    authority = mandate.get("authority", {})
    action_type = action.get("type")
    permissions = set(authority.get("permissions", []))
    if action_type not in permissions:
        denied_reasons.append("scope_violation")
        denied_paths.append("$.authority.permissions")

    prohibited_actions = set(authority.get("prohibited_actions", []))
    if action_type in prohibited_actions:
        denied_reasons.append("scope_violation")
        denied_paths.append("$.authority.prohibited_actions")

    if (
        action_type == "accept_deal"
        and "accept_deal_without_approval" in prohibited_actions
        and not context.get("approval")
    ):
        escalation_reasons.append("escalation_required")
        escalation_paths.append("$.authority.prohibited_actions")

    _evaluate_amount(
        mandate=mandate,
        action=action,
        denied_reasons=denied_reasons,
        denied_paths=denied_paths,
    )
    _evaluate_disclosures(
        mandate=mandate,
        action=action,
        denied_reasons=denied_reasons,
        denied_paths=denied_paths,
    )
    _evaluate_escalation(
        mandate=mandate,
        action=action,
        context=context,
        escalation_reasons=escalation_reasons,
        escalation_paths=escalation_paths,
    )

    if denied_reasons:
        decision = "denied"
        reason_codes = _stable_unique(denied_reasons)
        paths = _stable_unique(denied_paths)
    elif escalation_reasons:
        decision = "requires_escalation"
        reason_codes = _stable_unique(escalation_reasons)
        paths = _stable_unique(escalation_paths)
    else:
        decision = "allowed"
        reason_codes = []
        paths = []

    return {
        "aump": {
            "version": mandate.get("aump", {}).get("version", "0.1.0"),
            "type": "action_evaluation_response",
        },
        "mandate_ref": {
            "id": mandate.get("id", ""),
            "version": mandate.get("aump", {}).get("version", "0.1.0"),
        },
        "decision": decision,
        "reason_codes": reason_codes,
        "paths": paths,
        "summary": _summary(decision, reason_codes),
    }


def _has_objective_bound(mandate: dict[str, Any]) -> bool:
    authority = mandate.get("authority", {})
    purpose = mandate.get("purpose", {})
    return any(
        [
            "budget" in authority,
            bool(authority.get("prohibited_actions")),
            bool(purpose.get("allowed_categories")),
            bool(purpose.get("allowed_counterparties")),
            bool(purpose.get("excluded_counterparties")),
        ]
    )


def _evaluate_amount(
    *,
    mandate: dict[str, Any],
    action: dict[str, Any],
    denied_reasons: list[str],
    denied_paths: list[str],
) -> None:
    amount = action.get("amount")
    if not isinstance(amount, dict):
        return

    budget = mandate.get("authority", {}).get("budget")
    if not isinstance(budget, dict):
        denied_reasons.append("scope_violation")
        denied_paths.append("$.authority.budget")
        return

    currency = amount.get("currency")
    if currency != budget.get("currency"):
        denied_reasons.append("hard_constraint_violation")
        denied_reasons.append("currency_mismatch")
        denied_paths.append("$.authority.budget.currency")

    total_minor = amount.get("total_minor")
    max_total_minor = budget.get("max_total_minor")
    if (
        isinstance(total_minor, int)
        and isinstance(max_total_minor, int)
        and total_minor > max_total_minor
    ):
        denied_reasons.append("hard_constraint_violation")
        denied_reasons.append("price_above_budget")
        denied_paths.append("$.authority.budget.max_total_minor")

    max_item_minor = budget.get("max_item_minor")
    item_minor = amount.get("item_minor")
    if (
        isinstance(item_minor, int)
        and isinstance(max_item_minor, int)
        and item_minor > max_item_minor
    ):
        denied_reasons.append("hard_constraint_violation")
        denied_reasons.append("price_above_budget")
        denied_paths.append("$.authority.budget.max_item_minor")


def _evaluate_disclosures(
    *,
    mandate: dict[str, Any],
    action: dict[str, Any],
    denied_reasons: list[str],
    denied_paths: list[str],
) -> None:
    disclosures = action.get("disclosures", [])
    if not isinstance(disclosures, list) or not disclosures:
        return

    disclosure_policy = mandate.get("disclosure", {})
    allowed = {
        rule.get("field")
        for rule in disclosure_policy.get("allowed", [])
        if isinstance(rule, dict)
    }
    prohibited = {
        rule.get("field")
        for rule in disclosure_policy.get("prohibited", [])
        if isinstance(rule, dict)
    }
    default = disclosure_policy.get("default", "deny")

    for index, disclosure in enumerate(disclosures):
        if not isinstance(disclosure, dict):
            continue
        field = disclosure.get("field")
        path = f"$.proposed_action.disclosures[{index}].field"
        should_deny = (
            field in prohibited
            or _is_protected_field(mandate, field)
            or (default == "deny" and field not in allowed)
        )
        if should_deny:
            denied_reasons.append("disclosure_denied")
            denied_paths.append(path)


def _evaluate_escalation(
    *,
    mandate: dict[str, Any],
    action: dict[str, Any],
    context: dict[str, Any],
    escalation_reasons: list[str],
    escalation_paths: list[str],
) -> None:
    escalation = mandate.get("escalation", {})
    required_conditions = set(escalation.get("required_conditions", []))
    active_conditions = set(context.get("conditions", []))
    matched_conditions = sorted(required_conditions & active_conditions)
    if matched_conditions:
        escalation_reasons.append("escalation_required")
        escalation_paths.append("$.escalation.required_conditions")

    threshold = escalation.get("confidence_threshold")
    confidence = context.get("confidence")
    if (
        isinstance(threshold, int | float)
        and isinstance(confidence, int | float)
        and confidence < threshold
    ):
        escalation_reasons.append("escalation_required")
        escalation_reasons.append("confidence_below_threshold")
        escalation_paths.append("$.escalation.confidence_threshold")

    authority = mandate.get("authority", {})
    is_commitment = (
        bool(action.get("commitment")) or action.get("type") in COMMITMENT_ACTIONS
    )
    if authority.get("mode") == "supervised" and is_commitment:
        escalation_reasons.append("escalation_required")
        escalation_paths.append("$.authority.mode")

    if (
        authority.get("requires_trusted_ui_for_commitment")
        and is_commitment
        and not context.get("trusted_ui_approved")
    ):
        escalation_reasons.append("escalation_required")
        escalation_paths.append("$.authority.requires_trusted_ui_for_commitment")


def _is_protected_field(mandate: dict[str, Any], field: Any) -> bool:
    if not isinstance(field, str):
        return False

    negotiation = mandate.get("negotiation", {})
    for protected in negotiation.get("protected_fields", []):
        if field == protected or field.endswith(f".{protected}"):
            return True

    return field == "preferences.private_notes" or field.endswith(".private_notes")


def _stable_unique(values: list[str]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for value in values:
        if value not in seen:
            seen.add(value)
            result.append(value)
    return result


def _summary(decision: str, reason_codes: list[str]) -> str:
    if decision == "allowed":
        return "The proposed action is allowed by the active mandate."
    if decision == "requires_escalation":
        return "The proposed action requires trusted review before continuing."
    return "The proposed action is denied by mandate policy: " + ", ".join(
        reason_codes
    )
