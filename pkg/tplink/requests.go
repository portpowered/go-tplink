package tplink

import "github.com/portpowered/go-tplink/pkg/tplinkmodels"

// Client request and result shapes are generated from api/client-models.openapi.yaml.
// These aliases keep the operation types beside the public Client interface.
type (
	AuthContext          = tplinkmodels.AuthContext
	LoginRequest         = tplinkmodels.LoginRequest
	GetDevicesRequest    = tplinkmodels.GetDevicesRequest
	GetDevicesResult     = tplinkmodels.GetDevicesResult
	TurnOnRequest        = tplinkmodels.TurnOnRequest
	TurnOffRequest       = tplinkmodels.TurnOffRequest
	GetPowerStateRequest = tplinkmodels.GetPowerStateRequest
	RebootRequest        = tplinkmodels.RebootRequest
	SetBrightnessRequest = tplinkmodels.SetBrightnessRequest
	SetColorTempRequest  = tplinkmodels.SetColorTempRequest
	SetColorRequest      = tplinkmodels.SetColorRequest
	SetLightStateRequest = tplinkmodels.SetLightStateRequest
	GetLightStateRequest = tplinkmodels.GetLightStateRequest
	GetBrightnessRequest = tplinkmodels.GetBrightnessRequest
	BrightnessResult     = tplinkmodels.BrightnessResult
	GetColorTempRequest  = tplinkmodels.GetColorTempRequest
	ColorTempResult      = tplinkmodels.ColorTempResult
	GetColorRequest      = tplinkmodels.GetColorRequest
	ColorResult          = tplinkmodels.ColorResult
	SetAliasRequest      = tplinkmodels.SetAliasRequest
)
