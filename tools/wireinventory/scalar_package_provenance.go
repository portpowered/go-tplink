package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	goTrueLiteral  = "true"
	goFalseLiteral = "false"
)

type generatedScalarMetadata struct {
	constants      map[string]map[string]bool
	constantValues map[string]map[string]string
	fields         map[string]map[string]map[string]bool
	closedEnums    map[string]map[string]map[string]map[string]bool
	modelFields    map[string]map[string]map[string]generatedModelFieldType
	modelTypes     map[string]map[string]bool
}

type generatedModelFieldType struct {
	packagePath string
	typeName    string
}

func readGeneratedScalarMetadata(root string) (generatedScalarMetadata, error) {
	metadata := generatedScalarMetadata{
		constants:      make(map[string]map[string]bool),
		constantValues: make(map[string]map[string]string),
		fields:         make(map[string]map[string]map[string]bool),
		closedEnums:    make(map[string]map[string]map[string]map[string]bool),
		modelFields:    make(map[string]map[string]map[string]generatedModelFieldType),
		modelTypes:     make(map[string]map[string]bool),
	}

	scalarTypes, err := readGeneratedModelScalars(root)
	if err != nil {
		return generatedScalarMetadata{}, err
	}

	schemaPaths := make(map[string]string)

	for _, source := range generatedModelSources() {
		if isDependencyModelOutput(source.outputPath) {
			schemaPaths[source.outputPath] = source.schemaPath
		}
	}

	for _, output := range generatedDependencyModelOutputs() {
		err = mergeGeneratedOutputScalarMetadata(root, output, schemaPaths, scalarTypes, &metadata)
		if err != nil {
			return generatedScalarMetadata{}, err
		}
	}

	err = mergeGeneratedWireConstantMetadata(root, &metadata)
	if err != nil {
		return generatedScalarMetadata{}, err
	}

	err = mergeGeneratedCompatibilityScalarMetadata(root, &metadata)
	if err != nil {
		return generatedScalarMetadata{}, err
	}

	return metadata, nil
}

func mergeGeneratedOutputScalarMetadata(
	root, output string,
	schemaPaths map[string]string,
	scalarTypes map[string]bool,
	metadata *generatedScalarMetadata,
) error {
	path := filepath.Join(root, filepath.FromSlash(output))

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse generated scalar metadata %s: %w", output, err)
	}

	importPath := generatedModelImportPath(output)
	mergeGeneratedConstantNames(metadata.constants, importPath, generatedConstantNames(file))
	mergeGeneratedConstantValues(metadata.constantValues, importPath, generatedConstantValues(file))
	mergeGeneratedScalarFields(metadata.fields, importPath, generatedScalarFieldNames(file, scalarTypes))

	if schemaPath := schemaPaths[output]; schemaPath != "" {
		schemaFile := filepath.Join(root, filepath.FromSlash(schemaPath))

		err = mergeGeneratedClosedEnums(metadata.closedEnums, importPath, schemaFile, file)
		if err != nil {
			return err
		}
	}

	mergeGeneratedModelFields(metadata.modelFields, importPath, generatedModelFieldTypes(file, importPath))
	mergeGeneratedModelTypes(metadata.modelTypes, importPath, generatedModelTypeNames(file))

	return nil
}

func mergeGeneratedWireConstantMetadata(root string, metadata *generatedScalarMetadata) error {
	path := filepath.Join(root, "pkg", "dependencymodels", "wire_constants.gen.go")

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse generated scalar metadata %s: %w", path, err)
	}

	mergeGeneratedConstantNames(metadata.constants, dependencymodelsImportPath, generatedConstantNames(file))
	mergeGeneratedConstantValues(metadata.constantValues, dependencymodelsImportPath, generatedConstantValues(file))

	return nil
}

func mergeGeneratedCompatibilityScalarMetadata(root string, metadata *generatedScalarMetadata) error {
	path := filepath.Join(root, "pkg", "generatedwire", "compat.gen.go")

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse generated scalar metadata %s: %w", path, err)
	}

	compatibilityPath := generatedModelImportPath("pkg/generatedwire/compat.gen.go")
	metadata.constants[compatibilityPath] = generatedConstantNames(file)
	metadata.fields[compatibilityPath] = metadata.fields[dependencymodelsImportPath]
	metadata.closedEnums[compatibilityPath] = metadata.closedEnums[dependencymodelsImportPath]
	metadata.modelFields[compatibilityPath] = metadata.modelFields[dependencymodelsImportPath]
	metadata.modelTypes[compatibilityPath] = metadata.modelTypes[dependencymodelsImportPath]
	metadata.constantValues[compatibilityPath] = make(map[string]string)

	for name := range metadata.constants[compatibilityPath] {
		if value, found := metadata.constantValues[dependencymodelsImportPath][name]; found {
			metadata.constantValues[compatibilityPath][name] = value
		}
	}

	return nil
}

func mergeGeneratedModelTypes(destination map[string]map[string]bool, packagePath string, modelTypes map[string]bool) {
	if destination[packagePath] == nil {
		destination[packagePath] = make(map[string]bool)
	}

	for name := range modelTypes {
		destination[packagePath][name] = true
	}
}

func generatedModelTypeNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)

	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}

		for _, rawSpec := range general.Specs {
			typeDefinition, ok := rawSpec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if _, ok := unparen(typeDefinition.Type).(*ast.StructType); ok {
				names[typeDefinition.Name.Name] = true
			}
		}
	}

	return names
}

func mergeGeneratedConstantValues(
	destination map[string]map[string]string,
	packagePath string,
	values map[string]string,
) {
	if destination[packagePath] == nil {
		destination[packagePath] = make(map[string]string)
	}

	maps.Copy(destination[packagePath], values)
}

func mergeGeneratedClosedEnums(
	destination map[string]map[string]map[string]map[string]bool,
	packagePath, schemaPath string,
	file *ast.File,
) error {
	closedEnums, err := generatedClosedScalarEnums(schemaPath, file)
	if err != nil {
		return err
	}

	if len(closedEnums) == 0 {
		return nil
	}

	if destination[packagePath] == nil {
		destination[packagePath] = make(map[string]map[string]map[string]bool)
	}

	for typeName, fields := range closedEnums {
		if destination[packagePath][typeName] == nil {
			destination[packagePath][typeName] = make(map[string]map[string]bool)
		}

		maps.Copy(destination[packagePath][typeName], fields)
	}

	return nil
}

func mergeGeneratedModelFields(
	destination map[string]map[string]map[string]generatedModelFieldType,
	packagePath string,
	fields map[string]map[string]generatedModelFieldType,
) {
	if destination[packagePath] == nil {
		destination[packagePath] = make(map[string]map[string]generatedModelFieldType)
	}

	for typeName, modelFields := range fields {
		if destination[packagePath][typeName] == nil {
			destination[packagePath][typeName] = make(map[string]generatedModelFieldType)
		}

		maps.Copy(destination[packagePath][typeName], modelFields)
	}
}

func mergeGeneratedConstantNames(destination map[string]map[string]bool, packagePath string, names map[string]bool) {
	if destination[packagePath] == nil {
		destination[packagePath] = make(map[string]bool)
	}

	for name := range names {
		destination[packagePath][name] = true
	}
}

func mergeGeneratedScalarFields(
	destination map[string]map[string]map[string]bool,
	packagePath string,
	fields map[string]map[string]bool,
) {
	if destination[packagePath] == nil {
		destination[packagePath] = make(map[string]map[string]bool)
	}

	for typeName, names := range fields {
		if destination[packagePath][typeName] == nil {
			destination[packagePath][typeName] = make(map[string]bool)
		}

		for name := range names {
			destination[packagePath][typeName][name] = true
		}
	}
}

func generatedModelImportPath(output string) string {
	return "github.com/portpowered/go-tplink/" + filepath.ToSlash(filepath.Dir(output))
}

func generatedConstantNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)

	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}

		for _, rawSpec := range general.Specs {
			spec, ok := rawSpec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, name := range spec.Names {
				names[name.Name] = true
			}
		}
	}

	return names
}

func generatedConstantValues(file *ast.File) map[string]string {
	values := make(map[string]string)

	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}

		for _, rawSpec := range general.Specs {
			spec, ok := rawSpec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for index, name := range spec.Names {
				if index >= len(spec.Values) {
					continue
				}

				if value, found := generatedConstantValue(spec.Values[index]); found {
					values[name.Name] = value
				}
			}
		}
	}

	return values
}

func generatedConstantValue(expression ast.Expr) (string, bool) {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.BasicLit:
		return generatedBasicLiteralConstant(typed)
	case *ast.UnaryExpr:
		return generatedUnaryConstant(typed)
	case *ast.Ident:
		if typed.Name == goTrueLiteral || typed.Name == goFalseLiteral {
			return typed.Name, true
		}
	}

	return "", false
}

func generatedBasicLiteralConstant(literal *ast.BasicLit) (string, bool) {
	//nolint:exhaustive // Only string and integer literals can represent generated scalar constants.
	switch literal.Kind {
	case token.STRING:
		value, err := strconv.Unquote(literal.Value)

		return value, err == nil
	case token.INT:
		value, err := strconv.ParseInt(literal.Value, 0, 64)
		if err != nil {
			return literal.Value, true
		}

		return strconv.FormatInt(value, 10), true
	default:
		return "", false
	}
}

func generatedUnaryConstant(expression *ast.UnaryExpr) (string, bool) {
	if expression.Op != token.ADD && expression.Op != token.SUB {
		return "", false
	}

	value, found := generatedConstantValue(expression.X)
	if !found {
		return "", false
	}

	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return "", false
	}

	if expression.Op == token.SUB {
		number = -number
	}

	return strconv.FormatInt(number, 10), true
}

func generatedScalarFieldNames(file *ast.File, scalarTypes map[string]bool) map[string]map[string]bool {
	fields := make(map[string]map[string]bool)

	for _, declaration := range file.Decls {
		general, isGeneralDeclaration := declaration.(*ast.GenDecl)
		if !isGeneralDeclaration || general.Tok != token.TYPE {
			continue
		}

		for _, rawSpec := range general.Specs {
			typeDefinition, isTypeDefinition := rawSpec.(*ast.TypeSpec)
			if !isTypeDefinition {
				continue
			}

			typeFields := generatedScalarStructFields(typeDefinition, file, scalarTypes)
			if len(typeFields) != 0 {
				fields[typeDefinition.Name.Name] = typeFields
			}
		}
	}

	return fields
}

func generatedScalarStructFields(
	typeDefinition *ast.TypeSpec,
	file *ast.File,
	scalarTypes map[string]bool,
) map[string]bool {
	structure, isStructure := unparen(typeDefinition.Type).(*ast.StructType)
	if !isStructure {
		return nil
	}

	fields := make(map[string]bool)

	for _, field := range structure.Fields.List {
		if !generatedScalarFieldType(field.Type, file, scalarTypes) {
			continue
		}

		for _, name := range field.Names {
			fields[name.Name] = true
		}
	}

	return fields
}

func generatedScalarFieldType(expression ast.Expr, file *ast.File, scalarTypes map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return generatedScalarFieldType(typed.X, file, scalarTypes)
	case *ast.ArrayType:
		return generatedScalarFieldType(typed.Elt, file, scalarTypes)
	case *ast.Ident:
		return scalarTypes[typed.Name] || isScalarConversion(typed.Name)
	case *ast.SelectorExpr:
		return importedPath(file, typed.X) == dependencymodelsImportPath && scalarTypes[typed.Sel.Name]
	default:
		return false
	}
}

func generatedClosedScalarEnums(schemaPath string, file *ast.File) (map[string]map[string]map[string]bool, error) {
	//nolint:gosec // Schema paths come from the checked-in generated model inventory.
	schemaSource, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("read generated scalar schema %s: %w", schemaPath, err)
	}

	var document schemaDocument

	err = yaml.Unmarshal(schemaSource, &document)
	if err != nil {
		return nil, fmt.Errorf("parse generated scalar schema %s: %w", schemaPath, err)
	}

	closedEnums := make(map[string]map[string]map[string]bool)

	for schemaName, schemaNode := range document.Components.Schemas {
		structure := generatedStructForSchema(file, schemaName)
		if structure == nil {
			continue
		}

		properties, decodeErr := generatedSchemaScalarProperties(schemaNode)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode scalar component %s in %s: %w", schemaName, schemaPath, decodeErr)
		}

		closed := generatedClosedScalarProperties(structure, properties)
		if len(closed) > 0 {
			closedEnums[schemaName] = closed
		}
	}

	return closedEnums, nil
}

func generatedSchemaScalarProperties(schemaNode yaml.Node) (map[string]struct {
	Enum []any `yaml:"enum"`
}, error) {
	var component struct {
		Properties map[string]struct {
			Enum []any `yaml:"enum"`
		} `yaml:"properties"`
	}

	err := schemaNode.Decode(&component)
	if err != nil {
		return nil, fmt.Errorf("decode schema component properties: %w", err)
	}

	return component.Properties, nil
}

func generatedClosedScalarProperties(
	structure *ast.StructType,
	properties map[string]struct {
		Enum []any `yaml:"enum"`
	},
) map[string]map[string]bool {
	closed := make(map[string]map[string]bool)

	for propertyName, property := range properties {
		if len(property.Enum) == 0 {
			continue
		}

		fieldName := generatedJSONStructFieldName(structure, propertyName)
		if fieldName == "" {
			continue
		}

		values := make(map[string]bool, len(property.Enum))
		for _, value := range property.Enum {
			values[fmt.Sprint(value)] = true
		}

		if len(values) > 0 {
			closed[fieldName] = values
		}
	}

	return closed
}

func generatedStructForSchema(file *ast.File, schemaName string) *ast.StructType {
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}

		for _, rawSpec := range general.Specs {
			typeDefinition, ok := rawSpec.(*ast.TypeSpec)
			if !ok || typeDefinition.Name.Name != schemaName {
				continue
			}

			structure, _ := unparen(typeDefinition.Type).(*ast.StructType)

			return structure
		}
	}

	return nil
}

func generatedJSONStructFieldName(structure *ast.StructType, propertyName string) string {
	if structure == nil || structure.Fields == nil {
		return ""
	}

	for _, field := range structure.Fields.List {
		if field.Tag == nil {
			continue
		}

		tag, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			continue
		}

		jsonName, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
		if jsonName != propertyName {
			continue
		}

		if len(field.Names) == 0 {
			continue
		}

		return field.Names[0].Name
	}

	return ""
}

func generatedModelFieldTypes(file *ast.File, packagePath string) map[string]map[string]generatedModelFieldType {
	fields := make(map[string]map[string]generatedModelFieldType)

	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}

		for _, rawSpec := range general.Specs {
			typeDefinition, isType := rawSpec.(*ast.TypeSpec)
			if !isType {
				continue
			}

			modelFields := generatedModelStructFieldTypes(file, packagePath, typeDefinition)
			if len(modelFields) > 0 {
				fields[typeDefinition.Name.Name] = modelFields
			}
		}
	}

	return fields
}

func generatedModelStructFieldTypes(
	file *ast.File,
	packagePath string,
	typeDefinition *ast.TypeSpec,
) map[string]generatedModelFieldType {
	fields := make(map[string]generatedModelFieldType)

	structure, isStruct := unparen(typeDefinition.Type).(*ast.StructType)
	if !isStruct || structure.Fields == nil {
		return fields
	}

	for _, field := range structure.Fields.List {
		target, found := generatedFieldModelType(field.Type, file, packagePath)
		if !found {
			continue
		}

		for _, name := range field.Names {
			fields[name.Name] = target
		}
	}

	return fields
}

func generatedFieldModelType(expression ast.Expr, file *ast.File, packagePath string) (generatedModelFieldType, bool) {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return generatedFieldModelType(typed.X, file, packagePath)
	case *ast.ArrayType:
		return generatedFieldModelType(typed.Elt, file, packagePath)
	case *ast.Ident:
		if isScalarConversion(typed.Name) {
			return generatedModelFieldType{packagePath: "", typeName: ""}, false
		}

		return generatedModelFieldType{packagePath: packagePath, typeName: typed.Name}, true
	case *ast.SelectorExpr:
		resolvedPath := importedPath(file, typed.X)
		if resolvedPath == "" {
			return generatedModelFieldType{packagePath: "", typeName: ""}, false
		}

		return generatedModelFieldType{packagePath: resolvedPath, typeName: typed.Sel.Name}, true
	default:
		return generatedModelFieldType{packagePath: "", typeName: ""}, false
	}
}

type scalarPackageIndex struct {
	functions        map[string][]scalarFunction
	methods          map[string][]scalarFunction
	globals          map[string][]scalarGlobal
	generatedScalars map[string]bool
	parameterSafety  map[string][]bool
	metadata         generatedScalarMetadata
}

type scalarFunction struct {
	source         packageProvenanceFile
	declaration    *ast.FuncDecl
	literal        *ast.FuncLit
	receiver       ast.Expr
	captures       map[token.Pos]bool
	typeOverride   *ast.FuncType
	callerSupplied bool
}

type scalarGlobal struct {
	source   packageProvenanceFile
	spec     *ast.ValueSpec
	index    int
	constant bool
}

type scalarEvaluation struct {
	index             *scalarPackageIndex
	source            packageProvenanceFile
	values            map[token.Pos]bool
	functionValues    map[token.Pos][]scalarFunction
	writes            map[token.Pos][]ast.Expr
	resolving         map[token.Pos]bool
	callStack         map[string]bool
	allowedEnumValues map[string]bool
}

func newDeclaredScalarFunction(source packageProvenanceFile, declaration *ast.FuncDecl) scalarFunction {
	return scalarFunction{
		source: source, declaration: declaration, literal: nil, receiver: nil, captures: nil,
		typeOverride: nil, callerSupplied: false,
	}
}

func newLiteralScalarFunction(
	source packageProvenanceFile,
	literal *ast.FuncLit,
	captures map[token.Pos]bool,
) scalarFunction {
	return scalarFunction{
		source: source, declaration: nil, literal: literal, receiver: nil, captures: captures,
		typeOverride: nil, callerSupplied: false,
	}
}

func newCallerScalarFunction(source packageProvenanceFile, functionType *ast.FuncType) scalarFunction {
	return scalarFunction{
		source: source, declaration: nil, literal: nil, receiver: nil, captures: nil,
		typeOverride: functionType, callerSupplied: true,
	}
}

func emptyScalarFunction() scalarFunction {
	return scalarFunction{
		source: packageProvenanceFile{path: "", fileSet: nil, file: nil}, declaration: nil,
		literal: nil, receiver: nil, captures: nil, typeOverride: nil, callerSupplied: false,
	}
}

func newScalarPackageIndex(
	files []packageProvenanceFile,
	generatedScalars map[string]bool,
	metadata generatedScalarMetadata,
) *scalarPackageIndex {
	index := &scalarPackageIndex{
		functions:        make(map[string][]scalarFunction),
		methods:          make(map[string][]scalarFunction),
		globals:          make(map[string][]scalarGlobal),
		generatedScalars: generatedScalars,
		parameterSafety:  make(map[string][]bool),
		metadata:         metadata,
	}

	for _, source := range files {
		for _, declaration := range source.file.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				if typed.Body == nil {
					continue
				}

				function := newDeclaredScalarFunction(source, typed)
				if typed.Recv == nil {
					index.functions[typed.Name.Name] = append(index.functions[typed.Name.Name], function)
				} else {
					index.methods[typed.Name.Name] = append(index.methods[typed.Name.Name], function)
				}
			case *ast.GenDecl:
				collectScalarGlobals(source, typed, index.globals)
			}
		}
	}

	initializeScalarParameterSafety(index)

	return index
}

func initializeScalarParameterSafety(index *scalarPackageIndex) {
	functions := append(flattenScalarFunctions(index.functions), flattenScalarFunctions(index.methods)...)
	for _, function := range functions {
		index.parameterSafety[scalarFunctionKey(function)] = scalarInitialParameterSafety(function)
	}

	propagateScalarParameterSafety(index, scalarPackageCallSites(index))
}

func scalarInitialParameterSafety(function scalarFunction) []bool {
	values := make([]bool, scalarParameterCount(function.functionType()))
	if function.declaration != nil && ast.IsExported(function.declaration.Name.Name) {
		for parameter := range values {
			values[parameter] = true
		}
	}

	return values
}

func propagateScalarParameterSafety(index *scalarPackageIndex, calls []scalarCallSite) {
	for {
		if !propagateOneScalarParameterSafetyPass(index, calls) {
			return
		}
	}
}

func propagateOneScalarParameterSafetyPass(index *scalarPackageIndex, calls []scalarCallSite) bool {
	changed := false

	for key, current := range index.parameterSafety {
		function, found := scalarFunctionByKey(index, key)
		if !found || function.declaration == nil || ast.IsExported(function.declaration.Name.Name) {
			continue
		}

		for parameter, safe := range current {
			if safe {
				continue
			}

			if scalarParameterProvenAtEveryCall(index, function, parameter, calls) {
				current[parameter] = true
				changed = true
			}
		}
	}

	return changed
}

func flattenScalarFunctions(functions map[string][]scalarFunction) []scalarFunction {
	var flattened []scalarFunction
	for _, candidates := range functions {
		flattened = append(flattened, candidates...)
	}

	return flattened
}

func scalarParameterCount(functionType *ast.FuncType) int {
	if functionType == nil || functionType.Params == nil {
		return 0
	}

	count := 0

	for _, field := range functionType.Params.List {
		if len(field.Names) == 0 {
			count++

			continue
		}

		count += len(field.Names)
	}

	return count
}

type scalarCallSite struct {
	caller   scalarFunction
	target   scalarFunction
	args     []ast.Expr
	receiver ast.Expr
}

func scalarPackageCallSites(index *scalarPackageIndex) []scalarCallSite {
	var calls []scalarCallSite

	for _, caller := range append(flattenScalarFunctions(index.functions), flattenScalarFunctions(index.methods)...) {
		if caller.declaration == nil || caller.declaration.Body == nil {
			continue
		}

		callerValues := index.parameterSafety[scalarFunctionKey(caller)]
		argumentValues := make([]bool, scalarParameterCount(caller.functionType()))
		copy(argumentValues, callerValues)
		evaluation := newScalarEvaluation(index, caller.source, caller.declaration, argumentValues, nil,
			ast.IsExported(caller.declaration.Name.Name))

		ast.Inspect(caller.declaration.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			for _, target := range scalarCallTargets(call.Fun, evaluation, make(map[token.Pos]bool)) {
				calls = append(calls, scalarCallSite{
					caller: caller, target: target, args: call.Args, receiver: target.receiver,
				})
			}

			return true
		})
	}

	return calls
}

func scalarParameterProvenAtEveryCall(
	index *scalarPackageIndex,
	function scalarFunction,
	parameter int,
	calls []scalarCallSite,
) bool {
	foundExternalSource := false

	for _, call := range calls {
		if scalarFunctionKey(call.target) != scalarFunctionKey(function) {
			continue
		}

		if call.target.declaration != nil && scalarFunctionKey(call.caller) == scalarFunctionKey(function) &&
			scalarCallPassesSameParameter(call, function, parameter) {
			continue
		}

		if parameter >= len(call.args) {
			return false
		}

		callerValues := index.parameterSafety[scalarFunctionKey(call.caller)]
		argumentValues := make([]bool, scalarParameterCount(call.caller.functionType()))
		copy(argumentValues, callerValues)

		callerEvaluation := newScalarEvaluation(index, call.caller.source, call.caller.declaration,
			argumentValues, nil, ast.IsExported(call.caller.declaration.Name.Name))
		if !scalarExpressionSafe(call.args[parameter], callerEvaluation) {
			return false
		}

		foundExternalSource = true
	}

	return foundExternalSource
}

func scalarCallPassesSameParameter(call scalarCallSite, function scalarFunction, parameter int) bool {
	if function.declaration == nil || function.declaration.Type.Params == nil || parameter >= len(call.args) {
		return false
	}

	name := scalarFunctionParameterName(function.functionType(), parameter)
	if name == "" {
		return false
	}

	identifier, ok := unparen(call.args[parameter]).(*ast.Ident)

	return ok && identifier.Name == name && identifier.Obj != nil &&
		identifier.Obj.Pos() == functionParameterPosition(function.functionType(), parameter)
}

func scalarFunctionParameterName(functionType *ast.FuncType, index int) string {
	parameterIndex := 0

	if functionType == nil || functionType.Params == nil {
		return ""
	}

	for _, field := range functionType.Params.List {
		if len(field.Names) == 0 {
			if parameterIndex == index {
				return ""
			}

			parameterIndex++

			continue
		}

		for _, name := range field.Names {
			if parameterIndex == index {
				return name.Name
			}

			parameterIndex++
		}
	}

	return ""
}

func functionParameterPosition(functionType *ast.FuncType, index int) token.Pos {
	parameterIndex := 0

	if functionType == nil || functionType.Params == nil {
		return token.NoPos
	}

	for _, field := range functionType.Params.List {
		if len(field.Names) == 0 {
			parameterIndex++

			continue
		}

		for _, name := range field.Names {
			if parameterIndex == index {
				return identifierObjectPosition(name)
			}

			parameterIndex++
		}
	}

	return token.NoPos
}

func scalarFunctionByKey(index *scalarPackageIndex, key string) (scalarFunction, bool) {
	for _, function := range append(flattenScalarFunctions(index.functions), flattenScalarFunctions(index.methods)...) {
		if scalarFunctionKey(function) == key {
			return function, true
		}
	}

	return emptyScalarFunction(), false
}

func collectScalarGlobals(source packageProvenanceFile, declaration *ast.GenDecl, globals map[string][]scalarGlobal) {
	if declaration.Tok != token.VAR && declaration.Tok != token.CONST {
		return
	}

	for _, rawSpec := range declaration.Specs {
		spec, ok := rawSpec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for index, name := range spec.Names {
			globals[name.Name] = append(globals[name.Name], scalarGlobal{
				source:   source,
				spec:     spec,
				index:    index,
				constant: declaration.Tok == token.CONST,
			})
		}
	}
}

func checkPackageScalarProvenance(
	files []packageProvenanceFile,
	generatedTypes, generatedScalars map[string]bool,
	metadata generatedScalarMetadata,
) error {
	index := newScalarPackageIndex(files, generatedScalars, metadata)
	for _, source := range files {
		err := checkScalarSinksInFile(source, index, generatedTypes, metadata)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkScalarSinksInFile(
	source packageProvenanceFile,
	index *scalarPackageIndex,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
) error {
	for _, declaration := range source.file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		functionRef := newDeclaredScalarFunction(source, function)
		argumentValues := index.parameterSafety[scalarFunctionKey(functionRef)]
		evaluation := newScalarEvaluation(index, source, function, argumentValues, nil,
			ast.IsExported(function.Name.Name))

		var violation error

		ast.Inspect(function.Body, func(node ast.Node) bool {
			if node == nil || violation != nil {
				return false
			}

			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}

			violation = scalarSinkViolation(source, node, evaluation, generatedTypes, metadata)

			return violation == nil
		})

		if violation != nil {
			return violation
		}
	}

	return nil
}

func scalarSinkViolation(
	source packageProvenanceFile,
	node ast.Node,
	evaluation *scalarEvaluation,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
) error {
	switch typed := node.(type) {
	case *ast.CompositeLit:
		return scalarCompositeSinkViolation(source, typed, evaluation, generatedTypes, metadata)
	case *ast.AssignStmt:
		return scalarAssignmentSinkViolation(source, typed, evaluation, generatedTypes, metadata)
	default:
		return nil
	}
}

func scalarCompositeSinkViolation(
	source packageProvenanceFile,
	composite *ast.CompositeLit,
	evaluation *scalarEvaluation,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
) error {
	if !isProviderWireModelType(source.file, composite.Type, generatedTypes) {
		return nil
	}

	packagePath, typeName := generatedModelTypeReference(source.file, composite.Type, metadata.fields)
	scalarFields := metadata.fields[packagePath][typeName]

	for _, element := range composite.Elts {
		fieldName, ok := compositeFieldName(element)
		if !ok || !scalarFields[fieldName] {
			continue
		}

		value := compositeValue(element)

		valueEvaluation := scalarEvaluationForClosedEnum(
			evaluation, metadata.closedEnums[packagePath][typeName][fieldName],
		)
		if !scalarExpressionSafe(value, valueEvaluation) {
			return sourceError(source.fileSet, source.path, value.Pos(), generatedScalarMessage)
		}
	}

	return nil
}

func generatedModelTypeReference(
	file *ast.File,
	expression ast.Expr,
	fieldsByPackage map[string]map[string]map[string]bool,
) (string, string) {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		for packagePath, types := range fieldsByPackage {
			if types[typed.Name] != nil {
				return packagePath, typed.Name
			}
		}

		return "", typed.Name
	case *ast.SelectorExpr:
		return importedPath(file, typed.X), typed.Sel.Name
	default:
		return "", ""
	}
}

func compositeFieldName(element ast.Expr) (string, bool) {
	keyValue, isKeyValue := element.(*ast.KeyValueExpr)
	if !isKeyValue {
		return "", false
	}

	identifier, isIdentifier := unparen(keyValue.Key).(*ast.Ident)
	if !isIdentifier {
		return "", false
	}

	return identifier.Name, true
}

func scalarAssignmentSinkViolation(
	source packageProvenanceFile,
	assignment *ast.AssignStmt,
	evaluation *scalarEvaluation,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
) error {
	for index, target := range assignment.Lhs {
		if index >= len(assignment.Rhs) || !isGeneratedModelField(
			target, collectGeneratedVariablePositionSet(source.file, generatedTypes),
		) {
			continue
		}

		selector, ok := unparen(target).(*ast.SelectorExpr)
		if !ok || !generatedScalarFieldName(metadata.fields, selector.Sel.Name) {
			continue
		}

		closedValues, closed, resolved := generatedClosedEnumFieldValues(
			source.file, target, generatedTypes, metadata,
		)
		if !resolved && generatedClosedEnumFieldName(metadata.closedEnums, selector.Sel.Name) {
			return sourceError(source.fileSet, source.path, assignment.Rhs[index].Pos(), generatedScalarMessage)
		}

		if !closed {
			closedValues = nil
		}

		valueEvaluation := scalarEvaluationForClosedEnum(evaluation, closedValues)
		if !scalarExpressionSafe(assignment.Rhs[index], valueEvaluation) {
			return sourceError(source.fileSet, source.path, assignment.Rhs[index].Pos(), generatedScalarMessage)
		}
	}

	return nil
}

func collectGeneratedVariablePositionSet(file *ast.File, generatedTypes map[string]bool) map[token.Pos]bool {
	generated := make(map[token.Pos]bool)
	collectGeneratedVariableAliases(file, generatedTypes, generated)

	return generated
}

func generatedScalarFieldName(fieldsByPackage map[string]map[string]map[string]bool, name string) bool {
	for _, fields := range fieldsByPackage {
		for _, names := range fields {
			if names[name] {
				return true
			}
		}
	}

	return false
}

func scalarEvaluationForClosedEnum(evaluation *scalarEvaluation, allowedValues map[string]bool) *scalarEvaluation {
	if len(allowedValues) == 0 {
		return evaluation
	}

	closedEvaluation := *evaluation

	closedEvaluation.values = make(map[token.Pos]bool, len(evaluation.values))
	for position := range evaluation.values {
		closedEvaluation.values[position] = false
	}

	closedEvaluation.functionValues = make(map[token.Pos][]scalarFunction)
	closedEvaluation.resolving = make(map[token.Pos]bool)
	closedEvaluation.callStack = cloneScalarCallStack(evaluation.callStack)
	closedEvaluation.allowedEnumValues = allowedValues

	return &closedEvaluation
}

func generatedClosedEnumFieldName(
	closedEnums map[string]map[string]map[string]map[string]bool,
	fieldName string,
) bool {
	for _, types := range closedEnums {
		for _, fields := range types {
			if fields[fieldName] != nil {
				return true
			}
		}
	}

	return false
}

func generatedClosedEnumFieldValues(
	file *ast.File,
	target ast.Expr,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
) (map[string]bool, bool, bool) {
	var selectors []*ast.SelectorExpr

	current := unparen(target)

	for {
		selector, ok := current.(*ast.SelectorExpr)
		if !ok {
			break
		}

		selectors = append(selectors, selector)
		current = unparen(selector.X)
	}

	if len(selectors) == 0 {
		return nil, false, false
	}

	identifier, ok := current.(*ast.Ident)
	if !ok {
		return nil, false, false
	}

	modelType, found := generatedModelTypeForIdentifier(file, identifier, generatedTypes, metadata,
		make(map[token.Pos]bool))
	if !found {
		return nil, false, false
	}

	for index, selector := range slices.Backward(selectors) {
		if index == 0 {
			allowed, closed := metadata.closedEnums[modelType.packagePath][modelType.typeName][selector.Sel.Name]

			return allowed, closed, true
		}

		modelType, found = metadata.modelFields[modelType.packagePath][modelType.typeName][selector.Sel.Name]
		if !found {
			return nil, false, false
		}
	}

	return nil, false, false
}

func generatedModelTypeForIdentifier(
	file *ast.File,
	identifier *ast.Ident,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
	visiting map[token.Pos]bool,
) (generatedModelFieldType, bool) {
	position := identifierObjectPosition(identifier)
	if position == token.NoPos || visiting[position] {
		return generatedModelFieldType{packagePath: "", typeName: ""}, false
	}

	visiting[position] = true
	defer delete(visiting, position)

	if identifier.Obj == nil {
		return generatedModelTypeForExpression(file, identifier, generatedTypes, metadata, visiting)
	}

	return generatedModelTypeForDeclaration(file, identifier.Obj.Decl, position, generatedTypes, metadata, visiting)
}

func generatedModelTypeForDeclaration(
	file *ast.File,
	declaration any,
	position token.Pos,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
	visiting map[token.Pos]bool,
) (generatedModelFieldType, bool) {
	switch typed := declaration.(type) {
	case *ast.Field:
		return generatedModelTypeForExpression(file, typed.Type, generatedTypes, metadata, visiting)
	case *ast.ValueSpec:
		return generatedModelTypeForValueSpec(file, typed, position, generatedTypes, metadata, visiting)
	case *ast.AssignStmt:
		return generatedModelTypeForAssignment(file, typed, position, generatedTypes, metadata, visiting)
	}

	return generatedModelFieldType{packagePath: "", typeName: ""}, false
}

func generatedModelTypeForValueSpec(
	file *ast.File,
	value *ast.ValueSpec,
	position token.Pos,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
	visiting map[token.Pos]bool,
) (generatedModelFieldType, bool) {
	for index, name := range value.Names {
		if identifierObjectPosition(name) != position {
			continue
		}

		if value.Type != nil {
			return generatedModelTypeForExpression(file, value.Type, generatedTypes, metadata, visiting)
		}

		if index < len(value.Values) {
			return generatedModelTypeForExpression(file, value.Values[index], generatedTypes, metadata, visiting)
		}
	}

	return generatedModelFieldType{packagePath: "", typeName: ""}, false
}

func generatedModelTypeForAssignment(
	file *ast.File,
	assignment *ast.AssignStmt,
	position token.Pos,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
	visiting map[token.Pos]bool,
) (generatedModelFieldType, bool) {
	for index, target := range assignment.Lhs {
		name, isIdentifier := unparen(target).(*ast.Ident)
		if !isIdentifier || identifierObjectPosition(name) != position || index >= len(assignment.Rhs) {
			continue
		}

		return generatedModelTypeForExpression(file, assignment.Rhs[index], generatedTypes, metadata, visiting)
	}

	return generatedModelFieldType{packagePath: "", typeName: ""}, false
}

func generatedModelTypeForExpression(
	file *ast.File,
	expression ast.Expr,
	generatedTypes map[string]bool,
	metadata generatedScalarMetadata,
	visiting map[token.Pos]bool,
) (generatedModelFieldType, bool) {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.StarExpr, *ast.ArrayType:
		return generatedModelTypeForExpression(file, generatedModelTypeElement(typed), generatedTypes, metadata, visiting)
	case *ast.CompositeLit:
		packagePath, typeName := generatedModelTypeReference(file, typed.Type, metadata.fields)

		return generatedModelFieldType{packagePath: packagePath, typeName: typeName},
			generatedModelMetadataType(metadata, packagePath, typeName)
	case *ast.Ident:
		if typed.Obj != nil && (typed.Obj.Kind == ast.Var || typed.Obj.Kind == ast.Con) {
			return generatedModelTypeForIdentifier(file, typed, generatedTypes, metadata, visiting)
		}

		packagePath, typeName := generatedModelTypeReference(file, typed, metadata.fields)

		return generatedModelFieldType{packagePath: packagePath, typeName: typeName},
			generatedModelMetadataType(metadata, packagePath, typeName)
	case *ast.SelectorExpr:
		packagePath := importedPath(file, typed.X)
		if packagePath == "" {
			return generatedModelFieldType{packagePath: "", typeName: ""}, false
		}

		return generatedModelFieldType{packagePath: packagePath, typeName: typed.Sel.Name},
			generatedModelMetadataType(metadata, packagePath, typed.Sel.Name)
	default:
		return generatedModelFieldType{packagePath: "", typeName: ""}, false
	}
}

func generatedModelTypeElement(expression ast.Expr) ast.Expr {
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return typed.X
	case *ast.ArrayType:
		return typed.Elt
	default:
		return expression
	}
}

func generatedModelMetadataType(metadata generatedScalarMetadata, packagePath, typeName string) bool {
	return metadata.fields[packagePath][typeName] != nil || metadata.modelFields[packagePath][typeName] != nil ||
		metadata.modelTypes[packagePath][typeName]
}

func newScalarEvaluation(
	index *scalarPackageIndex,
	source packageProvenanceFile,
	function *ast.FuncDecl,
	argumentValues []bool,
	callStack map[string]bool,
	callerInputs bool,
) *scalarEvaluation {
	if function == nil {
		return newScalarBodyEvaluation(index, source, nil, nil, argumentValues, callStack, callerInputs)
	}

	return newScalarBodyEvaluation(index, source, function.Type, function.Body,
		argumentValues, callStack, callerInputs)
}

func newScalarBodyEvaluation(
	index *scalarPackageIndex,
	source packageProvenanceFile,
	functionType *ast.FuncType,
	body *ast.BlockStmt,
	argumentValues []bool,
	callStack map[string]bool,
	callerInputs bool,
) *scalarEvaluation {
	evaluation := &scalarEvaluation{
		index:             index,
		source:            source,
		values:            make(map[token.Pos]bool),
		functionValues:    make(map[token.Pos][]scalarFunction),
		writes:            make(map[token.Pos][]ast.Expr),
		resolving:         make(map[token.Pos]bool),
		callStack:         cloneScalarCallStack(callStack),
		allowedEnumValues: nil,
	}
	if functionType == nil {
		return evaluation
	}

	scalarBindParameters(evaluation, functionType.Params, argumentValues, callerInputs, source)
	collectScalarFunctionWrites(body, evaluation.writes)

	return evaluation
}

func scalarBindParameters(
	evaluation *scalarEvaluation,
	parameters *ast.FieldList,
	argumentValues []bool,
	callerInputs bool,
	source packageProvenanceFile,
) {
	if parameters == nil {
		return
	}

	parameterIndex := 0

	for _, field := range parameters.List {
		for _, name := range field.Names {
			scalarBindParameter(evaluation, name, field.Type, parameterIndex, argumentValues, callerInputs, source)

			parameterIndex++
		}
	}
}

func scalarBindParameter(
	evaluation *scalarEvaluation,
	name *ast.Ident,
	fieldType ast.Expr,
	parameterIndex int,
	argumentValues []bool,
	callerInputs bool,
	source packageProvenanceFile,
) {
	position := identifierObjectPosition(name)

	if parameterIndex < len(argumentValues) {
		evaluation.values[position] = argumentValues[parameterIndex]
	} else if callerInputs {
		evaluation.values[position] = true
	}

	if !callerInputs {
		return
	}

	callbackType, isCallback := unparen(fieldType).(*ast.FuncType)
	if isCallback {
		evaluation.functionValues[position] = []scalarFunction{newCallerScalarFunction(source, callbackType)}
	}
}

func cloneScalarCallStack(stack map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(stack))
	for key := range stack {
		cloned[key] = true
	}

	return cloned
}

func collectScalarFunctionWrites(body *ast.BlockStmt, writes map[token.Pos][]ast.Expr) {
	if body == nil {
		return
	}

	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			return false
		}

		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}

		switch typed := node.(type) {
		case *ast.ValueSpec:
			collectScalarValueWrites(typed, writes)
		case *ast.AssignStmt:
			collectScalarAssignmentWrites(typed, writes)
		}

		return true
	})
}

func collectScalarValueWrites(value *ast.ValueSpec, writes map[token.Pos][]ast.Expr) {
	for index, name := range value.Names {
		if name.Obj == nil || index >= len(value.Values) {
			continue
		}

		position := identifierObjectPosition(name)
		writes[position] = append(writes[position], value.Values[index])
	}
}

func collectScalarAssignmentWrites(assignment *ast.AssignStmt, writes map[token.Pos][]ast.Expr) {
	for index, target := range assignment.Lhs {
		identifier, isIdentifier := unparen(target).(*ast.Ident)
		if !isIdentifier || identifier.Obj == nil || index >= len(assignment.Rhs) {
			continue
		}

		position := identifierObjectPosition(identifier)
		writes[position] = append(writes[position], assignment.Rhs[index])
	}
}

func scalarExpressionSafe(expression ast.Expr, evaluation *scalarEvaluation) bool {
	if expression == nil {
		return false
	}

	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return scalarIdentifierExpressionSafe(typed, evaluation)
	case *ast.CompositeLit:
		return evaluation.allowedEnumValues == nil && scalarGeneratedModelValue(typed, evaluation)
	case *ast.SelectorExpr:
		return scalarSelectorExpressionSafe(typed, evaluation)
	case *ast.CallExpr:
		return scalarCallSafe(typed, evaluation)
	case *ast.StarExpr, *ast.UnaryExpr, *ast.IndexExpr, *ast.TypeAssertExpr:
		return scalarWrappedExpressionSafe(typed, evaluation)
	default:
		return false
	}
}

func scalarIdentifierExpressionSafe(identifier *ast.Ident, evaluation *scalarEvaluation) bool {
	return (evaluation.allowedEnumValues == nil && scalarGeneratedModelValue(identifier, evaluation)) ||
		scalarIdentifierSafe(identifier, evaluation)
}

func scalarSelectorExpressionSafe(selector *ast.SelectorExpr, evaluation *scalarEvaluation) bool {
	return scalarGeneratedConstant(selector, evaluation) || scalarExpressionSafe(selector.X, evaluation)
}

func scalarWrappedExpressionSafe(expression ast.Expr, evaluation *scalarEvaluation) bool {
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return scalarExpressionSafe(typed.X, evaluation)
	case *ast.UnaryExpr:
		return typed.Op == token.AND && scalarExpressionSafe(typed.X, evaluation)
	case *ast.IndexExpr:
		return scalarExpressionSafe(typed.X, evaluation)
	case *ast.TypeAssertExpr:
		return scalarExpressionSafe(typed.X, evaluation)
	default:
		return false
	}
}

func scalarGeneratedModelValue(expression ast.Expr, evaluation *scalarEvaluation) bool {
	var (
		modelType generatedModelFieldType
		found     bool
	)

	switch typed := unparen(expression).(type) {
	case *ast.Ident:
		modelType, found = generatedModelTypeForIdentifier(evaluation.source.file, typed, nil,
			evaluation.index.metadata, make(map[token.Pos]bool))
	case *ast.CompositeLit:
		modelType, found = generatedModelTypeForExpression(evaluation.source.file, typed.Type, nil,
			evaluation.index.metadata, make(map[token.Pos]bool))
	}

	return found && generatedModelMetadataType(evaluation.index.metadata, modelType.packagePath, modelType.typeName)
}

func scalarIdentifierSafe(identifier *ast.Ident, evaluation *scalarEvaluation) bool {
	position := identifierObjectPosition(identifier)
	if safe, found := evaluation.values[position]; found {
		return safe
	}

	if len(evaluation.writes[position]) != 0 {
		return scalarAssignedValuesSafe(position, evaluation)
	}

	return scalarIdentifierObjectSafe(identifier, evaluation)
}

func scalarAssignedValuesSafe(position token.Pos, evaluation *scalarEvaluation) bool {
	if evaluation.resolving[position] {
		return false
	}

	evaluation.resolving[position] = true
	defer delete(evaluation.resolving, position)

	for _, value := range evaluation.writes[position] {
		if !scalarExpressionSafe(value, evaluation) {
			return false
		}
	}

	return true
}

func scalarIdentifierObjectSafe(identifier *ast.Ident, evaluation *scalarEvaluation) bool {
	if identifier.Obj != nil {
		if spec, isValueSpec := identifier.Obj.Decl.(*ast.ValueSpec); isValueSpec {
			isLocalValue := evaluation.source.file.Scope.Lookup(identifier.Name) != identifier.Obj
			if isLocalValue && spec.Type == nil && len(spec.Values) == 0 {
				return false
			}
		}
	}

	return scalarGlobalSafe(identifier.Name, evaluation)
}

func scalarGlobalSafe(name string, evaluation *scalarEvaluation) bool {
	globals := evaluation.index.globals[name]
	if len(globals) != 1 || !globals[0].constant ||
		evaluation.resolving[globals[0].spec.Names[globals[0].index].Pos()] {
		return false
	}

	global := globals[0]
	if global.index >= len(global.spec.Values) {
		return false
	}

	position := global.spec.Names[global.index].Pos()

	evaluation.resolving[position] = true
	defer delete(evaluation.resolving, position)

	return scalarExpressionSafe(global.spec.Values[global.index], &scalarEvaluation{
		index:             evaluation.index,
		source:            global.source,
		values:            make(map[token.Pos]bool),
		functionValues:    make(map[token.Pos][]scalarFunction),
		writes:            make(map[token.Pos][]ast.Expr),
		resolving:         evaluation.resolving,
		callStack:         evaluation.callStack,
		allowedEnumValues: evaluation.allowedEnumValues,
	})
}

func scalarGeneratedConstant(selector *ast.SelectorExpr, evaluation *scalarEvaluation) bool {
	packagePath := importedPath(evaluation.source.file, selector.X)
	if packagePath == "" || !evaluation.index.metadata.constants[packagePath][selector.Sel.Name] {
		return false
	}

	if evaluation.allowedEnumValues == nil {
		return true
	}

	value, found := evaluation.index.metadata.constantValues[packagePath][selector.Sel.Name]

	return found && evaluation.allowedEnumValues[value]
}

func scalarCallSafe(call *ast.CallExpr, evaluation *scalarEvaluation) bool {
	if scalarConversionCall(call, evaluation) {
		return len(call.Args) == 1 && scalarExpressionSafe(call.Args[0], evaluation)
	}

	if scalarJSONMarshalCall(call, evaluation) {
		return evaluation.allowedEnumValues == nil && len(call.Args) == 1 &&
			scalarExpressionSafe(call.Args[0], evaluation)
	}

	targets := scalarCallTargets(call.Fun, evaluation, make(map[token.Pos]bool))
	if len(targets) == 0 {
		return false
	}

	for _, target := range targets {
		if !scalarFunctionResultSafe(target, call.Args, evaluation) {
			return false
		}
	}

	return true
}

func scalarJSONMarshalCall(call *ast.CallExpr, evaluation *scalarEvaluation) bool {
	selector, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Marshal" {
		return false
	}

	return importedPath(evaluation.source.file, selector.X) == "encoding/json"
}

func scalarConversionCall(call *ast.CallExpr, evaluation *scalarEvaluation) bool {
	if len(call.Args) != 1 {
		return false
	}

	function := unparen(call.Fun)
	if identifier, ok := function.(*ast.Ident); ok {
		return (isScalarConversion(identifier.Name) && identifier.Obj == nil) ||
			isGeneratedWireScalarType(evaluation.source.file, identifier, evaluation.index.generatedScalars)
	}

	selector, ok := function.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	packagePath := importedPath(evaluation.source.file, selector.X)

	return packagePath != "" && evaluation.index.generatedScalars[selector.Sel.Name]
}

func scalarCallTargets(
	expression ast.Expr,
	evaluation *scalarEvaluation,
	visiting map[token.Pos]bool,
) []scalarFunction {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.FuncLit:
		return []scalarFunction{newLiteralScalarFunction(
			evaluation.source, typed, scalarClosureValues(typed, evaluation))}
	case *ast.Ident:
		return scalarIdentifierFunctionTargets(typed, evaluation, visiting)
	case *ast.SelectorExpr:
		return scalarSelectorFunctionTargets(typed, evaluation)
	case *ast.CallExpr:
		return scalarReturnedCallbackTargets(typed, evaluation, visiting)
	default:
		return nil
	}
}

func scalarIdentifierFunctionTargets(
	identifier *ast.Ident,
	evaluation *scalarEvaluation,
	visiting map[token.Pos]bool,
) []scalarFunction {
	if targets := evaluation.functionValues[identifierObjectPosition(identifier)]; len(targets) != 0 {
		return targets
	}

	if identifier.Obj != nil && (identifier.Obj.Kind == ast.Var || identifier.Obj.Kind == ast.Con) {
		return scalarVariableFunctionTargets(identifier, evaluation, visiting)
	}

	if identifier.Obj != nil {
		function, isFunction := identifier.Obj.Decl.(*ast.FuncDecl)
		if isFunction {
			resolved, found := evaluation.index.functionForDeclaration(function)
			if found {
				return []scalarFunction{resolved}
			}

			return nil
		}
	}

	return evaluation.index.functions[identifier.Name]
}

func scalarSelectorFunctionTargets(selector *ast.SelectorExpr, evaluation *scalarEvaluation) []scalarFunction {
	if importedPath(evaluation.source.file, selector.X) != "" {
		return nil
	}

	methods := evaluation.index.methods[selector.Sel.Name]
	for index := range methods {
		methods[index].receiver = selector.X
	}

	return methods
}

func (index *scalarPackageIndex) functionForDeclaration(function *ast.FuncDecl) (scalarFunction, bool) {
	for _, candidate := range append(flattenScalarFunctions(index.functions), flattenScalarFunctions(index.methods)...) {
		if candidate.declaration == function {
			return candidate, true
		}
	}

	return emptyScalarFunction(), false
}

func scalarVariableFunctionTargets(
	identifier *ast.Ident,
	evaluation *scalarEvaluation,
	visiting map[token.Pos]bool,
) []scalarFunction {
	position := identifierObjectPosition(identifier)
	if visiting[position] {
		return nil
	}

	visiting[position] = true
	defer delete(visiting, position)

	var targets []scalarFunction
	for _, value := range evaluation.writes[position] {
		targets = append(targets, scalarCallTargets(value, evaluation, visiting)...)
	}

	return targets
}

func scalarFunctionResultSafe(
	target scalarFunction,
	arguments []ast.Expr,
	caller *scalarEvaluation,
) bool {
	if target.callerSupplied && caller.allowedEnumValues == nil {
		return true
	}

	declaration := target.declaration
	if declaration == nil && target.literal == nil {
		return false
	}

	name := scalarFunctionKey(target)
	if caller.callStack[name] {
		return false
	}

	callStack := cloneScalarCallStack(caller.callStack)
	callStack[name] = true

	functionType := target.functionType()

	body := target.body()

	if functionType == nil || body == nil {
		return false
	}

	argumentValues := make([]bool, 0, len(arguments))
	for _, argument := range arguments {
		argumentValues = append(argumentValues, scalarExpressionSafe(argument, caller))
	}

	evaluation := scalarFunctionEvaluation(target, arguments, argumentValues, caller, callStack)
	evaluation.allowedEnumValues = caller.allowedEnumValues

	return scalarFunctionBodySafe(functionType, body, evaluation)
}

func scalarFunctionEvaluation(
	target scalarFunction,
	arguments []ast.Expr,
	argumentValues []bool,
	caller *scalarEvaluation,
	callStack map[string]bool,
) *scalarEvaluation {
	evaluation := newScalarBodyEvaluation(caller.index, target.source, target.functionType(), target.body(),
		argumentValues, callStack, false)
	maps.Copy(evaluation.values, target.captures)
	scalarBindFunctionArguments(evaluation, target.functionType(), arguments, caller)
	scalarBindMethodReceiver(evaluation, target, caller)

	return evaluation
}

func scalarBindFunctionArguments(
	evaluation *scalarEvaluation,
	functionType *ast.FuncType,
	arguments []ast.Expr,
	caller *scalarEvaluation,
) {
	for index, position := range scalarFunctionParameterPositions(functionType) {
		if index >= len(arguments) || position == token.NoPos {
			continue
		}

		targets := scalarCallTargets(arguments[index], caller, make(map[token.Pos]bool))
		if len(targets) != 0 {
			evaluation.functionValues[position] = targets
		}
	}
}

func scalarBindMethodReceiver(evaluation *scalarEvaluation, target scalarFunction, caller *scalarEvaluation) {
	if target.receiver == nil || target.declaration == nil || target.declaration.Recv == nil {
		return
	}

	receiverSafe := scalarExpressionSafe(target.receiver, caller)

	for _, field := range target.declaration.Recv.List {
		for _, name := range field.Names {
			evaluation.values[identifierObjectPosition(name)] = receiverSafe
		}
	}
}

func scalarFunctionParameterPositions(functionType *ast.FuncType) []token.Pos {
	var positions []token.Pos
	if functionType == nil || functionType.Params == nil {
		return positions
	}

	for _, field := range functionType.Params.List {
		if len(field.Names) == 0 {
			positions = append(positions, token.NoPos)

			continue
		}

		for _, name := range field.Names {
			positions = append(positions, identifierObjectPosition(name))
		}
	}

	return positions
}

func scalarClosureValues(literal *ast.FuncLit, evaluation *scalarEvaluation) map[token.Pos]bool {
	internal := scalarFunctionObjectPositions(literal)
	captures := make(map[token.Pos]bool)

	ast.Inspect(literal.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Obj == nil {
			return true
		}

		position := identifierObjectPosition(identifier)
		if internal[position] {
			return true
		}

		if _, already := captures[position]; !already {
			captures[position] = scalarIdentifierSafe(identifier, evaluation)
		}

		return true
	})

	return captures
}

func scalarFunctionObjectPositions(literal *ast.FuncLit) map[token.Pos]bool {
	positions := make(map[token.Pos]bool)

	if literal.Type.Params != nil {
		collectScalarParameterObjects(literal.Type.Params, positions)
	}

	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if node == nil {
			return false
		}

		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}

		collectScalarBodyObject(node, positions)

		return true
	})

	return positions
}

func collectScalarParameterObjects(parameters *ast.FieldList, positions map[token.Pos]bool) {
	for _, field := range parameters.List {
		for _, name := range field.Names {
			positions[identifierObjectPosition(name)] = true
		}
	}
}

func collectScalarBodyObject(node ast.Node, positions map[token.Pos]bool) {
	switch typed := node.(type) {
	case *ast.ValueSpec:
		collectScalarValueObjects(typed.Names, positions)
	case *ast.AssignStmt:
		collectScalarAssignmentObjects(typed.Lhs, positions)
	}
}

func collectScalarValueObjects(names []*ast.Ident, positions map[token.Pos]bool) {
	for _, name := range names {
		if name.Obj != nil {
			positions[identifierObjectPosition(name)] = true
		}
	}
}

func collectScalarAssignmentObjects(targets []ast.Expr, positions map[token.Pos]bool) {
	for _, target := range targets {
		identifier, isIdentifier := unparen(target).(*ast.Ident)
		if isIdentifier && identifier.Obj != nil && identifier.Obj.Pos() == identifier.Pos() {
			positions[identifierObjectPosition(identifier)] = true
		}
	}
}

func scalarReturnedCallbackTargets(
	call *ast.CallExpr,
	evaluation *scalarEvaluation,
	visiting map[token.Pos]bool,
) []scalarFunction {
	factories := scalarCallTargets(call.Fun, evaluation, visiting)

	var callbacks []scalarFunction

	for _, factory := range factories {
		name := scalarFunctionKey(factory)
		if evaluation.callStack[name] {
			continue
		}

		factoryStack := cloneScalarCallStack(evaluation.callStack)
		factoryStack[name] = true

		arguments := make([]bool, 0, len(call.Args))
		for _, argument := range call.Args {
			arguments = append(arguments, scalarExpressionSafe(argument, evaluation))
		}

		factoryEvaluation := scalarFunctionEvaluation(factory, call.Args, arguments, evaluation, factoryStack)
		callbacks = append(callbacks, scalarFunctionReturnedCallbacks(factory, factoryEvaluation, visiting)...)
	}

	return callbacks
}

func scalarFunctionReturnedCallbacks(
	factory scalarFunction,
	evaluation *scalarEvaluation,
	visiting map[token.Pos]bool,
) []scalarFunction {
	functionType := factory.functionType()
	if functionType == nil || functionType.Results == nil || len(functionType.Results.List) != 1 {
		return nil
	}

	if factory.callerSupplied {
		return scalarCallerReturnedCallbacks(factory)
	}

	resultNames := functionResultNames(functionType)
	if len(resultNames) == 1 && scalarNamedCallbackResultIsConditional(factory.body(), resultNames[0]) {
		return nil
	}

	callbacks, hasReturn, allResolved := scalarCallbackTargetsByReturn(factory, evaluation, visiting, resultNames)
	if !hasReturn || !allResolved {
		return nil
	}

	return callbacks
}

func scalarCallerReturnedCallbacks(factory scalarFunction) []scalarFunction {
	functionType := factory.functionType()

	callbackType, isCallback := unparen(functionType.Results.List[0].Type).(*ast.FuncType)
	if !isCallback {
		return nil
	}

	return []scalarFunction{newCallerScalarFunction(factory.source, callbackType)}
}

func scalarCallbackTargetsByReturn(
	factory scalarFunction,
	evaluation *scalarEvaluation,
	visiting map[token.Pos]bool,
	resultNames []*ast.Ident,
) ([]scalarFunction, bool, bool) {
	var callbacks []scalarFunction

	hasReturn := false
	allReturnsResolved := true

	ast.Inspect(factory.body(), func(node ast.Node) bool {
		if node == nil {
			return true
		}

		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}

		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		hasReturn = true
		returned, resolved := scalarCallbackTargetsForReturn(statement, evaluation, visiting, resultNames)
		callbacks = append(callbacks, returned...)
		allReturnsResolved = allReturnsResolved && resolved

		return true
	})

	return callbacks, hasReturn, allReturnsResolved
}

func scalarCallbackTargetsForReturn(
	statement *ast.ReturnStmt,
	evaluation *scalarEvaluation,
	visiting map[token.Pos]bool,
	resultNames []*ast.Ident,
) ([]scalarFunction, bool) {
	if len(statement.Results) == 1 {
		returned := scalarCallTargets(statement.Results[0], evaluation, visiting)

		return returned, len(returned) != 0
	}

	if len(statement.Results) == 0 && len(resultNames) == 1 {
		returned := scalarVariableFunctionTargets(resultNames[0], evaluation, visiting)

		return returned, len(returned) != 0
	}

	return nil, false
}

func scalarNamedCallbackResultIsConditional(body *ast.BlockStmt, result *ast.Ident) bool {
	if body == nil || result == nil {
		return true
	}

	return scalarHasConditionalResultWrite(body, identifierObjectPosition(result), scalarConditionalRanges(body))
}

func scalarConditionalRanges(body *ast.BlockStmt) []scalarSourceRange {
	var ranges []scalarSourceRange

	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			return false
		}

		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}

		ranges = append(ranges, scalarControlFlowRanges(node)...)

		return true
	})

	return ranges
}

func scalarControlFlowRanges(node ast.Node) []scalarSourceRange {
	var ranges []scalarSourceRange

	switch typed := node.(type) {
	case *ast.IfStmt:
		ranges = append(ranges, scalarNodeRange(typed.Body))
		if typed.Else != nil {
			ranges = append(ranges, scalarNodeRange(typed.Else))
		}
	case *ast.SwitchStmt:
		ranges = append(ranges, scalarNodeRange(typed.Body))
	case *ast.TypeSwitchStmt:
		ranges = append(ranges, scalarNodeRange(typed.Body))
	case *ast.SelectStmt:
		ranges = append(ranges, scalarNodeRange(typed.Body))
	case *ast.ForStmt:
		ranges = append(ranges, scalarNodeRange(typed.Body))
	case *ast.RangeStmt:
		ranges = append(ranges, scalarNodeRange(typed.Body))
	}

	return ranges
}

func scalarNodeRange(node ast.Node) scalarSourceRange {
	return scalarSourceRange{start: node.Pos(), end: node.End()}
}

func scalarHasConditionalResultWrite(
	body *ast.BlockStmt,
	resultPosition token.Pos,
	ranges []scalarSourceRange,
) bool {
	conditionalWrite := false

	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || conditionalWrite {
			return false
		}

		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}

		assignment, isAssignment := node.(*ast.AssignStmt)
		if !isAssignment {
			return true
		}

		conditionalWrite = scalarAssignmentHasConditionalResult(assignment, resultPosition, ranges)

		return !conditionalWrite
	})

	return conditionalWrite
}

func scalarAssignmentHasConditionalResult(
	assignment *ast.AssignStmt,
	resultPosition token.Pos,
	ranges []scalarSourceRange,
) bool {
	for _, target := range assignment.Lhs {
		identifier, isIdentifier := unparen(target).(*ast.Ident)
		if !isIdentifier || identifierObjectPosition(identifier) != resultPosition {
			continue
		}

		for _, sourceRange := range ranges {
			if identifier.Pos() >= sourceRange.start && identifier.Pos() <= sourceRange.end {
				return true
			}
		}
	}

	return false
}

type scalarSourceRange struct {
	start token.Pos
	end   token.Pos
}

func scalarFunctionBodySafe(functionType *ast.FuncType, body *ast.BlockStmt, evaluation *scalarEvaluation) bool {
	if functionType == nil || functionType.Results == nil || len(functionType.Results.List) != 1 {
		return false
	}

	return scalarReturnsSafe(body, functionResultNames(functionType), evaluation)
}

func scalarReturnsSafe(body *ast.BlockStmt, resultNames []*ast.Ident, evaluation *scalarEvaluation) bool {
	returnCount := 0
	valid := true

	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || !valid {
			return false
		}

		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}

		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		returnCount++
		valid = scalarReturnSafe(statement, resultNames, evaluation)

		return valid
	})

	return returnCount != 0 && valid
}

func scalarReturnSafe(
	statement *ast.ReturnStmt,
	resultNames []*ast.Ident,
	evaluation *scalarEvaluation,
) bool {
	if len(statement.Results) == 1 {
		return scalarExpressionSafe(statement.Results[0], evaluation)
	}

	if len(statement.Results) != 0 || len(resultNames) != 1 {
		return false
	}

	return scalarIdentifierSafe(resultNames[0], evaluation)
}

func scalarFunctionKey(function scalarFunction) string {
	if function.declaration != nil {
		return function.source.path + ":" + function.declaration.Name.Name + ":" +
			fmt.Sprint(function.declaration.Pos())
	}

	if function.literal == nil {
		return fmt.Sprintf("%s:external:%p", function.source.path, function.typeOverride)
	}

	return function.source.path + ":literal:" + fmt.Sprint(function.literal.Pos())
}

func (function scalarFunction) functionType() *ast.FuncType {
	if function.typeOverride != nil {
		return function.typeOverride
	}

	if function.declaration != nil {
		return function.declaration.Type
	}

	if function.literal != nil {
		return function.literal.Type
	}

	return nil
}

func (function scalarFunction) body() *ast.BlockStmt {
	if function.declaration != nil {
		return function.declaration.Body
	}

	if function.literal != nil {
		return function.literal.Body
	}

	return nil
}
