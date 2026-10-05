# Independent review record

Status: **open; fixes and two final independent approvals are required**.

These reports audit `a2d78a0b704f8837853574884f4e90fcc9bf8bf4`.
The current [checklist](template-checklist.md) pins shared template
`62cc3cb5a1308dae8700f92052f99b1c455a1d98`.
Later repairs and standards changes require verification at the final commit.

## Findings being resolved

- Closed wire enum checks must cover nested generated fields.
- Paired request matching must validate userinfo and effective authority.
- Anonymous generated objects need individual inventory entries.
- Supported token renewal and explicit credential retrieval need customer guidance.
- Current rendered Pages and separately published CLI evidence remain outstanding.

The first reviewer delivered a blind all-item audit before its document appendix.
The second report disclosed historical verdict exposure and is repair evidence;
a replacement blind reviewer must provide final approval. Passing checks do not
close these findings. Historical reports remain in Git history.

## First independent review

# Independent checklist audit — Reviewer 1

**Reviewed implementation:** `a2d78a0b704f8837853574884f4e90fcc9bf8bf4`
**Pull request:** [PR 1](https://github.com/portpowered/go-tplink/pull/1), base `main`
**Checklist:** `docs/template-checklist.md` from that commit
**Standards reviewed:** `go-third-party-template@05e93ff08899414207e9335717e7d7b0190ebd09`; clarification and checklist source `go-third-party-template@585b5677caebd4e6d58db215e5278d35a4a7aadf`
**Reviewer:** Codex independent Reviewer 1
**Review date:** 2026-10-04 (America/Los_Angeles)

## Recommendation

**Not merge-eligible at this SHA.** I found two unresolved gaps: the wire inventory accepts arbitrary caller values for nested schema-closed enums (item 4), and the paired replay matcher accepts unpaired URL user information and `Request.Host` overrides (item 15). The source request gate separately rejects the latter fields on the current production path, but the replay pair itself is incomplete. Do not treat this report as a scoped sign-off.

Publication evidence is separate: the PR documentation build and artifact pass, but deployment was skipped for this pull request. The SDK and CLI consumer installs, published Pages checks, and final-commit two-reviewer audit remain post-merge/release proof.

The consolidated verdicts and post-delivery appendix below add a third item-4 finding
about anonymous-object inventory and correct item 11 to open under the literal
checklist requirement.

## Findings

### F1 — Nested closed enum provenance is not fail-closed (item 4)

`tools/wireinventory/scalar_package_provenance.go:457-508` builds closed-enum metadata from each component's direct `properties`. It does not descend through nested object properties. In `api/passthrough.openapi.yaml:90-102` and `:118-127`, `SystemSetRelayStateCommand.system.set_relay_state.state` is the closed enum `{0,1}` and `SystemRebootCommand.system.reboot.delay` is the closed enum `{1}`. Their generated Go fields are inside anonymous nested structs, so they are not in the current direct-component closed-enum map. The actual SDK builders use generated fixed constants (`pkg/tplink/client_power.go`), but the gate permits other production code to supply invalid values.

I added this compile-valid TEMP-only probe under `pkg/tplink` and removed it after testing:

```go
func BuildRelayFromArbitraryCaller(value int) dependencymodels.SystemSetRelayStateCommand {
    var command dependencymodels.SystemSetRelayStateCommand
    command.System.SetRelayState.State = dependencymodels.SystemSetRelayStateCommandSystemSetRelayStateState(value)
    return command
}

func BuildRebootFromArbitraryCaller(value int) dependencymodels.SystemRebootCommand {
    var command dependencymodels.SystemRebootCommand
    command.System.Reboot.Delay = dependencymodels.SystemRebootCommandSystemRebootDelay(value)
    return command
}
```

From the repository root, `go test -run '^$' ./...` compiled it, and `go run ./tools/wireinventory` returned exit 0. This contradicts the checklist's nested fixed-value and helper-provenance requirements. The direct, flat `LoginCloudRequest.Method` case is correctly rejected: an exported helper accepting `dependencymodels.LoginCloudRequestMethod` and assigning it to `LoginCloudRequest.Method` compiled, then the exact root command rejected it with `fixed scalar in generated wire model must come from a schema-bound constant or caller input`. True open caller values remain accepted by `TestPackageScalarProvenanceAcceptsCallerOpenValueThroughHelperSink` (`LoginParams.CloudUserName`) and `TestCallerOpenStateForwardingIsAllowed` (`LightTransitionState.AdditionalProperties`).

The body-buffer addendum is enforced: adding `bodyBytes[0] = 'x'` after `http.NewRequestWithContext` in `pkg/dependencies/cloud/cloud.go` compiled, and the exact root command rejected it with `constructed HTTP request must reach the injected Do without mutation or escape`.

### F2 — Paired replay ignores authority-affecting request fields (item 15)

`tests/replay/transport_test.go:401-429`, `matchFixtureRequest`, compares method, `scheme://host`, escaped path, query, headers, operation, and JSON body. It does not compare `request.URL.User` or `request.Host`. I added and removed a TEMP-only replay test using an otherwise expected device-list request. Both of these mutations were accepted and returned the paired response:

- `request.URL.User = url.UserPassword("probe-user", "probe-password")`
- `request.Host = "attacker.invalid"`

`go test ./tests/replay -run '^TestProbeReplayRejectsUnpairedAuthorityFields$' -count=1` failed because the replay accepted both. The source mutation gate has separate negative controls for URL user information and `Host`, so this finding is about the paired-replay contract and its independent protection, not a current path through the production SDK. Add those effective authority/auth fields to the fixture expectation/matcher and a regression test before item 15 is signed off.

## Sixteen separate verdicts

| # | Verdict at `a2d78a0` | Evidence |
| --- | --- | --- |
| 1 | **Pass** | The SDK, README, examples, and Pages guides are application-independent. `pkg/tplink` owns provider operations; no consuming-application adapter or rollout plan was found. The CLI is isolated under `cmd/go-tplink` as its own module. |
| 2 | **Pass** | Exported options and methods are documented with examples in README and the MDX guides. Authentication, token handling, errors, transport injection, supported operations, and synthetic/implementation-derived evidence are explicit. I checked guide signatures and payloads against `pkg/tplink` and generated DTOs. |
| 3 | **Pass for source; live badge refresh pending publication** | README has Go version, CI, replay coverage, release, Go Reference, license, and Docs badges; all repository owner values point to `portpowered/go-tplink`. The coverage endpoint is generated by the docs workflow. Final live badge destinations should be checked after Pages deployment and release. |
| 4 | **Fail — F1** | The component inventory, generator records, endpoint record, and generated outputs are present. All 74 declarations found in registered `.gen.go` files appear by name in `docs/wire-model-inventory.md`; `go run ./tools/wireinventory` passes on the clean frozen tree. Production network sources include SDK, dependencies, generated packages, CLI, and examples. Exact default-root negative probes reject flat closed-enum drift, unregistered CLI/example outbound calls, and mutable request backing bytes. However, F1 proves nested closed relay/reboot enum caller values pass. |
| 5 | **Pass** | Root and CLI configs literally set `linters.default: all`; CI pins golangci-lint v2.14.0 and invokes full `./...` runs. No `issues-exit-code=0`, `continue-on-error`, baseline, or path-wide exclusion was found. Inline suppressions name a specific linter and reason. Exact PR CI run `37248370982` completed successfully on this SHA, including lint, CLI lint, schema generation, API compatibility, and all Linux/macOS/Windows Go 1.24/1.26 jobs. |
| 6 | **Pass** | Synthetic request/response and stored error outcomes are paired and labeled synthetic. `go run ./tools/replaycoverage` reports SDK 94.1% (269/286), public models 96.3% (78/81), cloud transport 100% (81/81), and combined non-generated coverage 95.5% (428/448), above the 80% gate and 90% target. Generated exclusion count: 1. |
| 7 | **Pass for implementation; published consumer proof pending** | Public client/projection types are under `pkg/tplink` and `pkg/tplinkmodels`; provider wire types are generated in `pkg/dependencymodels`; transport is in `pkg/dependencies/cloud`. API responsibilities have separate schemas and generated outputs, with a separate projection schema and generated compatibility aliases. CLI is a separate module consuming the public SDK. The release workflow has an isolated public consumer-module check; that check is tag-triggered and remains pending. |
| 8 | **Pass** | `NewClient` uses functional options with generated default base URL and rejects nil/invalid options, invalid URLs, and an effective unsafe HTTP client. Credentials and tokens are passed in named operation requests, not reusable configuration. |
| 9 | **Pass** | Client state is limited to configured endpoint/HTTP doer and synchronized closed state; auth is request-scoped. A direct `*http.Client` with non-nil `Jar` returns `ConfigurationError`; accepted clients are shallow-copied. Tests simulate a login `Set-Cookie`, then an account-B request on the same SDK client, and assert both full request envelopes and no `Cookie`. They also test mutation of the original supplied/default client after construction. Custom doers are documented as responsible for avoiding account-specific cookie state. |
| 10 | **Pass for current surface** | Source scan covers `pkg/tplink`, `pkg/tplinkmodels`, `pkg/dependencies`, `pkg/dependencymodels`, `pkg/generatedwire`, `cmd/go-tplink`, and `examples`. Independently found only the injected HTTP `Do` call in `pkg/dependencies/cloud`; the SDK, CLI, and examples have no WebSocket/MQTT/RTC/socket edge or active runtime dependency network call. HTTP injection reaches the single edge. |
| 11 | **Pass** | `Login` accepts caller credentials and returns `tplinkmodels.LoginResult` including the token. Every later authenticated operation takes `AuthContext`; no implicit refresh or token retention exists. Docs explicitly say there is no refresh operation in this supported surface and callers must log in again after expiry. |
| 12 | **Build/links pass; final deployment proof pending** | Docs CI run `37248370920` succeeded on the exact SHA, uploaded the Pages artifact, and skipped deploy because this was a pull request. I downloaded and inspected the artifact: 15 rendered HTML pages include root, Guides, all eight guide pages, and the generated API reference. Internal links resolve with the configured `/go-tplink` base path; every guide has the generated reference route; each destination renders guide/API content rather than a fallback. `api/openapi.yaml` has no `externalDocs`; the release workflow's upgrade-guide URL resolves in the artifact. Live external Pages/coverage destinations still need post-deploy verification. |
| 13 | **Source/page content pass; appendix and post-publish checks pending** | I reviewed README and every tracked documentation file except `docs/independent-review.md`, which was intentionally withheld during this blind first pass. Contributor inventories, architecture, protocol, and fixture notes are not linked as customer guides; customer guides stay in MDX. README remains focused on install, authenticated usage, scope, lifecycle/configuration, and guides. The rendered artifact pages are concise and on-purpose. After this initial report is delivered, I will restore and audit the review record, append that result here, and then recheck source/document consistency. Release-note URLs and published pages require the post-merge publication pass. |
| 14 | **Open** | This is one independent report for the frozen SHA. F1 and F2 remain unresolved; a second independent reviewer must verify every fix at the final implementation SHA, and both verdicts must be recorded in the current review document before checklist completion. |
| 15 | **Fail — F2** | Fixture coverage includes method, origin, escaped path, repeated query values/order, complete relevant headers, JSON body variants, response status/headers/body, ordered sequences, duplicate/extra rejection, and exhaustion. Public SDK methods are mapped to paired fixtures. F2 shows the matcher accepts authority-affecting URL userinfo and Host overrides not present in a pair. Current transport scope is HTTP; there is no socket/session transcript to verify. |
| 16 | **Pass for implementation; published CLI consumer/install proof pending** | CLI has its own nested module, blocking pinned all-linter/build/race-test/vet/format/tidy CI, paired offline workflows for login/errors/device listing/plug and bulb operations/alias, JSON outputs, nonzero errors, cancellation and close, protected file/stdin/env credential input, and explicit credential export. Its MDX guide documents installation but explicitly states the first standalone CLI release is pending. The tag-triggered workflow installs `cmd/go-tplink/v*` from the Go proxy; that proof has not run yet. |

## Independent commands and external evidence

From the clean frozen TEMP clone root, I ran:

```text
go run ./tools/wireinventory                         PASS
go test ./tools/wireinventory -count=1               PASS
go run ./tools/replaycoverage                        PASS (95.5%, 428/448 combined)
go test ./tests/replay -run 'Cookie|TransportPair|ReplayFixture' -count=1  PASS
go test -race ./...                                  PASS (from cmd/go-tplink)
go test ./tools/wireinventory -run 'TestPackageScalarProvenanceAcceptsCallerOpenValueThroughHelperSink|TestPackageScalarProvenanceAcceptsCallerSuppliedCallbacks|TestPackageScalarProvenanceAcceptsCallerSuppliedReturnedCallback|TestGeneratedModelCallerValuesAreAllowed|TestCallerOpenStateForwardingIsAllowed' -count=1  PASS
```

Both GitHub Actions runs point to the exact reviewed SHA. CI run `37248370982` and Documentation run `37248370920` completed successfully; Documentation's `deploy` job was skipped under its pull-request condition. The PR's canonical review URL is `https://github.com/portpowered/go-tplink/pull/1`.

## Blind-review document handling

For the blind first pass, I copied `docs/independent-review.md` byte-for-byte to `C:\Users\andre\AppData\Local\Temp\go-tplink-independent-review-a2d78a0b-withheld.bin` before any broad search, then placed a neutral placeholder at that path **in this TEMP clone only**. I did not inspect the saved file or another reviewer's findings before delivering this report. No source file in the original checkout was edited. The clone still resolves to exact HEAD `a2d78a0b704f8837853574884f4e90fcc9bf8bf4`; only the intentional review-document placeholder is currently dirty.

After this initial report is delivered, I will restore the saved review document byte-for-byte in the TEMP clone, verify the hash, and add an appendix covering its audience, staleness, links, and consistency with this audit. That delayed appendix does not change the two implementation findings or make item 13 blocked solely due to this temporary deferral.

**Signed:** Codex independent Reviewer 1 — 2026-10-04

## Post-delivery documentation and verdict appendix

After the initial blind report was delivered, I restored and inspected the withheld
`docs/independent-review.md` in the same TEMP clone. Before broad searches, I had
copied it byte-for-byte outside the clone and replaced only that clone's copy with a
neutral placeholder. The saved and restored file SHA-256 is
`26B34EFF1C67A2F399DD43784E4D1F09B35020B0B6C6278584804090E1D13846`; the clone is
clean after restoration and still resolves to `a2d78a0b704f8837853574884f4e90fcc9bf8bf4`.
No file in the original checkout was edited.

### Review-record audience, links, and freshness

The record is maintainer-facing review evidence, linked from the checklist and
contributor fixture notes, and it is not part of customer Pages navigation. Its two
review sections identify `7b1742fb0efbfe441f31c3a0317e63584988f516`; the local checklist
link resolves and the canonical PR link is `https://github.com/portpowered/go-tplink/pull/1`.
The top status accurately says fixes and final independent approvals are required.
The record does not yet contain an all-16 audit at this newer `a2d78a0` SHA, so its
current-evidence role is stale until the final reviews and finding dispositions are
appended. Keep the 7b reports as clearly labeled history rather than carrying their
old findings forward as current.

At `a2d78a0`, the older 7b findings for the flat login enum, request backing bytes,
example network inventory, aliased `cloud.Send`, injected CookieJar, architecture
account-sharing claim, check-description table, and checklist link are resolved by
current source/tests/docs. The old anonymous-object inventory concern remains: the
complete schema-to-Go table lists named component declarations, and the field map
flattens nested keys, but it still has no entry for the anonymous Go object paths in
generated command DTOs. See F3 below. The record's old item-11 refresh concern also
remains open under the literal checklist wording.

### Additional item-4 finding — anonymous generated objects are not inventoried

The checklist requires each production wire struct, including anonymous objects, to
be recorded with its schema owner, generated Go type, generator command, and actual
conversion/use site. `docs/wire-model-inventory.md:28-66` lists named components;
`:188-214` lists component keys and nested JSON paths. It does not identify the
anonymous generated types at paths such as
`SystemSetRelayStateCommand.System.SetRelayState` or
`SystemRebootCommand.System.Reboot`. The generated declarations are visible at
`pkg/dependencymodels/passthrough.gen.go:299-307` and `:278-285`, while the inventory
only lists their parent component rows and leaf values. A flattened field map does
not provide the requested per-object schema-to-Go mapping. This is F3 and is a second
item-4 completeness issue, independent of F1's executable provenance bypass.

### Verdict corrections after reading the frozen checklist and record

- **Item 11 is OPEN.** The exact checklist at `docs/template-checklist.md:142` requires
  explicit token exchange and refresh operations. The SDK returns login credentials
  and does not silently retain refreshed tokens, but its API and schemas have no
  refresh operation or supported refresh contract (`README.md:88`,
  `docs/architecture.md:37`, `docs/guides/authentication.mdx:60`). Caller re-login
  documentation does not satisfy the explicit refresh-operation clause. This
  supersedes the initial table's item-11 PASS.
- **Item 13 is OPEN pending the current record update and publication review.** The
  audience and rendered-copy review is complete; the review record remains scoped to
  7b and does not yet include this a2 report or current dispositions. The PR artifact
  is sound, but live Pages and release-note destinations remain post-merge checks.
  This open status reflects the actual stale current-review record and pending
  publication evidence, not the temporary blind-review deferral.
- **Items 4 and 15 remain FAIL (F1/F3 and F2); item 14 remains OPEN.** Together with
  item 11, these prevent merge eligibility at the reviewed SHA. Source fixes and
  publication work must be reviewed at their final SHA by both independent reviewers.

**Post-delivery appendix signed:** Codex independent Reviewer 1 — 2026-10-04

### Consolidated sixteen-item verdicts

| # | Final reviewer-1 verdict at `a2d78a0` |
| --- | --- |
| 1 | Pass |
| 2 | Pass |
| 3 | Pass for source; live badge destinations pending publication |
| 4 | Fail — F1 nested closed-enum provenance; F3 anonymous-object inventory |
| 5 | Pass — exact-SHA blocking CI and literal-all pinned lint |
| 6 | Pass — deterministic paired synthetic fixtures and measured coverage |
| 7 | Pass for package/schema boundaries and consumer import; published proxy proof pending |
| 8 | Pass — options, defaults, and validation |
| 9 | Pass — injected CookieJar rejection and two-account isolation tests |
| 10 | Pass for all currently shipped network edges |
| 11 | Open — the required explicit refresh operation has no supported contract or API |
| 12 | Pass for PR artifact and internal links; published Pages proof pending |
| 13 | Open — current review record and post-publication documentation review remain pending |
| 14 | Open — two final independent all-item reviews and finding closure required |
| 15 | Fail — F2 paired replay omits URL user information and `Request.Host` |
| 16 | Pass for CLI implementation; published CLI module/install proof pending |

**Final implementation merge eligibility at this SHA: NO.** Findings F1, F2, and F3
remain, and item 11 is open. Post-merge Pages and post-tag SDK/CLI consumer proof are
separate publication gates.

## Second review: repair evidence

# go-tplink all-16 reviewer evidence - NOT final blind approval

## Scope and disclosure

Reviewer: delegated independent reviewer / /root/tplink_a2_final_r2; I did not implement this change. This is repair evidence for the parent, not checklist item 14 final sign-off.

Repository: portpowered/go-tplink
Reviewed HEAD: a2d78a0b704f8837853574884f4e90fcc9bf8bf4
PR 1: https://github.com/portpowered/go-tplink/pull/1 (open, base main)
Standards: shared go-third-party-template main at 585b5677caebd4e6d58db215e5278d35a4a7aadf; reviewed template-checklist.md plus linked client-design.md, verification.md, website.md, library-standards.md, reference/README.md, and releasing.md.
Fresh temporary clone: C:/Users/andre/AppData/Local/Temp/go-tplink-review-a2d78a0/repo. No original-repository source changes.

Review isolation disclosure: the task required deferring docs/independent-review.md until initial findings were delivered. Before delivery, I accidentally ran git diff for that file and saw a short fragment from the saved record. I stopped, disclosed it to the parent, and read no other reviewer findings. Thus this report is not blind final approval. The parent directed me to deliver repair evidence and have a fresh reviewer use a sanitized archive for final approval. I will only restore/audit the deferred item 13/14 appendix after delivery.

## Exact-head evidence

- GitHub reports PR 1 open, base main, exact head above. CI run 37248370982 passed lint, CLI lint, API compatibility, schema generation, SDK verify on Ubuntu/macOS/Windows Go 1.24/1.26, and CLI verify on Ubuntu/Windows Go 1.24/1.26: https://github.com/portpowered/go-tplink/actions/runs/37248370982
- Docs run 37248370920 passed build; deploy was skipped for the PR: https://github.com/portpowered/go-tplink/actions/runs/37248370920
- Local Go 1.26.8 and pinned golangci-lint v2.14.0. Plain make check passed at exact HEAD: root and CLI lint each 0 issues; build, race tests, vet, tidy, format, replay coverage, wire inventory all passed. Linter emitted deprecation warnings for gomodguard, exhaustruct, wsl names but no findings; no linter was disabled.
- Coverage: pkg/tplink 94.1% (269/286), pkg/tplinkmodels 96.3% (78/81), pkg/dependencies/cloud 100% (81/81), combined non-generated 95.5% (428/448), above 80% floor and 90% target; one generated file excluded.
- PR Pages artifact: 426 files, 13 HTML pages, 159 local hrefs, 0 broken, one unique external link to pkg.go.dev (HTTP HEAD 200). Root, guide, generated POST / reference, and coverage routes rendered. Current deployed Pages URLs and pkg.go.dev return 200 but are the existing main publication; exact PR deployment remains pending.
- Current root tag is v0.2.1 at b4a683e697a17f18e5b20d6a6c0adaf22c59e15e. No SDK v0.3.0 or nested cmd/go-tplink/v0.3.0 tag exists.

## Checklist verdicts

### 1. Keep the library independent of a consuming application - PASS
README.md describes a standalone provider SDK and leaves credential storage and device workflows to callers (README.md:12-14, 24-26, 69-72); docs/architecture.md:3-6 states the same. Examples and package search found no application adapter or rollout plan.

### 2. Document exported API, supported operations, errors, transport and customer guides - PASS
go doc ./pkg/tplink and go doc ./pkg/tplink.ClientInterface match README and guides: Login, device listing, plug state/on/off/reboot, bulb state/controls/scalar getters, alias update and Close; options WithBaseURL/WithHTTPClient; request-scoped AuthContext. README has an authenticated example (README.md:28-73). Guides cover auth, CLI, errors, devices, plugs, lighting and upgrades. Evidence status and synthetic fixtures are explicit (docs/guides/index.mdx:28-33, docs/protocol.md:121-127); docs claims matched source.

### 3. Badges and repository values - PASS
README.md:3-9 includes Go, CI, replay coverage, release, Go Reference, license and docs badges with correct repo/report targets. Current Pages, coverage.json, coverage.html and pkg.go.dev targets returned HTTP 200. Latest published root release is v0.2.1; exact PR publication remains a separate release gate.

### 4. Wire endpoint/model inventory, schema bindings, provenance and API reference - PARTIAL
Wire/schema/source-gate evidence passes; exact Pages publication is pending. api/openapi.yaml and docs/wire-model-inventory.md:104-108 define the single POST / route. Login, getDeviceList, passthrough, nested passthrough payloads/results, cloud envelopes, public projections and CLI-local JSON models have separate schemas/generated files (api/README.md:3-24; tools/wireinventory/generation_inventory.go:33-89). The inventory maps components, primitives, constants, generated Go declarations and uses. The endpoint gate checks schema plus the exact inventory row (generation_inventory.go:105-154); generated outputs must be tracked and registered, and unknown .gen.go files under pkg/cmd are rejected (lines 177-260). The AST/source scan includes pkg/tplink, tplinkmodels, dependencies, dependency models, generatedwire, CLI and examples (wireinventory/main.go:177-238). Production source search found one outbound edge: injected HTTPDoer in pkg/dependencies/cloud/cloud.go:20-48; request build at lines 65-100; no private/encrypted/event/socket/WebSocket/MQTT or networked dependency edge. CI default-root controls and extensive negative/positive tests cover helper provenance, fixed values, generated maps, route/request mutation, import binding, body buffers, hand-written models and unknown network primitives (wireinventory/main_test.go:30-133, 243-916, 916-1377; package_provenance_test.go:18-123, 435-661). Temporary compile-valid probes in the fresh clone confirmed: two-file fixed scalar helper return rejected; caller input accepted; two-file url.Values escape rejected; URL.Path mutation before Do rejected by the exact default command. Probes were reverted. Exact-head Fumadocs build passed, but Pages deployment was skipped.

### 5. Pinned blocking full all-linter CI - PASS
Root and CLI .golangci.yml set version 2 and literal linters.default: all. CI pins v2.14.0 and runs full SDK/CLI ./... lint (ci.yml:13-44); no issue-exit override or continue-on-error. Only narrow depguard allowlists and forbidigo policy were observed. Exact-head lint jobs and plain make check passed at 0 issues.

### 6. Synthetic pairs and coverage - PASS
Fixtures are labeled synthetic in tests/replay/fixtures/synthetic/README.md and docs/fixtures-and-testing.md. Tests cover success, errors, token expiry/redaction, cancellation, malformed/oversized response and cleanup. Combined coverage is 95.5%, meeting both thresholds; generated exclusion is reported. No live capture is claimed.

### 7. Package boundaries and public compatibility - PARTIAL
SDK API is pkg/tplink, semantic models/errors pkg/tplinkmodels, provider wire types pkg/dependencymodels, transport pkg/dependencies/cloud. CLI is a separate module with its own local schema. CLI currently resolves released SDK v0.2.1 (cmd/go-tplink/go.mod:1-8) and compatibility CI passed. No clean external module resolves the exact PR SDK API from its intended v0.3.0 publication; isolated public import proof awaits that tag.

### 8. Functional options/defaults/validation - PASS
NewClient uses public Option, defaults the endpoint and HTTP client, rejects nil option/client and invalid URL, rejects direct *http.Client cookie jars and snapshots accepted clients (pkg/tplink/client.go:51-86). WithHTTPClient and WithBaseURL are exposed (lines 131-160); no account credential in reusable config.

### 9. Stateless sessions and cookie isolation - PASS
Client holds config and a closed flag only; AuthContext carries each token, Close prevents later calls and leaves caller transport ownership with caller (client.go:37-44, 89-97; docs/architecture.md:19-27). Jar-bearing *http.Client is rejected with typed ConfigurationError; tests exercise two accounts and complete cookie-free requests (tests/replay/client_error_paths_test.go:30-80; client_cookie_isolation_test.go:17-63). Custom HTTPDoer contract explicitly prohibits account-cookie retention and requires concurrency safety (client.go:131-135).

### 10. Injection at every implemented network edge - PASS
Shipped production search found only cloud.Send through injected HTTPDoer; no other HTTP, socket, event, signaling or dependency network call. WithHTTPClient replaces that edge. SDK and CLI pairs exercise requests/responses offline.

### 11. Visible tokens and renewal responsibility - PASS
Login returns tplinkmodels.LoginResult including token (pkg/tplink/auth.go:13-58); authenticated calls take AuthContext. Implemented schemas have no refresh operation; expiry docs instruct caller re-login and caller-owned token storage, and warn query URLs/logs are sensitive (docs/architecture.md:29-41; docs/guides/errors.mdx:73-81). No silent token retention/refresh.

### 12. Customer MDX, generated reference and rendered links - PARTIAL
Customer guides live in docs/guides and link the generated cloud reference. Exact workflow invokes shared Fumadocs action v0.3.0 (docs.yml:31-53). PR artifact has all 159 local hrefs resolving and expected routes/content. pkg.go.dev resolves; schemas contain no externalDocs. Build passed but PR deploy skipped, so exact-head Pages publication and live-page post-migration review are open.

### 13. Page copy/audience, duplication and all-doc review - OPEN / PROVISIONAL
I inspected all tracked docs except the deliberately deferred docs/independent-review.md: README, API/examples/fixture READMEs, AGENTS, checklist, inventory, architecture, protocol, fixtures/testing, and all guides. Customer content is in MDX; contributor inventory, architecture, protocol, generation and coverage detail are in contributor docs. README focuses on installation, short authenticated example, scope, caller duties, safety and guide links. Artifact links are clean. The deferred review record and exact Pages deployment remain unreviewed; therefore item 13 is not complete. I will inspect only item 13/14 appendix after report delivery.

### 14. Two independent final reviewers - OPEN
This report is repair evidence and cannot serve as blind approval due the disclosure above. The parent intends a fresh blind reviewer from sanitized archive. Keep unchecked until two independent final-commit reviews, every finding disposition, and all remaining checklist items are verified.

### 15. Paired requests/responses and lifecycle - PASS
Replay matches method, origin, path, query, headers and body before returning responses; rejects unexpected calls and enforces sequence/exhaustion (transport_pair_test.go:32-184, 485-603); failure outcomes are paired (321-484). CLI paired transport asserts requests, responses and consumption for login, discovery and device flows (cmd/go-tplink/main_test.go:101-158, 231-549); auth-failure/cancel tests assert Close (284-333, 550-580). All pairs are labeled synthetic.

### 16. Standalone installable CLI - PARTIAL
Separate module cmd/go-tplink provides explicit auth, discovery, plug/bulb controls, aliases, JSON, nonzero errors, cancellation, cleanup and protected credential inputs/export. Paired offline tests cover login success/failure, discovery, state/control, cancellation, redaction/export and cleanup. CI runs blocking all-linter/build/race/vet/format/module checks for Go 1.24/1.26 on Windows and Ubuntu; exact jobs passed. Guide explicitly says first CLI release is pending and installation is available after publication. No nested cmd/go-tplink/v0.3.0 tag or external go install/consumer proof exists yet; SDK v0.3.0 is also pending.

## Disposition

No source-level functional or schema-gate defect was confirmed in this repair-evidence audit. Keep these gates separate from implementation merge readiness:
1. SDK v0.3.0 tag and isolated SDK consumer/import proof pending.
2. CLI cmd/go-tplink/v0.3.0 tag and external go install/consumer proof pending.
3. Exact-head Pages deploy pending (PR build passed, deploy skipped).
4. Items 13/14 open pending the deferred appendix audit and fresh independent blind review.

No source edits, merge, tag or publication were made by this reviewer.
## Post-delivery item 13/14 record appendix audit

After initial delivery, I restored docs/independent-review.md from the saved external copy. SHA256 before/after is 031029041E847AB92443BC26C552CEF0B7E8B1EA97BBC5D5C342EF6296F546E0. I inspected only numbered item 13/14 status lines. They say item 13 is still awaiting the audience/incoming-link appendix and item 14 remains open; there is no completed item 13 appendix or overall approval in those entries. This supports the provisional/open dispositions above. I did not inspect the rest of the saved record.

Disclosure: my first post-delivery extraction included too much context and showed adjacent item 15/16 status text and one brief item 15 reference to F4. I did not seek or read detailed F1-F4 material. I notified the parent immediately and stopped broader inspection. This report remains repair evidence, and the fresh sanitized-archive review remains necessary.
