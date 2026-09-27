package tplink

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// TurnOn turns on a plug device by setting relay_state to 1.
func (client *Client) TurnOn(ctx context.Context, request TurnOnRequest) error {
	cmd := map[string]any{
		NamespaceSystem: map[string]any{
			CmdSetRelayState: map[string]any{"state": 1},
		},
	}
	data, err := client.doPassthrough(ctx, "TurnOn", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}
	return checkPassthroughCommandError(data, NamespaceSystem, CmdSetRelayState, "TurnOn")
}

// TurnOff turns off a plug device by setting relay_state to 0.
func (client *Client) TurnOff(ctx context.Context, request TurnOffRequest) error {
	cmd := map[string]any{
		NamespaceSystem: map[string]any{
			CmdSetRelayState: map[string]any{"state": 0},
		},
	}
	data, err := client.doPassthrough(ctx, "TurnOff", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}
	return checkPassthroughCommandError(data, NamespaceSystem, CmdSetRelayState, "TurnOff")
}

// GetPowerState retrieves the power state of a plug device via get_sysinfo.
func (client *Client) GetPowerState(ctx context.Context, request GetPowerStateRequest) (tplinkmodels.PowerState, error) {
	cmd := map[string]any{
		NamespaceSystem: map[string]any{
			CmdGetSysInfo: "",
		},
	}
	data, err := client.doPassthrough(ctx, "GetPowerState", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return tplinkmodels.PowerState{}, err
	}

	if _, err := passthroughCommandResult(data, NamespaceSystem, CmdGetSysInfo, "GetPowerState"); err != nil {
		return tplinkmodels.PowerState{}, err
	}

	var parsed struct {
		System struct {
			GetSysInfo tplinkmodels.SysInfo `json:"get_sysinfo"`
		} `json:"system"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return tplinkmodels.PowerState{}, tplinkmodels.NewInvalidResponseError("GetPowerState", "failed to parse sysinfo response", err)
	}

	sysInfo := parsed.System.GetSysInfo
	if sysInfo.ErrCode != 0 {
		return tplinkmodels.PowerState{}, tplinkmodels.NewDeviceError(sysInfo.ErrCode, "get_sysinfo failed")
	}

	return tplinkmodels.PowerState{
		IsOn:   sysInfo.RelayState == 1,
		OnTime: sysInfo.OnTime,
	}, nil
}

// Reboot reboots a plug device after a 1-second delay.
func (client *Client) Reboot(ctx context.Context, request RebootRequest) error {
	cmd := map[string]any{
		NamespaceSystem: map[string]any{
			CmdReboot: map[string]any{"delay": 1},
		},
	}
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

	var result struct {
		ErrCode int    `json:"err_code"`
		ErrMsg  string `json:"err_msg"`
	}
	if err := json.Unmarshal(cmdData, &result); err != nil {
		return tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough command result", err)
	}

	if result.ErrCode != 0 {
		if result.ErrCode == -1 {
			return tplinkmodels.NewUnsupportedOperationError(result.ErrMsg)
		}
		return tplinkmodels.NewDeviceError(result.ErrCode, result.ErrMsg)
	}

	return nil
}

func passthroughCommandResult(data []byte, namespace, command, operation string) (json.RawMessage, error) {
	if err := checkDeviceError(data); err != nil {
		return nil, err
	}

	var parsed map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough command response", err)
	}

	ns, ok := parsed[namespace]
	if !ok {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "missing passthrough namespace", nil)
	}

	cmdData, ok := ns[command]
	if !ok || len(cmdData) == 0 || string(cmdData) == "null" {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "missing passthrough command result", nil)
	}
	return cmdData, nil
}
