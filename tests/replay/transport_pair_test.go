package replay_test

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestEverySyntheticReplayFixtureHasRequestAndResponse(t *testing.T) {
	paths, err := fs.Glob(fixtureFiles, "fixtures/synthetic/synthetic_tplink_*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no synthetic replay fixtures")
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			data, err := fixtureFiles.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var exchange fixtureExchange
			if err := json.Unmarshal(data, &exchange); err != nil {
				t.Fatal(err)
			}
			if exchange.Request.Method == "" || exchange.Request.Origin == "" || len(exchange.Request.Operations) == 0 || len(exchange.Request.Bodies) == 0 || exchange.Response.Status == 0 || len(exchange.Response.Body) == 0 {
				t.Fatal("fixture is missing its request or response")
			}
			stem := strings.TrimSuffix(strings.TrimPrefix(path, "fixtures/synthetic/synthetic_tplink_"), ".json")
			for variant, body := range exchange.Request.Bodies {
				t.Run("variant "+strconv.Itoa(variant), func(t *testing.T) {
					operation, err := requestFixtureKey(body)
					if err != nil {
						t.Fatal(err)
					}
					transport := newReplayTransport()
					transport.expectSequence(operation)
					transport.useFixture(operation, stem)
					transport.expectVariant(0, variant)
					endpoint := exchange.Request.Origin + exchange.Request.Path
					query := url.Values(exchange.Request.Query)
					if len(query) > 0 {
						endpoint += "?" + query.Encode()
					}
					makeRequest := func() *http.Request {
						t.Helper()
						request, err := http.NewRequest(exchange.Request.Method, endpoint, strings.NewReader(string(body)))
						if err != nil {
							t.Fatal(err)
						}
						request.Header = http.Header(exchange.Request.Headers)
						return request
					}
					request := makeRequest()
					response, err := transport.RoundTrip(request)
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					got, err := io.ReadAll(response.Body)
					if err != nil {
						t.Fatal(err)
					}
					if response.StatusCode != exchange.Response.Status || !bytes.Equal(got, exchange.Response.Body) || !reflect.DeepEqual(response.Header, http.Header(exchange.Response.Headers)) {
						t.Fatalf("response metadata/body differs for %s variant %d", stem, variant)
					}
					if err := transport.assertConsumed(); err != nil {
						t.Fatal(err)
					}
					if duplicate, err := transport.RoundTrip(makeRequest()); err == nil || duplicate != nil {
						t.Fatalf("variant replayed twice: %v, %v", duplicate, err)
					}
				})
			}
		})
	}
}

func TestPublicMethodReplayInventory(t *testing.T) {
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
	for i := 0; i < typeOf.NumMethod(); i++ {
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
		if err := json.Unmarshal(data, &pair); err != nil {
			t.Fatal(err)
		}
		if entry.variant >= len(pair.Request.Bodies) {
			t.Errorf("%s variant %d is absent", name, entry.variant)
		}
	}
}

func TestEverySyntheticFailureOutcomeHasStoredRequestAndResponse(t *testing.T) {
	data, err := fixtureFiles.ReadFile("fixtures/synthetic/paired_outcomes.synthetic.json")
	if err != nil {
		t.Fatal(err)
	}
	var outcomes []storedOutcome
	if err := json.Unmarshal(data, &outcomes); err != nil {
		t.Fatal(err)
	}
	if len(outcomes) == 0 {
		t.Fatal("no stored failure outcomes")
	}
	seen := map[string]bool{}
	for _, outcome := range outcomes {
		if outcome.ID == "" || seen[outcome.ID] {
			t.Fatalf("empty or duplicate outcome ID %q", outcome.ID)
		}
		seen[outcome.ID] = true
		if outcome.Operation == "" || outcome.Request.Method != "POST" || outcome.Request.Origin != replayBaseURL || len(outcome.Request.Headers) == 0 || len(outcome.Request.Bodies) == 0 {
			t.Fatalf("incomplete request in %s", outcome.ID)
		}
		if outcome.Response.Fault == "" && outcome.Response.TransportError == "" && (outcome.Response.Status == 0 || len(outcome.Response.Headers) == 0 || (outcome.Response.BodyText == "" && outcome.Response.BodyRepeat == nil)) {
			t.Fatalf("incomplete response in %s", outcome.ID)
		}
		if outcome.Response.Fault != "" && outcome.Response.Fault != "nil-response" && outcome.Response.Fault != "nil-body" && outcome.Response.Fault != "read-error" && outcome.Response.Fault != "response-and-error" && outcome.Response.Fault != "transport-error" && outcome.Response.Fault != "url-error" {
			t.Fatalf("unknown fault %q in %s", outcome.Response.Fault, outcome.ID)
		}
		for _, body := range outcome.Request.Bodies {
			key, err := requestFixtureKey(body)
			if err != nil {
				t.Fatal(err)
			}
			if key != outcome.Operation {
				t.Fatalf("outcome %s request key %s differs from %s", outcome.ID, key, outcome.Operation)
			}
		}
		t.Run(outcome.ID, func(t *testing.T) {
			transport := newReplayTransport()
			transport.expectSequence(outcome.Operation)
			transport.configureStep(outcome.Operation, func(step *replayStep) {
				step.outcome = &outcome
				if outcome.Response.TransportError != "" {
					step.transportError = errors.New(outcome.Response.TransportError)
				}
				if outcome.Response.Fault == "response-and-error" {
					step.faultBody = io.NopCloser(strings.NewReader(outcome.Response.BodyText))
				}
			})
			makeRequest := func(body json.RawMessage) *http.Request {
				t.Helper()
				endpoint := outcome.Request.Origin + outcome.Request.Path
				if query := url.Values(outcome.Request.Query).Encode(); query != "" {
					endpoint += "?" + query
				}
				request, err := http.NewRequest(outcome.Request.Method, endpoint, bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				request.Header = http.Header(outcome.Request.Headers)
				return request
			}
			bad := makeRequest(json.RawMessage(`{"method":"unexpected"}`))
			if response, err := transport.RoundTrip(bad); err == nil || response != nil {
				t.Fatalf("mismatched outcome request returned %v, %v", response, err)
			}
			response, err := transport.RoundTrip(makeRequest(outcome.Request.Bodies[0]))
			if outcome.Response.TransportError != "" && err == nil {
				t.Fatal("expected transport error")
			}
			if outcome.Response.TransportError == "" && err != nil {
				t.Fatal(err)
			}
			if response != nil && response.Body != nil {
				response.Body.Close()
			}
			if err := transport.assertConsumed(); err != nil {
				t.Fatal(err)
			}
			if duplicate, err := transport.RoundTrip(makeRequest(outcome.Request.Bodies[0])); err == nil || duplicate != nil {
				t.Fatalf("outcome replayed twice: %v, %v", duplicate, err)
			}
		})
	}
}

func TestReplayFixtureRejectsRequestOutsidePair(t *testing.T) {
	transport := newReplayTransport()
	transport.expectSequence("getDeviceList")
	request := func(origin, token string) *http.Request {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, origin+"?token="+token, strings.NewReader(`{"method":"getDeviceList"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req
	}
	for _, bad := range []*http.Request{
		request("https://wrong.example.invalid", "test-token"),
		request(replayBaseURL, ""),
		request(replayBaseURL, "wrong-token"),
	} {
		if response, err := transport.RoundTrip(bad); err == nil || response != nil {
			t.Fatalf("mismatched request returned response %v, error %v", response, err)
		}
	}
	unexpectedBody, err := http.NewRequest(http.MethodPost, replayBaseURL+"?token=test-token", strings.NewReader(`{"method":"getDeviceList","unexpected":true}`))
	if err != nil {
		t.Fatal(err)
	}
	unexpectedBody.Header.Set("Content-Type", "application/json")
	if response, err := transport.RoundTrip(unexpectedBody); err == nil || response != nil {
		t.Fatalf("mismatched body returned response %v, error %v", response, err)
	}
	response, err := transport.RoundTrip(request(replayBaseURL, "test-token"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response status = %d", response.StatusCode)
	}
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	if err := transport.assertConsumed(); err != nil {
		t.Fatal(err)
	}
	if response, err := transport.RoundTrip(request(replayBaseURL, "test-token")); err == nil || response != nil {
		t.Fatalf("duplicate request returned response %v, error %v", response, err)
	}
}

func TestReplayFixtureRequiresExpectedCallOrderAndExhaustion(t *testing.T) {
	transport := newReplayTransport()
	transport.expectSequence("login", "getDeviceList")
	if err := transport.assertConsumed(); err == nil {
		t.Fatal("unconsumed replay sequence was accepted")
	}
	request, err := http.NewRequest(http.MethodPost, replayBaseURL+"?token=test-token", strings.NewReader(`{"method":"getDeviceList"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if response, err := transport.RoundTrip(request); err == nil || response != nil {
		t.Fatalf("out-of-order request returned response %v, error %v", response, err)
	}
}

func TestReplayOverrideStillMatchesRequest(t *testing.T) {
	transport := newReplayTransport()
	transport.useResponse("getDeviceList", http.StatusTooManyRequests, []byte(`{"error_code":-20004}`))
	request, err := http.NewRequest(http.MethodPost, "https://wrong.example.invalid?token=test-token", strings.NewReader(`{"method":"getDeviceList"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if response, err := transport.RoundTrip(request); err == nil || response != nil {
		t.Fatalf("override returned response for mismatched request: %v, %v", response, err)
	}
}
