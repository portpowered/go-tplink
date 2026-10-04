package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestProductionWireInventoryPasses(t *testing.T) {
	t.Parallel()

	err := checkRepository("../..")
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

			err := checkSource(source, nil, generatedMaps)
			if err == nil || !strings.Contains(err.Error(), "map") {
				t.Fatalf("checkSource() error = %v, want map construction rejection", err)
			}
		})
	}
}

func TestCallerOpenStateMutationNegatives(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"direct": `package fixture
func Set(request Request) { request.State["invented"] = true }
`,
		"map alias": `package fixture
func Set(request Request) { state := request.State; state["invented"] = true }
`,
		"conversion alias": `package fixture
func SetLightState(request Request) { state := (LightStateParams)(request.State); state["invented"] = true }
`,
		"request alias": `package fixture
func Set(request Request) { req := request; state := req.State; state["invented"] = true }
`,
		"closure alias": `package fixture
func Set(request Request) { state := request.State; func() { state["invented"] = true }() }
`,
		"delete": `package fixture
func Set(request Request) { delete(request.State, "invented") }
`,
		"clear alias": `package fixture
func Set(request Request) { state := request.State; clear(state) }
`,
		"helper escape": `package fixture
func SetLightState(request Request) { state := request.State; send(state) }
`,
		"return escape": `package fixture
func SetLightState(request Request) LightStateParams { return request.State }
`,
		"aggregate escape": `package fixture
func SetLightState(request Request) { holder.Value = request.State }
`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkSource(source, nil, nil)
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
func SetLightState(request Request) {
  var state dependencymodels.LightTransitionState
  state.AdditionalProperties = request.State
}
`

	err := checkSource(source, nil, nil)
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

	err := checkSource(source, allowedValues, nil)
	if err == nil || !strings.Contains(err.Error(), "raw schema-defined wire string") {
		t.Fatalf("checkSource() error = %v, want raw wire string rejection", err)
	}
}

func checkSource(source string, wireValues, generatedMaps map[string]bool) error {
	const path = "fixture.go"

	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, path, source, 0)
	if err != nil {
		return fmt.Errorf("parse fixture: %w", err)
	}

	return checkFile(path, fileSet, file, generatedMaps, wireValues, make(map[string]bool))
}
