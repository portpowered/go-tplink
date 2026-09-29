package tplink

import "github.com/portpowered/go-tplink/pkg/tplinkmodels"

// Client request and result shapes are generated from api/client-models.openapi.yaml.
// These aliases keep the operation types beside the public Client interface.
type (
	// AuthContext supplies the session token for an authenticated operation.
	AuthContext = tplinkmodels.AuthContext
	// LoginRequest contains credentials used by Login.
	LoginRequest = tplinkmodels.LoginRequest
	// GetDevicesRequest contains authentication for GetDevices.
	GetDevicesRequest = tplinkmodels.GetDevicesRequest
	// GetDevicesResult contains devices returned by GetDevices.
	GetDevicesResult = tplinkmodels.GetDevicesResult
	// TurnOnRequest contains parameters for TurnOn.
	TurnOnRequest = tplinkmodels.TurnOnRequest
	// TurnOffRequest contains parameters for TurnOff.
	TurnOffRequest = tplinkmodels.TurnOffRequest
	// GetPowerStateRequest contains parameters for GetPowerState.
	GetPowerStateRequest = tplinkmodels.GetPowerStateRequest
	// RebootRequest contains parameters for Reboot.
	RebootRequest = tplinkmodels.RebootRequest
	// SetBrightnessRequest contains parameters for SetBrightness.
	SetBrightnessRequest = tplinkmodels.SetBrightnessRequest
	// SetColorTempRequest contains parameters for SetColorTemp.
	SetColorTempRequest = tplinkmodels.SetColorTempRequest
	// SetColorRequest contains parameters for SetColor.
	SetColorRequest = tplinkmodels.SetColorRequest
	// SetLightStateRequest contains parameters for SetLightState.
	SetLightStateRequest = tplinkmodels.SetLightStateRequest
	// GetLightStateRequest contains parameters for GetLightState.
	GetLightStateRequest = tplinkmodels.GetLightStateRequest
	// GetBrightnessRequest contains parameters for GetBrightness.
	GetBrightnessRequest = tplinkmodels.GetBrightnessRequest
	// BrightnessResult contains the brightness returned by a device.
	BrightnessResult = tplinkmodels.BrightnessResult
	// GetColorTempRequest contains parameters for GetColorTemp.
	GetColorTempRequest = tplinkmodels.GetColorTempRequest
	// ColorTempResult contains the color temperature returned by a device.
	ColorTempResult = tplinkmodels.ColorTempResult
	// GetColorRequest contains parameters for GetColor.
	GetColorRequest = tplinkmodels.GetColorRequest
	// ColorResult contains the color returned by a device.
	ColorResult = tplinkmodels.ColorResult
	// SetAliasRequest contains parameters for SetAlias.
	SetAliasRequest = tplinkmodels.SetAliasRequest
)
