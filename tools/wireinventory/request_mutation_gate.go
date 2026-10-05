package main

import (
	"go/ast"
	"go/token"
)

const requestMutationMessage = "constructed HTTP request must reach the injected Do without mutation or escape"

type requestMutationAliases struct {
	requests       map[token.Pos]bool
	urls           map[token.Pos]bool
	headers        map[token.Pos]bool
	bodies         map[token.Pos]bool
	buffers        map[token.Pos]bool
	bufferLiveFrom map[token.Pos]token.Pos
}

func hasNoSendRequestMutations(file *ast.File, function *ast.FuncDecl) bool {
	if function == nil || function.Body == nil {
		return false
	}

	requestObject, constructorCount := constructedSendRequestObject(function)
	if constructorCount != 1 || requestObject == token.NoPos {
		return false
	}

	return hasNoRequestMutations(file, function, map[token.Pos]bool{requestObject: true}, false)
}

func hasNoNewRequestMutations(file *ast.File, function *ast.FuncDecl) bool {
	requestObjects := constructedHTTPRequestObjects(file, function)
	if function == nil || function.Body == nil || len(requestObjects) != 1 {
		return false
	}

	return hasNoRequestMutations(file, function, requestObjects, true)
}

func constructedHTTPRequestObjects(file *ast.File, function *ast.FuncDecl) map[token.Pos]bool {
	requestObjects := make(map[token.Pos]bool)
	if function == nil || function.Body == nil {
		return requestObjects
	}

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, isAssignment := node.(*ast.AssignStmt)
		if !isAssignment || len(assignment.Rhs) != 1 || len(assignment.Lhs) < 1 {
			return true
		}

		call, isCall := unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !isCall || !isHTTPNewRequestCall(file, call) {
			return true
		}

		request, isIdentifier := unparen(assignment.Lhs[0]).(*ast.Ident)
		if isIdentifier && request.Obj != nil {
			requestObjects[identifierObjectPosition(request)] = true
		}

		return true
	})

	return requestObjects
}

func hasNoRequestMutations(
	file *ast.File,
	function *ast.FuncDecl,
	requestObjects map[token.Pos]bool,
	allowContentTypeSetter bool,
) bool {
	aliases := collectRequestMutationAliases(file, function, requestObjects)
	parameters := functionParameters(function)
	allowedCalls, allowedSelectors := allowedConstructedRequestCalls(
		file, function, requestObjects, allowContentTypeSetter,
	)

	var violation bool

	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil || violation {
			return false
		}

		violation = requestMutationNodeViolation(
			file, function, node, parameters, requestObjects, aliases, allowedCalls, allowedSelectors,
			allowContentTypeSetter,
		)

		return !violation
	})

	return !violation
}

func requestMutationNodeViolation(
	file *ast.File,
	function *ast.FuncDecl,
	node ast.Node,
	parameters map[string]token.Pos,
	requestObjects map[token.Pos]bool,
	aliases requestMutationAliases,
	allowedCalls map[*ast.CallExpr]bool,
	allowedSelectors map[*ast.SelectorExpr]bool,
	allowContentTypeSetter bool,
) bool {
	switch typed := node.(type) {
	case *ast.AssignStmt:
		return requestAssignmentMutates(file, typed, parameters, requestObjects, aliases)
	case *ast.IncDecStmt:
		return isRequestMutationTargetAt(typed.X, aliases, typed.Pos())
	case *ast.CallExpr:
		return requestCallEscapesWithAllowances(typed, function, aliases, allowedCalls)
	case *ast.SelectorExpr:
		return !allowedSelectors[typed] && requestMethodValueEscapes(typed, aliases)
	default:
		return requestMutationValueNodeViolation(file, function, node, aliases, allowContentTypeSetter)
	}
}

func requestMutationValueNodeViolation(
	file *ast.File,
	function *ast.FuncDecl,
	node ast.Node,
	aliases requestMutationAliases,
	allowContentTypeSetter bool,
) bool {
	switch typed := node.(type) {
	case *ast.CompositeLit:
		return requestValueEscapesAt(typed, aliases, typed.Pos())
	case *ast.ReturnStmt:
		return requestReturnEscapes(function, typed, aliases, allowContentTypeSetter)
	case *ast.UnaryExpr:
		return typed.Op == token.AND && requestValueEscapesAt(typed.X, aliases, typed.Pos())
	case *ast.ValueSpec:
		return requestPackageValueEscapes(file, typed, aliases)
	default:
		return false
	}
}

func collectRequestMutationAliases(
	file *ast.File,
	function *ast.FuncDecl,
	requestObjects map[token.Pos]bool,
) requestMutationAliases {
	aliases := requestMutationAliases{
		requests:       clonePositionSet(requestObjects),
		urls:           make(map[token.Pos]bool),
		headers:        make(map[token.Pos]bool),
		bodies:         make(map[token.Pos]bool),
		buffers:        make(map[token.Pos]bool),
		bufferLiveFrom: make(map[token.Pos]token.Pos),
	}
	collectConstructedRequestBodyBuffers(file, function, requestObjects, aliases)

	for {
		before := len(aliases.requests) + len(aliases.urls) + len(aliases.headers) +
			len(aliases.bodies) + len(aliases.buffers)

		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.ValueSpec:
				collectRequestValueAliases(file, typed, aliases)
			case *ast.AssignStmt:
				collectRequestAssignmentAliases(file, typed, aliases)
			}

			return true
		})

		after := len(aliases.requests) + len(aliases.urls) + len(aliases.headers) +
			len(aliases.bodies) + len(aliases.buffers)
		if before == after {
			return aliases
		}
	}
}

func collectRequestValueAliases(file *ast.File, value *ast.ValueSpec, aliases requestMutationAliases) {
	for index, name := range value.Names {
		if name.Obj == nil || index >= len(value.Values) {
			continue
		}

		addRequestAlias(identifierObjectPosition(name), value.Values[index], file, aliases)
	}
}

func collectRequestAssignmentAliases(file *ast.File, assignment *ast.AssignStmt, aliases requestMutationAliases) {
	for index, target := range assignment.Lhs {
		name, ok := unparen(target).(*ast.Ident)
		if !ok || name.Obj == nil || index >= len(assignment.Rhs) {
			continue
		}

		addRequestAlias(identifierObjectPosition(name), assignment.Rhs[index], file, aliases)
	}
}

func addRequestAlias(position token.Pos, expression ast.Expr, file *ast.File, aliases requestMutationAliases) {
	if position == token.NoPos {
		return
	}

	switch {
	case requestAliasSource(expression, aliases.requests):
		aliases.requests[position] = true
	case requestURLAliasSource(expression, aliases):
		aliases.urls[position] = true
	case requestHeaderAliasSource(expression, file, aliases):
		aliases.headers[position] = true
	case requestBodyAliasSource(expression, aliases):
		aliases.bodies[position] = true
	case requestBufferAliasSource(expression, aliases):
		if liveFrom, found := requestBufferLiveFrom(expression, aliases); found {
			aliases.buffers[position] = true
			aliases.bufferLiveFrom[position] = liveFrom
		}
	}
}

func collectConstructedRequestBodyBuffers(
	file *ast.File,
	function *ast.FuncDecl,
	requestObjects map[token.Pos]bool,
	aliases requestMutationAliases,
) {
	if function == nil || function.Body == nil {
		return
	}

	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, isAssignment := node.(*ast.AssignStmt)
		if isAssignment {
			collectConstructedRequestBodyBufferAssignment(file, function, requestObjects, aliases, assignment)
		}

		return true
	})
}

func collectConstructedRequestBodyBufferAssignment(
	file *ast.File,
	function *ast.FuncDecl,
	requestObjects map[token.Pos]bool,
	aliases requestMutationAliases,
	assignment *ast.AssignStmt,
) {
	if len(assignment.Rhs) != 1 || len(assignment.Lhs) < 1 {
		return
	}

	request, isIdentifier := unparen(assignment.Lhs[0]).(*ast.Ident)
	if !isIdentifier || !requestObjects[identifierObjectPosition(request)] {
		return
	}

	constructor, isCall := unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !isCall || !isHTTPNewRequestCall(file, constructor) {
		return
	}

	body, found := httpRequestBodyArgument(constructor)
	if !found {
		return
	}

	registerRequestBodyBufferAliases(file, function, aliases, constructor, body)
}

func registerRequestBodyBufferAliases(
	file *ast.File,
	function *ast.FuncDecl,
	aliases requestMutationAliases,
	constructor *ast.CallExpr,
	body ast.Expr,
) {
	for position := range requestBodyBufferIdentifiers(file, function, body, make(map[token.Pos]bool)) {
		aliases.buffers[position] = true
		if existing, found := aliases.bufferLiveFrom[position]; !found || constructor.End() < existing {
			aliases.bufferLiveFrom[position] = constructor.End()
		}
	}
}

func httpRequestBodyArgument(constructor *ast.CallExpr) (ast.Expr, bool) {
	switch len(constructor.Args) {
	case requestArgumentCount:
		return constructor.Args[requestBodyArgumentIndex], true
	case requestArgumentCountWithoutContext:
		return constructor.Args[2], true
	default:
		return nil, false
	}
}

const requestBodyArgumentIndex = 3
const requestArgumentCountWithoutContext = 3

func requestBodyBufferIdentifiers(
	file *ast.File,
	function *ast.FuncDecl,
	expression ast.Expr,
	visiting map[token.Pos]bool,
) map[token.Pos]bool {
	identifiers := make(map[token.Pos]bool)
	expression = unparen(expression)

	switch typed := expression.(type) {
	case *ast.CallExpr:
		return requestBodyCallBufferIdentifiers(file, function, typed, visiting)
	case *ast.Ident:
		return requestBodyIdentifierBufferIdentifiers(file, function, typed, visiting)
	case *ast.UnaryExpr, *ast.StarExpr:
		for nested := range requestBodyBufferIdentifiers(file, function, expressionOperand(typed), visiting) {
			identifiers[nested] = true
		}
	}

	return identifiers
}

func requestBodyCallBufferIdentifiers(
	file *ast.File,
	function *ast.FuncDecl,
	call *ast.CallExpr,
	visiting map[token.Pos]bool,
) map[token.Pos]bool {
	identifiers := make(map[token.Pos]bool)
	if !isBytesBodyReader(file, call) || len(call.Args) != 1 {
		return identifiers
	}

	ast.Inspect(call.Args[0], func(node ast.Node) bool {
		identifier, isIdentifier := node.(*ast.Ident)
		if !isIdentifier || identifier.Obj == nil {
			return true
		}

		for position := range requestBodyIdentifierBufferIdentifiers(file, function, identifier, visiting) {
			identifiers[position] = true
		}

		return true
	})

	return identifiers
}

func requestBodyIdentifierBufferIdentifiers(
	file *ast.File,
	function *ast.FuncDecl,
	identifier *ast.Ident,
	visiting map[token.Pos]bool,
) map[token.Pos]bool {
	identifiers := make(map[token.Pos]bool)

	position := identifierObjectPosition(identifier)
	if position == token.NoPos || visiting[position] {
		return identifiers
	}

	visiting[position] = true
	defer delete(visiting, position)

	identifiers[position] = true

	for _, source := range requestIdentifierAssignments(function, position) {
		for nested := range requestBodyBufferIdentifiers(file, function, source, visiting) {
			identifiers[nested] = true
		}
	}

	return identifiers
}

func requestIdentifierAssignments(function *ast.FuncDecl, position token.Pos) []ast.Expr {
	var values []ast.Expr
	if function == nil || function.Body == nil {
		return values
	}

	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.ValueSpec:
			values = append(values, requestValueSpecAssignments(typed, position)...)
		case *ast.AssignStmt:
			values = append(values, requestAssignStmtAssignments(typed, position)...)
		}

		return true
	})

	return values
}

func requestValueSpecAssignments(value *ast.ValueSpec, position token.Pos) []ast.Expr {
	var values []ast.Expr

	for index, name := range value.Names {
		if identifierObjectPosition(name) == position && index < len(value.Values) {
			values = append(values, value.Values[index])
		}
	}

	return values
}

func requestAssignStmtAssignments(assignment *ast.AssignStmt, position token.Pos) []ast.Expr {
	var values []ast.Expr

	for index, target := range assignment.Lhs {
		name, isIdentifier := unparen(target).(*ast.Ident)
		if isIdentifier && identifierObjectPosition(name) == position && index < len(assignment.Rhs) {
			values = append(values, assignment.Rhs[index])
		}
	}

	return values
}

func isBytesBodyReader(file *ast.File, call *ast.CallExpr) bool {
	selector, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || importedPath(file, selector.X) != "bytes" {
		return false
	}

	return selector.Sel.Name == "NewReader" || selector.Sel.Name == "NewBuffer"
}

func requestBufferLiveFrom(expression ast.Expr, aliases requestMutationAliases) (token.Pos, bool) {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		position := identifierObjectPosition(typed)
		liveFrom, found := aliases.bufferLiveFrom[position]

		return liveFrom, found
	case *ast.SliceExpr:
		return requestBufferLiveFrom(typed.X, aliases)
	case *ast.UnaryExpr, *ast.StarExpr:
		return requestBufferLiveFrom(expressionOperand(typed), aliases)
	}

	return token.NoPos, false
}

func requestBufferAliasSource(expression ast.Expr, aliases requestMutationAliases) bool {
	_, found := requestBufferLiveFrom(expression, aliases)

	return found
}

func requestAliasSource(expression ast.Expr, requests map[token.Pos]bool) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return requests[identifierObjectPosition(typed)]
	case *ast.UnaryExpr, *ast.StarExpr:
		return requestAliasSource(expressionOperand(typed), requests)
	case *ast.CallExpr:
		selector, ok := unparen(typed.Fun).(*ast.SelectorExpr)

		return ok && (selector.Sel.Name == "Clone" || selector.Sel.Name == "WithContext") &&
			requestAliasSource(selector.X, requests)
	}

	return false
}

func requestURLAliasSource(expression ast.Expr, aliases requestMutationAliases) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return aliases.urls[identifierObjectPosition(typed)]
	case *ast.SelectorExpr:
		return typed.Sel.Name == "URL" && requestAliasSource(typed.X, aliases.requests)
	case *ast.UnaryExpr, *ast.StarExpr:
		return requestURLAliasSource(expressionOperand(typed), aliases)
	}

	return false
}

func requestHeaderAliasSource(expression ast.Expr, file *ast.File, aliases requestMutationAliases) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return aliases.headers[identifierObjectPosition(typed)]
	case *ast.SelectorExpr:
		return typed.Sel.Name == httpHeaderFieldName && requestAliasSource(typed.X, aliases.requests)
	case *ast.CallExpr:
		return len(typed.Args) == 1 && isWireMapType(file, typed.Fun, collectWireMapTypes(file)) &&
			requestHeaderAliasSource(typed.Args[0], file, aliases)
	case *ast.UnaryExpr, *ast.StarExpr:
		return requestHeaderAliasSource(expressionOperand(typed), file, aliases)
	}

	return false
}

func requestBodyAliasSource(expression ast.Expr, aliases requestMutationAliases) bool {
	expression = unparen(expression)
	switch typed := expression.(type) {
	case *ast.Ident:
		return aliases.bodies[identifierObjectPosition(typed)]
	case *ast.SelectorExpr:
		return typed.Sel.Name == "Body" && requestAliasSource(typed.X, aliases.requests)
	case *ast.UnaryExpr, *ast.StarExpr:
		return requestBodyAliasSource(expressionOperand(typed), aliases)
	}

	return false
}

func requestAssignmentMutates(
	file *ast.File,
	assignment *ast.AssignStmt,
	parameters map[string]token.Pos,
	requestObjects map[token.Pos]bool,
	aliases requestMutationAliases,
) bool {
	if object, matched := sendRequestAssignmentObject(assignment, parameters); matched && requestObjects[object] {
		return false
	}

	if isHTTPRequestConstructorAssignment(file, assignment, requestObjects) {
		return false
	}

	for index, target := range assignment.Lhs {
		if identifier, ok := unparen(target).(*ast.Ident); ok && isRequestAliasAt(identifier, aliases, assignment.Pos()) {
			if index < len(assignment.Rhs) && isNewRequestAliasBinding(
				file, assignment, identifier, assignment.Rhs[index], aliases,
			) {
				continue
			}

			return true
		}

		if isRequestMutationTargetAt(target, aliases, assignment.Pos()) {
			return true
		}
	}

	return false
}

func isHTTPRequestConstructorAssignment(
	file *ast.File,
	assignment *ast.AssignStmt,
	requestObjects map[token.Pos]bool,
) bool {
	for _, target := range assignment.Lhs {
		identifier, ok := unparen(target).(*ast.Ident)
		if !ok || !requestObjects[identifierObjectPosition(identifier)] {
			continue
		}

		for _, value := range assignment.Rhs {
			call, ok := unparen(value).(*ast.CallExpr)
			if ok && isHTTPNewRequestCall(file, call) {
				return true
			}
		}
	}

	return false
}

func isNewRequestAliasBinding(
	file *ast.File,
	assignment *ast.AssignStmt,
	identifier *ast.Ident,
	expression ast.Expr,
	aliases requestMutationAliases,
) bool {
	if assignment.Tok != token.DEFINE || identifier.Obj == nil || identifier.Obj.Pos() != identifier.Pos() {
		return false
	}

	return requestAliasSource(expression, aliases.requests) ||
		requestURLAliasSource(expression, aliases) ||
		requestHeaderAliasSource(expression, file, aliases) ||
		requestBodyAliasSource(expression, aliases) ||
		requestBufferAliasSource(expression, aliases)
}

func isRequestAliasAt(identifier *ast.Ident, aliases requestMutationAliases, position token.Pos) bool {
	objectPosition := identifierObjectPosition(identifier)
	if aliases.requests[objectPosition] || aliases.urls[objectPosition] || aliases.headers[objectPosition] ||
		aliases.bodies[objectPosition] {
		return true
	}

	liveFrom, found := aliases.bufferLiveFrom[objectPosition]

	return found && (position == token.NoPos || position >= liveFrom)
}

func isRequestMutationTargetAt(expression ast.Expr, aliases requestMutationAliases, position token.Pos) bool {
	root := requestMutationRootIdentifier(expression)
	if root == nil {
		return false
	}

	return isRequestAliasAt(root, aliases, position)
}

func requestMutationRootIdentifier(expression ast.Expr) *ast.Ident {
	for {
		expression = unparen(expression)
		switch typed := expression.(type) {
		case *ast.IndexExpr:
			expression = typed.X
		case *ast.SliceExpr:
			expression = typed.X
		default:
			return selectorRootIdentifier(expression)
		}
	}
}

func requestCallEscapesWithAllowances(
	call *ast.CallExpr,
	function *ast.FuncDecl,
	aliases requestMutationAliases,
	allowedCalls map[*ast.CallExpr]bool,
) bool {
	if allowedCalls[call] {
		return false
	}

	selector, hasSelector := unparen(call.Fun).(*ast.SelectorExpr)
	if hasSelector && isRequestInjectedDo(selector, call, function, aliases) {
		return false
	}

	if hasSelector && requestValueEscapesAt(selector.X, aliases, call.Pos()) {
		return true
	}

	for _, argument := range call.Args {
		if requestValueEscapesAt(argument, aliases, call.Pos()) {
			return true
		}
	}

	return false
}

func allowedConstructedRequestCalls(
	file *ast.File,
	function *ast.FuncDecl,
	requestObjects map[token.Pos]bool,
	allowContentTypeSetter bool,
) (map[*ast.CallExpr]bool, map[*ast.SelectorExpr]bool) {
	allowedCalls := make(map[*ast.CallExpr]bool)
	allowedSelectors := make(map[*ast.SelectorExpr]bool)

	if !allowContentTypeSetter {
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if isCall && isBytesBodyReader(file, call) {
				allowedCalls[call] = true
			}

			return true
		})

		return allowedCalls, allowedSelectors
	}

	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}

		if isBytesBodyReader(file, call) {
			allowedCalls[call] = true
		}

		if !isGeneratedContentTypeSet(file, call, requestObjects) {
			return true
		}

		allowedCalls[call] = true
		selector, ok := unparen(call.Fun).(*ast.SelectorExpr)

		if ok {
			allowedSelectors[selector] = true
		}

		return true
	})

	return allowedCalls, allowedSelectors
}

func requestReturnEscapes(
	function *ast.FuncDecl,
	statement *ast.ReturnStmt,
	aliases requestMutationAliases,
	allowConstructedReturn bool,
) bool {
	for index, result := range statement.Results {
		if !requestValueEscapesAt(result, aliases, result.Pos()) {
			continue
		}

		if allowConstructedReturn && function.Name.Name == "newRequest" && index == 0 &&
			requestAliasSource(result, aliases.requests) {
			continue
		}

		return true
	}

	return false
}

func clonePositionSet(values map[token.Pos]bool) map[token.Pos]bool {
	cloned := make(map[token.Pos]bool, len(values))
	for value := range values {
		cloned[value] = true
	}

	return cloned
}

func isRequestInjectedDo(
	selector *ast.SelectorExpr,
	call *ast.CallExpr,
	function *ast.FuncDecl,
	aliases requestMutationAliases,
) bool {
	if selector.Sel.Name != "Do" || len(call.Args) != 1 || function == nil {
		return false
	}

	client, ok := unparen(selector.X).(*ast.Ident)

	return ok && identifierObjectPosition(client) == functionParameters(function)["httpClient"] &&
		requestAliasSource(call.Args[0], aliases.requests)
}

func requestValueEscapes(node ast.Node, aliases requestMutationAliases) bool {
	return requestValueEscapesAt(node, aliases, token.NoPos)
}

func requestValueEscapesAt(node ast.Node, aliases requestMutationAliases, position token.Pos) bool {
	found := false

	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || found {
			return false
		}

		identifier, ok := child.(*ast.Ident)
		if ok && isRequestAliasAt(identifier, aliases, position) {
			found = true

			return false
		}

		return true
	})

	return found
}

func requestMethodValueEscapes(selector *ast.SelectorExpr, aliases requestMutationAliases) bool {
	if !requestValueEscapesAt(selector.X, aliases, selector.Pos()) {
		return false
	}

	switch selector.Sel.Name {
	case "URL", "Method", "Header", "Body", "Host", "RequestURI", "Proto", "ProtoMajor", "ProtoMinor",
		"ContentLength", "TransferEncoding", "Close", "Trailer", "RemoteAddr", "TLS", "Cancel",
		"Scheme", "Opaque", "User", "Path", "RawPath", "OmitHost", "ForceQuery", "RawQuery",
		"Fragment", "RawFragment", "GetBody":
		return false
	default:
		return true
	}
}

func requestPackageValueEscapes(file *ast.File, value *ast.ValueSpec, aliases requestMutationAliases) bool {
	for index, name := range value.Names {
		if index < len(value.Values) && name.Obj != nil && file.Scope.Lookup(name.Name) == name.Obj &&
			requestValueEscapes(value.Values[index], aliases) {
			return true
		}
	}

	return false
}
