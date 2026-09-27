# Synthetic TP-Link response fixtures

These payloads were migrated from `portos-backend/integrations/go-tplink/test/fixtures`.
That directory had no capture dates, source descriptions, redaction notes, or other
provenance records. Their origin cannot be verified, so they are classified as
synthetic here and do not establish observed TP-Link behavior.

The payloads support deterministic, offline replay tests for these response
shapes and client behaviors:

| Fixture suffix | Test behavior |
| --- | --- |
| `error_parameter` | Cloud parameter error mapping |
| `error_rate_limit` | Cloud throttling error mapping |
| `error_token_expired` | Expired session error mapping |
| `error_unsupported` | Device unsupported-operation response |
| `getDeviceList_empty` | Empty device list |
| `getDeviceList_mixed` | Multiple plug and bulb shapes and metadata |
| `getDeviceList` | One plug device |
| `login_failure` | Rejected login |
| `login` | Successful login response shape |
| `passthrough_lightingservice_get_light_state` | Bulb state response |
| `passthrough_lightingservice_transition_light_state` | Successful bulb update |
| `passthrough_system_get_sysinfo` | Plug state response |
| `passthrough_system_reboot` | Successful reboot |
| `passthrough_system_set_dev_alias` | Successful alias update |
| `passthrough_system_set_relay_state` | Successful plug state update |

They contain placeholder account, device, and token values. Do not use them as
credentials or as evidence of live service behavior.

To add a real capture, keep it in `../captured/` and include a neighboring
provenance note recording the operation, UTC capture date, source category,
redactions, and behavior the artifact supports.
