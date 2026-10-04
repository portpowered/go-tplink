package tplink

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// TurnOn turns on a plug device by setting relay_state to 1.
func (client *Client) TurnOn(ctx context.Context, request TurnOnRequest) error {
	var cmd dependencymodels.SystemSetRelayStateCommand

	cmd.System.SetRelayState.State = dependencymodels.SystemSetRelayStateCommandSystemSetRelayStateStateN1

	data, err := client.doPassthrough(ctx, "TurnOn", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}

	return checkPassthroughCommandError(
		data,
		dependencymodels.NamespaceSystem,
		dependencymodels.CmdSetRelayState,
		"TurnOn",
	)
}

// TurnOff turns off a plug device by setting relay_state to 0.
func (client *Client) TurnOff(ctx context.Context, request TurnOffRequest) error {
	var cmd dependencymodels.SystemSetRelayStateCommand

	cmd.System.SetRelayState.State = dependencymodels.SystemSetRelayStateCommandSystemSetRelayStateStateN0

	data, err := client.doPassthrough(ctx, "TurnOff", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}

	return checkPassthroughCommandError(
		data,
		dependencymodels.NamespaceSystem,
		dependencymodels.CmdSetRelayState,
		"TurnOff",
	)
}

// GetPowerState retrieves the power state of a plug device via get_sysinfo.
func (client *Client) GetPowerState(
	ctx context.Context,
	request GetPowerStateRequest,
) (tplinkmodels.PowerState, error) {
	var cmd dependencymodels.SystemGetSysInfoCommand

	cmd.System.GetSysinfo = dependencymodels.SystemGetSysInfoCommandSystemGetSysinfoEmpty

	data, err := client.doPassthrough(ctx, "GetPowerState", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return tplinkmodels.PowerState{}, err
	}

	_, err = passthroughCommandResult(
		data,
		dependencymodels.NamespaceSystem,
		dependencymodels.CmdGetSysInfo,
		"GetPowerState",
	)
	if err != nil {
		return tplinkmodels.PowerState{}, err
	}

	var parsed dependencymodels.SystemCommandResult

	decodeErr := json.Unmarshal(data, &parsed)
	if decodeErr != nil {
		return tplinkmodels.PowerState{}, tplinkmodels.NewInvalidResponseError(
			"GetPowerState",
			"failed to parse sysinfo response",
			decodeErr,
		)
	}

	if parsed.System == nil || parsed.System.GetSysinfo == nil {
		return tplinkmodels.PowerState{}, tplinkmodels.NewInvalidResponseError("GetPowerState", "missing sysinfo result", nil)
	}

	sysInfo := parsed.System.GetSysinfo
	if errCode := valueOrZero(sysInfo.ErrCode); errCode != dependencymodels.DeviceErrorOK {
		return tplinkmodels.PowerState{}, tplinkmodels.NewDeviceError(errCode, "get_sysinfo failed")
	}

	return tplinkmodels.PowerState{
		IsOn:   valueOrZero(sysInfo.RelayState) == dependencymodels.DeviceStateOn,
		OnTime: valueOrZero(sysInfo.OnTime),
	}, nil
}

// Reboot reboots a plug device after a 1-second delay.
func (client *Client) Reboot(ctx context.Context, request RebootRequest) error {
	var cmd dependencymodels.SystemRebootCommand

	cmd.System.Reboot.Delay = dependencymodels.SystemRebootCommandSystemRebootDelayN1

	data, err := client.doPassthrough(ctx, "Reboot", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}

	return checkPassthroughCommandError(data, dependencymodels.NamespaceSystem, dependencymodels.CmdReboot, "Reboot")
}

// checkPassthroughCommandError extracts and checks the err_code from a
// namespaced passthrough response like {"system":{"set_relay_state":{"err_code":0}}}.
func checkPassthroughCommandError(data []byte, namespace, command, operation string) error {
	cmdData, err := passthroughCommandResult(data, namespace, command, operation)
	if err != nil {
		return err
	}

	var result dependencymodels.CommandAcknowledgement

	decodeErr := json.Unmarshal(cmdData, &result)
	if decodeErr != nil {
		return tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough command result", decodeErr)
	}

	errCode := valueOrZero(result.ErrCode)
	if errCode != dependencymodels.DeviceErrorOK {
		errMsg := valueOrZero(result.ErrMsg)
		if errCode == dependencymodels.DeviceErrorUnsupported {
			return tplinkmodels.NewUnsupportedOperationError(errMsg)
		}

		return tplinkmodels.NewDeviceError(errCode, errMsg)
	}

	return nil
}

func passthroughCommandResult(data []byte, namespace, command, operation string) (json.RawMessage, error) {
	err := checkDeviceError(data)
	if err != nil {
		return nil, err
	}

	var parsed dependencymodels.PassthroughCommandResult

	decodeErr := json.Unmarshal(data, &parsed)
	if decodeErr != nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough command response", decodeErr)
	}

	var commandResult any

	switch namespace {
	case dependencymodels.NamespaceSystem:
		commandResult, err = systemCommandResult(&parsed, command, operation)
	case dependencymodels.NamespaceLightingService:
		commandResult, err = lightingCommandResult(&parsed, command, operation)
	default:
		return nil, tplinkmodels.NewInvalidResponseError(operation, "missing passthrough namespace", nil)
	}

	if err != nil {
		return nil, err
	}

	if commandResult == nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "missing passthrough command result", nil)
	}

	commandData, marshalErr := json.Marshal(commandResult)
	if marshalErr != nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "failed to encode passthrough command result", marshalErr)
	}

	return commandData, nil
}

func systemCommandResult(
	parsed *dependencymodels.PassthroughCommandResult,
	command string,
	operation string,
) (any, error) {
	result, err := parsed.AsSystemCommandResult()
	if err != nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough system result", err)
	}

	if result.System == nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "missing passthrough namespace", nil)
	}

	value, found := systemResultValue(result.System, command)
	if !found {
		return noCommandResult()
	}

	return value, nil
}

func systemResultValue(result *dependencymodels.SystemCommandResult_System, command string) (any, bool) {
	switch command {
	case dependencymodels.CmdGetSysInfo:
		return result.GetSysinfo, result.GetSysinfo != nil
	case dependencymodels.CmdSetRelayState:
		return result.SetRelayState, result.SetRelayState != nil
	case dependencymodels.CmdReboot:
		return result.Reboot, result.Reboot != nil
	case dependencymodels.CmdSetDevAlias:
		return result.SetDevAlias, result.SetDevAlias != nil
	default:
		return nil, false
	}
}

func noCommandResult() (any, error) {
	//nolint:nilnil // Missing optional command data uses the public contract's nil-result success.
	return nil, nil
}

func lightingCommandResult(
	parsed *dependencymodels.PassthroughCommandResult,
	command string,
	operation string,
) (any, error) {
	result, err := parsed.AsLightingCommandResult()
	if err != nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough lighting result", err)
	}

	if result.SmartlifeIotSmartbulbLightingservice == nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "missing passthrough namespace", nil)
	}

	value, found := lightingResultValue(result.SmartlifeIotSmartbulbLightingservice, command)
	if !found {
		return noCommandResult()
	}

	return value, nil
}

func lightingResultValue(
	result *dependencymodels.LightingCommandResult_SmartlifeIotSmartbulbLightingservice,
	command string,
) (any, bool) {
	switch command {
	case dependencymodels.CmdGetLightState:
		return result.GetLightState, result.GetLightState != nil
	case dependencymodels.CmdTransitionLightState:
		return result.TransitionLightState, result.TransitionLightState != nil
	default:
		return nil, false
	}
}
