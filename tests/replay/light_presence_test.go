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

// These synthetic responses exercise the real passthrough decoder. In
// particular, omitted and null fields must never become observed zero values.
func TestLightStatePreservesVendorFieldPresence(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name, state string
		want        tplinkmodels.LightStatePresence
	}{
		{"omitted", `{"err_code":0}`,
			tplinkmodels.LightStatePresence{OnOff: false, Brightness: false, Hue: false, Saturation: false, ColorTemp: false}},
		{"null", `{"err_code":0,"brightness":null,"hue":null,"saturation":null,"color_temp":null,"on_off":null}`,
			tplinkmodels.LightStatePresence{OnOff: false, Brightness: false, Hue: false, Saturation: false, ColorTemp: false}},
		{"explicit zeros", `{"err_code":0,"brightness":0,"hue":0,"saturation":0,"color_temp":0,"on_off":0}`,
			tplinkmodels.LightStatePresence{OnOff: true, Brightness: true, Hue: true, Saturation: true, ColorTemp: true}},
		{"white", `{"err_code":0,"on_off":1,"brightness":50,"color_temp":2700}`,
			tplinkmodels.LightStatePresence{OnOff: true, Brightness: true, Hue: false, Saturation: false, ColorTemp: true}},
		{"color", `{"err_code":0,"on_off":1,"brightness":50,"hue":120,"saturation":75,"color_temp":0}`,
			tplinkmodels.LightStatePresence{OnOff: true, Brightness: true, Hue: true, Saturation: true, ColorTemp: true}},
	} {
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
			require.Equal(t, 1, calls)
		})
	}
}
