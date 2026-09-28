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
func DetectCapabilities(d Device) Capabilities {
	t := ClassifyDevice(d.DeviceType)
	caps := Capabilities{OnOff: t == EndpointTypePlug || t == EndpointTypeBulb}

	switch t {
	case EndpointTypePlug:
		if d.Brightness > 0 {
			caps.Brightness = true
		}
	case EndpointTypeBulb:
		caps.Brightness = d.IsDimmable == 1
		caps.Color = d.IsColor == 1
		caps.ColorTemp = d.IsVariableColor == 1
	}

	return caps
}
