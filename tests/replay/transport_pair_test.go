package replay_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/go-tplink/pkg/tplink"
)

func newReplayTestRequest(t *testing.T, method, endpoint string, body io.Reader) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), method, endpoint, body)
	if err != nil {
		t.Fatal(err)
	}

	return request
}

func TestEverySyntheticReplayFixtureHasRequestAndResponse(t *testing.T) {
	t.Parallel()

	paths, globErr := fs.Glob(fixtureFiles, "fixtures/synthetic/synthetic_tplink_*.json")
	if globErr != nil {
		t.Fatal(globErr)
	}

	if len(paths) == 0 {
		t.Fatal("no synthetic replay fixtures")
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			checkSyntheticReplayFixture(t, path)
		})
	}
}

func checkSyntheticReplayFixture(t *testing.T, path string) {
	t.Helper()

	data, err := fixtureFiles.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var exchange fixtureExchange

	decodeErr := json.Unmarshal(data, &exchange)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}

	if exchange.Request.Method == "" || exchange.Request.Origin == "" ||
		len(exchange.Request.Operations) == 0 || len(exchange.Request.Bodies) == 0 ||
		exchange.Response.Status == 0 || len(exchange.Response.Body) == 0 {
		t.Fatal("fixture is missing its request or response")
	}

	stem := strings.TrimSuffix(strings.TrimPrefix(path, "fixtures/synthetic/synthetic_tplink_"), ".json")

	for variant, body := range exchange.Request.Bodies {
		t.Run("variant "+strconv.Itoa(variant), func(t *testing.T) {
			t.Parallel()
			checkSyntheticReplayVariant(t, exchange, stem, variant, body)
		})
	}
}

func checkSyntheticReplayVariant(
	t *testing.T,
	exchange fixtureExchange,
	stem string,
	variant int,
	body json.RawMessage,
) {
	t.Helper()

	operation, err := requestFixtureKey(body)
	if err != nil {
		t.Fatal(err)
	}

	transport := newReplayTransport()
	transport.expectSequence(operation)
	transport.useFixture(operation, stem)
	transport.expectVariant(variant)

	response, err := transport.RoundTrip(replayFixtureRequest(t, exchange.Request, body))
	if err != nil {
		closeReplayResponseBody(t, response)
		t.Fatal(err)
	}

	t.Cleanup(func() { closeReplayResponseBody(t, response) })

	assertReplayFixtureResponse(t, response, exchange, stem, variant)

	assertionErr := transport.assertConsumed()
	if assertionErr != nil {
		t.Fatal(assertionErr)
	}

	assertReplayFixtureCannotRepeat(t, transport, exchange.Request, body)
}

func assertReplayFixtureResponse(
	t *testing.T,
	response *http.Response,
	exchange fixtureExchange,
	stem string,
	variant int,
) {
	t.Helper()

	got, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatal(readErr)
	}

	if response.StatusCode != exchange.Response.Status ||
		!bytes.Equal(got, exchange.Response.Body) ||
		!reflect.DeepEqual(response.Header, http.Header(exchange.Response.Headers)) {
		t.Fatalf("response metadata/body differs for %s variant %d", stem, variant)
	}
}

func assertReplayFixtureCannotRepeat(
	t *testing.T,
	transport *replayTransport,
	fixture fixtureRequest,
	body json.RawMessage,
) {
	t.Helper()
	request := replayFixtureRequest(t, fixture, body)

	response, err := transport.RoundTrip(request)
	closeReplayResponseBody(t, response)

	if err == nil || response != nil {
		t.Fatalf("variant replayed twice: %v, %v", response, err)
	}
}

func replayFixtureRequest(t *testing.T, fixture fixtureRequest, body json.RawMessage) *http.Request {
	t.Helper()

	endpoint := fixture.Origin + fixture.Path
	if query := url.Values(fixture.Query); len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	request := newReplayTestRequest(t, fixture.Method, endpoint, bytes.NewReader(body))
	request.Header = http.Header(fixture.Headers)

	return request
}

func closeReplayResponseBody(t *testing.T, response *http.Response) {
	t.Helper()

	if response == nil || response.Body == nil {
		return
	}

	closeErr := response.Body.Close()
	if closeErr != nil {
		t.Errorf("close replay response body: %v", closeErr)
	}
}

func TestPublicMethodReplayInventory(t *testing.T) {
	t.Parallel()
	// Every exported network operation must map to a stored request variant.
	operations := map[string]struct {
		fixture string
		variant int
	}{
		"Login": {"login", 0}, "GetDevices": {"getDeviceList", 0},
		"TurnOn": {"passthrough_system_set_relay_state", 0}, "TurnOff": {"passthrough_system_set_relay_state", 1},
		"GetPowerState": {"passthrough_system_get_sysinfo", 0}, "Reboot": {"passthrough_system_reboot", 0},
		"SetAlias":      {"passthrough_system_set_dev_alias", 0},
		"SetLightState": {"passthrough_lightingservice_transition_light_state", 3},
		"SetBrightness": {"passthrough_lightingservice_transition_light_state", 0},
		"SetColorTemp":  {"passthrough_lightingservice_transition_light_state", 1},
		"SetColor":      {"passthrough_lightingservice_transition_light_state", 2},
		"GetLightState": {"passthrough_lightingservice_get_light_state", 0},
		"GetBrightness": {"passthrough_lightingservice_get_light_state", 0},
		"GetColorTemp":  {"passthrough_lightingservice_get_light_state", 0},
		"GetColor":      {"passthrough_lightingservice_get_light_state", 0},
	}
	typeOf := reflect.TypeOf(&tplink.Client{})

	var missing []string

	for i := range typeOf.NumMethod() {
		name := typeOf.Method(i).Name
		if name != "Close" {
			if _, ok := operations[name]; !ok {
				missing = append(missing, name)
			}
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("public methods without paired replay inventory: %v", missing)
	}

	for name, entry := range operations {
		if _, ok := typeOf.MethodByName(name); !ok {
			t.Errorf("stale method inventory %s", name)
		}

		data, err := fixtureFiles.ReadFile("fixtures/synthetic/synthetic_tplink_" + entry.fixture + ".json")
		if err != nil {
			t.Fatal(err)
		}

		var pair fixtureExchange

		decodeErr := json.Unmarshal(data, &pair)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}

		if entry.variant >= len(pair.Request.Bodies) {
			t.Errorf("%s variant %d is absent", name, entry.variant)
		}
	}
}

func TestEverySyntheticFailureOutcomeHasStoredRequestAndResponse(t *testing.T) {
	t.Parallel()

	outcomes := readStoredOutcomes(t)
	if len(outcomes) == 0 {
		t.Fatal("no stored failure outcomes")
	}

	seen := map[string]bool{}
	for _, outcome := range outcomes {
		validateStoredOutcome(t, outcome, seen)

		t.Run(outcome.ID, func(t *testing.T) {
			t.Parallel()
			replayStoredOutcome(t, outcome)
		})
	}
}

func readStoredOutcomes(t *testing.T) []storedOutcome {
	t.Helper()

	data, err := fixtureFiles.ReadFile("fixtures/synthetic/paired_outcomes.synthetic.json")
	if err != nil {
		t.Fatal(err)
	}

	var outcomes []storedOutcome

	decodeErr := json.Unmarshal(data, &outcomes)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}

	return outcomes
}

func validateStoredOutcome(t *testing.T, outcome storedOutcome, seen map[string]bool) {
	t.Helper()

	validateStoredOutcomeID(t, outcome.ID, seen)
	validateStoredRequest(t, outcome)
	validateStoredResponse(t, outcome)
	validateStoredRequestBodies(t, outcome)
}

func validateStoredOutcomeID(t *testing.T, outcomeID string, seen map[string]bool) {
	t.Helper()

	if outcomeID == "" || seen[outcomeID] {
		t.Fatalf("empty or duplicate outcome ID %q", outcomeID)
	}

	seen[outcomeID] = true
}

func validateStoredRequest(t *testing.T, outcome storedOutcome) {
	t.Helper()

	if outcome.Operation == "" || outcome.Request.Method != http.MethodPost ||
		outcome.Request.Origin != replayBaseURL || len(outcome.Request.Headers) == 0 ||
		len(outcome.Request.Bodies) == 0 {
		t.Fatalf("incomplete request in %s", outcome.ID)
	}
}

func validateStoredResponse(t *testing.T, outcome storedOutcome) {
	t.Helper()

	if outcome.Response.Fault == "" && outcome.Response.TransportError == "" &&
		(outcome.Response.Status == 0 || len(outcome.Response.Headers) == 0 ||
			(outcome.Response.BodyText == "" && outcome.Response.BodyRepeat == nil)) {
		t.Fatalf("incomplete response in %s", outcome.ID)
	}

	if !isKnownStoredFault(outcome.Response.Fault) {
		t.Fatalf("unknown fault %q in %s", outcome.Response.Fault, outcome.ID)
	}
}

func validateStoredRequestBodies(t *testing.T, outcome storedOutcome) {
	t.Helper()

	for _, body := range outcome.Request.Bodies {
		key, err := requestFixtureKey(body)
		if err != nil {
			t.Fatal(err)
		}

		if key != outcome.Operation {
			t.Fatalf("outcome %s request key %s differs from %s", outcome.ID, key, outcome.Operation)
		}
	}
}

func isKnownStoredFault(fault string) bool {
	switch fault {
	case "", "nil-response", "nil-body", "read-error", storedFaultResponseAndError, "transport-error", "url-error":
		return true
	default:
		return false
	}
}

func replayStoredOutcome(t *testing.T, outcome storedOutcome) {
	t.Helper()

	transport := newReplayTransport()
	transport.expectSequence(outcome.Operation)
	transport.configureStep(outcome.Operation, configureStoredReplayOutcome(outcome))

	badRequest := replayFixtureRequest(t, outcome.Request, json.RawMessage(`{"method":"unexpected"}`))
	assertStoredOutcomeRequestRejected(t, transport, badRequest)

	request := replayFixtureRequest(t, outcome.Request, outcome.Request.Bodies[0])
	response, err := transport.RoundTrip(request)
	closeReplayResponseBody(t, response)
	assertStoredOutcomeResult(t, outcome, err)

	err = transport.assertConsumed()
	if err != nil {
		t.Fatal(err)
	}

	duplicateRequest := replayFixtureRequest(t, outcome.Request, outcome.Request.Bodies[0])
	assertStoredOutcomeRequestRejected(t, transport, duplicateRequest)
}

func assertStoredOutcomeRequestRejected(t *testing.T, transport *replayTransport, request *http.Request) {
	t.Helper()

	response, err := transport.RoundTrip(request)
	closeReplayResponseBody(t, response)

	if err == nil || response != nil {
		t.Fatalf("unexpected replay response %v, error %v", response, err)
	}
}

func configureStoredReplayOutcome(outcome storedOutcome) func(*replayStep) {
	return func(step *replayStep) {
		step.outcome = &outcome
		if outcome.Response.TransportError != "" {
			step.transportError = storedTransportFailureError(outcome.Response.TransportError)
		}

		if outcome.Response.Fault == storedFaultResponseAndError {
			step.faultBody = io.NopCloser(strings.NewReader(outcome.Response.BodyText))
		}
	}
}

func assertStoredOutcomeResult(t *testing.T, outcome storedOutcome, err error) {
	t.Helper()

	if outcome.Response.TransportError != "" && err == nil {
		t.Fatal("expected transport error")
	}

	if outcome.Response.TransportError == "" && err != nil {
		t.Fatal(err)
	}
}

func TestReplayFixtureRejectsRequestOutsidePair(t *testing.T) {
	t.Parallel()

	transport := newReplayTransport()
	transport.expectSequence("getDeviceList")

	request := func(origin, token string) *http.Request {
		req := newReplayTestRequest(
			t, http.MethodPost, origin+"?token="+token,
			strings.NewReader(`{"method":"getDeviceList"}`),
		)
		req.Header.Set("Content-Type", "application/json")

		return req
	}

	for _, bad := range []*http.Request{
		request("https://wrong.example.invalid", "test-token"),
		request(replayBaseURL, ""),
		request(replayBaseURL, "wrong-token"),
	} {
		assertReplayRoundTripRejected(t, transport, bad, "mismatched request")
	}

	unexpectedBody := newReplayTestRequest(
		t,
		http.MethodPost,
		replayBaseURL+"?token=test-token",
		strings.NewReader(`{"method":"getDeviceList","unexpected":true}`),
	)

	unexpectedBody.Header.Set("Content-Type", "application/json")

	assertReplayRoundTripRejected(t, transport, unexpectedBody, "mismatched body")

	response, err := transport.RoundTrip(request(replayBaseURL, "test-token"))
	if err != nil {
		closeReplayResponseBody(t, response)
		t.Fatal(err)
	}

	t.Cleanup(func() { closeReplayResponseBody(t, response) })

	if response.StatusCode != http.StatusOK {
		t.Fatalf("response status = %d", response.StatusCode)
	}

	_, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatal(readErr)
	}

	assertionErr := transport.assertConsumed()
	if assertionErr != nil {
		t.Fatal(assertionErr)
	}

	assertReplayRoundTripRejected(t, transport, request(replayBaseURL, "test-token"), "duplicate request")
}

func assertReplayRoundTripRejected(t *testing.T, transport *replayTransport, request *http.Request, message string) {
	t.Helper()

	response, err := transport.RoundTrip(request)
	closeReplayResponseBody(t, response)

	if err == nil || response != nil {
		t.Fatalf("%s: response %v, error %v", message, response, err)
	}
}

func TestReplayFixtureRequiresExpectedCallOrderAndExhaustion(t *testing.T) {
	t.Parallel()

	transport := newReplayTransport()
	transport.expectSequence("login", "getDeviceList")

	assertionErr := transport.assertConsumed()
	if assertionErr == nil {
		t.Fatal("unconsumed replay sequence was accepted")
	}

	request := newReplayTestRequest(
		t,
		http.MethodPost,
		replayBaseURL+"?token=test-token",
		strings.NewReader(`{"method":"getDeviceList"}`),
	)

	request.Header.Set("Content-Type", "application/json")

	response, err := transport.RoundTrip(request)
	closeReplayResponseBody(t, response)

	if err == nil || response != nil {
		t.Fatalf("out-of-order request returned response %v, error %v", response, err)
	}
}

func TestReplayOverrideStillMatchesRequest(t *testing.T) {
	t.Parallel()

	transport := newReplayTransport()
	transport.useResponse("getDeviceList", http.StatusTooManyRequests, []byte(`{"error_code":-20004}`))

	request := newReplayTestRequest(
		t,
		http.MethodPost,
		"https://wrong.example.invalid?token=test-token",
		strings.NewReader(`{"method":"getDeviceList"}`),
	)

	request.Header.Set("Content-Type", "application/json")

	response, err := transport.RoundTrip(request)
	closeReplayResponseBody(t, response)

	if err == nil || response != nil {
		t.Fatalf("override returned response for mismatched request: %v, %v", response, err)
	}
}
