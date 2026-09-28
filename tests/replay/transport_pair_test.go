package replay_test

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"
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
		})
	}
}

func TestReplayFixtureRejectsRequestOutsidePair(t *testing.T) {
	transport := newReplayTransport()
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
