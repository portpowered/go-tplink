package tplinkmodels_test

import (
	"testing"

	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

func TestDetectCapabilitiesUsesGeneratedCapabilityValues(t *testing.T) {
	t.Parallel()

	var device tplinkmodels.Device

	device.DeviceType = dependencymodels.DeviceTypeSmartBulb
	device.IsDimmable = dependencymodels.DeviceCapabilityEnabled
	device.IsColor = dependencymodels.DeviceCapabilityDisabled
	device.IsVariableColor = dependencymodels.DeviceCapabilityEnabled

	capabilities := tplinkmodels.DetectCapabilities(device)
	if !capabilities.OnOff || !capabilities.Brightness || capabilities.Color || !capabilities.ColorTemp {
		t.Fatalf("DetectCapabilities() = %+v, want enabled values to follow the provider flags", capabilities)
	}

	device.IsDimmable = 2
	if tplinkmodels.DetectCapabilities(device).Brightness {
		t.Fatal("unknown capability value was treated as enabled")
	}
}
