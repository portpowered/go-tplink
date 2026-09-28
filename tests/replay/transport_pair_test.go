package replay_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

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
