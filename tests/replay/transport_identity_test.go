package replay_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type requestIdentityCase struct {
	name         string
	mutate       func(*http.Request)
	wantError    string
	secretToHide string
}

func TestReplayTransportMatchesFullRequestIdentity(t *testing.T) {
	t.Parallel()

	for _, test := range requestIdentityCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runRequestIdentityCase(t, test)
		})
	}
}

//nolint:funlen // Keep each request-identity mutation beside its expected rejection and secret check.
func requestIdentityCases() []requestIdentityCase {
	return []requestIdentityCase{
		{name: "original device-list fixture", mutate: nil, wantError: "", secretToHide: ""},
		{
			name: "matching Host override is accepted",
			mutate: func(request *http.Request) {
				request.Host = request.URL.Host
			},
			wantError:    "",
			secretToHide: "",
		},
		{
			name: "userinfo is rejected without echoing credentials",
			mutate: func(request *http.Request) {
				request.URL.User = url.UserPassword("fixture-user-secret", "fixture-password-secret")
			},
			wantError:    "request URL userinfo is unsupported",
			secretToHide: "",
		},
		{
			name: "Host override is rejected",
			mutate: func(request *http.Request) {
				request.Host = "other.example"
			},
			wantError:    "request Host override does not match URL authority",
			secretToHide: "",
		},
		{
			name: "opaque URL is rejected",
			mutate: func(request *http.Request) {
				request.URL.Opaque = "/alternate-target"
			},
			wantError:    "request URL opaque form is unsupported",
			secretToHide: "",
		},
		{
			name: "malformed query is rejected",
			mutate: func(request *http.Request) {
				request.URL.RawQuery += "&unexpected=%ZZ"
			},
			wantError:    "request URL query is malformed",
			secretToHide: "",
		},
		{
			name: "token mismatch diagnostic hides the token",
			mutate: func(request *http.Request) {
				query := request.URL.Query()
				query.Set("token", "probe-actual-session-secret")
				request.URL.RawQuery = query.Encode()
			},
			wantError:    `request query field "token" does not match fixture`,
			secretToHide: "probe-actual-session-secret",
		},
		{
			name: "header mismatch diagnostic hides the value",
			mutate: func(request *http.Request) {
				request.Header.Set("Content-Type", "probe-actual-header-secret")
			},
			wantError:    `request header field "Content-Type" does not match fixture`,
			secretToHide: "probe-actual-header-secret",
		},
		{
			name: "body mismatch diagnostic hides the value",
			mutate: func(request *http.Request) {
				body := `{"method":"getDeviceList","marker":"probe-body-value-713"}`
				request.Body = io.NopCloser(strings.NewReader(body))
				request.ContentLength = int64(len(body))
				request.GetBody = func() (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader(body)), nil
				}
			},
			wantError:    "request body did not match any",
			secretToHide: "probe-body-value-713",
		},
		{
			name: "request fragment is rejected",
			mutate: func(request *http.Request) {
				request.URL.Fragment = "unexpected-fragment"
			},
			wantError:    "request URL fragment is unsupported",
			secretToHide: "",
		},
		{
			name: "RequestURI is rejected without echoing its query",
			mutate: func(request *http.Request) {
				request.RequestURI = "/?token=probe-request-uri-value"
			},
			wantError:    "request RequestURI is unsupported for outbound replay",
			secretToHide: "probe-request-uri-value",
		},
	}
}

func runRequestIdentityCase(t *testing.T, test requestIdentityCase) {
	t.Helper()

	transport, request := newDeviceListReplayCase(t)
	if test.mutate != nil {
		test.mutate(request)
	}

	response, err := transport.RoundTrip(request)
	if test.wantError != "" {
		assertRejectedIdentityRequest(t, transport, response, err, test.wantError, test.secretToHide)

		return
	}

	assertAcceptedIdentityRequest(t, transport, response, err)
}

func assertAcceptedIdentityRequest(
	t *testing.T,
	transport *replayTransport,
	response *http.Response,
	err error,
) {
	t.Helper()
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))

	responseBody, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()

	require.NoError(t, readErr)
	require.NoError(t, closeErr)
	assert.Contains(t, string(responseBody), "device-plug-001")
	assert.NoError(t, transport.assertConsumed())
}

func assertRejectedIdentityRequest(
	t *testing.T,
	transport *replayTransport,
	response *http.Response,
	err error,
	wantError string,
	secretToHide string,
) {
	t.Helper()
	require.Error(t, err)
	assert.Nil(t, response)
	require.ErrorContains(t, err, wantError)
	assert.NotContains(t, err.Error(), "fixture-user-secret")
	assert.NotContains(t, err.Error(), "fixture-password-secret")

	if secretToHide != "" {
		assert.NotContains(t, err.Error(), secretToHide)
	}

	assert.Len(t, transport.recordedRequests(), 1)
	assert.EqualError(t, transport.assertConsumed(), "matched 0 of 1 expected replay calls")
}

func newDeviceListReplayCase(t *testing.T) (*replayTransport, *http.Request) {
	t.Helper()

	transport := newReplayTransport()
	transport.expectSequence(replayDeviceListMethod)
	transport.expectToken(0, replayTestToken)

	return transport, newDeviceListReplayRequest(t, replayTestToken)
}

func newDeviceListReplayRequest(t *testing.T, token string) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		replayBaseURL+"/?token="+url.QueryEscape(token),
		strings.NewReader(`{"method":"getDeviceList"}`),
	)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() {
		closeErr := request.Body.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	return request
}

//nolint:funlen // One transport checks invalid requests in sequence, then accepts the still-available valid pair.
func TestReplayTransportRejectsMutationsWithoutConsumingPair(t *testing.T) {
	t.Parallel()

	transport := newReplayTransport()
	transport.expectSequence(replayDeviceListMethod)
	transport.expectToken(0, replayTestToken)

	for _, test := range []struct {
		name      string
		mutate    func(*http.Request)
		wantError string
	}{
		{
			name: "RequestURI",
			mutate: func(request *http.Request) {
				request.RequestURI = "/?token=probe-request-uri-value"
			},
			wantError: "request RequestURI is unsupported for outbound replay",
		},
		{
			name:      "content length",
			mutate:    func(request *http.Request) { request.ContentLength = 0 },
			wantError: "request ContentLength does not match fixture",
		},
		{
			name: "transfer encoding",
			mutate: func(request *http.Request) {
				request.TransferEncoding = []string{"chunked"}
			},
			wantError: "request TransferEncoding does not match fixture",
		},
		{
			name:      "close flag",
			mutate:    func(request *http.Request) { request.Close = true },
			wantError: "request Close does not match fixture",
		},
		{
			name: "GetBody mismatch",
			mutate: func(request *http.Request) {
				request.GetBody = func() (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader(`{"method":"getDeviceList","token":"synthetic-other-token"}`)), nil
				}
			},
			wantError: "request GetBody does not match request body",
		},
		{
			name: "missing GetBody",
			mutate: func(request *http.Request) {
				request.GetBody = nil
			},
			wantError: "request GetBody is missing",
		},
	} {
		request := newDeviceListReplayRequest(t, replayTestToken)
		test.mutate(request)

		response, err := transport.RoundTrip(request)
		closeReplayResponseBody(t, response)

		if err == nil || response != nil {
			t.Fatalf("%s mutation returned response %v, error %v", test.name, response, err)
		}

		if !strings.Contains(err.Error(), test.wantError) {
			t.Fatalf("%s mutation error = %v, want %q", test.name, err, test.wantError)
		}

		assertErr := transport.assertConsumed()
		if assertErr == nil {
			t.Fatalf("%s mutation consumed the expected replay pair", test.name)
		}
	}

	response, err := transport.RoundTrip(newDeviceListReplayRequest(t, replayTestToken))
	assertAcceptedIdentityRequest(t, transport, response, err)
}
