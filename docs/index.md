# go-tplink

`go-tplink` is a standalone Go client for the TP-Link Kasa Cloud API. It
provides typed operations for authentication, device listing, plug and bulb
controls, and device aliases.

Start with the [repository README](https://github.com/portpowered/go-tplink#readme)
for installation and usage. The pages here describe the package boundaries,
protocol behavior, and test evidence.

The documentation workflow publishes a replay coverage report alongside
these pages. It is generated from the offline replay tests after enforcing a
90% statement coverage floor for `pkg/tplink`.

## Pages

- [Client architecture](architecture.md)
- [Protocol notes](protocol.md)
- [Fixtures and testing](fixtures-and-testing.md)
