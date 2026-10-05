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
	name      string
	mutate    func(*http.Request)
	wantError string
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

func requestIdentityCases() []requestIdentityCase {
	return []requestIdentityCase{
		{name: "original device-list fixture", mutate: nil, wantError: ""},
		{
			name: "userinfo is rejected without echoing credentials",
			mutate: func(request *http.Request) {
				request.URL.User = url.UserPassword("fixture-user-secret", "fixture-password-secret")
			},
			wantError: "request URL userinfo is unsupported",
		},
		{
			name: "Host override is rejected",
			mutate: func(request *http.Request) {
				request.Host = "other.example"
			},
			wantError: "request Host override does not match URL authority",
		},
		{
			name: "opaque URL is rejected",
			mutate: func(request *http.Request) {
				request.URL.Opaque = "/alternate-target"
			},
			wantError: "request URL opaque form is unsupported",
		},
		{
			name: "malformed query is rejected",
			mutate: func(request *http.Request) {
				request.URL.RawQuery += "&unexpected=%ZZ"
			},
			wantError: "request URL query is malformed",
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
		assertRejectedIdentityRequest(t, transport, response, err, test.wantError)

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
) {
	t.Helper()
	require.Error(t, err)
	assert.Nil(t, response)
	require.ErrorContains(t, err, wantError)
	assert.NotContains(t, err.Error(), "fixture-user-secret")
	assert.NotContains(t, err.Error(), "fixture-password-secret")
	assert.Len(t, transport.recordedRequests(), 1)
	assert.EqualError(t, transport.assertConsumed(), "matched 0 of 1 expected replay calls")
}

func newDeviceListReplayCase(t *testing.T) (*replayTransport, *http.Request) {
	t.Helper()

	transport := newReplayTransport()
	transport.expectSequence(replayDeviceListMethod)
	transport.expectToken(0, replayTestToken)

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		replayBaseURL+"/?token="+url.QueryEscape(replayTestToken),
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

	return transport, request
}
