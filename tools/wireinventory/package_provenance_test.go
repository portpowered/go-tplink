package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	helperSourceFilename = "helpers.go"
	useSourceFilename    = "use.go"
)

func TestPackageWireProvenanceRejectsSiblingFixedScalarAndMapEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := writePackageSourceFiles(t, root, map[string]string{
		helperSourceFilename: `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func r2FixedMethod() (result dependencymodels.LoginCloudRequestMethod) {
  result = "r2-unregistered-method"
  return
}
func r2FixedWrapper() (result dependencymodels.LoginCloudRequestMethod) {
  result = r2FixedMethod()
  return
}
func r2CallerQuery(values url.Values) (result url.Values) {
  result = values
  return
}
`,
		useSourceFilename: `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func r2Mutate(values url.Values) { values.Set("r2-unregistered-query", "value") }
func r2Build(values url.Values) {
  _ = dependencymodels.LoginCloudRequest{Method: r2FixedWrapper()}
  r2Mutate(r2CallerQuery(values))
}
`,
	})

	err := checkPackageWireProvenance(paths, map[string]bool{loginCloudRequestModel: true},
		map[string]bool{"LoginCloudRequestMethod": true}, readCurrentGeneratedScalarMetadata(t))

	if err == nil || !strings.Contains(err.Error(), "fixed scalar") ||
		!strings.Contains(err.Error(), useSourceFilename) {
		t.Fatalf("checkPackageWireProvenance() error = %v, want sibling scalar rejection in use.go", err)
	}

	wireValues, generatedMaps, generatedTypes, generatedScalars, scalarMetadata := readCurrentGeneratedInventory(t)

	err = checkProductionFiles(paths, generatedMaps, wireValues, generatedTypes, generatedScalars,
		scalarMetadata, make(map[string]bool))
	if err == nil || !strings.Contains(err.Error(), useSourceFilename) {
		t.Fatalf("checkProductionFiles() error = %v, want package-scoped production rejection in use.go", err)
	}

	assertPackageWireMapEscape(t, root, "unverified helper", "sibling map escape")
}

func assertPackageWireMapEscape(t *testing.T, root, expectedMessage, scenario string) {
	t.Helper()

	files := []packageProvenanceFile{
		parsePackageProvenanceFile(t, filepath.Join(root, helperSourceFilename)),
		parsePackageProvenanceFile(t, filepath.Join(root, useSourceFilename)),
	}
	helpers := packageHelpers(files, map[string]bool{loginCloudRequestModel: true}, nil)

	err := checkPackageWireMapEscapes(files[1], helpers)
	if err == nil || !strings.Contains(err.Error(), expectedMessage) || !strings.Contains(err.Error(), useSourceFilename) {
		t.Fatalf("checkPackageWireMapEscapes() error = %v, want %s rejection in use.go", err, scenario)
	}
}

func TestPackageWireProvenanceAcceptsCallerValuesThroughProductionHook(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := writePackageSourceFiles(t, root, map[string]string{
		helperSourceFilename: `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func r2NameFromCaller(value string) string {
  return value
}
func r2QueryFromCaller(values url.Values) url.Values { return values }
`,
		useSourceFilename: `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func BuildFromCaller(value string, values url.Values) {
  _ = dependencymodels.LoginParams{CloudUserName: r2NameFromCaller(value)}
  _ = r2QueryFromCaller(values).Encode()
}
`,
	})

	wireValues, generatedMaps, generatedTypes, generatedScalars, scalarMetadata := readCurrentGeneratedInventory(t)

	err := checkProductionFiles(paths, generatedMaps, wireValues, generatedTypes, generatedScalars,
		scalarMetadata, make(map[string]bool))
	if err != nil {
		t.Fatalf("checkProductionFiles() rejected caller-owned values: %v", err)
	}
}

func TestGeneratedScalarMetadataIncludesEnumFieldsAndConstants(t *testing.T) {
	t.Parallel()

	metadata := readCurrentGeneratedScalarMetadata(t)
	if !metadata.constants[dependencymodelsImportPath]["MethodLogin"] {
		t.Fatal("generated scalar metadata omitted dependencymodels.MethodLogin")
	}

	if !metadata.fields[dependencymodelsImportPath][loginCloudRequestModel]["Method"] {
		t.Fatal("generated scalar metadata omitted LoginCloudRequest.Method")
	}

	if !metadata.closedEnums[dependencymodelsImportPath][loginCloudRequestModel]["Method"]["login"] {
		t.Fatal("generated scalar metadata omitted the closed LoginCloudRequest.Method enum")
	}

	if metadata.closedEnums[dependencymodelsImportPath]["LoginParams"]["CloudUserName"] != nil {
		t.Fatal("generated scalar metadata marked caller-defined LoginParams.CloudUserName closed")
	}

	if metadata.constantValues[dependencymodelsImportPath]["MethodLogin"] != "login" {
		t.Fatal("generated scalar metadata did not bind MethodLogin to its schema value")
	}
}

func TestPackageScalarProvenanceRejectsUnresolvedPackageAndMethodValues(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]string{
		"sibling package global": map[string]string{
			helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
var r2GlobalMethod dm.LoginCloudRequestMethod = "r2-unregistered-global"
func r2GlobalHelper() dm.LoginCloudRequestMethod { return r2GlobalMethod }
`,
			useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2Build() { _ = dm.LoginCloudRequest{Method: r2GlobalHelper()} }
`,
		},
		"sibling receiver method": map[string]string{
			helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
type r2MethodSource struct{}
func (r2MethodSource) r2Method() dm.LoginCloudRequestMethod { return "r2-unregistered-method" }
`,
			useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2Build() { _ = dm.LoginCloudRequest{Method: (r2MethodSource{}).r2Method()} }
`,
		},
		"sibling callback return": map[string]string{
			helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2CallbackValue() dm.LoginCloudRequestMethod { return "r2-unregistered-callback" }
func r2Callback() func() dm.LoginCloudRequestMethod { return r2CallbackValue }
`,
			useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2Build() { _ = dm.LoginCloudRequest{Method: r2Callback()()} }
`,
		},
		"recursive fixed fallback": map[string]string{
			helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2RecursiveFixed(flag bool) dm.LoginCloudRequestMethod {
  if flag { return "r2-recursive-unregistered" }
  return r2RecursiveFixed(!flag)
}
`,
			useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2Build() { _ = dm.LoginCloudRequest{Method: r2RecursiveFixed(true)} }
`,
		},
	}

	for name, files := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertProductionScalarFixtureRejected(t, files)
		})
	}
}

func TestPackageScalarProvenanceRejectsLongSiblingHelperChain(t *testing.T) {
	t.Parallel()

	var first strings.Builder

	first.WriteString("package fixture\nimport dm \"github.com/portpowered/go-tplink/pkg/dependencymodels\"\n")

	var second strings.Builder

	second.WriteString("package fixture\nimport dm \"github.com/portpowered/go-tplink/pkg/dependencymodels\"\n")

	for index := range 80 {
		builder := &first
		if index%2 == 1 {
			builder = &second
		}

		if index == 0 {
			fmt.Fprintf(builder, "func r2Chain0() dm.LoginCloudRequestMethod { return \"r2-long-chain\" }\n")

			continue
		}

		fmt.Fprintf(builder, "func r2Chain%d() dm.LoginCloudRequestMethod { return r2Chain%d() }\n", index, index-1)
	}

	assertProductionScalarFixtureRejected(t, map[string]string{
		"chain-a.go": first.String(),
		"chain-b.go": second.String(),
		useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2Build() { _ = dm.LoginCloudRequest{Method: r2Chain79()} }
`,
	})
}

func TestPackageScalarProvenanceAcceptsCallerAndGeneratedValues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := writePackageSourceFiles(t, root, map[string]string{
		helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2CallerName(value string) (result string) {
  result = value
  return
}
func r2GeneratedMethod() dm.LoginCloudRequestMethod { return dm.MethodLogin }
`,
		useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func Build(value string) {
  _ = dm.LoginParams{CloudUserName: r2CallerName(value)}
  _ = dm.LoginCloudRequest{Method: r2GeneratedMethod()}
}
`,
	})

	err := checkProductionScalarFiles(t, paths)
	if err != nil {
		t.Fatalf("checkProductionFiles() rejected caller or generated values: %v", err)
	}
}

func TestPackageScalarProvenanceRejectsUnvalidatedClosedEnumCallerValues(t *testing.T) {
	t.Parallel()

	assertProductionScalarFixtureRejected(t, map[string]string{
		useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func Build(value dm.LoginCloudRequestMethod) {
  _ = dm.LoginCloudRequest{Method: value}
  request := dm.LoginCloudRequest{}
  request.Method = value
}
`,
	})
}

func TestPackageScalarProvenanceRejectsClosedEnumFromExportedModelParameter(t *testing.T) {
	t.Parallel()

	assertProductionScalarFixtureRejected(t, map[string]string{
		useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func Build(value dm.LoginCloudRequest) {
  _ = dm.LoginCloudRequest{Method: value.Method}
}
`,
	})
}

func TestPackageScalarProvenanceRejectsHelperSinkWithFixedClosedEnumArgument(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := writePackageSourceFiles(t, root, map[string]string{
		helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2BuildRequest(method dm.LoginCloudRequestMethod) {
  _ = dm.LoginCloudRequest{Method: method}
}
`,
		useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func Build() { r2BuildRequest(dm.LoginCloudRequestMethod("unregistered")) }
`,
	})

	_, _, generatedTypes, generatedScalars, metadata := readCurrentGeneratedInventory(t)

	sources := make([]packageProvenanceFile, 0, len(paths))
	for _, path := range paths {
		sources = append(sources, parsePackageProvenanceFile(t, path))
	}

	err := checkPackageScalarProvenance(sources, generatedTypes, generatedScalars, metadata)
	if err == nil || !strings.Contains(err.Error(), generatedScalarMessage) ||
		!strings.Contains(err.Error(), helperSourceFilename) {
		t.Fatalf("checkPackageScalarProvenance() error = %v, want helper sink rejection", err)
	}
}

func TestPackageScalarProvenanceAcceptsCallerOpenValueThroughHelperSink(t *testing.T) {
	t.Parallel()

	paths := writePackageSourceFiles(t, t.TempDir(), map[string]string{
		helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2BuildRequest(name string) {
  _ = dm.LoginParams{CloudUserName: name}
}
`,
		useSourceFilename: `package fixture
func Build(name string) { r2BuildRequest(name) }
`,
	})

	err := checkProductionScalarFiles(t, paths)
	if err != nil {
		t.Fatalf("checkProductionFiles() rejected a caller-open value at a helper sink: %v", err)
	}
}

func TestPackageScalarProvenanceAcceptsCallerSuppliedCallbacks(t *testing.T) {
	t.Parallel()

	paths := writePackageSourceFiles(t, t.TempDir(), map[string]string{
		helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2ForwardCallback(callback func/*helper*/ () string) string {
  return callback()
}
`,
		useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func BuildFromCaller(callback func/*caller*/ () string) {
  _ = dm.LoginParams{CloudUserName: r2ForwardCallback(callback)}
}
`,
	})

	err := checkProductionScalarFiles(t, paths)
	if err != nil {
		t.Fatalf("checkProductionFiles() rejected caller-supplied callbacks: %v", err)
	}
}

func TestPackageScalarProvenanceAcceptsCallerSuppliedReturnedCallback(t *testing.T) {
	t.Parallel()

	paths := writePackageSourceFiles(t, t.TempDir(), map[string]string{
		useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func BuildFromCaller(callbackFactory func/*factory*/ () func/*method*/ () string) {
  _ = dm.LoginParams{CloudUserName: callbackFactory()()}
}
`,
	})

	err := checkProductionScalarFiles(t, paths)
	if err != nil {
		t.Fatalf("checkProductionFiles() rejected a caller-supplied returned callback: %v", err)
	}
}

func TestPackageScalarProvenanceRejectsConditionalNamedCallbackResult(t *testing.T) {
	t.Parallel()

	assertProductionScalarFixtureRejected(t, map[string]string{
		helperSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2KnownCallback() dm.LoginCloudRequestMethod { return dm.MethodLogin }
func r2ConditionalCallback(flag bool) (result func/*conditional*/ () dm.LoginCloudRequestMethod) {
  if flag { result = r2KnownCallback }
  return
}
`,
		useSourceFilename: `package fixture
import dm "github.com/portpowered/go-tplink/pkg/dependencymodels"
func r2Build(flag bool) { _ = dm.LoginCloudRequest{Method: r2ConditionalCallback(flag)()} }
`,
	})
}

func assertProductionScalarFixtureRejected(t *testing.T, files map[string]string) {
	t.Helper()

	root := t.TempDir()
	paths := writePackageSourceFiles(t, root, files)

	err := checkProductionScalarFiles(t, paths)
	if err == nil || !strings.Contains(err.Error(), generatedScalarMessage) {
		t.Fatalf("checkProductionFiles() error = %v, want unresolved generated scalar rejection", err)
	}
}

func checkProductionScalarFiles(t *testing.T, paths []string) error {
	t.Helper()

	wireValues, generatedMaps, generatedTypes, generatedScalars, scalarMetadata := readCurrentGeneratedInventory(t)

	return checkProductionFiles(paths, generatedMaps, wireValues, generatedTypes, generatedScalars,
		scalarMetadata, make(map[string]bool))
}

func TestProductionWireMapGateRejectsSiblingHelperEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := writePackageSourceFiles(t, root, map[string]string{
		helperSourceFilename: `package fixture
import "net/url"
func r2CallerQuery(values url.Values) (result url.Values) { result = values; return }
`,
		useSourceFilename: `package fixture
import "net/url"
func r2MutateQuery(values url.Values) { values.Set("r2-unregistered", "value") }
func r2BuildQuery(values url.Values) {
  query := r2CallerQuery(values)
  r2MutateQuery(query)
}
`,
	})

	wireValues, generatedMaps, generatedTypes, generatedScalars, scalarMetadata := readCurrentGeneratedInventory(t)
	err := checkProductionFiles(paths, generatedMaps, wireValues, generatedTypes, generatedScalars,
		scalarMetadata, make(map[string]bool))

	if err == nil || !strings.Contains(err.Error(), "unverified helper") ||
		!strings.Contains(err.Error(), useSourceFilename) {
		t.Fatalf("checkProductionFiles() error = %v, want sibling map escape rejection in use.go", err)
	}
}

func TestPackageWireMapGateRejectsGeneratedSiblingMapEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writePackageSourceFiles(t, root, map[string]string{
		helperSourceFilename: `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func r2GeneratedQuery() (result url.Values) {
  result = make(url.Values)
  result.Set(dependencymodels.TokenQueryKey, "r2-token")
  return
}

`,
		useSourceFilename: `package fixture
import "net/url"
func r2MutateQuery(values url.Values) { values.Set("r2-unregistered", "value") }
func r2BuildQuery() { r2MutateQuery(r2GeneratedQuery()) }
`,
	})

	assertPackageWireMapEscape(t, root, "unverified helper", "generated map escape")
}

func TestPackageWireMapGateRejectsSiblingMethodValueEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writePackageSourceFiles(t, root, map[string]string{
		helperSourceFilename: `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func r2GeneratedQuery() (result url.Values) {
  result = make(url.Values)
  result.Set(dependencymodels.TokenQueryKey, "r2-token")
  return
}
`,
		useSourceFilename: `package fixture
func r2BuildQuery() {
  set := r2GeneratedQuery().Set
  set("r2-unregistered", "value")
}
`,
	})

	assertPackageWireMapEscape(t, root, "method values", "sibling method-value")
}

func TestHTTPRequestObjectMutationGateRejectsMutationsAndEscapes(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Test reads the fixed production cloud transport source as a mutation fixture.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	const constructor = "request, err := newRequest(ctx, baseURL, cloudRequest, token)"

	fixtures := map[string]string{
		"method mutation":             `request.Method = "GET"`,
		"path mutation":               `request.URL.Path = "/r2-unregistered-path"`,
		"scheme mutation":             `request.URL.Scheme = "http"`,
		"user information mutation":   `request.URL.User = nil`,
		"origin mutation":             `request.URL.Host = "attacker.invalid"`,
		"query mutation":              `request.URL.RawQuery = "r2-unregistered=value"`,
		"header mutation":             `request.Header.Set("X-R2-Unregistered", "value")`,
		"body mutation":               `request.Body = nil`,
		"body factory mutation":       `request.GetBody = nil`,
		"content length mutation":     `request.ContentLength = 0`,
		"transfer encoding mutation":  `request.TransferEncoding = []string{"chunked"}`,
		"URL alias mutation":          "requestURL := request.URL\n\trequestURL.RawPath = \"/r2-alias-path\"",
		"header alias mutation":       "requestHeaders := request.Header\n\trequestHeaders.Add(\"X-R2-Alias\", \"value\")",
		"request alias mutation":      "requestAlias := request\n\trequestAlias.Method = \"GET\"",
		"request method value escape": "setHeader := request.Header.Set\n\t_ = setHeader",
		"unverified helper":           "r2MutateRequest(request)",
		"aggregate helper escape":     "r2MutateRequest(struct{ Request *http.Request }{request})",
		"clone mutation":              "requestClone := request.Clone(ctx)\n\trequestClone.URL.Path = \"/r2-clone-path\"",
	}

	for name, mutation := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mutated := strings.Replace(string(source), constructor, constructor+"\n\t"+mutation, 1)
			if mutated == string(source) {
				t.Fatal("test fixture did not find the constructed request")
			}

			err := checkEndpointSource(path, []byte(mutated))
			if err == nil || !strings.Contains(err.Error(), requestMutationMessage) {
				t.Fatalf("checkEndpointSource() error = %v, want constructed request mutation rejection", err)
			}
		})
	}
}

func TestHTTPRequestObjectMutationGateAcceptsReadOnlyLocalAliases(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Test reads the fixed production cloud transport source as a read-only alias fixture.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	const constructor = "request, err := newRequest(ctx, baseURL, cloudRequest, token)"

	const readOnlyAliases = "requestAlias := request\n\trequestURL := request.URL\n\trequestHeaders := request.Header\n" +
		"_ = requestAlias.Method\n\t_ = requestURL.Path\n\t_ = requestHeaders"

	mutated := strings.Replace(string(source), constructor, constructor+"\n\t"+readOnlyAliases, 1)
	if mutated == string(source) {
		t.Fatal("test fixture did not find the constructed request")
	}

	err = checkEndpointSource(path, []byte(mutated))
	if err != nil {
		t.Fatalf("checkEndpointSource() rejected read-only request aliases: %v", err)
	}
}

func TestHTTPRequestConstructorMutationGateRejectsMutationsAndEscapes(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Test reads the fixed production cloud transport source as a constructor mutation fixture.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	const returnRequest = "return request, nil"

	fixtures := map[string]string{
		"method mutation":            `request.Method = "PUT"`,
		"path mutation":              `request.URL.Path = "/r2-constructor-path"`,
		"scheme mutation":            `request.URL.Scheme = "http"`,
		"user information mutation":  `request.URL.User = nil`,
		"origin mutation":            `request.URL.Host = "attacker.invalid"`,
		"query mutation":             `request.URL.RawQuery = "r2-constructor=value"`,
		"header mutation":            `request.Header.Set("X-R2-Constructor", "value")`,
		"body mutation":              `request.Body = nil`,
		"body factory mutation":      `request.GetBody = nil`,
		"content length mutation":    `request.ContentLength = 0`,
		"transfer encoding mutation": `request.TransferEncoding = []string{"chunked"}`,
		"URL alias mutation":         "requestURL := request.URL\n\trequestURL.RawQuery = \"r2-constructor-alias=value\"",
		"header alias mutation": "requestHeaders := request.Header\n\trequestHeaders.Add(\"X-R2-Constructor-" +
			"Alias\", \"value\")",
		"request alias mutation":      "requestAlias := request\n\trequestAlias.Method = \"PUT\"",
		"unverified helper":           "r2MutateRequest(request)",
		"aggregate helper escape":     "r2MutateRequest(struct{ Request *http.Request }{request})",
		"body backing mutation":       "bodyBytes[0] = 'x'",
		"body backing alias mutation": "mutableBody := bodyBytes\n\tmutableBody[0] = 'x'",
	}

	for name, mutation := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mutated := strings.Replace(string(source), returnRequest, mutation+"\n\treturn request, nil", 1)
			if mutated == string(source) {
				t.Fatal("test fixture did not find the constructed request return")
			}

			err := checkEndpointSource(path, []byte(mutated))
			if err == nil || !strings.Contains(err.Error(), requestMutationMessage) {
				t.Fatalf("checkEndpointSource() error = %v, want constructed request mutation rejection", err)
			}
		})
	}
}

func TestHTTPRequestConstructorMutationGateAcceptsUnchangedBodyBackingBuffer(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Test reads the fixed production cloud transport as the immutable body control.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	checkErr := checkEndpointSource(path, source)
	if checkErr != nil {
		t.Fatalf("checkEndpointSource() rejected the unchanged request body buffer: %v", checkErr)
	}
}

func TestHTTPRequestConstructorMutationGateTracksBackingBufferBeforeReader(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "pkg", "dependencies", "cloud", "cloud.go")
	//nolint:gosec // Test reads the fixed production cloud transport source as an alias mutation fixture.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	mutated := strings.Replace(string(source),
		"request, err := http.NewRequestWithContext(",
		"readerBytes := bodyBytes\n\trequest, err := http.NewRequestWithContext(", 1)

	mutated = strings.Replace(mutated, "bytes.NewReader(bodyBytes)", "bytes.NewReader(readerBytes)", 1)

	mutated = strings.Replace(mutated, "return request, nil", "bodyBytes[0] = 'x'\n\treturn request, nil", 1)

	if mutated == string(source) {
		t.Fatal("test fixture did not find the body reader and request return")
	}

	checkErr := checkEndpointSource(path, []byte(mutated))
	if checkErr == nil || !strings.Contains(checkErr.Error(), requestMutationMessage) {
		t.Fatalf("checkEndpointSource() error = %v, want original backing-buffer mutation rejection", checkErr)
	}
}

func readCurrentGeneratedInventory(
	t *testing.T,
) (map[string]bool, map[string]bool, map[string]bool, map[string]bool, generatedScalarMetadata) {
	t.Helper()

	root := filepath.Clean(filepath.Join("..", ".."))

	wireValues, generatedMaps, err := readGeneratedInventory(root)
	if err != nil {
		t.Fatal(err)
	}

	generatedTypes, err := readGeneratedModelTypes(root)
	if err != nil {
		t.Fatal(err)
	}

	generatedScalars, err := readGeneratedModelScalars(root)
	if err != nil {
		t.Fatal(err)
	}

	scalarMetadata, err := readGeneratedScalarMetadata(root)
	if err != nil {
		t.Fatal(err)
	}

	return wireValues, generatedMaps, generatedTypes, generatedScalars, scalarMetadata
}

func readCurrentGeneratedScalarMetadata(t *testing.T) generatedScalarMetadata {
	t.Helper()

	root := filepath.Clean(filepath.Join("..", ".."))

	metadata, err := readGeneratedScalarMetadata(root)
	if err != nil {
		t.Fatal(err)
	}

	return metadata
}

func writePackageSourceFiles(t *testing.T, root string, files map[string]string) []string {
	t.Helper()

	paths := make([]string, 0, len(files))

	for name, source := range files {
		path := filepath.Join(root, name)

		writeError := os.WriteFile(path, []byte(source), 0o600)
		if writeError != nil {
			t.Fatal(writeError)
		}

		paths = append(paths, path)
	}

	return paths
}

func parsePackageProvenanceFile(t *testing.T, path string) packageProvenanceFile {
	t.Helper()

	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	return packageProvenanceFile{path: path, fileSet: fileSet, file: file}
}
