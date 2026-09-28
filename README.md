# go-tplink

[![Go version](https://img.shields.io/github/go-mod/go-version/portpowered/go-tplink)](go.mod)
[![CI](https://github.com/portpowered/go-tplink/actions/workflows/ci.yml/badge.svg)](https://github.com/portpowered/go-tplink/actions/workflows/ci.yml)
[![Replay coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fportpowered.github.io%2Fgo-tplink%2Fcoverage.json)](https://portpowered.github.io/go-tplink/coverage.html)
[![Release](https://img.shields.io/github/v/release/portpowered/go-tplink?display_name=tag)](https://github.com/portpowered/go-tplink/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/portpowered/go-tplink.svg)](https://pkg.go.dev/github.com/portpowered/go-tplink)
[![License](https://img.shields.io/github/license/portpowered/go-tplink)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-GitHub%20Pages-blue)](https://portpowered.github.io/go-tplink/)

A standalone Go client for the TP-Link Kasa Cloud API. It provides typed
operations for authentication, device listing, plug controls, bulb controls,
and device aliases. Applications manage credential storage and their own
device workflows.

## Install

Requires Go 1.24 or later.

```sh
go get github.com/portpowered/go-tplink@v0.1.0
```

The public client is in `github.com/portpowered/go-tplink/pkg/tplink`; provider
models and typed errors are in
`github.com/portpowered/go-tplink/pkg/tplinkmodels`.
Request and result structs are generated from
[`api/client-models.openapi.yaml`](api/client-models.openapi.yaml); cloud wire
structs are generated separately from [`api/openapi.yaml`](api/openapi.yaml).
Run `make generate-api` after changing either schema.

## Usage

Credentials are supplied to `Login`. Each later operation receives its session
token through a request-scoped `AuthContext`:

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
support concurrent calls if the client is shared. See the
[client architecture](docs/architecture.md).

The [read-only device-list example](examples/README.md) shows live usage with
credentials supplied through environment variables.

## Authentication and session model

The library exchanges an email and password for an opaque provider session
token. It does not persist credentials, and the supported provider surface has
no token refresh operation. When the provider rejects an expired token, call
`Login` again with credentials supplied by the application. Treat the token
and token-bearing request URLs as secrets.

## Supported scope

The current client supports cloud login and device listing; plug power state,
on/off, and reboot; bulb state, brightness, color temperature, and
hue/saturation; and device alias updates. It also exposes scalar light-state
getters. Device classification recognizes known plug and bulb types while
retaining unknown device records in the provider response.

The client does not currently provide local-network control, energy history,
schedules, scenes, groups, firmware management, setup or provisioning, camera
or media streaming, or push and event subscriptions. These are limits of this
client surface, not claims about every TP-Link product's capabilities.

## Testing and evidence

The automated suite uses offline HTTP tests and synthetic fixtures. The JSON
fixtures have no verifiable capture metadata and are not presented as live
captures. See [fixture and testing notes](docs/fixtures-and-testing.md) for
their status, redaction rules, and local commands. The
[protocol notes](docs/protocol.md) distinguish implementation observations
from vendor-verified facts.

## Commands

Run commands from the module root:

| Purpose | Command | Requires credentials |
| --- | --- | --- |
| Build | `make build` | No |
| Test | `make test` | No |
| Format | `make fmt` | No |
| Vet | `make lint` | No |
| Combined checks | `make check` | No |
| Replay coverage | `make replay-coverage` | No |
| API compatibility report | `make api-compatibility` | No |

The [shared API Docs action](https://github.com/portpowered/api-docs-website-github-action)
generates the Fumadocs website from the checked-in [OpenAPI contract](api/openapi.yaml).
The contract describes this client's implemented cloud wire format and is not
an official or vendor-verified TP-Link specification. The documentation
workflow also publishes a replay coverage report. Pushes to `main` refresh
the site through GitHub Pages; pull requests build the site for verification.

## Safety and redaction

Never commit account passwords, session tokens, cookies, email addresses,
provider account IDs, device IDs, MAC addresses, or unredacted private
responses. Avoid logging full request URLs because the provider token is sent
in a query parameter. Keep sanitized live captures separate from synthetic
fixtures and give each capture a neighboring provenance note.

## Documentation

- [Documentation website](https://portpowered.github.io/go-tplink/)
- [Client architecture](docs/architecture.md)
- [Protocol notes](docs/protocol.md)
- [Fixtures and testing](docs/fixtures-and-testing.md)
- [Examples](examples/README.md)
