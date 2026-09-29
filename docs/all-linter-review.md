# Independent all-linter review

Reviewer: Codex `tplink_final_lint_review`, independent of the linter migration.
Reviewed implementation commit: `65c2626a1f88e1d275163552fefd817bdab01773`.
Criterion: item 5 of the [shared library standard](https://github.com/portpowered/go-third-party-template/blob/main/docs/library-standards.md).

**Verdict: Pass.** Item 5 is verified at the exact implementation commit above.

| Criterion | Verdict | Evidence |
| --- | --- | --- |
| Enable every linter without broad finding exclusions | **Pass** | `.golangci.yml` declares `version: "2"` and the literal `linters.default: all`. With golangci-lint v2.3.0, `golangci-lint linters` showed no disabled linters; `golangci-lint config verify` passed. The config has no issue exclusions or baseline. Source `nolint` annotations name specific linters and explain the local reason. |
| Run full-repository lint as a failing CI job on a pinned version | **Pass** | `.github/workflows/ci.yml` pins v2.3.0 and runs `./...` in the `lint` job without `continue-on-error`, an issue-exit override, or a new-issues filter. [CI run 36523037366](https://github.com/portpowered/go-tplink/actions/runs/36523037366) completed successfully on the reviewed SHA; its lint job and all six build/test verification matrix jobs passed. |
| Keep local checks and replay coverage in the combined gate | **Pass** | `make lint` runs `golangci-lint run ./...`; `make check` includes lint, build, race tests, and replay coverage. The implementation author reports both targets passed on the reviewed commit, with replay coverage at 96.4%. The CI verification matrix also passed the race and replay-coverage steps. |
| Keep synthetic and captured behavior distinct | **Pass** | Replay fixtures remain under `tests/replay/fixtures/synthetic/`; no captured fixture files or fixture payloads changed in the migration. `docs/fixtures-and-testing.md` continues to describe their synthetic provenance. |
| Preserve library behavior through the migration | **Pass** | I reviewed the implementation diff against its parent. It changes no public method signatures, API schemas, generated models, or fixture payloads. The production changes are linter-driven refactors and contextual error wrapping; I found no changed wire operation or error mapping. |

## Finding disposition

The initial migration commit, `7ca33bd85902521abf2db7e75742e68217c1c758`, explicitly set `linters.exclusions.generated: strict`. I tested a temporary copy of the config without that stanza using golangci-lint v2.3.0; `run ./...` reported zero issues. The pinned version already applies strict generated-file handling by default, so the explicit setting was redundant. The follow-up commit reviewed here removes it. **Disposition: resolved.**

[Documentation run 36523037393](https://github.com/portpowered/go-tplink/actions/runs/36523037393) also completed successfully on the reviewed SHA.

## Separate repository observation

At review time, GitHub reported that `main` had no branch protection and the repository had no rulesets. This is recorded as repository governance context; the item 5 verdict above covers the failing lint job and workflow behavior.
