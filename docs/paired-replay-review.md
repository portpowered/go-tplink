# Independent paired-replay review

## Final re-audit at `2298ea5`

Reviewer: Codex `alexa_independent_review`, independent of the TP-Link replay
implementation. Standard 15 is **verified** for the library's current HTTP
surface. The previous four findings below were rechecked against the source,
all stored pairs, and fresh race tests. These fixtures remain synthetic; they
do not establish observed provider behavior.

| Previous finding | Independent disposition |
| --- | --- |
| Repeated call count without one-time exchange consumption | **Resolved.** `replayTransport.steps` now names an operation for every expected call; `RoundTrip` selects only the next step, checks its request before incrementing `matchedCalls`, and rejects an extra or out-of-order call. `TestLoginAndDeviceListReplay` declares login plus two separate list steps, each with its own exact token. `assertConsumed` checks all declared steps were used. The negative tests reject mismatches, duplicates, out-of-order calls, and unconsumed steps. |
| Inline override responses and errors | **Resolved.** `paired_outcomes.synthetic.json` stores 33 identified request/response or transport-fault pairs. `useResponse`, `useError`, and `useFault` resolve to a stored outcome and compare supplied status/body/error text before replay. `RoundTrip` matches the stored request before returning any normal response, override, or fault. `TestEverySyntheticFailureOutcomeHasStoredRequestAndResponse` checks each ID, provenance shape, outbound operation, mismatch rejection, consumption, and duplicate rejection. |
| Token wildcard | **Resolved.** All 13 token-bearing normal fixtures use the exact `test-token` value; login fixtures have no token. `expectToken` sets an exact per-step alternative for the two-account and sensitive-token tests. The request matcher compares every query value, and `TestReplayFixtureRejectsRequestOutsidePair` rejects both empty and wrong nonempty tokens. No `<nonempty>` token rule remains. |
| Body variants not inventoried or consumed | **Resolved.** The 15 normal fixture files contain 22 complete request-body variants. `TestEverySyntheticReplayFixtureHasRequestAndResponse` selects each variant in a separate one-step replay, checks its response status/headers/body, asserts consumption, and rejects a duplicate call. `TestPublicMethodReplayInventory` maps all 15 exported network methods to stored operations/variants; the public operation tests replay relay, lighting, reads, and account actions through the injectable HTTP edge. |

The matcher checks method, origin, escaped path, full repeated query values,
relevant headers, parsed operation, and complete JSON body before returning
the stored status, headers, and body. Every failure outcome also carries a
stored request expectation. Normal test cleanup checks that the expected
sequence was consumed. The only built-in network edge found is HTTP.

At `2298ea5`, `make lint` passed. `make check` passed with a process-local Git
`safe.directory` setting needed for this Windows checkout; it ran vet, build,
race tests, and the replay-coverage gate at 96.4% (270/280 statements) for
`pkg/tplink`. Fresh `go test -count=1 -race ./tests/replay` passed. Standard 15
is signed off for the current implementation; the synthetic fixtures and
historical provenance limits remain as labeled.

## Historical open review at `f73b979`

The findings below are retained as review history and are superseded by the
final re-audit above.

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
