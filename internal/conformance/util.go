package conformance

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func parseDateTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func obj(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	asObj, _ := value.(map[string]any)
	if asObj == nil {
		return map[string]any{}
	}
	return asObj
}

func getObj(m map[string]any, key string) map[string]any {
	return obj(m[key])
}

func getString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}

func getBool(m map[string]any, key string) bool {
	value, _ := m[key].(bool)
	return value
}

func getFloat(m map[string]any, key string) (float64, bool) {
	switch value := m[key].(type) {
	case json.Number:
		number, err := value.Float64()
		return number, err == nil
	case float64:
		return value, true
	case int:
		return float64(value), true
	default:
		return 0, false
	}
}

func stringSlice(value any) []string {
	if typed, ok := value.([]string); ok {
		return typed
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func stringSet(value any) map[string]bool {
	result := map[string]bool{}
	for _, item := range stringSlice(value) {
		result[item] = true
	}
	return result
}

func stableUnique(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func equalStringSlices(actual []string, expected []string) bool {
	a := append([]string{}, actual...)
	e := append([]string{}, expected...)
	sort.Strings(a)
	sort.Strings(e)
	if len(a) != len(e) {
		return false
	}
	for i := range a {
		if a[i] != e[i] {
			return false
		}
	}
	return true
}

func joinMessages(messages []string) string {
	return strings.Join(messages, "; ")
}

func jsonPath(parts []string) string {
	if len(parts) == 0 {
		return "$"
	}
	var builder strings.Builder
	builder.WriteString("$")
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err == nil {
			builder.WriteString("[")
			builder.WriteString(part)
			builder.WriteString("]")
		} else {
			builder.WriteString(".")
			builder.WriteString(part)
		}
	}
	return builder.String()
}

func asMap(value any, path string) (map[string]any, error) {
	if asObj, ok := value.(map[string]any); ok {
		return asObj, nil
	}
	return nil, fmt.Errorf("%s: expected object", path)
}
