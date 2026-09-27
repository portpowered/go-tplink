package replay_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/portpowered/go-tplink/pkg/tplink"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const replayBaseURL = "https://wap.tplinkcloud.com"

func newTestClient(t *testing.T) (*tplink.Client, *replayTransport) {
	t.Helper()

	transport := newReplayTransport()
	client := newTestClientWithDoer(t, &http.Client{Transport: transport})
	return client, transport
}

func newTestClientWithDoer(t *testing.T, doer tplink.HTTPDoer) *tplink.Client {
	t.Helper()

	client, err := tplink.NewClient(
		tplink.WithBaseURL(replayBaseURL),
		tplink.WithHTTPClient(doer),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestLoginAndDeviceListReplay(t *testing.T) {
	client, transport := newTestClient(t)
	ctx := context.Background()
	login, err := client.Login(ctx, tplink.LoginRequest{
		Email:    "user@example.com",
		Password: "placeholder-password",
	})
	require.NoError(t, err)
	assert.Equal(t, "test-auth-token-abc123", login.Token)
	assert.Equal(t, "acc-123456", login.AccountID)
	assert.Equal(t, "user@example.com", login.Email)
	assert.Equal(t, "US", login.CountryCode)

	loginRequest := onlyRecordedRequest(t, transport)
	assert.Equal(t, http.MethodPost, loginRequest.Method)
	assert.Empty(t, loginRequest.URL.Query().Get("token"))
	var loginEnvelope struct {
		Method string `json:"method"`
		Params struct {
			AppType       string `json:"appType"`
			CloudUserName string `json:"cloudUserName"`
			CloudPassword string `json:"cloudPassword"`
		} `json:"params"`
	}
	require.NoError(t, json.Unmarshal(loginRequest.Body, &loginEnvelope))
	assert.Equal(t, "login", loginEnvelope.Method)
	assert.Equal(t, "Tapo_Android", loginEnvelope.Params.AppType)
	assert.Equal(t, "user@example.com", loginEnvelope.Params.CloudUserName)
	assert.Equal(t, "placeholder-password", loginEnvelope.Params.CloudPassword)

	// Reuse one client for different request-scoped tokens to ensure auth does
	// not leak from one account request to the next.
	first, err := client.GetDevices(ctx, tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: "account-one-token"},
	})
	require.NoError(t, err)
	require.Len(t, first.Devices, 1)
	assert.Equal(t, "device-plug-001", first.Devices[0].DeviceID)
	assert.Equal(t, "Living Room Plug", first.Devices[0].Alias)
	assert.Equal(t, "HS105(US)", first.Devices[0].DeviceModel)
	assert.Equal(t, 1, first.Devices[0].Status)

	secondToken := "account two+token&scope=devices"
	second, err := client.GetDevices(ctx, tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: secondToken},
	})
	require.NoError(t, err)
	require.Len(t, second.Devices, 1)

	requests := transport.recordedRequests()
	require.Len(t, requests, 3)
	assert.Empty(t, requests[0].URL.Query().Get("token"))
	assert.Equal(t, "account-one-token", requests[1].URL.Query().Get("token"))
	assert.Equal(t, secondToken, requests[2].URL.Query().Get("token"))
	for _, request := range requests[1:] {
		assert.Equal(t, http.MethodPost, request.Method)
		var envelope struct {
			Method string `json:"method"`
		}
		require.NoError(t, json.Unmarshal(request.Body, &envelope))
		assert.Equal(t, "getDeviceList", envelope.Method)
	}
}

func TestDeviceListReplayShapes(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		client, transport := newTestClient(t)
		transport.useFixture("getDeviceList", "getDeviceList_empty")

		result, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.NoError(t, err)
		assert.Empty(t, result.Devices)
	})

	t.Run("mixed device types", func(t *testing.T) {
		client, transport := newTestClient(t)
		transport.useFixture("getDeviceList", "getDeviceList_mixed")

		result, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.NoError(t, err)
		require.Len(t, result.Devices, 4)

		assert.Equal(t, tplinkmodels.EndpointTypePlug, tplinkmodels.ClassifyDevice(result.Devices[0].DeviceType))
		assert.Equal(t, tplinkmodels.EndpointTypeBulb, tplinkmodels.ClassifyDevice(result.Devices[1].DeviceType))
		assert.Equal(t, tplinkmodels.EndpointTypePlug, tplinkmodels.ClassifyDevice(result.Devices[2].DeviceType))
		assert.Equal(t, tplinkmodels.EndpointTypeBulb, tplinkmodels.ClassifyDevice(result.Devices[3].DeviceType))

		colorBulb := tplinkmodels.DetectCapabilities(result.Devices[1])
		assert.True(t, colorBulb.OnOff)
		assert.True(t, colorBulb.Brightness)
		assert.True(t, colorBulb.Color)
		assert.True(t, colorBulb.ColorTemp)

		dimmableBulb := tplinkmodels.DetectCapabilities(result.Devices[3])
		assert.True(t, dimmableBulb.OnOff)
		assert.True(t, dimmableBulb.Brightness)
		assert.False(t, dimmableBulb.Color)
		assert.False(t, dimmableBulb.ColorTemp)
	})
}

func TestCloudErrorReplayMappings(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		wantErr func(error) bool
	}{
		{name: "rate limit", fixture: "error_rate_limit", wantErr: tplinkmodels.IsRateLimitError},
		{name: "expired token", fixture: "error_token_expired", wantErr: tplinkmodels.IsTokenExpiredError},
		{name: "parameter error", fixture: "error_parameter", wantErr: tplinkmodels.IsParameterError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, transport := newTestClient(t)
			transport.useFixture("getDeviceList", test.fixture)

			_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
				Auth: tplink.AuthContext{AccessToken: "test-token"},
			})
			require.Error(t, err)
			assert.True(t, test.wantErr(err), "unexpected error type: %T (%v)", err, err)
		})
	}

	t.Run("unrecognized cloud error", func(t *testing.T) {
		client, transport := newTestClient(t)
		transport.useResponse("getDeviceList", http.StatusOK, []byte(`{"error_code":-99999,"msg":"Unknown error"}`))

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsCloudAPIError(err))
	})
}

func TestLoginAndDeviceErrorReplayMappings(t *testing.T) {
	t.Run("rejected login", func(t *testing.T) {
		client, transport := newTestClient(t)
		transport.useFixture("login", "login_failure")

		_, err := client.Login(context.Background(), tplink.LoginRequest{
			Email:    "bad@example.com",
			Password: "placeholder-password",
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsAuthenticationError(err))
	})

	t.Run("unsupported plug operation", func(t *testing.T) {
		client, transport := newTestClient(t)
		transport.useFixture("passthrough_system_get_sysinfo", "error_unsupported")

		_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
			Auth:     tplink.AuthContext{AccessToken: "test-token"},
			DeviceID: "device-unknown-001",
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsUnsupportedOperationError(err))
	})

	t.Run("unsupported bulb operation", func(t *testing.T) {
		client, transport := newTestClient(t)
		transport.useFixture("passthrough_lightingservice_transition_light_state", "error_unsupported")

		err := client.SetBrightness(context.Background(), tplink.SetBrightnessRequest{
			Auth:       tplink.AuthContext{AccessToken: "test-token"},
			DeviceID:   "device-plug-001",
			Brightness: 50,
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsUnsupportedOperationError(err))
	})

	t.Run("missing token stops before HTTP", func(t *testing.T) {
		client, transport := newTestClient(t)

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsTokenNotSetError(err))
		assert.Empty(t, transport.recordedRequests())
	})
}

func TestClosedClientRejectsRequests(t *testing.T) {
	client, _ := newTestClient(t)
	require.NoError(t, client.Close())

	_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: "test-token"},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsClientClosedError(err))
}

func TestPassthroughRequestReplay(t *testing.T) {
	auth := tplink.AuthContext{AccessToken: "test-token"}
	ctx := context.Background()
	tests := []struct {
		name       string
		deviceID   string
		command    string
		wantFields map[string]any
		call       func(*tplink.Client) error
	}{
		{
			name:       "turn on plug",
			deviceID:   "device-plug-001",
			command:    "system.set_relay_state",
			wantFields: map[string]any{"state": float64(1)},
			call: func(client *tplink.Client) error {
				return client.TurnOn(ctx, tplink.TurnOnRequest{Auth: auth, DeviceID: "device-plug-001"})
			},
		},
		{
			name:       "turn off plug",
			deviceID:   "device-plug-001",
			command:    "system.set_relay_state",
			wantFields: map[string]any{"state": float64(0)},
			call: func(client *tplink.Client) error {
				return client.TurnOff(ctx, tplink.TurnOffRequest{Auth: auth, DeviceID: "device-plug-001"})
			},
		},
		{
			name:       "reboot plug",
			deviceID:   "device-plug-001",
			command:    "system.reboot",
			wantFields: map[string]any{"delay": float64(1)},
			call: func(client *tplink.Client) error {
				return client.Reboot(ctx, tplink.RebootRequest{Auth: auth, DeviceID: "device-plug-001"})
			},
		},
		{
			name:       "rename device",
			deviceID:   "device-plug-001",
			command:    "system.set_dev_alias",
			wantFields: map[string]any{"alias": "New Name"},
			call: func(client *tplink.Client) error {
				return client.SetAlias(ctx, tplink.SetAliasRequest{Auth: auth, DeviceID: "device-plug-001", Alias: "New Name"})
			},
		},
		{
			name:       "set brightness",
			deviceID:   "device-bulb-001",
			command:    "smartlife.iot.smartbulb.lightingservice.transition_light_state",
			wantFields: map[string]any{"brightness": float64(50)},
			call: func(client *tplink.Client) error {
				return client.SetBrightness(ctx, tplink.SetBrightnessRequest{Auth: auth, DeviceID: "device-bulb-001", Brightness: 50})
			},
		},
		{
			name:       "set color temperature",
			deviceID:   "device-bulb-001",
			command:    "smartlife.iot.smartbulb.lightingservice.transition_light_state",
			wantFields: map[string]any{"color_temp": float64(4000)},
			call: func(client *tplink.Client) error {
				return client.SetColorTemp(ctx, tplink.SetColorTempRequest{Auth: auth, DeviceID: "device-bulb-001", ColorTemp: 4000})
			},
		},
		{
			name:       "set color",
			deviceID:   "device-bulb-001",
			command:    "smartlife.iot.smartbulb.lightingservice.transition_light_state",
			wantFields: map[string]any{"hue": float64(240), "saturation": float64(80)},
			call: func(client *tplink.Client) error {
				return client.SetColor(ctx, tplink.SetColorRequest{Auth: auth, DeviceID: "device-bulb-001", Hue: 240, Saturation: 80})
			},
		},
		{
			name:       "set light state",
			deviceID:   "device-bulb-001",
			command:    "smartlife.iot.smartbulb.lightingservice.transition_light_state",
			wantFields: map[string]any{"on_off": float64(1), "brightness": float64(75), "hue": float64(120), "saturation": float64(50)},
			call: func(client *tplink.Client) error {
				return client.SetLightState(ctx, tplink.SetLightStateRequest{
					Auth:     auth,
					DeviceID: "device-bulb-001",
					State:    map[string]any{"on_off": 1, "brightness": 75, "hue": 120, "saturation": 50},
				})
			},
		},
		{
			name:     "read plug state",
			deviceID: "device-plug-001",
			command:  "system.get_sysinfo",
			call: func(client *tplink.Client) error {
				_, err := client.GetPowerState(ctx, tplink.GetPowerStateRequest{Auth: auth, DeviceID: "device-plug-001"})
				return err
			},
		},
		{
			name:     "read bulb state",
			deviceID: "device-bulb-001",
			command:  "smartlife.iot.smartbulb.lightingservice.get_light_state",
			call: func(client *tplink.Client) error {
				_, err := client.GetLightState(ctx, tplink.GetLightStateRequest{Auth: auth, DeviceID: "device-bulb-001"})
				return err
			},
		},
		{
			name:     "read brightness",
			deviceID: "device-bulb-001",
			command:  "smartlife.iot.smartbulb.lightingservice.get_light_state",
			call: func(client *tplink.Client) error {
				_, err := client.GetBrightness(ctx, tplink.GetBrightnessRequest{Auth: auth, DeviceID: "device-bulb-001"})
				return err
			},
		},
		{
			name:     "read color temperature",
			deviceID: "device-bulb-001",
			command:  "smartlife.iot.smartbulb.lightingservice.get_light_state",
			call: func(client *tplink.Client) error {
				_, err := client.GetColorTemp(ctx, tplink.GetColorTempRequest{Auth: auth, DeviceID: "device-bulb-001"})
				return err
			},
		},
		{
			name:     "read color",
			deviceID: "device-bulb-001",
			command:  "smartlife.iot.smartbulb.lightingservice.get_light_state",
			call: func(client *tplink.Client) error {
				_, err := client.GetColor(ctx, tplink.GetColorRequest{Auth: auth, DeviceID: "device-bulb-001"})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, transport := newTestClient(t)
			require.NoError(t, test.call(client))
			request := onlyRecordedRequest(t, transport)
			assert.Equal(t, "test-token", request.URL.Query().Get("token"))

			command, fields := decodePassthroughCommand(t, request.Body)
			assert.Equal(t, test.deviceID, command.deviceID)
			assert.Equal(t, test.command, command.name)
			for key, want := range test.wantFields {
				assert.Equal(t, want, fields[key], "command field %q", key)
			}
		})
	}
}

func TestPowerAndLightStateReplayResults(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()
	auth := tplink.AuthContext{AccessToken: "test-token"}

	power, err := client.GetPowerState(ctx, tplink.GetPowerStateRequest{Auth: auth, DeviceID: "device-plug-001"})
	require.NoError(t, err)
	assert.True(t, power.IsOn)
	assert.Equal(t, 3600, power.OnTime)

	light, err := client.GetLightState(ctx, tplink.GetLightStateRequest{Auth: auth, DeviceID: "device-bulb-001"})
	require.NoError(t, err)
	assert.Equal(t, 1, light.OnOff)
	assert.Equal(t, 75, light.Brightness)
	assert.Equal(t, 240, light.Hue)
	assert.Equal(t, 80, light.Saturation)
	assert.Equal(t, 0, light.ColorTemp)
	assert.Equal(t, "normal", light.Mode)

	brightness, err := client.GetBrightness(ctx, tplink.GetBrightnessRequest{Auth: auth, DeviceID: "device-bulb-001"})
	require.NoError(t, err)
	assert.Equal(t, 75, brightness.Brightness)

	colorTemp, err := client.GetColorTemp(ctx, tplink.GetColorTempRequest{Auth: auth, DeviceID: "device-bulb-001"})
	require.NoError(t, err)
	assert.Equal(t, 0, colorTemp.ColorTemp)

	color, err := client.GetColor(ctx, tplink.GetColorRequest{Auth: auth, DeviceID: "device-bulb-001"})
	require.NoError(t, err)
	assert.Equal(t, 240, color.Hue)
	assert.Equal(t, 80, color.Saturation)
}

func TestMalformedAndHTTPFailureResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   []byte
		want   func(error) bool
	}{
		{name: "http server failure", status: http.StatusInternalServerError, body: []byte(`{"error":"failed"}`), want: tplinkmodels.IsHTTPStatusError},
		{name: "malformed JSON", status: http.StatusOK, body: []byte(`{"error_code":`), want: tplinkmodels.IsInvalidResponseError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, transport := newTestClient(t)
			transport.useResponse("getDeviceList", test.status, test.body)

			_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
				Auth: tplink.AuthContext{AccessToken: "test-token"},
			})
			require.Error(t, err)
			assert.True(t, test.want(err), "unexpected error type: %T (%v)", err, err)
		})
	}
}

func TestPassthroughMissingCommandIsInvalidResponse(t *testing.T) {
	client, transport := newTestClient(t)
	transport.useResponse("passthrough_system_set_relay_state", http.StatusOK,
		[]byte(`{"error_code":0,"result":{"responseData":"{\"system\":{}}"}}`))
	transport.useResponse("passthrough_system_get_sysinfo", http.StatusOK,
		[]byte(`{"error_code":0,"result":{"responseData":"{\"system\":{}}"}}`))

	auth := tplink.AuthContext{AccessToken: "test-token"}
	err := client.TurnOn(context.Background(), tplink.TurnOnRequest{Auth: auth, DeviceID: "device-plug-001"})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsInvalidResponseError(err))

	_, err = client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{Auth: auth, DeviceID: "device-plug-001"})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsInvalidResponseError(err))
}

func TestOversizedAndNilHTTPResponses(t *testing.T) {
	t.Run("oversized body", func(t *testing.T) {
		client, transport := newTestClient(t)
		transport.useResponse("getDeviceList", http.StatusOK, []byte(strings.Repeat(" ", (1<<20)+1)))

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsResponseTooLargeError(err))
	})

	t.Run("nil response", func(t *testing.T) {
		client := newTestClientWithDoer(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, nil
		}))

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsInvalidResponseError(err))
	})

	t.Run("nil response body", func(t *testing.T) {
		client := newTestClientWithDoer(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK}, nil
		}))

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: "test-token"},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsInvalidResponseError(err))
	})
}

func TestTransportErrorsDoNotLeakRequestTokens(t *testing.T) {
	client, transport := newTestClient(t)
	transportFailure := errors.New("connection refused")
	transport.useError("getDeviceList", transportFailure)
	token := "private token+&scope=devices"

	_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: token},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsNetworkError(err))
	assert.NotContains(t, err.Error(), token)
	assert.NotContains(t, err.Error(), url.QueryEscape(token))
	assert.ErrorIs(t, err, transportFailure)

	var requestError *url.Error
	require.ErrorAs(t, err, &requestError)
	redactedURL, parseErr := url.Parse(requestError.URL)
	require.NoError(t, parseErr)
	assert.Equal(t, "[REDACTED]", redactedURL.Query().Get("token"))
	assert.NotContains(t, requestError.URL, token)
}

type decodedCommand struct {
	deviceID string
	name     string
}

func onlyRecordedRequest(t *testing.T, transport *replayTransport) recordedRequest {
	t.Helper()
	requests := transport.recordedRequests()
	require.Len(t, requests, 1)
	return requests[0]
}

func decodePassthroughCommand(t *testing.T, body []byte) (decodedCommand, map[string]any) {
	t.Helper()
	var request struct {
		Method string `json:"method"`
		Params struct {
			DeviceID    string `json:"deviceId"`
			RequestData string `json:"requestData"`
		} `json:"params"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	assert.Equal(t, "passthrough", request.Method)

	var commands map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(request.Params.RequestData), &commands))
	require.Len(t, commands, 1)
	var decoded decodedCommand
	decoded.deviceID = request.Params.DeviceID
	var commandBody json.RawMessage
	for namespace, methods := range commands {
		for method, raw := range methods {
			decoded.name = namespace + "." + method
			commandBody = raw
		}
	}
	var fields map[string]any
	if len(commandBody) > 0 {
		_ = json.Unmarshal(commandBody, &fields)
	}
	return decoded, fields
}
