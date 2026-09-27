package replay_test

import (
	"context"
	"encoding/json"
	"errors"
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

func TestClientConfigurationErrors(t *testing.T) {
	t.Run("nil option", func(t *testing.T) {
		_, err := tplink.NewClient(nil)
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsConfigurationError(err))
	})

	t.Run("option application failure", func(t *testing.T) {
		_, err := tplink.NewClient(failingClientOption{})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsConfigurationError(err))
	})

	t.Run("nil HTTP client", func(t *testing.T) {
		var doer *http.Client
		_, err := tplink.NewClient(tplink.WithHTTPClient(doer))
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsConfigurationError(err))
	})

	t.Run("nil HTTP client interface", func(t *testing.T) {
		_, err := tplink.NewClient(tplink.WithHTTPClient(nil))
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsConfigurationError(err))
	})

	t.Run("value HTTP client", func(t *testing.T) {
		_, err := tplink.NewClient(tplink.WithHTTPClient(valueHTTPDoer{}))
		require.NoError(t, err)
	})

	for _, baseURL := range []string{
		"not a URL",
		"https://example.com/%zz",
		"ftp://example.com",
		"https://user:password@example.com",
		"https://example.com?token=secret",
		"https://example.com#fragment",
	} {
		t.Run("invalid base URL "+baseURL, func(t *testing.T) {
			_, err := tplink.NewClient(tplink.WithBaseURL(baseURL))
			require.Error(t, err)
			assert.True(t, tplinkmodels.IsConfigurationError(err))
		})
	}

	t.Run("nil client receiver", func(t *testing.T) {
		var client *tplink.Client
		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsConfigurationError(err))
	})

	t.Run("nil context", func(t *testing.T) {
		client, _ := newTestClient(t)
		_, err := client.GetDevices(nil, tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsInvalidRequestError(err))
	})
}

type failingClientOption struct{}

func (failingClientOption) Apply(*tplink.Client) error {
	return errors.New("synthetic option failure")
}

type valueHTTPDoer struct{}

func (valueHTTPDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("synthetic value HTTP doer")
}

func TestOperationReplayDecodeErrors(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		response  string
		call      func(*tplink.Client) error
	}{
		{
			name:     "login result has wrong shape",
			response: `{"error_code":0,"result":[]}`,
			call: func(client *tplink.Client) error {
				_, err := client.Login(context.Background(), tplink.LoginRequest{
					Email: "user@example.com", Password: "placeholder-password",
				})
				return err
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
				return err
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

func TestPassthroughReplayErrorResponses(t *testing.T) {
	tests := []struct {
		name         string
		operationKey string
		responseData string
		call         func(*tplink.Client) error
		want         func(error) bool
	}{
		{
			name:         "device error outside namespace",
			operationKey: "passthrough_system_get_sysinfo",
			responseData: `{"err_code":5,"err_msg":"device refused request"}`,
			call: func(client *tplink.Client) error {
				_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
				})
				return err
			},
			want: tplinkmodels.IsDeviceError,
		},
		{
			name:         "malformed device response JSON",
			operationKey: "passthrough_system_get_sysinfo",
			responseData: `[]`,
			call: func(client *tplink.Client) error {
				_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
				})
				return err
			},
			want: tplinkmodels.IsInvalidResponseError,
		},
		{
			name:         "power state result has wrong shape",
			operationKey: "passthrough_system_get_sysinfo",
			responseData: `{"system":{"get_sysinfo":[]}}`,
			call: func(client *tplink.Client) error {
				_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
				})
				return err
			},
			want: tplinkmodels.IsInvalidResponseError,
		},
		{
			name:         "power state reports device error",
			operationKey: "passthrough_system_get_sysinfo",
			responseData: `{"system":{"get_sysinfo":{"err_code":5}}}`,
			call: func(client *tplink.Client) error {
				_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
				})
				return err
			},
			want: tplinkmodels.IsDeviceError,
		},
		{
			name:         "light state result has wrong shape",
			operationKey: "passthrough_lightingservice_get_light_state",
			responseData: `{"smartlife.iot.smartbulb.lightingservice":{"get_light_state":[]}}`,
			call: func(client *tplink.Client) error {
				_, err := client.GetLightState(context.Background(), tplink.GetLightStateRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
				})
				return err
			},
			want: tplinkmodels.IsInvalidResponseError,
		},
		{
			name:         "light state reports device error",
			operationKey: "passthrough_lightingservice_get_light_state",
			responseData: `{"smartlife.iot.smartbulb.lightingservice":{"get_light_state":{"err_code":5}}}`,
			call: func(client *tplink.Client) error {
				_, err := client.GetLightState(context.Background(), tplink.GetLightStateRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
				})
				return err
			},
			want: tplinkmodels.IsDeviceError,
		},
		{
			name:         "command reports device error",
			operationKey: "passthrough_system_set_dev_alias",
			responseData: `{"system":{"set_dev_alias":{"err_code":5,"err_msg":"alias rejected"}}}`,
			call: func(client *tplink.Client) error {
				return client.SetAlias(context.Background(), tplink.SetAliasRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001", Alias: "New name",
				})
			},
			want: tplinkmodels.IsDeviceError,
		},
		{
			name:         "command reports unsupported operation",
			operationKey: "passthrough_system_set_relay_state",
			responseData: `{"system":{"set_relay_state":{"err_code":-1,"err_msg":"unsupported"}}}`,
			call: func(client *tplink.Client) error {
				return client.TurnOff(context.Background(), tplink.TurnOffRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001",
				})
			},
			want: tplinkmodels.IsUnsupportedOperationError,
		},
		{
			name:         "command result has wrong shape",
			operationKey: "passthrough_system_set_dev_alias",
			responseData: `{"system":{"set_dev_alias":[]}}`,
			call: func(client *tplink.Client) error {
				return client.SetAlias(context.Background(), tplink.SetAliasRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-plug-001", Alias: "New name",
				})
			},
			want: tplinkmodels.IsInvalidResponseError,
		},
		{
			name:         "missing light namespace",
			operationKey: "passthrough_lightingservice_get_light_state",
			responseData: `{"system":{}}`,
			call: func(client *tplink.Client) error {
				_, err := client.GetColor(context.Background(), tplink.GetColorRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
				})
				return err
			},
			want: tplinkmodels.IsInvalidResponseError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, transport := newTestClient(t)
			transport.useResponse(test.operationKey, http.StatusOK, passthroughReplayBody(t, test.responseData))
			err := test.call(client)
			require.Error(t, err)
			assert.True(t, test.want(err), "got %T: %v", err, err)
		})
	}
}

func TestPowerStatePropagatesCloudError(t *testing.T) {
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
				return err
			},
		},
		{
			name: "color temperature",
			call: func(client *tplink.Client) error {
				_, err := client.GetColorTemp(context.Background(), tplink.GetColorTempRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
				})
				return err
			},
		},
		{
			name: "color",
			call: func(client *tplink.Client) error {
				_, err := client.GetColor(context.Background(), tplink.GetColorRequest{
					Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
				})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
	client, transport := newTestClient(t)
	err := client.SetLightState(context.Background(), tplink.SetLightStateRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001",
		State: map[string]any{"unsupported": func() {}},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsInvalidRequestError(err))
	assert.Empty(t, transport.recordedRequests())

	transport.useError("passthrough_lightingservice_transition_light_state", errors.New("synthetic network failure"))
	err = client.SetColorTemp(context.Background(), tplink.SetColorTempRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"}, DeviceID: "device-bulb-001", ColorTemp: 3000,
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsNetworkError(err))
}

func TestPlugAndAliasMethodsPropagateCloudErrors(t *testing.T) {
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
			client, transport := newTestClient(t)
			transport.useResponse("passthrough_system_set_relay_state", http.StatusOK,
				[]byte(`{"error_code":-99999,"msg":"synthetic failure"}`))
			transport.useResponse("passthrough_system_set_dev_alias", http.StatusOK,
				[]byte(`{"error_code":-99999,"msg":"synthetic failure"}`))
			err := test.call(client)
			require.Error(t, err)
			assert.True(t, tplinkmodels.IsCloudAPIError(err))
		})
	}
}

func TestHTTPBodyReadAndCloseFailures(t *testing.T) {
	t.Run("transport returns response and error", func(t *testing.T) {
		body := &closeTrackingBody{reader: strings.NewReader("unused")}
		client := newTestClientWithDoer(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, errors.New("synthetic transport failure")
		}))

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsNetworkError(err))
		assert.True(t, body.closed)
	})

	t.Run("response body read fails", func(t *testing.T) {
		client := newTestClientWithDoer(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: readErrorBody{}}, nil
		}))

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsNetworkError(err))
	})

	t.Run("non URL transport error is preserved", func(t *testing.T) {
		cause := errors.New("synthetic direct transport failure")
		client := newTestClientWithDoer(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, cause
		}))

		_, err := client.Login(context.Background(), tplink.LoginRequest{
			Email: "user@example.com", Password: "placeholder-password",
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsNetworkError(err))
		assert.NotContains(t, err.Error(), "synthetic direct transport failure")
		require.ErrorIs(t, err, cause)
	})

	for _, test := range []struct {
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
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClientWithDoer(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
				return nil, &url.Error{Op: "Post", URL: test.transportURL, Err: errors.New("synthetic transport failure")}
			}))

			_, err := client.Login(context.Background(), tplink.LoginRequest{
				Email: "user@example.com", Password: "placeholder-password",
			})
			require.Error(t, err)
			var requestError *url.Error
			require.ErrorAs(t, err, &requestError)
			assert.Equal(t, test.wantURL, requestError.URL)
		})
	}
}

type closeTrackingBody struct {
	reader io.Reader
	closed bool
}

func (body *closeTrackingBody) Read(buffer []byte) (int, error) {
	return body.reader.Read(buffer)
}

func (body *closeTrackingBody) Close() error {
	body.closed = true
	return nil
}

type readErrorBody struct{}

func (readErrorBody) Read([]byte) (int, error) {
	return 0, errors.New("synthetic body read failure")
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
