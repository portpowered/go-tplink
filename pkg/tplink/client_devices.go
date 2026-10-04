package tplink

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/portpowered/go-tplink/pkg/dependencies/cloud"
	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// GetDevices retrieves all devices registered to the authenticated account.
func (client *Client) GetDevices(ctx context.Context, request GetDevicesRequest) (GetDevicesResult, error) {
	cloudReq := dependencymodels.GetDeviceListCloudRequest{
		Method: dependencymodels.MethodGetDeviceList,
	}

	respBytes, err := client.doCloudRequest(ctx, "GetDevices", cloudReq, &request.Auth)
	if err != nil {
		return GetDevicesResult{}, err
	}

	err = cloud.CheckError(respBytes, "GetDevices")
	if err != nil {
		return GetDevicesResult{}, fmt.Errorf("%w", err)
	}

	var devResp dependencymodels.DeviceListResponse

	decodeErr := json.Unmarshal(respBytes, &devResp)
	if decodeErr != nil {
		return GetDevicesResult{}, tplinkmodels.NewInvalidResponseError(
			"GetDevices",
			"failed to parse device list response",
			decodeErr,
		)
	}

	var devices []tplinkmodels.Device

	if devResp.Result != nil && devResp.Result.DeviceList != nil {
		wireDevices := *devResp.Result.DeviceList

		devices = make([]tplinkmodels.Device, len(wireDevices))

		for index, device := range wireDevices {
			devices[index] = tplinkmodels.Device{
				DeviceType:      valueOrZero(device.DeviceType),
				DeviceID:        valueOrZero(device.DeviceId),
				Alias:           valueOrZero(device.Alias),
				DeviceModel:     valueOrZero(device.DeviceModel),
				DeviceMac:       valueOrZero(device.DeviceMac),
				FwVer:           valueOrZero(device.FwVer),
				DeviceHwVer:     valueOrZero(device.DeviceHwVer),
				HwID:            valueOrZero(device.HwId),
				FwID:            valueOrZero(device.FwId),
				OemID:           valueOrZero(device.OemId),
				AppServerURL:    valueOrZero(device.AppServerUrl),
				DeviceRegion:    valueOrZero(device.DeviceRegion),
				Status:          valueOrZero(device.Status),
				IsSameRegion:    valueOrZero(device.IsSameRegion),
				IsDimmable:      valueOrZero(device.IsDimmable),
				IsColor:         valueOrZero(device.IsColor),
				IsVariableColor: valueOrZero(device.IsVariableColorTemp),
				Brightness:      valueOrZero(device.Brightness),
			}
		}
	}

	return GetDevicesResult{Devices: devices}, nil
}
