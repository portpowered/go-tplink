package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const exampleEvidenceKey = "x-example-evidence"

const windowsOS = "windows"

const fileScheme = "file"

var (
	errNoSchemaDocuments      = errors.New("no YAML or JSON schema documents found")
	errRemoteLoadingDisabled  = errors.New("remote schema loading is disabled")
	errReferenceNotLocal      = errors.New("reference is not a local file reference")
	errReferenceFragment      = errors.New("reference fragment does not resolve")
	errCyclicReference        = errors.New("cyclic reference")
	errCyclicExampleReference = errors.New("cyclic example reference")
	errUnresolvedReference    = errors.New("unresolved reference")
)

type document struct {
	path     string
	uri      string
	data     any
	resource any
}

type location struct {
	doc     *document
	pointer string
}

type report struct {
	documents int
	examples  int
	groups    int
}

type validationIssuesError []string

func (issues validationIssuesError) Error() string {
	return strings.Join(issues, "\n")
}

type validator struct {
	docs               map[string]*document
	byURI              map[string]*document
	groupScopes        map[string]string
	groupComponents    map[string]map[string]any
	issues             []string
	usedExampleObjects map[string]bool
	compiled           map[string]*jsonschema.Schema
	compiler           *jsonschema.Compiler
	report             report
}

func validateRoots(roots []string) (report, error) {
	schemaValidator := &validator{
		docs:               make(map[string]*document),
		byURI:              make(map[string]*document),
		groupScopes:        make(map[string]string),
		groupComponents:    make(map[string]map[string]any),
		issues:             nil,
		usedExampleObjects: make(map[string]bool),
		compiled:           make(map[string]*jsonschema.Schema),
		compiler:           nil,
		report:             report{documents: 0, examples: 0, groups: 0},
	}

	err := schemaValidator.loadRoots(roots)
	if err != nil {
		return schemaValidator.report, err
	}

	schemaValidator.prepareCanonicalGroups()
	schemaValidator.loadReferencedDocuments()
	schemaValidator.buildCompiler()
	schemaValidator.validateSchemaDeclarations()
	schemaValidator.validateOpenAPIExamplesAndGroups()
	schemaValidator.validateAsyncAPIExamplesAndGroups()
	schemaValidator.rejectUnownedExampleComponents()

	schemaValidator.report.documents = len(schemaValidator.docs)
	if len(schemaValidator.issues) > 0 {
		sort.Strings(schemaValidator.issues)

		return schemaValidator.report, validationIssuesError(schemaValidator.issues)
	}

	return schemaValidator.report, nil
}

func (v *validator) loadRoots(roots []string) error {
	var paths []string

	for _, root := range roots {
		rootPaths, err := schemaPaths(root)
		if err != nil {
			return err
		}

		paths = append(paths, rootPaths...)
	}

	sort.Strings(paths)

	for _, path := range paths {
		_, err := v.loadDocument(path)
		if err != nil {
			v.addIssue(path, "#", "cannot parse schema document")
		}
	}

	if len(v.docs) == 0 && len(v.issues) == 0 {
		return errNoSchemaDocuments
	}

	return nil
}

func schemaPaths(root string) ([]string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve schema root: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("read schema root: %w", err)
	}

	if !info.IsDir() {
		if isYAMLOrJSON(abs) {
			return []string{abs}, nil
		}

		return nil, nil
	}

	var paths []string

	err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || !isYAMLOrJSON(path) || isGeneratedOpenAPIBundle(path) {
			return nil
		}

		paths = append(paths, path)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan schema root %s: %w", root, err)
	}

	return paths, nil
}

func isGeneratedOpenAPIBundle(path string) bool {
	if !strings.EqualFold(filepath.Base(path), "openapi.yaml") {
		return false
	}

	dir := filepath.Dir(path)
	for _, marker := range []string{
		filepath.Join(dir, "openapi.base.yaml"),
		filepath.Join(dir, "model-groups.json"),
		filepath.Join(dir, "openapi", "sources.yaml"),
	} {
		_, err := os.Stat(marker)
		if err == nil {
			return true
		}
	}

	return false
}

func isYAMLOrJSON(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func canonicalPath(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == windowsOS {
		return strings.ToLower(clean)
	}

	return clean
}

func fileURI(path string) string {
	abs, _ := filepath.Abs(path)

	uriPath := filepath.ToSlash(abs)
	if runtime.GOOS == windowsOS && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}

	var uri url.URL

	uri.Scheme = fileScheme
	uri.Path = uriPath

	return uri.String()
}

func (v *validator) loadDocument(path string) (*document, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve schema document path: %w", err)
	}

	key := canonicalPath(abs)
	if doc, ok := v.docs[key]; ok {
		return doc, nil
	}

	contents, err := fs.ReadFile(os.DirFS(filepath.Dir(abs)), filepath.Base(abs))
	if err != nil {
		return nil, fmt.Errorf("read schema document: %w", err)
	}

	var parsed any
	if strings.EqualFold(filepath.Ext(abs), ".json") {
		err = json.Unmarshal(contents, &parsed)
	} else {
		err = yaml.Unmarshal(contents, &parsed)
	}

	if err != nil {
		return nil, fmt.Errorf("parse schema document: %w", err)
	}

	jsonBytes, err := json.Marshal(parsed)
	if err != nil {
		return nil, fmt.Errorf("normalize schema document: %w", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(jsonBytes)))
	decoder.UseNumber()

	err = decoder.Decode(&parsed)
	if err != nil {
		return nil, fmt.Errorf("decode normalized schema document: %w", err)
	}

	doc := &document{path: abs, uri: fileURI(abs), data: parsed, resource: nil}
	v.docs[key] = doc
	v.byURI[doc.uri] = doc

	return doc, nil
}

func (v *validator) prepareCanonicalGroups() {
	for _, manifest := range v.sortedDocuments() {
		if !strings.EqualFold(filepath.Base(manifest.path), "model-groups.json") {
			continue
		}

		root, ok := asMap(manifest.data)
		if !ok {
			continue
		}

		apiRoot := filepath.Dir(manifest.path)
		repoRoot := filepath.Dir(apiRoot)
		scope := canonicalPath(apiRoot)
		sources := canonicalGroupSources(root)
		groupDocs := v.loadCanonicalGroupDocuments(manifest, repoRoot, scope, sources)
		v.groupComponents[scope] = mergeCanonicalGroupComponents(v, groupDocs)
	}
}

func canonicalGroupSources(root map[string]any) []string {
	var sources []string
	if input, ok := root["input"].(string); ok {
		sources = append(sources, input)
	}

	groups, groupsAreArray := root["groups"].([]any)
	if !groupsAreArray {
		return sources
	}

	for _, value := range groups {
		group, groupIsObject := asMap(value)
		if !groupIsObject {
			continue
		}

		source, sourceIsString := group["source"].(string)
		if sourceIsString {
			sources = append(sources, source)
		}
	}

	return sources
}

func (v *validator) loadCanonicalGroupDocuments(
	manifest *document,
	repoRoot string,
	scope string,
	sources []string,
) []*document {
	groupDocs := make([]*document, 0, len(sources))
	seen := make(map[string]bool)

	for _, source := range sources {
		path := source
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, filepath.FromSlash(source))
		}

		doc, err := v.loadDocument(path)
		if err != nil {
			v.addIssue(manifest.path, "#", "canonical schema source is missing")

			continue
		}

		key := canonicalPath(doc.path)
		if seen[key] {
			continue
		}

		seen[key] = true
		v.groupScopes[key] = scope

		groupDocs = append(groupDocs, doc)
	}

	return groupDocs
}

func mergeCanonicalGroupComponents(schemaValidator *validator, groupDocs []*document) map[string]any {
	components := make(map[string]any)

	for _, doc := range groupDocs {
		documentRoot, documentRootIsObject := asMap(doc.data)
		if !documentRootIsObject {
			continue
		}

		docComponents, docComponentsAreMap := asMap(documentRoot["components"])
		if !docComponentsAreMap {
			continue
		}

		mergeCanonicalComponentSections(schemaValidator, doc, components, docComponents)
	}

	return components
}

func mergeCanonicalComponentSections(
	schemaValidator *validator,
	doc *document,
	components map[string]any,
	docComponents map[string]any,
) {
	for section, rawValues := range docComponents {
		values, valuesAreMap := asMap(rawValues)
		if !valuesAreMap {
			continue
		}

		merged, ok := asMap(components[section])
		if !ok {
			merged = make(map[string]any)
			components[section] = merged
		}

		for name, value := range values {
			existing, exists := merged[name]
			if exists && !jsonValuesEqual(existing, value) {
				componentPointer := joinPointer(joinPointer(joinPointer("#", "components"), section), name)
				schemaValidator.addIssue(doc.path, componentPointer, "ambiguous canonical component owner")

				continue
			}

			merged[name] = value
		}
	}
}

func jsonValuesEqual(first, second any) bool {
	firstJSON, firstErr := json.Marshal(first)
	secondJSON, secondErr := json.Marshal(second)

	return firstErr == nil && secondErr == nil && string(firstJSON) == string(secondJSON)
}

func (v *validator) loadReferencedDocuments() {
	seen := make(map[string]bool)

	for {
		var pending []*document

		for key, doc := range v.docs {
			if !seen[key] {
				pending = append(pending, doc)
			}
		}

		if len(pending) == 0 {
			return
		}

		sort.Slice(pending, func(i, j int) bool { return pending[i].path < pending[j].path })

		for _, doc := range pending {
			key := canonicalPath(doc.path)
			seen[key] = true

			v.visitReferences(doc, doc.data, "#")
		}
	}
}

func (v *validator) visitReferences(doc *document, value any, pointer string) {
	switch current := value.(type) {
	case map[string]any:
		v.visitObjectReferences(doc, current, pointer)
	case []any:
		v.visitArrayReferences(doc, current, pointer)
	}
}

func (v *validator) visitObjectReferences(doc *document, object map[string]any, pointer string) {
	if ref, ok := object["$ref"].(string); ok {
		_, _, _, err := v.resolveReference(doc, ref)
		if err != nil {
			v.addIssue(doc.path, pointer, "unresolved reference")
		}
	}

	if isSchemaObject(object) {
		v.visitSchemaObjectReferences(doc, object, pointer)

		return
	}

	if isOpenAPIExampleObject(object) {
		return
	}

	for key, child := range object {
		v.visitObjectChildReferences(doc, key, child, joinPointer(pointer, key))
	}
}

func (v *validator) visitSchemaObjectReferences(doc *document, object map[string]any, pointer string) {
	for key, child := range object {
		if !isSchemaChildKey(key) {
			continue
		}

		v.visitSchemaReferences(doc, key, child, joinPointer(pointer, key))
	}
}

func (v *validator) visitObjectChildReferences(doc *document, key string, child any, pointer string) {
	switch key {
	case exampleKey, valueKey, "default", constKey, enumKey:
		return
	case examplesKey:
		v.visitExampleReferences(doc, child, pointer)
	case "schemas":
		if schemas, ok := asMap(child); ok {
			for name, schema := range schemas {
				v.visitReferences(doc, schema, joinPointer(pointer, name))
			}

			return
		}
	}

	v.visitReferences(doc, child, pointer)
}

func (v *validator) visitArrayReferences(doc *document, values []any, pointer string) {
	for index, child := range values {
		v.visitReferences(doc, child, joinPointer(pointer, strconv.Itoa(index)))
	}
}

func (v *validator) visitSchemaReferences(doc *document, key string, value any, pointer string) {
	switch child := value.(type) {
	case map[string]any:
		if isSchemaMapContainer(key) {
			for name, schema := range child {
				v.visitReferences(doc, schema, joinPointer(pointer, name))
			}

			return
		}

		if key == dependenciesKey {
			for name, schema := range child {
				if _, ok := asMap(schema); ok {
					v.visitReferences(doc, schema, joinPointer(pointer, name))
				}
			}

			return
		}

		v.visitReferences(doc, child, pointer)
	case []any:
		for index, schema := range child {
			v.visitReferences(doc, schema, joinPointer(pointer, strconv.Itoa(index)))
		}
	}
}

func (v *validator) visitExampleReferences(doc *document, examples any, pointer string) {
	visit := func(value any, valuePointer string) {
		if example, ok := asMap(value); ok {
			if ref, ok := example["$ref"].(string); ok {
				_, _, _, err := v.resolveReference(doc, ref)
				if err != nil {
					v.addIssue(doc.path, valuePointer, "unresolved example reference")
				}
			}
		}
	}

	switch values := examples.(type) {
	case map[string]any:
		for name, value := range values {
			visit(value, joinPointer(pointer, name))
		}
	case []any:
		for index, value := range values {
			visit(value, joinPointer(pointer, strconv.Itoa(index)))
		}
	}
}

func (v *validator) buildCompiler() {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.AssertContent()
	compiler.UseLoader(rejectRemoteLoader{})

	keys := make([]string, 0, len(v.docs))
	for key := range v.docs {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		doc := v.docs[key]

		resource := doc.data
		if scope := v.groupScopes[key]; scope != "" {
			if root, ok := asMap(resource); ok {
				resourceCopy := make(map[string]any, len(root))
				maps.Copy(resourceCopy, root)
				resourceCopy["components"] = v.groupComponents[scope]
				resource = resourceCopy
			}
		}

		doc.resource = normalizeSchemaResource(resource)

		err := compiler.AddResource(doc.uri, doc.resource)
		if err != nil {
			v.addIssue(doc.path, "#", "cannot register schema resource")
		}
	}

	v.compiler = compiler
}

type rejectRemoteLoader struct{}

func (rejectRemoteLoader) Load(string) (any, error) {
	return nil, errRemoteLoadingDisabled
}

func normalizeSchemaResource(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}

	var clone any

	err = json.Unmarshal(encoded, &clone)
	if err != nil {
		return value
	}

	normalizeSchemaKeywords(clone)

	return clone
}

func normalizeSchemaKeywords(value any) {
	switch current := value.(type) {
	case map[string]any:
		normalizeSchemaMap(current)
	case []any:
		for _, child := range current {
			normalizeSchemaKeywords(child)
		}
	}
}

func normalizeSchemaMap(schema map[string]any) {
	normalizeNullable(schema)
	normalizeExclusiveBound(schema, "exclusiveMinimum", "minimum")
	normalizeExclusiveBound(schema, "exclusiveMaximum", "maximum")

	for _, child := range schema {
		normalizeSchemaKeywords(child)
	}
}

func normalizeNullable(schema map[string]any) {
	if schema["nullable"] != true {
		return
	}

	schemaType, ok := schema["type"].(string)
	if ok && schemaType != "null" {
		schema["type"] = []any{schemaType, "null"}
	}
}

func normalizeExclusiveBound(schema map[string]any, exclusiveKeyword, boundaryKeyword string) {
	exclusive, ok := schema[exclusiveKeyword].(bool)
	if !ok {
		return
	}

	boundary, hasBoundary := schema[boundaryKeyword]
	if exclusive && hasBoundary {
		schema[exclusiveKeyword] = boundary
		delete(schema, boundaryKeyword)

		return
	}

	delete(schema, exclusiveKeyword)
}

func (v *validator) resolveReference(from *document, reference string) (*document, string, any, error) {
	parsed, err := url.Parse(reference)
	if err != nil {
		return nil, "", nil, fmt.Errorf("parse local reference: %w", err)
	}

	if !isLocalReference(parsed) {
		return nil, "", nil, errReferenceNotLocal
	}

	base, err := url.Parse(from.uri)
	if err != nil {
		return nil, "", nil, fmt.Errorf("parse source document URI: %w", err)
	}

	target := base.ResolveReference(parsed)
	if !isLocalFileURL(target) {
		return nil, "", nil, errReferenceNotLocal
	}

	path := localFilePath(target.Path)

	doc, err := v.loadDocument(path)
	if err != nil {
		return nil, "", nil, fmt.Errorf("load referenced schema document: %w", err)
	}

	pointer := "#"
	if target.Fragment != "" {
		pointer += target.Fragment
	}

	resolved, ok := resolvePointer(doc.data, pointer)
	if !ok {
		if parsed.Path == "" {
			return v.resolveGroupedReference(from, pointer)
		}

		return nil, "", nil, errReferenceFragment
	}

	return doc, pointer, resolved, nil
}

func isLocalReference(reference *url.URL) bool {
	return reference.RawQuery == "" && (reference.Scheme == "" || reference.Scheme == fileScheme)
}

func isLocalFileURL(target *url.URL) bool {
	return target.Scheme == fileScheme && target.Host == ""
}

func localFilePath(path string) string {
	if runtime.GOOS == windowsOS {
		path = strings.TrimPrefix(path, "/")
	}

	return filepath.Clean(filepath.FromSlash(path))
}

func (v *validator) resolveGroupedReference(from *document, pointer string) (*document, string, any, error) {
	ownerDoc, ownerPointer, owner, found := v.resolveGroupedPointer(from, pointer)
	if found {
		return ownerDoc, ownerPointer, owner, nil
	}

	return nil, "", nil, errReferenceFragment
}

func (v *validator) resolveGroupedPointer(from *document, pointer string) (*document, string, any, bool) {
	scope := v.groupScopes[canonicalPath(from.path)]
	if scope == "" || !strings.HasPrefix(pointer, "#/components/") {
		return nil, "", nil, false
	}

	var (
		matchDoc *document
		match    any
	)

	for key, candidate := range v.docs {
		if v.groupScopes[key] != scope {
			continue
		}

		value, ok := resolvePointer(candidate.data, pointer)
		if !ok {
			continue
		}

		if matchDoc != nil {
			return nil, "", nil, false
		}

		matchDoc, match = candidate, value
	}

	if matchDoc == nil {
		return nil, "", nil, false
	}

	return matchDoc, pointer, match, true
}

func resolvePointer(root any, pointer string) (any, bool) {
	if pointer == "" || pointer == "#" {
		return root, true
	}

	fragment := strings.TrimPrefix(pointer, "#")
	if !strings.HasPrefix(fragment, "/") {
		return findAnchor(root, fragment, "#")
	}

	return resolvePointerTokens(root, strings.Split(strings.TrimPrefix(fragment, "/"), "/"))
}

func resolvePointerTokens(root any, tokens []string) (any, bool) {
	current := root

	for _, token := range tokens {
		part := strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")

		switch node := current.(type) {
		case map[string]any:
			child, ok := node[part]
			if !ok {
				return nil, false
			}

			current = child
		case []any:
			child, ok := resolveArrayPointer(node, part)
			if !ok {
				return nil, false
			}

			current = child
		default:
			return nil, false
		}
	}

	return current, true
}

func resolveArrayPointer(values []any, part string) (any, bool) {
	index := -1

	_, err := fmt.Sscanf(part, "%d", &index)
	if err != nil || index < 0 || index >= len(values) {
		return nil, false
	}

	return values[index], true
}

func findAnchor(root any, anchor, pointer string) (any, bool) {
	switch node := root.(type) {
	case map[string]any:
		if node["$anchor"] == anchor {
			return node, true
		}

		for key, child := range node {
			if result, ok := findAnchor(child, anchor, joinPointer(pointer, key)); ok {
				return result, true
			}
		}
	case []any:
		for index, child := range node {
			if result, ok := findAnchor(child, anchor, joinPointer(pointer, strconv.Itoa(index))); ok {
				return result, true
			}
		}
	}

	return nil, false
}

func (v *validator) compileSchema(owner location) *jsonschema.Schema {
	key := owner.doc.uri + owner.pointer
	if schema, ok := v.compiled[key]; ok {
		return schema
	}

	if v.compiler == nil {
		return nil
	}

	schema, err := v.compiler.Compile(pointerURL(owner.doc, owner.pointer))
	if err != nil {
		v.addIssue(owner.doc.path, owner.pointer, "schema could not be compiled")

		return nil
	}

	v.compiled[key] = schema

	return schema
}

func safeValidationReason(err error) string {
	var validationErr *jsonschema.ValidationError
	if errors.As(err, &validationErr) && validationErr.ErrorKind != nil {
		keywords := validationErr.ErrorKind.KeywordPath()
		if len(keywords) > 0 {
			return "does not satisfy schema keyword " + strings.Join(keywords, ".")
		}
	}

	return "does not satisfy its schema"
}

func (v *validator) validateValue(owner location, sample any, samplePointer string, evidence any) {
	v.report.examples++
	if evidence != "synthetic" {
		v.addIssue(owner.doc.path, samplePointer, "sample requires x-example-evidence: synthetic")
	}

	schema := v.compileSchema(owner)
	if schema == nil {
		return
	}

	err := schema.Validate(sample)
	if err != nil {
		v.addIssue(owner.doc.path, samplePointer, safeValidationReason(err))
	}
}

func (v *validator) addIssue(path, pointer, message string) {
	relative, err := filepath.Rel(".", path)
	if err == nil {
		path = filepath.ToSlash(relative)
	}

	formattedIssue := fmt.Sprintf("%s%s: %s", filepath.ToSlash(path), pointer, message)
	v.issues = append(v.issues, formattedIssue)
}

func asMap(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)

	return result, ok
}

func joinPointer(pointer, token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")

	if pointer == "" {
		pointer = "#"
	}

	return pointer + "/" + token
}

func (v *validator) rootExampleExists(owner location, seen map[string]bool) bool {
	key := owner.doc.uri + owner.pointer
	if seen[key] {
		return false
	}

	seen[key] = true

	node, ok := resolvePointer(owner.doc.data, owner.pointer)
	if !ok {
		return false
	}

	if schema, ok := asMap(node); ok {
		if hasKey(schema, exampleKey) || nonEmptyExamples(schema[examplesKey]) {
			return true
		}

		if ref, ok := schema["$ref"].(string); ok {
			doc, pointer, _, err := v.resolveReference(owner.doc, ref)
			if err == nil {
				return v.rootExampleExists(location{doc: doc, pointer: pointer}, seen)
			}
		}
	}

	return false
}

func hasKey(value map[string]any, key string) bool {
	_, ok := value[key]

	return ok
}

func nonEmptyExamples(value any) bool {
	switch examples := value.(type) {
	case []any:
		return len(examples) > 0
	case map[string]any:
		return len(examples) > 0
	default:
		return false
	}
}

func isSchemaObject(value map[string]any) bool {
	for _, key := range []string{
		"$schema", "$id", "$ref", "$dynamicRef", "$anchor", defsKey, definitionsKey, "type",
		propertiesKey, patternPropertiesKey, "additionalProperties", "unevaluatedProperties", "required",
		enumKey, constKey, "items", "prefixItems", "allOf", "anyOf", "oneOf", "not", "if", "then", "else",
		"format", "contentSchema", "contentMediaType", "contentEncoding", "minimum", "maximum",
		"exclusiveMinimum", "exclusiveMaximum", "minLength", "maxLength", "pattern", "uniqueItems",
		"minItems", "maxItems", "minProperties", "maxProperties", dependenciesKey, dependentSchemasKey,
		"propertyNames", "readOnly", "writeOnly", "nullable",
	} {
		if _, ok := value[key]; ok {
			return true
		}
	}

	return false
}

func isSchemaChildKey(key string) bool {
	switch key {
	case propertiesKey, patternPropertiesKey, "additionalProperties", "unevaluatedProperties", dependentSchemasKey,
		definitionsKey, defsKey, "items", "prefixItems", "contains", "propertyNames", "if", "then", "else",
		"not", "allOf", "anyOf", "oneOf", "additionalItems", dependenciesKey, "contentSchema":
		return true
	default:
		return false
	}
}
