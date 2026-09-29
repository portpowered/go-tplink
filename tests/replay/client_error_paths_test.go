package replay_test

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

	"github.com/portpowered/go-tplink/pkg/tplink"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errSyntheticOptionFailure          = errors.New("synthetic option failure")
	errSyntheticValueHTTPDoer          = errors.New("synthetic value HTTP doer")
	errSyntheticNetworkFailure         = errors.New("synthetic network failure")
	errSyntheticTransportFailure       = errors.New("synthetic transport failure")
	errSyntheticDirectTransportFailure = errors.New("synthetic direct transport failure")
	errSyntheticBodyReadFailure        = errors.New("synthetic body read failure")
)

func TestClientConfigurationErrors(t *testing.T) {
	t.Parallel()
	t.Run("nil option", testNilClientOption)
	t.Run("option application failure", testFailingClientOption)
	t.Run("nil HTTP client", testNilHTTPClient)
	t.Run("nil HTTP client interface", testNilHTTPClientInterface)
	t.Run("value HTTP client", testValueHTTPClient)
	t.Run("invalid base URLs", testInvalidBaseURLs)
	t.Run("nil client receiver", testNilClientReceiver)
	t.Run("nil context", testNilRequestContext)
}

func assertConfigurationError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsConfigurationError(err))
}

func testNilClientOption(t *testing.T) {
	t.Parallel()

	_, err := tplink.NewClient(nil)
	assertConfigurationError(t, err)
}

func testFailingClientOption(t *testing.T) {
	t.Parallel()

	_, err := tplink.NewClient(failingClientOption{})
	assertConfigurationError(t, err)
}

func testNilHTTPClient(t *testing.T) {
	t.Parallel()

	var doer *http.Client

	_, err := tplink.NewClient(tplink.WithHTTPClient(doer))
	assertConfigurationError(t, err)
}

func testNilHTTPClientInterface(t *testing.T) {
	t.Parallel()

	_, err := tplink.NewClient(tplink.WithHTTPClient(nil))
	assertConfigurationError(t, err)
}

func testValueHTTPClient(t *testing.T) {
	t.Parallel()

	client, err := tplink.NewClient(tplink.WithHTTPClient(valueHTTPDoer{}))

	require.NoError(t, err)
	require.NoError(t, client.Close())
}

func testInvalidBaseURLs(t *testing.T) {
	t.Parallel()

	baseURLs := []string{
		"not a URL",
		"https://example.com/%zz",
		"ftp://example.com",
		"https://user:password@example.com",
		"https://example.com?token=secret",
		"https://example.com#fragment",
	}

	for _, baseURL := range baseURLs {
		t.Run(baseURL, func(t *testing.T) {
			t.Parallel()

			_, err := tplink.NewClient(tplink.WithBaseURL(baseURL))

			assertConfigurationError(t, err)
		})
	}
}

func testNilClientReceiver(t *testing.T) {
	t.Parallel()

	var client *tplink.Client

	_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"},
	})
	assertConfigurationError(t, err)
}

func testNilRequestContext(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	transport.expectCalls(0)

	//nolint:staticcheck // This test verifies nil-context rejection.
	_, err := client.GetDevices(nil, tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsInvalidRequestError(err))
}

type failingClientOption struct{}

func (failingClientOption) Apply(*tplink.Client) error {
	return errSyntheticOptionFailure
}

type valueHTTPDoer struct{}

func (valueHTTPDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errSyntheticValueHTTPDoer
}

func TestOperationReplayDecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		response  string
		call      func(*tplink.Client) error
	}{
		{
			name:      "login result has wrong shape",
			operation: "login",
			response:  `{"error_code":0,"result":[]}`,
			call: func(client *tplink.Client) error {
				_, err := client.Login(context.Background(), tplink.LoginRequest{
					Email: "user@example.com", Password: "placeholder-password",
				})

				return fmt.Errorf("replay operation failed: %w", err)
			},
		},
		{
			name:      "device list result has wrong shape",
			operation: "getDeviceList",
			response:  `{"error_code":0,"result":[]}`,
			call: func(client *tplink.Client) error {
				_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"},
				})

				return fmt.Errorf("replay operation failed: %w", err)
			},
		},
		{
			name:      "passthrough result has wrong shape",
			operation: "passthrough_system_reboot",
			response:  `{"error_code":0,"result":[]}`,
			call: func(client *tplink.Client) error {
				return client.Reboot(context.Background(), tplink.RebootRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, transport := newTestClient(t)

			operation := test.operation
			if operation == "" {
				operation = "login"
			}

			transport.useResponse(operation, http.StatusOK, []byte(test.response))
			err := test.call(client)
			require.Error(t, err)
			assert.True(t, tplinkmodels.IsInvalidResponseError(err), "got %T: %v", err, err)
		})
	}
}

type passthroughErrorReplayCase struct {
	name         string
	operationKey string
	responseData string
	variant      int
	call         func(*tplink.Client) error
	want         func(error) bool
}

func TestPassthroughReplayErrorResponses(t *testing.T) {
	t.Parallel()
	t.Run("system response parsing", func(t *testing.T) {
		t.Parallel()
		runPassthroughErrorReplayCases(t, systemPassthroughErrorCases())
	})
	t.Run("light response parsing", func(t *testing.T) {
		t.Parallel()
		runPassthroughErrorReplayCases(t, lightPassthroughErrorCases())
	})
	t.Run("command acknowledgement parsing", func(t *testing.T) {
		t.Parallel()
		runPassthroughErrorReplayCases(t, commandPassthroughErrorCases())
	})
}

func runPassthroughErrorReplayCases(t *testing.T, tests []passthroughErrorReplayCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, transport := newTestClient(t)
			transport.useResponse(test.operationKey, http.StatusOK, passthroughReplayBody(t, test.responseData))

			if test.variant != 0 {
				transport.expectVariant(test.variant)
			}

			err := test.call(client)
			require.Error(t, err)
			assert.True(t, test.want(err), "got %T: %v", err, err)
		})
	}
}

func systemPassthroughErrorCases() []passthroughErrorReplayCase {
	return []passthroughErrorReplayCase{
		{
			name: "device error outside namespace", operationKey: "passthrough_system_get_sysinfo",
			responseData: `{"err_code":5,"err_msg":"device refused request"}`,
			variant:      0,
			call:         replayPowerStateError, want: tplinkmodels.IsDeviceError,
		},
		{
			name: "malformed device response JSON", operationKey: "passthrough_system_get_sysinfo",
			responseData: `[]`, call: replayPowerStateError, want: tplinkmodels.IsInvalidResponseError,
			variant: 0,
		},
		{
			name: "power state result has wrong shape", operationKey: "passthrough_system_get_sysinfo",
			responseData: `{"system":{"get_sysinfo":[]}}`,
			variant:      0,
			call:         replayPowerStateError, want: tplinkmodels.IsInvalidResponseError,
		},
		{
			name: "power state reports device error", operationKey: "passthrough_system_get_sysinfo",
			responseData: `{"system":{"get_sysinfo":{"err_code":5}}}`,
			variant:      0,
			call:         replayPowerStateError, want: tplinkmodels.IsDeviceError,
		},
	}
}

func lightPassthroughErrorCases() []passthroughErrorReplayCase {
	return []passthroughErrorReplayCase{
		{
			name:         "light state result has wrong shape",
			operationKey: "passthrough_lightingservice_get_light_state",
			responseData: `{"smartlife.iot.smartbulb.lightingservice":{"get_light_state":[]}}`,
			variant:      0,
			call:         replayLightStateError, want: tplinkmodels.IsInvalidResponseError,
		},
		{
			name:         "light state reports device error",
			operationKey: "passthrough_lightingservice_get_light_state",
			responseData: `{"smartlife.iot.smartbulb.lightingservice":{"get_light_state":{"err_code":5}}}`,
			variant:      0,
			call:         replayLightStateError, want: tplinkmodels.IsDeviceError,
		},
		{
			name:         "missing light namespace",
			operationKey: "passthrough_lightingservice_get_light_state",
			responseData: `{"system":{}}`,
			variant:      0,
			call:         replayColorError, want: tplinkmodels.IsInvalidResponseError,
		},
	}
}

func commandPassthroughErrorCases() []passthroughErrorReplayCase {
	return []passthroughErrorReplayCase{
		{
			name: "command reports device error", operationKey: "passthrough_system_set_dev_alias",
			responseData: `{"system":{"set_dev_alias":{"err_code":5,"err_msg":"alias rejected"}}}`,
			variant:      1, call: replaySetAliasError, want: tplinkmodels.IsDeviceError,
		},
		{
			name: "command reports unsupported operation", operationKey: "passthrough_system_set_relay_state",
			responseData: `{"system":{"set_relay_state":{"err_code":-1,"err_msg":"unsupported"}}}`,
			variant:      1, call: replayTurnOffError, want: tplinkmodels.IsUnsupportedOperationError,
		},
		{
			name: "command result has wrong shape", operationKey: "passthrough_system_set_dev_alias",
			responseData: `{"system":{"set_dev_alias":[]}}`,
			variant:      1, call: replaySetAliasError, want: tplinkmodels.IsInvalidResponseError,
		},
	}
}

func replayPowerStateError(client *tplink.Client) error {
	_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
	})

	return fmt.Errorf("replay operation failed: %w", err)
}

func replayLightStateError(client *tplink.Client) error {
	_, err := client.GetLightState(context.Background(), tplink.GetLightStateRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
	})

	return fmt.Errorf("replay operation failed: %w", err)
}

func replayColorError(client *tplink.Client) error {
	_, err := client.GetColor(context.Background(), tplink.GetColorRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
	})

	return fmt.Errorf("replay operation failed: %w", err)
}

func replaySetAliasError(client *tplink.Client) error {
	err := client.SetAlias(context.Background(), tplink.SetAliasRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001", Alias: "New name",
	})

	return fmt.Errorf("replay operation failed: %w", err)
}

func replayTurnOffError(client *tplink.Client) error {
	err := client.TurnOff(context.Background(), tplink.TurnOffRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
	})

	return fmt.Errorf("replay operation failed: %w", err)
}

func TestPowerStatePropagatesCloudError(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	transport.useResponse("passthrough_system_get_sysinfo", http.StatusOK,
		[]byte(`{"error_code":-99999,"msg":"synthetic failure"}`))

	_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsCloudAPIError(err))
}

func TestDerivedLightGettersPropagateReplayErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*tplink.Client) error
	}{
		{
			name: "brightness",
			call: func(client *tplink.Client) error {
				_, err := client.GetBrightness(context.Background(), tplink.GetBrightnessRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
				})

				return fmt.Errorf("replay operation failed: %w", err)
			},
		},
		{
			name: "color temperature",
			call: func(client *tplink.Client) error {
				_, err := client.GetColorTemp(context.Background(), tplink.GetColorTempRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
				})

				return fmt.Errorf("replay operation failed: %w", err)
			},
		},
		{
			name: "color",
			call: func(client *tplink.Client) error {
				_, err := client.GetColor(context.Background(), tplink.GetColorRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
				})

				return fmt.Errorf("replay operation failed: %w", err)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, transport := newTestClient(t)
			transport.useResponse("passthrough_lightingservice_get_light_state", http.StatusOK,
				[]byte(`{"error_code":-99999,"msg":"synthetic failure"}`))

			err := test.call(client)
			require.Error(t, err)
			assert.True(t, tplinkmodels.IsCloudAPIError(err))
		})
	}
}

func TestLightSetterMarshalAndNetworkErrors(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	err := client.SetLightState(context.Background(), tplink.SetLightStateRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
		State: map[string]any{"unsupported": func() {}},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsInvalidRequestError(err))
	assert.Empty(t, transport.recordedRequests())

	transport.useError("passthrough_lightingservice_transition_light_state", errSyntheticNetworkFailure)
	transport.expectVariant(4)

	err = client.SetColorTemp(context.Background(), tplink.SetColorTempRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001", ColorTemp: 3000,
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsNetworkError(err))
}

func TestPlugAndAliasMethodsPropagateCloudErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*tplink.Client) error
	}{
		{
			name: "turn on",
			call: func(client *tplink.Client) error {
				return client.TurnOn(context.Background(), tplink.TurnOnRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
				})
			},
		},
		{
			name: "turn off",
			call: func(client *tplink.Client) error {
				return client.TurnOff(context.Background(), tplink.TurnOffRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
				})
			},
		},
		{
			name: "set alias",
			call: func(client *tplink.Client) error {
				return client.SetAlias(context.Background(), tplink.SetAliasRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001", Alias: "New name",
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, transport := newTestClient(t)

			operation := "passthrough_system_set_relay_state"
			if test.name == "set alias" {
				operation = "passthrough_system_set_dev_alias"
			}

			transport.useResponse(operation, http.StatusOK, []byte(`{"error_code":-99999,"msg":"synthetic failure"}`))

			if test.name == "turn off" || test.name == "set alias" {
				transport.expectVariant(1)
			}

			err := test.call(client)
			require.Error(t, err)
			assert.True(t, tplinkmodels.IsCloudAPIError(err))
		})
	}
}

func TestHTTPBodyReadAndCloseFailures(t *testing.T) {
	t.Parallel()
	t.Run("transport returns response and error", testResponseAndTransportError)
	t.Run("response body read fails", testResponseBodyReadFailure)
	t.Run("non URL transport error is preserved", testDirectTransportError)
	t.Run("malformed transport error URLs", testMalformedTransportErrorURLs)
}

func testResponseAndTransportError(t *testing.T) {
	t.Parallel()

	body := new(closeTrackingBody)

	body.reader = strings.NewReader("unused")
	transport := newReplayTransport()
	transport.useFault("fault-device-response-and-error", errSyntheticTransportFailure, body)

	client := newTestClientWithDoer(t, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		return transport.RoundTrip(request)
	}))

	t.Cleanup(func() {
		assertionErr := transport.assertConsumed()
		require.NoError(t, assertionErr)
	})

	_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsNetworkError(err))
	assert.True(t, body.closed)
}

func testResponseBodyReadFailure(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	transport.useFault("fault-device-read-error", nil, nil)

	_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsNetworkError(err))
}

func testDirectTransportError(t *testing.T) {
	t.Parallel()

	cause := errSyntheticDirectTransportFailure
	client, transport := newTestClient(t)
	transport.useFault("fault-login-direct-error", cause, nil)

	_, err := client.Login(context.Background(), tplink.LoginRequest{
		Email: "user@example.com", Password: "placeholder-password",
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsNetworkError(err))
	assert.NotContains(t, err.Error(), "synthetic direct transport failure")
	require.ErrorIs(t, err, cause)
}

func testMalformedTransportErrorURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, transportURL, wantURL string
	}{
		{
			name:         "malformed URL with query",
			transportURL: "https://bad host/path?token=secret&scope=devices",
			wantURL:      "https://bad host/path?[REDACTED]",
		},
		{
			name:         "malformed URL without query",
			transportURL: "https://[broken",
			wantURL:      "https://[broken",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertMalformedTransportErrorURL(t, test.transportURL, test.wantURL)
		})
	}
}

func assertMalformedTransportErrorURL(t *testing.T, transportURL, wantURL string) {
	t.Helper()

	faultID := "fault-login-url-malformed"

	if strings.Contains(transportURL, "?") {
		faultID = "fault-login-url-query"
	}

	transport := newReplayTransport()
	transport.useFault(faultID, errSyntheticTransportFailure, nil)
	client := newTestClientWithDoer(t, httpDoerFunc(transport.RoundTrip))
	t.Cleanup(func() { require.NoError(t, transport.assertConsumed()) })

	_, err := client.Login(context.Background(), tplink.LoginRequest{
		Email: "user@example.com", Password: "placeholder-password",
	})
	require.Error(t, err)

	var requestError *url.Error

	require.ErrorAs(t, err, &requestError)
	assert.Equal(t, wantURL, requestError.URL)
}

type closeTrackingBody struct {
	reader io.Reader
	closed bool
}

func (body *closeTrackingBody) Read(buffer []byte) (int, error) {
	count, err := body.reader.Read(buffer)
	if err != nil {
		return count, fmt.Errorf("read tracking body: %w", err)
	}

	return count, nil
}

func (body *closeTrackingBody) Close() error {
	body.closed = true

	return nil
}

type readErrorBody struct{}

func (readErrorBody) Read([]byte) (int, error) {
	return 0, errSyntheticBodyReadFailure
}

func (readErrorBody) Close() error {
	return nil
}

func passthroughReplayBody(t *testing.T, responseData string) []byte {
	t.Helper()

	body := map[string]any{
		"error_code": 0,
		"result":     map[string]string{"responseData": responseData},
	}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)

	return encoded
}
