package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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

func TestGeneratedAnonymousModelInventoryNormalizesCRLFWithoutHidingDrift(t *testing.T) {
	t.Parallel()

	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	copyGeneratedAnonymousInventoryInputs(t, sourceRoot, root)
	inventory := readAnonymousInventoryFixture(t, root)

	crlfInventory := strings.ReplaceAll(string(inventory), "\n", "\r\n")
	writeAnonymousInventoryFixture(t, root, crlfInventory)

	err = checkAnonymousSchemaInventoryFixture(root)
	if err != nil {
		t.Fatalf("checkGeneratedSchemaModels() rejected CRLF checkout: %v", err)
	}

	const original = "SystemRebootCommand.System.Reboot` (anonymous struct)"

	const changed = "SystemRebootCommand.System.Wrong` (anonymous struct)"

	driftedInventory := strings.Replace(crlfInventory, original, changed, 1)

	if driftedInventory == crlfInventory {
		t.Fatal("test fixture did not find the generated anonymous-object record")
	}

	writeAnonymousInventoryFixture(t, root, driftedInventory)

	err = checkAnonymousSchemaInventoryFixture(root)
	if err == nil || !strings.Contains(err.Error(), "anonymous generated model inventory is incomplete or stale") {
		t.Fatalf("checkGeneratedSchemaModels() error = %v, want drift rejection", err)
	}
}

func TestWriteGeneratedAnonymousModelInventoryNormalizesCRLFCheckout(t *testing.T) {
	t.Parallel()

	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	copyGeneratedAnonymousInventoryInputs(t, sourceRoot, root)

	inventory := readAnonymousInventoryFixture(t, root)

	crlfInventory := strings.ReplaceAll(string(inventory), "\n", "\r\n")
	writeAnonymousInventoryFixture(t, root, crlfInventory)

	err = writeGeneratedAnonymousModelInventory(root)
	if err != nil {
		t.Fatalf("writeGeneratedAnonymousModelInventory() failed: %v", err)
	}

	updated := readAnonymousInventoryFixture(t, root)

	if strings.Contains(string(updated), "\r") {
		t.Fatal("writeGeneratedAnonymousModelInventory() left non-LF line endings")
	}

	err = checkGeneratedAnonymousModelInventory(root, string(updated))
	if err != nil {
		t.Fatalf("checkGeneratedAnonymousModelInventory() rejected generated inventory: %v", err)
	}
}

func readAnonymousInventoryFixture(t *testing.T, root string) []byte {
	t.Helper()

	repositoryRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := repositoryRoot.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	contents, err := fs.ReadFile(repositoryRoot.FS(), anonymousInventoryRelativePath)
	if err != nil {
		t.Fatal(err)
	}

	return contents
}

func writeAnonymousInventoryFixture(t *testing.T, root, contents string) {
	t.Helper()

	repositoryRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := repositoryRoot.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	err = writeAnonymousInventoryFile(
		repositoryRoot,
		anonymousInventoryRelativePath,
		[]byte(contents),
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func checkAnonymousSchemaInventoryFixture(root string) error {
	generatedTypes, err := readGeneratedModelTypes(root)
	if err != nil {
		return err
	}

	return checkGeneratedSchemaModels(root, generatedTypes)
}

func copyGeneratedAnonymousInventoryInputs(t *testing.T, sourceRoot, destinationRoot string) {
	t.Helper()

	sources := generatedModelSources()
	compatibilityOutputs := compatibilityModelOutputs()
	paths := make([]string, 0, 2+len(sources)*3+len(compatibilityOutputs))
	paths = append(paths, anonymousInventoryRelativePath)

	for _, source := range sources {
		paths = append(paths, source.schemaPath, source.outputPath, source.configPath)
	}

	paths = append(paths, compatibilityOutputs...)

	paths = append(paths, cliImportsTemplatePath)

	for _, relativePath := range paths {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(relativePath))
		//nolint:gosec // The test copies fixed repository fixtures into a temporary directory.
		contents, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}

		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(relativePath))

		err = os.MkdirAll(filepath.Dir(destinationPath), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		//nolint:gosec // The destination stays under the test's temporary directory.
		err = os.WriteFile(destinationPath, contents, 0o600)
		if err != nil {
			t.Fatal(err)
		}
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
