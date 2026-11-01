package replay_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/go-tplink/pkg/tplink"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
	"github.com/stretchr/testify/require"
)

type lightPresenceCase struct {
	name, state string
	want        tplinkmodels.LightStatePresence
}

const (
	lightPresenceWhite      = "white sample"
	lightPresenceColor      = "color sample"
	lightPresenceHue        = "hue sample"
	lightPresenceSaturation = "saturation sample"
)

func lightPresenceCases() []lightPresenceCase {
	return []lightPresenceCase{
		{"omitted", `{"err_code":0}`,
			tplinkmodels.LightStatePresence{OnOff: false, Brightness: false, Hue: false, Saturation: false, ColorTemp: false}},
		{"null", `{"err_code":0,"brightness":null,"hue":null,"saturation":null,"color_temp":null,"on_off":null}`,
			tplinkmodels.LightStatePresence{OnOff: false, Brightness: false, Hue: false, Saturation: false, ColorTemp: false}},
		{"explicit zeros", `{"err_code":0,"brightness":0,"hue":0,"saturation":0,"color_temp":0,"on_off":0}`,
			tplinkmodels.LightStatePresence{OnOff: true, Brightness: true, Hue: true, Saturation: true, ColorTemp: true}},
		{lightPresenceWhite, `{"err_code":0,"on_off":1,"brightness":50,"color_temp":2700}`,
			tplinkmodels.LightStatePresence{OnOff: true, Brightness: true, Hue: false, Saturation: false, ColorTemp: true}},
		{lightPresenceColor, `{"err_code":0,"on_off":1,"brightness":50,"hue":120,"saturation":75,"color_temp":0}`,
			tplinkmodels.LightStatePresence{OnOff: true, Brightness: true, Hue: true, Saturation: true, ColorTemp: true}},
		{lightPresenceHue, `{"err_code":0,"hue":120}`,
			tplinkmodels.LightStatePresence{OnOff: false, Brightness: false, Hue: true, Saturation: false, ColorTemp: false}},
		{lightPresenceSaturation, `{"err_code":0,"saturation":75}`,
			tplinkmodels.LightStatePresence{OnOff: false, Brightness: false, Hue: false, Saturation: true, ColorTemp: false}},
	}
}

// These synthetic responses exercise the real passthrough decoder. In
// particular, omitted and null fields must never become observed zero values.
func TestLightStatePreservesVendorFieldPresence(t *testing.T) {
	t.Parallel()

	for _, test := range lightPresenceCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			client := newTestClientWithDoer(t, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
				calls++

				require.Equal(t, http.MethodPost, request.Method)
				require.Equal(t, "fixture-token", request.URL.Query().Get("token"))

				var envelope struct {
					Method string `json:"method"`
					Params struct {
						DeviceID    string `json:"deviceId"`
						RequestData string `json:"requestData"`
					} `json:"params"`
				}

				require.NoError(t, json.NewDecoder(request.Body).Decode(&envelope))
				require.Equal(t, "passthrough", envelope.Method)
				require.Equal(t, "fixture-bulb", envelope.Params.DeviceID)
				require.JSONEq(t, `{"smartlife.iot.smartbulb.lightingservice":{"get_light_state":""}}`, envelope.Params.RequestData)

				wire := `{"smartlife.iot.smartbulb.lightingservice":{"get_light_state":` + test.state + `}}`
				quoted, err := json.Marshal(wire)
				require.NoError(t, err)

				body := `{"error_code":0,"result":{"responseData":` + string(quoted) + `}}`
				response := httptest.NewRecorder()
				_, err = response.WriteString(body)
				require.NoError(t, err)

				return response.Result(), nil
			}))
			state, err := client.GetLightState(context.Background(), tplink.GetLightStateRequest{
				Auth: tplink.AuthContext{AccessToken: "fixture-token"}, DeviceID: "fixture-bulb",
			})
			require.NoError(t, err)
			require.NotNil(t, state.Present)
			require.Equal(t, test.want, *state.Present)
			assertLightNumericValues(t, test.name, state)
			require.Equal(t, 1, calls)
		})
	}
}

func assertLightNumericValues(t *testing.T, name string, state tplinkmodels.LightState) {
	t.Helper()

	if name == lightPresenceColor || name == lightPresenceHue {
		require.Equal(t, 120, state.Hue)
	}

	if name == lightPresenceColor || name == lightPresenceSaturation {
		require.Equal(t, 75, state.Saturation)
	}

	if name == lightPresenceWhite || name == lightPresenceColor {
		require.Equal(t, 50, state.Brightness)
		require.Equal(t, 1, state.OnOff)
	}

	if name == lightPresenceWhite {
		require.Equal(t, 2700, state.ColorTemp)
	}
}
