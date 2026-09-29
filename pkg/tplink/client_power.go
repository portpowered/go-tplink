package tplink

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-tplink/pkg/generatedwire"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// TurnOn turns on a plug device by setting relay_state to 1.
func (client *Client) TurnOn(ctx context.Context, request TurnOnRequest) error {
	var cmd generatedwire.SystemSetRelayStateCommand

	cmd.System.SetRelayState.State = generatedwire.SystemSetRelayStateCommandSystemSetRelayStateStateN1

	data, err := client.doPassthrough(ctx, "TurnOn", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}

	return checkPassthroughCommandError(data, NamespaceSystem, CmdSetRelayState, "TurnOn")
}

// TurnOff turns off a plug device by setting relay_state to 0.
func (client *Client) TurnOff(ctx context.Context, request TurnOffRequest) error {
	var cmd generatedwire.SystemSetRelayStateCommand

	cmd.System.SetRelayState.State = generatedwire.SystemSetRelayStateCommandSystemSetRelayStateStateN0

	data, err := client.doPassthrough(ctx, "TurnOff", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}

	return checkPassthroughCommandError(data, NamespaceSystem, CmdSetRelayState, "TurnOff")
}

// GetPowerState retrieves the power state of a plug device via get_sysinfo.
func (client *Client) GetPowerState(
	ctx context.Context,
	request GetPowerStateRequest,
) (tplinkmodels.PowerState, error) {
	var cmd generatedwire.SystemGetSysInfoCommand

	cmd.System.GetSysinfo = generatedwire.SystemGetSysInfoCommandSystemGetSysinfoEmpty

	data, err := client.doPassthrough(ctx, "GetPowerState", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return tplinkmodels.PowerState{}, err
	}

	_, err = passthroughCommandResult(data, NamespaceSystem, CmdGetSysInfo, "GetPowerState")
	if err != nil {
		return tplinkmodels.PowerState{}, err
	}

	var parsed generatedwire.SystemCommandResult

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
	if errCode := valueOrZero(sysInfo.ErrCode); errCode != 0 {
		return tplinkmodels.PowerState{}, tplinkmodels.NewDeviceError(errCode, "get_sysinfo failed")
	}

	return tplinkmodels.PowerState{
		IsOn:   valueOrZero(sysInfo.RelayState) == 1,
		OnTime: valueOrZero(sysInfo.OnTime),
	}, nil
}

// Reboot reboots a plug device after a 1-second delay.
func (client *Client) Reboot(ctx context.Context, request RebootRequest) error {
	var cmd generatedwire.SystemRebootCommand

	cmd.System.Reboot.Delay = generatedwire.SystemRebootCommandSystemRebootDelayN1

	data, err := client.doPassthrough(ctx, "Reboot", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}

	return checkPassthroughCommandError(data, NamespaceSystem, CmdReboot, "Reboot")
}

// checkPassthroughCommandError extracts and checks the err_code from a
// namespaced passthrough response like {"system":{"set_relay_state":{"err_code":0}}}.
func checkPassthroughCommandError(data []byte, namespace, command, operation string) error {
	cmdData, err := passthroughCommandResult(data, namespace, command, operation)
	if err != nil {
		return err
	}

	var result generatedwire.CommandAcknowledgement

	decodeErr := json.Unmarshal(cmdData, &result)
	if decodeErr != nil {
		return tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough command result", decodeErr)
	}

	errCode := valueOrZero(result.ErrCode)
	if errCode != 0 {
		errMsg := valueOrZero(result.ErrMsg)
		if errCode == -1 {
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

	var parsed map[string]map[string]json.RawMessage

	decodeErr := json.Unmarshal(data, &parsed)
	if decodeErr != nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough command response", decodeErr)
	}

	namespaceResult, found := parsed[namespace]
	if !found {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "missing passthrough namespace", nil)
	}

	cmdData, found := namespaceResult[command]
	if !found || len(cmdData) == 0 || string(cmdData) == "null" {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "missing passthrough command result", nil)
	}

	return cmdData, nil
}
