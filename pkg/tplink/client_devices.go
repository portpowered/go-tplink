package tplink

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// GetDevices retrieves all devices registered to the authenticated account.
func (client *Client) GetDevices(ctx context.Context, request GetDevicesRequest) (GetDevicesResult, error) {
	cloudReq := tplinkmodels.CloudRequest{
		Method: MethodGetDeviceList,
	}

	respBytes, err := client.doCloudRequest(ctx, "GetDevices", cloudReq, &request.Auth)
	if err != nil {
		return GetDevicesResult{}, err
	}

	if err := checkCloudError(respBytes, "GetDevices"); err != nil {
		return GetDevicesResult{}, err
	}

	var devResp tplinkmodels.DeviceListResponse
	if err := json.Unmarshal(respBytes, &devResp); err != nil {
		return GetDevicesResult{}, tplinkmodels.NewInvalidResponseError("GetDevices", "failed to parse device list response", err)
	}

	return GetDevicesResult{Devices: devResp.Result.DeviceList}, nil
}
