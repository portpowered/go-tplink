// Package tplinkmodels contains generated provider models and handwritten device helpers.
//
//nolint:godoclint // The generated model file also includes its package comment.
package tplinkmodels

import "github.com/portpowered/go-tplink/pkg/dependencymodels"

// ClassifyDevice returns the endpoint type for a device based on its deviceType field.
func ClassifyDevice(deviceType string) EndpointType {
	switch deviceType {
	case dependencymodels.DeviceTypeSmartPlug, dependencymodels.DeviceTypeRangeExtenderPlug:
		return EndpointTypePlug
	case dependencymodels.DeviceTypeSmartBulb:
		return EndpointTypeBulb
	default:
		return EndpointTypeOther
	}
}

// DetectCapabilities infers device capabilities from metadata fields.
func DetectCapabilities(device Device) Capabilities {
	endpointType := ClassifyDevice(device.DeviceType)
	caps := Capabilities{
		OnOff:      endpointType == EndpointTypePlug || endpointType == EndpointTypeBulb,
		Brightness: false,
		Color:      false,
		ColorTemp:  false,
	}

	switch endpointType {
	case EndpointTypePlug:
		if device.Brightness > 0 {
			caps.Brightness = true
		}
	case EndpointTypeBulb:
		caps.Brightness = capabilityEnabled(device.IsDimmable)
		caps.Color = capabilityEnabled(device.IsColor)
		caps.ColorTemp = capabilityEnabled(device.IsVariableColor)
	case EndpointTypeOther:
		return caps
	}

	return caps
}

func capabilityEnabled(value int) bool {
	switch value {
	case dependencymodels.DeviceCapabilityEnabled:
		return true
	case dependencymodels.DeviceCapabilityDisabled:
		return false
	default:
		return false
	}
}
