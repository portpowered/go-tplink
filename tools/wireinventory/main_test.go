package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const loginCloudRequestModel = "LoginCloudRequest"

func TestProductionWireInventoryPasses(t *testing.T) {
	t.Parallel()

	err := checkRepository("../..")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCLIImportsTemplateConfigRejectsDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configuration string
		wantError     bool
	}{
		{
			name: "registered imports template",
			configuration: `output-options:
  user-templates:
    imports.tmpl: cmd/go-tplink/templates/imports.tmpl
`,
			wantError: false,
		},
		{
			name: "missing template mapping",
			configuration: `output-options:
  skip-prune: true
`,
			wantError: true,
		},
		{
			name: "different template mapping",
			configuration: `output-options:
  user-templates:
    imports.tmpl: another/imports.tmpl
`,
			wantError: true,
		},
		{
			name: "extra template mapping",
			configuration: `output-options:
  user-templates:
    imports.tmpl: cmd/go-tplink/templates/imports.tmpl
    typedef.tmpl: another/typedef.tmpl
`,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := validateCLIImportsTemplateConfig([]byte(test.configuration))
			if test.wantError && err == nil {
				t.Fatal("validateCLIImportsTemplateConfig() accepted template drift")
			}

			if !test.wantError && err != nil {
				t.Fatalf("validateCLIImportsTemplateConfig() rejected valid template config: %v", err)
			}
		})
	}
}

func TestCLIImportsTemplateSourceKeepsNativeSectionsWithoutPackageComment(t *testing.T) {
	t.Parallel()

	err := checkCLIImportsTemplate("../..")
	if err != nil {
		t.Fatal(err)
	}
}

func TestMapPayloadConstructionNegatives(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"nested literals": `package fixture
func build() { _ = map[string]any{"system": map[string]any{"command": nil}} }
`,
		"map alias": `package fixture
type Payload = map[string]any
func build() { _ = Payload{"system": true} }
`,
		"map allocation": `package fixture
type Payload map[string]any
func build() { payload := (make)(Payload); payload["system"] = true }
`,
		"map pointer mutation": `package fixture
type Payload map[string]any
func build() { payload := new(Payload); (*payload)["system"] = true }
`,
		"generated map alias literal": `package fixture
func build() { _ = WirePayload{"system": true} }
`,
		"generated map alias allocation": `package fixture
func build() { _ = (make)(WirePayload) }
`,
		"generated map alias pointer": `package fixture
func build() { _ = new(WirePayload) }
`,
		"generated map alias conversion": `package fixture
func build(payload WirePayload) { _ = WirePayload(payload) }
`,
		"parenthesized generated map alias conversion": `package fixture
func build(payload WirePayload) { _ = (WirePayload)(payload) }
`,
		"generated map variable": `package fixture
func build() { var payload WirePayload; payload["system"] = true }
`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			generatedMaps := map[string]bool{"WirePayload": true}

			err := checkSource(source, nil, generatedMaps, nil)
			if err == nil || !strings.Contains(err.Error(), "map") {
				t.Fatalf("checkSource() error = %v, want map construction rejection", err)
			}
		})
	}
}

func TestHandwrittenJSONModelNegatives(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"unreferenced exported model": `package fixture
type Forgotten struct { State string "json:\"state\"" }
`,
		"anonymous nested object": `package fixture
func build() { _ = struct { State struct { On int "json:\"on\"" } "json:\"state\"" }{} }
`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkSource(source, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), "JSON") {
				t.Fatalf("checkSource() error = %v, want handwritten JSON model rejection", err)
			}
		})
	}
}

func callerOpenStateMutationFixtureGroup1() map[string]string {
	return map[string]string{
		"direct": `package fixture
func SetLightState(input SetLightStateRequest) { input.State["invented"] = true }
`,
		"map alias": `package fixture
func SetLightState(input SetLightStateRequest) { state := input.State; state["invented"] = true }
`,
		"conversion alias": `package fixture
func SetLightState(input SetLightStateRequest) { state := (LightStateParams)(input.State); state["invented"] = true }
`,
		"request alias": `package fixture
func SetLightState(input SetLightStateRequest) { req := input; state := req.State; state["invented"] = true }
`,
		"closure alias": `package fixture
func SetLightState(input SetLightStateRequest) { state := input.State; func() { state["invented"] = true }() }
`,
		"delete": `package fixture
func SetLightState(input SetLightStateRequest) { delete(input.State, "invented") }
`,
		"clear alias": `package fixture
func SetLightState(input SetLightStateRequest) { state := input.State; clear(state) }
`,
		"helper escape": `package fixture
func SetLightState(input SetLightStateRequest) { state := input.State; send(state) }
`,
	}
}

func callerOpenStateMutationFixtureGroup2() map[string]string {
	return map[string]string{
		"return escape": `package fixture
func SetLightState(input SetLightStateRequest) LightStateParams { return input.State }
`,
		"aggregate escape": `package fixture
func SetLightState(input SetLightStateRequest) { holder.Value = input.State }
`,
		"pointer escape": `package fixture
func SetLightState(input SetLightStateRequest) { pointer := &input.State; send(pointer) }
`,
		"pointer index mutation": `package fixture
func SetLightState(input SetLightStateRequest) { pointer := &input.State; (*pointer)["invented"] = true }
`,
		"increment map entry": `package fixture
func SetLightState(input SetLightStateRequest) { input.State["invented"]++ }
`,
		"package state escape": `package fixture
var saved any
func SetLightState(input SetLightStateRequest) { saved = input.State }
`,
		"renamed request parameter and shadow": `package fixture
func SetLightState(input SetLightStateRequest) {
  { input := OtherRequest{}; _ = input }
  input.State["invented"] = true
}
`,
	}
}

func TestCallerOpenStateMutationNegatives(t *testing.T) {
	t.Parallel()

	fixtures := make(map[string]string)
	maps.Copy(fixtures, callerOpenStateMutationFixtureGroup1())
	maps.Copy(fixtures, callerOpenStateMutationFixtureGroup2())

	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkSource(source, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), "caller-open") {
				t.Fatalf("checkSource() error = %v, want caller-map mutation rejection", err)
			}
		})
	}
}

func TestCallerOpenStateForwardingIsAllowed(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func SetLightState(request SetLightStateRequest) {
  var state dependencymodels.LightTransitionState
  state.AdditionalProperties = request.State
}
`

	err := checkSource(source, nil, nil, nil)
	if err != nil {
		t.Fatalf("checkSource() rejected caller-open state forwarding: %v", err)
	}
}

func TestRawSchemaWireStringIsRejected(t *testing.T) {
	t.Parallel()

	const source = `package fixture
const namespace = "system"
`

	allowedValues := map[string]bool{"system": true}

	err := checkSource(source, allowedValues, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "raw schema-defined wire string") {
		t.Fatalf("checkSource() error = %v, want raw wire string rejection", err)
	}
}

func TestGeneratedModelFixedScalarNegatives(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"known wire string literal": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func build() { _ = dependencymodels.LoginCloudRequest{Method: "login"} }
`,
		"novel numeric field mutation": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func build() { var command dependencymodels.SystemRebootCommand; command.System.Reboot.Delay = 7 }
`,
		"const string alias": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
const invented = "invented"
func build() { var command dependencymodels.SystemSetDevAliasCommand; command.System.SetDevAlias.Alias = invented }
`,
		"local numeric alias": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func build() { var command dependencymodels.SystemRebootCommand; value := 9; command.System.Reboot.Delay = value }
`,
		"wrapped string alias": `package fixture
import (
  "fmt"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func build() {
  var command dependencymodels.SystemSetDevAliasCommand
  value := fmt.Sprintf("%s", "invented")
  command.System.SetDevAlias.Alias = value
}
`,
		"computed numeric alias": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func build() { var command dependencymodels.SystemRebootCommand; value := 3 + 4; command.System.Reboot.Delay = value }
`,
		"boolean local alias": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func build() { var device dependencymodels.Device; enabled := !false; device.IsSameRegion = &enabled }
`,
		"boolean in generated nested composite": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func build() { _ = dependencymodels.Device{IsSameRegion: func() *bool { value := true; return &value }()} }
`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			generatedTypes := map[string]bool{
				loginCloudRequestModel: true, "SystemRebootCommand": true,
				"SystemSetDevAliasCommand": true, "Device": true,
			}

			err := checkSource(source, nil, nil, generatedTypes)
			if err == nil || !strings.Contains(err.Error(), "fixed scalar") {
				t.Fatalf("checkSource() error = %v, want generated scalar rejection", err)
			}
		})
	}
}

func TestGeneratedModelCallerValuesAreAllowed(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func build(alias string, delay int) {
  var command dependencymodels.SystemSetDevAliasCommand
  command.System.SetDevAlias.Alias = alias
  var reboot dependencymodels.SystemRebootCommand
  reboot.System.Reboot.Delay = dependencymodels.SystemRebootCommandSystemRebootDelay(delay)
}
`

	err := checkSource(source, nil, nil, map[string]bool{
		"SystemSetDevAliasCommand":             true,
		"Device":                               true,
		"SystemRebootCommand":                  true,
		"SystemRebootCommandSystemRebootDelay": true,
	})
	if err != nil {
		t.Fatalf("checkSource() rejected caller values: %v", err)
	}
}

func TestGeneratedScalarHelperResultsRejectFixedValues(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"named result with bare return": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func fixed() (result dependencymodels.LoginCloudRequestMethod) { result = "invented"; return }
func build() { _ = dependencymodels.LoginCloudRequest{Method: fixed()} }
`,
		"chained named result": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func fixed() (result dependencymodels.LoginCloudRequestMethod) { result = "invented"; return }
func wrapped() (result dependencymodels.LoginCloudRequestMethod) { result = fixed(); return }
func build() { _ = dependencymodels.LoginCloudRequest{Method: wrapped()} }
`,
		"function value alias": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func fixed() (result dependencymodels.LoginCloudRequestMethod) { result = "invented"; return }
func build() { helper := fixed; _ = dependencymodels.LoginCloudRequest{Method: helper()} }
`,
		"returned callback": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func fixed() (result dependencymodels.LoginCloudRequestMethod) { result = "invented"; return }
func callback() func() dependencymodels.LoginCloudRequestMethod { return fixed }
func build() { _ = dependencymodels.LoginCloudRequest{Method: callback()()} }
`,
		"returned callback through named result": `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func fixed() (result dependencymodels.LoginCloudRequestMethod) { result = "invented"; return }
func callback() (result func() dependencymodels.LoginCloudRequestMethod) { result = fixed; return }
func build() { _ = dependencymodels.LoginCloudRequest{Method: callback()()} }
`,
	}

	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkSource(source, nil, nil, map[string]bool{loginCloudRequestModel: true})
			if err == nil || !strings.Contains(err.Error(), "fixed scalar") {
				t.Fatalf("checkSource() error = %v, want fixed helper result rejection", err)
			}
		})
	}
}

func TestGeneratedScalarCallerValueHelpersRemainAllowed(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import "github.com/portpowered/go-tplink/pkg/dependencymodels"
func passthrough(value dependencymodels.LoginCloudRequestMethod) dependencymodels.LoginCloudRequestMethod {
  return value
}
func callback(value dependencymodels.LoginCloudRequestMethod) func() dependencymodels.LoginCloudRequestMethod {
  return func() dependencymodels.LoginCloudRequestMethod { return value }
}
func build(value dependencymodels.LoginCloudRequestMethod) {
  _ = dependencymodels.LoginCloudRequest{Method: passthrough(value)}
  _ = dependencymodels.LoginCloudRequest{Method: callback(value)()}
}
`

	err := checkSource(source, nil, nil, map[string]bool{loginCloudRequestModel: true})
	if err != nil {
		t.Fatalf("checkSource() rejected caller-provided helper values: %v", err)
	}
}

func TestGeneratedQueryAndHeaderKeyNegatives(t *testing.T) {
	t.Parallel()

	assertRejectedSourceFixtures(t, queryAndHeaderKeyNegativeFixtures())
}

func queryAndHeaderKeyNegativeFixtures() map[string]string {
	return map[string]string{
		"raw query key": `package fixture
import "net/url"
func build() { query := make(url.Values); query.Set("token", "secret") }
`,
		"aliased raw query key": `package fixture
import "net/url"
func build() { query := make(url.Values); params := query; params.Add("invented", "value") }
`,
		"direct header map write": `package fixture
import "net/http"
func build(request *http.Request) { request.Header["X-Invented"] = []string{"value"} }
`,
		"query method value escape": `package fixture
import "net/url"
func build() { query := make(url.Values); set := query.Set; _ = set }
`,
		"new query map through pointer alias": `package fixture
import "net/url"
func build() { query := new(url.Values); (*query).Set("raw", "value") }
`,
		"query map composite literal": `package fixture
import "net/url"
func build() { _ = url.Values{"raw": []string{"value"}} }
`,
		"named query alias allocation": `package fixture
import "net/url"
type Query = url.Values
func build() { query := new(Query); (*query)["raw"] = []string{"value"} }
`,
		"defined query alias literal": `package fixture
import "net/url"
type Query url.Values
func build() { query := Query{"raw": []string{"value"}}; _ = query }
`,
		"parenthesized query receiver": `package fixture
import "net/url"
func build() { query := make(url.Values); (query).Set("raw", "value") }
`,
		"parenthesized query index": `package fixture
import "net/url"
func build() { query := make(url.Values); (query)["raw"] = []string{"value"} }
`,
		"header alias allocation": `package fixture
import "net/http"
type Header = http.Header
func build() { header := new(Header); (*header)["Cookie"] = []string{"value"} }
`,
		"header composite literal": `package fixture
import "net/http"
func build() { _ = http.Header{"Cookie": []string{"value"}} }
`,
	}
}

func assertRejectedSourceFixtures(t *testing.T, fixtures map[string]string) {
	t.Helper()

	const message = "query and header"

	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkSource(source, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), message) {
				t.Fatalf("checkSource() error = %v, want rejection containing %q", err, message)
			}
		})
	}
}

func TestSchemaKeyedMapConversionsRejectUntrustedSources(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import "net/url"
func build(raw string) { query := url.Values(raw); _ = query }
`

	err := checkSource(source, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "schema-bound source") {
		t.Fatalf("checkSource() error = %v, want untrusted map conversion rejection", err)
	}
}

func TestCallerProvidedQueryMapMayPassThroughNamedResult(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import "net/url"
func passThrough(input url.Values) (result url.Values) { result = input; return }
func build(input url.Values) { _ = passThrough(input) }
`

	err := checkSource(source, nil, nil, nil)
	if err != nil {
		t.Fatalf("checkSource() rejected a caller-provided query map: %v", err)
	}
}

func TestSchemaKeyedMapsRejectEscapesAndAggregateStorage(t *testing.T) {
	t.Parallel()

	assertRejectedSourceFixtures(t, schemaKeyedMapEscapeFixtures())
	assertRejectedSourceFixtures(t, schemaKeyedMapAggregateFixtures())
	assertRejectedSourceFixtures(t, schemaKeyedMapReaderFixtures())
}

func schemaKeyedMapEscapeFixtures() map[string]string {
	return map[string]string{
		"helper argument": `package fixture
import "net/url"
func build() { query := make(url.Values); mutate(query) }
`,
		"nested helper aggregate": `package fixture
import "net/url"
func build() { query := make(url.Values); mutate(struct{ Values url.Values }{query}) }
`,
		"return escape": `package fixture
import "net/url"
func build() url.Values { query := make(url.Values); return query }
`,
		"named result helper escape": `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func generated() (result url.Values) {
  result = make(url.Values)
  result.Set(dependencymodels.TokenQueryKey, "token")
  return
}
func mutate(values url.Values) { values.Set("unregistered", "x") }
func build() { mutate(generated()) }
`,
		"chained named result callback escape": `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func generated() (result url.Values) {
  result = make(url.Values)
  result.Set(dependencymodels.TokenQueryKey, "token")
  return
}
func callback() func() url.Values { return generated }
func mutate(values url.Values) { values.Set("unregistered", "x") }
func build() { helper := callback(); mutate(helper()) }
`,
	}
}

func schemaKeyedMapAggregateFixtures() map[string]string {
	return map[string]string{
		"struct storage": `package fixture
import "net/url"
func build() { query := make(url.Values); holder := struct{ Values url.Values }{query}; _ = holder }
`,
		"slice storage": `package fixture
import "net/url"
func build() { query := make(url.Values); values := []url.Values{query}; _ = values }
`,
		"pointer escape": `package fixture
import "net/url"
func build() { query := make(url.Values); pointer := &query; _ = pointer }
`,
		"package state storage": `package fixture
import "net/url"
var saved url.Values
func build() { query := make(url.Values); saved = query }
`,
		"header helper argument": `package fixture
import "net/http"
func build(request *http.Request) { mutate(request.Header) }
`,
		"header conversion alias escape": `package fixture
import "net/http"
func build(request *http.Request) { headers := (http.Header)(request.Header); mutate(headers) }
`,
	}
}

func schemaKeyedMapReaderFixtures() map[string]string {
	return map[string]string{
		"url query raw key": `package fixture
import "net/url"
func build(parsed *url.URL) { query := parsed.Query(); query.Set("raw", "value") }
`,
		"parse query raw key": `package fixture
import "net/url"
func build(raw string) { query, _ := url.ParseQuery(raw); query.Set("raw", "value") }
`,
	}
}

func TestGeneratedQueryKeyIsAllowed(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func build() { query := make(url.Values); query.Set(dependencymodels.TokenQueryKey, "secret") }
`

	err := checkSource(source, nil, nil, nil)
	if err != nil {
		t.Fatalf("checkSource() rejected generated query key: %v", err)
	}
}

func TestSchemaKeyedMapAliasAndParenthesizedGeneratedKeyAreAllowed(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
type Query = url.Values
func build() { query := make(Query); alias := query; (alias).Set(dependencymodels.TokenQueryKey, "value") }
`

	err := checkSource(source, nil, nil, nil)
	if err != nil {
		t.Fatalf("checkSource() rejected generated key through tracked alias: %v", err)
	}
}

func TestPackageQualifierResolutionUsesImportedPackageObject(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name       string
		source     string
		wantImport string
	}{
		{
			name: "valid aliased standard import",
			source: `package fixture
import network "net/http"
func build() { network.Get("https://example.invalid") }
`,
			wantImport: "net/http",
		},
		{
			name: "local shadows imported qualifier",
			source: `package fixture
import network "net/http"
func build() { network := struct{ Get func(string) }{}; network.Get("https://example.invalid") }
`,
			wantImport: "",
		},
		{
			name: "counterfeit aliased import",
			source: `package fixture
import network "example.invalid/net/http"
func build() { network.Get("https://example.invalid") }
`,
			wantImport: "example.invalid/net/http",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			resolved, err := packageSelectorImportPath(fixture.source)
			if err != nil {
				t.Fatal(err)
			}

			if resolved != fixture.wantImport {
				t.Fatalf("importedPath() = %q, want %q", resolved, fixture.wantImport)
			}
		})
	}
}

func packageSelectorImportPath(source string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		return "", fmt.Errorf("parse package selector fixture: %w", err)
	}

	var resolved string

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		selector, selectorFound := unparen(call.Fun).(*ast.SelectorExpr)
		if selectorFound {
			resolved = importedPath(file, selector.X)
		}

		return true
	})

	return resolved, nil
}

func TestGeneratedQueryKeyRejectsLocalModelPackageShadow(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func build() {
  dependencymodels := struct{ TokenQueryKey string }{TokenQueryKey: "token"}
  query := make(url.Values)
  query.Set(dependencymodels.TokenQueryKey, "value")
}
`

	err := checkSource(source, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "query and header keys") {
		t.Fatalf("checkSource() error = %v, want shadowed import rejection", err)
	}
}

func TestHTTPEndpointInventoryRejectsRouteDrift(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Test reads a fixed repository source fixture for route drift checks.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	assertEndpointRouteDrifts(t, path, string(source), endpointRouteDriftFixtures())
}

func endpointRouteDriftFixtures() map[string][2]string {
	return map[string][2]string{
		"hardcoded method": {
			"dependencymodels.CloudRequestHTTPMethod,",
			`"POST",`,
		},
		"hardcoded path": {
			"result.Path = dependencymodels.CloudRequestPath",
			`result.Path = "/v2"`,
		},
		"token-nil path skip": {
			"result.Path = dependencymodels.CloudRequestPath",
			"if token != nil { result.Path = dependencymodels.CloudRequestPath }",
		},
		"empty-path branch skipped when token is nil": {
			"if result.Path == \"\" {\n\t\tresult.Path = dependencymodels.CloudRequestPath\n\t}",
			"if token != nil {\n\t\tif result.Path == \"\" { result.Path = dependencymodels.CloudRequestPath }\n\t}",
		},
		"appended route path": {
			"requestURL.String(),",
			`requestURL.String() + "/extra",`,
		},
		"hardcoded token key": {
			"query.Set(dependencymodels.TokenQueryKey, *token)",
			`query.Set("token", *token)`,
		},
		"generated query reassigned from parse query": {
			"query.Set(dependencymodels.TokenQueryKey, *token)",
			"query, _ = url.ParseQuery(*token)\n\t\tquery.Set(dependencymodels.TokenQueryKey, *token)",
		},
		"outbound URL query source": {
			"query := make(url.Values)",
			"query := baseURL.Query()",
		},
		"base URL replaced at request": {
			"requestURL(baseURL, token)",
			"requestURL(otherURL, token)",
		},
		"injected transport bypass": {
			"httpClient.Do(request)",
			"http.DefaultClient.Do(request)",
		},
		"shadowed HTTP package qualifier": {
			"\tbodyBytes, err := json.Marshal(cloudRequest)",
			"\thttp := fakeHTTP{}\n\tbodyBytes, err := json.Marshal(cloudRequest)",
		},
		"shadowed generated model package qualifier": {
			"\tbodyBytes, err := json.Marshal(cloudRequest)",
			"\tdependencymodels := fakeModels{}\n\tbodyBytes, err := json.Marshal(cloudRequest)",
		},
		"shadowed URL package qualifier": {
			"\tresult := *baseURL",
			"\turl := fakeURL{}\n\tresult := *baseURL",
		},
	}
}

func TestHTTPEndpointInventoryAcceptsBranchCompleteRouteAssignment(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Test reads a fixed repository source fixture for route control-flow checks.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	const original = "if result.Path == \"\" {\n\t\tresult.Path = dependencymodels.CloudRequestPath\n\t}"

	complete := strings.Join([]string{
		`if result.Path == "" {`,
		"\tif token == nil {",
		"\t\tresult.Path = dependencymodels.CloudRequestPath",
		"\t} else {",
		"\t\tresult.Path = dependencymodels.CloudRequestPath",
		"\t}",
		"}",
	}, "\n")

	branchComplete := strings.Replace(string(source), original, complete, 1)
	if branchComplete == string(source) {
		t.Fatal("test fixture did not find the generated route assignment")
	}

	err = checkEndpointSource(path, []byte(branchComplete))
	if err != nil {
		t.Fatalf("checkEndpointSource() rejected branch-complete generated route assignment: %v", err)
	}
}

func assertEndpointRouteDrifts(t *testing.T, path, source string, fixtures map[string][2]string) {
	t.Helper()

	for name, replacement := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mutated := strings.Replace(source, replacement[0], replacement[1], 1)
			if mutated == source {
				t.Fatalf("test fixture did not find source segment %q", replacement[0])
			}

			err := checkEndpointSource(path, []byte(mutated))
			if err == nil {
				t.Fatal("checkEndpointSource() accepted route drift")
			}
		})
	}
}

func TestHTTPEndpointInventoryAcceptsAliasedImports(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Test reads a fixed repository source fixture for import alias checks.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	aliased := strings.ReplaceAll(string(source), `"net/http"`, `nethttp "net/http"`)
	aliased = strings.ReplaceAll(aliased, `"net/url"`, `neturl "net/url"`)
	aliased = strings.ReplaceAll(
		aliased,
		`"github.com/portpowered/go-tplink/pkg/dependencymodels"`,
		`wire "github.com/portpowered/go-tplink/pkg/dependencymodels"`,
	)
	aliased = strings.ReplaceAll(aliased, "http.", "nethttp.")
	aliased = strings.ReplaceAll(aliased, "url.", "neturl.")
	aliased = strings.ReplaceAll(aliased, "dependencymodels.", "wire.")

	err = checkEndpointSource(path, []byte(aliased))
	if err != nil {
		t.Fatalf("checkEndpointSource() rejected exact aliased imports: %v", err)
	}
}

func TestClientCloudSendUsesConfiguredReceiverFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "tplink", "client.go")
	//nolint:gosec // Test reads a fixed SDK source fixture for transport provenance checks.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	assertClientCloudSendSourceAccepted(t, path, string(source))

	const original = "response, err := cloud.Send(ctx, client.httpClient, client.baseURL, operation, cloudRequest, token)"

	fixtures := map[string]string{
		"default transport": "response, err := cloud.Send(ctx, http.DefaultClient, " +
			"client.baseURL, operation, cloudRequest, token)",
		"unconfigured authority": "response, err := cloud.Send(ctx, client.httpClient, " +
			"&url.URL{Scheme: \"https\", Host: \"attacker.invalid\"}, operation, cloudRequest, token)",
		"base URL alias mutation": "baseURL := client.baseURL\n\tbaseURL.Host = \"attacker.invalid\"\n\t" +
			"response, err := cloud.Send(ctx, client.httpClient, baseURL, operation, cloudRequest, token)",
		"transport alias mutation": "transport := client.httpClient\n\ttransport = http.DefaultClient\n\t" +
			"response, err := cloud.Send(ctx, transport, client.baseURL, operation, cloudRequest, token)",
	}

	for name, replacement := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mutated := strings.Replace(string(source), original, replacement, 1)
			if mutated == string(source) {
				t.Fatal("test fixture did not find the checked cloud.Send call")
			}

			err := checkClientCloudSendSource(path, mutated)
			if err == nil {
				t.Fatal("checkClientCloudSendCallSite() accepted a different transport or authority")
			}
		})
	}
}

func TestClientCloudSendAcceptsUnmodifiedReceiverAliases(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "tplink", "client.go")
	//nolint:gosec // Test reads a fixed SDK source fixture for transport provenance checks.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	const (
		original = "response, err := cloud.Send(ctx, client.httpClient, client.baseURL, operation, cloudRequest, token)"
		aliased  = "transport := client.httpClient\n\tbaseURL := client.baseURL\n\t" +
			"response, err := cloud.Send(ctx, transport, baseURL, operation, cloudRequest, token)"
	)

	mutated := strings.Replace(string(source), original, aliased, 1)
	if mutated == string(source) {
		t.Fatal("test fixture did not find the checked cloud.Send call")
	}

	assertClientCloudSendSourceAccepted(t, path, mutated)
}

func TestProductionClientInventoryRejectsTransportAndAuthoritySubstitution(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	path := filepath.Join(root, "pkg", "tplink", "client.go")
	//nolint:gosec // Test reads a fixed SDK source fixture for production gate verification.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	wireValues, generatedMaps, err := readGeneratedInventory(root)
	if err != nil {
		t.Fatal(err)
	}

	generatedTypes, err := readGeneratedModelTypes(root)
	if err != nil {
		t.Fatal(err)
	}

	fixtures := productionClientSubstitutionFixtures(string(source))

	for name, mutated := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if mutated == string(source) {
				t.Fatal("test fixture did not find the cloud.Send call")
			}

			productionPath := writeProductionClientFixture(t, source)

			uses := make(map[string]bool)

			err := checkProductionFile(productionPath, generatedMaps, wireValues, generatedTypes, uses)
			if err != nil {
				t.Fatalf("checkProductionFile() rejected production baseline: %v", err)
			}

			err = os.WriteFile(productionPath, []byte(mutated), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			err = checkProductionFile(productionPath, generatedMaps, wireValues, generatedTypes, uses)
			if err == nil {
				t.Fatal("checkProductionFile() accepted an unconfigured cloud transport or authority")
			}
		})
	}
}

func productionClientSubstitutionFixtures(source string) map[string]string {
	return map[string]string{
		"default transport": strings.Replace(
			source,
			"cloud.Send(ctx, client.httpClient, client.baseURL, operation, cloudRequest, token)",
			"cloud.Send(ctx, http.DefaultClient, client.baseURL, operation, cloudRequest, token)",
			1,
		),
		"unconfigured authority": strings.Replace(
			source,
			"cloud.Send(ctx, client.httpClient, client.baseURL, operation, cloudRequest, token)",
			"cloud.Send(ctx, client.httpClient, &url.URL{Scheme: \"https\", Host: \"attacker.invalid\"}, "+
				"operation, cloudRequest, token)",
			1,
		),
	}
}

func writeProductionClientFixture(t *testing.T, source []byte) string {
	t.Helper()

	productionPath := filepath.Join(t.TempDir(), "pkg", "tplink", "client.go")

	err := os.MkdirAll(filepath.Dir(productionPath), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // The path is confined to a fixture directory created by t.TempDir.
	err = os.WriteFile(productionPath, source, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return productionPath
}

func assertClientCloudSendSourceAccepted(t *testing.T, path, source string) {
	t.Helper()

	err := checkClientCloudSendSource(path, source)
	if err != nil {
		t.Fatalf("checkClientCloudSendCallSite() rejected configured receiver fields: %v", err)
	}
}

func checkClientCloudSendSource(path, source string) error {
	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, path, []byte(source), parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse Client transport fixture: %w", err)
	}

	return checkClientCloudSendCallSite(path, fileSet, file)
}

func TestUnregisteredNetworkPrimitiveGate(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"convenience request": `package fixture
import "net/http"
func build() { _, _ = http.Get("https://example.invalid") }
`,
		"injected client send": `package fixture
import "net/http"
func build(client *http.Client) { _, _ = client.Do(nil) }
`,
		"file scope method value": `package fixture
import "net/http"
var send = http.DefaultClient.Do
`,
		"method expression": `package fixture
import "net/http"
var send = (*http.Client).Do
`,
		"unlisted raw dial": `package fixture
import "net"
func build() { _, _ = net.Dial("tcp", "example.invalid:443") }
`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fileSet := token.NewFileSet()

			file, err := parser.ParseFile(fileSet, "fixture.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}

			err = checkUnregisteredNetworkPrimitives("fixture.go", fileSet, file)
			if err == nil {
				t.Fatal("network inventory accepted an unregistered network edge")
			}
		})
	}
}

func TestUnregisteredNetworkPrimitiveGateAllowsInjectedConfiguration(t *testing.T) {
	t.Parallel()

	const source = `package fixture
import (
  "net/http"
  "net/url"
)
func build() (*url.URL, http.Client) {
  parsed, _ := url.Parse("https://example.invalid")
  return parsed, *http.DefaultClient
}
`

	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}

	err = checkUnregisteredNetworkPrimitives("fixture.go", fileSet, file)
	if err != nil {
		t.Fatalf("network inventory rejected configuration without a network edge: %v", err)
	}
}

func TestGeneratedOutputMustBeTracked(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initialize := exec.CommandContext(context.Background(), "git", "init", "--quiet")
	initialize.Dir = root

	output, err := initialize.CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}

	outputPath := filepath.Join(root, "models.gen.go")

	err = os.WriteFile(outputPath, []byte("// generated fixture\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	tracked, err := gitTracksPath(root, "models.gen.go")
	if err != nil {
		t.Fatal(err)
	}

	if tracked {
		t.Fatal("gitTracksPath() accepted an untracked generated output")
	}

	add := exec.CommandContext(context.Background(), "git", "add", "--", "models.gen.go")
	add.Dir = root

	output, err = add.CombinedOutput()
	if err != nil {
		t.Fatalf("git add fixture: %v: %s", err, output)
	}

	tracked, err = gitTracksPath(root, "models.gen.go")
	if err != nil {
		t.Fatal(err)
	}

	if !tracked {
		t.Fatal("gitTracksPath() did not recognize the staged generated output")
	}
}

func TestEndpointInventoryRequiresSchemaAndCallSiteMapping(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, directory := range []string{"api", "docs"} {
		err := os.MkdirAll(filepath.Join(root, directory), 0o700)
		if err != nil {
			t.Fatal(err)
		}
	}

	const schema = "openapi: 3.0.3\npaths:\n  /:\n    post:\n      responses: {}\n"

	const row = "| TP-Link Cloud JSON request/response | `api/openapi.yaml` | " +
		"`CloudRequestHTTPMethod=POST`, `CloudRequestPath=/`, " +
		"`CloudRequestContentTypeHeader=Content-Type`, `CloudRequestContentType=application/json`, " +
		"`TokenQueryKey=token` | `pkg/dependencies/cloud/cloud.go:newRequest`, `requestURL`, and `Send` |\n"

	writeEndpointInventoryFixture(t, root, schema, row)

	err := checkEndpointInventory(root)
	if err != nil {
		t.Fatalf("checkEndpointInventory() rejected matched route inventory: %v", err)
	}

	writeEndpointInventoryFixture(t, root, strings.ReplaceAll(schema, "  /:", "  /v2:"), row)

	err = checkEndpointInventory(root)
	if err == nil {
		t.Fatal("checkEndpointInventory() accepted an unregistered endpoint")
	}

	writeEndpointInventoryFixture(
		t,
		root,
		strings.ReplaceAll(
			schema,
			"    post:\n      responses:",
			"    get:\n      responses: {}\n    post:\n      responses:",
		),
		row,
	)

	err = checkEndpointInventory(root)
	if err == nil {
		t.Fatal("checkEndpointInventory() accepted an additional method")
	}

	writeEndpointInventoryFixture(t, root, schema, strings.ReplaceAll(row, "POST", "PUT"))

	err = checkEndpointInventory(root)
	if err == nil {
		t.Fatal("checkEndpointInventory() accepted an inventory row without the generated method")
	}
}

func writeEndpointInventoryFixture(t *testing.T, root, schemaText, rowText string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(root, "api", "openapi.yaml"), []byte(schemaText), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(root, "docs", "wire-model-inventory.md"), []byte(rowText), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func checkSource(source string, wireValues, generatedMaps, generatedTypes map[string]bool) error {
	const path = "fixture.go"

	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, path, source, 0)
	if err != nil {
		return fmt.Errorf("parse fixture: %w", err)
	}

	return checkFile(path, fileSet, file, generatedMaps, wireValues, generatedTypes, make(map[string]bool))
}
