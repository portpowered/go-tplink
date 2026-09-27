package tplink

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// transitionLightState sends a transition_light_state command with the given parameters.
func (client *Client) transitionLightState(ctx context.Context, operation string, auth AuthContext, deviceID string, params map[string]any) error {
	cmd := map[string]any{
		NamespaceLightingService: map[string]any{
			CmdTransitionLightState: params,
		},
	}
	data, err := client.doPassthrough(ctx, operation, auth, deviceID, cmd)
	if err != nil {
		return err
	}
	return checkPassthroughCommandError(data, NamespaceLightingService, CmdTransitionLightState, operation)
}

// SetLightState sets arbitrary light state parameters on a bulb device.
// This is the low-level method; prefer the specific methods below.
func (client *Client) SetLightState(ctx context.Context, request SetLightStateRequest) error {
	return client.transitionLightState(ctx, "SetLightState", request.Auth, request.DeviceID, request.State)
}

// SetBrightness sets the brightness of a bulb device (0-100).
func (client *Client) SetBrightness(ctx context.Context, request SetBrightnessRequest) error {
	return client.transitionLightState(ctx, "SetBrightness", request.Auth, request.DeviceID, map[string]any{"brightness": request.Brightness})
}

// SetColorTemp sets the color temperature of a bulb device in Kelvin.
func (client *Client) SetColorTemp(ctx context.Context, request SetColorTempRequest) error {
	return client.transitionLightState(ctx, "SetColorTemp", request.Auth, request.DeviceID, map[string]any{"color_temp": request.ColorTemp})
}

// SetColor sets the hue (0-360) and saturation (0-100) of a bulb device.
func (client *Client) SetColor(ctx context.Context, request SetColorRequest) error {
	return client.transitionLightState(ctx, "SetColor", request.Auth, request.DeviceID, map[string]any{"hue": request.Hue, "saturation": request.Saturation})
}

// GetLightState retrieves the current light state of a bulb device.
func (client *Client) GetLightState(ctx context.Context, request GetLightStateRequest) (tplinkmodels.LightState, error) {
	cmd := map[string]any{
		NamespaceLightingService: map[string]any{
			CmdGetLightState: "",
		},
	}
	data, err := client.doPassthrough(ctx, "GetLightState", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return tplinkmodels.LightState{}, err
	}

	if _, err := passthroughCommandResult(data, NamespaceLightingService, CmdGetLightState, "GetLightState"); err != nil {
		return tplinkmodels.LightState{}, err
	}

	var parsed struct {
		LightingService struct {
			GetLightState tplinkmodels.LightState `json:"get_light_state"`
		} `json:"smartlife.iot.smartbulb.lightingservice"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return tplinkmodels.LightState{}, tplinkmodels.NewInvalidResponseError("GetLightState", "failed to parse light state response", err)
	}

	ls := parsed.LightingService.GetLightState
	if ls.ErrCode != 0 {
		return tplinkmodels.LightState{}, tplinkmodels.NewDeviceError(ls.ErrCode, "get_light_state failed")
	}

	return ls, nil
}

// GetBrightness retrieves the current brightness of a bulb device.
func (client *Client) GetBrightness(ctx context.Context, request GetBrightnessRequest) (BrightnessResult, error) {
	ls, err := client.GetLightState(ctx, GetLightStateRequest{Auth: request.Auth, DeviceID: request.DeviceID})
	if err != nil {
		return BrightnessResult{}, err
	}
	return BrightnessResult{Brightness: ls.Brightness}, nil
}

// GetColorTemp retrieves the current color temperature of a bulb device.
func (client *Client) GetColorTemp(ctx context.Context, request GetColorTempRequest) (ColorTempResult, error) {
	ls, err := client.GetLightState(ctx, GetLightStateRequest{Auth: request.Auth, DeviceID: request.DeviceID})
	if err != nil {
		return ColorTempResult{}, err
	}
	return ColorTempResult{ColorTemp: ls.ColorTemp}, nil
}

// GetColor retrieves the current hue and saturation of a bulb device.
func (client *Client) GetColor(ctx context.Context, request GetColorRequest) (ColorResult, error) {
	ls, err := client.GetLightState(ctx, GetLightStateRequest{Auth: request.Auth, DeviceID: request.DeviceID})
	if err != nil {
		return ColorResult{}, err
	}
	return ColorResult{Hue: ls.Hue, Saturation: ls.Saturation}, nil
}
