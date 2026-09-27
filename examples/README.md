# Examples

`list-devices` demonstrates the read-only login and account device-list flow.
It makes live requests to TP-Link and requires credentials supplied through
environment variables. It does not change device state. It is deliberately
not part of the normal offline test suite.

On macOS or Linux:

```sh
export TP_LINK_EMAIL='account@example.com'
export TP_LINK_PASSWORD='use-a-secret-store'
# Optional when the account uses another region:
export TP_LINK_BASE_URL='https://eu-wap.tplinkcloud.com'
go run ./examples/list-devices
```

In PowerShell:

```powershell
$env:TP_LINK_EMAIL = 'account@example.com'
$env:TP_LINK_PASSWORD = 'use-a-secret-store'
# Optional when the account uses another region:
$env:TP_LINK_BASE_URL = 'https://eu-wap.tplinkcloud.com'
go run ./examples/list-devices
```

The example does not print credentials or session tokens. Keep credentials in
an appropriate local secret store rather than saving them in the source tree.
