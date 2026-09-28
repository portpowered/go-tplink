// Package tplinkmodels provides data models and error types for the TP-Link Cloud API client.
//
// This package is separate from the client so callers can use provider models
// and inspect typed errors without depending on HTTP client construction.
package tplinkmodels

// --- Cloud API envelope ---

// CloudResponse represents the standard TP-Link Cloud API response envelope.
// All API responses include an error_code field; 0 indicates success.
type CloudResponse struct {
	ErrorCode int    `json:"error_code"`
	Msg       string `json:"msg,omitempty"`
}

// LoginResult contains the fields returned after successful authentication.
type LoginResult struct {
	AccountID   string `json:"accountId"`
	Token       string `json:"token"`
	Email       string `json:"email"`
	RegTime     string `json:"regTime"`
	CountryCode string `json:"countryCode"`
}

// LoginResponse is the cloud API response for the login method.
type LoginResponse struct {
	CloudResponse
	Result LoginResult `json:"result"`
}

// --- Device models ---

// Device represents a device from the getDeviceList response.
type Device struct {
	DeviceType      string `json:"deviceType"`
	DeviceID        string `json:"deviceId"`
	Alias           string `json:"alias"`
	DeviceModel     string `json:"deviceModel"`
	DeviceMac       string `json:"deviceMac"`
	FwVer           string `json:"fwVer"`
	DeviceHwVer     string `json:"deviceHwVer"`
	HwID            string `json:"hwId"`
	FwID            string `json:"fwId"`
	OemID           string `json:"oemId"`
	AppServerURL    string `json:"appServerUrl"`
	DeviceRegion    string `json:"deviceRegion"`
	Status          int    `json:"status"`
	IsSameRegion    bool   `json:"isSameRegion"`
	IsDimmable      int    `json:"is_dimmable,omitempty"`
	IsColor         int    `json:"is_color,omitempty"`
	IsVariableColor int    `json:"is_variable_color_temp,omitempty"`
	Brightness      int    `json:"brightness,omitempty"`
}

// DeviceListResult wraps the device list in the cloud response.
type DeviceListResult struct {
	DeviceList []Device `json:"deviceList"`
}

// DeviceListResponse is the cloud API response for getDeviceList.
type DeviceListResponse struct {
	CloudResponse
	Result DeviceListResult `json:"result"`
}

// --- Endpoint type classification ---

// EndpointType classifies a device based on its deviceType field.
type EndpointType string

const (
	EndpointTypePlug  EndpointType = "Plug"
	EndpointTypeBulb  EndpointType = "Bulb"
	EndpointTypeOther EndpointType = "Other"
)

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

// --- Capability detection ---

// Capabilities describes what a device can do, inferred from device metadata.
type Capabilities struct {
	OnOff      bool
	Brightness bool
	Color      bool
	ColorTemp  bool
}

// DetectCapabilities infers device capabilities from metadata fields.
func DetectCapabilities(d Device) Capabilities {
	t := ClassifyDevice(d.DeviceType)
	caps := Capabilities{OnOff: t == EndpointTypePlug || t == EndpointTypeBulb}

	switch t {
	case EndpointTypePlug:
		// Plugs with a brightness field (e.g., HS220 dimmer) support brightness
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

// --- Passthrough models ---

// PassthroughResult wraps the responseData string from passthrough calls.
type PassthroughResult struct {
	ResponseData string `json:"responseData"`
}

// PassthroughResponse is the cloud API response for passthrough method.
type PassthroughResponse struct {
	CloudResponse
	Result PassthroughResult `json:"result"`
}

// --- Power state (from get_sysinfo) ---

// SysInfo represents the parsed system info from a plug's get_sysinfo response.
type SysInfo struct {
	SwVer      string `json:"sw_ver"`
	HwVer      string `json:"hw_ver"`
	Model      string `json:"model"`
	DeviceID   string `json:"deviceId"`
	Alias      string `json:"alias"`
	RelayState int    `json:"relay_state"`
	OnTime     int    `json:"on_time"`
	ErrCode    int    `json:"err_code"`
}

// PowerState represents the power state of a plug device.
type PowerState struct {
	IsOn   bool
	OnTime int
}

// --- Light state (from get_light_state) ---

// LightState represents the current state of a smart bulb.
type LightState struct {
	OnOff      int    `json:"on_off"`
	Brightness int    `json:"brightness"`
	Hue        int    `json:"hue"`
	Saturation int    `json:"saturation"`
	ColorTemp  int    `json:"color_temp"`
	Mode       string `json:"mode"`
	ErrCode    int    `json:"err_code"`
}

// --- Cloud API request types ---

// LoginParams holds the parameters for the login method.
type LoginParams struct {
	AppType       string `json:"appType"`
	CloudUserName string `json:"cloudUserName"`
	CloudPassword string `json:"cloudPassword"`
	TerminalUUID  string `json:"terminalUUID"`
}

// CloudRequest is the generic request envelope for the TP-Link Cloud API.
type CloudRequest struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// PassthroughParams holds the parameters for the passthrough method.
type PassthroughParams struct {
	DeviceID    string `json:"deviceId"`
	RequestData string `json:"requestData"`
}
