package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGeneratedAnonymousModelInventoryIncludesNestedCommandObjects(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	inventory, err := readAnonymousModelInventory(root)
	if err != nil {
		t.Fatal(err)
	}

	text := string(inventory)

	err = checkGeneratedAnonymousModelInventory(root, text)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"SystemSetRelayStateCommand.System` (anonymous struct)",
		"SystemSetRelayStateCommand.System.SetRelayState` (anonymous struct)",
		"SystemRebootCommand.System.Reboot` (anonymous struct)",
		"SystemSetDevAliasCommand.System.SetDevAlias` (anonymous struct)",
		"LightingGetLightStateCommand.SmartlifeIotSmartbulbLightingservice` (anonymous struct)",
		"LightingTransitionLightStateCommand.SmartlifeIotSmartbulbLightingservice` (anonymous struct)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generated anonymous model inventory omitted %q", want)
		}
	}
}

func TestGeneratedAnonymousModelInventoryRejectsMissingFieldRecord(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	inventory, err := readAnonymousModelInventory(root)
	if err != nil {
		t.Fatal(err)
	}

	const row = "| `api/passthrough.openapi.yaml#SystemRebootCommand/properties/system/properties/reboot` |"

	changed := strings.Replace(string(inventory), row, "| `removed inventory row` |", 1)
	if changed == string(inventory) {
		t.Fatal("test did not remove the expected anonymous-object record")
	}

	err = checkGeneratedAnonymousModelInventory(root, changed)
	if err == nil || !strings.Contains(err.Error(), "anonymous generated model inventory is incomplete or stale") {
		t.Fatalf("checkGeneratedAnonymousModelInventory() error = %v, want missing-row rejection", err)
	}
}

func TestGeneratedAnonymousModelInventoryFollowsArrayItems(t *testing.T) {
	t.Parallel()

	const goSource = `package fixture
type Root struct {
  Children []struct { Value int ` + "`json:\"value\"`" + ` } ` + "`json:\"children\"`" + `
}
`

	file, err := parser.ParseFile(token.NewFileSet(), "models.gen.go", goSource, 0)
	if err != nil {
		t.Fatal(err)
	}

	const schemaSource = `type: object
properties:
  children:
    type: array
    items:
      type: object
      properties:
        value:
          type: integer
`

	var schemaNode generatedScalarSchemaNode

	err = yaml.Unmarshal([]byte(schemaSource), &schemaNode)
	if err != nil {
		t.Fatal(err)
	}

	componentPath := generatedSchemaComponentPointer("Root")
	source := generatedModelSource{
		schemaPath: "api/synthetic.yaml",
		outputPath: "pkg/dependencymodels/models.gen.go",
		configPath: "",
		command:    "oapi-codegen synthetic",
	}

	var objects []generatedAnonymousModelObject

	err = collectGeneratedAnonymousModelObjects(
		source,
		file,
		"Root",
		schemaNode,
		ast.NewIdent("Root"),
		componentPath,
		componentPath,
		nil,
		"pkg/example.go:Build",
		&objects,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(objects) != 1 || objects[0].goFieldPath != "pkg/dependencymodels.Root.Children.[]" {
		t.Fatalf("generated anonymous objects = %#v, want one array-item object path", objects)
	}
}
