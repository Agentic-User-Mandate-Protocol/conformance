package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func loadJSON(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

func loadJSONObject(path string) (map[string]any, error) {
	value, err := loadJSON(path)
	if err != nil {
		return nil, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected JSON object", path)
	}
	return obj, nil
}

func writeText(path string, data string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

func WriteReport(path string, data string) error {
	return writeText(path, data)
}

func dumpJSON(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}
