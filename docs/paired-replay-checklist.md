# Paired replay checklist

This checklist applies item 15 of the shared library standard to TP-Link.
Synthetic replay verifies the implementation contract and is not captured
provider evidence. The [independent review](paired-replay-review.md) was made
at `860e876`; it must be repeated at the final commit.

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
- [ ] Have the independent reviewer recheck the final commit and sign off
  item 15 only after all findings above are resolved.
