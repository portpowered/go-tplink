package replay_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"

	"github.com/portpowered/go-tplink/pkg/tplink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReusableClientKeepsTwoAccountsCookieFree(t *testing.T) {
	t.Parallel()

	transport := newReplayTransport()
	transport.expectSequence(replayLoginOperation, replayDeviceListMethod)
	transport.expectToken(1, "account-b-token")
	t.Cleanup(func() {
		transportErr := transport.assertConsumed()
		if transportErr != nil {
			t.Error(transportErr)
		}
	})

	httpClient := new(http.Client)
	httpClient.Transport = loginCookieRoundTripper{next: transport}
	require.Nil(t, httpClient.Jar)
	client, err := tplink.NewClient(
		tplink.WithBaseURL(replayBaseURL),
		tplink.WithHTTPClient(httpClient),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	// Mutating the caller's client after construction must not affect the SDK snapshot.
	httpClient.Jar = jar

	assertSameAccountCookieWorkflow(t, client, transport)
}

//nolint:paralleltest // Temporarily replaces the process-global default client.
func TestNewClientSnapshotsDefaultHTTPClient(t *testing.T) {
	defaultClient := http.DefaultClient
	transport := newReplayTransport()
	transport.expectSequence(replayLoginOperation, replayDeviceListMethod)
	transport.expectToken(1, "account-b-token")
	t.Cleanup(func() {
		transportErr := transport.assertConsumed()
		if transportErr != nil {
			t.Error(transportErr)
		}
	})

	httpClient := new(http.Client)
	httpClient.Transport = loginCookieRoundTripper{next: transport}
	http.DefaultClient = httpClient

	t.Cleanup(func() { http.DefaultClient = defaultClient })

	client, err := tplink.NewClient(tplink.WithBaseURL(replayBaseURL))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	// NewClient must retain its Jar-free snapshot if the global client is later changed.
	http.DefaultClient.Jar = jar

	assertSameAccountCookieWorkflow(t, client, transport)
}

func assertSameAccountCookieWorkflow(t *testing.T, client *tplink.Client, transport *replayTransport) {
	t.Helper()

	login, err := client.Login(context.Background(), tplink.LoginRequest{
		Email:    "user@example.com",
		Password: "placeholder-password",
	})
	require.NoError(t, err, "wrapped cause: %v", errors.Unwrap(errors.Unwrap(err)))
	assert.Equal(t, "acc-123456", login.AccountID)

	devices, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: "account-b-token"},
	})
	require.NoError(t, err)
	assert.Len(t, devices.Devices, 1)

	requests := transport.recordedRequests()
	require.Len(t, requests, 2)
	assertCookieFreeCloudRequest(t, requests[0])
	assertCookieFreeCloudRequest(t, requests[1])

	assert.Empty(t, requests[0].URL.RawQuery)
	assert.JSONEq(t,
		`{
			"method":"login",
			"params":{
				"appType":"Tapo_Android",
				"cloudPassword":"placeholder-password",
				"cloudUserName":"user@example.com",
				"terminalUUID":"go-tplink-client"
			}
		}`,
		string(requests[0].Body),
	)
	assert.Equal(t, url.Values{"token": []string{"account-b-token"}}, requests[1].URL.Query())
	assert.JSONEq(t, `{"method":"getDeviceList"}`, string(requests[1].Body))
}

func assertCookieFreeCloudRequest(t *testing.T, request recordedRequest) {
	t.Helper()
	assert.Equal(t, http.MethodPost, request.Method)
	assert.Equal(t, "https", request.URL.Scheme)
	assert.Equal(t, "wap.tplinkcloud.com", request.URL.Host)
	assert.Equal(t, "/", request.URL.EscapedPath())
	assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
	assert.Empty(t, request.Header.Get("Cookie"))
}

type loginCookieRoundTripper struct {
	next http.RoundTripper
}

func (transport loginCookieRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.next.RoundTrip(request)
	if err != nil {
		return response, fmt.Errorf("replay request: %w", err)
	}

	if response == nil || request.URL.Query().Get("token") != "" {
		return response, nil
	}

	response.Header.Add("Set-Cookie", "account-session=account-a; Path=/")

	return response, nil
}
