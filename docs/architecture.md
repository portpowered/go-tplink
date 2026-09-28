# Client architecture

`go-tplink` is a provider client for TP-Link Kasa Cloud operations. It owns
provider-facing login, request encoding, response decoding, and error
classification. It does not own an application's credential store or device
workflows.

## Package boundaries

- `pkg/tplink` contains the client interface, request and result types, client
  options, and operations.
- `pkg/tplinkmodels` contains TP-Link response models, device classification
  helpers, capability detection, and typed provider errors. Callers can use
  these types without depending on HTTP client construction details.
- The HTTP dependency is injected into `pkg/tplink` through
  `WithHTTPClient`. Callers can configure a standard `http.Client` or another
  `Do`-compatible transport for proxies, tracing, and offline tests.

The client uses request-scoped `AuthContext` values. A client can therefore be
shared across accounts without changing shared token state. Callers own request
contexts and deadlines. `Close` prevents future client operations; it does not
close a caller-owned HTTP transport.

## Authentication and session handling

`Login` sends the supplied email and password to the configured cloud endpoint
and returns the provider's session token and account metadata. The library does
not persist credentials or tokens. Authenticated calls receive the session
token in that operation's `AuthContext`.

The provider token is opaque and has server-controlled expiry. The supported
surface has no refresh operation: when the provider reports an expired token,
the caller must obtain credentials again and call `Login`. Keep the token in a
secret store at the application boundary. Do not log it, include it in an
error, or commit it in a fixture. The wire protocol places the token in the
request URL query; treat URLs as sensitive in transport logs as well.

## Request and response flow

Operations use the configured regional HTTPS endpoint. The client provides
US, EU, Asia-Pacific, and global base URLs and defaults to US. Callers can
choose a base URL explicitly when they need another region. Device metadata
may contain `appServerUrl`; the client does not use that field to redirect
later requests automatically.

Cloud calls use one JSON POST endpoint. Its top-level `method` selects the
operation. Login and device-list responses have an `error_code` envelope with
an operation result. Device control calls use the `passthrough` method: the
outer `requestData` field contains a JSON-encoded device command, and the
`responseData` field in the result contains another JSON document. Device
commands are namespaced, for example `system.get_sysinfo`,
`system.set_relay_state`, and
`smartlife.iot.smartbulb.lightingservice.get_light_state`.

The supported flow uses HTTPS to the cloud endpoint and has no separate
request-signing or application-layer encryption step. HTTPS TLS handling
comes from the injected Go HTTP dependency. The passthrough JSON string is an
envelope detail; it is not evidence of end-to-end encryption to a device.

Cloud errors and device errors are separate layers. Cloud errors are reported
in the outer envelope. Device errors can appear at the top level of
`responseData` or under the requested namespace and command. The client turns
recognized errors into `tplinkmodels` error types while retaining provider
error codes where available. See [protocol notes](protocol.md) for wire-level
request shapes and details that remain unverified.

## Device model

The device-list model retains identifiers, alias, model and firmware
metadata, device type, and advertised lighting fields. Classification
recognizes `IOT.SMARTPLUGSWITCH` and `IOT.RANGEEXTENDER.SMARTPLUG` as plugs and
`IOT.SMARTBULB` as a bulb. Capability detection uses those type and metadata
fields: recognized plugs and bulbs expose on/off; a plug-like device with a
positive `brightness` field is also marked as brightness-capable. Bulb
brightness, color, and variable color temperature depend on the corresponding
device-list flags. Other device types are classified as `Other` and remain
available in the returned device list.

## Concurrency and lifecycle

Authentication is supplied per operation; do not add mutable account tokens to
a shared client. The injected HTTP dependency must be safe for concurrent use
when the client is shared. Pass cancellation and deadlines through
`context.Context`. Close a client when its caller-owned lifecycle ends;
repeated close calls are safe. The client does not start background polling,
subscribe to events, or own a persistent connection.

## Supported and unsupported scope

The client surface covers cloud login, device listing, plug operations (power
state, on/off, and reboot), and bulb operations (light state, brightness,
color temperature, and hue/saturation). It also exposes device alias updates
and scalar getters for fields of the light state.

The client does not expose local-network control, energy history, schedules,
scenes, groups, firmware management, device setup or provisioning,
camera/media streams, or event subscriptions. This means the library has no
supported Go operation for those areas; it does not establish that TP-Link
services or devices lack them. Unknown device types remain in the returned
device list but are not assigned a plug or bulb classification.

There is no pagination, push, MQTT, or websocket flow in the current client
surface. `getDeviceList` returns the list from one response, without a cursor
contract represented in the client.
