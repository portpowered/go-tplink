package main

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type generatedModelSource struct {
	schemaPath string
	outputPath string
	configPath string
	command    string
}

const httpDeleteMethodName = deleteName

const (
	cliGeneratorConfigPath = "cmd/go-tplink/config.yaml"
	cliImportsTemplatePath = "cmd/go-tplink/templates/imports.tmpl"
)

func generatedModelSources() []generatedModelSource {
	return []generatedModelSource{
		{
			schemaPath: "api/cloud-envelope.openapi.yaml",
			outputPath: "pkg/dependencymodels/cloud_envelope.gen.go",
			configPath: "pkg/dependencymodels/config.yaml",
			command: "oapi-codegen v2.8.0 -config pkg/dependencymodels/config.yaml " +
				"api/cloud-envelope.openapi.yaml",
		},
		{
			schemaPath: "api/authentication.openapi.yaml",
			outputPath: "pkg/dependencymodels/authentication.gen.go",
			configPath: "pkg/dependencymodels/config-authentication.yaml",
			command: "oapi-codegen v2.8.0 -config pkg/dependencymodels/config-authentication.yaml " +
				"api/authentication.openapi.yaml",
		},
		{
			schemaPath: "api/devices.openapi.yaml",
			outputPath: "pkg/dependencymodels/devices.gen.go",
			configPath: "pkg/dependencymodels/config-devices.yaml",
			command:    "oapi-codegen v2.8.0 -config pkg/dependencymodels/config-devices.yaml api/devices.openapi.yaml",
		},
		{
			schemaPath: "api/passthrough.openapi.yaml",
			outputPath: "pkg/dependencymodels/passthrough.gen.go",
			configPath: "pkg/dependencymodels/config-passthrough.yaml",
			command:    "oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml",
		},
		{
			schemaPath: "api/client-models.openapi.yaml",
			outputPath: "pkg/tplinkmodels/models.gen.go",
			configPath: "pkg/tplinkmodels/config.yaml",
			command:    "oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml",
		},
		{
			schemaPath: "api/cli-contracts.openapi.yaml",
			outputPath: "cmd/go-tplink/models.gen.go",
			configPath: cliGeneratorConfigPath,
			command:    "oapi-codegen v2.8.0 -config cmd/go-tplink/config.yaml api/cli-contracts.openapi.yaml",
		},
	}
}

func registeredGeneratedOutputs() []string {
	return []string{
		"pkg/dependencymodels/authentication.gen.go",
		"pkg/dependencymodels/cloud_envelope.gen.go",
		"pkg/dependencymodels/cloud_request_compat.gen.go",
		"pkg/dependencymodels/devices.gen.go",
		"pkg/dependencymodels/passthrough.gen.go",
		"pkg/dependencymodels/wire_constants.gen.go",
		"pkg/generatedwire/compat.gen.go",
		"pkg/tplink/constants.gen.go",
		"pkg/tplinkmodels/models.gen.go",
		"cmd/go-tplink/models.gen.go",
	}
}

func compatibilityModelOutputs() []string {
	return []string{"pkg/dependencymodels/cloud_request_compat.gen.go"}
}

type schemaDocument struct {
	Components struct {
		Schemas map[string]yaml.Node `yaml:"schemas"`
	} `yaml:"components"`
}

type endpointDocument struct {
	Paths map[string]map[string]yaml.Node `yaml:"paths"`
}

func checkEndpointInventory(root string) error {
	openAPIPath := filepath.Join(root, "api", "openapi.yaml")
	//nolint:gosec // OpenAPI path is fixed relative to the repository root.
	openAPISource, err := os.ReadFile(openAPIPath)
	if err != nil {
		return fmt.Errorf("read endpoint schema: %w", err)
	}

	var document endpointDocument

	err = yaml.Unmarshal(openAPISource, &document)
	if err != nil {
		return fmt.Errorf("parse endpoint schema: %w", err)
	}

	operations := make(map[string]bool)

	for path, pathItem := range document.Paths {
		for method := range pathItem {
			if isHTTPMethod(method) {
				operations[strings.ToUpper(method)+" "+path] = true
			}
		}
	}

	if len(operations) != 1 || !operations["POST /"] {
		return inventoryError(
			"endpoint schema changed; inventory and wire call-site gate currently cover only POST /, found %s",
			strings.Join(sortedMapKeys(operations), ", "),
		)
	}

	inventoryPath := filepath.Join(root, "docs", "wire-model-inventory.md")
	//nolint:gosec // Inventory path is fixed relative to the repository root.
	inventory, err := os.ReadFile(inventoryPath)
	if err != nil {
		return fmt.Errorf("read endpoint inventory: %w", err)
	}

	const inventoryRow = "| TP-Link Cloud JSON request/response | `api/openapi.yaml` | " +
		"`CloudRequestHTTPMethod=POST`, `CloudRequestPath=/`, " +
		"`CloudRequestContentTypeHeader=Content-Type`, " +
		"`CloudRequestContentType=application/json`, `TokenQueryKey=token` | " +
		"`pkg/dependencies/cloud/cloud.go:newRequest`, `requestURL`, and `Send` |"

	if !strings.Contains(string(inventory), inventoryRow) {
		return inventoryError("wire-model inventory lacks the exact POST / method, path, metadata, and call-site record")
	}

	return nil
}

func isHTTPMethod(method string) bool {
	switch strings.ToLower(method) {
	case "get", "put", "post", httpDeleteMethodName, "options", "head", "patch", "trace":
		return true
	default:
		return false
	}
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func checkGeneratedOutputs(root string) error {
	outputs := registeredGeneratedOutputs()

	registered := make(map[string]bool, len(outputs))
	for _, output := range outputs {
		registered[filepath.Clean(output)] = true
	}

	err := checkRegisteredGeneratedOutputs(root, outputs)
	if err != nil {
		return err
	}

	err = checkUnknownGeneratedOutputs(root, registered)
	if err != nil {
		return err
	}

	return nil
}

func checkRegisteredGeneratedOutputs(root string, outputs []string) error {
	for _, output := range outputs {
		err := checkRegisteredGeneratedOutput(root, output)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkRegisteredGeneratedOutput(root, output string) error {
	path := filepath.Join(root, filepath.FromSlash(output))

	//nolint:gosec // Output path comes from the fixed generated-output registry.
	contents, err := os.ReadFile(path)
	if err != nil {
		return inventoryError("%s: %s: %v", output, generatedOutputMissing, err)
	}

	file, err := parser.ParseFile(token.NewFileSet(), path, contents, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse generated output %s: %w", output, err)
	}

	if !ast.IsGenerated(file) {
		return inventoryError("%s: generated marker is missing", output)
	}

	tracked, err := gitTracksPath(root, output)
	if err != nil {
		return err
	}

	if !tracked {
		return inventoryError("%s: %s", output, generatedOutputUntracked)
	}

	return nil
}

func checkUnknownGeneratedOutputs(root string, registered map[string]bool) error {
	for _, directory := range []string{"pkg", "cmd/go-tplink"} {
		rootPath := filepath.Join(root, filepath.FromSlash(directory))

		err := filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("walk generated output directory: %w", walkErr)
			}

			if entry.IsDir() || !strings.HasSuffix(path, ".gen.go") {
				return nil
			}

			relative, err := filepath.Rel(root, path)
			if err != nil {
				return fmt.Errorf("resolve generated output path: %w", err)
			}

			if !registered[filepath.Clean(relative)] {
				return inventoryError("%s: %s", filepath.ToSlash(relative), generatedOutputUnknown)
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("scan generated outputs under %s: %w", directory, err)
		}
	}

	return nil
}

func gitTracksPath(root, relative string) (bool, error) {
	//nolint:gosec // Git arguments come from the fixed output registry and the repository root.
	command := exec.CommandContext(
		context.Background(),
		"git",
		"-c",
		"safe.directory="+filepath.ToSlash(root),
		"ls-files",
		"--error-unmatch",
		"--",
		filepath.ToSlash(relative),
	)
	command.Dir = root

	output, err := command.CombinedOutput()
	if err == nil {
		return true, nil
	}

	var exitError *exec.ExitError

	if errors.As(err, &exitError) {
		return false, nil
	}

	return false, fmt.Errorf("check tracked generated output %s: %w: %s", relative, err, strings.TrimSpace(string(output)))
}

func readGeneratedModelTypes(root string) (map[string]bool, error) {
	types := make(map[string]bool)
	sources := generatedModelSources()
	compatibilityOutputs := compatibilityModelOutputs()

	paths := make([]string, 0, len(sources)+len(compatibilityOutputs))
	for _, source := range sources {
		paths = append(paths, source.outputPath)
	}

	paths = append(paths, compatibilityOutputs...)

	for _, output := range paths {
		fileTypes, err := readGeneratedTypeSet(filepath.Join(root, filepath.FromSlash(output)))
		if err != nil {
			return nil, fmt.Errorf("parse generated model output %s: %w", output, err)
		}

		for name := range fileTypes {
			types[name] = true
		}
	}

	return types, nil
}

func readGeneratedTypeSet(path string) (map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse generated types from %s: %w", path, err)
	}

	types := make(map[string]bool)

	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}

		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if ok {
				types[typeSpec.Name.Name] = true
			}
		}
	}

	return types, nil
}

func checkGeneratedSchemaModels(root string, generatedTypes map[string]bool) error {
	inventoryPath := filepath.Join(root, "docs", "wire-model-inventory.md")
	//nolint:gosec // Inventory path is fixed relative to the repository root.
	inventory, err := os.ReadFile(inventoryPath)
	if err != nil {
		return fmt.Errorf("read wire model inventory: %w", err)
	}

	inventoryText := string(inventory)

	missing, err := missingSchemaRecords(root, inventoryText, generatedTypes)
	if err != nil {
		return err
	}

	if len(missing) != 0 {
		return inventoryError("schema components without generated Go models: %s", strings.Join(missing, ", "))
	}

	err = checkCompatibilityModelRecords(root, generatedTypes, inventoryText)
	if err != nil {
		return err
	}

	return checkGeneratorRecords(root, inventoryText)
}

func missingSchemaRecords(root, inventoryText string, generatedTypes map[string]bool) ([]string, error) {
	var missing []string

	for _, source := range generatedModelSources() {
		document, err := readSchemaDocument(filepath.Join(root, filepath.FromSlash(source.schemaPath)))
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", source.schemaPath, err)
		}

		sourceTypes, err := readGeneratedTypeSet(filepath.Join(root, filepath.FromSlash(source.outputPath)))
		if err != nil {
			return nil, fmt.Errorf("read generated model output %s: %w", source.outputPath, err)
		}

		schemaNames := make([]string, 0, len(document.Components.Schemas))
		for name := range document.Components.Schemas {
			schemaNames = append(schemaNames, name)
		}

		sort.Strings(schemaNames)

		for _, name := range schemaNames {
			if !generatedTypes[name] || !sourceTypes[name] {
				missing = append(missing, source.schemaPath+":"+name)

				continue
			}

			if !inventoryHasSchemaRecord(inventoryText, source, name) {
				return nil, inventoryError("%s: %s `%s`", source.schemaPath, generatedTypeUndocumented, name)
			}
		}
	}

	return missing, nil
}

func checkCompatibilityModelRecords(root string, generatedTypes map[string]bool, inventoryText string) error {
	compatibilityTypes := []string{"SendCloudRequestParams", "SendCloudRequestJSONRequestBody"}
	compatibilityOutputs := compatibilityModelOutputs()
	compatibilityPath := filepath.Join(root, filepath.FromSlash(compatibilityOutputs[0]))

	compatibilityModels, err := readGeneratedTypeSet(compatibilityPath)
	if err != nil {
		return fmt.Errorf("read compatibility output: %w", err)
	}

	for _, name := range compatibilityTypes {
		if !generatedTypes[name] || !compatibilityModels[name] || !strings.Contains(inventoryText, "`"+name+"`") {
			return inventoryError("legacy generated compatibility type %s is missing from models or inventory", name)
		}
	}

	return nil
}

func checkGeneratorRecords(root, inventoryText string) error {
	for _, source := range generatedModelSources() {
		configPath := filepath.Join(root, filepath.FromSlash(source.configPath))

		_, err := os.Stat(configPath)
		if err != nil {
			return inventoryError("generator config %s is missing for %s", source.configPath, source.schemaPath)
		}

		if !strings.Contains(inventoryText, source.outputPath) || !strings.Contains(inventoryText, source.command) {
			return inventoryError("wire-model inventory lacks schema, output, and generator mapping for %s", source.schemaPath)
		}

		if source.configPath == cliGeneratorConfigPath {
			err := checkCLIImportsTemplate(root)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func checkCLIImportsTemplate(root string) error {
	configPath := filepath.Join(root, filepath.FromSlash(cliGeneratorConfigPath))
	//nolint:gosec // The CLI generator config path is fixed by the generated-model registry.
	configuration, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read CLI generator config: %w", err)
	}

	err = validateCLIImportsTemplateConfig(configuration)
	if err != nil {
		return err
	}

	templatePath := filepath.Join(root, filepath.FromSlash(cliImportsTemplatePath))
	//nolint:gosec // The template path is fixed by the generated-model registry.
	template, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read CLI generator import template: %w", err)
	}

	return validateCLIImportsTemplateSource(string(template))
}

func validateCLIImportsTemplateConfig(configuration []byte) error {
	var config map[string]any

	err := yaml.Unmarshal(configuration, &config)
	if err != nil {
		return fmt.Errorf("parse CLI generator config: %w", err)
	}

	outputOptions, validOutputOptions := config["output-options"].(map[string]any)
	if !validOutputOptions {
		return inventoryError("CLI generator config is missing output-options")
	}

	userTemplates, validUserTemplates := outputOptions["user-templates"].(map[string]any)

	templatePath, validTemplatePath := userTemplates["imports.tmpl"].(string)
	if !validUserTemplates || !validTemplatePath || len(userTemplates) != 1 || templatePath != cliImportsTemplatePath {
		return inventoryError(
			"CLI generator config must bind imports.tmpl to %s",
			cliImportsTemplatePath,
		)
	}

	return nil
}

func validateCLIImportsTemplateSource(template string) error {
	requiredSections := []string{
		"// Code generated by {{.ModuleName}} version {{.Version}} DO NOT EDIT.",
		"package {{.PackageName}}",
		"import (",
		"{{- range .RouterImports}}",
		"{{- range .ExternalImports}}",
		"{{- range .AdditionalImports}}",
	}

	for _, section := range requiredSections {
		if !strings.Contains(template, section) {
			return inventoryError("CLI imports.tmpl is missing native generator section %q", section)
		}
	}

	if strings.Contains(template, "// Package {{.PackageName}}") {
		return inventoryError("CLI imports.tmpl must omit the duplicate generated package comment")
	}

	return nil
}

func inventoryHasSchemaRecord(inventory string, source generatedModelSource, name string) bool {
	generatedType := "pkg/dependencymodels." + name
	if strings.HasPrefix(source.outputPath, "pkg/tplinkmodels/") {
		generatedType = "pkg/tplinkmodels." + name
	}

	if strings.HasPrefix(source.outputPath, "cmd/go-tplink/") {
		generatedType = "main." + name
	}

	for line := range strings.SplitSeq(inventory, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) != 7 || strings.TrimSpace(fields[1]) != "`"+source.schemaPath+"#"+name+"`" {
			continue
		}

		return strings.TrimSpace(fields[2]) == "`"+generatedType+"`" &&
			strings.TrimSpace(fields[3]) == "`"+source.outputPath+"`" &&
			strings.Contains(fields[4], source.command) && strings.TrimSpace(fields[5]) != ""
	}

	return false
}

func readSchemaDocument(path string) (schemaDocument, error) {
	//nolint:gosec // Paths are formed from the fixed schema registry under the repository root.
	data, err := os.ReadFile(path)
	if err != nil {
		return schemaDocument{}, fmt.Errorf("read schema document %s: %w", path, err)
	}

	var document schemaDocument

	err = yaml.Unmarshal(data, &document)
	if err != nil {
		return schemaDocument{}, fmt.Errorf("parse schema document %s: %w", path, err)
	}

	if len(document.Components.Schemas) == 0 {
		return schemaDocument{}, inventoryError("no schema components found in %s", path)
	}

	return document, nil
}
