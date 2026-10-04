package main

import (
	"go/ast"
	"go/token"
)

const requestMutationMessage = "constructed HTTP request must reach the injected Do without mutation or escape"

type requestMutationAliases struct {
	requests map[token.Pos]bool
	urls     map[token.Pos]bool
	headers  map[token.Pos]bool
	bodies   map[token.Pos]bool
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
		return isRequestMutationTarget(typed.X, aliases)
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
		return requestValueEscapes(typed, aliases)
	case *ast.ReturnStmt:
		return requestReturnEscapes(function, typed, aliases, allowContentTypeSetter)
	case *ast.UnaryExpr:
		return typed.Op == token.AND && requestValueEscapes(typed.X, aliases)
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
		requests: clonePositionSet(requestObjects),
		urls:     make(map[token.Pos]bool),
		headers:  make(map[token.Pos]bool),
		bodies:   make(map[token.Pos]bool),
	}

	for {
		before := len(aliases.requests) + len(aliases.urls) + len(aliases.headers) + len(aliases.bodies)

		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.ValueSpec:
				collectRequestValueAliases(file, typed, aliases)
			case *ast.AssignStmt:
				collectRequestAssignmentAliases(file, typed, aliases)
			}

			return true
		})

		after := len(aliases.requests) + len(aliases.urls) + len(aliases.headers) + len(aliases.bodies)
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
	}
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
		if identifier, ok := unparen(target).(*ast.Ident); ok && isRequestAlias(identifier, aliases) {
			if index < len(assignment.Rhs) && isNewRequestAliasBinding(
				file, assignment, identifier, assignment.Rhs[index], aliases,
			) {
				continue
			}

			return true
		}

		if isRequestMutationTarget(target, aliases) {
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
		requestBodyAliasSource(expression, aliases)
}

func isRequestAlias(identifier *ast.Ident, aliases requestMutationAliases) bool {
	position := identifierObjectPosition(identifier)

	return aliases.requests[position] || aliases.urls[position] || aliases.headers[position] || aliases.bodies[position]
}

func isRequestMutationTarget(expression ast.Expr, aliases requestMutationAliases) bool {
	root := selectorRootIdentifier(expression)
	if root == nil {
		return false
	}

	position := identifierObjectPosition(root)

	return aliases.requests[position] || aliases.urls[position] || aliases.headers[position] || aliases.bodies[position]
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

	if hasSelector && requestValueEscapes(selector.X, aliases) {
		return true
	}

	for _, argument := range call.Args {
		if requestValueEscapes(argument, aliases) {
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
		return allowedCalls, allowedSelectors
	}

	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || !isGeneratedContentTypeSet(file, call, requestObjects) {
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
		if !requestValueEscapes(result, aliases) {
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
	found := false

	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || found {
			return false
		}

		identifier, ok := child.(*ast.Ident)
		if ok && isRequestAlias(identifier, aliases) {
			found = true

			return false
		}

		return true
	})

	return found
}

func requestMethodValueEscapes(selector *ast.SelectorExpr, aliases requestMutationAliases) bool {
	if !requestValueEscapes(selector.X, aliases) {
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
