# TP-Link wire model inventory

This inventory maps each cloud request and response component in
[`api/openapi.yaml`](../api/openapi.yaml) to its generated Go model and the
client code that uses it. The contract describes shapes implemented by this
client and exercised with synthetic replay fixtures; it is not vendor-issued
and has not been verified against live captures.

## Generation and package boundaries

`make generate-api` runs oapi-codegen v2.8.0 for the cloud contract into
[`pkg/dependencymodels`](../pkg/dependencymodels/config.yaml), generates the
public projection from [`api/client-models.openapi.yaml`](../api/client-models.openapi.yaml)
into `pkg/tplinkmodels`, and runs `tools/wireconstants`. The latter emits
schema-derived provider constants and the compatibility aliases in
`pkg/generatedwire/compat.gen.go`. CI regenerates all outputs and rejects
drift.

The reusable client lives in `pkg/tplink`. HTTP construction, response limits,
token redaction, and cloud error mapping live in `pkg/dependencies/cloud`.
`pkg/tplinkmodels` contains public request/result projections and semantic
device classifications; it does not define cloud envelope fields.

## Endpoint and request models

| Schema component or primitive | Generated definition | Actual SDK use |
| --- | --- | --- |
| Servers: US, EU, Asia-Pacific, global | `BaseURLUS`, `BaseURLEU`, `BaseURLAsiaPacific`, `BaseURLGlobal`; `DefaultBaseURL` in `wire_constants.gen.go` | `NewClient` default and exported compatibility constants in `pkg/tplink/constants.gen.go` |
| `POST /` | `CloudRequestHTTPMethod`, `CloudRequestPath` | `pkg/dependencies/cloud.Send` |
| `application/json`, `Content-Type` | `CloudRequestContentType`, `CloudRequestContentTypeHeader` | `pkg/dependencies/cloud.Send` request header |
| Query parameter `token` | `TokenQueryKey` | `pkg/dependencies/cloud.Send` and URL redaction |
| `CloudRequest` | Discriminated generated union with login, device-list, and passthrough variants | Compatibility surface; SDK builders use concrete request models below |
| `LoginCloudRequest`, `LoginCloudRequestMethod` | Login envelope and generated method enum | `pkg/tplink/auth.go:Login` |
| `LoginParams`, `LoginParamsAppType`, `LoginParamsTerminalUUID` | `AppType`, `TerminalUUID` schema constants plus generated model enums | `pkg/tplink/auth.go:Login` |
| `GetDeviceListCloudRequest`, `GetDeviceListCloudRequestMethod` | Device-list envelope and generated method enum | `pkg/tplink/client_devices.go:GetDevices` |
| `PassthroughCloudRequest`, `PassthroughCloudRequestMethod`, `PassthroughParams` | Passthrough envelope and device/request-data fields | `pkg/tplink/client.go:doPassthrough` |
| `PassthroughCommand` | Generated union of the six known command shapes | Schema documentation for the nested JSON string; builders use concrete types below |
| `SystemGetSysInfoCommand`, `SystemGetSysInfoCommandSystemGetSysinfo` | Typed `system.get_sysinfo` request | `pkg/tplink/client_power.go:GetPowerState` |
| `SystemSetRelayStateCommand`, `SystemSetRelayStateCommandSystemSetRelayStateState` | Typed `system.set_relay_state` request and 0/1 enum | `pkg/tplink/client_power.go:TurnOn` and `TurnOff` |
| `SystemRebootCommand`, `SystemRebootCommandSystemRebootDelay` | Typed `system.reboot` request and one-second enum | `pkg/tplink/client_power.go:Reboot` |
| `SystemSetDevAliasCommand` | Typed `system.set_dev_alias` request | `pkg/tplink/client_manage.go:SetAlias` |
| `LightingGetLightStateCommand`, `LightingGetLightStateCommandSmartlifeIotSmartbulbLightingserviceGetLightState` | Typed light-state read request | `pkg/tplink/client_lighting.go:GetLightState` |
| `LightingTransitionLightStateCommand` | Typed transition request; the required nested state is nullable and serializes `nil` as JSON `null` | `pkg/tplink/client_lighting.go:lightingTransitionCommand` and `SetLightState` |
| `LightTransitionState`, `LightTransitionStateOnOff` | Known fields plus generated `AdditionalProperties` for the explicitly caller-open state object | `SetLightState`, `SetBrightness`, `SetColorTemp`, and `SetColor` |

`MethodLogin`, `MethodGetDeviceList`, `MethodPassthrough`, `AppType`, command
namespace names, and command property names are generated from the relevant
enum or property metadata in the schema. The old exported `pkg/tplink` names
remain untyped string constants generated as aliases to schema-derived values.

## Response and nested command models

| Schema component | Generated definition | Actual SDK use |
| --- | --- | --- |
| `CloudResponseEnvelope` | Generated response union | Preserves the historical generated model API; current operations decode their concrete response types |
| `CloudResponse`, `CloudErrorResponse` | Cloud error fields and known extensible error codes | `pkg/dependencies/cloud.CheckError`; maps known codes to typed SDK errors and keeps unknown non-zero codes |
| `LoginResponse`, `LoginResult` | Login result/token/account fields | `pkg/tplink/auth.go:Login` projects the result to `tplinkmodels.LoginResult` |
| `DeviceListResponse`, `DeviceListResult`, `Device` | Device list and provider device metadata | `pkg/tplink/client_devices.go:GetDevices` projects records and preserves unknown types |
| `PassthroughResponse`, `PassthroughResult` | Nested `responseData` JSON string | `pkg/tplink/client.go:doPassthrough` |
| `PassthroughCommandResult` | Generated union of system results, lighting results, and top-level device errors | `pkg/tplink/client_power.go:passthroughCommandResult` |
| `SystemCommandResult`, `SystemCommandResult_System` | Known system response commands plus response-side extensibility | Power reads and command acknowledgement selection in `client_power.go` |
| `LightingCommandResult`, `LightingCommandResult_SmartlifeIotSmartbulbLightingservice` | Known lighting response commands plus response-side extensibility | Light reads and acknowledgements in `client_lighting.go` and `client_power.go` |
| `CommandAcknowledgement` | `err_code`, optional message, and extensible device fields | Turn, reboot, alias, and transition acknowledgement checks |
| `DeviceCommandError` | Top-level device error response | `pkg/tplink/client.go:checkDeviceError` |
| `SysInfo` | System power state and status fields | `pkg/tplink/client_power.go:GetPowerState` |
| `LightState` | Bulb state fields | `pkg/tplink/client_lighting.go:GetLightState` |

## Complete OpenAPI component field map

The following table records every component under `components.schemas` in the
wire schema, including the leaf keys within each known nested command. Fields
marked extensible also preserve provider additions; only
`LightTransitionState` is an open caller-input object.

| Component | Declared fields or variants |
| --- | --- |
| `CloudRequest` | Discriminator `method`; `LoginCloudRequest`, `GetDeviceListCloudRequest`, or `PassthroughCloudRequest` |
| `LoginCloudRequest` | `method=login`, `params: LoginParams` |
| `GetDeviceListCloudRequest` | `method=getDeviceList`; no `params` |
| `PassthroughCloudRequest` | `method=passthrough`, `params: PassthroughParams` |
| `LoginParams` | `appType`, `cloudUserName`, `cloudPassword`, `terminalUUID` |
| `PassthroughParams` | `deviceId`, `requestData` (JSON encoded as a string) |
| `PassthroughCommand` | `SystemGetSysInfoCommand`, `SystemSetRelayStateCommand`, `SystemRebootCommand`, `SystemSetDevAliasCommand`, `LightingGetLightStateCommand`, or `LightingTransitionLightStateCommand` |
| `SystemGetSysInfoCommand` | `system.get_sysinfo`, whose value is the empty string |
| `SystemSetRelayStateCommand` | `system.set_relay_state.state` (`0` or `1`) |
| `SystemRebootCommand` | `system.reboot.delay` (`1`) |
| `SystemSetDevAliasCommand` | `system.set_dev_alias.alias` |
| `LightingGetLightStateCommand` | `smartlife.iot.smartbulb.lightingservice.get_light_state`, whose value is the empty string |
| `LightingTransitionLightStateCommand` | `smartlife.iot.smartbulb.lightingservice.transition_light_state` (`LightTransitionState` or `null`) |
| `LightTransitionState` | `on_off` (`0` or `1`), `brightness` (`0`–`100`), `color_temp`, `hue` (`0`–`360`), `saturation` (`0`–`100`), plus caller-open additional fields |
| `CloudResponseEnvelope` | Union of `LoginResponse`, `DeviceListResponse`, `PassthroughResponse`, and `CloudErrorResponse` |
| `CloudResponse` | `error_code` (`0`, `-20004`, `-20651`, `-20104`, `-20601`, or an extensible provider value), optional `msg` |
| `CloudErrorResponse` | Nonzero `error_code`, optional `msg` |
| `LoginResponse` | `error_code`, `msg`, `result: LoginResult` |
| `LoginResult` | `accountId`, `token`, `email`, `regTime`, `countryCode` |
| `DeviceListResponse` | `error_code`, `msg`, `result: DeviceListResult` |
| `DeviceListResult` | `deviceList: []Device` |
| `Device` | `deviceType`, `deviceId`, `alias`, `deviceModel`, `deviceMac`, `fwVer`, `deviceHwVer`, `hwId`, `fwId`, `oemId`, `appServerUrl`, `deviceRegion`, `status`, `isSameRegion`, `is_dimmable`, `is_color`, `is_variable_color_temp`, `brightness`, and extensible response properties |
| `PassthroughResponse` | `error_code`, `msg`, `result: PassthroughResult` |
| `PassthroughResult` | `responseData` (JSON encoded as a string) |
| `PassthroughCommandResult` | Union of `SystemCommandResult`, `LightingCommandResult`, or `DeviceCommandError` |
| `SystemCommandResult` | `system.get_sysinfo`, `system.set_relay_state`, `system.reboot`, `system.set_dev_alias`, plus extensible response properties at both levels |
| `LightingCommandResult` | `smartlife.iot.smartbulb.lightingservice.get_light_state`, `smartlife.iot.smartbulb.lightingservice.transition_light_state`, plus extensible response properties at both levels |
| `CommandAcknowledgement` | `err_code` (`0`, `-1`, or extensible provider value), optional `err_msg`, extensible response properties |
| `DeviceCommandError` | Required `err_code` (`-1` known, nonzero values allowed), optional `err_msg`, extensible response properties |
| `SysInfo` | `sw_ver`, `hw_ver`, `model`, `deviceId`, `alias`, `relay_state` (`0` or `1`), `on_time`, `err_code`, extensible response properties |
| `LightState` | `on_off`, `brightness`, `hue`, `saturation`, `color_temp`, `mode`, `err_code`, extensible response properties |

Known cloud error codes, device error values, relay states, capability flags,
and device-type strings are emitted in `pkg/dependencymodels/wire_constants.gen.go`
and referenced at the code paths that interpret them. All request/response
fields and nested command keys are also present in generated JSON struct tags
or generated union decoding code; handwritten client code does not build or
index provider command maps.

## Public projection models

`api/client-models.openapi.yaml` generates caller-facing inputs and outputs in
`pkg/tplinkmodels/models.gen.go`: `AuthContext`, `LoginRequest`, the device,
power, lighting, and alias request/result types, `Device`, `LoginResult`,
`EndpointType`, `Capabilities`, `PowerState`, `SysInfo`, and `LightState`.
`pkg/tplinkmodels/devices.go` contains behavior helpers that use generated
provider device-type and capability constants. This schema is intentionally
separate from cloud request and response envelopes.

## Inventory gates and verification

`tools/wireconstants` validates required schema components, extensible-enum
metadata, known nested payload names, the caller-open transition-state rule,
and conflicting constant definitions before generating outputs. Its tests
include invalid enum metadata and missing required payload/constants.

`tools/wireinventory` runs in `make check` and CI. It checks that each required
schema-derived constant is used, rejects handwritten map types/construction
and raw schema-defined string values in the SDK transport/model packages, and
rejects writes or deletions through direct and aliased `SetLightState` input.
Negative controls cover nested map literals, map aliases, `make`/`new`,
parenthesized constructions, caller-map aliases, request aliases, closures,
`delete`, `clear`, and raw wire strings. Forwarding the caller's map into the
generated `LightTransitionState.AdditionalProperties` remains allowed.

Offline paired replay tests assert the exact outer request and decoded nested
command JSON, response handling, error variants, and complete fixture
consumption in `tests/replay`. Run `make generate-api`, `go test ./...`,
`go run ./tools/wireinventory`, `make lint`, and `make check` when changing the
contract or its client use.
