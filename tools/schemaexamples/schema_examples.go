package main

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	exampleKey           = "example"
	examplesKey          = "examples"
	valueKey             = "value"
	constKey             = "const"
	enumKey              = "enum"
	defsKey              = "$defs"
	definitionsKey       = "definitions"
	propertiesKey        = "properties"
	patternPropertiesKey = "patternProperties"
	dependentSchemasKey  = "dependentSchemas"
	dependenciesKey      = "dependencies"
)

func (v *validator) validateSchemaDeclarations() {
	docs := v.sortedDocuments()
	for _, doc := range docs {
		v.walkDocumentSchemas(doc, doc.data, "#")
	}
}

func (v *validator) walkDocumentSchemas(doc *document, value any, pointer string) {
	switch current := value.(type) {
	case map[string]any:
		v.walkDocumentSchemaObject(doc, current, pointer)
	case []any:
		for index, child := range current {
			v.walkDocumentSchemas(doc, child, joinPointer(pointer, strconv.Itoa(index)))
		}
	}
}

func (v *validator) walkDocumentSchemaObject(doc *document, object map[string]any, pointer string) {
	if isOpenAPIExampleObject(object) {
		return
	}

	if pointer == "#" && isSchemaObject(object) {
		v.schemaNode(doc, object, pointer)

		return
	}

	for key, child := range object {
		childPointer := joinPointer(pointer, key)
		if v.walkNamedDocumentSchema(doc, key, child, childPointer) {
			continue
		}

		v.walkDocumentSchemas(doc, child, childPointer)
	}
}

func (v *validator) walkNamedDocumentSchema(doc *document, key string, child any, pointer string) bool {
	switch key {
	case "schemas":
		schemaMap, ok := asMap(child)
		if !ok {
			return false
		}

		for name, schema := range schemaMap {
			v.schemaNode(doc, schema, joinPointer(pointer, name))
		}

		return true
	case "schema":
		v.schemaNode(doc, child, pointer)

		return true
	case exampleKey, examplesKey, valueKey, "default", constKey, enumKey, "messages":
		return true
	default:
		return false
	}
}

func (v *validator) schemaNode(doc *document, value any, pointer string) {
	schema, isMap := asMap(value)
	owner := location{doc: doc, pointer: pointer}

	if !isMap {
		return
	}

	if sample, ok := schema[exampleKey]; ok {
		v.validateValue(owner, sample, joinPointer(pointer, exampleKey), schema[exampleEvidenceKey])
	}

	if examples, ok := schema[examplesKey]; ok {
		list, ok := examples.([]any)

		switch {
		case !ok:
			v.addIssue(doc.path, joinPointer(pointer, examplesKey), "schema examples must be an array")
		case len(list) == 0:
			v.addIssue(doc.path, joinPointer(pointer, examplesKey), "schema examples must contain at least one value")
		default:
			for index, sample := range list {
				examplesPointer := joinPointer(joinPointer(pointer, examplesKey), strconv.Itoa(index))
				v.validateValue(owner, sample, examplesPointer, schema[exampleEvidenceKey])
			}
		}
	}

	for key, child := range schema {
		if isSchemaChildKey(key) {
			v.schemaChild(doc, key, child, joinPointer(pointer, key))
		}
	}
}

func (v *validator) schemaChild(doc *document, key string, value any, pointer string) {
	switch child := value.(type) {
	case map[string]any:
		if isSchemaMapContainer(key) {
			for name, schema := range child {
				v.schemaNode(doc, schema, joinPointer(pointer, name))
			}

			return
		}

		if key == dependenciesKey {
			for name, schema := range child {
				if _, ok := asMap(schema); ok {
					v.schemaNode(doc, schema, joinPointer(pointer, name))
				}
			}

			return
		}

		v.schemaNode(doc, child, pointer)
	case []any:
		for index, schema := range child {
			v.schemaNode(doc, schema, joinPointer(pointer, strconv.Itoa(index)))
		}
	}
}

func isSchemaMapContainer(key string) bool {
	switch key {
	case propertiesKey, patternPropertiesKey, definitionsKey, defsKey, dependentSchemasKey:
		return true
	default:
		return false
	}
}

func isOpenAPIExampleObject(value map[string]any) bool {
	if !hasKey(value, valueKey) && !hasKey(value, "externalValue") {
		return false
	}

	for key := range value {
		switch key {
		case "$ref", "summary", "description", valueKey, "externalValue", exampleEvidenceKey:
		default:
			return false
		}
	}

	return true
}

func (v *validator) validateOpenAPIExamplesAndGroups() {
	docs := v.sortedDocuments()
	for _, doc := range docs {
		root, ok := asMap(doc.data)
		if !ok {
			continue
		}

		_, isOpenAPI := root["openapi"]
		if isOpenAPI || hasMediaComponents(root) {
			v.walkOpenAPIMedia(doc, doc.data, "#")
		}

		if isOpenAPI {
			v.collectOpenAPIGroups(doc, root)
		}
	}
}

func hasMediaComponents(root map[string]any) bool {
	components, componentsAreMap := asMap(root["components"])
	if !componentsAreMap {
		return false
	}

	return hasKey(components, "responses") || hasKey(components, "requestBodies")
}

func (v *validator) walkOpenAPIMedia(doc *document, value any, pointer string) {
	switch current := value.(type) {
	case map[string]any:
		v.walkOpenAPIMediaObject(doc, current, pointer)
	case []any:
		for index, child := range current {
			v.walkOpenAPIMedia(doc, child, joinPointer(pointer, strconv.Itoa(index)))
		}
	}
}

func (v *validator) walkOpenAPIMediaObject(doc *document, object map[string]any, pointer string) {
	if isOpenAPIExampleObject(object) {
		return
	}

	for key, child := range object {
		if key == "schema" || key == exampleKey || key == examplesKey || key == valueKey {
			continue
		}

		if key == "content" {
			v.walkOpenAPIMediaContent(doc, child, joinPointer(pointer, key))

			continue
		}

		v.walkOpenAPIMedia(doc, child, joinPointer(pointer, key))
	}
}

func (v *validator) walkOpenAPIMediaContent(doc *document, value any, pointer string) {
	content, ok := asMap(value)
	if !ok {
		return
	}

	for _, mediaType := range sortedKeys(content) {
		mediaPointer := joinPointer(pointer, mediaType)

		media, ok := asMap(content[mediaType])
		if !ok {
			continue
		}

		v.validateMediaExamples(doc, mediaPointer, media)
		v.walkOpenAPIMedia(doc, media, mediaPointer)
	}
}

func (v *validator) validateMediaExamples(doc *document, pointer string, media map[string]any) bool {
	_, hasSchema := media["schema"]
	schemaPointer := joinPointer(pointer, "schema")

	if !hasSchema {
		if hasKey(media, exampleKey) || nonEmptyExamples(media[examplesKey]) {
			v.addIssue(doc.path, pointer, "media examples have no schema owner")
		}

		return false
	}

	owner := location{doc: doc, pointer: schemaPointer}
	found := false

	if sample, ok := media[exampleKey]; ok {
		found = true

		v.validateValue(owner, sample, joinPointer(pointer, exampleKey), media[exampleEvidenceKey])
	}

	if examples, ok := media[examplesKey]; ok {
		found = v.validateMediaExampleObjects(doc, owner, pointer, examples) || found
	}

	return found
}

func (v *validator) validateMediaExampleObjects(doc *document, owner location, pointer string, rawExamples any) bool {
	entries, ok := asMap(rawExamples)
	if !ok {
		v.addIssue(doc.path, joinPointer(pointer, examplesKey), "media examples must be a named object map")

		return false
	}

	found := false

	for name, rawExample := range entries {
		examplePointer := joinPointer(joinPointer(pointer, examplesKey), name)

		exampleDoc, resolvedPointer, exampleObject, err := v.resolveExampleObject(doc, rawExample, examplePointer)
		if err != nil {
			v.addIssue(doc.path, examplePointer, "unresolved example owner reference")

			found = true

			continue
		}

		exampleMap, ok := asMap(exampleObject)
		if !ok {
			v.addIssue(exampleDoc.path, resolvedPointer, "media example must be an Example Object")

			found = true

			continue
		}

		found = true

		v.validateOpenAPIExampleObject(owner, exampleDoc, resolvedPointer, exampleMap)
	}

	return found
}

func (v *validator) resolveExampleObject(doc *document, raw any, pointer string) (*document, string, any, error) {
	currentDoc := doc
	currentPointer := pointer
	current := raw
	seen := make(map[string]bool)

	for {
		object, ok := asMap(current)
		if !ok {
			return currentDoc, currentPointer, current, nil
		}

		ref, hasRef := object["$ref"].(string)
		if !hasRef {
			return currentDoc, currentPointer, current, nil
		}

		key := currentDoc.uri + currentPointer + ref
		if seen[key] {
			return nil, "", nil, errCyclicExampleReference
		}

		seen[key] = true

		resolvedDoc, resolvedPointer, resolved, err := v.resolveReference(currentDoc, ref)
		if err != nil {
			return nil, "", nil, err
		}

		if strings.Contains(resolvedPointer, "#/components/examples/") {
			v.usedExampleObjects[resolvedDoc.uri+resolvedPointer] = true
		}

		currentDoc, currentPointer, current = resolvedDoc, resolvedPointer, resolved
	}
}

func (v *validator) validateOpenAPIExampleObject(
	owner location,
	exampleDoc *document,
	pointer string,
	example map[string]any,
) {
	if hasKey(example, "externalValue") {
		v.addIssue(exampleDoc.path, joinPointer(pointer, "externalValue"), "external example values are not allowed")
	}

	value, hasValue := example[valueKey]
	if !hasValue {
		v.addIssue(exampleDoc.path, pointer, "Example Object requires a local value")

		return
	}

	v.validateValue(owner, value, joinPointer(pointer, valueKey), example[exampleEvidenceKey])
}

func (v *validator) collectOpenAPIGroups(doc *document, root map[string]any) {
	paths, ok := asMap(root["paths"])
	if !ok {
		return
	}

	for _, pathName := range sortedKeys(paths) {
		v.collectOpenAPIPathGroups(doc, pathName, paths[pathName])
	}
}

func (v *validator) collectOpenAPIPathGroups(doc *document, pathName string, rawPath any) {
	pathPointer := joinPointer(joinPointer("#", "paths"), pathName)

	pathDoc, resolvedPointer, resolved, err := v.resolveObject(doc, pathPointer, rawPath)
	if err != nil {
		v.addIssue(doc.path, pathPointer, "unresolved path item")

		return
	}

	pathItem, ok := asMap(resolved)
	if !ok {
		return
	}

	for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
		v.collectOpenAPIOperationGroups(pathDoc, pathName, resolvedPointer, pathItem, method)
	}
}

func (v *validator) collectOpenAPIOperationGroups(
	pathDoc *document,
	pathName string,
	pathPointer string,
	pathItem map[string]any,
	method string,
) {
	operation, exists := asMap(pathItem[method])
	if !exists {
		return
	}

	operationPointer := joinPointer(pathPointer, method)
	identity := strings.ToUpper(method) + " " + pathName
	v.collectOpenAPIRequestBodyGroup(pathDoc, operationPointer, operation, identity)
	v.collectOpenAPIResponseGroups(pathDoc, operationPointer, operation, identity)
}

func (v *validator) collectOpenAPIRequestBodyGroup(
	pathDoc *document,
	operationPointer string,
	operation map[string]any,
	identity string,
) {
	requestBody, exists := operation["requestBody"]
	if !exists {
		return
	}

	requestPointer := joinPointer(operationPointer, "requestBody")

	bodyDoc, bodyPointer, bodyValue, err := v.resolveObject(pathDoc, requestPointer, requestBody)
	if err != nil {
		v.addIssue(pathDoc.path, requestPointer, "unresolved request body")

		return
	}

	body, isObject := asMap(bodyValue)
	if isObject {
		v.collectMediaGroups(bodyDoc, bodyPointer, body["content"], identity+" request")
	}
}

func (v *validator) collectOpenAPIResponseGroups(
	pathDoc *document,
	operationPointer string,
	operation map[string]any,
	identity string,
) {
	responses, ok := asMap(operation["responses"])
	if !ok {
		return
	}

	for _, status := range sortedKeys(responses) {
		role, required := responseRole(status)
		if !required {
			continue
		}

		v.collectOpenAPIResponseGroup(pathDoc, operationPointer, status, role, responses[status], identity)
	}
}

func (v *validator) collectOpenAPIResponseGroup(
	pathDoc *document,
	operationPointer string,
	status string,
	role string,
	rawResponse any,
	identity string,
) {
	responsePointer := joinPointer(joinPointer(operationPointer, "responses"), status)

	responseDoc, resolvedPointer, responseValue, err := v.resolveObject(pathDoc, responsePointer, rawResponse)
	if err != nil {
		v.addIssue(pathDoc.path, responsePointer, "unresolved response")

		return
	}

	response, ok := asMap(responseValue)
	if !ok {
		return
	}

	label := identity + " " + role + " response " + status
	v.collectMediaGroups(responseDoc, resolvedPointer, response["content"], label)
}

func (v *validator) collectMediaGroups(doc *document, parentPointer string, rawContent any, label string) {
	content, ok := asMap(rawContent)
	if !ok {
		return
	}

	for _, mediaType := range sortedKeys(content) {
		media, ok := asMap(content[mediaType])
		if !ok {
			continue
		}

		_, hasSchema := media["schema"]
		if !hasSchema {
			continue
		}

		mediaPointer := joinPointer(joinPointer(parentPointer, "content"), mediaType)
		schemaPointer := joinPointer(mediaPointer, "schema")
		groupName := label + " " + mediaType
		v.report.groups++

		if hasMediaExampleDeclaration(media) {
			continue
		}

		if v.rootExampleExists(location{doc: doc, pointer: schemaPointer}, make(map[string]bool)) {
			continue
		}

		v.addIssue(doc.path, schemaPointer, "missing required sample group: "+groupName)
	}
}

func hasMediaExampleDeclaration(media map[string]any) bool {
	if hasKey(media, exampleKey) {
		return true
	}

	return nonEmptyExamples(media[examplesKey])
}

func responseRole(status string) (string, bool) {
	switch {
	case strings.EqualFold(status, "default"), strings.HasPrefix(status, "4"), strings.HasPrefix(status, "5"):
		return "failure", true
	case strings.HasPrefix(status, "2"):
		return "success", true
	default:
		return "", false
	}
}

func (v *validator) resolveObject(doc *document, pointer string, value any) (*document, string, any, error) {
	currentDoc, currentPointer, current := doc, pointer, value
	seen := make(map[string]bool)

	for {
		object, ok := asMap(current)
		if !ok {
			return currentDoc, currentPointer, current, nil
		}

		ref, hasRef := object["$ref"].(string)
		if !hasRef {
			return currentDoc, currentPointer, current, nil
		}

		key := currentDoc.uri + currentPointer + ref
		if seen[key] {
			return nil, "", nil, errCyclicReference
		}

		seen[key] = true

		resolvedDoc, resolvedPointer, resolved, err := v.resolveReference(currentDoc, ref)
		if err != nil || resolvedDoc == nil {
			return nil, "", nil, errUnresolvedReference
		}

		currentDoc, currentPointer, current = resolvedDoc, resolvedPointer, resolved
	}
}

func (v *validator) validateAsyncAPIExamplesAndGroups() {
	for _, doc := range v.sortedDocuments() {
		root, ok := asMap(doc.data)
		if !ok {
			continue
		}

		if _, isAsyncAPI := root["asyncapi"]; !isAsyncAPI {
			continue
		}

		seen := make(map[string]bool)

		for _, message := range v.asyncAPIMessages(doc, root) {
			key := message.doc.uri + message.pointer
			if seen[key] {
				continue
			}

			seen[key] = true

			v.validateAsyncAPIMessage(message)
		}
	}
}

func (v *validator) asyncAPIMessages(doc *document, root map[string]any) []location {
	var result []location

	add := func(pointer string, value any) {
		v.collectAsyncMessageLocations(doc, pointer, value, make(map[string]bool), &result)
	}
	collectAsyncComponentMessages(root, add)
	collectAsyncChannelMessages(root, add)
	collectAsyncOperationMessages(root, add)

	return result
}

func collectAsyncComponentMessages(root map[string]any, add func(string, any)) {
	components, componentsAreMap := asMap(root["components"])
	if !componentsAreMap {
		return
	}

	messages, ok := asMap(components["messages"])
	if !ok {
		return
	}

	for _, name := range sortedKeys(messages) {
		pointer := joinPointer(joinPointer(joinPointer("#", "components"), "messages"), name)
		add(pointer, messages[name])
	}
}

func collectAsyncChannelMessages(root map[string]any, add func(string, any)) {
	channels, ok := asMap(root["channels"])
	if !ok {
		return
	}

	for _, channelName := range sortedKeys(channels) {
		channelPointer := joinPointer(joinPointer("#", "channels"), channelName)

		channel, ok := asMap(channels[channelName])
		if !ok {
			continue
		}

		collectAsyncChannelNamedMessages(channelPointer, channel, add)
		collectAsyncChannelOperationMessages(channelPointer, channel, add)
	}
}

func collectAsyncChannelNamedMessages(channelPointer string, channel map[string]any, add func(string, any)) {
	messages, ok := asMap(channel["messages"])
	if !ok {
		return
	}

	for _, messageName := range sortedKeys(messages) {
		pointer := joinPointer(joinPointer(channelPointer, "messages"), messageName)
		add(pointer, messages[messageName])
	}
}

func collectAsyncChannelOperationMessages(channelPointer string, channel map[string]any, add func(string, any)) {
	for _, action := range []string{"publish", "subscribe"} {
		operation, ok := asMap(channel[action])
		if !ok || !hasKey(operation, "message") {
			continue
		}

		pointer := joinPointer(joinPointer(channelPointer, action), "message")
		add(pointer, operation["message"])
	}
}

func collectAsyncOperationMessages(root map[string]any, add func(string, any)) {
	operations, ok := asMap(root["operations"])
	if !ok {
		return
	}

	for _, operationName := range sortedKeys(operations) {
		operation, ok := asMap(operations[operationName])
		if !ok {
			continue
		}

		operationPointer := joinPointer(joinPointer("#", "operations"), operationName)
		collectAsyncOperationNamedMessages(operationPointer, operation["messages"], add)
	}
}

func collectAsyncOperationNamedMessages(operationPointer string, value any, add func(string, any)) {
	messagesPointer := joinPointer(operationPointer, "messages")

	switch messages := value.(type) {
	case []any:
		for index, message := range messages {
			add(joinPointer(messagesPointer, strconv.Itoa(index)), message)
		}
	case map[string]any:
		for _, messageName := range sortedKeys(messages) {
			add(joinPointer(messagesPointer, messageName), messages[messageName])
		}
	}
}

func (v *validator) collectAsyncMessageLocations(
	doc *document,
	pointer string,
	value any,
	seen map[string]bool,
	result *[]location,
) {
	object, ok := asMap(value)
	if !ok {
		return
	}

	if v.collectAsyncMessageReference(doc, pointer, object, seen, result) {
		return
	}

	if v.collectAsyncMessageAlternatives(doc, pointer, object, seen, result) {
		return
	}

	if hasKey(object, "payload") {
		*result = append(*result, location{doc: doc, pointer: pointer})
	}
}

func (v *validator) collectAsyncMessageReference(
	doc *document,
	pointer string,
	object map[string]any,
	seen map[string]bool,
	result *[]location,
) bool {
	ref, ok := object["$ref"].(string)
	if !ok {
		return false
	}

	key := doc.uri + pointer + ref
	if seen[key] {
		return true
	}

	seen[key] = true

	targetDoc, targetPointer, target, err := v.resolveReference(doc, ref)
	if err != nil {
		v.addIssue(doc.path, pointer, "unresolved AsyncAPI message reference")

		return true
	}

	v.collectAsyncMessageLocations(targetDoc, targetPointer, target, seen, result)

	return true
}

func (v *validator) collectAsyncMessageAlternatives(
	doc *document,
	pointer string,
	object map[string]any,
	seen map[string]bool,
	result *[]location,
) bool {
	alternatives, ok := object["oneOf"].([]any)
	if !ok {
		return false
	}

	for index, alternative := range alternatives {
		alternativePointer := joinPointer(joinPointer(pointer, "oneOf"), strconv.Itoa(index))
		v.collectAsyncMessageLocations(doc, alternativePointer, alternative, seen, result)
	}

	return true
}

func (v *validator) validateAsyncAPIMessage(message location) {
	value, found := resolvePointer(message.doc.data, message.pointer)
	if !found {
		v.addIssue(message.doc.path, message.pointer, "unresolved AsyncAPI message owner")

		return
	}

	object, isObject := asMap(value)
	if !isObject {
		return
	}

	payload, hasPayload := object["payload"]
	if !hasPayload {
		if nonEmptyExamples(object[examplesKey]) {
			examplesPointer := joinPointer(message.pointer, examplesKey)
			v.addIssue(message.doc.path, examplesPointer, "AsyncAPI examples have no payload schema owner")
		}

		return
	}

	payloadPointer := joinPointer(message.pointer, "payload")
	payloadOwner := location{doc: message.doc, pointer: payloadPointer}
	v.schemaNode(message.doc, payload, payloadPointer)

	var headersOwner location

	headers, hasHeaders := object["headers"]
	if hasHeaders {
		headersOwner = location{doc: message.doc, pointer: joinPointer(message.pointer, "headers")}
		v.schemaNode(message.doc, headers, headersOwner.pointer)
	}

	messageName := messageNameAt(message.pointer)
	group := "AsyncAPI message " + messageName
	v.report.groups++
	messageEvidence := object[exampleEvidenceKey]

	foundExample := v.validateAsyncAPIExamples(message, object, payloadOwner, headersOwner, hasHeaders, messageEvidence)
	if !foundExample && !v.rootExampleExists(payloadOwner, make(map[string]bool)) {
		v.addIssue(message.doc.path, payloadPointer, "missing required sample group: "+group)
	}
}

func (v *validator) validateAsyncAPIExamples(
	message location,
	object map[string]any,
	payloadOwner location,
	headersOwner location,
	hasHeaders bool,
	evidence any,
) bool {
	examples, exists := object[examplesKey]
	if !exists {
		return false
	}

	list, validList := examples.([]any)
	if !validList {
		v.addIssue(message.doc.path, joinPointer(message.pointer, examplesKey), "AsyncAPI message examples must be an array")

		return false
	}

	foundExample := false

	for index, raw := range list {
		examplePointer := joinPointer(joinPointer(message.pointer, examplesKey), strconv.Itoa(index))
		v.validateAsyncAPIExample(
			message,
			payloadOwner,
			headersOwner,
			hasHeaders,
			evidence,
			raw,
			examplePointer,
		)

		foundExample = true
	}

	return foundExample
}

func (v *validator) validateAsyncAPIExample(
	message location,
	payloadOwner location,
	headersOwner location,
	hasHeaders bool,
	evidence any,
	raw any,
	examplePointer string,
) {
	exampleDoc, resolvedPointer, resolved, err := v.resolveExampleObject(message.doc, raw, examplePointer)
	if err != nil {
		v.addIssue(message.doc.path, examplePointer, "unresolved AsyncAPI example owner reference")

		return
	}

	example, isObject := asMap(resolved)
	if !isObject {
		v.addIssue(exampleDoc.path, resolvedPointer, "AsyncAPI example must be an object")

		return
	}

	if hasKey(example, exampleEvidenceKey) {
		evidencePointer := joinPointer(resolvedPointer, exampleEvidenceKey)
		v.addIssue(exampleDoc.path, evidencePointer, "AsyncAPI Example Object evidence belongs on the Message Object")
	}

	v.validateAsyncAPIExamplePayload(payloadOwner, exampleDoc, resolvedPointer, example, evidence)
	v.validateAsyncAPIExampleHeaders(headersOwner, hasHeaders, exampleDoc, resolvedPointer, example, evidence)
}

func (v *validator) validateAsyncAPIExamplePayload(
	payloadOwner location,
	exampleDoc *document,
	resolvedPointer string,
	example map[string]any,
	evidence any,
) {
	samplePayload, hasSamplePayload := example["payload"]
	if !hasSamplePayload {
		v.addIssue(exampleDoc.path, resolvedPointer, "AsyncAPI example requires a payload")

		return
	}

	samplePointer := joinPointer(resolvedPointer, "payload")
	v.validateValue(payloadOwner, samplePayload, samplePointer, evidence)
}

func (v *validator) validateAsyncAPIExampleHeaders(
	headersOwner location,
	hasHeaders bool,
	exampleDoc *document,
	resolvedPointer string,
	example map[string]any,
	evidence any,
) {
	sampleHeaders, hasSampleHeaders := example["headers"]
	if !hasSampleHeaders {
		return
	}

	headerPointer := joinPointer(resolvedPointer, "headers")
	if !hasHeaders {
		v.addIssue(exampleDoc.path, headerPointer, "AsyncAPI example headers have no schema owner")

		return
	}

	v.validateValue(headersOwner, sampleHeaders, headerPointer, evidence)
}

func messageNameAt(pointer string) string {
	parts := strings.Split(strings.TrimPrefix(pointer, "#/"), "/")
	if len(parts) == 0 {
		return "<unnamed>"
	}

	last := parts[len(parts)-1]
	last = strings.ReplaceAll(strings.ReplaceAll(last, "~1", "/"), "~0", "~")

	return last
}

func (v *validator) rejectUnownedExampleComponents() {
	for _, doc := range v.sortedDocuments() {
		root, isObject := asMap(doc.data)
		if !isObject {
			continue
		}

		components, componentsAreMap := asMap(root["components"])
		if !componentsAreMap {
			continue
		}

		examples, ok := asMap(components[examplesKey])
		if !ok {
			continue
		}

		for _, name := range sortedKeys(examples) {
			pointer := joinPointer(joinPointer(joinPointer("#", "components"), examplesKey), name)
			if !v.usedExampleObjects[doc.uri+pointer] {
				v.addIssue(doc.path, pointer, "Example Object has no schema owner")
			}
		}
	}
}

func (v *validator) sortedDocuments() []*document {
	result := make([]*document, 0, len(v.docs))
	for _, doc := range v.docs {
		result = append(result, doc)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })

	return result
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func safePointerFragment(pointer string) string {
	if strings.HasPrefix(pointer, "#") {
		return pointer
	}

	return "#" + pointer
}

func pointerURL(doc *document, pointer string) string {
	fragment := strings.TrimPrefix(safePointerFragment(pointer), "#")

	parts := strings.Split(fragment, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}

	return doc.uri + "#" + strings.Join(parts, "/")
}
