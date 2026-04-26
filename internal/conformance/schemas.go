package conformance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var schemaFiles = map[string]string{
	"mandate":           "mandate.schema.json",
	"profile":           "profile.schema.json",
	"action-evaluation": "action-evaluation.schema.json",
	"evidence-event":    "evidence-event.schema.json",
}

type SchemaRegistry struct {
	schemas map[string]*jsonschema.Schema
}

func LoadSchemas(root string) (*SchemaRegistry, error) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()

	for name, filename := range schemaFiles {
		path := filepath.Join(root, "schemas", filename)
		doc, err := loadJSON(path)
		if err != nil {
			return nil, err
		}
		if err := compiler.AddResource(schemaURL(name), doc); err != nil {
			return nil, err
		}
	}

	loaded := map[string]*jsonschema.Schema{}
	for name := range schemaFiles {
		schema, err := compiler.Compile(schemaURL(name))
		if err != nil {
			return nil, err
		}
		loaded[name] = schema
	}
	return &SchemaRegistry{schemas: loaded}, nil
}

func (r *SchemaRegistry) Validate(name string, payload any) []string {
	schema, ok := r.schemas[name]
	if !ok {
		return []string{fmt.Sprintf("unknown schema %q", name)}
	}
	payload = normalizeJSONValue(payload)
	err := schema.Validate(payload)
	if err == nil {
		return nil
	}

	var validationErr *jsonschema.ValidationError
	if errors.As(err, &validationErr) {
		return flattenValidationErrors(validationErr)
	}
	return []string{err.Error()}
}

func normalizeJSONValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return value
	}
	return normalized
}

func schemaURL(name string) string {
	return "schema://aump/" + name
}

func flattenValidationErrors(err *jsonschema.ValidationError) []string {
	if len(err.Causes) == 0 {
		message := err.Error()
		return []string{jsonPath(err.InstanceLocation) + ": " + message}
	}

	result := []string{}
	for _, cause := range err.Causes {
		result = append(result, flattenValidationErrors(cause)...)
	}
	return result
}
