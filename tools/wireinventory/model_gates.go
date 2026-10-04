package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	generatedScalarMessage = "fixed scalar in generated wire model must come from a schema-bound constant " +
		"or caller input"
	netHTTPImportPath           = "net/http"
	dependencymodelsImportPath  = "github.com/portpowered/go-tplink/pkg/dependencymodels"
	tplinkmodelsImportPath      = "github.com/portpowered/go-tplink/pkg/tplinkmodels"
	netURLImportPath            = "net/url"
	stringTypeName              = "string"
	getMethodName               = "Get"
	deleteName                  = "delete"
	requestArgumentCount        = 4
	cloudRequestArgumentCount   = 4
	requestHeaderArgumentCount  = 2
	httpNewRequestMethodName    = "NewRequestWithContext"
	urlParseArgumentCount       = 2
	generatedRequestStatusField = "Path"
	generatedRequestQueryField  = "RawQuery"
	urlValuesMakeFunctionName   = "make"
	querySetMethodName          = "Set"
	queryEncodeMethodName       = "Encode"
	httpHeaderFieldName         = "Header"
)

func identifierObjectPosition(identifier *ast.Ident) token.Pos {
	if identifier == nil || identifier.Obj == nil {
		return token.NoPos
	}

	return identifier.Obj.Pos()
}

func checkHandwrittenJSONModels(path string, fileSet *token.FileSet, file *ast.File) error {
	var violation error

	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil || violation != nil {
			return false
		}

		structure, ok := node.(*ast.StructType)
		if !ok || !hasJSONTag(structure) {
			return true
		}

		if typeSpecForStruct(file, structure) != nil {
			violation = sourceError(fileSet, path, structure.Pos(), handwrittenJSONModelMessage)

			return false
		}

		violation = sourceError(fileSet, path, structure.Pos(), anonymousJSONModelMessage)

		return false
	})

	return violation
}

func hasJSONTag(structure *ast.StructType) bool {
	for _, field := range structure.Fields.List {
		if field.Tag == nil {
			continue
		}

		tag, err := strconv.Unquote(field.Tag.Value)

		if err == nil && strings.Contains(tag, `json:`) {
			return true
		}
	}

	return false
}

func typeSpecForStruct(file *ast.File, structure *ast.StructType) *ast.TypeSpec {
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}

		for _, rawSpec := range general.Specs {
			spec, specFound := rawSpec.(*ast.TypeSpec)
			if !specFound {
				continue
			}

			candidate, ok := spec.Type.(*ast.StructType)
			if ok && candidate == structure {
				return spec
			}
		}
	}

	return nil
}

func checkGeneratedScalarValues(
	path string,
	fileSet *token.FileSet,
	file *ast.File,
	generatedTypes map[string]bool,
) error {
	generatedVariables := make(map[token.Pos]bool)
	scalarAliases := make(map[token.Pos]bool)

	collectGeneratedVariableAliases(file, generatedTypes, generatedVariables)
	collectScalarAliases(file, scalarAliases, generatedTypes)

	err := checkGeneratedScalarAssignments(path, fileSet, file, generatedVariables, scalarAliases)
	if err != nil {
		return err
	}

	return checkGeneratedScalarComposites(path, fileSet, file, generatedTypes, scalarAliases)
}

func checkGeneratedScalarAssignments(
	path string,
	fileSet *token.FileSet,
	file *ast.File,
	generatedVariables, scalarAliases map[token.Pos]bool,
) error {
	var violation error

	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil || violation != nil {
			return false
		}

		assignment, ok := node.(*ast.AssignStmt)

		if ok {
			violation = generatedAssignmentScalarViolation(path, fileSet, assignment, generatedVariables, scalarAliases)
		}

		return violation == nil
	})

	return violation
}

func generatedAssignmentScalarViolation(
	path string,
	fileSet *token.FileSet,
	assignment *ast.AssignStmt,
	generatedVariables, scalarAliases map[token.Pos]bool,
) error {
	for index, target := range assignment.Lhs {
		if index >= len(assignment.Rhs) || !isGeneratedModelField(target, generatedVariables) {
			continue
		}

		if containsFixedScalar(assignment.Rhs[index], scalarAliases) {
			return sourceError(fileSet, path, assignment.Rhs[index].Pos(), generatedScalarMessage)
		}
	}

	return nil
}

func checkGeneratedScalarComposites(
	path string,
	fileSet *token.FileSet,
	file *ast.File,
	generatedTypes map[string]bool,
	scalarAliases map[token.Pos]bool,
) error {
	var violation error

	ast.Inspect(file, func(node ast.Node) bool {
		if violation != nil {
			return false
		}

		composite, ok := node.(*ast.CompositeLit)

		if !ok || !isProviderWireModelType(file, composite.Type, generatedTypes) {
			return true
		}

		for _, element := range composite.Elts {
			if containsFixedScalar(compositeValue(element), scalarAliases) {
				violation = sourceError(fileSet, path, element.Pos(), generatedScalarMessage)

				return false
			}
		}

		return violation == nil
	})

	return violation
}

func collectGeneratedVariableAliases(file *ast.File, generatedTypes map[string]bool, variables map[token.Pos]bool) {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		collectGeneratedParameterAliases(file, function, generatedTypes, variables)
		collectGeneratedLocalAliases(file, function, generatedTypes, variables)
	}
}

func collectGeneratedParameterAliases(
	file *ast.File,
	function *ast.FuncDecl,
	generatedTypes map[string]bool,
	variables map[token.Pos]bool,
) {
	if function.Type.Params == nil {
		return
	}

	for _, field := range function.Type.Params.List {
		if !isGeneratedModelType(file, field.Type, generatedTypes) {
			continue
		}

		for _, name := range field.Names {
			if name.Obj != nil {
				variables[identifierObjectPosition(name)] = true
			}
		}
	}
}

func collectGeneratedLocalAliases(
	file *ast.File,
	function *ast.FuncDecl,
	generatedTypes map[string]bool,
	variables map[token.Pos]bool,
) {
	for {
		before := len(variables)

		ast.Inspect(function.Body, func(node ast.Node) bool {
			collectGeneratedAliasNode(file, node, generatedTypes, variables)

			return true
		})

		if before == len(variables) {
			return
		}
	}
}

func collectGeneratedAliasNode(
	file *ast.File,
	node ast.Node,
	generatedTypes map[string]bool,
	variables map[token.Pos]bool,
) {
	switch typed := node.(type) {
	case *ast.ValueSpec:
		collectGeneratedValueAliases(file, typed, generatedTypes, variables)
	case *ast.AssignStmt:
		collectGeneratedAssignmentAliases(file, typed, generatedTypes, variables)
	}
}

func collectGeneratedValueAliases(
	file *ast.File,
	value *ast.ValueSpec,
	generatedTypes map[string]bool,
	variables map[token.Pos]bool,
) {
	for index, name := range value.Names {
		if name.Obj == nil {
			continue
		}

		if isProviderWireModelType(file, value.Type, generatedTypes) ||
			(index < len(value.Values) && isProviderWireModelExpression(file, value.Values[index], generatedTypes, variables)) {
			variables[identifierObjectPosition(name)] = true
		}
	}
}

func collectGeneratedAssignmentAliases(
	file *ast.File,
	assignment *ast.AssignStmt,
	generatedTypes map[string]bool,
	variables map[token.Pos]bool,
) {
	for index, target := range assignment.Lhs {
		name, ok := unparen(target).(*ast.Ident)
		if ok && name.Obj != nil && index < len(assignment.Rhs) &&
			isProviderWireModelExpression(file, assignment.Rhs[index], generatedTypes, variables) {
			variables[identifierObjectPosition(name)] = true
		}
	}
}

func isProviderWireModelExpression(
	file *ast.File,
	expression ast.Expr,
	generatedTypes map[string]bool,
	variables map[token.Pos]bool,
) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.CompositeLit:
		return isProviderWireModelType(file, typed.Type, generatedTypes)
	case *ast.Ident:
		return variables[identifierObjectPosition(typed)]
	case *ast.UnaryExpr:
		return isProviderWireModelExpression(file, typed.X, generatedTypes, variables)
	case *ast.CallExpr:
		return isProviderWireModelType(file, typed.Fun, generatedTypes) && len(typed.Args) == 1 &&
			isProviderWireModelExpression(file, typed.Args[0], generatedTypes, variables)
	}

	return false
}

func isProviderWireModelType(file *ast.File, expression ast.Expr, generatedTypes map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return isProviderWireModelType(file, typed.X, generatedTypes)
	case *ast.SelectorExpr:
		return generatedTypes[typed.Sel.Name] &&
			importedPath(file, typed.X) == dependencymodelsImportPath
	}

	return false
}

func isGeneratedModelType(file *ast.File, expression ast.Expr, generatedTypes map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return isGeneratedModelType(file, typed.X, generatedTypes)
	case *ast.Ident:
		return file.Name.Name == "main" && generatedTypes[typed.Name]
	case *ast.SelectorExpr:
		return generatedTypes[typed.Sel.Name] && isGeneratedModelPackage(file, typed.X)
	}

	return false
}

func isGeneratedModelPackage(file *ast.File, expression ast.Expr) bool {
	path := importedPath(file, expression)

	return path == dependencymodelsImportPath || path == tplinkmodelsImportPath
}

func importedPath(file *ast.File, expression ast.Expr) string {
	identifier, ok := unparen(expression).(*ast.Ident)
	if !ok {
		return ""
	}

	for _, importSpec := range file.Imports {
		path, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			continue
		}

		name := filepath.Base(path)
		if importSpec.Name != nil {
			name = importSpec.Name.Name
		}

		if identifier.Name == name {
			if identifier.Obj != nil && file.Scope.Lookup(name) != identifier.Obj {
				return ""
			}

			return path
		}
	}

	return ""
}

func collectScalarAliases(file *ast.File, aliases map[token.Pos]bool, generatedTypes map[string]bool) {
	for _, declaration := range file.Decls {
		collectPackageScalarAliases(declaration, aliases, file, generatedTypes)
	}

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		collectFunctionScalarAliases(file, function, aliases, generatedTypes)
	}
}

func collectPackageScalarAliases(
	declaration ast.Decl,
	aliases map[token.Pos]bool,
	file *ast.File,
	generatedTypes map[string]bool,
) {
	general, ok := declaration.(*ast.GenDecl)
	if !ok || (general.Tok != token.CONST && general.Tok != token.VAR) {
		return
	}

	for _, rawSpec := range general.Specs {
		spec, ok := rawSpec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for index, name := range spec.Names {
			addScalarAlias(name, index, spec.Values, aliases, file, generatedTypes)
		}
	}
}

func collectFunctionScalarAliases(
	file *ast.File,
	function *ast.FuncDecl,
	aliases map[token.Pos]bool,
	generatedTypes map[string]bool,
) {
	for {
		before := len(aliases)

		ast.Inspect(function.Body, func(node ast.Node) bool {
			collectScalarAliasNode(file, node, aliases, generatedTypes)

			return true
		})

		if before == len(aliases) {
			return
		}
	}
}

func collectScalarAliasNode(file *ast.File, node ast.Node, aliases map[token.Pos]bool, generatedTypes map[string]bool) {
	switch typed := node.(type) {
	case *ast.ValueSpec:
		for index, name := range typed.Names {
			addScalarAlias(name, index, typed.Values, aliases, file, generatedTypes)
		}
	case *ast.AssignStmt:
		for index, target := range typed.Lhs {
			name, ok := unparen(target).(*ast.Ident)
			if ok {
				addScalarAlias(name, index, typed.Rhs, aliases, file, generatedTypes)
			}
		}
	}
}

func addScalarAlias(
	name *ast.Ident,
	index int,
	values []ast.Expr,
	aliases map[token.Pos]bool,
	file *ast.File,
	generatedTypes map[string]bool,
) {
	if name.Obj == nil || index >= len(values) {
		return
	}

	value := values[index]
	if isFixedScalarSource(value, aliases, file, generatedTypes) || containsScalarSyntax(value, aliases) {
		aliases[identifierObjectPosition(name)] = true
	}
}

func containsFixedScalar(expression ast.Expr, aliases map[token.Pos]bool) bool {
	return containsScalarSyntax(expression, aliases)
}

func isFixedScalarSource(
	expression ast.Expr,
	aliases map[token.Pos]bool,
	file *ast.File,
	generatedTypes map[string]bool,
) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.BasicLit:
		return isScalarLiteral(typed)
	case *ast.Ident:
		return typed.Name == "true" || typed.Name == "false" || aliases[identifierObjectPosition(typed)]
	case *ast.UnaryExpr:
		return isFixedScalarSource(typed.X, aliases, file, generatedTypes)
	case *ast.CallExpr:
		return fixedScalarCallSource(typed, aliases, file, generatedTypes)
	}

	return false
}

func isScalarLiteral(literal *ast.BasicLit) bool {
	return literal.Kind == token.STRING || literal.Kind == token.INT || literal.Kind == token.FLOAT ||
		literal.Kind == token.CHAR || literal.Kind == token.IMAG
}

func fixedScalarCallSource(
	call *ast.CallExpr,
	aliases map[token.Pos]bool,
	file *ast.File,
	generatedTypes map[string]bool,
) bool {
	if len(call.Args) != 1 {
		return false
	}

	function, ok := unparen(call.Fun).(*ast.Ident)

	isConversion := ok && isScalarConversion(function.Name)
	if !isConversion && !isProviderWireModelType(file, call.Fun, generatedTypes) {
		return false
	}

	return isFixedScalarSource(call.Args[0], aliases, file, generatedTypes)
}

func isScalarConversion(name string) bool {
	switch name {
	case stringTypeName, "bool", "byte", "rune", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
		return true
	default:
		return false
	}
}

func containsScalarSyntax(expression ast.Expr, aliases map[token.Pos]bool) bool {
	var found bool

	ast.Inspect(expression, func(node ast.Node) bool {
		if found || node == nil {
			return false
		}

		found = isScalarSyntaxNode(node, aliases)

		return !found
	})

	return found
}

func isScalarSyntaxNode(node ast.Node, aliases map[token.Pos]bool) bool {
	switch typed := node.(type) {
	case *ast.BasicLit:
		return isScalarLiteral(typed)
	case *ast.Ident:
		return typed.Name == "true" || typed.Name == "false" || aliases[identifierObjectPosition(typed)]
	default:
		return false
	}
}

func isGeneratedModelField(expression ast.Expr, variables map[token.Pos]bool) bool {
	selector, selectorFound := unparen(expression).(*ast.SelectorExpr)
	if !selectorFound {
		return false
	}

	for {
		base, ok := unparen(selector.X).(*ast.SelectorExpr)
		if !ok {
			break
		}

		selector = base
	}

	root, ok := unparen(selector.X).(*ast.Ident)

	return ok && variables[identifierObjectPosition(root)]
}

func checkEndpointCallSite(root string) error {
	path := filepath.Join(root, "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Source path is fixed to the registered cloud transport.
	source, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read HTTP endpoint call site: %w", err)
	}

	return checkEndpointSource(path, source)
}

func checkUnregisteredNetworkPrimitives(path string, fileSet *token.FileSet, file *ast.File) error {
	err := checkNetworkImports(path, fileSet, file)
	if err != nil {
		return err
	}

	return checkNetworkSelectors(path, fileSet, file)
}

func checkNetworkImports(path string, fileSet *token.FileSet, file *ast.File) error {
	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			continue
		}

		if isUnregisteredNetworkImport(importPath) {
			return sourceError(fileSet, path, importSpec.Pos(), "network package is not registered in the endpoint inventory")
		}
	}

	return nil
}

func isUnregisteredNetworkImport(importPath string) bool {
	return importPath == "net" || importPath == "crypto/tls" || strings.Contains(importPath, "websocket")
}

func checkNetworkSelectors(path string, fileSet *token.FileSet, file *ast.File) error {
	var violation error

	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil || violation != nil {
			return false
		}

		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		violation = networkSelectorViolation(path, fileSet, file, selector)

		return violation == nil
	})

	return violation
}

func networkSelectorViolation(path string, fileSet *token.FileSet, file *ast.File, selector *ast.SelectorExpr) error {
	if isNetworkMethod(selector.Sel.Name) {
		return sourceError(
			fileSet,
			path,
			selector.Pos(),
			"network send or dial primitive is outside the registered cloud transport",
		)
	}

	if importedPath(file, selector.X) == netHTTPImportPath && isHTTPRequestMethod(selector.Sel.Name) {
		return sourceError(
			fileSet,
			path,
			selector.Pos(),
			"HTTP constructor or convenience request is outside the registered cloud transport",
		)
	}

	return nil
}

func isNetworkMethod(name string) bool {
	switch name {
	case "Do", "RoundTrip", "Dial", "DialContext", "DialTimeout", "Listen", "ListenPacket", "ListenAndServe":
		return true
	default:
		return false
	}
}

func isHTTPRequestMethod(name string) bool {
	switch name {
	case getMethodName, "Head", "Post", "PostForm", "NewRequest", httpNewRequestMethodName:
		return true
	default:
		return false
	}
}

func checkEndpointSource(path string, source []byte) error {
	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, path, source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse HTTP endpoint call site: %w", err)
	}

	if !hasCloudTransportImports(file) {
		return inventoryError("cloud transport must resolve HTTP and generated model qualifiers to their exact imports")
	}

	return checkEndpointFunctions(file, endpointFunctions(file))
}

func hasCloudTransportImports(file *ast.File) bool {
	return hasImportPath(file, netHTTPImportPath) &&
		hasImportPath(file, dependencymodelsImportPath)
}

func endpointFunctions(file *ast.File) map[string]*ast.FuncDecl {
	functions := make(map[string]*ast.FuncDecl)

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Body != nil {
			functions[function.Name.Name] = function
		}
	}

	return functions
}

func checkEndpointFunctions(file *ast.File, functions map[string]*ast.FuncDecl) error {
	newRequest, ok := functions["newRequest"]

	err := checkRequestConstruction(file, newRequest, ok)
	if err != nil {
		return err
	}

	err = checkRequestMetadata(file, newRequest, functions["requestURL"])
	if err != nil {
		return err
	}

	err = checkRouteAndSendFlow(file, newRequest, functions)
	if err != nil {
		return err
	}

	return nil
}

func checkRequestConstruction(file *ast.File, newRequest *ast.FuncDecl, found bool) error {
	if !found || !hasRequestConstruction(file, newRequest) || !usesRequestURL(newRequest) {
		return inventoryError(
			"generated HTTP method and endpoint path must reach http.NewRequestWithContext (found=%t method=%t URL=%t)",
			found,
			hasRequestConstruction(file, newRequest),
			usesRequestURL(newRequest),
		)
	}

	return nil
}

func checkRequestMetadata(file *ast.File, newRequest, requestURL *ast.FuncDecl) error {
	if !hasGeneratedContentTypeHeader(file, newRequest) {
		return inventoryError("generated content-type header name and value must reach the constructed request")
	}

	if !hasGeneratedPathAssignment(file, requestURL) || !hasGeneratedTokenQuery(file, requestURL) {
		return inventoryError("generated endpoint path and query key must reach the request URL")
	}

	if !hasRouteReturned(requestURL) || !hasNoRouteMutations(requestURL) {
		return inventoryError("the generated route must be returned unchanged after schema-bound construction")
	}

	return nil
}

func checkRouteAndSendFlow(file *ast.File, newRequest *ast.FuncDecl, functions map[string]*ast.FuncDecl) error {
	if !hasInjectedHTTPDo(functions["Send"]) {
		return inventoryError("HTTP request must be sent through the injected HTTPDoer")
	}

	if !hasSendRequestFlow(functions["Send"]) {
		return inventoryError("only the generated endpoint request may reach the injected HTTPDoer")
	}

	if !hasNoExtraNetworkPrimitives(file, newRequest, functions["Send"]) {
		return inventoryError("HTTP requests must use the single schema-bound constructor and injected Do call")
	}

	return nil
}

func hasRequestConstruction(file *ast.File, function *ast.FuncDecl) bool {
	if function == nil {
		return false
	}

	count := 0
	foundMethod := false

	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		if !isHTTPConstructor(file, call, httpNewRequestMethodName) {
			return true
		}

		count++
		foundMethod = isGeneratedRequestConstruction(file, function, call)

		return true
	})

	return count == 1 && foundMethod
}

func isHTTPConstructor(file *ast.File, call *ast.CallExpr, name string) bool {
	selector, ok := unparen(call.Fun).(*ast.SelectorExpr)

	return ok && selector.Sel.Name == name && importedPath(file, selector.X) == netHTTPImportPath
}

func isGeneratedRequestConstruction(file *ast.File, function *ast.FuncDecl, call *ast.CallExpr) bool {
	if len(call.Args) != requestArgumentCount {
		return false
	}

	if !isGeneratedSelector(file, call.Args[1], "CloudRequestHTTPMethod") {
		return false
	}

	urlVariable, found := requestURLStringVariable(call.Args[2])

	return found && requestURLVariables(function)[identifierObjectPosition(urlVariable)]
}

func requestURLStringVariable(expression ast.Expr) (*ast.Ident, bool) {
	call, callFound := unparen(expression).(*ast.CallExpr)
	if !callFound {
		return nil, false
	}

	selector, selectorFound := unparen(call.Fun).(*ast.SelectorExpr)
	if !selectorFound || selector.Sel.Name != "String" {
		return nil, false
	}

	variable, variableFound := unparen(selector.X).(*ast.Ident)

	return variable, variableFound
}

func usesRequestURL(function *ast.FuncDecl) bool {
	if function == nil {
		return false
	}

	return len(requestURLVariables(function)) != 0
}

func requestURLVariables(function *ast.FuncDecl) map[token.Pos]bool {
	variables := make(map[token.Pos]bool)
	if function == nil || function.Type.Params == nil {
		return variables
	}

	parameters := functionParameters(function)
	baseURL := parameters["baseURL"]
	token := parameters["token"]

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}

		if result, matched := requestURLAssignmentResult(assignment, baseURL, token); matched {
			variables[identifierObjectPosition(result)] = true
		}

		return true
	})

	return variables
}

func functionParameters(function *ast.FuncDecl) map[string]token.Pos {
	parameters := make(map[string]token.Pos)
	if function.Type.Params == nil {
		return parameters
	}

	for _, field := range function.Type.Params.List {
		for _, name := range field.Names {
			parameters[name.Name] = identifierObjectPosition(name)
		}
	}

	return parameters
}

func requestURLAssignmentResult(assignment *ast.AssignStmt, baseURL, tokenPosition token.Pos) (*ast.Ident, bool) {
	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return nil, false
	}

	left, leftOK := unparen(assignment.Lhs[0]).(*ast.Ident)
	if !leftOK || left.Obj == nil {
		return nil, false
	}

	call, callOK := unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !callOK || !requestURLCallUsesParameters(call, baseURL, tokenPosition) {
		return nil, false
	}

	return left, true
}

func requestURLCallUsesParameters(call *ast.CallExpr, baseURL, tokenPosition token.Pos) bool {
	if baseURL == token.NoPos || tokenPosition == token.NoPos {
		return false
	}

	functionName, nameOK := unparen(call.Fun).(*ast.Ident)

	if !nameOK || functionName.Name != "requestURL" || len(call.Args) != 2 {
		return false
	}

	first, firstOK := unparen(call.Args[0]).(*ast.Ident)
	second, secondOK := unparen(call.Args[1]).(*ast.Ident)

	return firstOK && secondOK && identifierObjectPosition(first) == baseURL &&
		identifierObjectPosition(second) == tokenPosition
}

func hasGeneratedPathAssignment(file *ast.File, function *ast.FuncDecl) bool {
	resultObject := requestURLResultObject(function)
	if resultObject == token.NoPos {
		return false
	}

	return hasGeneratedPathInEmptyPathBranch(file, function, resultObject)
}

func hasGeneratedPathInEmptyPathBranch(file *ast.File, function *ast.FuncDecl, resultObject token.Pos) bool {
	found := false

	ast.Inspect(function.Body, func(node ast.Node) bool {
		conditional, ok := node.(*ast.IfStmt)
		if !ok || !isEmptyPathCondition(conditional.Cond, resultObject) {
			return true
		}

		found = found || hasPathAssignmentInBlock(file, conditional.Body, resultObject)

		return true
	})

	return found
}

func requestURLResultObject(function *ast.FuncDecl) token.Pos {
	if function == nil || function.Type.Params == nil {
		return token.NoPos
	}

	baseURLObject := functionParameters(function)["baseURL"]

	var resultObject token.Pos

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}

		if result, matched := dereferencedAssignmentResult(assignment, baseURLObject); matched {
			resultObject = identifierObjectPosition(result)
		}

		return true
	})

	return resultObject
}

func dereferencedAssignmentResult(assignment *ast.AssignStmt, baseURLObject token.Pos) (*ast.Ident, bool) {
	if baseURLObject == token.NoPos {
		return nil, false
	}

	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return nil, false
	}

	left, leftOK := unparen(assignment.Lhs[0]).(*ast.Ident)

	right, rightOK := unparen(assignment.Rhs[0]).(*ast.StarExpr)
	if !leftOK || !rightOK || left.Obj == nil {
		return nil, false
	}

	base, ok := unparen(right.X).(*ast.Ident)

	return left, ok && identifierObjectPosition(base) == baseURLObject
}

func hasRouteReturned(function *ast.FuncDecl) bool {
	resultObject := requestURLResultObject(function)
	if resultObject == token.NoPos {
		return false
	}

	returnCount := 0
	validReturn := false

	ast.Inspect(function.Body, func(node ast.Node) bool {
		statement, returnFound := node.(*ast.ReturnStmt)
		if !returnFound {
			return true
		}

		returnCount++

		if len(statement.Results) != 1 {
			return true
		}

		address, addressFound := unparen(statement.Results[0]).(*ast.UnaryExpr)
		if !addressFound || address.Op != token.AND {
			return true
		}

		identifier, identifierFound := unparen(address.X).(*ast.Ident)
		validReturn = validReturn || (identifierFound && identifierObjectPosition(identifier) == resultObject)

		return true
	})

	return returnCount == 1 && validReturn
}

func hasNoRouteMutations(function *ast.FuncDecl) bool {
	resultObject := requestURLResultObject(function)
	if resultObject == token.NoPos {
		return false
	}

	pathAssignments, queryAssignments, valid := routeMutationCounts(function, resultObject)

	return valid && pathAssignments == 1 && queryAssignments == 1
}

func routeMutationCounts(function *ast.FuncDecl, resultObject token.Pos) (int, int, bool) {
	pathAssignments := 0
	queryAssignments := 0
	valid := true

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for _, target := range assignment.Lhs {
			selector, ok := unparen(target).(*ast.SelectorExpr)
			if !ok || !isExpressionRootObject(selector.X, resultObject) {
				continue
			}

			switch selector.Sel.Name {
			case generatedRequestStatusField:
				pathAssignments++
			case generatedRequestQueryField:
				queryAssignments++
			case "RawPath", "Host", "Opaque":
				valid = false
			}
		}

		return true
	})

	return pathAssignments, queryAssignments, valid
}

func isExpressionRootObject(expression ast.Expr, expected token.Pos) bool {
	if expected == token.NoPos {
		return false
	}

	for {
		expression = unparen(expression)

		selector, ok := expression.(*ast.SelectorExpr)
		if !ok {
			break
		}

		expression = selector.X
	}

	identifier, ok := expression.(*ast.Ident)

	return ok && identifierObjectPosition(identifier) == expected
}

func isEmptyPathCondition(expression ast.Expr, resultObject token.Pos) bool {
	binary, ok := unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.EQL {
		return false
	}

	selector, selectorOK := unparen(binary.X).(*ast.SelectorExpr)

	value, valueOK := unparen(binary.Y).(*ast.BasicLit)
	if !selectorOK || !valueOK || selector.Sel.Name != generatedRequestStatusField ||
		value.Kind != token.STRING || value.Value != `""` {
		return false
	}

	base, baseOK := unparen(selector.X).(*ast.Ident)

	return baseOK && identifierObjectPosition(base) == resultObject
}

func hasPathAssignmentInBlock(file *ast.File, block *ast.BlockStmt, resultObject token.Pos) bool {
	found := false

	ast.Inspect(block, func(node ast.Node) bool {
		assignment, assignmentFound := node.(*ast.AssignStmt)
		if !assignmentFound || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}

		selector, selectorFound := unparen(assignment.Lhs[0]).(*ast.SelectorExpr)
		if !selectorFound || selector.Sel.Name != generatedRequestStatusField {
			return true
		}

		base, ok := unparen(selector.X).(*ast.Ident)
		if ok && identifierObjectPosition(base) == resultObject &&
			isGeneratedSelector(file, assignment.Rhs[0], "CloudRequestPath") {
			found = true
		}

		return true
	})

	return found
}

func hasGeneratedTokenQuery(file *ast.File, function *ast.FuncDecl) bool {
	if function == nil {
		return false
	}

	queryObjects, queryInitializers := collectTokenQueryObjects(file, function)

	if len(queryObjects) == 0 {
		return false
	}

	if queryWasReassigned(function, queryObjects, queryInitializers) {
		return false
	}

	if !querySetsGeneratedToken(file, function, queryObjects, functionParameters(function)["token"]) {
		return false
	}

	return rawQueryReceivesEncodedValues(function.Body, queryObjects, requestURLResultObject(function))
}

func collectTokenQueryObjects(file *ast.File, function *ast.FuncDecl) (map[token.Pos]bool, map[*ast.AssignStmt]bool) {
	queryObjects := make(map[token.Pos]bool)
	queryInitializers := make(map[*ast.AssignStmt]bool)

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}

		if left, matches := urlValuesMakeAssignment(file, assignment); matches {
			queryObjects[identifierObjectPosition(left)] = true
			queryInitializers[assignment] = true
		}

		return true
	})

	return queryObjects, queryInitializers
}

func urlValuesMakeAssignment(file *ast.File, assignment *ast.AssignStmt) (*ast.Ident, bool) {
	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return nil, false
	}

	left, leftOK := unparen(assignment.Lhs[0]).(*ast.Ident)

	call, callOK := unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !leftOK || !callOK || left.Obj == nil || len(call.Args) != 1 {
		return nil, false
	}

	functionName, nameOK := unparen(call.Fun).(*ast.Ident)

	return left, nameOK && functionName.Name == urlValuesMakeFunctionName && isURLValuesType(file, call.Args[0])
}

func queryWasReassigned(
	function *ast.FuncDecl,
	queryObjects map[token.Pos]bool,
	queryInitializers map[*ast.AssignStmt]bool,
) bool {
	reassigned := false

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || queryInitializers[assignment] {
			return true
		}

		for _, target := range assignment.Lhs {
			identifier, ok := unparen(target).(*ast.Ident)
			if ok && queryObjects[identifierObjectPosition(identifier)] {
				reassigned = true

				return false
			}
		}

		return true
	})

	return reassigned
}

func querySetsGeneratedToken(
	file *ast.File,
	function *ast.FuncDecl,
	queryObjects map[token.Pos]bool,
	tokenObject token.Pos,
) bool {
	setFound := false
	encodeFound := false

	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok {
			setUsesToken, hasEncode := queryCallFlags(file, call, queryObjects, tokenObject)
			setFound = setFound || setUsesToken
			encodeFound = encodeFound || hasEncode
		}

		return true
	})

	return setFound && encodeFound
}

func queryCallFlags(
	file *ast.File,
	call *ast.CallExpr,
	queryObjects map[token.Pos]bool,
	tokenObject token.Pos,
) (bool, bool) {
	selector, selectorFound := unparen(call.Fun).(*ast.SelectorExpr)
	if !selectorFound {
		return false, false
	}

	base, baseFound := unparen(selector.X).(*ast.Ident)
	if !baseFound || !queryObjects[identifierObjectPosition(base)] {
		return false, false
	}

	setUsesToken := selector.Sel.Name == querySetMethodName && len(call.Args) == 2 &&
		isGeneratedSelector(file, call.Args[0], "TokenQueryKey") && dereferencesObject(call.Args[1], tokenObject)
	hasEncode := selector.Sel.Name == queryEncodeMethodName && len(call.Args) == 0

	return setUsesToken, hasEncode
}

func dereferencesObject(expression ast.Expr, expected token.Pos) bool {
	star, starFound := unparen(expression).(*ast.StarExpr)
	if !starFound {
		return false
	}

	identifier, identifierFound := unparen(star.X).(*ast.Ident)

	return identifierFound && expected != token.NoPos && identifierObjectPosition(identifier) == expected
}

func rawQueryReceivesEncodedValues(block *ast.BlockStmt, queryObjects map[token.Pos]bool, resultObject token.Pos) bool {
	found, assignmentCount := encodedRawQueryAssignments(block, queryObjects, resultObject)

	return found && assignmentCount == 1
}

func encodedRawQueryAssignments(
	block *ast.BlockStmt,
	queryObjects map[token.Pos]bool,
	resultObject token.Pos,
) (bool, int) {
	found := false
	assignmentCount := 0

	ast.Inspect(block, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if ok {
			isRawQuery, isEncoded := encodedRawQueryAssignment(assignment, queryObjects, resultObject)
			if isRawQuery {
				assignmentCount++
			}

			found = found || isEncoded
		}

		return true
	})

	return found, assignmentCount
}

func encodedRawQueryAssignment(
	assignment *ast.AssignStmt,
	queryObjects map[token.Pos]bool,
	resultObject token.Pos,
) (bool, bool) {
	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false, false
	}

	left, leftFound := unparen(assignment.Lhs[0]).(*ast.SelectorExpr)
	if !leftFound || left.Sel.Name != generatedRequestQueryField {
		return false, false
	}

	if !isExpressionRootObject(left.X, resultObject) {
		return true, false
	}

	call, ok := unparen(assignment.Rhs[0]).(*ast.CallExpr)

	if !ok {
		return true, false
	}

	return true, rawQueryAssignmentUsesEncodedValues(left, call, queryObjects)
}

func rawQueryAssignmentUsesEncodedValues(
	left *ast.SelectorExpr,
	call *ast.CallExpr,
	queryObjects map[token.Pos]bool,
) bool {
	base, baseOK := unparen(left.X).(*ast.Ident)

	encode, encodeOK := unparen(call.Fun).(*ast.SelectorExpr)
	if !baseOK || !encodeOK || encode.Sel.Name != queryEncodeMethodName || len(call.Args) != 0 {
		return false
	}

	query, queryOK := unparen(encode.X).(*ast.Ident)

	return queryOK && identifierObjectPosition(base) != token.NoPos && queryObjects[identifierObjectPosition(query)]
}

func hasGeneratedContentTypeHeader(file *ast.File, function *ast.FuncDecl) bool {
	if function == nil {
		return false
	}

	requestObjects := constructedRequestObjects(file, function)

	return hasGeneratedHeaderSet(file, function, requestObjects)
}

func constructedRequestObjects(file *ast.File, function *ast.FuncDecl) map[token.Pos]bool {
	requestObjects := make(map[token.Pos]bool)

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, assignmentFound := node.(*ast.AssignStmt)
		if !assignmentFound || len(assignment.Rhs) != 1 {
			return true
		}

		call, callFound := unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !callFound || !isHTTPNewRequestCall(file, call) {
			return true
		}

		for _, target := range assignment.Lhs {
			if identifier, ok := unparen(target).(*ast.Ident); ok {
				if identifier.Obj != nil {
					requestObjects[identifierObjectPosition(identifier)] = true
				}
			}
		}

		return true
	})

	return requestObjects
}

func hasGeneratedHeaderSet(file *ast.File, function *ast.FuncDecl, requestObjects map[token.Pos]bool) bool {
	found := false

	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok {
			found = found || isGeneratedContentTypeSet(file, call, requestObjects)
		}

		return true
	})

	return found
}

func isGeneratedContentTypeSet(file *ast.File, call *ast.CallExpr, requestObjects map[token.Pos]bool) bool {
	if len(call.Args) != requestHeaderArgumentCount {
		return false
	}

	selector, selectorFound := unparen(call.Fun).(*ast.SelectorExpr)

	if !selectorFound || selector.Sel.Name != querySetMethodName {
		return false
	}

	header, headerFound := unparen(selector.X).(*ast.SelectorExpr)
	if !headerFound || header.Sel.Name != httpHeaderFieldName {
		return false
	}

	request, requestFound := unparen(header.X).(*ast.Ident)
	if !requestFound || !requestObjects[identifierObjectPosition(request)] {
		return false
	}

	return isGeneratedSelector(file, call.Args[0], "CloudRequestContentTypeHeader") &&
		isGeneratedSelector(file, call.Args[1], "CloudRequestContentType")
}

func isHTTPNewRequestCall(file *ast.File, call *ast.CallExpr) bool {
	selector, ok := unparen(call.Fun).(*ast.SelectorExpr)

	return ok && selector.Sel.Name == httpNewRequestMethodName && importedPath(file, selector.X) == netHTTPImportPath
}

func hasInjectedHTTPDo(function *ast.FuncDecl) bool {
	if function == nil {
		return false
	}

	parameterObject := functionParameters(function)["httpClient"]
	if parameterObject == token.NoPos {
		return false
	}

	return callsInjectedHTTPDo(function, parameterObject)
}

func callsInjectedHTTPDo(function *ast.FuncDecl, parameterObject token.Pos) bool {
	found := false

	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, callFound := node.(*ast.CallExpr)
		if !callFound {
			return true
		}

		selector, selectorFound := unparen(call.Fun).(*ast.SelectorExpr)
		if selectorFound && selector.Sel.Name == "Do" && len(call.Args) == 1 {
			identifier, identifierFound := unparen(selector.X).(*ast.Ident)
			found = found || (identifierFound && identifierObjectPosition(identifier) == parameterObject)
		}

		return true
	})

	return found
}

func hasSendRequestFlow(function *ast.FuncDecl) bool {
	if function == nil || function.Type.Params == nil {
		return false
	}

	requestObject, constructorCount := constructedSendRequestObject(function)

	return constructorCount == 1 && requestObject != token.NoPos && singleDoUsesRequest(function, requestObject)
}

func constructedSendRequestObject(function *ast.FuncDecl) (token.Pos, int) {
	parameters := functionParameters(function)

	var (
		requestObject    token.Pos
		constructorCount int
	)

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if ok {
			request, matchedConstructor := sendRequestAssignmentObject(assignment, parameters)
			if matchedConstructor {
				constructorCount++
				requestObject = request
			}
		}

		return true
	})

	return requestObject, constructorCount
}

func sendRequestAssignmentObject(assignment *ast.AssignStmt, parameters map[string]token.Pos) (token.Pos, bool) {
	if len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return token.NoPos, false
	}

	call, callFound := unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !callFound {
		return token.NoPos, false
	}

	name, nameFound := unparen(call.Fun).(*ast.Ident)
	if !nameFound || name.Name != "newRequest" {
		return token.NoPos, false
	}

	if !sendRequestArgumentsMatch(call, parameters) {
		return token.NoPos, true
	}

	identifier, ok := unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok {
		return token.NoPos, true
	}

	return identifierObjectPosition(identifier), true
}

func sendRequestArgumentsMatch(call *ast.CallExpr, parameters map[string]token.Pos) bool {
	if !hasRequiredSendRequestParameters(parameters) || len(call.Args) != cloudRequestArgumentCount {
		return false
	}

	return sendRequestArgumentsMatchParameters(call, parameters)
}

func hasRequiredSendRequestParameters(parameters map[string]token.Pos) bool {
	return parameters["ctx"] != token.NoPos && parameters["baseURL"] != token.NoPos &&
		parameters["cloudRequest"] != token.NoPos && parameters["token"] != token.NoPos
}

func sendRequestArgumentsMatchParameters(call *ast.CallExpr, parameters map[string]token.Pos) bool {
	contextArg, contextOK := unparen(call.Args[0]).(*ast.Ident)
	baseURLArg, baseURLOK := unparen(call.Args[1]).(*ast.Ident)
	requestArg, requestOK := unparen(call.Args[2]).(*ast.Ident)
	tokenArg, tokenOK := unparen(call.Args[3]).(*ast.Ident)

	return contextOK && baseURLOK && requestOK && tokenOK &&
		identifierObjectPosition(contextArg) == parameters["ctx"] &&
		identifierObjectPosition(baseURLArg) == parameters["baseURL"] &&
		identifierObjectPosition(requestArg) == parameters["cloudRequest"] &&
		identifierObjectPosition(tokenArg) == parameters["token"]
}

func singleDoUsesRequest(function *ast.FuncDecl, requestObject token.Pos) bool {
	var doCount int

	var doUsesConstructedRequest bool

	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, callFound := node.(*ast.CallExpr)
		if !callFound {
			return true
		}

		selector, ok := unparen(call.Fun).(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Do" {
			return true
		}

		doCount++

		if len(call.Args) == 1 {
			request, ok := unparen(call.Args[0]).(*ast.Ident)
			doUsesConstructedRequest = doUsesConstructedRequest || (ok && identifierObjectPosition(request) == requestObject)
		}

		return true
	})

	return doCount == 1 && doUsesConstructedRequest
}

func hasNoExtraNetworkPrimitives(file *ast.File, requestConstructor, sendFunction *ast.FuncDecl) bool {
	if hasUnregisteredNetworkImport(file) {
		return false
	}

	constructorCount, doCount, valid := networkSelectorCounts(file)

	return valid && constructorCount == 1 && doCount == 1 &&
		hasRequestConstruction(file, requestConstructor) && hasInjectedHTTPDo(sendFunction)
}

func hasUnregisteredNetworkImport(file *ast.File) bool {
	for _, importSpec := range file.Imports {
		path, err := strconv.Unquote(importSpec.Path.Value)
		if err == nil && isUnregisteredNetworkImport(path) {
			return true
		}
	}

	return false
}

func networkSelectorCounts(file *ast.File) (int, int, bool) {
	constructorSelectorCount := 0
	doSelectorCount := 0
	valid := true

	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if selector.Sel.Name == "Do" {
			doSelectorCount++
		}

		if importedPath(file, selector.X) != netHTTPImportPath {
			return true
		}

		switch selector.Sel.Name {
		case httpNewRequestMethodName:
			constructorSelectorCount++
		case "NewRequest", getMethodName, "Head", "Post", "PostForm", "Client":
			valid = false
		}

		return true
	})

	return constructorSelectorCount, doSelectorCount, valid
}

func isGeneratedSelector(file *ast.File, expression ast.Expr, name string) bool {
	selector, ok := unparen(expression).(*ast.SelectorExpr)

	return ok && selector.Sel.Name == name &&
		importedPath(file, selector.X) == dependencymodelsImportPath
}

func checkEndpointKeyConstruction(path string, fileSet *token.FileSet, file *ast.File) error {
	wireMaps, wireMapTypes := collectWireMapAliases(file)
	calledSelectors := calledWireMapSelectors(file)

	var violation error

	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil || violation != nil {
			return false
		}

		violation = endpointKeyNodeViolation(path, fileSet, node, file, wireMaps, wireMapTypes, calledSelectors)

		return true
	})

	return violation
}

func calledWireMapSelectors(file *ast.File) map[*ast.SelectorExpr]bool {
	called := make(map[*ast.SelectorExpr]bool)

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok {
				selector, selectorOK := unparen(call.Fun).(*ast.SelectorExpr)
				if selectorOK {
					called[selector] = true
				}
			}

			return true
		})
	}

	return called
}

func endpointKeyNodeViolation(
	path string,
	fileSet *token.FileSet,
	node ast.Node,
	file *ast.File,
	wireMaps, wireMapTypes map[string]bool,
	calledSelectors map[*ast.SelectorExpr]bool,
) error {
	err := endpointKeySyntaxViolation(path, fileSet, node, file, wireMaps, wireMapTypes, calledSelectors)
	if err != nil {
		return err
	}

	return wireMapEscape(path, fileSet, node, file, wireMaps, wireMapTypes)
}

func endpointKeySyntaxViolation(
	path string,
	fileSet *token.FileSet,
	node ast.Node,
	file *ast.File,
	wireMaps, wireMapTypes map[string]bool,
	calledSelectors map[*ast.SelectorExpr]bool,
) error {
	switch typed := node.(type) {
	case *ast.CallExpr:
		return endpointKeyCallViolation(path, fileSet, file, typed, wireMaps, wireMapTypes)
	case *ast.SelectorExpr:
		return endpointKeyMethodValueViolation(path, fileSet, file, typed, wireMaps, wireMapTypes, calledSelectors)
	case *ast.IndexExpr:
		if isWireMapSource(typed.X, file, wireMaps, wireMapTypes) && !isGeneratedModelConstant(file, typed.Index) {
			return generatedWireKeyError(fileSet, path, typed.Index.Pos())
		}
	case *ast.CompositeLit:
		return endpointKeyLiteralViolation(path, fileSet, file, typed, wireMapTypes)
	case *ast.AssignStmt:
		return endpointKeyAssignmentViolation(path, fileSet, file, typed, wireMaps, wireMapTypes)
	}

	return nil
}

func generatedWireKeyError(fileSet *token.FileSet, path string, position token.Pos) error {
	return sourceError(fileSet, path, position, "query and header keys must come from generated schema constants")
}

func endpointKeyCallViolation(
	path string,
	fileSet *token.FileSet,
	file *ast.File,
	call *ast.CallExpr,
	wireMaps, wireMapTypes map[string]bool,
) error {
	if isWireMapType(file, call.Fun, wireMapTypes) &&
		(len(call.Args) != 1 || !isWireMapSource(call.Args[0], file, wireMaps, wireMapTypes)) {
		return sourceError(fileSet, path, call.Pos(), "query and header conversions must preserve a schema-bound source")
	}

	selector, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || !isWireMapSource(selector.X, file, wireMaps, wireMapTypes) || !isSchemaKeyMethod(selector.Sel.Name) {
		return nil
	}

	if len(call.Args) == 0 || !isGeneratedModelConstant(file, call.Args[0]) {
		return generatedWireKeyError(fileSet, path, selector.Pos())
	}

	return nil
}

func endpointKeyMethodValueViolation(
	path string,
	fileSet *token.FileSet,
	file *ast.File,
	selector *ast.SelectorExpr,
	wireMaps, wireMapTypes map[string]bool,
	calledSelectors map[*ast.SelectorExpr]bool,
) error {
	if calledSelectors[selector] ||
		(!isSchemaKeyMethod(selector.Sel.Name) && selector.Sel.Name != queryEncodeMethodName) ||
		!isWireMapSource(selector.X, file, wireMaps, wireMapTypes) {
		return nil
	}

	return sourceError(fileSet, path, selector.Pos(), "query and header method values cannot escape key validation")
}

func endpointKeyAssignmentViolation(
	path string,
	fileSet *token.FileSet,
	file *ast.File,
	assignment *ast.AssignStmt,
	wireMaps, wireMapTypes map[string]bool,
) error {
	for index, target := range assignment.Lhs {
		identifier, ok := unparen(target).(*ast.Ident)
		if !ok || !wireMaps[identifierKey(identifier)] || index >= len(assignment.Rhs) ||
			isWireMapSource(assignment.Rhs[index], file, wireMaps, wireMapTypes) {
			continue
		}

		return sourceError(
			fileSet,
			path,
			assignment.Rhs[index].Pos(),
			"schema-bound query and header maps cannot be reassigned from an untrusted source",
		)
	}

	return nil
}

func endpointKeyLiteralViolation(
	path string,
	fileSet *token.FileSet,
	file *ast.File,
	literal *ast.CompositeLit,
	wireMapTypes map[string]bool,
) error {
	if !isWireMapType(file, literal.Type, wireMapTypes) {
		return nil
	}

	for _, rawElement := range literal.Elts {
		entry, ok := rawElement.(*ast.KeyValueExpr)
		if ok && !isGeneratedModelConstant(file, entry.Key) {
			return generatedWireKeyError(fileSet, path, entry.Key.Pos())
		}
	}

	return nil
}

func isSchemaKeyMethod(name string) bool {
	switch name {
	case querySetMethodName, "Add", "Del", getMethodName, "Has":
		return true
	default:
		return false
	}
}

func wireMapEscape(
	path string,
	fileSet *token.FileSet,
	node ast.Node,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) error {
	switch typed := node.(type) {
	case *ast.CallExpr:
		return wireMapCallEscape(path, fileSet, typed, file, aliases, wireMapTypes)
	case *ast.ReturnStmt:
		return wireMapReturnEscape(path, fileSet, typed, file, aliases, wireMapTypes)
	case *ast.CompositeLit:
		return wireMapCompositeEscape(path, fileSet, typed, file, aliases, wireMapTypes)
	case *ast.AssignStmt:
		return wireMapAssignmentEscape(path, fileSet, typed, file, aliases, wireMapTypes)
	case *ast.UnaryExpr:
		return wireMapUnaryEscape(path, fileSet, typed, file, aliases, wireMapTypes)
	}

	return nil
}

func wireMapCallEscape(
	path string,
	fileSet *token.FileSet,
	call *ast.CallExpr,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) error {
	selector, hasSelector := unparen(call.Fun).(*ast.SelectorExpr)
	if allowedWireMapCall(call, selector, hasSelector, file, aliases, wireMapTypes) {
		return nil
	}

	violation := wireMapBuiltinEscape(path, fileSet, call, file, aliases, wireMapTypes)
	if violation != nil {
		return violation
	}

	violation = wireMapCallArgumentEscape(path, fileSet, call, file, aliases, wireMapTypes)
	if violation != nil {
		return violation
	}

	if hasSelector && isWireMapSource(selector.X, file, aliases, wireMapTypes) {
		return sourceError(fileSet, path, selector.Pos(), "query and header method values cannot escape key validation")
	}

	return nil
}

func allowedWireMapCall(
	call *ast.CallExpr,
	selector *ast.SelectorExpr,
	hasSelector bool,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) bool {
	if hasSelector && isWireMapSource(selector.X, file, aliases, wireMapTypes) {
		return isSchemaKeyMethod(selector.Sel.Name) || (selector.Sel.Name == queryEncodeMethodName && len(call.Args) == 0)
	}

	return isWireMapType(file, call.Fun, wireMapTypes) && len(call.Args) == 1 &&
		isWireMapSource(call.Args[0], file, aliases, wireMapTypes)
}

func wireMapBuiltinEscape(
	path string,
	fileSet *token.FileSet,
	call *ast.CallExpr,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) error {
	builtin, ok := unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil
	}

	if builtin.Name == deleteName && len(call.Args) == 2 && isWireMapSource(call.Args[0], file, aliases, wireMapTypes) {
		if !isGeneratedModelConstant(file, call.Args[1]) {
			return generatedWireKeyError(fileSet, path, call.Args[1].Pos())
		}

		return nil
	}

	if builtin.Name == "clear" && len(call.Args) == 1 && isWireMapSource(call.Args[0], file, aliases, wireMapTypes) {
		return sourceError(fileSet, path, call.Pos(), "query and header maps cannot be cleared before a wire request")
	}

	return nil
}

func wireMapCallArgumentEscape(
	path string,
	fileSet *token.FileSet,
	call *ast.CallExpr,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) error {
	for _, argument := range call.Args {
		if containsWireMapSource(argument, file, aliases, wireMapTypes) {
			return sourceError(fileSet, path, argument.Pos(), "query and header maps cannot escape to an unverified helper")
		}
	}

	return nil
}

func wireMapReturnEscape(
	path string,
	fileSet *token.FileSet,
	statement *ast.ReturnStmt,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) error {
	for _, result := range statement.Results {
		if containsWireMapSource(result, file, aliases, wireMapTypes) {
			return sourceError(fileSet, path, result.Pos(), "query and header maps cannot escape their checked function")
		}
	}

	return nil
}

func wireMapCompositeEscape(
	path string,
	fileSet *token.FileSet,
	literal *ast.CompositeLit,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) error {
	if isWireMapType(file, literal.Type, wireMapTypes) {
		return nil
	}

	for _, element := range literal.Elts {
		if containsWireMapSource(element, file, aliases, wireMapTypes) {
			return sourceError(fileSet, path, element.Pos(), "query and header maps cannot be stored in aggregate values")
		}
	}

	return nil
}

func wireMapAssignmentEscape(
	path string,
	fileSet *token.FileSet,
	assignment *ast.AssignStmt,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) error {
	for index, target := range assignment.Lhs {
		if index >= len(assignment.Rhs) || !containsWireMapSource(assignment.Rhs[index], file, aliases, wireMapTypes) {
			continue
		}

		switch destination := unparen(target).(type) {
		case *ast.SelectorExpr, *ast.IndexExpr:
			return sourceError(fileSet, path, target.Pos(), "query and header maps cannot be stored in aggregate values")
		case *ast.Ident:
			if destination.Obj != nil && file.Scope.Lookup(destination.Name) == destination.Obj {
				return sourceError(fileSet, path, target.Pos(), "query and header maps cannot be stored in package state")
			}
		}
	}

	return nil
}

func wireMapUnaryEscape(
	path string,
	fileSet *token.FileSet,
	expression *ast.UnaryExpr,
	file *ast.File,
	aliases, wireMapTypes map[string]bool,
) error {
	if expression.Op == token.AND && containsWireMapSource(expression.X, file, aliases, wireMapTypes) {
		return sourceError(fileSet, path, expression.Pos(), "query and header maps cannot escape through an address")
	}

	return nil
}

func containsWireMapSource(node ast.Node, file *ast.File, aliases, wireMapTypes map[string]bool) bool {
	found := false

	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || found {
			return false
		}

		if call, ok := child.(*ast.CallExpr); ok {
			selector, selectorOK := unparen(call.Fun).(*ast.SelectorExpr)
			if selectorOK && isWireMapSource(selector.X, file, aliases, wireMapTypes) &&
				(isSchemaKeyMethod(selector.Sel.Name) || selector.Sel.Name == queryEncodeMethodName) {
				return false
			}
		}

		expression, ok := child.(ast.Expr)
		if ok && isWireMapSource(expression, file, aliases, wireMapTypes) {
			found = true

			return false
		}

		return true
	})

	return found
}

func collectWireMapAliases(file *ast.File) (map[string]bool, map[string]bool) {
	aliases := make(map[string]bool)
	types := collectWireMapTypes(file)

	for {
		before := len(aliases)

		ast.Inspect(file, func(node ast.Node) bool {
			collectWireMapAliasNode(file, node, aliases, types)

			return true
		})

		if before == len(aliases) {
			return aliases, types
		}
	}
}

func collectWireMapAliasNode(file *ast.File, node ast.Node, aliases, wireMapTypes map[string]bool) {
	switch typed := node.(type) {
	case *ast.ValueSpec:
		collectWireMapValueAliases(file, typed, aliases, wireMapTypes)
	case *ast.AssignStmt:
		collectWireMapAssignmentAliases(file, typed, aliases, wireMapTypes)
	}
}

func collectWireMapValueAliases(file *ast.File, value *ast.ValueSpec, aliases, wireMapTypes map[string]bool) {
	for index, name := range value.Names {
		if name.Obj == nil {
			continue
		}

		if isWireMapType(file, value.Type, wireMapTypes) ||
			(index < len(value.Values) && isWireMapSource(value.Values[index], file, aliases, wireMapTypes)) {
			aliases[identifierKey(name)] = true
		}
	}
}

func collectWireMapAssignmentAliases(
	file *ast.File,
	assignment *ast.AssignStmt,
	aliases, wireMapTypes map[string]bool,
) {
	for index, target := range assignment.Lhs {
		name, ok := unparen(target).(*ast.Ident)
		if ok && name.Obj != nil && index < len(assignment.Rhs) &&
			isWireMapSource(assignment.Rhs[index], file, aliases, wireMapTypes) {
			aliases[identifierKey(name)] = true
		}
	}
}

func collectWireMapTypes(file *ast.File) map[string]bool {
	types := make(map[string]bool)

	for {
		before := len(types)

		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}

			for _, rawSpec := range general.Specs {
				spec, ok := rawSpec.(*ast.TypeSpec)
				if ok && isWireMapType(file, spec.Type, types) {
					types[spec.Name.Name] = true
				}
			}
		}

		if before == len(types) {
			return types
		}
	}
}

func isURLValuesType(file *ast.File, expression ast.Expr) bool {
	selector, ok := unparen(expression).(*ast.SelectorExpr)

	return ok && selector.Sel.Name == "Values" && importedPath(file, selector.X) == netURLImportPath
}

func isWireMapType(file *ast.File, expression ast.Expr, wireMapTypes map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return isWireMapType(file, typed.X, wireMapTypes)
	case *ast.Ident:
		return wireMapTypes[typed.Name]
	case *ast.SelectorExpr:
		return isURLOrHTTPWireMapType(file, typed)
	case *ast.MapType:
		return isStringSliceMapType(typed)
	}

	return false
}

func isURLOrHTTPWireMapType(file *ast.File, selector *ast.SelectorExpr) bool {
	return (selector.Sel.Name == "Values" && importedPath(file, selector.X) == netURLImportPath) ||
		(selector.Sel.Name == httpHeaderFieldName && importedPath(file, selector.X) == netHTTPImportPath)
}

func isStringSliceMapType(expression *ast.MapType) bool {
	key, keyOK := unparen(expression.Key).(*ast.Ident)

	value, valueOK := unparen(expression.Value).(*ast.ArrayType)
	if !keyOK || key.Name != stringTypeName || !valueOK || value.Len != nil {
		return false
	}

	element, elementOK := unparen(value.Elt).(*ast.Ident)

	return elementOK && element.Name == stringTypeName
}

func isWireMapSource(expression ast.Expr, file *ast.File, aliases, wireMapTypes map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.SelectorExpr:
		return typed.Sel.Name == httpHeaderFieldName && hasImportPath(file, netHTTPImportPath)
	case *ast.Ident:
		return aliases[identifierKey(typed)]
	case *ast.CompositeLit:
		return isWireMapType(file, typed.Type, wireMapTypes)
	case *ast.CallExpr:
		return isWireMapCallSource(file, typed, aliases, wireMapTypes)
	case *ast.UnaryExpr, *ast.StarExpr:
		return isWireMapSource(expressionOperand(typed), file, aliases, wireMapTypes)
	}

	return false
}

func isWireMapCallSource(file *ast.File, call *ast.CallExpr, aliases, wireMapTypes map[string]bool) bool {
	selector, selectorOK := unparen(call.Fun).(*ast.SelectorExpr)
	if selectorOK && selector.Sel.Name == "ParseQuery" && importedPath(file, selector.X) == netURLImportPath {
		return true
	}

	if selectorOK && selector.Sel.Name == "Query" && hasImportPath(file, netURLImportPath) {
		return true
	}

	if isWireMapBuiltinConstruction(file, call, wireMapTypes) {
		return true
	}

	return isWireMapType(file, call.Fun, wireMapTypes) && len(call.Args) == 1 &&
		isWireMapSource(call.Args[0], file, aliases, wireMapTypes)
}

func isWireMapBuiltinConstruction(file *ast.File, call *ast.CallExpr, wireMapTypes map[string]bool) bool {
	function, ok := unparen(call.Fun).(*ast.Ident)

	return ok && (function.Name == "make" || function.Name == "new") && len(call.Args) > 0 &&
		isWireMapType(file, call.Args[0], wireMapTypes)
}

func hasImportPath(file *ast.File, expectedPath string) bool {
	for _, importSpec := range file.Imports {
		path, err := strconv.Unquote(importSpec.Path.Value)
		if err == nil && path == expectedPath {
			return true
		}
	}

	return false
}

func isGeneratedModelConstant(file *ast.File, expression ast.Expr) bool {
	selector, ok := unparen(expression).(*ast.SelectorExpr)

	return ok && selector.Sel.Name != "" && isGeneratedModelPackage(file, selector.X) &&
		importedPath(file, selector.X) == dependencymodelsImportPath
}
