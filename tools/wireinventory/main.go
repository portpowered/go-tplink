// Command wireinventory rejects handwritten provider-wire construction in SDK code.
package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	callerStateMutationMessage = "mutation of caller-open SetLightState state; " +
		"forward it through generated AdditionalProperties"
	callerStateHelperEscapeMessage = "caller-open SetLightState state escapes to a helper; " +
		"forward it through generated AdditionalProperties"
	callerStateDTOEscapeMessage = "caller-open SetLightState state escapes its generated DTO boundary"
	mapTypeMessage              = "map type in handwritten SDK source; use a generated wire DTO"
	mapAliasMessage             = "map alias in handwritten SDK source; use a generated wire DTO"
	mapVariableMessage          = "map variable in handwritten SDK source; use a generated wire DTO"
	mapPayloadMessage           = "map payload construction in handwritten SDK source; use a generated wire DTO"
	mapAllocationMessage        = "map allocation in handwritten SDK source; use a generated wire DTO"
	rawWireStringMessage        = "raw schema-defined wire string in handwritten SDK source; " +
		"use dependencymodels constants"
	handwrittenJSONModelMessage = "handwritten JSON model in production source; add a schema and generated DTO"
	anonymousJSONModelMessage   = "anonymous JSON object in production source; add a named schema component"
	generatedOutputMissing      = "registered generated output is missing"
	generatedOutputUntracked    = "generated output is not tracked by Git"
	generatedOutputUnknown      = "unregistered generated output"
	generatedSchemaMissing      = "schema component has no generated Go model"
	generatedTypeUndocumented   = "schema component is missing from the wire-model inventory"
)

var errWireInventoryViolation = errors.New("wire inventory violation")

func requiredGeneratedUses() []string {
	return []string{
		"AppType", "CloudErrorCodeAuthentication", "CloudErrorCodeOK", "CloudErrorCodeParameter",
		"CloudErrorCodeRateLimited", "CloudErrorCodeTokenExpired",
		"CloudRequestContentType", "CloudRequestContentTypeHeader", "CloudRequestHTTPMethod",
		"CloudRequestPath", "DefaultBaseURL", "CmdGetLightState", "CmdGetSysInfo",
		"CmdReboot", "CmdSetDevAlias", "CmdSetRelayState", "CmdTransitionLightState",
		"DeviceCapabilityDisabled", "DeviceCapabilityEnabled", "DeviceErrorOK", "DeviceErrorUnsupported",
		"DeviceStateOff", "DeviceStateOn", "DeviceTypeRangeExtenderPlug", "DeviceTypeSmartBulb", "DeviceTypeSmartPlug",
		"MethodGetDeviceList", "MethodLogin", "MethodPassthrough", "NamespaceLightingService",
		"NamespaceSystem", "TerminalUUID", "TokenQueryKey",
	}
}

func main() {
	err := checkRepository(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkRepository(root string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}

	root = absoluteRoot

	err = checkGeneratedOutputs(root)
	if err != nil {
		return err
	}

	wireValues, generatedMaps, err := readGeneratedInventory(root)
	if err != nil {
		return err
	}

	generatedTypes, generatedScalars, scalarMetadata, err := readRepositoryModelInventory(root)
	if err != nil {
		return err
	}

	err = checkGeneratedSchemaModels(root, generatedTypes)
	if err != nil {
		return err
	}

	err = checkEndpointInventory(root)
	if err != nil {
		return err
	}

	paths, err := productionGoFiles(root)
	if err != nil {
		return err
	}

	uses := make(map[string]bool)

	err = checkProductionFiles(paths, generatedMaps, wireValues, generatedTypes, generatedScalars,
		scalarMetadata, uses)
	if err != nil {
		return err
	}

	err = checkNetworkCallSites(root, paths)
	if err != nil {
		return err
	}

	return requireGeneratedUses(uses)
}

func readRepositoryModelInventory(root string) (
	map[string]bool,
	map[string]bool,
	generatedScalarMetadata,
	error,
) {
	generatedTypes, generatedScalars, err := readGeneratedWireModels(root)
	if err != nil {
		return nil, nil, generatedScalarMetadata{}, err
	}

	scalarMetadata, err := readGeneratedScalarMetadata(root)
	if err != nil {
		return nil, nil, generatedScalarMetadata{}, err
	}

	return generatedTypes, generatedScalars, scalarMetadata, nil
}

func checkNetworkCallSites(root string, paths []string) error {
	err := checkClientCloudSendLocations(root, paths)
	if err != nil {
		return err
	}

	return checkEndpointCallSite(root)
}

func readGeneratedInventory(root string) (map[string]bool, map[string]bool, error) {
	wirePath := filepath.Join(root, "pkg", "dependencymodels", "wire_constants.gen.go")

	wireValues, err := readGeneratedStringValues(wirePath)
	if err != nil {
		return nil, nil, err
	}

	generatedMaps := make(map[string]bool)

	for _, source := range generatedModelSources() {
		modelPath := filepath.Join(root, filepath.FromSlash(source.outputPath))

		modelMaps, err := readGeneratedMapTypes(modelPath)
		if err != nil {
			return nil, nil, err
		}

		for name := range modelMaps {
			generatedMaps[name] = true
		}
	}

	return wireValues, generatedMaps, nil
}

func productionGoFiles(root string) ([]string, error) {
	var paths []string

	directories := []string{
		filepath.Join(root, "pkg", "tplink"),
		filepath.Join(root, "pkg", "tplinkmodels"),
		filepath.Join(root, "pkg", "dependencies"),
		filepath.Join(root, "pkg", "dependencymodels"),
		filepath.Join(root, "pkg", "generatedwire"),
		filepath.Join(root, "cmd", "go-tplink"),
	}

	for _, directory := range directories {
		err := collectProductionGoFiles(directory, &paths)
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(paths)

	return paths, nil
}

func collectProductionGoFiles(directory string, paths *[]string) error {
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		*paths = append(*paths, path)

		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", directory, err)
	}

	return nil
}

func checkProductionFiles(
	paths []string,
	generatedMaps map[string]bool,
	wireValues map[string]bool,
	generatedTypes map[string]bool,
	generatedScalars map[string]bool,
	scalarMetadata generatedScalarMetadata,
	uses map[string]bool,
) error {
	for _, path := range paths {
		if isRegisteredGeneratedPath(path) {
			continue
		}

		err := checkProductionFile(path, generatedMaps, wireValues, generatedTypes, uses)
		if err != nil {
			return err
		}
	}

	return checkPackageWireProvenance(paths, generatedTypes, generatedScalars, scalarMetadata)
}

func checkProductionFile(path string, generatedMaps, wireValues, generatedTypes, uses map[string]bool) error {
	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	if !strings.HasSuffix(filepath.ToSlash(path), "pkg/dependencies/cloud/cloud.go") {
		err := checkUnregisteredNetworkPrimitives(path, fileSet, file)
		if err != nil {
			return err
		}
	}

	if strings.HasSuffix(filepath.ToSlash(path), "pkg/tplink/client.go") || hasCloudSendCall(file) {
		err := checkClientCloudSendCallSite(path, fileSet, file)
		if err != nil {
			return err
		}
	}

	return checkFile(path, fileSet, file, generatedMaps, wireValues, generatedTypes, uses)
}

func checkClientCloudSendLocations(root string, paths []string) error {
	expectedPath := filepath.Clean(filepath.Join(root, "pkg", "tplink", "client.go"))
	callCount := 0

	for _, path := range paths {
		//nolint:gosec // Source paths are restricted to the registered production package list.
		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read production file while locating cloud.Send: %w", err)
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if err != nil {
			return fmt.Errorf("parse production file while locating cloud.Send: %w", err)
		}

		if !hasCloudSendCall(file) {
			continue
		}

		if filepath.Clean(path) != expectedPath {
			return inventoryError("cloud.Send call sites must remain in pkg/tplink/client.go")
		}

		callCount++
	}

	if callCount != 1 {
		return inventoryError("the production inventory requires exactly one cloud.Send call in pkg/tplink/client.go")
	}

	return nil
}

func isRegisteredGeneratedPath(path string) bool {
	clean := filepath.Clean(path)
	for _, output := range registeredGeneratedOutputs() {
		if strings.HasSuffix(clean, filepath.Clean(filepath.FromSlash(output))) {
			return true
		}
	}

	return false
}

func requireGeneratedUses(uses map[string]bool) error {
	for _, name := range requiredGeneratedUses() {
		if !uses[name] {
			return inventoryError(
				"SDK production code does not use schema-derived constant dependencymodels.%s",
				name,
			)
		}
	}

	return nil
}

func checkFile(
	path string,
	fileSet *token.FileSet,
	file *ast.File,
	generatedMaps, wireValues, generatedTypes, uses map[string]bool,
) error {
	err := checkFunctionBodies(path, fileSet, file)
	if err != nil {
		return err
	}

	err = checkMapConstruction(path, fileSet, file, generatedMaps)
	if err != nil {
		return err
	}

	err = checkHandwrittenJSONModels(path, fileSet, file)
	if err != nil {
		return err
	}

	err = checkGeneratedScalarValues(path, fileSet, file, generatedTypes)
	if err != nil {
		return err
	}

	collectGeneratedUses(file, uses)

	err = checkRawWireStrings(path, fileSet, file, wireValues)
	if err != nil {
		return err
	}

	return checkEndpointKeyConstruction(path, fileSet, file)
}

func checkFunctionBodies(path string, fileSet *token.FileSet, file *ast.File) error {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		err := checkFunctionBody(path, fileSet, file, function)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkFunctionBody(path string, fileSet *token.FileSet, file *ast.File, function *ast.FuncDecl) error {
	requestAliases := make(map[string]bool)

	if function.Type.Params != nil {
		for _, field := range function.Type.Params.List {
			if !isCallerOpenRequestType(file, field.Type) {
				continue
			}

			for _, name := range field.Names {
				requestAliases[identifierKey(name)] = true
			}
		}
	}

	collectRequestAliases(function.Body, requestAliases)

	aliases := make(map[string]bool)
	collectOpenMapAliases(function.Body, aliases, requestAliases)
	stateVariables := generatedLightStateVariables(function.Body, file)

	var violation error

	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil || violation != nil {
			return false
		}

		violation = callerOpenViolation(path, fileSet, function, node, aliases, requestAliases, stateVariables)

		return violation == nil
	})

	return violation
}

func isCallerOpenRequestType(file *ast.File, expression ast.Expr) bool {
	expression = unparen(expression)
	if selector, ok := expression.(*ast.SelectorExpr); ok {
		return selector.Sel.Name == "SetLightStateRequest" &&
			importedPath(file, selector.X) == "github.com/portpowered/go-tplink/pkg/tplinkmodels"
	}

	identifier, ok := expression.(*ast.Ident)

	return ok && identifier.Name == "SetLightStateRequest"
}

func identifierKey(identifier *ast.Ident) string {
	if identifier == nil {
		return ""
	}

	if identifier.Obj != nil {
		return fmt.Sprintf("%p", identifier.Obj)
	}

	return identifier.Name
}

func callerOpenViolation(
	path string,
	fileSet *token.FileSet,
	function *ast.FuncDecl,
	node ast.Node,
	aliases, requestAliases, stateVariables map[string]bool,
) error {
	switch typed := node.(type) {
	case *ast.CallExpr:
		return callOpenInputViolation(path, fileSet, typed, aliases, requestAliases)
	case *ast.AssignStmt:
		return assignmentOpenInputViolation(path, fileSet, function, typed, aliases, requestAliases, stateVariables)
	case *ast.IncDecStmt:
		if isOpenMapIndex(typed.X, aliases, requestAliases) {
			return sourceError(fileSet, path, typed.Pos(), callerStateMutationMessage)
		}

		return nil
	case *ast.ReturnStmt:
		return resultOpenInputViolation(path, fileSet, typed.Results, aliases, requestAliases)
	case *ast.CompositeLit:
		return compositeOpenInputViolation(path, fileSet, typed, aliases, requestAliases)
	default:
		return nil
	}
}

func callOpenInputViolation(
	path string,
	fileSet *token.FileSet,
	call *ast.CallExpr,
	aliases, requestAliases map[string]bool,
) error {
	if isMapMutationCall(call, aliases, requestAliases) {
		return sourceError(
			fileSet,
			path,
			call.Pos(),
			callerStateMutationMessage,
		)
	}

	for _, argument := range call.Args {
		if isOpenMapSource(argument, aliases, requestAliases) {
			return sourceError(
				fileSet,
				path,
				argument.Pos(),
				callerStateHelperEscapeMessage,
			)
		}
	}

	return nil
}

func assignmentOpenInputViolation(
	path string,
	fileSet *token.FileSet,
	function *ast.FuncDecl,
	assignment *ast.AssignStmt,
	aliases, requestAliases, stateVariables map[string]bool,
) error {
	for index, target := range assignment.Lhs {
		if isOpenMapIndex(target, aliases, requestAliases) {
			return sourceError(
				fileSet,
				path,
				target.Pos(),
				callerStateMutationMessage,
			)
		}

		if !assignmentCarriesOpenInput(index, assignment.Rhs, aliases, requestAliases) {
			continue
		}

		if identifier, isAlias := unparen(target).(*ast.Ident); isAlias && isLocalIdentifier(function, identifier) {
			continue
		}

		if !isGeneratedLightStateSink(function, target, stateVariables) {
			return sourceError(
				fileSet,
				path,
				target.Pos(),
				callerStateDTOEscapeMessage,
			)
		}
	}

	return nil
}

func isLocalIdentifier(function *ast.FuncDecl, identifier *ast.Ident) bool {
	return function != nil &&
		identifier.Obj != nil &&
		identifier.Obj.Pos() >= function.Pos() &&
		identifier.Obj.Pos() <= function.End()
}

func assignmentCarriesOpenInput(index int, values []ast.Expr, aliases, requestAliases map[string]bool) bool {
	return index < len(values) && isOpenMapSource(values[index], aliases, requestAliases)
}

func resultOpenInputViolation(
	path string,
	fileSet *token.FileSet,
	results []ast.Expr,
	aliases, requestAliases map[string]bool,
) error {
	for _, result := range results {
		if isOpenMapSource(result, aliases, requestAliases) {
			return sourceError(fileSet, path, result.Pos(), callerStateDTOEscapeMessage)
		}
	}

	return nil
}

func compositeOpenInputViolation(
	path string,
	fileSet *token.FileSet,
	literal *ast.CompositeLit,
	aliases, requestAliases map[string]bool,
) error {
	for _, element := range literal.Elts {
		value := compositeValue(element)
		if isOpenMapSource(value, aliases, requestAliases) {
			return sourceError(fileSet, path, value.Pos(), callerStateDTOEscapeMessage)
		}
	}

	return nil
}

func compositeValue(element ast.Expr) ast.Expr {
	keyed, ok := element.(*ast.KeyValueExpr)
	if !ok {
		return element
	}

	return keyed.Value
}

func checkMapConstruction(path string, fileSet *token.FileSet, file *ast.File, generatedMaps map[string]bool) error {
	var violation error

	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil || violation != nil {
			return false
		}

		violation = mapConstructionViolation(path, fileSet, node, generatedMaps)

		return violation == nil
	})

	return violation
}

func mapConstructionViolation(path string, fileSet *token.FileSet, node ast.Node, generatedMaps map[string]bool) error {
	switch typed := node.(type) {
	case *ast.MapType:
		return sourceError(fileSet, path, typed.Pos(), mapTypeMessage)
	case *ast.TypeSpec:
		return typedMapViolation(path, fileSet, typed.Type, typed.Pos(), generatedMaps, mapAliasMessage)
	case *ast.ValueSpec:
		return typedMapViolation(path, fileSet, typed.Type, typed.Pos(), generatedMaps, mapVariableMessage)
	case *ast.CompositeLit:
		return typedMapViolation(path, fileSet, typed.Type, typed.Pos(), generatedMaps, mapPayloadMessage)
	case *ast.CallExpr:
		if isMapBuiltinConstruction(typed, generatedMaps) || isMapConversion(typed, generatedMaps) {
			return sourceError(fileSet, path, typed.Pos(), mapAllocationMessage)
		}
	}

	return nil
}

func typedMapViolation(
	path string,
	fileSet *token.FileSet,
	expression ast.Expr,
	position token.Pos,
	generatedMaps map[string]bool,
	message string,
) error {
	if expression != nil && isMapConstruction(expression, generatedMaps) {
		return sourceError(fileSet, path, position, message)
	}

	return nil
}

func collectGeneratedUses(file *ast.File, uses map[string]bool) {
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && importedPackageName(file, selector.X, "dependencymodels") {
			uses[selector.Sel.Name] = true
		}

		return true
	})
}

func checkRawWireStrings(path string, fileSet *token.FileSet, file *ast.File, wireValues map[string]bool) error {
	localCommandLiterals := localCLICommandLiterals(file)

	for value := range wireValues {
		var violation error

		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}

			decoded, err := strconv.Unquote(literal.Value)

			if err == nil && decoded == value && !localCommandLiterals[literal.Pos()] {
				violation = sourceError(fileSet, path, literal.Pos(), rawWireStringMessage)

				return false
			}

			return true
		})

		if violation != nil {
			return violation
		}
	}

	return nil
}

func localCLICommandLiterals(file *ast.File) map[token.Pos]bool {
	allowedNames := map[string]bool{
		"operationState": true, "operationOn": true, "operationOff": true,
		"operationReboot": true, "operationBrightness": true,
		"operationColorTemp": true, "operationColor": true,
	}

	allowed := make(map[token.Pos]bool)

	if file.Name.Name != "main" {
		return allowed
	}

	markCLICommandErrors(file, allowed)
	markCLICommandConstants(file, allowed, allowedNames)

	return allowed
}

func markCLICommandErrors(file *ast.File, allowed map[token.Pos]bool) {
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CaseClause:
			for _, expression := range typed.List {
				if isCLICommandLiteral(expression, "login") {
					allowed[expression.Pos()] = true
				}
			}
		case *ast.CallExpr:
			function, ok := unparen(typed.Fun).(*ast.Ident)
			if ok && function.Name == "safeOperationError" && len(typed.Args) > 0 &&
				isCLICommandLiteral(typed.Args[0], "login") {
				allowed[typed.Args[0].Pos()] = true
			}
		}

		return true
	})
}

func markCLICommandConstants(file *ast.File, allowed map[token.Pos]bool, allowedNames map[string]bool) {
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}

		for _, rawSpec := range general.Specs {
			spec, ok := rawSpec.(*ast.ValueSpec)
			if ok {
				markCLICommandValueSpec(spec, allowed, allowedNames)
			}
		}
	}
}

func markCLICommandValueSpec(spec *ast.ValueSpec, allowed map[token.Pos]bool, allowedNames map[string]bool) {
	for index, name := range spec.Names {
		if !allowedNames[name.Name] || index >= len(spec.Values) {
			continue
		}

		literal, ok := unparen(spec.Values[index]).(*ast.BasicLit)
		if ok && literal.Kind == token.STRING {
			allowed[literal.Pos()] = true
		}
	}
}

func isCLICommandLiteral(expression ast.Expr, expected string) bool {
	literal, ok := unparen(expression).(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return false
	}

	value, err := strconv.Unquote(literal.Value)

	return err == nil && value == expected
}

func collectRequestAliases(body *ast.BlockStmt, aliases map[string]bool) {
	for {
		aliasCount := len(aliases)

		ast.Inspect(body, func(node ast.Node) bool {
			collectRequestAliasesFromNode(node, aliases)

			return true
		})

		if len(aliases) == aliasCount {
			return
		}
	}
}

func collectRequestAliasesFromNode(node ast.Node, aliases map[string]bool) {
	switch typed := node.(type) {
	case *ast.ValueSpec:
		collectRequestAliasesFromIdentifiers(typed.Names, typed.Values, aliases)
	case *ast.AssignStmt:
		collectRequestAliasesFromValues(typed.Lhs, typed.Rhs, aliases)
	}
}

func collectRequestAliasesFromIdentifiers(names []*ast.Ident, values []ast.Expr, aliases map[string]bool) {
	for index, source := range values {
		if index >= len(names) || !isRequestSource(source, aliases) {
			continue
		}

		aliases[identifierKey(names[index])] = true
	}
}

func collectRequestAliasesFromValues(names []ast.Expr, values []ast.Expr, aliases map[string]bool) {
	for index, source := range values {
		if index >= len(names) || !isRequestSource(source, aliases) {
			continue
		}

		target, isTarget := unparen(names[index]).(*ast.Ident)
		if isTarget {
			aliases[identifierKey(target)] = true
		}
	}
}

func isRequestSource(expression ast.Expr, aliases map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return aliases[identifierKey(typed)]
	case *ast.StarExpr, *ast.UnaryExpr:
		return isRequestSource(expressionOperand(typed), aliases)
	}

	return false
}

func expressionOperand(expression ast.Expr) ast.Expr {
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return typed.X
	case *ast.UnaryExpr:
		return typed.X
	default:
		return expression
	}
}

func generatedLightStateVariables(body *ast.BlockStmt, file *ast.File) map[string]bool {
	variables := make(map[string]bool)

	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.ValueSpec:
			if isGeneratedLightStateType(file, typed.Type) {
				for _, name := range typed.Names {
					variables[identifierKey(name)] = true
				}
			}
		case *ast.AssignStmt:
			for index, value := range typed.Rhs {
				if index >= len(typed.Lhs) || !isGeneratedLightStateLiteral(file, value) {
					continue
				}

				if name, ok := unparen(typed.Lhs[index]).(*ast.Ident); ok {
					variables[identifierKey(name)] = true
				}
			}
		}

		return true
	})

	return variables
}

func isGeneratedLightStateSink(function *ast.FuncDecl, target ast.Expr, variables map[string]bool) bool {
	if function.Name.Name != "SetLightState" {
		return false
	}

	selector, isSelector := unparen(target).(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "AdditionalProperties" {
		return false
	}

	base, isIdentifier := unparen(selector.X).(*ast.Ident)

	return isIdentifier && variables[identifierKey(base)]
}

func isGeneratedLightStateType(file *ast.File, expression ast.Expr) bool {
	selector, ok := unparen(expression).(*ast.SelectorExpr)

	return ok && selector.Sel.Name == "LightTransitionState" && importedPackageName(file, selector.X, "dependencymodels")
}

func isGeneratedLightStateLiteral(file *ast.File, expression ast.Expr) bool {
	literal, ok := unparen(expression).(*ast.CompositeLit)

	return ok && isGeneratedLightStateType(file, literal.Type)
}

func collectOpenMapAliases(body *ast.BlockStmt, aliases, requestAliases map[string]bool) {
	for {
		aliasCount := len(aliases)

		ast.Inspect(body, func(node ast.Node) bool {
			collectOpenMapAliasesFromNode(node, aliases, requestAliases)

			return true
		})

		if len(aliases) == aliasCount {
			return
		}
	}
}

func collectOpenMapAliasesFromNode(node ast.Node, aliases, requestAliases map[string]bool) {
	switch typed := node.(type) {
	case *ast.ValueSpec:
		collectOpenMapAliasesFromIdentifiers(typed.Names, typed.Values, aliases, requestAliases)
	case *ast.AssignStmt:
		collectOpenMapAliasesFromValues(typed.Lhs, typed.Rhs, aliases, requestAliases)
	}
}

func collectOpenMapAliasesFromIdentifiers(
	names []*ast.Ident,
	values []ast.Expr,
	aliases, requestAliases map[string]bool,
) {
	for index, source := range values {
		if index < len(names) && isOpenMapSource(source, aliases, requestAliases) {
			aliases[identifierKey(names[index])] = true
		}
	}
}

func collectOpenMapAliasesFromValues(names []ast.Expr, values []ast.Expr, aliases, requestAliases map[string]bool) {
	for index, source := range values {
		if index >= len(names) || !isOpenMapSource(source, aliases, requestAliases) {
			continue
		}

		if identifier, ok := unparen(names[index]).(*ast.Ident); ok {
			aliases[identifierKey(identifier)] = true
		}
	}
}

func isOpenMapSource(expression ast.Expr, aliases, requestAliases map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return aliases[identifierKey(typed)]
	case *ast.SelectorExpr:
		return typed.Sel.Name == "State" && isRequestIdentifier(unparen(typed.X), requestAliases)
	case *ast.IndexExpr, *ast.IndexListExpr, *ast.TypeAssertExpr, *ast.StarExpr, *ast.UnaryExpr:
		return isOpenMapOperand(typed, aliases, requestAliases)
	case *ast.CallExpr:
		return isOpenMapCall(typed, aliases, requestAliases)
	}

	return false
}

func isOpenMapOperand(expression ast.Expr, aliases, requestAliases map[string]bool) bool {
	switch typed := expression.(type) {
	case *ast.IndexExpr:
		return isOpenMapSource(typed.X, aliases, requestAliases)
	case *ast.IndexListExpr:
		return isOpenMapSource(typed.X, aliases, requestAliases)
	case *ast.TypeAssertExpr:
		return isOpenMapSource(typed.X, aliases, requestAliases)
	case *ast.StarExpr:
		return isOpenMapSource(typed.X, aliases, requestAliases)
	case *ast.UnaryExpr:
		return isOpenMapSource(typed.X, aliases, requestAliases)
	default:
		return false
	}
}

func isOpenMapCall(call *ast.CallExpr, aliases, requestAliases map[string]bool) bool {
	// Type conversions and helper calls both have CallExpr nodes. Reject either
	// form when it wraps caller-open input.
	for _, argument := range call.Args {
		if isOpenMapSource(argument, aliases, requestAliases) {
			return true
		}
	}

	return false
}

func isRequestIdentifier(expression ast.Expr, requestAliases map[string]bool) bool {
	identifier, ok := expression.(*ast.Ident)

	return ok && requestAliases[identifierKey(identifier)]
}

func isOpenMapIndex(expression ast.Expr, aliases, requestAliases map[string]bool) bool {
	expression = unparen(expression)
	index, ok := expression.(*ast.IndexExpr)

	return ok && isOpenMapSource(index.X, aliases, requestAliases)
}

func isMapConstruction(expression ast.Expr, generatedMaps map[string]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.MapType:
		return true
	case *ast.Ident:
		return generatedMaps[typed.Name]
	case *ast.SelectorExpr:
		return generatedMaps[typed.Sel.Name]
	}

	return false
}

func isMapBuiltinConstruction(call *ast.CallExpr, generatedMaps map[string]bool) bool {
	function := unparen(call.Fun)

	identifier, ok := function.(*ast.Ident)
	if !ok || (identifier.Name != urlValuesMakeFunctionName && identifier.Name != "new") || len(call.Args) == 0 {
		return false
	}

	return isMapConstruction(call.Args[0], generatedMaps)
}

func isMapConversion(call *ast.CallExpr, generatedMaps map[string]bool) bool {
	return len(call.Args) == 1 && isMapConstruction(call.Fun, generatedMaps)
}

func isMapMutationCall(call *ast.CallExpr, aliases, requestAliases map[string]bool) bool {
	function, ok := unparen(call.Fun).(*ast.Ident)
	if !ok || (function.Name != deleteName && function.Name != "clear") || len(call.Args) == 0 {
		return false
	}

	return isOpenMapSource(call.Args[0], aliases, requestAliases)
}

func unparen(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}

		expression = parenthesized.X
	}
}

func importedPackageName(file *ast.File, expression ast.Expr, packageName string) bool {
	identifier, ok := unparen(expression).(*ast.Ident)
	if !ok || identifier.Name != packageName {
		return false
	}

	for _, importSpec := range file.Imports {
		path, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil || path != dependencymodelsImportPath {
			continue
		}

		if importSpec.Name == nil {
			return packageName == "dependencymodels" && packageQualifierIsUnshadowed(file, identifier)
		}

		return importSpec.Name.Name == identifier.Name && packageQualifierIsUnshadowed(file, identifier)
	}

	return false
}

func packageQualifierIsUnshadowed(file *ast.File, identifier *ast.Ident) bool {
	return identifier.Obj == nil || file.Scope.Lookup(identifier.Name) == identifier.Obj
}

func readGeneratedStringValues(path string) (map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse generated constants %s: %w", path, err)
	}

	values := make(map[string]bool)
	collectGeneratedStringValues(file, values)

	return values, nil
}

func collectGeneratedStringValues(file *ast.File, values map[string]bool) {
	for _, declaration := range file.Decls {
		genDecl, ok := declaration.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		collectConstantDeclaration(genDecl, values)
	}
}

func collectConstantDeclaration(declaration *ast.GenDecl, values map[string]bool) {
	for _, rawSpec := range declaration.Specs {
		spec, ok := rawSpec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		collectStringValueLiterals(spec.Values, values)
	}
}

func collectStringValueLiterals(rawValues []ast.Expr, values map[string]bool) {
	for _, rawValue := range rawValues {
		literal, ok := rawValue.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}

		value, err := strconv.Unquote(literal.Value)
		if err == nil {
			values[value] = true
		}
	}
}

func readGeneratedMapTypes(path string) (map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse generated models %s: %w", path, err)
	}

	types := make(map[string]bool)

	for _, declaration := range file.Decls {
		genDecl, ok := declaration.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, rawSpec := range genDecl.Specs {
			spec, ok := rawSpec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if isMapConstruction(spec.Type, types) {
				types[spec.Name.Name] = true
			}
		}
	}

	return types, nil
}

func sourceError(fileSet *token.FileSet, path string, position token.Pos, message string) error {
	location := fileSet.Position(position)

	return inventoryError("%s:%d:%d: %s", path, location.Line, location.Column, message)
}

func inventoryError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errWireInventoryViolation, fmt.Sprintf(format, args...))
}
