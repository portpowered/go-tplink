package tplink

import (
	"context"

	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// ClientInterface defines the contract for the TP-Link Cloud API client.
// Used for dependency injection and mocking in tests.
type ClientInterface interface {
	// Auth
	Login(ctx context.Context, request LoginRequest) (tplinkmodels.LoginResult, error)

	// Device enumeration
	GetDevices(ctx context.Context, request GetDevicesRequest) (GetDevicesResult, error)

	// Power control (plugs)
	TurnOn(ctx context.Context, request TurnOnRequest) error
	TurnOff(ctx context.Context, request TurnOffRequest) error
	GetPowerState(ctx context.Context, request GetPowerStateRequest) (tplinkmodels.PowerState, error)
	Reboot(ctx context.Context, request RebootRequest) error

	// Lighting control (bulbs)
	SetBrightness(ctx context.Context, request SetBrightnessRequest) error
	SetColorTemp(ctx context.Context, request SetColorTempRequest) error
	SetColor(ctx context.Context, request SetColorRequest) error
	SetLightState(ctx context.Context, request SetLightStateRequest) error
	GetLightState(ctx context.Context, request GetLightStateRequest) (tplinkmodels.LightState, error)
	GetBrightness(ctx context.Context, request GetBrightnessRequest) (BrightnessResult, error)
	GetColorTemp(ctx context.Context, request GetColorTempRequest) (ColorTempResult, error)
	GetColor(ctx context.Context, request GetColorRequest) (ColorResult, error)

	// Device management
	SetAlias(ctx context.Context, request SetAliasRequest) error

	// Lifecycle
	Close() error
}

// Compile-time check that Client implements ClientInterface.
var _ ClientInterface = (*Client)(nil)
