# TP-Link wire model inventory

This maintainer inventory maps the route, each provider wire component, each
public projection, and the CLI's local JSON types to generated declarations
and their uses. The provider contract describes behavior implemented by this
client and exercised with synthetic replay fixtures; it is not vendor-issued
and has not been verified against live captures.

## Generation and package boundaries

`make generate-api` runs oapi-codegen v2.8.0 for the route, cloud envelope,
authentication, device, and passthrough contracts. It separately generates
the public projection in `pkg/tplinkmodels` and the CLI's local JSON DTOs in
`cmd/go-tplink`. `tools/wireconstants` emits schema-derived provider values,
legacy constants, and type aliases in `pkg/generatedwire/compat.gen.go`. CI
regenerates every registered output and rejects drift or untracked generated
files.

The reusable client lives in `pkg/tplink`. HTTP construction, response limits,
token redaction, and cloud error mapping live in `pkg/dependencies/cloud`.
`pkg/tplinkmodels` contains public request/result projections and semantic
device classifications; it does not define cloud envelope fields.

The generator commands and call sites for every schema component are listed
in the complete component table below. CLI contracts in that table are local
command inputs and outputs, not provider payloads.

## Complete schema-to-Go model inventory

Every concrete component has a generated declaration and a package-resolved use. Legacy components with no current builder are marked as compatibility-only. CLI models are local JSON contracts and are not provider wire models.

| Schema component | Generated Go type | Generator output | Generator command | Actual call site or compatibility role |
| --- | --- | --- | --- | --- |
| `api/cloud-envelope.openapi.yaml#CloudRequest` | `pkg/dependencymodels.CloudRequest` | `pkg/dependencymodels/cloud_envelope.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config.yaml api/cloud-envelope.openapi.yaml` | generated compatibility type; builders use concrete request variants |
| `api/cloud-envelope.openapi.yaml#CloudResponseEnvelope` | `pkg/dependencymodels.CloudResponseEnvelope` | `pkg/dependencymodels/cloud_envelope.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config.yaml api/cloud-envelope.openapi.yaml` | generated compatibility type; request-specific response types are decoded by pkg/tplink/auth.go, client_devices.go, and client.go |
| `api/cloud-envelope.openapi.yaml#CloudResponse` | `pkg/dependencymodels.CloudResponse` | `pkg/dependencymodels/cloud_envelope.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config.yaml api/cloud-envelope.openapi.yaml` | pkg/dependencies/cloud/cloud.go:CheckError |
| `api/cloud-envelope.openapi.yaml#CloudErrorResponse` | `pkg/dependencymodels.CloudErrorResponse` | `pkg/dependencymodels/cloud_envelope.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config.yaml api/cloud-envelope.openapi.yaml` | schema-compatible legacy response variant; pkg/dependencies/cloud/cloud.go decodes its shared fields through CloudResponse |
| `api/authentication.openapi.yaml#LoginCloudRequest` | `pkg/dependencymodels.LoginCloudRequest` | `pkg/dependencymodels/authentication.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-authentication.yaml api/authentication.openapi.yaml` | pkg/tplink/auth.go:Login |
| `api/authentication.openapi.yaml#LoginParams` | `pkg/dependencymodels.LoginParams` | `pkg/dependencymodels/authentication.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-authentication.yaml api/authentication.openapi.yaml` | pkg/tplink/auth.go:Login |
| `api/authentication.openapi.yaml#LoginResponse` | `pkg/dependencymodels.LoginResponse` | `pkg/dependencymodels/authentication.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-authentication.yaml api/authentication.openapi.yaml` | pkg/tplink/auth.go:Login response decode |
| `api/authentication.openapi.yaml#LoginResult` | `pkg/dependencymodels.LoginResult` | `pkg/dependencymodels/authentication.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-authentication.yaml api/authentication.openapi.yaml` | pkg/tplink/auth.go:Login response projection |
| `api/devices.openapi.yaml#GetDeviceListCloudRequest` | `pkg/dependencymodels.GetDeviceListCloudRequest` | `pkg/dependencymodels/devices.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-devices.yaml api/devices.openapi.yaml` | pkg/tplink/client_devices.go:GetDevices |
| `api/devices.openapi.yaml#DeviceListResponse` | `pkg/dependencymodels.DeviceListResponse` | `pkg/dependencymodels/devices.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-devices.yaml api/devices.openapi.yaml` | pkg/tplink/client_devices.go:GetDevices response decode |
| `api/devices.openapi.yaml#DeviceListResult` | `pkg/dependencymodels.DeviceListResult` | `pkg/dependencymodels/devices.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-devices.yaml api/devices.openapi.yaml` | pkg/tplink/client_devices.go:GetDevices response projection |
| `api/devices.openapi.yaml#Device` | `pkg/dependencymodels.Device` | `pkg/dependencymodels/devices.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-devices.yaml api/devices.openapi.yaml` | pkg/tplink/client_devices.go:GetDevices |
| `api/passthrough.openapi.yaml#PassthroughCloudRequest` | `pkg/dependencymodels.PassthroughCloudRequest` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client.go:doPassthrough |
| `api/passthrough.openapi.yaml#PassthroughParams` | `pkg/dependencymodels.PassthroughParams` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client.go:doPassthrough |
| `api/passthrough.openapi.yaml#PassthroughCommand` | `pkg/dependencymodels.PassthroughCommand` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | schema union for JSON inside requestData; builders use the concrete command DTOs below |
| `api/passthrough.openapi.yaml#SystemGetSysInfoCommand` | `pkg/dependencymodels.SystemGetSysInfoCommand` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_power.go:GetPowerState |
| `api/passthrough.openapi.yaml#SystemSetRelayStateCommand` | `pkg/dependencymodels.SystemSetRelayStateCommand` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_power.go:TurnOn and TurnOff |
| `api/passthrough.openapi.yaml#SystemRebootCommand` | `pkg/dependencymodels.SystemRebootCommand` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_power.go:Reboot |
| `api/passthrough.openapi.yaml#SystemSetDevAliasCommand` | `pkg/dependencymodels.SystemSetDevAliasCommand` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_manage.go:SetAlias |
| `api/passthrough.openapi.yaml#LightingGetLightStateCommand` | `pkg/dependencymodels.LightingGetLightStateCommand` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_lighting.go:GetLightState |
| `api/passthrough.openapi.yaml#LightingTransitionLightStateCommand` | `pkg/dependencymodels.LightingTransitionLightStateCommand` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_lighting.go:lightingTransitionCommand |
| `api/passthrough.openapi.yaml#LightTransitionState` | `pkg/dependencymodels.LightTransitionState` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_lighting.go:SetLightState and typed light controls |
| `api/passthrough.openapi.yaml#PassthroughResponse` | `pkg/dependencymodels.PassthroughResponse` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client.go:doPassthrough response decode |
| `api/passthrough.openapi.yaml#PassthroughResult` | `pkg/dependencymodels.PassthroughResult` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client.go:doPassthrough response projection |
| `api/passthrough.openapi.yaml#PassthroughCommandResult` | `pkg/dependencymodels.PassthroughCommandResult` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_power.go:passthroughCommandResult |
| `api/passthrough.openapi.yaml#SystemCommandResult` | `pkg/dependencymodels.SystemCommandResult` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_power.go:systemCommandResult |
| `api/passthrough.openapi.yaml#LightingCommandResult` | `pkg/dependencymodels.LightingCommandResult` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_lighting.go:GetLightState and client_power.go:lightingCommandResult |
| `api/passthrough.openapi.yaml#CommandAcknowledgement` | `pkg/dependencymodels.CommandAcknowledgement` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_power.go:checkPassthroughCommandError |
| `api/passthrough.openapi.yaml#DeviceCommandError` | `pkg/dependencymodels.DeviceCommandError` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client.go:checkDeviceError and passthrough result decoding |
| `api/passthrough.openapi.yaml#SysInfo` | `pkg/dependencymodels.SysInfo` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_power.go:GetPowerState projection |
| `api/passthrough.openapi.yaml#LightState` | `pkg/dependencymodels.LightState` | `pkg/dependencymodels/passthrough.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-passthrough.yaml api/passthrough.openapi.yaml` | pkg/tplink/client_lighting.go:GetLightState projection |
| `api/client-models.openapi.yaml#AuthContext` | `pkg/tplinkmodels.AuthContext` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/requests.go alias; supplied to authenticated client methods |
| `api/client-models.openapi.yaml#LoginRequest` | `pkg/tplinkmodels.LoginRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/auth.go:Login |
| `api/client-models.openapi.yaml#GetDevicesRequest` | `pkg/tplinkmodels.GetDevicesRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_devices.go:GetDevices |
| `api/client-models.openapi.yaml#GetDevicesResult` | `pkg/tplinkmodels.GetDevicesResult` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_devices.go:GetDevices result |
| `api/client-models.openapi.yaml#TurnOnRequest` | `pkg/tplinkmodels.TurnOnRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_power.go:TurnOn |
| `api/client-models.openapi.yaml#TurnOffRequest` | `pkg/tplinkmodels.TurnOffRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_power.go:TurnOff |
| `api/client-models.openapi.yaml#GetPowerStateRequest` | `pkg/tplinkmodels.GetPowerStateRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_power.go:GetPowerState |
| `api/client-models.openapi.yaml#RebootRequest` | `pkg/tplinkmodels.RebootRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_power.go:Reboot |
| `api/client-models.openapi.yaml#SetBrightnessRequest` | `pkg/tplinkmodels.SetBrightnessRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:SetBrightness |
| `api/client-models.openapi.yaml#SetColorTempRequest` | `pkg/tplinkmodels.SetColorTempRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:SetColorTemp |
| `api/client-models.openapi.yaml#SetColorRequest` | `pkg/tplinkmodels.SetColorRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:SetColor |
| `api/client-models.openapi.yaml#SetLightStateRequest` | `pkg/tplinkmodels.SetLightStateRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:SetLightState; State is the schema-declared caller-open object |
| `api/client-models.openapi.yaml#GetLightStateRequest` | `pkg/tplinkmodels.GetLightStateRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:GetLightState |
| `api/client-models.openapi.yaml#GetBrightnessRequest` | `pkg/tplinkmodels.GetBrightnessRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:GetBrightness |
| `api/client-models.openapi.yaml#BrightnessResult` | `pkg/tplinkmodels.BrightnessResult` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:GetBrightness |
| `api/client-models.openapi.yaml#GetColorTempRequest` | `pkg/tplinkmodels.GetColorTempRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:GetColorTemp |
| `api/client-models.openapi.yaml#ColorTempResult` | `pkg/tplinkmodels.ColorTempResult` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:GetColorTemp |
| `api/client-models.openapi.yaml#GetColorRequest` | `pkg/tplinkmodels.GetColorRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:GetColor |
| `api/client-models.openapi.yaml#ColorResult` | `pkg/tplinkmodels.ColorResult` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:GetColor |
| `api/client-models.openapi.yaml#SetAliasRequest` | `pkg/tplinkmodels.SetAliasRequest` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_manage.go:SetAlias |
| `api/client-models.openapi.yaml#LoginResult` | `pkg/tplinkmodels.LoginResult` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/auth.go:Login projection |
| `api/client-models.openapi.yaml#Device` | `pkg/tplinkmodels.Device` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_devices.go:GetDevices projection |
| `api/client-models.openapi.yaml#EndpointType` | `pkg/tplinkmodels.EndpointType` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplinkmodels/devices.go:ClassifyDevice |
| `api/client-models.openapi.yaml#Capabilities` | `pkg/tplinkmodels.Capabilities` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplinkmodels/devices.go:DeviceCapabilities |
| `api/client-models.openapi.yaml#PowerState` | `pkg/tplinkmodels.PowerState` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_power.go:GetPowerState projection |
| `api/client-models.openapi.yaml#SysInfo` | `pkg/tplinkmodels.SysInfo` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | public generated projection; fields derive from provider SysInfo |
| `api/client-models.openapi.yaml#LightState` | `pkg/tplinkmodels.LightState` | `pkg/tplinkmodels/models.gen.go` | `oapi-codegen v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml` | pkg/tplink/client_lighting.go:GetLightState projection |
| `api/cli-contracts.openapi.yaml#LoginCredentials` | `main.LoginCredentials` | `cmd/go-tplink/models.gen.go` | `oapi-codegen v2.8.0 -config cmd/go-tplink/config.yaml api/cli-contracts.openapi.yaml` | cmd/go-tplink/credentials.go:readLoginCredentials |
| `api/cli-contracts.openapi.yaml#StoredCredentials` | `main.StoredCredentials` | `cmd/go-tplink/models.gen.go` | `oapi-codegen v2.8.0 -config cmd/go-tplink/config.yaml api/cli-contracts.openapi.yaml` | cmd/go-tplink/credentials.go:load and save credentials |
| `api/cli-contracts.openapi.yaml#AuthStatusOutput` | `main.AuthStatusOutput` | `cmd/go-tplink/models.gen.go` | `oapi-codegen v2.8.0 -config cmd/go-tplink/config.yaml api/cli-contracts.openapi.yaml` | cmd/go-tplink/main.go:auth status JSON output |
| `api/cli-contracts.openapi.yaml#DeviceSummary` | `main.DeviceSummary` | `cmd/go-tplink/models.gen.go` | `oapi-codegen v2.8.0 -config cmd/go-tplink/config.yaml api/cli-contracts.openapi.yaml` | cmd/go-tplink/main.go:device discovery output |
| `api/cli-contracts.openapi.yaml#PowerStateOutput` | `main.PowerStateOutput` | `cmd/go-tplink/models.gen.go` | `oapi-codegen v2.8.0 -config cmd/go-tplink/config.yaml api/cli-contracts.openapi.yaml` | cmd/go-tplink/main.go:power read output |
| `api/cli-contracts.openapi.yaml#LightStateOutput` | `main.LightStateOutput` | `cmd/go-tplink/models.gen.go` | `oapi-codegen v2.8.0 -config cmd/go-tplink/config.yaml api/cli-contracts.openapi.yaml` | cmd/go-tplink/main.go:light read output |

| `api/openapi.yaml#/paths/~1` request operation | `pkg/dependencymodels.SendCloudRequestParams` | `pkg/dependencymodels/cloud_request_compat.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-route-compat.yaml api/openapi.yaml` | generated compatibility alias; `pkg/dependencies/cloud/cloud.go:newRequest` sends its JSON body |
| `api/openapi.yaml#/paths/~1` request body | `pkg/dependencymodels.SendCloudRequestJSONRequestBody` | `pkg/dependencymodels/cloud_request_compat.gen.go` | `oapi-codegen v2.8.0 -config pkg/dependencymodels/config-route-compat.yaml api/openapi.yaml` | generated compatibility alias for `CloudRequest`; concrete builders in `pkg/tplink/auth.go`, `client_devices.go`, and `client.go` |

The route-level `POST /` schema lives in `api/openapi.yaml`; compatibility request-parameter aliases are generated from it by `pkg/dependencymodels/config-route-compat.yaml` into `pkg/dependencymodels/cloud_request_compat.gen.go` and re-exported from `pkg/generatedwire/compat.gen.go`. The compatibility-only types `SendCloudRequestParams` and `SendCloudRequestJSONRequestBody` preserve the historical exported API, while active builders use concrete request DTOs.

## HTTP endpoint inventory

| Endpoint and exchange | Schema owner | Generated method/path and metadata | Actual call site |
| --- | --- | --- | --- |
| TP-Link Cloud JSON request/response | `api/openapi.yaml` | `CloudRequestHTTPMethod=POST`, `CloudRequestPath=/`, `CloudRequestContentTypeHeader=Content-Type`, `CloudRequestContentType=application/json`, `TokenQueryKey=token` | `pkg/dependencies/cloud/cloud.go:newRequest`, `requestURL`, and `Send` |

The configured server origins are `BaseURLUS`, `BaseURLEU`, `BaseURLAsiaPacific`,
and `BaseURLGlobal`, generated from the OpenAPI servers. `DefaultBaseURL` selects
the declared default and is used by `NewClient`. The regional origins remain
exported compatibility constants. `WithBaseURL` accepts a caller-configured
authority and optional base path; that path is preserved when building the
request URL. The client does not add an endpoint suffix to a configured base path.

## Primitive and wire-value inventory

| Schema owner and field | Generated primitive or constants | Actual use |
| --- | --- | --- |
| `api/openapi.yaml`: `POST /`, JSON body, `token` query | `CloudRequestHTTPMethod`, `CloudRequestPath`, `CloudRequestContentTypeHeader`, `CloudRequestContentType`, `TokenQueryKey` | `pkg/dependencies/cloud/cloud.go:newRequest` and `requestURL` |
| `api/openapi.yaml`: server origins and default server | `BaseURLUS`, `BaseURLEU`, `BaseURLAsiaPacific`, `BaseURLGlobal`, `DefaultBaseURL` | `pkg/tplink/client.go:NewClient` uses `dependencymodels.DefaultBaseURL`; regional origins are compatibility exports |
| `api/authentication.openapi.yaml`: `LoginCloudRequest.method` | `LoginCloudRequestMethod`, typed `Login`, schema-derived `dependencymodels.MethodLogin`, and compatibility `tplink.MethodLogin` | `pkg/tplink/auth.go:Login` uses `dependencymodels.MethodLogin` |
| `api/authentication.openapi.yaml`: `LoginParams.appType`, `terminalUUID` | `LoginParamsAppType` / `TapoAndroid`, `LoginParamsTerminalUUID` / `GoTplinkClient`, and schema constants `AppType`, `TerminalUUID` | `pkg/tplink/auth.go:Login` |
| `api/devices.openapi.yaml`: `GetDeviceListCloudRequest.method` | `GetDeviceListCloudRequestMethod`, typed `GetDeviceList`, schema-derived `dependencymodels.MethodGetDeviceList`, and compatibility `tplink.MethodGetDeviceList` | `pkg/tplink/client_devices.go:GetDevices` uses `dependencymodels.MethodGetDeviceList` |
| `api/devices.openapi.yaml`: `Device.deviceType` | `DeviceTypeSmartPlug`, `DeviceTypeRangeExtenderPlug`, `DeviceTypeSmartBulb` | `pkg/tplinkmodels/devices.go:ClassifyDevice` |
| `api/devices.openapi.yaml`: capability fields `is_dimmable`, `is_color`, `is_variable_color_temp` | `DeviceCapabilityDisabled=0`, `DeviceCapabilityEnabled=1` | `pkg/tplinkmodels/devices.go:DeviceCapabilities` handles both values for each field |
| `api/passthrough.openapi.yaml`: `PassthroughCloudRequest.method` | `PassthroughCloudRequestMethod`, typed `Passthrough`, schema-derived `dependencymodels.MethodPassthrough`, and compatibility `tplink.MethodPassthrough` | `pkg/tplink/client.go:doPassthrough` uses `dependencymodels.MethodPassthrough` |
| `api/passthrough.openapi.yaml`: nested command namespace and key names | `NamespaceSystem`, `NamespaceLightingService`, `CmdGetSysInfo`, `CmdSetRelayState`, `CmdReboot`, `CmdSetDevAlias`, `CmdGetLightState`, `CmdTransitionLightState` | `pkg/tplink/client_power.go`, `client_manage.go`, `client_lighting.go`, and `pkg/tplink/client.go` response selection |
| `api/passthrough.openapi.yaml`: `system.set_relay_state.state` | `SystemSetRelayStateCommandSystemSetRelayStateState` typed enum constants `N0`/`N1` | Generated API compatibility; active builders use the shared on/off values below |
| `api/passthrough.openapi.yaml`: `SysInfo.relay_state` | `DeviceStateOff=0`, `DeviceStateOn=1` | `pkg/tplink/client_power.go:TurnOff` and `TurnOn` use the matching values; `GetPowerState` returns the decoded integer without mapping it to these constants |
| `api/passthrough.openapi.yaml`: `system.reboot.delay` | `SystemRebootCommandSystemRebootDelay` and its typed `N1` constant | `pkg/tplink/client_power.go:Reboot` |
| `api/passthrough.openapi.yaml`: empty read-command values | `SystemGetSysInfoCommandSystemGetSysinfoEmpty`, `LightingGetLightStateCommandSmartlifeIotSmartbulbLightingserviceGetLightStateEmpty` | `pkg/tplink/client_power.go:GetPowerState` and `pkg/tplink/client_lighting.go:GetLightState` |
| `api/cloud-envelope.openapi.yaml`: `CloudResponse.error_code` | `CloudErrorCodeOK=0`, `CloudErrorCodeRateLimited=-20004`, `CloudErrorCodeTokenExpired=-20651`, `CloudErrorCodeParameter=-20104`, `CloudErrorCodeAuthentication=-20601` | `pkg/dependencies/cloud/cloud.go:CheckError` |
| `api/passthrough.openapi.yaml`: response `err_code` fields | `DeviceErrorOK=0`, `DeviceErrorUnsupported=-1` | `pkg/tplink/client.go:checkDeviceError` and `pkg/tplink/client_power.go:checkPassthroughCommandError` |
| `api/passthrough.openapi.yaml`: `LightTransitionState.on_off` | `LightTransitionStateOnOff` with generated `N0`/`N1` values | Compatibility-only enum; `SetLightState` forwards the caller-open object, and typed brightness, temperature, hue, and saturation builders supply request values |

The typed generated enum constants remain available for callers through
`pkg/generatedwire/compat.gen.go`. Historical untyped `pkg/tplink` constants
are emitted by `tools/wireconstants`; builders use the generated
`dependencymodels` declarations above. The SDK retains unknown device types and
capability values instead of treating them as a closed provider enum.

## Endpoint and request models

| Schema component or primitive | Generated definition | Actual SDK use |
| --- | --- | --- |
| Servers: US, EU, Asia-Pacific, global | `BaseURLUS`, `BaseURLEU`, `BaseURLAsiaPacific`, `BaseURLGlobal`; `DefaultBaseURL` in `wire_constants.gen.go` | `NewClient` uses `dependencymodels.DefaultBaseURL`; regional values are exported compatibility constants |
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
| `SystemSetRelayStateCommand`, `SystemSetRelayStateCommandSystemSetRelayStateState` | Typed `system.set_relay_state` request and generated 0/1 enum | Request DTO is active; the typed enum remains generated compatibility while builders use `DeviceStateOn`/`DeviceStateOff` |
| `SystemRebootCommand`, `SystemRebootCommandSystemRebootDelay` | Typed `system.reboot` request and one-second enum | `pkg/tplink/client_power.go:Reboot` |
| `SystemSetDevAliasCommand` | Typed `system.set_dev_alias` request | `pkg/tplink/client_manage.go:SetAlias` |
| `LightingGetLightStateCommand`, `LightingGetLightStateCommandSmartlifeIotSmartbulbLightingserviceGetLightState` | Typed light-state read request | `pkg/tplink/client_lighting.go:GetLightState` |
| `LightingTransitionLightStateCommand` | Typed transition request; the required nested state is nullable and serializes `nil` as JSON `null` | `pkg/tplink/client_lighting.go:lightingTransitionCommand` and `SetLightState` |
| `LightTransitionState`, `LightTransitionStateOnOff` | Known fields plus generated `AdditionalProperties` for the explicitly caller-open state object | `SetLightState` forwards caller keys; typed light controls set generated DTO fields. The typed `on_off` enum is compatibility-only |

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
