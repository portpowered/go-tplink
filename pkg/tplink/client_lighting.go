package tplink

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// transitionLightState sends a transition_light_state command with the given parameters.
func (client *Client) transitionLightState(
	ctx context.Context,
	operation string,
	auth AuthContext,
	deviceID string,
	command any,
) error {
	data, err := client.doPassthrough(ctx, operation, auth, deviceID, command)
	if err != nil {
		return err
	}

	return checkPassthroughCommandError(
		data,
		dependencymodels.NamespaceLightingService,
		dependencymodels.CmdTransitionLightState,
		operation,
	)
}

func lightingTransitionCommand(
	state *dependencymodels.LightTransitionState,
) dependencymodels.LightingTransitionLightStateCommand {
	var command dependencymodels.LightingTransitionLightStateCommand

	command.SmartlifeIotSmartbulbLightingservice.TransitionLightState = state

	return command
}

// SetLightState sets arbitrary light state parameters on a bulb device.
// This is the low-level method; prefer the specific methods below.
func (client *Client) SetLightState(ctx context.Context, request SetLightStateRequest) error {
	if request.State == nil {
		return client.transitionLightState(
			ctx,
			"SetLightState",
			request.Auth,
			request.DeviceID,
			lightingTransitionCommand(nil),
		)
	}

	var state dependencymodels.LightTransitionState

	state.AdditionalProperties = request.State

	return client.transitionLightState(
		ctx,
		"SetLightState",
		request.Auth,
		request.DeviceID,
		lightingTransitionCommand(&state),
	)
}

// SetBrightness sets the brightness of a bulb device (0-100).
func (client *Client) SetBrightness(ctx context.Context, request SetBrightnessRequest) error {
	var state dependencymodels.LightTransitionState

	state.Brightness = &request.Brightness

	return client.transitionLightState(
		ctx,
		"SetBrightness",
		request.Auth,
		request.DeviceID,
		lightingTransitionCommand(&state),
	)
}

// SetColorTemp sets the color temperature of a bulb device in Kelvin.
func (client *Client) SetColorTemp(ctx context.Context, request SetColorTempRequest) error {
	var state dependencymodels.LightTransitionState

	state.ColorTemp = &request.ColorTemp

	return client.transitionLightState(
		ctx,
		"SetColorTemp",
		request.Auth,
		request.DeviceID,
		lightingTransitionCommand(&state),
	)
}

// SetColor sets the hue (0-360) and saturation (0-100) of a bulb device.
func (client *Client) SetColor(ctx context.Context, request SetColorRequest) error {
	var state dependencymodels.LightTransitionState

	state.Hue = &request.Hue

	state.Saturation = &request.Saturation

	return client.transitionLightState(
		ctx,
		"SetColor",
		request.Auth,
		request.DeviceID,
		lightingTransitionCommand(&state),
	)
}

// GetLightState retrieves the current light state of a bulb device.
func (client *Client) GetLightState(
	ctx context.Context,
	request GetLightStateRequest,
) (tplinkmodels.LightState, error) {
	var cmd dependencymodels.LightingGetLightStateCommand

	cmd.SmartlifeIotSmartbulbLightingservice.GetLightState =
		dependencymodels.LightingGetLightStateCommandSmartlifeIotSmartbulbLightingserviceGetLightStateEmpty

	data, err := client.doPassthrough(ctx, "GetLightState", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return tplinkmodels.LightState{}, err
	}

	_, err = passthroughCommandResult(
		data,
		dependencymodels.NamespaceLightingService,
		dependencymodels.CmdGetLightState,
		"GetLightState",
	)
	if err != nil {
		return tplinkmodels.LightState{}, err
	}

	var parsed dependencymodels.LightingCommandResult

	decodeErr := json.Unmarshal(data, &parsed)
	if decodeErr != nil {
		return tplinkmodels.LightState{}, tplinkmodels.NewInvalidResponseError(
			"GetLightState",
			"failed to parse light state response",
			decodeErr,
		)
	}

	if parsed.SmartlifeIotSmartbulbLightingservice == nil ||
		parsed.SmartlifeIotSmartbulbLightingservice.GetLightState == nil {
		return tplinkmodels.LightState{}, tplinkmodels.NewInvalidResponseError(
			"GetLightState",
			"missing light state result",
			nil,
		)
	}

	wireState := parsed.SmartlifeIotSmartbulbLightingservice.GetLightState

	lightState := tplinkmodels.LightState{
		OnOff:      valueOrZero(wireState.OnOff),
		Brightness: valueOrZero(wireState.Brightness),
		Hue:        valueOrZero(wireState.Hue),
		Saturation: valueOrZero(wireState.Saturation),
		ColorTemp:  valueOrZero(wireState.ColorTemp),
		Mode:       valueOrZero(wireState.Mode),
		ErrCode:    valueOrZero(wireState.ErrCode),
	}
	if lightState.ErrCode != dependencymodels.DeviceErrorOK {
		return tplinkmodels.LightState{}, tplinkmodels.NewDeviceError(lightState.ErrCode, "get_light_state failed")
	}

	return lightState, nil
}

// GetBrightness retrieves the current brightness of a bulb device.
func (client *Client) GetBrightness(ctx context.Context, request GetBrightnessRequest) (BrightnessResult, error) {
	lightState, err := client.GetLightState(ctx, GetLightStateRequest(request))
	if err != nil {
		return BrightnessResult{}, err
	}

	return BrightnessResult{Brightness: lightState.Brightness}, nil
}

// GetColorTemp retrieves the current color temperature of a bulb device.
func (client *Client) GetColorTemp(ctx context.Context, request GetColorTempRequest) (ColorTempResult, error) {
	lightState, err := client.GetLightState(ctx, GetLightStateRequest(request))
	if err != nil {
		return ColorTempResult{}, err
	}

	return ColorTempResult{ColorTemp: lightState.ColorTemp}, nil
}

// GetColor retrieves the current hue and saturation of a bulb device.
func (client *Client) GetColor(ctx context.Context, request GetColorRequest) (ColorResult, error) {
	lightState, err := client.GetLightState(ctx, GetLightStateRequest(request))
	if err != nil {
		return ColorResult{}, err
	}

	return ColorResult{Hue: lightState.Hue, Saturation: lightState.Saturation}, nil
}
