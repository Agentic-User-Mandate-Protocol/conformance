"""Bridge fixture validators for MCP, A2A, and UCP/AP2 payload shapes."""

from __future__ import annotations

import re
from typing import Any

AUMP_META_ID = "org.agentic-user-mandate-protocol/aump_mandate_id"
AUMP_META_HASH = "org.agentic-user-mandate-protocol/aump_mandate_hash"
AUMP_A2A_EXTENSION_URI = (
    "https://agentic-user-mandate-protocol.github.io/spec/bindings/a2a"
)
META_KEY_RE = re.compile(
    r"^([A-Za-z][A-Za-z0-9-]*(?:\.[A-Za-z][A-Za-z0-9-]*)*/)?"
    r"[A-Za-z0-9](?:[A-Za-z0-9_.-]*[A-Za-z0-9])?$"
)


def validate_bridge(
    payload: dict[str, Any],
    bridge_type: str,
) -> tuple[bool, list[str]]:
    """Validate a protocol bridge payload."""
    if bridge_type == "mcp_meta":
        return _validate_mcp_meta(payload)
    if bridge_type == "a2a_extension":
        return _validate_a2a_extension(payload)
    if bridge_type == "a2a_message":
        return _validate_a2a_message(payload)
    if bridge_type == "ucp_reference":
        return _validate_ucp_reference(payload)
    return False, [f"unknown bridge_type {bridge_type!r}"]


def _validate_mcp_meta(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    errors: list[str] = []
    meta = _find_first_meta(payload)
    if meta is None:
        return False, ["missing MCP _meta object"]

    for key in meta:
        if not META_KEY_RE.match(key):
            errors.append(f"invalid MCP _meta key {key!r}")
        prefix = key.split("/", 1)[0] if "/" in key else ""
        labels = prefix.split(".")
        if len(labels) >= 2 and labels[1] in {"mcp", "modelcontextprotocol"}:
            errors.append(f"reserved MCP _meta prefix {prefix!r}")

    if not meta.get(AUMP_META_ID):
        errors.append(f"missing {AUMP_META_ID}")
    if not meta.get(AUMP_META_HASH):
        errors.append(f"missing {AUMP_META_HASH}")

    return not errors, errors


def _validate_a2a_extension(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    extensions = payload.get("capabilities", {}).get("extensions", [])
    if not isinstance(extensions, list):
        return False, ["capabilities.extensions must be an array"]

    for extension in extensions:
        if not isinstance(extension, dict):
            continue
        if extension.get("uri") == AUMP_A2A_EXTENSION_URI:
            return True, []
    return False, ["missing AUMP A2A AgentExtension declaration"]


def _validate_a2a_message(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    metadata = payload.get("message", {}).get("metadata", {})
    errors: list[str] = []
    if not metadata.get("aump_mandate_id"):
        errors.append("missing message.metadata.aump_mandate_id")
    if not metadata.get("aump_mandate_hash"):
        errors.append("missing message.metadata.aump_mandate_hash")
    return not errors, errors


def _validate_ucp_reference(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    aump = _find_ucp_aump_ref(payload)
    if not isinstance(aump, dict):
        return False, ["missing aump reference object"]

    errors: list[str] = []
    if "mandate" in aump:
        errors.append("UCP/AP2 bridge must not embed full private AUMP mandate")
    for field in ("mandate_id", "mandate_hash", "version"):
        if not aump.get(field):
            errors.append(f"missing aump.{field}")
    return not errors, errors


def _find_ucp_aump_ref(payload: dict[str, Any]) -> dict[str, Any] | None:
    aump = payload.get("aump")
    if isinstance(aump, dict):
        return aump
    meta = payload.get("meta")
    if isinstance(meta, dict) and isinstance(meta.get("aump"), dict):
        return meta["aump"]
    return None


def _find_first_meta(value: Any) -> dict[str, Any] | None:
    if isinstance(value, dict):
        meta = value.get("_meta")
        if isinstance(meta, dict):
            return meta
        for child in value.values():
            found = _find_first_meta(child)
            if found is not None:
                return found
    elif isinstance(value, list):
        for child in value:
            found = _find_first_meta(child)
            if found is not None:
                return found
    return None
