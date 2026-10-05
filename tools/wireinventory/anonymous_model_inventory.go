package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	anonymousInventoryStart = "<!-- wireinventory:anonymous-models:start -->"
	anonymousInventoryEnd   = "<!-- wireinventory:anonymous-models:end -->"
	writeAnonymousInventory = "-write-anonymous-model-inventory"
	minimumInventoryColumns = 6
)

const anonymousInventoryRelativePath = "docs/wire-model-inventory.md"

type generatedAnonymousModelObject struct {
	schemaOwner string
	goFieldPath string
	outputPath  string
	command     string
	callSite    string
}

func renderGeneratedAnonymousModelInventory(root, inventoryText string) (string, error) {
	objects, err := generatedAnonymousModelObjects(root, inventoryText)
	if err != nil {
		return "", err
	}

	sort.Slice(objects, func(left, right int) bool {
		if objects[left].schemaOwner != objects[right].schemaOwner {
			return objects[left].schemaOwner < objects[right].schemaOwner
		}

		return objects[left].goFieldPath < objects[right].goFieldPath
	})

	var rendered strings.Builder

	rendered.WriteString("## Anonymous generated model object inventory\n\n")
	rendered.WriteString(
		"Each row maps an inline schema object to its anonymous generated Go field path " +
			"and the parent component's actual use. This block is generated from the " +
			"checked-in schemas, generated Go AST, and component call-site records.\n\n",
	)
	rendered.WriteString(
		"| Schema owner and JSON path | Generated Go field path | Generator output | " +
			"Generator command | Actual call site |\n",
	)
	rendered.WriteString("| --- | --- | --- | --- | --- |\n")

	for _, object := range objects {
		fmt.Fprintf(&rendered, "| `%s` | `%s` (anonymous struct) | `%s` | `%s` | %s |\n",
			object.schemaOwner, object.goFieldPath, object.outputPath, object.command, object.callSite)
	}

	return strings.TrimSpace(rendered.String()), nil
}

func generatedAnonymousModelObjects(root, inventoryText string) ([]generatedAnonymousModelObject, error) {
	var objects []generatedAnonymousModelObject

	for _, source := range generatedModelSources() {
		sourceObjects, err := generatedAnonymousModelObjectsForSource(root, inventoryText, source)
		if err != nil {
			return nil, err
		}

		objects = append(objects, sourceObjects...)
	}

	return objects, nil
}

func generatedAnonymousModelObjectsForSource(
	root, inventoryText string,
	source generatedModelSource,
) ([]generatedAnonymousModelObject, error) {
	schemaPath := filepath.Join(root, filepath.FromSlash(source.schemaPath))

	schemas, err := loadGeneratedSchemaDocument(schemaPath)
	if err != nil {
		return nil, err
	}

	outputPath := filepath.Join(root, filepath.FromSlash(source.outputPath))

	file, err := parser.ParseFile(token.NewFileSet(), outputPath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse generated models for anonymous inventory %s: %w", source.outputPath, err)
	}

	schemaNames := make([]string, 0, len(schemas))
	for schemaName := range schemas {
		schemaNames = append(schemaNames, schemaName)
	}

	sort.Strings(schemaNames)

	var objects []generatedAnonymousModelObject
	for _, schemaName := range schemaNames {
		err := appendGeneratedAnonymousSchemaObjects(
			inventoryText, source, file, schemaName, schemas[schemaName], &objects,
		)
		if err != nil {
			return nil, err
		}
	}

	return objects, nil
}

func appendGeneratedAnonymousSchemaObjects(
	inventoryText string,
	source generatedModelSource,
	file *ast.File,
	schemaName string,
	schemaNode yaml.Node,
	objects *[]generatedAnonymousModelObject,
) error {
	if generatedStructForSchema(file, schemaName) == nil {
		return nil
	}

	callSite, found := generatedModelInventoryCallSite(inventoryText, source.schemaPath, schemaName)
	if !found || callSite == "" {
		return inventoryError(
			"anonymous model inventory cannot resolve the actual call site for %s#%s",
			source.schemaPath,
			schemaName,
		)
	}

	var rootNode generatedScalarSchemaNode

	err := schemaNode.Decode(&rootNode)
	if err != nil {
		return fmt.Errorf("decode schema component %s#%s: %w", source.schemaPath, schemaName, err)
	}

	if rootNode.Ref != "" || len(rootNode.OneOf) != 0 || len(rootNode.AnyOf) != 0 {
		return nil
	}

	componentPointer := generatedSchemaComponentPointer(schemaName)

	return collectGeneratedAnonymousModelObjects(
		source,
		file,
		schemaName,
		rootNode,
		ast.NewIdent(schemaName),
		componentPointer,
		componentPointer,
		nil,
		callSite,
		objects,
	)
}

func collectGeneratedAnonymousModelObjects(
	source generatedModelSource,
	file *ast.File,
	wireSchemaOwner string,
	schemaNode generatedScalarSchemaNode,
	modelExpression ast.Expr,
	schemaPointer, jsonPointer string,
	goPath []string,
	callSite string,
	objects *[]generatedAnonymousModelObject,
) error {
	if schemaNode.Ref != "" {
		return nil
	}

	appendGeneratedAnonymousModelObject(
		source,
		wireSchemaOwner,
		schemaNode,
		modelExpression,
		schemaPointer,
		goPath,
		callSite,
		objects,
	)

	if schemaNode.Items.Kind != 0 {
		err := collectGeneratedAnonymousArrayItem(
			source,
			file,
			wireSchemaOwner,
			schemaNode,
			modelExpression,
			schemaPointer,
			jsonPointer,
			goPath,
			callSite,
			objects,
		)
		if err != nil {
			return err
		}
	}

	return collectGeneratedAnonymousProperties(
		source,
		file,
		wireSchemaOwner,
		schemaNode,
		modelExpression,
		schemaPointer,
		jsonPointer,
		goPath,
		callSite,
		objects,
	)
}

func appendGeneratedAnonymousModelObject(
	source generatedModelSource,
	wireSchemaOwner string,
	schemaNode generatedScalarSchemaNode,
	modelExpression ast.Expr,
	schemaPointer string,
	goPath []string,
	callSite string,
	objects *[]generatedAnonymousModelObject,
) {
	if !isGeneratedInlineObject(schemaNode) || generatedAnonymousStruct(modelExpression) == nil || len(goPath) == 0 {
		return
	}

	packagePath := filepath.ToSlash(filepath.Dir(source.outputPath))
	generatedPath := packagePath + "." + wireSchemaOwner + "." + strings.Join(goPath, ".")
	*objects = append(*objects, generatedAnonymousModelObject{
		schemaOwner: source.schemaPath + "#" + wireSchemaOwner + strings.TrimPrefix(
			schemaPointer,
			generatedSchemaComponentPointer(wireSchemaOwner),
		),
		goFieldPath: generatedPath,
		outputPath:  source.outputPath,
		command:     source.command,
		callSite:    callSite,
	})
}

func collectGeneratedAnonymousArrayItem(
	source generatedModelSource,
	file *ast.File,
	wireSchemaOwner string,
	schemaNode generatedScalarSchemaNode,
	modelExpression ast.Expr,
	schemaPointer, jsonPointer string,
	goPath []string,
	callSite string,
	objects *[]generatedAnonymousModelObject,
) error {
	itemExpression, found := generatedArrayElementType(modelExpression)
	if !found {
		itemExpression = nil
	}

	var itemSchema generatedScalarSchemaNode

	err := schemaNode.Items.Decode(&itemSchema)
	if err != nil {
		return fmt.Errorf("decode array item in %s: %w", schemaPointer, err)
	}

	return collectGeneratedAnonymousModelObjects(
		source,
		file,
		wireSchemaOwner,
		itemSchema,
		itemExpression,
		schemaPointer+"/items",
		jsonPointer+"/items",
		appendGeneratedPath(goPath, "[]"),
		callSite,
		objects,
	)
}

func collectGeneratedAnonymousProperties(
	source generatedModelSource,
	file *ast.File,
	wireSchemaOwner string,
	schemaNode generatedScalarSchemaNode,
	modelExpression ast.Expr,
	schemaPointer, jsonPointer string,
	goPath []string,
	callSite string,
	objects *[]generatedAnonymousModelObject,
) error {
	if len(schemaNode.Properties) == 0 {
		return nil
	}

	structure := generatedStructForExpression(file, modelExpression, make(map[string]bool))
	for propertyName, propertyNode := range schemaNode.Properties {
		err := collectGeneratedAnonymousProperty(
			source,
			file,
			wireSchemaOwner,
			structure,
			propertyName,
			propertyNode,
			schemaPointer,
			jsonPointer,
			goPath,
			callSite,
			objects,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func collectGeneratedAnonymousProperty(
	source generatedModelSource,
	file *ast.File,
	wireSchemaOwner string,
	structure *ast.StructType,
	propertyName string,
	propertyNode yaml.Node,
	schemaPointer, jsonPointer string,
	goPath []string,
	callSite string,
	objects *[]generatedAnonymousModelObject,
) error {
	var propertyExpression ast.Expr

	var fieldName string

	if field, name := generatedJSONStructField(structure, propertyName); field != nil {
		propertyExpression = field.Type
		fieldName = name
	}

	var childSchema generatedScalarSchemaNode

	err := propertyNode.Decode(&childSchema)
	if err != nil {
		return fmt.Errorf("decode property %s in %s: %w", propertyName, schemaPointer, err)
	}

	nextGoPath := append([]string(nil), goPath...)
	if fieldName != "" {
		nextGoPath = append(nextGoPath, fieldName)
	}

	propertyPointer := "/properties/" + escapeGeneratedJSONPointer(propertyName)

	return collectGeneratedAnonymousModelObjects(
		source,
		file,
		wireSchemaOwner,
		childSchema,
		propertyExpression,
		schemaPointer+propertyPointer,
		jsonPointer+propertyPointer,
		nextGoPath,
		callSite,
		objects,
	)
}

func isGeneratedInlineObject(schema generatedScalarSchemaNode) bool {
	return schema.Ref == "" && (schema.Type == "object" || len(schema.Properties) != 0)
}

func generatedModelInventoryCallSite(inventoryText, schemaPath, schemaName string) (string, bool) {
	rowPrefix := "| `" + schemaPath + "#" + schemaName + "` |"

	for line := range strings.SplitSeq(inventoryText, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, rowPrefix) {
			continue
		}

		columns := strings.Split(line, "|")
		if len(columns) < minimumInventoryColumns {
			return "", false
		}

		return strings.TrimSpace(columns[5]), true
	}

	return "", false
}

func anonymousModelInventoryBlock(inventoryText string) (int, int, error) {
	start := strings.Index(inventoryText, anonymousInventoryStart)

	end := strings.Index(inventoryText, anonymousInventoryEnd)
	if start < 0 || end < 0 || end <= start {
		return 0, 0, inventoryError("wire-model inventory is missing the generated anonymous-model markers")
	}

	return start, end, nil
}

func checkGeneratedAnonymousModelInventory(root, inventoryText string) error {
	start, end, err := anonymousModelInventoryBlock(inventoryText)
	if err != nil {
		return err
	}

	want, err := renderGeneratedAnonymousModelInventory(root, inventoryText)
	if err != nil {
		return err
	}

	actual := strings.TrimSpace(inventoryText[start+len(anonymousInventoryStart) : end])
	if actual != want {
		return inventoryError(
			"anonymous generated model inventory is incomplete or stale; run go run ./tools/wireinventory %s",
			writeAnonymousInventory,
		)
	}

	return nil
}

func writeGeneratedAnonymousModelInventory(root string) error {
	contents, err := readAnonymousModelInventory(root)
	if err != nil {
		return fmt.Errorf("read wire model inventory for generation: %w", err)
	}

	text := string(contents)

	start, end, err := anonymousModelInventoryBlock(text)
	if err != nil {
		return err
	}

	rendered, err := renderGeneratedAnonymousModelInventory(root, text)
	if err != nil {
		return err
	}

	updated := text[:start+len(anonymousInventoryStart)] + "\n\n" + rendered + "\n\n" + text[end:]

	repositoryRoot, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open repository root for inventory generation: %w", err)
	}

	inventoryPath := filepath.FromSlash(anonymousInventoryRelativePath)

	fileInfo, err := repositoryRoot.Stat(inventoryPath)
	if err != nil {
		err = errors.Join(err, repositoryRoot.Close())

		return fmt.Errorf("stat wire model inventory: %w", err)
	}

	permissions := fileInfo.Mode().Perm()

	writeErr := writeAnonymousInventoryFile(
		repositoryRoot,
		inventoryPath,
		[]byte(updated),
		permissions,
	)

	writeErr = errors.Join(writeErr, repositoryRoot.Close())
	if writeErr != nil {
		return fmt.Errorf("write wire model inventory: %w", writeErr)
	}

	return nil
}

func readAnonymousModelInventory(root string) ([]byte, error) {
	repositoryRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open repository root for inventory read: %w", err)
	}

	contents, readErr := fs.ReadFile(repositoryRoot.FS(), anonymousInventoryRelativePath)

	readErr = errors.Join(readErr, repositoryRoot.Close())
	if readErr != nil {
		return nil, fmt.Errorf("read wire model inventory: %w", readErr)
	}

	return contents, nil
}

func writeAnonymousInventoryFile(
	repositoryRoot *os.Root,
	relativePath string,
	contents []byte,
	permissions os.FileMode,
) error {
	file, err := repositoryRoot.OpenFile(
		relativePath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		permissions,
	)
	if err != nil {
		return fmt.Errorf("open wire model inventory for writing: %w", err)
	}

	written, writeErr := file.Write(contents)
	closeErr := file.Close()
	writeErr = errors.Join(writeErr, closeErr)

	if written != len(contents) {
		return fmt.Errorf("write wire model inventory: %w", errors.Join(writeErr, io.ErrShortWrite))
	}

	if writeErr != nil {
		return fmt.Errorf("write wire model inventory: %w", writeErr)
	}

	return nil
}
