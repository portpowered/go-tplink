package cloud_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/portpowered/go-tplink/pkg/dependencies/cloud"
	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

const (
	testCloudToken = "synthetic-cloud-token"
	testCloudBody  = `{"value":"request"}`
	testHTTPPost   = "Post"
)

type testHTTPDoer struct {
	response *http.Response
	err      error
	request  *http.Request
	calls    int
}

func newTestHTTPDoer(response *http.Response, err error) *testHTTPDoer {
	return &testHTTPDoer{response: response, err: err, request: nil, calls: 0}
}

func (doer *testHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	doer.request = request
	doer.calls++

	return doer.response, doer.err
}

type trackedBody struct {
	reader  io.Reader
	readErr error
	closed  bool
}

func newTrackedBody(reader io.Reader, readErr error) *trackedBody {
	return &trackedBody{reader: reader, readErr: readErr, closed: false}
}

func (body *trackedBody) Read(buffer []byte) (int, error) {
	if body.readErr != nil {
		return 0, fmt.Errorf("read test response body: %w", body.readErr)
	}

	read, err := body.reader.Read(buffer)
	if err == io.EOF {
		return read, io.EOF
	}

	if err != nil {
		return read, fmt.Errorf("read test response body: %w", err)
	}

	return read, nil
}

func (body *trackedBody) Close() error {
	body.closed = true

	return nil
}

func newResponse(status int, body io.ReadCloser) *http.Response {
	response := new(http.Response)
	response.StatusCode = status
	response.Body = body

	return response
}

func parseTestURL(t *testing.T, value string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}

	return parsed
}

func TestSendBuildsRequestAndClosesResponse(t *testing.T) {
	t.Parallel()

	responseBody := newTrackedBody(strings.NewReader(`{"ok":true}`), nil)
	defer func() { _ = responseBody.Close() }()

	//nolint:bodyclose // Send under test owns this response body; its closure is asserted below.
	doer := newTestHTTPDoer(newResponse(http.StatusOK, responseBody), nil)
	token := "cloud token"
	requestBody := struct {
		Value string `json:"value"`
	}{Value: "request"}

	response, err := cloud.Send(
		context.Background(),
		doer,
		parseTestURL(t, "https://cloud.example"),
		"test",
		requestBody,
		&token,
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(response) != `{"ok":true}` {
		t.Fatalf("Send() body = %s", response)
	}

	assertRequest(t, doer.request)

	if doer.calls != 1 {
		t.Fatalf("Do() calls = %d, want 1", doer.calls)
	}

	if !responseBody.closed {
		t.Fatal("Send() did not close the response body")
	}
}

func assertRequest(t *testing.T, request *http.Request) {
	t.Helper()

	if request == nil {
		t.Fatal("Do() received a nil request")
	}

	if request.Method != dependencymodels.CloudRequestHTTPMethod ||
		request.URL.Path != dependencymodels.CloudRequestPath ||
		request.URL.RawQuery != "token=cloud+token" {
		t.Fatalf("request target = %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
	}

	contentType := request.Header.Get(dependencymodels.CloudRequestContentTypeHeader)
	if contentType != dependencymodels.CloudRequestContentType {
		t.Fatalf("request content type = %q", contentType)
	}

	requestBytes, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(requestBytes) != testCloudBody {
		t.Fatalf("request body = %s", requestBytes)
	}
}

type invalidInputCase struct {
	name    string
	context func() context.Context
	doer    cloud.HTTPDoer
	base    *url.URL
	body    any
	token   *string
	match   func(error) bool
}

func TestSendRejectsInvalidInputsBeforeNetwork(t *testing.T) {
	t.Parallel()

	baseURL := parseTestURL(t, "https://cloud.example")
	emptyToken := ""
	invalidBaseURL := new(url.URL)
	invalidBaseURL.Scheme = "https"
	invalidBaseURL.Host = "["
	tests := []invalidInputCase{
		{
			name: "nil context", context: func() context.Context { return nil },
			doer: newTestHTTPDoer(nil, nil), base: baseURL, body: struct{}{}, token: nil,
			match: tplinkmodels.IsInvalidRequestError,
		},
		{
			name: "nil doer", context: context.Background, doer: nil, base: baseURL,
			body: struct{}{}, token: nil, match: tplinkmodels.IsConfigurationError,
		},
		{
			name: "nil base URL", context: context.Background,
			doer: newTestHTTPDoer(nil, nil), base: nil, body: struct{}{}, token: nil,
			match: tplinkmodels.IsConfigurationError,
		},
		{
			name: "marshal failure", context: context.Background,
			doer: newTestHTTPDoer(nil, nil), base: baseURL, body: make(chan int), token: nil,
			match: tplinkmodels.IsInvalidRequestError,
		},
		{
			name: "empty token", context: context.Background,
			doer: newTestHTTPDoer(nil, nil), base: baseURL, body: struct{}{}, token: &emptyToken,
			match: tplinkmodels.IsTokenNotSetError,
		},
		{
			name: "invalid request URL", context: context.Background,
			doer: newTestHTTPDoer(nil, nil), base: invalidBaseURL, body: struct{}{}, token: nil,
			match: tplinkmodels.IsInvalidRequestError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := cloud.Send(test.context(), test.doer, test.base, "test", test.body, test.token)

			if !test.match(err) {
				t.Fatalf("Send() error = %T %v", err, err)
			}

			if doer, ok := test.doer.(*testHTTPDoer); ok && doer.calls != 0 {
				t.Fatalf("Do() calls = %d, want 0", doer.calls)
			}
		})
	}
}

type responseFailureCase struct {
	name     string
	response *http.Response
	closed   *trackedBody
	match    func(error) bool
}

func TestSendClassifiesResponseFailuresAndClosesBodies(t *testing.T) {
	t.Parallel()

	httpErrorBody := newTrackedBody(strings.NewReader("denied"), nil)

	t.Cleanup(func() { _ = httpErrorBody.Close() })

	readErrorBody := newTrackedBody(nil, io.ErrUnexpectedEOF)
	largeBody := newTrackedBody(strings.NewReader(strings.Repeat("x", (1<<20)+1)), nil)
	tests := []responseFailureCase{
		{
			name: "nil response", response: nil, closed: nil,
			match: tplinkmodels.IsInvalidResponseError,
		},
		{
			name: "http status failure",
			//nolint:bodyclose // Send under test owns this response body; its closure is asserted below.
			response: newResponse(http.StatusForbidden, httpErrorBody),
			closed:   httpErrorBody, match: tplinkmodels.IsHTTPStatusError,
		},
		{
			name: "nil body",
			//nolint:bodyclose // This nil body exercises invalid-response handling.
			response: newResponse(http.StatusOK, nil), closed: nil,
			match: tplinkmodels.IsInvalidResponseError,
		},
		{
			name: "read error",
			//nolint:bodyclose // Send under test owns this response body; its closure is asserted below.
			response: newResponse(http.StatusOK, readErrorBody),
			closed:   readErrorBody, match: tplinkmodels.IsNetworkError,
		},
		{
			name: "body too large",
			//nolint:bodyclose // Send under test owns this response body; its closure is asserted below.
			response: newResponse(http.StatusOK, largeBody),
			closed:   largeBody, match: tplinkmodels.IsResponseTooLargeError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertResponseFailure(t, test)
		})
	}
}

func assertResponseFailure(t *testing.T, test responseFailureCase) {
	t.Helper()

	if test.response != nil && test.response.Body != nil {
		defer func() { _ = test.response.Body.Close() }()
	}

	doer := newTestHTTPDoer(test.response, nil)

	_, err := cloud.Send(
		context.Background(),
		doer,
		parseTestURL(t, "https://cloud.example"),
		"test",
		struct{}{},
		nil,
	)
	if !test.match(err) {
		t.Fatalf("Send() error = %T %v", err, err)
	}

	if doer.calls != 1 {
		t.Fatalf("Do() calls = %d, want 1", doer.calls)
	}

	if test.closed != nil && !test.closed.closed {
		t.Fatal("Send() did not close the response body")
	}
}

func TestSendRedactsTokensFromTransportErrors(t *testing.T) {
	t.Parallel()

	transportError := &url.Error{
		Op: testHTTPPost, URL: "https://cloud.example/?token=" + testCloudToken,
		Err: io.ErrClosedPipe,
	}
	responseBody := newTrackedBody(strings.NewReader("ignored"), nil)

	//nolint:bodyclose // Send under test owns this response body; its closure is asserted below.
	doer := newTestHTTPDoer(newResponse(http.StatusBadGateway, responseBody), transportError)
	token := testCloudToken

	_, err := cloud.Send(
		context.Background(),
		doer,
		parseTestURL(t, "https://cloud.example"),
		"test",
		struct{}{},
		&token,
	)

	if !tplinkmodels.IsNetworkError(err) || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Send() error = %T %v", err, err)
	}

	if strings.Contains(err.Error(), testCloudToken) {
		t.Fatalf("network error exposed token: %v", err)
	}

	requestError := transportRequestError(t, err)

	parsedErrorURL := parseTestURL(t, requestError.URL)

	if strings.Contains(requestError.URL, testCloudToken) ||
		parsedErrorURL.Query().Get(dependencymodels.TokenQueryKey) != "[REDACTED]" {
		t.Fatalf("transport URL was not redacted: %s", requestError.URL)
	}

	if !responseBody.closed {
		t.Fatal("Send() did not close response body accompanying a transport error")
	}
}

func transportRequestError(t *testing.T, err error) *url.Error {
	t.Helper()

	var networkError *tplinkmodels.NetworkError
	if !errors.As(err, &networkError) {
		t.Fatalf("errors.As() did not find NetworkError in %T", err)
	}

	var requestError *url.Error
	if !errors.As(networkError, &requestError) {
		t.Fatalf("errors.As() did not find URL error in %T", networkError.Err)
	}

	return requestError
}

func TestSendRedactsMalformedTransportURL(t *testing.T) {
	t.Parallel()

	transportError := &url.Error{
		Op: testHTTPPost, URL: "://invalid?token=" + testCloudToken,
		Err: io.EOF,
	}
	doer := newTestHTTPDoer(nil, transportError)
	token := testCloudToken
	_, err := cloud.Send(
		context.Background(),
		doer,
		parseTestURL(t, "https://cloud.example"),
		"test",
		struct{}{},
		&token,
	)

	requestError := transportRequestError(t, err)

	if strings.Contains(requestError.URL, testCloudToken) || !strings.Contains(requestError.URL, "?[REDACTED]") {
		t.Fatalf("malformed transport URL was not redacted: %s", requestError.URL)
	}
}

func TestSendPreservesMalformedTransportURLWithoutQuery(t *testing.T) {
	t.Parallel()

	transportError := &url.Error{Op: testHTTPPost, URL: "://invalid", Err: io.EOF}
	doer := newTestHTTPDoer(nil, transportError)
	token := testCloudToken

	_, err := cloud.Send(
		context.Background(),
		doer,
		parseTestURL(t, "https://cloud.example"),
		"test",
		struct{}{},
		&token,
	)
	requestError := transportRequestError(t, err)

	if requestError.URL != "://invalid" {
		t.Fatalf("malformed transport URL = %q, want unchanged URL", requestError.URL)
	}
}

func TestSendPreservesNonURLTransportError(t *testing.T) {
	t.Parallel()

	doer := newTestHTTPDoer(nil, io.ErrClosedPipe)
	_, err := cloud.Send(
		context.Background(),
		doer,
		parseTestURL(t, "https://cloud.example"),
		"test",
		struct{}{},
		nil,
	)

	if !tplinkmodels.IsNetworkError(err) || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Send() error = %T %v", err, err)
	}
}

type cloudErrorCase struct {
	name  string
	code  int
	match func(error) bool
}

func TestCheckErrorMapsSchemaCodesAndMalformedResponses(t *testing.T) {
	t.Parallel()

	tests := []cloudErrorCase{
		{name: "success", code: dependencymodels.CloudErrorCodeOK, match: nil},
		{name: "rate limit", code: dependencymodels.CloudErrorCodeRateLimited, match: tplinkmodels.IsRateLimitError},
		{name: "expired token", code: dependencymodels.CloudErrorCodeTokenExpired, match: tplinkmodels.IsTokenExpiredError},
		{name: "parameter", code: dependencymodels.CloudErrorCodeParameter, match: tplinkmodels.IsParameterError},
		{
			name: "authentication", code: dependencymodels.CloudErrorCodeAuthentication,
			match: tplinkmodels.IsAuthenticationError,
		},
		{name: "unknown code", code: 987, match: tplinkmodels.IsCloudAPIError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertCloudError(t, test)
		})
	}

	err := cloud.CheckError([]byte("{"), "test")
	if !tplinkmodels.IsInvalidResponseError(err) {
		t.Fatalf("CheckError() malformed response error = %T %v", err, err)
	}

	err = cloud.CheckError([]byte(`{"msg":"missing code"}`), "test")
	if err != nil {
		t.Fatalf("CheckError() with omitted code = %v, want nil", err)
	}
}

func assertCloudError(t *testing.T, test cloudErrorCase) {
	t.Helper()

	message := "provider message"

	data, err := json.Marshal(dependencymodels.CloudResponse{
		ErrorCode: &test.code,
		Msg:       &message,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = cloud.CheckError(data, "test")
	if test.match == nil && err != nil {
		t.Fatalf("CheckError() error = %v, want nil", err)
	}

	if test.match != nil && !test.match(err) {
		t.Fatalf("CheckError() error = %T %v", err, err)
	}
}
