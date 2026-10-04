// Command wireconstants generates constants and model compatibility aliases from the OpenAPI contract.
package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type constant struct {
	value  string
	compat bool
}

type generator struct {
	constants map[string]constant
}

var errInvalidWireSchema = errors.New("invalid OpenAPI wire schema")

const (
	minimumPassthroughResultVariants = 3
	generatedDirectoryMode           = fs.FileMode(0o750)
	generatedFileMode                = fs.FileMode(0o600)
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	const rootSchema = "api/openapi.yaml"

	data, err := os.ReadFile(rootSchema)
	if err != nil {
		return fmt.Errorf("read %s: %w", rootSchema, err)
	}

	var schema map[string]any

	err = yaml.Unmarshal(data, &schema)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rootSchema, err)
	}

	gen := generator{constants: make(map[string]constant)}

	err = gen.collectSchemaMetadata(schema)
	if err != nil {
		return err
	}

	return gen.writeOutputs()
}

func (gen *generator) collectSchemaMetadata(schema map[string]any) error {
	collectors := []func(map[string]any) error{
		gen.collectServers,
		gen.collectRoutes,
		gen.collectParameters,
		gen.collectRequestMedia,
		gen.collectSchemaProperties,
	}
	for _, collect := range collectors {
		err := collect(schema)
		if err != nil {
			return err
		}
	}

	err := gen.validateRequiredConstants()
	if err != nil {
		return err
	}

	return validateWireSchemaShape(schema)
}

func (gen *generator) writeOutputs() error {
	internal, compat, err := gen.renderConstants()
	if err != nil {
		return err
	}

	err = writeFormatted("pkg/dependencymodels/wire_constants.gen.go", internal)
	if err != nil {
		return err
	}

	err = writeFormatted("pkg/tplink/constants.gen.go", compat)
	if err != nil {
		return err
	}

	err = generateModelAliases()
	if err != nil {
		return err
	}

	return nil
}

func (gen *generator) collectServers(schema map[string]any) error {
	servers := list(schema["servers"])
	defaultName := ""
	defaultTarget := ""

	for _, rawServer := range servers {
		server := object(rawServer)
		name := stringValue(server["x-go-compat-constant-name"])
		url := stringValue(server["url"])

		if name != "" {
			if url == "" {
				return invalidSchema("server %s compatibility constant has no URL", name)
			}

			err := gen.add(name, strconv.Quote(url), true)
			if err != nil {
				return err
			}
		}

		if value := stringValue(server["x-go-default-base-url-constant-name"]); value != "" {
			if name == "" {
				return invalidSchema("default server constant %s must also identify its compatibility constant", value)
			}

			defaultName = value
			defaultTarget = name
		}
	}

	if defaultName == "" {
		return invalidSchema("no server declares x-go-default-base-url-constant-name")
	}

	return gen.add(defaultName, defaultTarget, true)
}

func (gen *generator) collectRoutes(schema map[string]any) error {
	paths := object(schema["paths"])
	for path, rawPath := range paths {
		for method, rawOperation := range object(rawPath) {
			operation := object(rawOperation)
			methodName := stringValue(operation["x-go-wire-method-constant-name"])
			pathName := stringValue(operation["x-go-wire-path-constant-name"])

			if (methodName == "") != (pathName == "") {
				return invalidSchema("%s %s must declare both method and path constant names", strings.ToUpper(method), path)
			}

			if methodName == "" {
				continue
			}

			err := gen.add(methodName, strconv.Quote(strings.ToUpper(method)), false)
			if err != nil {
				return err
			}

			err = gen.add(pathName, strconv.Quote(path), false)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (gen *generator) collectParameters(schema map[string]any) error {
	paths := object(schema["paths"])
	for _, rawPath := range paths {
		for _, rawOperation := range object(rawPath) {
			operation := object(rawOperation)
			for _, rawParameter := range list(operation["parameters"]) {
				parameter := object(rawParameter)

				name := stringValue(parameter["x-go-wire-constant-name"])
				if name == "" {
					continue
				}

				wireName := stringValue(parameter["name"])
				if wireName == "" {
					return invalidSchema("parameter constant %s has no parameter name", name)
				}

				err := gen.add(name, strconv.Quote(wireName), false)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (gen *generator) collectRequestMedia(schema map[string]any) error {
	paths := object(schema["paths"])
	for path, rawPath := range paths {
		for method, rawOperation := range object(rawPath) {
			operation := object(rawOperation)
			requestBody := object(operation["requestBody"])
			headerName := stringValue(requestBody["x-go-wire-content-type-header-constant-name"])

			if headerName == "" {
				continue
			}

			headerValue := stringValue(requestBody["x-go-wire-content-type-header"])
			if headerValue == "" {
				return invalidSchema("%s %s content-type header constant has no wire header name", strings.ToUpper(method), path)
			}

			content := object(requestBody["content"])
			if len(content) != 1 {
				return invalidSchema(
					"%s %s content-type header metadata expects exactly one content type",
					strings.ToUpper(method),
					path,
				)
			}

			for mediaType, rawMediaType := range content {
				mediaMetadata := object(rawMediaType)
				mediaConstant := stringValue(mediaMetadata["x-go-wire-constant-name"])

				if mediaConstant == "" {
					return invalidSchema(
						"%s %s media type %s has no generated constant name",
						strings.ToUpper(method),
						path,
						mediaType,
					)
				}

				err := gen.add(mediaConstant, strconv.Quote(mediaType), false)
				if err != nil {
					return err
				}

				err = gen.add(headerName, strconv.Quote(headerValue), false)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (gen *generator) collectSchemaProperties(schema map[string]any) error {
	components := object(schema["components"])
	for _, rawSchema := range object(components["schemas"]) {
		err := gen.collectObjectProperties(rawSchema)
		if err != nil {
			return err
		}
	}

	return nil
}

func (gen *generator) collectObjectProperties(raw any) error {
	model := object(raw)
	properties := object(model["properties"])

	for propertyName, rawProperty := range properties {
		err := gen.collectProperty(propertyName, rawProperty)
		if err != nil {
			return err
		}
	}

	err := collectEnumMetadata(gen, model)
	if err != nil {
		return err
	}

	return nil
}

func (gen *generator) collectProperty(propertyName string, rawProperty any) error {
	property := object(rawProperty)
	for _, definition := range []struct {
		metadata string
		compat   bool
	}{
		{metadata: "x-go-compat-constant-name", compat: true},
		{metadata: "x-go-wire-constant-name", compat: false},
	} {
		err := gen.collectNamedPropertyConstant(propertyName, property, definition.metadata, definition.compat)
		if err != nil {
			return err
		}
	}

	err := collectEnumMetadata(gen, property)
	if err != nil {
		return fmt.Errorf("property %s: %w", propertyName, err)
	}

	err = gen.collectObjectProperties(property)
	if err != nil {
		return fmt.Errorf("property %s: %w", propertyName, err)
	}

	return nil
}

func (gen *generator) collectNamedPropertyConstant(
	propertyName string,
	property map[string]any,
	metadata string,
	compat bool,
) error {
	name := stringValue(property[metadata])
	if name == "" {
		return nil
	}

	value, err := propertyConstantValue(propertyName, property)
	if err != nil {
		return fmt.Errorf("property %s: %w", propertyName, err)
	}

	return gen.add(name, value, compat)
}

func propertyConstantValue(propertyName string, property map[string]any) (string, error) {
	if property["x-go-wire-constant-value"] != true {
		return strconv.Quote(propertyName), nil
	}

	for _, enumKey := range []string{"enum", "x-extensible-enum"} {
		values := list(property[enumKey])
		if len(values) > 1 {
			return "", invalidSchema("constant metadata must select one value, found %d values in %s", len(values), enumKey)
		}

		if len(values) == 1 {
			return goLiteral(values[0])
		}
	}

	return "", invalidSchema("x-go-wire-constant-value requires exactly one enum value")
}

func collectEnumMetadata(gen *generator, schema map[string]any) error {
	enumValues := make(map[string]struct{})

	for _, enumKey := range []string{"enum", "x-extensible-enum"} {
		for _, rawValue := range list(schema[enumKey]) {
			value, err := goLiteral(rawValue)
			if err != nil {
				return fmt.Errorf("%s: %w", enumKey, err)
			}

			enumValues[value] = struct{}{}
		}
	}

	for _, metadata := range []struct {
		key    string
		compat bool
	}{
		{key: "x-go-compat-constant-names", compat: true},
		{key: "x-go-wire-constant-names", compat: false},
	} {
		for _, rawEntry := range list(schema[metadata.key]) {
			entry := object(rawEntry)
			name := stringValue(entry["name"])

			if name == "" {
				return invalidSchema("%s entry has no name", metadata.key)
			}

			value, err := goLiteral(entry["value"])
			if err != nil {
				return fmt.Errorf("%s %s: %w", metadata.key, name, err)
			}

			if _, found := enumValues[value]; !found {
				return invalidSchema("%s %s selects %s, which is absent from enum and x-extensible-enum", metadata.key, name, value)
			}

			err = gen.add(name, value, metadata.compat)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (gen *generator) add(name, value string, compat bool) error {
	if name == "" {
		return invalidSchema("constant name must not be empty")
	}

	if prior, found := gen.constants[name]; found {
		if prior.value != value {
			return invalidSchema("constant %s has conflicting schema values %s and %s", name, prior.value, value)
		}

		prior.compat = prior.compat || compat

		gen.constants[name] = prior

		return nil
	}

	gen.constants[name] = constant{value: value, compat: compat}

	return nil
}

func (gen *generator) renderConstants() ([]byte, []byte, error) {
	names := make([]string, 0, len(gen.constants))
	compatNames := make([]string, 0, len(gen.constants))

	for name, value := range gen.constants {
		names = append(names, name)

		if value.compat {
			compatNames = append(compatNames, name)
		}
	}

	sort.Strings(names)
	sort.Strings(compatNames)

	var internal strings.Builder

	internal.WriteString("// Code generated by tools/wireconstants; DO NOT EDIT.\n\npackage dependencymodels\n\n")

	for _, name := range names {
		fmt.Fprintf(
			&internal,
			"// %s is a schema-derived wire value.\nconst %s = %s\n\n",
			name,
			name,
			gen.constants[name].value,
		)
	}

	var compat strings.Builder

	compat.WriteString(
		"// Code generated by tools/wireconstants; DO NOT EDIT.\n\npackage tplink\n\n" +
			"import \"github.com/portpowered/go-tplink/pkg/dependencymodels\"\n\n",
	)

	for _, name := range compatNames {
		fmt.Fprintf(
			&compat,
			"// %s is retained for source compatibility.\nconst %s = dependencymodels.%s\n\n",
			name,
			name,
			name,
		)
	}

	internalOutput, err := format.Source([]byte(internal.String()))
	if err != nil {
		return nil, nil, fmt.Errorf("format internal constants: %w", err)
	}

	compatOutput, err := format.Source([]byte(compat.String()))
	if err != nil {
		return nil, nil, fmt.Errorf("format compatibility constants: %w", err)
	}

	return internalOutput, compatOutput, nil
}

func (gen *generator) validateRequiredConstants() error {
	required := []string{
		"AppType", "BaseURLAsiaPacific", "BaseURLEU", "BaseURLGlobal", "BaseURLUS", "DefaultBaseURL",
		"CloudErrorCodeAuthentication", "CloudErrorCodeOK", "CloudErrorCodeParameter",
		"CloudErrorCodeRateLimited", "CloudErrorCodeTokenExpired",
		"CloudRequestContentType", "CloudRequestContentTypeHeader", "CloudRequestHTTPMethod", "CloudRequestPath",
		"CmdGetLightState", "CmdGetSysInfo", "CmdReboot", "CmdSetDevAlias", "CmdSetRelayState", "CmdTransitionLightState",
		"DeviceCapabilityDisabled", "DeviceCapabilityEnabled", "DeviceErrorOK", "DeviceErrorUnsupported",
		"DeviceStateOff", "DeviceStateOn",
		"DeviceTypeRangeExtenderPlug", "DeviceTypeSmartBulb", "DeviceTypeSmartPlug",
		"MethodGetDeviceList", "MethodLogin", "MethodPassthrough", "NamespaceLightingService",
		"NamespaceSystem", "TerminalUUID", "TokenQueryKey",
	}
	for _, name := range required {
		if _, found := gen.constants[name]; !found {
			return invalidSchema("required generated wire constant %s has no schema definition", name)
		}
	}

	return nil
}

func validateWireSchemaShape(schema map[string]any) error {
	schemas := object(object(object(schema["components"])["schemas"]))

	requiredComponents := []string{
		"CloudRequest", "LoginCloudRequest", "GetDeviceListCloudRequest", "PassthroughCloudRequest",
		"LoginParams", "PassthroughParams",
		"PassthroughCommand", "SystemGetSysInfoCommand", "SystemSetRelayStateCommand",
		"SystemRebootCommand", "SystemSetDevAliasCommand",
		"LightingGetLightStateCommand", "LightingTransitionLightStateCommand", "LightTransitionState",
		"CloudResponseEnvelope",
		"CloudResponse", "LoginResponse", "DeviceListResponse", "PassthroughResponse", "Device", "PassthroughCommandResult",
		"SystemCommandResult", "LightingCommandResult", "CommandAcknowledgement",
		"DeviceCommandError", "SysInfo", "LightState",
	}
	for _, name := range requiredComponents {
		if _, found := schemas[name]; !found {
			return invalidSchema("required generated wire schema component %s is missing", name)
		}
	}

	lightState := object(schemas["LightTransitionState"])
	if lightState["additionalProperties"] != true {
		return invalidSchema("LightTransitionState must retain additionalProperties: true for caller-provided state fields")
	}

	commandResult := object(schemas["PassthroughCommandResult"])
	if len(list(commandResult["anyOf"])) < minimumPassthroughResultVariants {
		return invalidSchema("PassthroughCommandResult must include system, lighting, and device-error variants")
	}

	return nil
}

func generateModelAliases() error {
	const sourcePath = "pkg/dependencymodels/models.gen.go"

	source, err := parser.ParseFile(token.NewFileSet(), sourcePath, nil, 0)
	if err != nil {
		return fmt.Errorf("parse generated models %s: %w", sourcePath, err)
	}

	types, constants := collectModelSymbols(source)

	var output strings.Builder

	output.WriteString(
		"// Code generated by tools/wireconstants; DO NOT EDIT.\n\n" +
			"// Package generatedwire preserves the historical generated model import path.\n" +
			"package generatedwire\n\n" +
			"import \"github.com/portpowered/go-tplink/pkg/dependencymodels\"\n\n",
	)
	writeTypeAliases(&output, types)
	writeConstantAliases(&output, constants)

	formatted, err := format.Source([]byte(output.String()))
	if err != nil {
		return fmt.Errorf("format generated model aliases: %w", err)
	}

	return writeFormatted("pkg/generatedwire/compat.gen.go", formatted)
}

func collectModelSymbols(source *ast.File) ([]string, []string) {
	types := make([]string, 0)
	constants := make([]string, 0)

	for _, declaration := range source.Decls {
		genDecl, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}

		for _, rawSpec := range genDecl.Specs {
			switch spec := rawSpec.(type) {
			case *ast.TypeSpec:
				if ast.IsExported(spec.Name.Name) {
					types = append(types, spec.Name.Name)
				}
			case *ast.ValueSpec:
				if genDecl.Tok != token.CONST {
					continue
				}

				for _, name := range spec.Names {
					if ast.IsExported(name.Name) {
						constants = append(constants, name.Name)
					}
				}
			}
		}
	}

	sort.Strings(types)
	sort.Strings(constants)

	return types, constants
}

func writeTypeAliases(output *strings.Builder, types []string) {
	for _, name := range types {
		fmt.Fprintf(
			output,
			"// %s is a compatibility alias for dependencymodels.%s.\ntype %s = dependencymodels.%s\n\n",
			name,
			name,
			name,
			name,
		)
	}
}

func writeConstantAliases(output *strings.Builder, constants []string) {
	for _, name := range constants {
		fmt.Fprintf(
			output,
			"// %s is a compatibility alias for dependencymodels.%s.\nconst %s = dependencymodels.%s\n\n",
			name,
			name,
			name,
			name,
		)
	}
}

func writeFormatted(path string, content []byte) error {
	err := os.MkdirAll(filepath.Dir(path), generatedDirectoryMode)
	if err != nil {
		return fmt.Errorf("create generated directory for %s: %w", path, err)
	}

	err = os.WriteFile(path, content, generatedFileMode)
	if err != nil {
		return fmt.Errorf("write generated file %s: %w", path, err)
	}

	return nil
}

func goLiteral(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return strconv.Quote(typed), nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case uint64:
		return strconv.FormatUint(typed, 10), nil
	case bool:
		if typed {
			return "true", nil
		}

		return "false", nil
	default:
		return "", invalidSchema("unsupported constant value %T (%v)", value, value)
	}
}

func invalidSchema(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errInvalidWireSchema, fmt.Sprintf(format, args...))
}

func object(value any) map[string]any {
	if result, ok := value.(map[string]any); ok {
		return result
	}

	return map[string]any{}
}

func list(value any) []any {
	if result, ok := value.([]any); ok {
		return result
	}

	return nil
}

func stringValue(value any) string {
	result, _ := value.(string)

	return result
}
