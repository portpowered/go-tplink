package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnumMetadataMustMatchSchemaValues(t *testing.T) {
	t.Parallel()

	var schema map[string]any

	err := yaml.Unmarshal([]byte(`
type: string
enum: [known]
x-go-wire-constant-names:
  - value: invented
    name: InventedValue
`), &schema)
	if err != nil {
		t.Fatalf("unmarshal schema fixture: %v", err)
	}

	err = collectEnumMetadata(&generator{constants: make(map[string]constant)}, schema)
	if err == nil || !strings.Contains(err.Error(), "absent from enum") {
		t.Fatalf("collectEnumMetadata() error = %v, want enum membership failure", err)
	}
}

func TestConflictingWireDefinitionsFail(t *testing.T) {
	t.Parallel()

	gen := generator{constants: make(map[string]constant)}

	firstErr := gen.add("WireKey", `"old"`, false)
	if firstErr != nil {
		t.Fatalf("add first definition: %v", firstErr)
	}

	conflictErr := gen.add("WireKey", `"new"`, false)
	if conflictErr == nil || !strings.Contains(conflictErr.Error(), "conflicting schema values") {
		t.Fatalf("add conflicting definition error = %v", conflictErr)
	}
}

func TestRequiredNestedPayloadAndWireDefinitionsAreGated(t *testing.T) {
	t.Parallel()

	gen := generator{constants: make(map[string]constant)}

	err := gen.validateRequiredConstants()
	if err == nil || !strings.Contains(err.Error(), "required generated wire constant") {
		t.Fatalf("validateRequiredConstants() error = %v, want missing-definition failure", err)
	}
}

func TestCallerOpenStateAndTypedResultVariantsAreRequired(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read API schema: %v", err)
	}

	var fixture map[string]any

	err = yaml.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatalf("parse API schema: %v", err)
	}

	schemas := object(object(fixture["components"])["schemas"])
	lightState := object(schemas["LightTransitionState"])
	lightState["additionalProperties"] = false

	err = validateWireSchemaShape(fixture)
	if err == nil || !strings.Contains(err.Error(), "LightTransitionState") {
		t.Fatalf("validateWireSchemaShape() error = %v, want caller-open state failure", err)
	}

	lightState["additionalProperties"] = true
	result := object(schemas["PassthroughCommandResult"])
	result["anyOf"] = []any{"system"}

	err = validateWireSchemaShape(fixture)
	if err == nil || !strings.Contains(err.Error(), "PassthroughCommandResult") {
		t.Fatalf("validateWireSchemaShape() error = %v, want result variant failure", err)
	}

	delete(schemas, "LightingTransitionLightStateCommand")

	err = validateWireSchemaShape(fixture)
	if err == nil || !strings.Contains(err.Error(), "LightingTransitionLightStateCommand") {
		t.Fatalf("validateWireSchemaShape() error = %v, want missing nested payload failure", err)
	}
}
