package tplink

// Regional base URLs for the TP-Link Cloud API.
const (
	BaseURLUS          = "https://use1-wap.tplinkcloud.com"
	BaseURLEU          = "https://eu-wap.tplinkcloud.com"
	BaseURLAsiaPacific = "https://aps1-wap.tplinkcloud.com"
	BaseURLGlobal      = "https://wap.tplinkcloud.com"

	// DefaultBaseURL is the US regional endpoint, used when no base URL is specified.
	DefaultBaseURL = BaseURLUS
)

// TP-Link Cloud API method constants (sent in the "method" field of request bodies).
const (
	MethodLogin         = "login"
	MethodGetDeviceList = "getDeviceList"
	MethodPassthrough   = "passthrough"
)

// AppType is the identifier sent during login.
const AppType = "Tapo_Android"

// Passthrough command namespaces.
const (
	NamespaceSystem          = "system"
	NamespaceLightingService = "smartlife.iot.smartbulb.lightingservice"
)

// Passthrough command names.
const (
	CmdSetRelayState        = "set_relay_state"
	CmdGetSysInfo           = "get_sysinfo"
	CmdReboot               = "reboot"
	CmdSetDevAlias          = "set_dev_alias"
	CmdTransitionLightState = "transition_light_state"
	CmdGetLightState        = "get_light_state"
)
