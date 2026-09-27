package replay_test

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

//go:embed fixtures/synthetic/*.json
var fixtureFiles embed.FS

type recordedRequest struct {
	Method string
	URL    *url.URL
	Header http.Header
	Body   []byte
}

type replayResponse struct {
	status int
	body   []byte
}

// replayTransport replays deterministic TP-Link response fixtures at the HTTP
// seam and records the actual requests made by the provider client.
type replayTransport struct {
	mu sync.Mutex

	requests       []recordedRequest
	fixtureRoutes  map[string]string
	responseRoutes map[string]replayResponse
	errorRoutes    map[string]error
}

func newReplayTransport() *replayTransport {
	return &replayTransport{
		fixtureRoutes:  make(map[string]string),
		responseRoutes: make(map[string]replayResponse),
		errorRoutes:    make(map[string]error),
	}
}

func (transport *replayTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var requestBody []byte
	if request.Body != nil {
		var err error
		requestBody, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
	}

	key, err := requestFixtureKey(requestBody)
	if err != nil {
		return nil, err
	}

	transport.mu.Lock()
	transport.requests = append(transport.requests, recordedRequest{
		Method: request.Method,
		URL:    cloneURL(request.URL),
		Header: request.Header.Clone(),
		Body:   append([]byte(nil), requestBody...),
	})
	responseOverride, hasOverride := transport.responseRoutes[key]
	responseError, hasError := transport.errorRoutes[key]
	fixtureStem, hasFixtureRoute := transport.fixtureRoutes[key]
	transport.mu.Unlock()

	if hasError {
		return nil, responseError
	}
	if hasOverride {
		return jsonResponse(responseOverride.status, responseOverride.body), nil
	}
	if !hasFixtureRoute {
		fixtureStem = key
	}

	fixturePath := "fixtures/synthetic/synthetic_tplink_" + fixtureStem + ".json"
	fixtureBody, err := fixtureFiles.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("no replay response for TP-Link operation %q: %w", key, err)
	}
	return jsonResponse(http.StatusOK, fixtureBody), nil
}

func (transport *replayTransport) useFixture(operationKey, fixtureStem string) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.fixtureRoutes[operationKey] = fixtureStem
	delete(transport.responseRoutes, operationKey)
	delete(transport.errorRoutes, operationKey)
}

func (transport *replayTransport) useResponse(operationKey string, status int, body []byte) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.responseRoutes[operationKey] = replayResponse{
		status: status,
		body:   append([]byte(nil), body...),
	}
	delete(transport.fixtureRoutes, operationKey)
	delete(transport.errorRoutes, operationKey)
}

func (transport *replayTransport) useError(operationKey string, err error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.errorRoutes[operationKey] = err
	delete(transport.fixtureRoutes, operationKey)
	delete(transport.responseRoutes, operationKey)
}

func (transport *replayTransport) recordedRequests() []recordedRequest {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	requests := make([]recordedRequest, len(transport.requests))
	copy(requests, transport.requests)
	return requests
}

func requestFixtureKey(body []byte) (string, error) {
	var request struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return "", fmt.Errorf("decode TP-Link request envelope: %w", err)
	}
	if request.Method == "" {
		return "", fmt.Errorf("TP-Link request has no method")
	}
	if request.Method != "passthrough" {
		return request.Method, nil
	}

	var params struct {
		RequestData string `json:"requestData"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return "", fmt.Errorf("decode TP-Link passthrough parameters: %w", err)
	}

	var command map[string]map[string]json.RawMessage
	if err := json.Unmarshal([]byte(params.RequestData), &command); err != nil {
		return "", fmt.Errorf("decode TP-Link passthrough command: %w", err)
	}
	for namespace, methods := range command {
		if separator := strings.LastIndex(namespace, "."); separator >= 0 {
			namespace = namespace[separator+1:]
		}
		for method := range methods {
			return "passthrough_" + namespace + "_" + method, nil
		}
	}
	return "", fmt.Errorf("TP-Link passthrough request has no command")
}

func jsonResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (doer httpDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}
