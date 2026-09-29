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
		BodyText   string              `json:"body_text,omitempty"` //nolint:tagliatelle,lll // Stored fixture schema uses snake_case keys.
		BodyRepeat *struct {
			Text  string `json:"text"`
			Count int    `json:"count"`
		} `json:"body_repeat,omitempty"` //nolint:tagliatelle // Stored fixture schema uses snake_case keys.
		TransportError string `json:"transport_error,omitempty"` //nolint:tagliatelle,lll // Stored fixture schema uses snake_case keys.
		Fault          string `json:"fault,omitempty"`
		ErrorURL       string `json:"error_url,omitempty"` //nolint:tagliatelle // Stored fixture schema uses snake_case keys.
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

type replayAttempt struct {
	requestBody []byte
	key         string
	stepIndex   int
	step        replayStep
}

type replayDiagnosticError string

const storedFaultResponseAndError = "response-and-error"

func (err replayDiagnosticError) Error() string {
	return string(err)
}

const (
	errMissingRequestMethod replayDiagnosticError = "TP-Link request has no method"
	errMissingDeviceCommand replayDiagnosticError = "TP-Link device request has no command"
)

func replayDiagnosticErrorf(format string, values ...any) error {
	return replayDiagnosticError(fmt.Sprintf(format, values...))
}

type storedTransportFailureError string

func (err storedTransportFailureError) Error() string {
	return string(err)
}

func newReplayTransport() *replayTransport {
	return new(replayTransport)
}

func (transport *replayTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	attempt, err := transport.prepareReplayAttempt(request)
	if err != nil {
		return nil, err
	}

	return transport.completeReplayAttempt(request, attempt)
}

func (transport *replayTransport) prepareReplayAttempt(request *http.Request) (replayAttempt, error) {
	var requestBody []byte

	if request.Body != nil {
		var err error

		requestBody, err = io.ReadAll(request.Body)
		if err != nil {
			return replayAttempt{}, fmt.Errorf("read request body: %w", err)
		}
	}

	key, err := requestFixtureKey(requestBody)
	if err != nil {
		return replayAttempt{}, err
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

		return replayAttempt{}, replayDiagnosticErrorf("unexpected extra replay request for operation %q", key)
	}

	attempt := replayAttempt{
		requestBody: requestBody,
		key:         key,
		stepIndex:   transport.matchedCalls,
		step:        transport.steps[transport.matchedCalls],
	}
	transport.mu.Unlock()

	if attempt.step.operation != key {
		return replayAttempt{}, replayDiagnosticErrorf(
			"replay operation %q is out of order; expected %q", key, attempt.step.operation,
		)
	}

	return attempt, nil
}

func (transport *replayTransport) completeReplayAttempt(
	request *http.Request,
	attempt replayAttempt,
) (*http.Response, error) {
	exchange, fixturePath, err := loadReplayExchange(attempt.step, attempt.key)
	if err != nil {
		return nil, err
	}

	err = configureReplayRequest(&exchange, attempt.step)
	if err != nil {
		return nil, fmt.Errorf("replay fixture %q request mismatch: %w", fixturePath, err)
	}

	err = matchFixtureRequest(request, attempt.requestBody, attempt.key, exchange)
	if err != nil {
		return nil, fmt.Errorf("replay fixture %q request mismatch: %w", fixturePath, err)
	}

	err = transport.markAttemptMatched(attempt)
	if err != nil {
		return nil, err
	}

	return responseForReplayStep(attempt.step, exchange)
}

func loadReplayExchange(step replayStep, key string) (fixtureExchange, string, error) {
	if step.outcome != nil {
		var exchange fixtureExchange

		exchange.Request = step.outcome.Request

		return exchange, "paired_outcomes.synthetic.json#" + step.outcome.ID, nil
	}

	fixturePath := step.fixture
	if fixturePath == "" {
		fixturePath = step.operation
	}

	fixturePath = "fixtures/synthetic/synthetic_tplink_" + fixturePath + ".json"

	fixtureBody, err := fixtureFiles.ReadFile(fixturePath)
	if err != nil {
		return fixtureExchange{}, "", fmt.Errorf("no replay pair for TP-Link operation %q: %w", key, err)
	}

	var exchange fixtureExchange

	decodeErr := json.Unmarshal(fixtureBody, &exchange)
	if decodeErr != nil {
		return fixtureExchange{}, "", fmt.Errorf("decode replay fixture %q: %w", fixturePath, decodeErr)
	}

	return exchange, fixturePath, nil
}

func configureReplayRequest(exchange *fixtureExchange, step replayStep) error {
	if step.token != "" {
		exchange.Request.Query["token"] = []string{step.token}
	}

	if step.variant < 0 || step.variant >= len(exchange.Request.Bodies) {
		return replayDiagnosticErrorf("fixture has no request variant %d", step.variant)
	}

	exchange.Request.Bodies = exchange.Request.Bodies[step.variant : step.variant+1]
	exchange.Request.Operations = []string{step.operation}

	return nil
}

func (transport *replayTransport) markAttemptMatched(attempt replayAttempt) error {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if transport.matchedCalls != attempt.stepIndex {
		return replayDiagnosticErrorf(
			"duplicate or out-of-order concurrent replay request for operation %q", attempt.key,
		)
	}

	transport.matchedCalls++

	return nil
}

func responseForReplayStep(step replayStep, exchange fixtureExchange) (*http.Response, error) {
	if step.outcome != nil {
		fault := responseForStoredFault(step)
		if fault.handled {
			return fault.response, fault.err
		}

		if step.outcome.Response.TransportError != "" {
			return nil, step.transportError
		}

		return responseForStoredOutcome(step.outcome), nil
	}

	return newReplayHTTPResponse(
		exchange.Response.Status,
		http.Header(exchange.Response.Headers),
		io.NopCloser(bytes.NewReader(exchange.Response.Body)),
	), nil
}

type replayFaultResult struct {
	response *http.Response
	err      error
	handled  bool
}

func responseForStoredFault(step replayStep) replayFaultResult {
	switch step.outcome.Response.Fault {
	case "nil-response":
		return replayFaultResult{response: nil, err: nil, handled: true}
	case "nil-body":
		//nolint:bodyclose // This fault fixture intentionally returns a response with no body.
		return replayFaultResult{
			response: newReplayHTTPResponse(step.outcome.Response.Status, nil, nil),
			err:      nil,
			handled:  true,
		}
	case "read-error":
		return replayFaultResult{
			//nolint:bodyclose // The replayed client owns and closes this response body.
			response: newReplayHTTPResponse(step.outcome.Response.Status, nil, readErrorBody{}),
			err:      nil,
			handled:  true,
		}
	case storedFaultResponseAndError:
		return replayFaultResult{
			//nolint:bodyclose // The replayed client owns and closes this response body.
			response: newReplayHTTPResponse(step.outcome.Response.Status, nil, step.faultBody),
			err:      step.transportError,
			handled:  true,
		}
	case "transport-error":
		return replayFaultResult{response: nil, err: step.transportError, handled: true}
	case "url-error":
		return replayFaultResult{
			response: nil,
			err:      &url.Error{Op: "Post", URL: step.outcome.Response.ErrorURL, Err: step.transportError},
			handled:  true,
		}
	case "":
		return replayFaultResult{response: nil, err: nil, handled: false}
	default:
		err := replayDiagnosticErrorf("unknown replay fault %q", step.outcome.Response.Fault)

		return replayFaultResult{response: nil, err: err, handled: true}
	}
}

func responseForStoredOutcome(outcome *storedOutcome) *http.Response {
	body := []byte(outcome.Response.BodyText)
	if repeat := outcome.Response.BodyRepeat; repeat != nil {
		body = []byte(strings.Repeat(repeat.Text, repeat.Count))
	}

	return newReplayHTTPResponse(
		outcome.Response.Status,
		http.Header(outcome.Response.Headers),
		io.NopCloser(bytes.NewReader(body)),
	)
}

func newReplayHTTPResponse(status int, header http.Header, body io.ReadCloser) *http.Response {
	response := new(http.Response)
	response.StatusCode = status
	response.Header = header
	response.Body = body

	return response
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

func (transport *replayTransport) expectVariant(variant int) {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	transport.steps[0].variant = variant
}

func (transport *replayTransport) assertConsumed() error {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if transport.matchedCalls != len(transport.steps) {
		return replayDiagnosticErrorf("matched %d of %d expected replay calls", transport.matchedCalls, len(transport.steps))
	}

	return nil
}

func matchFixtureRequest(request *http.Request, body []byte, operation string, exchange fixtureExchange) error {
	want := exchange.Request
	if request.Method != want.Method || request.URL.Scheme+"://"+request.URL.Host != want.Origin ||
		request.URL.EscapedPath() != want.Path {
		return replayDiagnosticErrorf(
			"got %s %s, want %s %s%s",
			request.Method,
			request.URL,
			want.Method,
			want.Origin,
			want.Path,
		)
	}

	err := matchFixtureQuery(request.URL.Query(), want.Query)
	if err != nil {
		return err
	}

	err = matchFixtureHeaders(request.Header, want.Headers)
	if err != nil {
		return err
	}

	if !containsString(want.Operations, operation) {
		return replayDiagnosticErrorf("operation %q is not in %v", operation, want.Operations)
	}

	return matchFixtureBody(body, want.Bodies)
}

func matchFixtureQuery(actual, expected url.Values) error {
	if len(actual) != len(expected) {
		return replayDiagnosticErrorf("query = %v, want %v", actual, expected)
	}

	for name, values := range expected {
		err := compareFixtureValues("query", name, actual[name], values)
		if err != nil {
			return err
		}
	}

	return nil
}

func matchFixtureHeaders(actual http.Header, expected map[string][]string) error {
	for name, values := range expected {
		err := compareFixtureValues("header", name, actual.Values(name), values)
		if err != nil {
			return err
		}
	}

	return nil
}

func compareFixtureValues(label, name string, actual, expected []string) error {
	if len(actual) != len(expected) {
		return replayDiagnosticErrorf("%s %s = %v, want %v", label, name, actual, expected)
	}

	for index, value := range expected {
		if actual[index] != value {
			return replayDiagnosticErrorf("%s %s[%d] = %q, want %q", label, name, index, actual[index], value)
		}
	}

	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}

	return false
}

func matchFixtureBody(body []byte, expectedBodies []json.RawMessage) error {
	var actual any

	decodeErr := json.Unmarshal(body, &actual)
	if decodeErr != nil {
		return fmt.Errorf("decode request body: %w", decodeErr)
	}

	for _, expected := range expectedBodies {
		var candidate any

		candidateErr := json.Unmarshal(expected, &candidate)
		if candidateErr != nil {
			return fmt.Errorf("decode expected request body: %w", candidateErr)
		}

		if reflect.DeepEqual(actual, candidate) {
			return nil
		}
	}

	return replayDiagnosticErrorf("request body did not match any of %d stored variants", len(expectedBodies))
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
	outcome := findStoredFaultOutcome(id)
	validateStoredFaultConfiguration(id, outcome, cause, body)

	transport.mu.Lock()
	transport.configureStep(outcome.Operation, func(step *replayStep) {
		step.outcome = &outcome
		step.transportError = cause
		step.faultBody = body
	})
	transport.mu.Unlock()
}

func findStoredOutcome(operation string, status int, body []byte, transportError string) storedOutcome {
	for _, outcome := range readStoredOutcomesForTransport() {
		if outcome.Operation != operation {
			continue
		}

		if storedOutcomeMatchesTransportError(outcome, transportError) {
			return outcome
		}

		if storedOutcomeMatchesResponse(outcome, status, body, transportError) {
			return outcome
		}
	}

	panic(fmt.Sprintf("no stored paired outcome for %s status %d", operation, status))
}

func readStoredOutcomesForTransport() []storedOutcome {
	data, err := fixtureFiles.ReadFile("fixtures/synthetic/paired_outcomes.synthetic.json")
	if err != nil {
		panic(err)
	}

	var outcomes []storedOutcome

	decodeErr := json.Unmarshal(data, &outcomes)
	if decodeErr != nil {
		panic(decodeErr)
	}

	return outcomes
}

func findStoredFaultOutcome(faultID string) storedOutcome {
	for _, outcome := range readStoredOutcomesForTransport() {
		if outcome.ID == faultID {
			return outcome
		}
	}

	panic("missing stored fault outcome: " + faultID)
}

func validateStoredFaultConfiguration(faultID string, outcome storedOutcome, cause error, body io.ReadCloser) {
	if outcome.Response.Fault == "" {
		panic("stored outcome is not a fault: " + faultID)
	}

	if !storedTransportErrorMatches(outcome.Response.TransportError, cause) {
		panic("fault cause differs from stored outcome: " + faultID)
	}

	if (body != nil) != (outcome.Response.Fault == storedFaultResponseAndError) {
		panic("fault body differs from stored outcome: " + faultID)
	}
}

func storedTransportErrorMatches(want string, cause error) bool {
	if cause == nil {
		return want == ""
	}

	return cause.Error() == want
}

func storedOutcomeMatchesTransportError(outcome storedOutcome, transportError string) bool {
	return transportError != "" && outcome.Response.TransportError == transportError
}

func storedOutcomeMatchesResponse(outcome storedOutcome, status int, body []byte, transportError string) bool {
	if transportError != "" || outcome.Response.TransportError != "" || outcome.Response.Status != status {
		return false
	}

	return bytes.Equal(storedOutcomeBody(outcome), body)
}

func storedOutcomeBody(outcome storedOutcome) []byte {
	if repeat := outcome.Response.BodyRepeat; repeat != nil {
		return []byte(strings.Repeat(repeat.Text, repeat.Count))
	}

	return []byte(outcome.Response.BodyText)
}

func (transport *replayTransport) configureStep(operation string, configure func(*replayStep)) {
	if len(transport.steps) == 0 {
		transport.steps = []replayStep{{
			operation:      operation,
			fixture:        "",
			token:          "",
			variant:        0,
			outcome:        nil,
			transportError: nil,
			faultBody:      nil,
		}}
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

	err := json.Unmarshal(body, &request)
	if err != nil {
		return "", fmt.Errorf("decode TP-Link request envelope: %w", err)
	}

	if request.Method == "" {
		return "", errMissingRequestMethod
	}

	if request.Method != "passthrough" {
		return request.Method, nil
	}

	var params struct {
		RequestData string `json:"requestData"`
	}

	err = json.Unmarshal(request.Params, &params)
	if err != nil {
		return "", fmt.Errorf("decode TP-Link passthrough parameters: %w", err)
	}

	var command map[string]map[string]json.RawMessage

	decodeErr := json.Unmarshal([]byte(params.RequestData), &command)
	if decodeErr != nil {
		return "", fmt.Errorf("decode TP-Link passthrough command: %w", decodeErr)
	}

	for namespace, methods := range command {
		if separator := strings.LastIndex(namespace, "."); separator >= 0 {
			namespace = namespace[separator+1:]
		}

		for method := range methods {
			return "passthrough_" + namespace + "_" + method, nil
		}
	}

	return "", errMissingDeviceCommand
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
