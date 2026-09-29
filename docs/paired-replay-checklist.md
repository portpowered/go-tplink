# Paired replay checklist

This checklist applies item 15 of the shared library standard to TP-Link.
Synthetic replay verifies the implementation contract and is not captured
provider evidence. The [independent review](paired-replay-review.md) re-audited
all previous findings at implementation commit `2298ea5`.

- [x] Store expected outbound method, origin, escaped path, query policy,
  relevant headers, and complete JSON body variants with each response in all
  15 synthetic replay fixtures.
- [x] Match a request before returning its normal, override, or error outcome.
  Reject an origin, query, body, or operation mismatch.
- [x] Reject extra calls, assert consumed-call counts at test cleanup, and
  check the order of multi-call replay sequences.
- [x] `TestPublicMethodReplayInventory` inventories all 15 exported client
  operations and their fixture variants. Normal and failure tests declare an
  expected sequence; `TestEverySyntheticReplayFixtureHasRequestAndResponse`
  consumes each normal variant exactly once. Failure outcomes have stored
  request and response halves in `paired_outcomes.synthetic.json`.
- [x] Fixtures require the exact synthetic token `test-token`; tests using
  other synthetic tokens declare their exact expected value per step. The
  request-mismatch test rejects empty and wrong nonempty tokens.
- [x] Have the independent reviewer recheck the final implementation commit
  and sign off item 15 after all findings above are resolved. The review
  counted 15 normal files with 22 body variants and 33 stored failure/fault
  pairs; the historical `make lint`, `make check`, and race replay tests passed
  under the toolchain in use at `2298ea5`. The later all-linter CI migration
  requires separate verification on its own final commit.
