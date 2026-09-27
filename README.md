# go-tplink

`go-tplink` is a standalone Go client for the TP-Link Kasa Cloud API operations
used to list and control supported plugs and bulbs. It keeps provider
protocol and typed errors reusable outside Port OS; applications remain
responsible for credential storage and their own device workflows.

## Getting started

Install the released `v0.1.0` module with:

```sh
go get github.com/portpowered/go-tplink@v0.1.0
```

The public client is in `github.com/portpowered/go-tplink/pkg/tplink`; provider
models and typed errors are in
`github.com/portpowered/go-tplink/pkg/tplinkmodels`.

The smallest end-to-end example logs in and lists devices. It makes live,
read-only cloud requests, requires a TP-Link account, and is not run by tests:

```sh
TP_LINK_EMAIL='account@example.com' \
TP_LINK_PASSWORD='use-a-secret-store' \
go run ./examples/list-devices
```

Set `TP_LINK_BASE_URL` when the account uses a non-default regional endpoint.
The example defaults to the module's US endpoint. See
[examples](examples/README.md) for PowerShell instructions and the example's
scope.

## Usage

Credentials are supplied to `Login`, and each later operation receives its
session token through a request-scoped `AuthContext`:

```go
import (
    "context"
    "fmt"

    "github.com/portpowered/go-tplink/pkg/tplink"
)

func listDevices(ctx context.Context, email, password string) error {
    client, err := tplink.NewClient()
    if err != nil {
        return err
    }
    defer client.Close()

    session, err := client.Login(ctx, tplink.LoginRequest{
        Email:    email,
        Password: password,
    })
    if err != nil {
        return err
    }

    devices, err := client.GetDevices(ctx, tplink.GetDevicesRequest{
        Auth: tplink.AuthContext{AccessToken: session.Token},
    })
    if err != nil {
        return err
    }
    for _, device := range devices.Devices {
        fmt.Println(device.Alias, device.DeviceModel)
    }
    return nil
}
```

Pass the same `AuthContext` explicitly to each authenticated operation. The
client does not store credentials or tokens. The injected HTTP dependency must
support concurrent calls if the client is shared; see
[client architecture](docs/architecture.md).

## Authentication and session model

The library exchanges an email and password for an opaque provider session
token. It does not persist credentials, and the supported provider surface has
no token refresh operation. When the provider rejects an expired token, the
application must request credentials again and call `Login`. Treat the token
and token-bearing request URLs as secrets. The backend integration owns
auth-link and secret persistence; the adapter boundary and cutover checklist
are described in [the backend migration guide](docs/backend-migration.md).

## Supported scope

The current client supports cloud login and device listing; plug power state,
on/off, and reboot; bulb state, brightness, color temperature, and
hue/saturation; and device alias updates. It also exposes scalar light-state
getters. Device types unknown to the current Port OS adapter remain in the
provider result but are not mapped to backend routes.

The wrapper does not currently provide local-network control, energy history,
schedules, scenes, groups, firmware management, setup/provisioning, camera or
media streaming, or push/event subscriptions. These are unsupported by this
client surface and are not claims about every TP-Link product's capabilities.

## Testing and evidence

The automated suite uses offline HTTP tests and synthetic fixtures. The JSON
fixtures migrated from the backend had no verifiable capture metadata and are
not presented as live captures. See [fixture and testing notes](docs/fixtures-and-testing.md)
for their status, redaction rules, and local commands. The
[protocol notes](docs/protocol.md) separate source-observed behavior from
vendor-verified facts.

## Command surface

Run commands from the module root:

| Purpose | Command | Requires live credentials | Notes |
| --- | --- | --- | --- |
| Build | `make build` | No | Builds all packages |
| Test | `make test` | No | Runs offline tests with the race detector |
| Format | `make fmt` | No | Runs `go fmt ./...` |
| Vet | `make lint` | No | The current target runs `go vet ./...` |
| Combined check | `make check` | No | Runs lint, build, and test |
| API compatibility | `make api-compatibility` | No | Reports changes against a prior release when available |
| Integration/live test | Intentionally omitted | — | No live-account test harness is provided |
| Docs check | Intentionally omitted | — | Review links and examples when documentation changes |

## Safety and redaction

Never commit account passwords, session tokens, cookies, email addresses,
provider account IDs, device IDs, MAC addresses, or unredacted private
responses. Avoid logging full request URLs because the provider token is sent
in a query parameter. Keep future sanitized live captures separate from
synthetic fixtures and give each capture a neighboring provenance note.

## Related docs

- [Client architecture](docs/architecture.md)
- [Protocol notes](docs/protocol.md)
- [Port OS backend migration](docs/backend-migration.md)
- [Fixtures and testing](docs/fixtures-and-testing.md)
- [Examples](examples/README.md)
