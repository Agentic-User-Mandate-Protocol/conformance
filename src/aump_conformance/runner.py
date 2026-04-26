"""Conformance suite execution."""

from __future__ import annotations

import hashlib
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from aump_conformance.bridges import validate_bridge
from aump_conformance.jsonio import load_json
from aump_conformance.models import CaseResult, SuiteReport
from aump_conformance.policy import (
    evaluate_action,
    parse_datetime,
    validate_mandate_semantics,
)
from aump_conformance.schemas import SchemaRegistry


def run_suite(target: Path, *, now: datetime | None = None) -> SuiteReport:
    """Run a conformance suite from a fixture directory or manifest path."""
    manifest_path = target / "manifest.json" if target.is_dir() else target
    manifest = load_json(manifest_path)
    fixture_root = manifest_path.parent
    default_now = parse_datetime(manifest.get("defaults", {}).get("now"))
    run_now = now or default_now
    schemas = SchemaRegistry.load(fixture_root)

    results = [
        _run_case(case, fixture_root=fixture_root, schemas=schemas, now=run_now)
        for case in manifest.get("cases", [])
    ]

    suite = manifest.get("aump_conformance", {})
    return SuiteReport(
        suite=suite.get("name", "AUMP conformance"),
        version=suite.get("version", "0.1.0"),
        spec_version=suite.get("spec_version", "0.1.0"),
        results=results,
    )


def _run_case(
    case: dict[str, Any],
    *,
    fixture_root: Path,
    schemas: SchemaRegistry,
    now: datetime,
) -> CaseResult:
    category = case.get("category", "")
    if category == "schema":
        return _run_schema_case(case, fixture_root=fixture_root, schemas=schemas)
    if category == "mandate":
        return _run_mandate_case(
            case,
            fixture_root=fixture_root,
            schemas=schemas,
            now=now,
        )
    if category == "action":
        return _run_action_case(
            case,
            fixture_root=fixture_root,
            schemas=schemas,
            now=now,
        )
    if category == "evidence":
        return _run_evidence_case(
            case,
            fixture_root=fixture_root,
            schemas=schemas,
            now=now,
        )
    if category == "bridge":
        return _run_bridge_case(case, fixture_root=fixture_root)

    return CaseResult(
        id=case.get("id", "unknown"),
        category=category,
        title=case.get("title", ""),
        passed=False,
        expected="known category",
        actual=category,
        message=f"unknown case category {category!r}",
    )


def _run_schema_case(
    case: dict[str, Any],
    *,
    fixture_root: Path,
    schemas: SchemaRegistry,
) -> CaseResult:
    payload = load_json(fixture_root / case["path"])
    errors = schemas.validate(case["schema"], payload)
    actual = "invalid" if errors else "valid"
    expected = case["expect"]
    return CaseResult(
        id=case["id"],
        category="schema",
        title=case.get("title", ""),
        passed=actual == expected,
        expected=expected,
        actual=actual,
        message="; ".join(errors),
        reason_codes=["schema_invalid"] if errors else [],
    )


def _run_mandate_case(
    case: dict[str, Any],
    *,
    fixture_root: Path,
    schemas: SchemaRegistry,
    now: datetime,
) -> CaseResult:
    mandate = load_json(fixture_root / case["path"])
    schema_errors = schemas.validate("mandate", mandate)
    if schema_errors:
        actual = "invalid"
        reason_codes = ["schema_invalid"]
        paths: list[str] = []
        message = "; ".join(schema_errors)
    else:
        valid, reason_codes, paths = validate_mandate_semantics(mandate, now=now)
        actual = "valid" if valid else "invalid"
        message = ""

    expected = case["expect"]
    expected_reasons = case.get("reason_codes", [])
    passed = actual == expected and _matches_reasons(reason_codes, expected_reasons)
    return CaseResult(
        id=case["id"],
        category="mandate",
        title=case.get("title", ""),
        passed=passed,
        expected={"validity": expected, "reason_codes": expected_reasons},
        actual={"validity": actual, "reason_codes": reason_codes},
        message=message,
        reason_codes=reason_codes,
        paths=paths,
    )


def _run_action_case(
    case: dict[str, Any],
    *,
    fixture_root: Path,
    schemas: SchemaRegistry,
    now: datetime,
) -> CaseResult:
    mandate = load_json(fixture_root / case["mandate"])
    action = load_json(fixture_root / case["action"])
    context = case.get("context", {})

    schema_errors = schemas.validate("mandate", mandate)
    request = {
        "aump": {
            "version": mandate.get("aump", {}).get("version", "0.1.0"),
            "type": "action_evaluation_request",
        },
        "mandate_ref": {
            "id": mandate.get("id", ""),
            "version": mandate.get("aump", {}).get("version", "0.1.0"),
        },
        "proposed_action": action,
        "context": context,
    }
    schema_errors.extend(schemas.validate("action-evaluation", request))
    if schema_errors:
        actual_decision = "denied"
        actual_reasons = ["schema_invalid"]
        actual_paths: list[str] = []
        message = "; ".join(schema_errors)
    else:
        response = evaluate_action(mandate, action, now=now, context=context)
        response_errors = schemas.validate("action-evaluation", response)
        actual_decision = response["decision"]
        actual_reasons = response["reason_codes"]
        actual_paths = response["paths"]
        message = "; ".join(response_errors)
        if response_errors:
            actual_reasons = [*actual_reasons, "schema_invalid"]

    expected_decision = case["expect"]
    expected_reasons = case.get("reason_codes", [])
    passed = (
        actual_decision == expected_decision
        and _matches_reasons(actual_reasons, expected_reasons)
        and not message
    )
    return CaseResult(
        id=case["id"],
        category="action",
        title=case.get("title", ""),
        passed=passed,
        expected={"decision": expected_decision, "reason_codes": expected_reasons},
        actual={"decision": actual_decision, "reason_codes": actual_reasons},
        message=message,
        reason_codes=actual_reasons,
        paths=actual_paths,
    )


def _run_bridge_case(case: dict[str, Any], *, fixture_root: Path) -> CaseResult:
    payload = load_json(fixture_root / case["path"])
    valid, errors = validate_bridge(payload, case["bridge_type"])
    actual = "valid" if valid else "invalid"
    expected = case["expect"]
    return CaseResult(
        id=case["id"],
        category="bridge",
        title=case.get("title", ""),
        passed=actual == expected,
        expected=expected,
        actual=actual,
        message="; ".join(errors),
        reason_codes=[] if valid else ["bridge_invalid"],
    )


def _run_evidence_case(
    case: dict[str, Any],
    *,
    fixture_root: Path,
    schemas: SchemaRegistry,
    now: datetime,
) -> CaseResult:
    mandate = load_json(fixture_root / case["mandate"])
    event = load_json(fixture_root / case["path"])
    schema_errors = schemas.validate("mandate", mandate)
    schema_errors.extend(schemas.validate("evidence-event", event))
    if schema_errors:
        actual = "invalid"
        reason_codes = ["schema_invalid"]
        paths: list[str] = []
        message = "; ".join(schema_errors)
    else:
        valid, reason_codes, paths = _validate_evidence_semantics(
            mandate,
            event,
            now=now,
        )
        actual = "valid" if valid else "invalid"
        message = ""

    expected = case["expect"]
    expected_reasons = case.get("reason_codes", [])
    passed = actual == expected and _matches_reasons(reason_codes, expected_reasons)
    return CaseResult(
        id=case["id"],
        category="evidence",
        title=case.get("title", ""),
        passed=passed,
        expected={"validity": expected, "reason_codes": expected_reasons},
        actual={"validity": actual, "reason_codes": reason_codes},
        message=message,
        reason_codes=reason_codes,
        paths=paths,
    )


def _validate_evidence_semantics(
    mandate: dict[str, Any],
    event: dict[str, Any],
    *,
    now: datetime,
) -> tuple[bool, list[str], list[str]]:
    reason_codes: list[str] = []
    paths: list[str] = []

    mandate_valid, mandate_reasons, mandate_paths = validate_mandate_semantics(
        mandate,
        now=now,
    )
    if not mandate_valid:
        reason_codes.extend(mandate_reasons)
        paths.extend(mandate_paths)

    mandate_ref = event.get("mandate_ref", {})
    mandate_id_matches = mandate_ref.get("id") == mandate.get("id")
    if not mandate_id_matches:
        reason_codes.append("evidence_mandate_mismatch")
        paths.append("$.mandate_ref.id")
    if mandate_id_matches and mandate_ref.get("hash") != _hash_payload(mandate):
        reason_codes.append("evidence_mandate_hash_mismatch")
        paths.append("$.mandate_ref.hash")

    evidence_policy = mandate.get("evidence", {})
    required_events = set(evidence_policy.get("events_required", []))
    if required_events and event.get("event_type") not in required_events:
        reason_codes.append("evidence_event_type_not_required")
        paths.append("$.event_type")

    retention = evidence_policy.get("retention")
    privacy = event.get("privacy", {})
    if retention and privacy.get("retention") != retention:
        reason_codes.append("evidence_retention_mismatch")
        paths.append("$.privacy.retention")
    if retention != "full_transcript" and privacy.get("contains_private_fields"):
        reason_codes.append("evidence_private_field_leak")
        paths.append("$.privacy.contains_private_fields")

    return not reason_codes, _stable_unique(reason_codes), _stable_unique(paths)


def _hash_payload(payload: dict[str, Any]) -> str:
    encoded = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    return f"sha256-{hashlib.sha256(encoded).hexdigest()}"


def _stable_unique(values: list[str]) -> list[str]:
    result: list[str] = []
    for value in values:
        if value not in result:
            result.append(value)
    return result


def _matches_reasons(actual: list[str], expected: list[str]) -> bool:
    return sorted(actual) == sorted(expected)
