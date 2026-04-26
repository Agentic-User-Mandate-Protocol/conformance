"""Bridge fixture validators for MCP, A2A, and UCP/AP2 payload shapes."""

from __future__ import annotations

import re
from typing import Any

AUMP_META_ID = "org.agentic-user-mandate-protocol/aump_mandate_id"
AUMP_META_HASH = "org.agentic-user-mandate-protocol/aump_mandate_hash"
AUMP_META_VERSION = "org.agentic-user-mandate-protocol/aump_version"
AUMP_A2A_EXTENSION_URI = (
    "https://agentic-user-mandate-protocol.github.io/spec/bindings/a2a/v0.1"
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
    if bridge_type == "mcp_tool":
        return _validate_mcp_tool(payload)
    if bridge_type == "a2a_extension":
        return _validate_a2a_extension(payload)
    if bridge_type == "a2a_message":
        return _validate_a2a_message(payload)
    if bridge_type == "ucp_reference":
        return _validate_ucp_reference(payload)
    return False, [f"unknown bridge_type {bridge_type!r}"]


def _validate_mcp_meta(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    errors: list[str] = []
    meta = _find_mcp_request_meta(payload)
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
    if not meta.get(AUMP_META_VERSION):
        errors.append(f"missing {AUMP_META_VERSION}")

    return not errors, errors


def _validate_mcp_tool(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    errors: list[str] = []
    if payload.get("name") != "aump.evaluate_action":
        errors.append("MCP tool name must be aump.evaluate_action")
    for field in ("inputSchema", "outputSchema"):
        if not isinstance(payload.get(field), dict):
            errors.append(f"missing {field} object")

    annotations = payload.get("annotations")
    if not isinstance(annotations, dict):
        errors.append("missing annotations object")
    else:
        if annotations.get("readOnlyHint") is not True:
            errors.append("aump.evaluate_action must be readOnlyHint=true")
        if annotations.get("destructiveHint") is not False:
            errors.append("aump.evaluate_action must be destructiveHint=false")
        if annotations.get("idempotentHint") is not True:
            errors.append("aump.evaluate_action must be idempotentHint=true")

    return not errors, errors


def _validate_a2a_extension(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    errors: list[str] = []
    for field in ("name", "version", "url"):
        if not payload.get(field):
            errors.append(f"missing Agent Card {field}")
    if not isinstance(payload.get("skills"), list) or not payload["skills"]:
        errors.append("Agent Card must advertise at least one skill")
    if not isinstance(payload.get("defaultInputModes"), list):
        errors.append("missing defaultInputModes array")
    if not isinstance(payload.get("defaultOutputModes"), list):
        errors.append("missing defaultOutputModes array")

    extensions = payload.get("capabilities", {}).get("extensions", [])
    if not isinstance(extensions, list):
        return False, ["capabilities.extensions must be an array"]

    for extension in extensions:
        if not isinstance(extension, dict):
            continue
        if extension.get("uri") == AUMP_A2A_EXTENSION_URI:
            if extension.get("required") is not False:
                errors.append("AUMP A2A extension must be optional by default")
            versions = extension.get("params", {}).get("versions", [])
            if "0.1.0" not in versions:
                errors.append("AUMP A2A extension params.versions must include 0.1.0")
            return not errors, errors
    errors.append("missing AUMP A2A AgentExtension declaration")
    return False, errors


def _validate_a2a_message(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    errors: list[str] = []
    message = payload.get("message", payload)
    if not isinstance(message, dict):
        return False, ["missing A2A message object"]

    if not message.get("messageId"):
        errors.append("missing message.messageId")

    headers = payload.get("headers", {})
    active_extensions = _split_extension_header(headers.get("A2A-Extensions", ""))
    if AUMP_A2A_EXTENSION_URI not in active_extensions:
        errors.append("missing A2A-Extensions activation for AUMP")

    message_extensions = message.get("extensions", [])
    if not isinstance(message_extensions, list) or (
        AUMP_A2A_EXTENSION_URI not in message_extensions
    ):
        errors.append("missing message.extensions AUMP URI")

    metadata = message.get("metadata", {})
    aump_metadata = (
        metadata.get(AUMP_A2A_EXTENSION_URI) if isinstance(metadata, dict) else None
    )
    if not isinstance(aump_metadata, dict):
        errors.append("missing extension-scoped AUMP message metadata")
        return False, errors

    if "mandate" in aump_metadata:
        errors.append("A2A message must not embed full private AUMP mandate")
    for field in ("mandate_id", "mandate_hash", "version"):
        if not aump_metadata.get(field):
            errors.append(f"missing A2A AUMP metadata {field}")
    return not errors, errors


def _validate_ucp_reference(payload: dict[str, Any]) -> tuple[bool, list[str]]:
    errors: list[str] = []
    meta = payload.get("meta")
    if not isinstance(meta, dict):
        return False, ["missing UCP meta object"]

    ucp_agent = meta.get("ucp-agent")
    if not isinstance(ucp_agent, dict) or not ucp_agent.get("profile"):
        errors.append('missing meta["ucp-agent"].profile')

    aump = meta.get("aump")
    if not isinstance(aump, dict):
        errors.append("missing meta.aump reference object")
        return False, errors

    ap2 = payload.get("ap2")
    if isinstance(ap2, dict) and "aump" in ap2:
        errors.append("AUMP references must not be placed under AP2 ap2 namespace")

    if "mandate" in aump:
        errors.append("UCP/AP2 bridge must not embed full private AUMP mandate")
    for field in ("mandate_id", "mandate_hash", "version"):
        if not aump.get(field):
            errors.append(f"missing meta.aump.{field}")
    return not errors, errors


def _split_extension_header(value: Any) -> set[str]:
    if not isinstance(value, str):
        return set()
    return {part.strip() for part in value.split(",") if part.strip()}


def _find_mcp_request_meta(payload: dict[str, Any]) -> dict[str, Any] | None:
    params = payload.get("params")
    if isinstance(params, dict) and isinstance(params.get("_meta"), dict):
        return params["_meta"]
    if isinstance(payload.get("_meta"), dict):
        return payload["_meta"]
    return None
