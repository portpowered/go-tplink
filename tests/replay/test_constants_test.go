package replay_test

const (
	replayTestToken         = "test-token"
	replayTestEmail         = "user@example.com"
	replayTestPassword      = "placeholder-password"
	replayPlugDeviceID      = "device-plug-001"
	replayBulbDeviceID      = "device-bulb-001"
	replayLoginOperation    = "login"
	replayDeviceListMethod  = "getDeviceList"
	replayOperationPower    = "passthrough_system_get_sysinfo"
	replayOperationReboot   = "passthrough_system_reboot"
	replayOperationAlias    = "passthrough_system_set_dev_alias"
	replayOperationRelay    = "passthrough_system_set_relay_state"
	replayOperationLightGet = "passthrough_lightingservice_get_light_state"
	replayOperationLightSet = "passthrough_lightingservice_transition_light_state"
	replayEmptyResult       = `{"error_code":0,"result":[]}`
	replayBrightnessField   = "brightness"
	replayHueField          = "hue"
	replaySaturationField   = "saturation"
	replayLightStateCommand = "smartlife.iot.smartbulb.lightingservice.get_light_state"
	replaySetAliasName      = "set alias"
)
