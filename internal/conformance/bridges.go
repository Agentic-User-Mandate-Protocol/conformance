package conformance

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	aumpMetaID      = "org.agentic-user-mandate-protocol/aump_mandate_id"
	aumpMetaHash    = "org.agentic-user-mandate-protocol/aump_mandate_hash"
	aumpMetaVersion = "org.agentic-user-mandate-protocol/aump_version"
	aumpA2AURI      = "https://agentic-user-mandate-protocol.github.io/spec/bindings/a2a/v0.1"
)

var metaKeyRE = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*(?:\.[A-Za-z][A-Za-z0-9-]*)*/)?[A-Za-z0-9](?:[A-Za-z0-9_.-]*[A-Za-z0-9])?$`)

func validateBridge(payload map[string]any, bridgeType string) (bool, []string) {
	switch bridgeType {
	case "mcp_meta":
		return validateMCPMeta(payload)
	case "mcp_tool":
		return validateMCPTool(payload)
	case "a2a_extension":
		return validateA2AExtension(payload)
	case "a2a_message":
		return validateA2AMessage(payload)
	case "ucp_reference":
		return validateUCPReference(payload)
	default:
		return false, []string{fmt.Sprintf("unknown bridge_type %q", bridgeType)}
	}
}

func validateMCPMeta(payload map[string]any) (bool, []string) {
	meta := findMCPRequestMeta(payload)
	if meta == nil {
		return false, []string{"missing MCP _meta object"}
	}

	errors := []string{}
	for key := range meta {
		if !metaKeyRE.MatchString(key) {
			errors = append(errors, fmt.Sprintf("invalid MCP _meta key %q", key))
		}
		prefix := ""
		if strings.Contains(key, "/") {
			prefix = strings.SplitN(key, "/", 2)[0]
		}
		labels := strings.Split(prefix, ".")
		if len(labels) >= 2 && (labels[1] == "mcp" || labels[1] == "modelcontextprotocol") {
			errors = append(errors, fmt.Sprintf("reserved MCP _meta prefix %q", prefix))
		}
	}
	if getString(meta, aumpMetaID) == "" {
		errors = append(errors, "missing "+aumpMetaID)
	}
	if getString(meta, aumpMetaHash) == "" {
		errors = append(errors, "missing "+aumpMetaHash)
	}
	if getString(meta, aumpMetaVersion) == "" {
		errors = append(errors, "missing "+aumpMetaVersion)
	}
	return len(errors) == 0, errors
}

func validateMCPTool(payload map[string]any) (bool, []string) {
	errors := []string{}
	if getString(payload, "name") != "aump.evaluate_action" {
		errors = append(errors, "MCP tool name must be aump.evaluate_action")
	}
	for _, field := range []string{"inputSchema", "outputSchema"} {
		if _, ok := payload[field].(map[string]any); !ok {
			errors = append(errors, "missing "+field+" object")
		}
	}
	annotations, ok := payload["annotations"].(map[string]any)
	if !ok {
		errors = append(errors, "missing annotations object")
	} else {
		if annotations["readOnlyHint"] != true {
			errors = append(errors, "aump.evaluate_action must be readOnlyHint=true")
		}
		if annotations["destructiveHint"] != false {
			errors = append(errors, "aump.evaluate_action must be destructiveHint=false")
		}
		if annotations["idempotentHint"] != true {
			errors = append(errors, "aump.evaluate_action must be idempotentHint=true")
		}
	}
	return len(errors) == 0, errors
}

func validateA2AExtension(payload map[string]any) (bool, []string) {
	errors := []string{}
	for _, field := range []string{"name", "version", "url"} {
		if getString(payload, field) == "" {
			errors = append(errors, "missing Agent Card "+field)
		}
	}
	if len(stringSlice(payload["defaultInputModes"])) == 0 {
		errors = append(errors, "missing defaultInputModes array")
	}
	if len(stringSlice(payload["defaultOutputModes"])) == 0 {
		errors = append(errors, "missing defaultOutputModes array")
	}
	if skills, ok := payload["skills"].([]any); !ok || len(skills) == 0 {
		errors = append(errors, "Agent Card must advertise at least one skill")
	}

	extensions, ok := getObj(payload, "capabilities")["extensions"].([]any)
	if !ok {
		return false, []string{"capabilities.extensions must be an array"}
	}

	for _, raw := range extensions {
		extension, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if getString(extension, "uri") == aumpA2AURI {
			if extension["required"] != false {
				errors = append(errors, "AUMP A2A extension must be optional by default")
			}
			versions := stringSet(getObj(extension, "params")["versions"])
			if !versions["0.1.0"] {
				errors = append(errors, "AUMP A2A extension params.versions must include 0.1.0")
			}
			return len(errors) == 0, errors
		}
	}
	errors = append(errors, "missing AUMP A2A AgentExtension declaration")
	return false, errors
}

func validateA2AMessage(payload map[string]any) (bool, []string) {
	errors := []string{}
	message := payload
	if rawMessage, ok := payload["message"].(map[string]any); ok {
		message = rawMessage
	}
	if getString(message, "messageId") == "" {
		errors = append(errors, "missing message.messageId")
	}

	headers := getObj(payload, "headers")
	if !splitExtensionHeader(getString(headers, "A2A-Extensions"))[aumpA2AURI] {
		errors = append(errors, "missing A2A-Extensions activation for AUMP")
	}
	if !stringSet(message["extensions"])[aumpA2AURI] {
		errors = append(errors, "missing message.extensions AUMP URI")
	}

	metadata, ok := message["metadata"].(map[string]any)
	if !ok {
		errors = append(errors, "missing extension-scoped AUMP message metadata")
		return false, errors
	}
	aumpMetadata, ok := metadata[aumpA2AURI].(map[string]any)
	if !ok {
		errors = append(errors, "missing extension-scoped AUMP message metadata")
		return false, errors
	}
	if _, ok := aumpMetadata["mandate"]; ok {
		errors = append(errors, "A2A message must not embed full private AUMP mandate")
	}
	for _, field := range []string{"mandate_id", "mandate_hash", "version"} {
		if getString(aumpMetadata, field) == "" {
			errors = append(errors, "missing A2A AUMP metadata "+field)
		}
	}
	return len(errors) == 0, errors
}

func validateUCPReference(payload map[string]any) (bool, []string) {
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		return false, []string{"missing UCP meta object"}
	}

	errors := []string{}
	ucpAgent, ok := meta["ucp-agent"].(map[string]any)
	if !ok || getString(ucpAgent, "profile") == "" {
		errors = append(errors, `missing meta["ucp-agent"].profile`)
	}

	aump, ok := meta["aump"].(map[string]any)
	if !ok {
		errors = append(errors, "missing meta.aump reference object")
		return false, errors
	}

	if ap2, ok := payload["ap2"].(map[string]any); ok {
		if _, exists := ap2["aump"]; exists {
			errors = append(errors, "AUMP references must not be placed under AP2 ap2 namespace")
		}
	}
	if _, ok := aump["mandate"]; ok {
		errors = append(errors, "UCP/AP2 bridge must not embed full private AUMP mandate")
	}
	for _, field := range []string{"mandate_id", "mandate_hash", "version"} {
		if getString(aump, field) == "" {
			errors = append(errors, "missing meta.aump."+field)
		}
	}
	return len(errors) == 0, errors
}

func findMCPRequestMeta(payload map[string]any) map[string]any {
	if params, ok := payload["params"].(map[string]any); ok {
		if meta, ok := params["_meta"].(map[string]any); ok {
			return meta
		}
	}
	if meta, ok := payload["_meta"].(map[string]any); ok {
		return meta
	}
	return nil
}

func splitExtensionHeader(value string) map[string]bool {
	result := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result[part] = true
		}
	}
	return result
}
