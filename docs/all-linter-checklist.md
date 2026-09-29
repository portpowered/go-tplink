# All-linter checklist

This checklist tracks item 5 of the
[shared library standard](https://github.com/portpowered/go-third-party-template/blob/main/docs/library-standards.md)
for TP-Link.

- [x] Configure golangci-lint v2 with the literal `linters.default: all`; the
  resolved v2.3.0 config has no disabled linters or global issue exclusions.
- [x] Pin golangci-lint v2.3.0 in CI and run the full repository with `./...`
  as a failing job, without an issue-exit override or new-issues filter.
- [x] Keep `make lint` and `make check` covering full-repository lint, build,
  race tests, and replay coverage. The implementation author reports both
  targets passed with 96.4% replay coverage.
- [x] Keep replay fixtures classified as synthetic and separate from captured data.
- [x] Have an independent reviewer confirm item 5 and passing CI on the exact
  implementation commit. See the [review record](all-linter-review.md) for
  commit `65c2626a1f88e1d275163552fefd817bdab01773` and CI run 36523037366.

This checklist covers item 5 only. The historical paired-replay signoff remains in [paired-replay-checklist.md](paired-replay-checklist.md).
