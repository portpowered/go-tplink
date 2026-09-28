# Independent paired-replay review

Reviewed commit: `860e876b529783342f2d280900240673a937d082`.
Standard: item 15 of `go-third-party-template/docs/library-standards.md`
and its `docs/verification.md`. This review did not change the replay
implementation.

| Item | Verdict | Evidence |
| --- | --- | --- |
| 15. Paired replay | **Open** | All 15 synthetic JSON files contain request and response sections, but the replay transport still permits response fallback, unvalidated overrides, and repeated or unconsumed exchanges. |
| 14. Independent verification | **Open** | Item 15 remains open. A reviewer must verify its repairs and recheck the complete checklist at the final commit before signing item 14. |

## Evidence and gaps

1. **The stored pairs cover the supported HTTP commands.** The single
   network edge is the injected `HTTPDoer` in `pkg/tplink/client.go`, which
   sends the OpenAPI cloud POST. The 15 files under
   `tests/replay/fixtures/synthetic/` contain method, origin, escaped path,
   query multimap, relevant headers, one or more complete JSON request body
   variants, and response status, headers, and body. They cover login,
   device list shapes, cloud and device errors, and the six passthrough
   command families. The relay-state file has both on/off bodies; the light
   transition file has four bodies; the unsupported-operation file has both
   supported test requests. Each is explicitly classified as synthetic in
   the fixture name and neighboring README. No WebSocket, MQTT, or other
   built-in transport was found.
2. **Default routing can replay a response again.**
   `replayTransport.RoundTrip` selects a fixture by the operation derived
   from the outbound JSON body when `fixtureRoutes` has no entry. It validates
   the request envelope before returning that fixture's response, but does
   not mark the pair consumed. A second matching call receives the same
   response. There is no ordered expected-exchange list or `AssertConsumed`;
   most tests check only their observed request count and selected fields.
   `TestLoginAndDeviceListReplay` intentionally calls device list twice and
   receives the same fixture each time. Item 15 requires each expected
   exchange to be consumed, rejects unexpected or duplicate calls, and
   preserves order where it matters.
3. **Error and response overrides bypass pair matching.** `useResponse`
   and `useError` return by operation key before `matchFixtureRequest` runs.
   Many malformed-response and failure tests in
   `tests/replay/client_replay_test.go` and
   `tests/replay/client_error_paths_test.go` use these paths. Those cases
   have no stored paired outbound request expectation, and an origin,
   token, header, or body mismatch could still receive the override. Pair
   each synthetic response and expected transport error with its request;
   never fall back to a response after any request mismatch.
4. **The token match rule is too broad.** Authorized fixtures use
   `<nonempty>` for the query token. That accepts any nonempty string,
   including malformed or unintended credentials, rather than checking a
   documented format or explicit synthetic/redacted-token meaning. An exact
   synthetic value or a constrained matcher would satisfy this part of
   item 15.
5. **Negative coverage is partial.**
   `TestReplayFixtureRejectsRequestOutsidePair` exercises wrong origin,
   empty token, and a body with an extra field. It does not test duplicate
   calls, unconsumed fixtures, out-of-order exchanges, or a mismatch through
   an override path. `TestEverySyntheticReplayFixtureHasRequestAndResponse`
   checks required fields, but does not verify that every body variant is
   invoked and consumed.

Fresh `go test -count=1 -race ./tests/replay` passed at the reviewed
commit. That passing gate does not resolve the replay-control gaps.

**Signoff:** items 15 and 14 remain unchecked.
