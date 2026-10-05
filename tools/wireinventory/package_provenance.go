package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"strings"
)

type packageProvenanceFile struct {
	path    string
	fileSet *token.FileSet
	file    *ast.File
}

type packageWireHelpers struct {
	fixedScalars map[string]bool
	wireMaps     map[string]bool
}

func readGeneratedModelScalars(root string) (map[string]bool, error) {
	types := make(map[string]ast.Expr)
	outputs := generatedDependencyModelOutputs()

	for _, output := range outputs {
		declarations, err := readGeneratedTypeDeclarations(root, output)
		if err != nil {
			return nil, err
		}

		maps.Copy(types, declarations)
	}

	scalars := make(map[string]bool)

	for name := range types {
		if generatedScalarUnderlyingType(name, types, make(map[string]bool)) {
			scalars[name] = true
		}
	}

	return scalars, nil
}

func generatedDependencyModelOutputs() []string {
	var outputs []string

	for _, source := range generatedModelSources() {
		if isDependencyModelOutput(source.outputPath) {
			outputs = append(outputs, source.outputPath)
		}
	}

	for _, output := range compatibilityModelOutputs() {
		if isDependencyModelOutput(output) {
			outputs = append(outputs, output)
		}
	}

	return outputs
}

func isDependencyModelOutput(output string) bool {
	return strings.HasPrefix(filepath.ToSlash(output), "pkg/dependencymodels/")
}

func readGeneratedTypeDeclarations(root, output string) (map[string]ast.Expr, error) {
	path := filepath.Join(root, filepath.FromSlash(output))

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse generated scalar declarations from %s: %w", output, err)
	}

	declarations := make(map[string]ast.Expr)

	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}

		collectGeneratedTypeSpecs(general, declarations)
	}

	return declarations, nil
}

func collectGeneratedTypeSpecs(declaration *ast.GenDecl, types map[string]ast.Expr) {
	for _, rawSpec := range declaration.Specs {
		spec, ok := rawSpec.(*ast.TypeSpec)
		if ok {
			types[spec.Name.Name] = spec.Type
		}
	}
}

func readGeneratedWireModels(root string) (map[string]bool, map[string]bool, error) {
	types, err := readGeneratedModelTypes(root)
	if err != nil {
		return nil, nil, err
	}

	scalars, err := readGeneratedModelScalars(root)
	if err != nil {
		return nil, nil, err
	}

	return types, scalars, nil
}

func generatedScalarUnderlyingType(name string, types map[string]ast.Expr, visiting map[string]bool) bool {
	if isScalarConversion(name) {
		return true
	}

	expression, found := types[name]
	if !found || visiting[name] {
		return false
	}

	visiting[name] = true
	defer delete(visiting, name)

	identifier, ok := unparen(expression).(*ast.Ident)

	return ok && generatedScalarUnderlyingType(identifier.Name, types, visiting)
}

func checkPackageWireProvenance(
	paths []string,
	generatedTypes, generatedScalars map[string]bool,
	scalarMetadata generatedScalarMetadata,
) error {
	packages := make(map[string][]packageProvenanceFile)

	for _, path := range paths {
		if isRegisteredGeneratedPath(path) {
			continue
		}

		fileSet := token.NewFileSet()

		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse package source for wire provenance %s: %w", path, err)
		}

		directory := filepath.Clean(filepath.Dir(path))
		packages[directory] = append(packages[directory], packageProvenanceFile{
			path: path, fileSet: fileSet, file: file,
		})
	}

	for _, files := range packages {
		helpers := packageHelpers(files, generatedTypes, generatedScalars)

		for _, source := range files {
			err := checkPackageGeneratedScalarSinks(source, helpers, generatedTypes, generatedScalars)
			if err != nil {
				return err
			}

			err = checkPackageWireMapEscapes(source, helpers)
			if err != nil {
				return err
			}
		}

		err := checkPackageScalarProvenance(files, generatedTypes, generatedScalars, scalarMetadata)
		if err != nil {
			return err
		}
	}

	return nil
}

func packageHelpers(
	files []packageProvenanceFile,
	generatedTypes, generatedScalars map[string]bool,
) packageWireHelpers {
	return packageWireHelpers{
		fixedScalars: packageFixedScalarHelpers(files, generatedTypes, generatedScalars),
		wireMaps:     packageWireMapHelpers(files),
	}
}

func packageWireMapHelpers(files []packageProvenanceFile) map[string]bool {
	helpers := make(map[string]bool)

	for _, source := range files {
		wireMapTypes := collectWireMapTypes(source.file)

		for _, declaration := range source.file.Decls {
			if function := packageWireMapResultFunction(declaration); function != nil {
				resultType := function.Type.Results.List[0].Type
				if isWireMapType(source.file, resultType, wireMapTypes) {
					helpers[function.Name.Name] = true
				}
			}
		}
	}

	return helpers
}

func packageWireMapResultFunction(declaration ast.Decl) *ast.FuncDecl {
	function, ok := declaration.(*ast.FuncDecl)
	if !ok || function.Recv != nil || function.Body == nil || function.Type.Results == nil ||
		len(function.Type.Results.List) != 1 {
		return nil
	}

	return function
}

func packageFixedScalarHelpers(
	files []packageProvenanceFile,
	generatedTypes, generatedScalars map[string]bool,
) map[string]bool {
	helpers := make(map[string]bool)
	scalarAnalysisTypes := cloneStringSet(generatedTypes)

	for name := range generatedScalars {
		delete(scalarAnalysisTypes, name)
	}

	for {
		before := len(helpers)

		for _, source := range files {
			addFixedScalarHelpers(source, scalarAnalysisTypes, generatedScalars, helpers)
		}

		if before == len(helpers) {
			return helpers
		}
	}
}

func addFixedScalarHelpers(
	source packageProvenanceFile,
	analysisTypes, generatedScalars, fixedFunctions map[string]bool,
) {
	for _, declaration := range source.file.Decls {
		function := packageScalarResultFunction(source.file, declaration, generatedScalars)
		if function == nil || !returnsPackageFixedScalar(source.file, function, analysisTypes, fixedFunctions) {
			continue
		}

		fixedFunctions[function.Name.Name] = true
	}
}

func packageScalarResultFunction(file *ast.File, declaration ast.Decl, generatedScalars map[string]bool) *ast.FuncDecl {
	function, ok := declaration.(*ast.FuncDecl)
	if !ok || function.Recv != nil || function.Body == nil || !hasOneScalarResult(file, function, generatedScalars) {
		return nil
	}

	return function
}

func returnsPackageFixedScalar(
	file *ast.File,
	function *ast.FuncDecl,
	analysisTypes, fixedFunctions map[string]bool,
) bool {
	aliases := make(map[token.Pos]bool)
	localFixedFunctions := make(map[token.Pos]bool)
	collectScalarAliases(file, aliases, localFixedFunctions, analysisTypes)
	collectPackageFixedScalarAliases(function.Body, aliases, fixedFunctions)

	return functionReturnsFixedScalar(file, function.Type, function.Body, aliases, localFixedFunctions) ||
		functionReturnsPackageFixedScalar(function.Body, fixedFunctions)
}

func cloneStringSet(values map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(values))
	for value := range values {
		cloned[value] = true
	}

	return cloned
}

func hasOneScalarResult(file *ast.File, function *ast.FuncDecl, generatedScalars map[string]bool) bool {
	if function.Type.Results == nil || len(function.Type.Results.List) != 1 {
		return false
	}

	return isGeneratedWireScalarType(file, function.Type.Results.List[0].Type, generatedScalars)
}

func isGeneratedWireScalarType(file *ast.File, expression ast.Expr, generatedScalars map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return isGeneratedWireScalarType(file, typed.X, generatedScalars)
	case *ast.Ident:
		if isScalarConversion(typed.Name) {
			return true
		}

		if typed.Obj == nil || typed.Obj.Kind != ast.Typ {
			return false
		}

		spec, ok := typed.Obj.Decl.(*ast.TypeSpec)

		return ok && isGeneratedWireScalarType(file, spec.Type, generatedScalars)
	case *ast.SelectorExpr:
		return importedPath(file, typed.X) == dependencymodelsImportPath && generatedScalars[typed.Sel.Name]
	default:
		return false
	}
}

func collectPackageFixedScalarAliases(
	body *ast.BlockStmt,
	aliases map[token.Pos]bool,
	fixedFunctions map[string]bool,
) {
	for {
		before := len(aliases)

		ast.Inspect(body, func(node ast.Node) bool {
			addPackageFixedScalarAlias(node, aliases, fixedFunctions)

			return true
		})

		if before == len(aliases) {
			return
		}
	}
}

func addPackageFixedScalarAlias(
	node ast.Node,
	aliases map[token.Pos]bool,
	fixedFunctions map[string]bool,
) {
	switch typed := node.(type) {
	case *ast.ValueSpec:
		for index, name := range typed.Names {
			if name.Obj != nil && index < len(typed.Values) &&
				containsPackageFixedScalarCall(typed.Values[index], fixedFunctions) {
				aliases[identifierObjectPosition(name)] = true
			}
		}
	case *ast.AssignStmt:
		collectAssignedPackageScalarAliases(typed, aliases, fixedFunctions)
	}
}

func collectAssignedPackageScalarAliases(
	assignment *ast.AssignStmt,
	aliases map[token.Pos]bool,
	fixedFunctions map[string]bool,
) {
	for index, target := range assignment.Lhs {
		name, ok := unparen(target).(*ast.Ident)
		if ok && name.Obj != nil && index < len(assignment.Rhs) &&
			containsPackageFixedScalarCall(assignment.Rhs[index], fixedFunctions) {
			aliases[identifierObjectPosition(name)] = true
		}
	}
}

func functionReturnsPackageFixedScalar(body *ast.BlockStmt, fixedFunctions map[string]bool) bool {
	found := false

	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || found {
			return false
		}

		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}

		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		for _, result := range statement.Results {
			if containsPackageFixedScalarCall(result, fixedFunctions) {
				found = true

				return false
			}
		}

		return true
	})

	return found
}

func containsPackageFixedScalarCall(node ast.Node, fixedFunctions map[string]bool) bool {
	found := false

	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || found {
			return false
		}

		call, ok := child.(*ast.CallExpr)
		if ok && packageFunctionTarget(call.Fun, fixedFunctions) {
			found = true

			return false
		}

		return true
	})

	return found
}

func packageFunctionTarget(expression ast.Expr, functions map[string]bool) bool {
	return packageFunctionTargetSeen(expression, functions, make(map[token.Pos]bool))
}

func packageFunctionTargetSeen(
	expression ast.Expr,
	functions map[string]bool,
	visiting map[token.Pos]bool,
) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return packageFunctionIdentifierTarget(typed, functions, visiting)
	case *ast.CallExpr:
		return packageFunctionTargetSeen(typed.Fun, functions, visiting)
	}

	return false
}

func packageFunctionIdentifierTarget(
	identifier *ast.Ident,
	functions map[string]bool,
	visiting map[token.Pos]bool,
) bool {
	if functions[identifier.Name] {
		return true
	}

	if identifier.Obj == nil {
		return false
	}

	position := identifierObjectPosition(identifier)
	if position == token.NoPos || visiting[position] {
		return false
	}

	visiting[position] = true
	defer delete(visiting, position)

	return packageFunctionDeclarationTarget(identifier, functions, visiting)
}

func packageFunctionDeclarationTarget(
	identifier *ast.Ident,
	functions map[string]bool,
	visiting map[token.Pos]bool,
) bool {
	switch declaration := identifier.Obj.Decl.(type) {
	case *ast.ValueSpec:
		index := valueNameIndex(declaration.Names, identifier)

		return index >= 0 && index < len(declaration.Values) &&
			packageFunctionTargetSeen(declaration.Values[index], functions, visiting)
	case *ast.AssignStmt:
		index := assignmentNameIndex(declaration.Lhs, identifier)

		return index >= 0 && index < len(declaration.Rhs) &&
			packageFunctionTargetSeen(declaration.Rhs[index], functions, visiting)
	case *ast.FuncDecl:
		return functions[declaration.Name.Name]
	default:
		return false
	}
}

func checkPackageGeneratedScalarSinks(
	source packageProvenanceFile,
	helpers packageWireHelpers,
	generatedTypes, generatedScalars map[string]bool,
) error {
	aliases := packageGeneratedScalarAliases(source, helpers, generatedTypes, generatedScalars)
	generatedVariables := packageGeneratedScalarVariables(source, generatedTypes)

	var violation error

	ast.Inspect(source.file, func(node ast.Node) bool {
		if node == nil || violation != nil {
			return false
		}

		violation = packageGeneratedScalarSinkViolation(source, node, aliases, helpers.fixedScalars,
			generatedTypes, generatedVariables)

		return violation == nil
	})

	return violation
}

func packageGeneratedScalarAliases(
	source packageProvenanceFile,
	helpers packageWireHelpers,
	generatedTypes, generatedScalars map[string]bool,
) map[token.Pos]bool {
	aliases := make(map[token.Pos]bool)
	localFixed := make(map[token.Pos]bool)
	analysisTypes := cloneStringSet(generatedTypes)

	for name := range generatedScalars {
		delete(analysisTypes, name)
	}

	collectScalarAliases(source.file, aliases, localFixed, analysisTypes)

	for _, declaration := range source.file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Body != nil {
			collectPackageFixedScalarAliases(function.Body, aliases, helpers.fixedScalars)
		}
	}

	return aliases
}

func packageGeneratedScalarVariables(
	source packageProvenanceFile,
	generatedTypes map[string]bool,
) map[token.Pos]bool {
	variables := make(map[token.Pos]bool)
	collectGeneratedVariableAliases(source.file, generatedTypes, variables)

	return variables
}

func packageGeneratedScalarSinkViolation(
	source packageProvenanceFile,
	node ast.Node,
	aliases map[token.Pos]bool,
	fixedFunctions, generatedTypes map[string]bool,
	generatedVariables map[token.Pos]bool,
) error {
	switch typed := node.(type) {
	case *ast.AssignStmt:
		return packageScalarAssignmentViolation(source, typed, aliases, fixedFunctions, generatedVariables)
	case *ast.CompositeLit:
		return packageScalarCompositeViolation(source, typed, aliases, fixedFunctions, generatedTypes)
	default:
		return nil
	}
}

func packageScalarAssignmentViolation(
	source packageProvenanceFile,
	assignment *ast.AssignStmt,
	aliases map[token.Pos]bool,
	fixedFunctions map[string]bool,
	generatedVariables map[token.Pos]bool,
) error {
	for index, target := range assignment.Lhs {
		if index < len(assignment.Rhs) && isGeneratedModelField(target, generatedVariables) &&
			containsPackageFixedScalarValue(assignment.Rhs[index], aliases, fixedFunctions) {
			return sourceError(source.fileSet, source.path, assignment.Rhs[index].Pos(), generatedScalarMessage)
		}
	}

	return nil
}

func packageScalarCompositeViolation(
	source packageProvenanceFile,
	composite *ast.CompositeLit,
	aliases map[token.Pos]bool,
	fixedFunctions, generatedTypes map[string]bool,
) error {
	if !isProviderWireModelType(source.file, composite.Type, generatedTypes) {
		return nil
	}

	for _, element := range composite.Elts {
		value := compositeValue(element)
		if containsPackageFixedScalarValue(value, aliases, fixedFunctions) {
			return sourceError(source.fileSet, source.path, element.Pos(), generatedScalarMessage)
		}
	}

	return nil
}

func containsPackageFixedScalarValue(
	node ast.Node,
	aliases map[token.Pos]bool,
	fixedFunctions map[string]bool,
) bool {
	found := false

	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || found {
			return false
		}

		switch typed := child.(type) {
		case *ast.CallExpr:
			found = packageFunctionTarget(typed.Fun, fixedFunctions)
		case *ast.Ident:
			found = aliases[identifierObjectPosition(typed)]
		}

		return !found
	})

	return found
}

func checkPackageWireMapEscapes(source packageProvenanceFile, helpers packageWireHelpers) error {
	aliases := collectPackageWireMapAliases(source, helpers.wireMaps)

	var violation error

	calledSelectors := calledWireMapSelectors(source.file)

	ast.Inspect(source.file, func(node ast.Node) bool {
		if node == nil || violation != nil {
			return false
		}

		violation = packageWireMapNodeViolation(source, node, aliases, helpers.wireMaps, calledSelectors)

		return violation == nil
	})

	return violation
}

func collectPackageWireMapAliases(source packageProvenanceFile, helpers map[string]bool) map[string]bool {
	aliases := make(map[string]bool)

	for {
		before := len(aliases)

		ast.Inspect(source.file, func(node ast.Node) bool {
			collectPackageWireMapAlias(source.file, node, aliases, helpers)

			return true
		})

		if before == len(aliases) {
			return aliases
		}
	}
}

func collectPackageWireMapAlias(
	file *ast.File,
	node ast.Node,
	aliases, helpers map[string]bool,
) {
	switch typed := node.(type) {
	case *ast.ValueSpec:
		for index, name := range typed.Names {
			if name.Obj != nil && index < len(typed.Values) &&
				isPackageWireMapSource(file, typed.Values[index], aliases, helpers) {
				aliases[identifierKey(name)] = true
			}
		}
	case *ast.AssignStmt:
		collectAssignedPackageWireMapAliases(file, typed, aliases, helpers)
	}
}

func collectAssignedPackageWireMapAliases(
	file *ast.File,
	assignment *ast.AssignStmt,
	aliases, helpers map[string]bool,
) {
	for index, target := range assignment.Lhs {
		name, ok := unparen(target).(*ast.Ident)
		if ok && name.Obj != nil && index < len(assignment.Rhs) &&
			isPackageWireMapSource(file, assignment.Rhs[index], aliases, helpers) {
			aliases[identifierKey(name)] = true
		}
	}
}

func packageWireMapNodeViolation(
	source packageProvenanceFile,
	node ast.Node,
	aliases map[string]bool,
	helpers map[string]bool,
	calledSelectors map[*ast.SelectorExpr]bool,
) error {
	switch typed := node.(type) {
	case *ast.CallExpr:
		return packageWireMapCallViolation(source, typed, aliases, helpers)
	case *ast.ReturnStmt:
		return packageWireMapReturnViolation(source, typed, aliases, helpers)
	case *ast.CompositeLit:
		return packageWireMapCompositeViolation(source, typed, aliases, helpers)
	case *ast.AssignStmt:
		return packageWireMapAssignmentViolation(source, typed, aliases, helpers)
	case *ast.ValueSpec:
		return packageWireMapValueViolation(source, typed, aliases, helpers)
	case *ast.SelectorExpr:
		return packageWireMapMethodValueViolation(source, typed, aliases, helpers, calledSelectors)
	case *ast.UnaryExpr:
		return packageWireMapAddressViolation(source, typed, aliases, helpers)
	case *ast.IndexExpr:
		return packageWireMapIndexViolation(source, typed, aliases, helpers)
	}

	return nil
}

func packageWireMapCallViolation(
	source packageProvenanceFile,
	call *ast.CallExpr,
	aliases, helpers map[string]bool,
) error {
	selector, hasSelector := unparen(call.Fun).(*ast.SelectorExpr)
	if hasSelector && isPackageWireMapSource(source.file, selector.X, aliases, helpers) {
		return packageWireMapMethodCallViolation(source, call, selector)
	}

	return packageWireMapArgumentViolation(source, call.Args, aliases, helpers)
}

func packageWireMapMethodCallViolation(
	source packageProvenanceFile,
	call *ast.CallExpr,
	selector *ast.SelectorExpr,
) error {
	if isSchemaKeyMethod(selector.Sel.Name) {
		if len(call.Args) == 0 || !isGeneratedModelConstant(source.file, call.Args[0]) {
			return generatedWireKeyError(source.fileSet, source.path, selector.Pos())
		}

		return nil
	}

	if selector.Sel.Name == queryEncodeMethodName && len(call.Args) == 0 {
		return nil
	}

	return sourceError(source.fileSet, source.path, selector.Pos(),
		"query and header method values cannot escape key validation")
}

func packageWireMapArgumentViolation(
	source packageProvenanceFile,
	arguments []ast.Expr,
	aliases, helpers map[string]bool,
) error {
	for _, argument := range arguments {
		if containsPackageWireMapSource(source.file, argument, aliases, helpers) {
			return sourceError(source.fileSet, source.path, argument.Pos(),
				"query and header maps cannot escape to an unverified helper")
		}
	}

	return nil
}

func packageWireMapReturnViolation(
	source packageProvenanceFile,
	statement *ast.ReturnStmt,
	aliases, helpers map[string]bool,
) error {
	for _, result := range statement.Results {
		if containsPackageWireMapSource(source.file, result, aliases, helpers) {
			return sourceError(source.fileSet, source.path, result.Pos(),
				"query and header maps cannot escape their checked function")
		}
	}

	return nil
}

func packageWireMapCompositeViolation(
	source packageProvenanceFile,
	composite *ast.CompositeLit,
	aliases, helpers map[string]bool,
) error {
	for _, element := range composite.Elts {
		if containsPackageWireMapSource(source.file, element, aliases, helpers) {
			return sourceError(source.fileSet, source.path, element.Pos(),
				"query and header maps cannot be stored in aggregate values")
		}
	}

	return nil
}

func packageWireMapAssignmentViolation(
	source packageProvenanceFile,
	assignment *ast.AssignStmt,
	aliases, helpers map[string]bool,
) error {
	for index, target := range assignment.Lhs {
		if index >= len(assignment.Rhs) ||
			!containsPackageWireMapSource(source.file, assignment.Rhs[index], aliases, helpers) {
			continue
		}

		targetViolation := packageWireMapAssignmentTargetViolation(source, target)
		if targetViolation != nil {
			return targetViolation
		}
	}

	return nil
}

func packageWireMapAssignmentTargetViolation(source packageProvenanceFile, target ast.Expr) error {
	switch destination := unparen(target).(type) {
	case *ast.SelectorExpr, *ast.IndexExpr:
		return sourceError(source.fileSet, source.path, target.Pos(),
			"query and header maps cannot be stored in aggregate values")
	case *ast.Ident:
		if destination.Obj != nil && source.file.Scope.Lookup(destination.Name) == destination.Obj {
			return sourceError(source.fileSet, source.path, target.Pos(),
				"query and header maps cannot be stored in package state")
		}
	}

	return nil
}

func packageWireMapValueViolation(
	source packageProvenanceFile,
	value *ast.ValueSpec,
	aliases, helpers map[string]bool,
) error {
	for index, name := range value.Names {
		if index < len(value.Values) && name.Obj != nil && source.file.Scope.Lookup(name.Name) == name.Obj &&
			containsPackageWireMapSource(source.file, value.Values[index], aliases, helpers) {
			return sourceError(source.fileSet, source.path, name.Pos(),
				"query and header maps cannot be stored in package state")
		}
	}

	return nil
}

func packageWireMapMethodValueViolation(
	source packageProvenanceFile,
	selector *ast.SelectorExpr,
	aliases, helpers map[string]bool,
	calledSelectors map[*ast.SelectorExpr]bool,
) error {
	if !calledSelectors[selector] &&
		(isSchemaKeyMethod(selector.Sel.Name) || selector.Sel.Name == queryEncodeMethodName) &&
		isPackageWireMapSource(source.file, selector.X, aliases, helpers) {
		return sourceError(source.fileSet, source.path, selector.Pos(),
			"query and header method values cannot escape key validation")
	}

	return nil
}

func packageWireMapAddressViolation(
	source packageProvenanceFile,
	expression *ast.UnaryExpr,
	aliases, helpers map[string]bool,
) error {
	if expression.Op == token.AND &&
		containsPackageWireMapSource(source.file, expression.X, aliases, helpers) {
		return sourceError(source.fileSet, source.path, expression.Pos(),
			"query and header maps cannot escape through an address")
	}

	return nil
}

func packageWireMapIndexViolation(
	source packageProvenanceFile,
	index *ast.IndexExpr,
	aliases, helpers map[string]bool,
) error {
	if isPackageWireMapSource(source.file, index.X, aliases, helpers) &&
		!isGeneratedModelConstant(source.file, index.Index) {
		return generatedWireKeyError(source.fileSet, source.path, index.Index.Pos())
	}

	return nil
}

func isPackageWireMapFunctionCall(expression ast.Expr, helpers map[string]bool) bool {
	return packageFunctionTarget(expression, helpers)
}

func isPackageWireMapSource(
	file *ast.File,
	expression ast.Expr,
	aliases map[string]bool,
	helpers map[string]bool,
) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return aliases[identifierKey(typed)]
	case *ast.CallExpr:
		return isPackageWireMapCallSource(file, typed, aliases, helpers)
	case *ast.UnaryExpr, *ast.StarExpr:
		return isPackageWireMapSource(file, expressionOperand(typed), aliases, helpers)
	case *ast.CompositeLit:
		return isPackageWireMapCompositeSource(file, typed, aliases, helpers)
	case *ast.SelectorExpr:
		return isPackageWireMapSelectorSource(file, typed, aliases, helpers)
	}

	return false
}

func isPackageWireMapCallSource(
	file *ast.File,
	call *ast.CallExpr,
	aliases, helpers map[string]bool,
) bool {
	if isPackageWireMapFunctionCall(call.Fun, helpers) {
		return true
	}

	if len(call.Args) != 1 || !isWireMapType(file, call.Fun, collectWireMapTypes(file)) {
		return false
	}

	return isPackageWireMapSource(file, call.Args[0], aliases, helpers)
}

func isPackageWireMapCompositeSource(
	file *ast.File,
	composite *ast.CompositeLit,
	aliases, helpers map[string]bool,
) bool {
	for _, element := range composite.Elts {
		if isPackageWireMapSource(file, compositeValue(element), aliases, helpers) {
			return true
		}
	}

	return false
}

func isPackageWireMapSelectorSource(
	file *ast.File,
	selector *ast.SelectorExpr,
	aliases, helpers map[string]bool,
) bool {
	return selector.Sel.Name == httpHeaderFieldName && hasImportPath(file, netHTTPImportPath) &&
		isPackageWireMapSource(file, selector.X, aliases, helpers)
}

func containsPackageWireMapSource(
	file *ast.File,
	node ast.Node,
	aliases map[string]bool,
	helpers map[string]bool,
) bool {
	found := false

	ast.Inspect(node, func(child ast.Node) bool {
		return inspectPackageWireMapSource(file, child, aliases, helpers, &found)
	})

	return found
}

func inspectPackageWireMapSource(
	file *ast.File,
	child ast.Node,
	aliases, helpers map[string]bool,
	found *bool,
) bool {
	if child == nil || *found {
		return false
	}

	if isApprovedPackageWireMapMethodCall(file, child, aliases, helpers) {
		return false
	}

	if expression, ok := child.(ast.Expr); ok && isPackageWireMapSource(file, expression, aliases, helpers) {
		*found = true

		return false
	}

	return true
}

func isApprovedPackageWireMapMethodCall(
	file *ast.File,
	child ast.Node,
	aliases, helpers map[string]bool,
) bool {
	call, isCall := child.(*ast.CallExpr)
	if !isCall {
		return false
	}

	selector, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || !isPackageWireMapSource(file, selector.X, aliases, helpers) {
		return false
	}

	return isSchemaKeyMethod(selector.Sel.Name) ||
		(selector.Sel.Name == queryEncodeMethodName && len(call.Args) == 0)
}
