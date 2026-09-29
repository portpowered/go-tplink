package tplinkmodels

// ClassifyDevice returns the endpoint type for a device based on its deviceType field.
func ClassifyDevice(deviceType string) EndpointType {
	switch deviceType {
	case "IOT.SMARTPLUGSWITCH", "IOT.RANGEEXTENDER.SMARTPLUG":
		return EndpointTypePlug
	case "IOT.SMARTBULB":
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
		caps.Brightness = device.IsDimmable == 1
		caps.Color = device.IsColor == 1
		caps.ColorTemp = device.IsVariableColor == 1
	case EndpointTypeOther:
		return caps
	}

	return caps
}
