# TP-Link cloud protocol notes

These notes describe behavior encoded by the client implementation. They are
not an official TP-Link protocol specification. The endpoint and command
details below are implementation observations covered by synthetic tests,
not vendor-guaranteed behavior.

## Cloud endpoint and envelope

The client sends HTTPS `POST` requests to a configured regional
base URL. The default was `https://use1-wap.tplinkcloud.com`; constants also
named `https://eu-wap.tplinkcloud.com`,
`https://aps1-wap.tplinkcloud.com`, and `https://wap.tplinkcloud.com`.
`WithBaseURL` let callers choose another endpoint. No pagination cursor or
regional redirect workflow was present in the wrapper.

The request envelope was:

```json
{
  "method": "getDeviceList"
}
```

Methods observed in the wrapper are `login`, `getDeviceList`, and `passthrough`.
Successful cloud responses have `error_code: 0` and operation data in `result`.
Error responses use `error_code` and may include `msg`.

## Authentication

The login request used the `login` method and these fields in `params`:

```json
{
  "appType": "Tapo_Android",
  "cloudUserName": "<email>",
  "cloudPassword": "<password>",
  "terminalUUID": "go-tplink-client"
}
```

The `Tapo_Android` string is the login request's `appType` value; it is not
evidence that this client supports Tapo product APIs or local Tapo protocols.

The result model includes `token`, `accountId`, `email`, `regTime`, and
`countryCode`. Authenticated requests put the session token in the `token`
query parameter. The wrapper does not implement refresh or token-expiry
discovery. On an expired-token response, callers need to authenticate again.

Because the token is part of the URL, request URLs can contain credentials.
Transport, tracing, and error logging should redact the `token` query value.
The library never needs the user's password after the login operation.

## Device listing

The `getDeviceList` operation has no `params` in the client implementation.
The result wraps devices in `result.deviceList`. Device fields used by the
client include `deviceId`, `deviceType`, `alias`, `deviceModel`,
`appServerUrl`, `status`, and lighting capability flags (`is_dimmable`,
`is_color`, and `is_variable_color_temp`). The Go model preserves additional
firmware and hardware fields.

The model helpers recognize `IOT.SMARTPLUGSWITCH`,
`IOT.RANGEEXTENDER.SMARTPLUG`, and `IOT.SMARTBULB`. Unknown types are
classified as `Other`; the client returns the device list without filtering.

## Device passthrough

The legacy wrapper sent a device command inside a JSON string in
`params.requestData`:

```json
{
  "method": "passthrough",
  "params": {
    "deviceId": "<device-id>",
    "requestData": "{\"system\":{\"set_relay_state\":{\"state\":1}}}"
  }
}
```

The cloud result contains `result.responseData`, also a JSON string. Plug
commands use the `system` namespace: `get_sysinfo`, `set_relay_state`, `reboot`,
and `set_dev_alias`. Bulb operations use
`smartlife.iot.smartbulb.lightingservice`: `get_light_state` and
`transition_light_state`. The transition parameters used by the old client
were `brightness` (0–100), `color_temp` in Kelvin, and `hue` (0–360) with
`saturation` (0–100). Device support and value ranges still depend on the
model; an advertised capability does not prove every command works on every
firmware revision.

The implementation has no extra signing or application-layer encryption
protocol. It sends the request over HTTPS by default; use the injected HTTP
dependency to control TLS, proxying, and request instrumentation. There is no
topic, event stream, continuation token, or second device-side transport in
the client surface described here.

## Errors and behavior gaps

The legacy implementation mapped these cloud error codes:

| Cloud code | Legacy error category |
| --- | --- |
| `-20004` | Rate limited |
| `-20651` | Session token expired |
| `-20104` | Invalid parameter |
| `-20601` | Authentication rejected |
| other non-zero codes | Generic cloud API error |

It treated device `err_code: -1` as an unsupported operation and other
non-zero device codes as device errors. Device error data may be at the top
level or nested under a namespace and command. The current Go API keeps typed
errors in `pkg/tplinkmodels`; see their source for exact exported fields and
inspect with `errors.As` when handling them.

An earlier implementation parsed the response body without branching on HTTP
status codes. The current client classifies HTTP and malformed-response
failures explicitly. There is no documented retry, refresh, pagination, push/event,
websocket, or long-lived-session protocol in the supported client surface.

## Protocol artifact status

This document is a human-readable protocol note, not OpenAPI or a vendor-issued
schema. No formal vendor schema is included with the client, and the checked-in
JSON fixtures have no verifiable capture provenance. They are maintained as
synthetic test inputs; they are not evidence of a live exchange. See
[fixture and verification notes](fixtures-and-testing.md).
