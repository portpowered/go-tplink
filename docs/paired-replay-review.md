# Independent paired-replay review

Reviewed implementation commit: `f73b979c90df5bdea2da76d9763da859dd15399a`.
Criterion: item 15 of `go-third-party-template/docs/library-standards.md`
and its fixture guidance in `docs/verification.md`. This review changes no
replay implementation.

| Criterion | Verdict | Evidence |
| --- | --- | --- |
| Item 15, paired replay | **Open** | The 15 synthetic files contain request and response sections, and request matching now precedes every outcome. Repeated calls, inline overrides, and token matching still fall short of one fully consumed pair per exchange. |
| Renewed independent signoff | **Open** | Recheck the repaired implementation and every checklist item at the final commit. |

## Verified improvements

`tests/replay/transport_test.go` now checks method, origin, escaped path,
complete query multimap, relevant headers, operation, and complete JSON body
before a normal response, response override, or transport error is returned.
It rejects calls beyond `expectedCalls`; `expectSequence` rejects an operation
issued out of its configured order; `assertConsumed` checks the matched count.
`tests/replay/transport_pair_test.go` covers request mismatch, extra call,
unconsumed count, out-of-order call, and an override-path mismatch. The only
built-in network edge found is HTTP. The 15 files are correctly identified as
**synthetic** in their paths and neighboring README.

## Open findings

1. **A call count is not one-time fixture consumption.** `RoundTrip` still
   selects a fixture by the operation parsed from each actual body, falling
   back to `synthetic_tplink_<operation>.json` when no route is configured.
   It increments `matchedCalls` but never consumes a particular fixture or
   body variant. `TestLoginAndDeviceListReplay` explicitly expects two
   `getDeviceList` calls and serves both from the same pair. For legitimate
   repeats, store two expected exchanges in order and consume each once.
   Single-call tests using the default `expectedCalls = 1` still do not name
   their expected operation; an unintended but supported operation can select
   its own fixture. `assertConsumed` only proves the total call count.
2. **Override outcomes have no stored paired response/error.** `useResponse`
   accepts a status and body assembled inside tests, and `useError` accepts
   an inline transport error. The outbound request is matched against a
   normal fixture before those outcomes, which repairs the earlier bypass,
   but the returned override or error is not the stored response/error half
   of that request pair. These paths are used throughout
   `client_error_paths_test.go` and `client_replay_test.go`. Record each
   synthetic failure outcome with its outbound expectation.
3. **The token rule accepts arbitrary nonempty text.** Thirteen authorized
   fixture files use `<nonempty>` for the query token, and
   `matchFixtureRequest` accepts any nonempty value. The suite's fixed
   `test-token` can be matched exactly; a changing or redacted token needs a
   rule that verifies its format or decoded meaning.
4. **Body variants are not individually inventoried or consumed.** Several
   files contain multiple complete request bodies, including relay on/off
   and light-state transitions. `matchFixtureRequest` accepts any listed
   body, but no fixture-level counter proves every variant was replayed.
   Add an operation/variant inventory tied to expected exchanges before
   signing off all supported commands.

`make lint`, `make check`, and fresh `go test -count=1 -race ./tests/replay`
passed at the reviewed commit. `make check` reported 96.8% replay coverage
for `pkg/tplink`; that percentage does not resolve the exchange gaps.

**Signoff:** item 15 and renewed independent signoff remain open.
