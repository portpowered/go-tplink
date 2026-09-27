# Client architecture

`go-tplink` is a provider client for the TP-Link Kasa Cloud API operations
currently used by the Port OS integration. It owns provider-facing login,
request encoding, response decoding, and error classification. It does not own
an application's credential store or device workflows.

## Package boundaries

- `pkg/tplink` contains the provider-specific client interface, request and
  result types, client options, and operations.
- `pkg/tplinkmodels` contains TP-Link response models, device classification
  helpers, capability detection, and typed provider errors. Backend code can
  inspect these types without depending on client construction details.
- The HTTP dependency is injected into `pkg/tplink` through `WithHTTPClient`.
  Callers can configure a standard `http.Client` or another `Do`-compatible
  transport for proxies, tracing, and offline tests.

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
error, or commit it in a fixture. The legacy wire protocol places the token in
the request URL query; treat URLs as sensitive in transport logs as well.

The old backend auth handler takes email and password directly; its
`RequestMultifactorCode` method is a no-op. That is the behavior of the current
Port OS adapter, not a claim that every TP-Link account or product has no
additional account security flow.

## Request and response flow

Operations use the configured regional HTTPS endpoint. The legacy wrapper
defined US, EU, Asia-Pacific, and global base URLs and defaulted to US. The
caller chooses a base URL explicitly when a different region is needed. Device
metadata may contain `appServerUrl`, but the legacy client did not use that
field to redirect later requests automatically.

Cloud calls use one JSON POST endpoint. Its top-level `method` selects the
operation. Login and device-list responses have an `error_code` envelope with
an operation result. Device control calls use the `passthrough` method: the
outer `requestData` field contains a JSON-encoded device command, and the
`responseData` field in the result contains another JSON document. Device
commands are namespaced, for example `system.get_sysinfo`,
`system.set_relay_state`, and
`smartlife.iot.smartbulb.lightingservice.get_light_state`.

The supported flow uses HTTPS to the cloud endpoint and has no separate
request-signing or application-layer encryption step in the wrapper. HTTPS
TLS handling comes from the injected Go HTTP dependency. The passthrough JSON
string is an envelope detail; it is not evidence of end-to-end encryption to a
device.

Cloud errors and device errors are separate layers. Cloud errors are reported
in the outer envelope. Device errors can appear at the top level of
`responseData` or under the requested namespace and command. The client turns
recognized errors into `tplinkmodels` error types while retaining provider
error codes where available. See [protocol notes](protocol.md) for the
wire-level request shapes and which details remain unverified.

## Device model

The device-list model retains identifiers, alias, model and firmware metadata,
device type, and advertised lighting fields. The Port OS adapter currently
classifies `IOT.SMARTPLUGSWITCH` and `IOT.RANGEEXTENDER.SMARTPLUG` as plugs, and
`IOT.SMARTBULB` as a bulb. Capability detection uses those type and metadata
fields: plug-like types expose on/off, and bulbs expose on/off. A plug-like
device with a positive `brightness` field is also marked as brightness-capable.
Bulb brightness, color, and variable color temperature
depend on the corresponding device-list flags. Other device types are not
currently surfaced by the backend adapter.

These mappings preserve the existing backend behavior. Device IDs and aliases
come from TP-Link's response; the adapter uses the provider device ID as the
route's external ID and associated provider route ID. The library does not
create Port OS routes or interpret Port OS capability schemas.

## Concurrency and lifecycle

Authentication is supplied per operation; do not add mutable account tokens to
a shared client. The injected HTTP dependency must be safe for concurrent use
when the client is shared. Pass cancellation and deadlines through
`context.Context`. Close a client when its caller-owned lifecycle ends;
repeated close calls are safe. The client does not start background polling,
subscribe to events, or own a persistent connection.

## Supported and unsupported scope

The client surface covers cloud login, device listing, the plug operations
needed by the backend (power state, on/off, and reboot), and bulb operations
(light state, brightness, color temperature, and hue/saturation). It also
exposes device alias updates and scalar getters for fields of the light state.

The current wrapper does not expose local-network control, energy history,
schedules, scenes, groups, firmware management, device setup/provisioning,
camera/media streams, or event subscriptions. This means the library has no
supported Go operation for those areas; it does not establish that TP-Link
services or devices lack them. Unknown device types remain available in the
returned device list but are not mapped to backend routes by the current
adapter.

There is no pagination, push, MQTT, or websocket flow in the current client
surface. `getDeviceList` returns the list from one response, without a cursor
contract represented in the wrapper.

## Backend adapter boundary

The backend adapter remains responsible for extracting the provider token from
Port OS secrets, constructing request-scoped auth inputs, mapping provider
results and errors to Port OS messages, route discovery, and publishing
attribute reports. It must not move Port OS secret handling or workflow into
this library. See the [backend migration plan](backend-migration.md) for the
current operation mapping and cutover steps.
