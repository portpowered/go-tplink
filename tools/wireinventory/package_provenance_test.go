package main

import (
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
		map[string]bool{"LoginCloudRequestMethod": true})

	if err == nil || !strings.Contains(err.Error(), "fixed scalar") ||
		!strings.Contains(err.Error(), useSourceFilename) {
		t.Fatalf("checkPackageWireProvenance() error = %v, want sibling scalar rejection in use.go", err)
	}

	wireValues, generatedMaps, generatedTypes, generatedScalars := readCurrentGeneratedInventory(t)

	err = checkProductionFiles(paths, generatedMaps, wireValues, generatedTypes, generatedScalars, make(map[string]bool))
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
func r2MethodFromCaller(value dependencymodels.LoginCloudRequestMethod) dependencymodels.LoginCloudRequestMethod {
  return value
}
func r2QueryFromCaller(values url.Values) url.Values { return values }
`,
		useSourceFilename: `package fixture
import (
  "net/url"
  "github.com/portpowered/go-tplink/pkg/dependencymodels"
)
func r2BuildFromCaller(value dependencymodels.LoginCloudRequestMethod, values url.Values) {
  _ = dependencymodels.LoginCloudRequest{Method: r2MethodFromCaller(value)}
  _ = r2QueryFromCaller(values).Encode()
}
`,
	})

	wireValues, generatedMaps, generatedTypes, generatedScalars := readCurrentGeneratedInventory(t)

	err := checkProductionFiles(paths, generatedMaps, wireValues, generatedTypes, generatedScalars, make(map[string]bool))
	if err != nil {
		t.Fatalf("checkProductionFiles() rejected caller-owned values: %v", err)
	}
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

	wireValues, generatedMaps, generatedTypes, generatedScalars := readCurrentGeneratedInventory(t)
	err := checkProductionFiles(paths, generatedMaps, wireValues, generatedTypes, generatedScalars, make(map[string]bool))

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
		"request alias mutation":  "requestAlias := request\n\trequestAlias.Method = \"PUT\"",
		"unverified helper":       "r2MutateRequest(request)",
		"aggregate helper escape": "r2MutateRequest(struct{ Request *http.Request }{request})",
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

func readCurrentGeneratedInventory(t *testing.T) (map[string]bool, map[string]bool, map[string]bool, map[string]bool) {
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

	return wireValues, generatedMaps, generatedTypes, generatedScalars
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
