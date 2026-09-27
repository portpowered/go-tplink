package tplink

import "github.com/portpowered/go-tplink/pkg/tplinkmodels"

// AuthContext supplies credentials for one account request.
type AuthContext struct {
	AccessToken string
}

// LoginRequest contains the account credentials used to create a cloud session.
type LoginRequest struct {
	Email    string
	Password string
}

// GetDevicesRequest identifies the account whose devices should be listed.
type GetDevicesRequest struct {
	Auth AuthContext
}

// GetDevicesResult contains the devices returned by the cloud account.
type GetDevicesResult struct {
	Devices []tplinkmodels.Device
}

// TurnOnRequest identifies a device to turn on.
type TurnOnRequest struct {
	Auth     AuthContext
	DeviceID string
}

// TurnOffRequest identifies a device to turn off.
type TurnOffRequest struct {
	Auth     AuthContext
	DeviceID string
}

// GetPowerStateRequest identifies a device whose power state should be read.
type GetPowerStateRequest struct {
	Auth     AuthContext
	DeviceID string
}

// RebootRequest identifies a device to reboot.
type RebootRequest struct {
	Auth     AuthContext
	DeviceID string
}

// SetBrightnessRequest specifies a bulb brightness from 0 through 100.
type SetBrightnessRequest struct {
	Auth       AuthContext
	DeviceID   string
	Brightness int
}

// SetColorTempRequest specifies a bulb color temperature in Kelvin.
type SetColorTempRequest struct {
	Auth      AuthContext
	DeviceID  string
	ColorTemp int
}

// SetColorRequest specifies a bulb hue and saturation.
type SetColorRequest struct {
	Auth       AuthContext
	DeviceID   string
	Hue        int
	Saturation int
}

// SetLightStateRequest specifies arbitrary transition_light_state parameters.
type SetLightStateRequest struct {
	Auth     AuthContext
	DeviceID string
	State    map[string]any
}

// GetLightStateRequest identifies a bulb whose state should be read.
type GetLightStateRequest struct {
	Auth     AuthContext
	DeviceID string
}

// GetBrightnessRequest identifies a bulb whose brightness should be read.
type GetBrightnessRequest struct {
	Auth     AuthContext
	DeviceID string
}

// BrightnessResult contains a bulb's current brightness from 0 through 100.
type BrightnessResult struct {
	Brightness int
}

// GetColorTempRequest identifies a bulb whose color temperature should be read.
type GetColorTempRequest struct {
	Auth     AuthContext
	DeviceID string
}

// ColorTempResult contains a bulb's current color temperature in Kelvin.
type ColorTempResult struct {
	ColorTemp int
}

// GetColorRequest identifies a bulb whose hue and saturation should be read.
type GetColorRequest struct {
	Auth     AuthContext
	DeviceID string
}

// ColorResult contains a bulb's current hue and saturation.
type ColorResult struct {
	Hue        int
	Saturation int
}

// SetAliasRequest specifies the display name for a device.
type SetAliasRequest struct {
	Auth     AuthContext
	DeviceID string
	Alias    string
}
