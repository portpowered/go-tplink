# Port OS backend migration

The standalone module is the provider-client boundary and is released as
`v0.1.0`. This guide records the adapter boundary and the initial cutover
checklist; it is not a live status page for the backend migration. Check the
backend's module dependency and contract checks to confirm a consumer's current
state. The standalone module remains independent of `portos-backend`.

The matrix was checked against the former client under
`portos-backend/integrations/go-tplink/pkg/tplink/` and its consumer under
`portos-backend/backend/internal/integrations/tplink/`. In particular, the
consumer's `auth.go`, `definition.go`, and `plugin.go` own auth-link flow,
client construction, message dispatch, route mapping, and attribute reporting.
These sibling-repository paths record the reviewed source boundary; the
standalone module does not depend on that repository.

## Operation matrix

| Provider behavior | Former client operation | Current backend use | Standalone adapter mapping |
| --- | --- | --- | --- |
| Authenticate | `Login(email, password)` | Auth handler exchanges account credentials for a stored session token | Call `Login` with `LoginRequest`; persist only the returned token through backend auth flow |
| List account devices | `GetDevices()` | Route discovery | Call `GetDevices` with request-scoped `AuthContext`; map `Devices` through the same provider-type and capability rules |
| Turn plug on/off | `TurnOn` / `TurnOff` | Power commands | Build per-call auth and device requests; return provider typed errors to backend translation |
| Read plug state | `GetPowerState` | Attribute reporting; bulbs fall back when this operation fails | Call `GetPowerState`; retain current bulb fallback to `GetLightState` |
| Reboot plug | `Reboot` | Available in client; no call in the current plugin command switch | Keep exposed at the library layer; add a backend command only with a Port OS contract and product requirement |
| Set bulb brightness | `SetBrightness` | Brightness command | Map backend brightness request to named provider request |
| Set bulb color temperature | `SetColorTemp` | Color-temperature command | Map Kelvin value without changing units |
| Set bulb color | `SetColor` | Color command | Map hue and saturation without changing ranges |
| Read bulb state | `GetLightState` | Attribute reporting fallback | Map `LightState.OnOff` to Port OS power state |
| Read individual light fields | `GetBrightness`, `GetColorTemp`, `GetColor` | Not currently called by the plugin | Preserve as provider convenience operations; do not add backend dependencies without a consumer need |
| Set alias | `SetAlias` | Not currently called by the plugin | Keep as provider operation; route rename support needs a separate backend contract |
| Refresh session | No provider operation | Backend auth handler reports that re-authentication is required | Keep refresh unsupported; return a clear re-authentication-required error |

The existing backend interface is intentionally narrower than the provider
client. It includes device listing, plug and bulb controls, state reporting, and
`Close`. Its route mapper ignores unknown device types and advertises power
and connectivity for recognized devices, with brightness, color, or color
temperature added from the capability flags. Preserve that behavior during the
initial adapter cutover; expand it only alongside explicit product and protocol
decisions.

## Adapter cutover checklist

1. Point the backend module to the standalone `github.com/portpowered/go-tplink`
   module and use the standalone package imports.
2. Replace client-wide token configuration with `tplink.AuthContext` on each
   authenticated operation. Read the token from the current Port OS message
   secret at the adapter boundary and do not persist it in a shared client.
3. Convert backend inputs to the provider's named request structs. Keep
   Port OS message parsing, secret access, route discovery, and event
   publication in the backend.
4. Update the backend's narrow client interface and test doubles to match the
   standalone signatures. Translate `tplinkmodels` typed errors at the backend
   boundary, including expired-token handling that publishes an authorization
   update.
5. Keep the auth handler's credential validation and token persistence in the
   backend. Use the standalone login operation only for the vendor exchange.
6. Verify every row in the matrix against backend tests and the repository's
   focused wrapper/plugin build check. Remove the duplicated old wrapper only
   after all consumers and checks use the standalone module.

## Boundary rules

- The library owns TP-Link request/response behavior and provider-specific
  models and errors.
- The backend owns auth-link and secret persistence, Port OS message dispatch,
  route mapping, attribute schemas, scheduling, logs, and published events.
- Do not carry the backend's `TPLinkDeviceClient` interface wholesale into the
  provider package. Keep the provider client close to provider operations; use
  a small adapter or backend interface where Port OS needs fewer methods.
- Avoid logging auth inputs, session tokens, full token-bearing URLs, or
  unredacted provider payloads.
