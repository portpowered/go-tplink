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
go get github.com/portpowered/go-tplink@latest
```

The public client is in `github.com/portpowered/go-tplink/pkg/tplink`; provider
models and typed errors are in
`github.com/portpowered/go-tplink/pkg/tplinkmodels`.

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
client does not store credentials or tokens. Supply an injected HTTP client
with `tplink.WithHTTPClient` when the application needs custom transport
behavior; use `tplink.WithBaseURL` for a configured cloud endpoint.

See the [device discovery guide](https://portpowered.github.io/go-tplink/docs/guides/devices)
for model fields and classification behavior.

## Terminal CLI

The separate `go-tplink` command supports login, device discovery, plug and
bulb operations, and device aliases. See the
[terminal CLI guide](https://portpowered.github.io/go-tplink/docs/guides/cli)
for current availability, credential handling, commands, and JSON output.

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

## Safety and redaction

Keep account passwords and session tokens secret. Avoid logging full request
URLs because the provider token is sent in a query parameter.

## Documentation

- [Documentation and operation guides](https://portpowered.github.io/go-tplink/)
- [Authentication](https://portpowered.github.io/go-tplink/docs/guides/authentication)
- [Device discovery](https://portpowered.github.io/go-tplink/docs/guides/devices)
- [Plug operations](https://portpowered.github.io/go-tplink/docs/guides/plugs)
- [Lighting operations](https://portpowered.github.io/go-tplink/docs/guides/lighting)
- [Error handling](https://portpowered.github.io/go-tplink/docs/guides/errors)
