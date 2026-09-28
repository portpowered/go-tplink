package replay_test

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
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

type fixtureRequest struct {
	Method     string              `json:"method"`
	Origin     string              `json:"origin"`
	Path       string              `json:"path"`
	Query      map[string][]string `json:"query"`
	Headers    map[string][]string `json:"headers"`
	Operations []string            `json:"operations"`
	Bodies     []json.RawMessage   `json:"bodies"`
}

type fixtureExchange struct {
	Request  fixtureRequest `json:"request"`
	Response struct {
		Status  int                 `json:"status"`
		Headers map[string][]string `json:"headers"`
		Body    json.RawMessage     `json:"body"`
	} `json:"response"`
}

type storedOutcome struct {
	ID        string         `json:"id"`
	Operation string         `json:"operation"`
	Request   fixtureRequest `json:"request"`
	Response  struct {
		Status     int                 `json:"status"`
		Headers    map[string][]string `json:"headers"`
		BodyText   string              `json:"body_text,omitempty"`
		BodyRepeat *struct {
			Text  string `json:"text"`
			Count int    `json:"count"`
		} `json:"body_repeat,omitempty"`
		TransportError string `json:"transport_error,omitempty"`
		Fault          string `json:"fault,omitempty"`
		ErrorURL       string `json:"error_url,omitempty"`
	} `json:"response"`
}

// replayTransport replays deterministic TP-Link response fixtures at the HTTP
// seam and records the actual requests made by the provider client.
type replayTransport struct {
	mu sync.Mutex

	requests     []recordedRequest
	steps        []replayStep
	matchedCalls int
}

type replayStep struct {
	operation      string
	fixture        string
	token          string
	variant        int
	outcome        *storedOutcome
	transportError error
	faultBody      io.ReadCloser
}

func newReplayTransport() *replayTransport {
	return &replayTransport{}
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
	if transport.matchedCalls >= len(transport.steps) {
		transport.mu.Unlock()
		return nil, fmt.Errorf("unexpected extra replay request for operation %q", key)
	}
	stepIndex := transport.matchedCalls
	step := transport.steps[stepIndex]
	transport.mu.Unlock()
	if step.operation != key {
		return nil, fmt.Errorf("replay operation %q is out of order; expected %q", key, step.operation)
	}
	var exchange fixtureExchange
	fixturePath := step.fixture
	if step.outcome != nil {
		fixturePath = "paired_outcomes.synthetic.json#" + step.outcome.ID
		exchange.Request = step.outcome.Request
	} else {
		if fixturePath == "" {
			fixturePath = step.operation
		}
		fixturePath = "fixtures/synthetic/synthetic_tplink_" + fixturePath + ".json"
		fixtureBody, err := fixtureFiles.ReadFile(fixturePath)
		if err != nil {
			return nil, fmt.Errorf("no replay pair for TP-Link operation %q: %w", key, err)
		}
		if err := json.Unmarshal(fixtureBody, &exchange); err != nil {
			return nil, fmt.Errorf("decode replay fixture %q: %w", fixturePath, err)
		}
	}
	if step.token != "" {
		exchange.Request.Query["token"] = []string{step.token}
	}
	if step.variant < 0 || step.variant >= len(exchange.Request.Bodies) {
		return nil, fmt.Errorf("fixture %q has no request variant %d", fixturePath, step.variant)
	}
	exchange.Request.Bodies = exchange.Request.Bodies[step.variant : step.variant+1]
	exchange.Request.Operations = []string{step.operation}
	if err := matchFixtureRequest(request, requestBody, key, exchange); err != nil {
		return nil, fmt.Errorf("replay fixture %q request mismatch: %w", fixturePath, err)
	}
	transport.mu.Lock()
	if transport.matchedCalls != stepIndex {
		transport.mu.Unlock()
		return nil, fmt.Errorf("duplicate or out-of-order concurrent replay request for operation %q", key)
	}
	transport.matchedCalls++
	transport.mu.Unlock()
	if step.outcome != nil {
		switch step.outcome.Response.Fault {
		case "nil-response":
			return nil, nil
		case "nil-body":
			return &http.Response{StatusCode: step.outcome.Response.Status}, nil
		case "read-error":
			return &http.Response{StatusCode: step.outcome.Response.Status, Body: readErrorBody{}}, nil
		case "response-and-error":
			return &http.Response{StatusCode: step.outcome.Response.Status, Body: step.faultBody}, step.transportError
		case "transport-error":
			return nil, step.transportError
		case "url-error":
			return nil, &url.Error{Op: "Post", URL: step.outcome.Response.ErrorURL, Err: step.transportError}
		case "":
		default:
			return nil, fmt.Errorf("unknown replay fault %q", step.outcome.Response.Fault)
		}
	}
	if step.outcome != nil && step.outcome.Response.TransportError != "" {
		return nil, step.transportError
	}
	if step.outcome != nil {
		body := []byte(step.outcome.Response.BodyText)
		if repeat := step.outcome.Response.BodyRepeat; repeat != nil {
			body = []byte(strings.Repeat(repeat.Text, repeat.Count))
		}
		return &http.Response{StatusCode: step.outcome.Response.Status, Header: http.Header(step.outcome.Response.Headers), Body: io.NopCloser(bytes.NewReader(body))}, nil
	}
	return &http.Response{
		StatusCode: exchange.Response.Status,
		Header:     http.Header(exchange.Response.Headers),
		Body:       io.NopCloser(bytes.NewReader(exchange.Response.Body)),
	}, nil
}

func (transport *replayTransport) expectCalls(count int) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if count != 0 {
		panic("use expectSequence for positive expected call counts")
	}
	transport.steps = nil
}

func (transport *replayTransport) expectSequence(operations ...string) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.steps = make([]replayStep, len(operations))
	for i, operation := range operations {
		transport.steps[i].operation = operation
	}
}

func (transport *replayTransport) expectToken(index int, token string) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.steps[index].token = token
}

func (transport *replayTransport) expectVariant(index, variant int) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.steps[index].variant = variant
}

func (transport *replayTransport) assertConsumed() error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.matchedCalls != len(transport.steps) {
		return fmt.Errorf("matched %d of %d expected replay calls", transport.matchedCalls, len(transport.steps))
	}
	return nil
}

func matchFixtureRequest(request *http.Request, body []byte, operation string, exchange fixtureExchange) error {
	want := exchange.Request
	if request.Method != want.Method || request.URL.Scheme+"://"+request.URL.Host != want.Origin || request.URL.EscapedPath() != want.Path {
		return fmt.Errorf("got %s %s, want %s %s%s", request.Method, request.URL, want.Method, want.Origin, want.Path)
	}
	query := request.URL.Query()
	if len(query) != len(want.Query) {
		return fmt.Errorf("query = %v, want %v", query, want.Query)
	}
	for name, values := range want.Query {
		got := query[name]
		if len(got) != len(values) {
			return fmt.Errorf("query %s = %v, want %v", name, got, values)
		}
		for i, value := range values {
			if got[i] != value {
				return fmt.Errorf("query %s[%d] = %q, want %q", name, i, got[i], value)
			}
		}
	}
	for name, values := range want.Headers {
		got := request.Header.Values(name)
		if len(got) != len(values) {
			return fmt.Errorf("header %s = %v, want %v", name, got, values)
		}
		for i, value := range values {
			if got[i] != value {
				return fmt.Errorf("header %s[%d] = %q, want %q", name, i, got[i], value)
			}
		}
	}
	matchedOperation := false
	for _, allowed := range want.Operations {
		if operation == allowed {
			matchedOperation = true
			break
		}
	}
	if !matchedOperation {
		return fmt.Errorf("operation %q is not in %v", operation, want.Operations)
	}
	var actual any
	if err := json.Unmarshal(body, &actual); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	for _, expected := range want.Bodies {
		var candidate any
		if err := json.Unmarshal(expected, &candidate); err != nil {
			return fmt.Errorf("decode expected request body: %w", err)
		}
		if reflect.DeepEqual(actual, candidate) {
			return nil
		}
	}
	return fmt.Errorf("request body did not match any of %d stored variants", len(want.Bodies))
}

func (transport *replayTransport) useFixture(operationKey, fixtureStem string) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.configureStep(operationKey, func(step *replayStep) { step.fixture = fixtureStem })
}

func (transport *replayTransport) useResponse(operationKey string, status int, body []byte) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	outcome := findStoredOutcome(operationKey, status, body, "")
	transport.configureStep(operationKey, func(step *replayStep) { step.outcome = &outcome })
}

func (transport *replayTransport) useError(operationKey string, err error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	outcome := findStoredOutcome(operationKey, 0, nil, err.Error())
	transport.configureStep(operationKey, func(step *replayStep) { step.outcome = &outcome; step.transportError = err })
}

func (transport *replayTransport) useFault(id string, cause error, body io.ReadCloser) {
	data, err := fixtureFiles.ReadFile("fixtures/synthetic/paired_outcomes.synthetic.json")
	if err != nil {
		panic(err)
	}
	var outcomes []storedOutcome
	if err := json.Unmarshal(data, &outcomes); err != nil {
		panic(err)
	}
	for _, outcome := range outcomes {
		if outcome.ID != id {
			continue
		}
		if outcome.Response.Fault == "" {
			panic("stored outcome is not a fault: " + id)
		}
		if cause == nil && outcome.Response.TransportError != "" || cause != nil && cause.Error() != outcome.Response.TransportError {
			panic("fault cause differs from stored outcome: " + id)
		}
		if (body != nil) != (outcome.Response.Fault == "response-and-error") {
			panic("fault body differs from stored outcome: " + id)
		}
		transport.mu.Lock()
		transport.configureStep(outcome.Operation, func(step *replayStep) { step.outcome = &outcome; step.transportError = cause; step.faultBody = body })
		transport.mu.Unlock()
		return
	}
	panic("missing stored fault outcome: " + id)
}

func findStoredOutcome(operation string, status int, body []byte, transportError string) storedOutcome {
	data, err := fixtureFiles.ReadFile("fixtures/synthetic/paired_outcomes.synthetic.json")
	if err != nil {
		panic(err)
	}
	var outcomes []storedOutcome
	if err := json.Unmarshal(data, &outcomes); err != nil {
		panic(err)
	}
	for _, outcome := range outcomes {
		if outcome.Operation != operation {
			continue
		}
		if transportError != "" && outcome.Response.TransportError == transportError {
			return outcome
		}
		storedBody := []byte(outcome.Response.BodyText)
		if repeat := outcome.Response.BodyRepeat; repeat != nil {
			storedBody = []byte(strings.Repeat(repeat.Text, repeat.Count))
		}
		if transportError == "" && outcome.Response.TransportError == "" && outcome.Response.Status == status && bytes.Equal(storedBody, body) {
			return outcome
		}
	}
	panic(fmt.Sprintf("no stored paired outcome for %s status %d", operation, status))
}

func (transport *replayTransport) configureStep(operation string, configure func(*replayStep)) {
	if len(transport.steps) == 0 {
		transport.steps = []replayStep{{operation: operation}}
	}
	for i := range transport.steps {
		if transport.steps[i].operation == operation {
			configure(&transport.steps[i])
		}
	}
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
