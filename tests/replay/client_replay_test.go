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

var errConnectionRefused = errors.New("connection refused")

func newTestClient(t *testing.T) (*tplink.Client, *replayTransport) {
	t.Helper()

	transport := newReplayTransport()

	t.Cleanup(func() {
		err := transport.assertConsumed()
		if err != nil {
			t.Error(err)
		}
	})

	httpClient := new(http.Client)
	httpClient.Transport = transport
	client := newTestClientWithDoer(t, httpClient)

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
	t.Parallel()
	client, transport := newTestClient(t)
	transport.expectSequence(replayLoginOperation, replayDeviceListMethod, replayDeviceListMethod)
	transport.expectToken(1, "account-one-token")
	transport.expectToken(2, "account two+token&scope=devices")

	ctx := context.Background()
	login, err := client.Login(ctx, tplink.LoginRequest{
		Email:    replayTestEmail,
		Password: replayTestPassword,
	})
	require.NoError(t, err)
	assert.Equal(t, "test-auth-token-abc123", login.Token)
	assert.Equal(t, "acc-123456", login.AccountID)
	assert.Equal(t, replayTestEmail, login.Email)
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
	assert.Equal(t, replayLoginOperation, loginEnvelope.Method)
	assert.Equal(t, "Tapo_Android", loginEnvelope.Params.AppType)
	assert.Equal(t, replayTestEmail, loginEnvelope.Params.CloudUserName)
	assert.Equal(t, replayTestPassword, loginEnvelope.Params.CloudPassword)

	assertRequestScopedDeviceTokens(ctx, t, client, transport)
}

func assertRequestScopedDeviceTokens(
	ctx context.Context,
	t *testing.T,
	client *tplink.Client,
	transport *replayTransport,
) {
	t.Helper()
	// Reuse one client for different request-scoped tokens to ensure auth does
	// not leak from one account request to the next.
	first, err := client.GetDevices(ctx, tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: "account-one-token"},
	})
	require.NoError(t, err)
	require.Len(t, first.Devices, 1)
	assert.Equal(t, replayPlugDeviceID, first.Devices[0].DeviceID)
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
		assert.Equal(t, replayDeviceListMethod, envelope.Method)
	}
}

func TestDeviceListReplayShapes(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.useFixture(replayDeviceListMethod, "getDeviceList_empty")

		result, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: replayTestToken},
		})
		require.NoError(t, err)
		assert.Empty(t, result.Devices)
	})

	t.Run("mixed device types", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.useFixture(replayDeviceListMethod, "getDeviceList_mixed")

		result, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: replayTestToken},
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
	t.Parallel()

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
			t.Parallel()
			client, transport := newTestClient(t)
			transport.useFixture(replayDeviceListMethod, test.fixture)

			_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
				Auth: tplink.AuthContext{AccessToken: replayTestToken},
			})
			require.Error(t, err)
			assert.True(t, test.wantErr(err), "unexpected error type: %T (%v)", err, err)
		})
	}

	t.Run("unrecognized cloud error", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.useResponse(replayDeviceListMethod, http.StatusOK, []byte(`{"error_code":-99999,"msg":"Unknown error"}`))

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: replayTestToken},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsCloudAPIError(err))
	})
}

func TestLoginAndDeviceErrorReplayMappings(t *testing.T) {
	t.Parallel()
	t.Run("rejected login", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.useFixture(replayLoginOperation, "login_failure")

		_, err := client.Login(context.Background(), tplink.LoginRequest{
			Email:    "bad@example.com",
			Password: replayTestPassword,
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsAuthenticationError(err))
	})

	t.Run("unsupported plug operation", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.useFixture(replayOperationPower, "error_unsupported")

		_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
			Auth:     tplink.AuthContext{AccessToken: replayTestToken},
			DeviceID: "device-unknown-001",
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsUnsupportedOperationError(err))
	})

	t.Run("unsupported bulb operation", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.useFixture(replayOperationLightSet, "error_unsupported")
		transport.expectVariant(1)

		err := client.SetBrightness(context.Background(), tplink.SetBrightnessRequest{
			Auth:       tplink.AuthContext{AccessToken: replayTestToken},
			DeviceID:   replayPlugDeviceID,
			Brightness: 50,
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsUnsupportedOperationError(err))
	})

	t.Run("missing token stops before HTTP", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.expectCalls(0)

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: ""},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsTokenNotSetError(err))
		assert.Empty(t, transport.recordedRequests())
	})
}

func TestClosedClientRejectsRequests(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	transport.expectCalls(0)
	require.NoError(t, client.Close())

	_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: replayTestToken},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsClientClosedError(err))
}

type passthroughReplayCase struct {
	name       string
	deviceID   string
	command    string
	variant    int
	wantFields map[string]any
	call       func(*tplink.Client) error
}

func TestPassthroughRequestReplay(t *testing.T) {
	t.Parallel()
	t.Run("plug operations", func(t *testing.T) {
		t.Parallel()
		runPassthroughRequestCases(t, plugCommandReplayCases())
	})
	t.Run("light mutations", func(t *testing.T) {
		t.Parallel()
		runPassthroughRequestCases(t, lightMutationReplayCases())
	})
	t.Run("state reads", func(t *testing.T) {
		t.Parallel()
		runPassthroughRequestCases(t, stateReadReplayCases())
	})
}

func TestNilLightStateUsesGeneratedNullablePayload(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	transport.expectSequence(replayOperationLightSet)
	transport.expectVariant(5)

	err := client.SetLightState(context.Background(), tplink.SetLightStateRequest{
		Auth:     tplink.AuthContext{AccessToken: replayTestToken},
		DeviceID: replayBulbDeviceID,
		State:    nil,
	})
	require.NoError(t, err)

	request := onlyRecordedRequest(t, transport)

	var outer struct {
		Params struct {
			RequestData string `json:"requestData"`
		} `json:"params"`
	}

	decodeErr := json.Unmarshal(request.Body, &outer)
	require.NoError(t, decodeErr)

	var command map[string]map[string]json.RawMessage

	decodeErr = json.Unmarshal([]byte(outer.Params.RequestData), &command)
	require.NoError(t, decodeErr)
	require.Contains(t, command, "smartlife.iot.smartbulb.lightingservice")
	nested := command["smartlife.iot.smartbulb.lightingservice"]
	require.Contains(t, nested, "transition_light_state")
	assert.Equal(t, "null", strings.TrimSpace(string(nested["transition_light_state"])))
}

func runPassthroughRequestCases(t *testing.T, tests []passthroughReplayCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, transport := newTestClient(t)
			parts := strings.Split(test.command, ".")
			operation := "passthrough_" + parts[len(parts)-2] + "_" + parts[len(parts)-1]
			transport.expectSequence(operation)
			transport.expectVariant(test.variant)
			require.NoError(t, test.call(client))
			assertPassthroughReplayRequest(t, transport, test)
		})
	}
}

func assertPassthroughReplayRequest(t *testing.T, transport *replayTransport, test passthroughReplayCase) {
	t.Helper()
	request := onlyRecordedRequest(t, transport)
	assert.Equal(t, replayTestToken, request.URL.Query().Get("token"))
	command, fields := decodePassthroughCommand(t, request.Body)
	assert.Equal(t, test.deviceID, command.deviceID)
	assert.Equal(t, test.command, command.name)

	for key, want := range test.wantFields {
		assert.Equal(t, want, fields[key], "command field %q", key)
	}
}

func plugCommandReplayCases() []passthroughReplayCase {
	auth := tplink.AuthContext{AccessToken: replayTestToken}

	return []passthroughReplayCase{
		{
			name: "turn on plug", deviceID: replayPlugDeviceID, command: "system.set_relay_state",
			variant: 0, wantFields: map[string]any{"state": float64(1)},
			call: func(client *tplink.Client) error {
				return client.TurnOn(context.Background(), tplink.TurnOnRequest{Auth: auth, DeviceID: replayPlugDeviceID})
			},
		},
		{
			name: "turn off plug", deviceID: replayPlugDeviceID, command: "system.set_relay_state",
			variant: 1, wantFields: map[string]any{"state": float64(0)},
			call: func(client *tplink.Client) error {
				return client.TurnOff(context.Background(), tplink.TurnOffRequest{Auth: auth, DeviceID: replayPlugDeviceID})
			},
		},
		{
			name: "reboot plug", deviceID: replayPlugDeviceID, command: "system.reboot",
			variant: 0, wantFields: map[string]any{"delay": float64(1)},
			call: func(client *tplink.Client) error {
				return client.Reboot(context.Background(), tplink.RebootRequest{Auth: auth, DeviceID: replayPlugDeviceID})
			},
		},
		{
			name: "rename device", deviceID: replayPlugDeviceID, command: "system.set_dev_alias",
			variant: 0, wantFields: map[string]any{"alias": "New Name"},
			call: func(client *tplink.Client) error {
				return client.SetAlias(context.Background(), tplink.SetAliasRequest{
					Auth: auth, DeviceID: replayPlugDeviceID, Alias: "New Name",
				})
			},
		},
	}
}

func lightMutationReplayCases() []passthroughReplayCase {
	auth := tplink.AuthContext{AccessToken: replayTestToken}
	command := "smartlife.iot.smartbulb.lightingservice.transition_light_state"

	return []passthroughReplayCase{
		{
			name: "set brightness", deviceID: replayBulbDeviceID, command: command,
			variant: 0, wantFields: map[string]any{replayBrightnessField: float64(50)},
			call: func(client *tplink.Client) error {
				return client.SetBrightness(context.Background(), tplink.SetBrightnessRequest{
					Auth: auth, DeviceID: replayBulbDeviceID, Brightness: 50,
				})
			},
		},
		{
			name: "set color temperature", deviceID: replayBulbDeviceID, command: command,
			variant: 1, wantFields: map[string]any{"color_temp": float64(4000)},
			call: func(client *tplink.Client) error {
				return client.SetColorTemp(context.Background(), tplink.SetColorTempRequest{
					Auth: auth, DeviceID: replayBulbDeviceID, ColorTemp: 4000,
				})
			},
		},
		{
			name: "set color", deviceID: replayBulbDeviceID, command: command,
			variant: 2, wantFields: map[string]any{replayHueField: float64(240), replaySaturationField: float64(80)},
			call: func(client *tplink.Client) error {
				return client.SetColor(context.Background(), tplink.SetColorRequest{
					Auth: auth, DeviceID: replayBulbDeviceID, Hue: 240, Saturation: 80,
				})
			},
		},
		{
			name: "set light state", deviceID: replayBulbDeviceID, command: command,
			variant: 3,
			wantFields: map[string]any{
				"on_off": float64(1), replayBrightnessField: float64(75),
				replayHueField: float64(120), replaySaturationField: float64(50),
			},
			call: func(client *tplink.Client) error {
				return client.SetLightState(context.Background(), tplink.SetLightStateRequest{
					Auth: auth, DeviceID: replayBulbDeviceID,
					State: map[string]any{
						"on_off": 1, replayBrightnessField: 75,
						replayHueField: 120, replaySaturationField: 50,
					},
				})
			},
		},
	}
}

func stateReadReplayCases() []passthroughReplayCase {
	auth := tplink.AuthContext{AccessToken: replayTestToken}

	return []passthroughReplayCase{
		{
			name: "read plug state", deviceID: replayPlugDeviceID, command: "system.get_sysinfo",
			variant: 0, wantFields: nil,
			call: func(client *tplink.Client) error {
				_, err := client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
					Auth: auth, DeviceID: replayPlugDeviceID,
				})

				//nolint:wrapcheck // The replay callback must pass client errors through unchanged.
				return err
			},
		},
		{
			name: "read bulb state", deviceID: replayBulbDeviceID,
			command: replayLightStateCommand, variant: 0, wantFields: nil,
			call: func(client *tplink.Client) error {
				_, err := client.GetLightState(context.Background(), tplink.GetLightStateRequest{
					Auth: auth, DeviceID: replayBulbDeviceID,
				})

				//nolint:wrapcheck // The replay callback must pass client errors through unchanged.
				return err
			},
		},
		{
			name: "read brightness", deviceID: replayBulbDeviceID,
			command: replayLightStateCommand, variant: 0, wantFields: nil,
			call: func(client *tplink.Client) error {
				_, err := client.GetBrightness(context.Background(), tplink.GetBrightnessRequest{
					Auth: auth, DeviceID: replayBulbDeviceID,
				})

				//nolint:wrapcheck // The replay callback must pass client errors through unchanged.
				return err
			},
		},
		{
			name: "read color temperature", deviceID: replayBulbDeviceID,
			command: replayLightStateCommand, variant: 0, wantFields: nil,
			call: func(client *tplink.Client) error {
				_, err := client.GetColorTemp(context.Background(), tplink.GetColorTempRequest{
					Auth: auth, DeviceID: replayBulbDeviceID,
				})

				//nolint:wrapcheck // The replay callback must pass client errors through unchanged.
				return err
			},
		},
		{
			name: "read color", deviceID: replayBulbDeviceID,
			command: replayLightStateCommand, variant: 0, wantFields: nil,
			call: func(client *tplink.Client) error {
				_, err := client.GetColor(context.Background(), tplink.GetColorRequest{
					Auth: auth, DeviceID: replayBulbDeviceID,
				})

				//nolint:wrapcheck // The replay callback must pass client errors through unchanged.
				return err
			},
		},
	}
}
func TestPowerAndLightStateReplayResults(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	transport.expectSequence(
		replayOperationPower,
		replayOperationLightGet,
		replayOperationLightGet,
		replayOperationLightGet,
		replayOperationLightGet,
	)

	ctx := context.Background()
	auth := tplink.AuthContext{AccessToken: replayTestToken}

	power, err := client.GetPowerState(ctx, tplink.GetPowerStateRequest{Auth: auth, DeviceID: replayPlugDeviceID})
	require.NoError(t, err)
	assert.True(t, power.IsOn)
	assert.Equal(t, 3600, power.OnTime)

	light, err := client.GetLightState(ctx, tplink.GetLightStateRequest{Auth: auth, DeviceID: replayBulbDeviceID})
	require.NoError(t, err)
	assert.Equal(t, 1, light.OnOff)
	assert.Equal(t, 75, light.Brightness)
	assert.Equal(t, 240, light.Hue)
	assert.Equal(t, 80, light.Saturation)
	assert.Equal(t, 0, light.ColorTemp)
	assert.Equal(t, "normal", light.Mode)

	brightness, err := client.GetBrightness(ctx, tplink.GetBrightnessRequest{Auth: auth, DeviceID: replayBulbDeviceID})
	require.NoError(t, err)
	assert.Equal(t, 75, brightness.Brightness)

	colorTemp, err := client.GetColorTemp(ctx, tplink.GetColorTempRequest{Auth: auth, DeviceID: replayBulbDeviceID})
	require.NoError(t, err)
	assert.Equal(t, 0, colorTemp.ColorTemp)

	color, err := client.GetColor(ctx, tplink.GetColorRequest{Auth: auth, DeviceID: replayBulbDeviceID})
	require.NoError(t, err)
	assert.Equal(t, 240, color.Hue)
	assert.Equal(t, 80, color.Saturation)
}

func TestMalformedAndHTTPFailureResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		body   []byte
		want   func(error) bool
	}{
		{
			name: "http server failure", status: http.StatusInternalServerError,
			body: []byte(`{"error":"failed"}`), want: tplinkmodels.IsHTTPStatusError,
		},
		{
			name: "malformed JSON", status: http.StatusOK,
			body: []byte(`{"error_code":`), want: tplinkmodels.IsInvalidResponseError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, transport := newTestClient(t)
			transport.useResponse(replayDeviceListMethod, test.status, test.body)

			_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
				Auth: tplink.AuthContext{AccessToken: replayTestToken},
			})
			require.Error(t, err)
			assert.True(t, test.want(err), "unexpected error type: %T (%v)", err, err)
		})
	}
}

func TestPassthroughMissingCommandIsInvalidResponse(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	transport.expectSequence(replayOperationRelay, replayOperationPower)
	transport.useResponse(replayOperationRelay, http.StatusOK,
		[]byte(`{"error_code":0,"result":{"responseData":"{\"system\":{}}"}}`))
	transport.useResponse(replayOperationPower, http.StatusOK,
		[]byte(`{"error_code":0,"result":{"responseData":"{\"system\":{}}"}}`))

	auth := tplink.AuthContext{AccessToken: replayTestToken}
	err := client.TurnOn(context.Background(), tplink.TurnOnRequest{Auth: auth, DeviceID: replayPlugDeviceID})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsInvalidResponseError(err))

	_, err = client.GetPowerState(context.Background(), tplink.GetPowerStateRequest{
		Auth: auth, DeviceID: replayPlugDeviceID,
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsInvalidResponseError(err))
}

func TestOversizedAndNilHTTPResponses(t *testing.T) {
	t.Parallel()
	t.Run("oversized body", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.useResponse(replayDeviceListMethod, http.StatusOK, []byte(strings.Repeat(" ", (1<<20)+1)))

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: replayTestToken},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsResponseTooLargeError(err))
	})

	t.Run("nil response", func(t *testing.T) {
		t.Parallel()

		transport := newReplayTransport()
		transport.useFault("fault-device-nil-response", nil, nil)
		client := newTestClientWithDoer(t, httpDoerFunc(transport.RoundTrip))
		t.Cleanup(func() { require.NoError(t, transport.assertConsumed()) })

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: replayTestToken},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsInvalidResponseError(err))
	})

	t.Run("nil response body", func(t *testing.T) {
		t.Parallel()
		client, transport := newTestClient(t)
		transport.useFault("fault-device-nil-body", nil, nil)

		_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
			Auth: tplink.AuthContext{AccessToken: replayTestToken},
		})
		require.Error(t, err)
		assert.True(t, tplinkmodels.IsInvalidResponseError(err))
	})
}

func TestTransportErrorsDoNotLeakRequestTokens(t *testing.T) {
	t.Parallel()
	client, transport := newTestClient(t)
	transportFailure := errConnectionRefused
	transport.useError(replayDeviceListMethod, transportFailure)

	token := "private token+&scope=devices"
	transport.expectToken(0, token)

	_, err := client.GetDevices(context.Background(), tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: token},
	})
	require.Error(t, err)
	assert.True(t, tplinkmodels.IsNetworkError(err))
	assert.NotContains(t, err.Error(), token)
	assert.NotContains(t, err.Error(), url.QueryEscape(token))
	require.ErrorIs(t, err, transportFailure)

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
