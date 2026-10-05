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

	fixture := mergedWireSchemaForTest(t)

	schemas := object(object(fixture["components"])["schemas"])
	lightState := object(schemas["LightTransitionState"])
	lightState["additionalProperties"] = false

	err := validateWireSchemaShape(fixture)
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

func TestWireSchemasAreSplitByResponsibility(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"../../api/cloud-envelope.openapi.yaml": "CloudRequest",
		"../../api/authentication.openapi.yaml": "LoginParams",
		"../../api/devices.openapi.yaml":        "Device",
		"../../api/passthrough.openapi.yaml":    "PassthroughCommand",
	}
	for path, expectedComponent := range fixtures {
		//nolint:gosec // Fixtures use fixed repository-relative schema paths.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		var document map[string]any

		err = yaml.Unmarshal(data, &document)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		if _, found := object(object(document["components"])["schemas"])[expectedComponent]; !found {
			t.Errorf("%s does not define responsibility component %s", path, expectedComponent)
		}
	}

	merged := mergedWireSchemaForTest(t)
	if count := len(object(object(merged["components"])["schemas"])); count != 31 {
		t.Fatalf("merged wire schema has %d components, want 31", count)
	}
}

func mergedWireSchemaForTest(t *testing.T) map[string]any {
	t.Helper()

	paths := []string{
		"../../api/openapi.yaml",
		"../../api/cloud-envelope.openapi.yaml",
		"../../api/authentication.openapi.yaml",
		"../../api/devices.openapi.yaml",
		"../../api/passthrough.openapi.yaml",
	}

	documents := make([]map[string]any, 0, len(paths))

	for _, path := range paths {
		//nolint:gosec // Fixtures use fixed repository-relative schema paths.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		var document map[string]any

		err = yaml.Unmarshal(data, &document)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		documents = append(documents, document)
	}

	return mergeSchemaComponents(documents)
}
