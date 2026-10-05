# Independent review record

At the user's request on 2026-10-04, final dual review and checklist closure
are deferred while known customer-facing fixes, required CI, and releases
are completed. These reports audit earlier snapshots and do not approve
the final release. Unverified checklist items remain open.

Status: open. Two independent initial all-16 audits are recorded below; neither approves the pending fixes or publication.

Initial source: `804b98d1d4ada73fc1b8820646dfc5be4c47d281`. Shared checklist: template `62cc3cb5a1308dae8700f92052f99b1c455a1d98`.

Open source findings cover schema-example validation, nested reference rendering, Windows inventory drift, replay diagnostics/framing, and the ordered CLI workflow test. Publication evidence and final re-verification remain pending. A PASS in either initial report does not resolve the other report's finding.

Historical reports remain in Git history. This is the current record linked from the checklist.

## Reviewer 1 — initial all-16 audit

### Blind independent ALL16 audit — Reviewer 1

**Reviewed source:** `804b98d1d4ada73fc1b8820646dfc5be4c47d281` (PR 1, base `main`).

**Reviewer:** `/root/tplink_7b_r1` — independent implementation reviewer, not an author.

**Signature:** Reviewer 1, 2026-10-05 UTC.

#### Snapshot and review boundary

The review used `C:\Users\andre\AppData\Local\Temp\go-ring-blind-review-va8fo7qq\checkout`, a fresh archive with local Git HEAD `e55280aa6d0b186126ecadd2fea597dfbdb7adc5`, no parent and no remote. The adjacent manifest identifies source SHA `804b98d1d4ada73fc1b8820646dfc5be4c47d281`; 120 regular files were hash-verified against that source. Only `docs/independent-review.md` is represented by a neutral placeholder in this snapshot and is excluded from source equivalence; the original source bytes were not included in the snapshot and were not read. The canonical source review record is deferred until the parent supplies the prepared docs-only file after this initial report.

I read the snapshot `AGENTS.md`, full `docs/template-checklist.md`, and the linked shared verification, client-design, website, releasing, and library standards at template commit `62cc3cb5a1308dae8700f92052f99b1c455a1d98`. No repository source edits were made. Temporary probe files were removed/restored; the checkout is clean. A build-created CLI executable in the TEMP clone was removed.

#### Verification performed

- `make lint` and `make check` passed locally on the exact source files using Go 1.26.8, the TEMP cache, and `C:\Users\andre\AppData\Local\Temp\portos_lint_214.cmd` (golangci-lint v2.14.0). `make check` covered SDK and CLI lint, build, race tests, vet, module tidy, formatting, replay coverage, and wire inventory. Combined replay coverage was 95.5% (428/448); public SDK 94.1% (269/286), models 96.3% (78/81), cloud transport 100% (81/81); the report excludes one generated file and enforces 80% with a 90% target.
- `go test ./tools/wireinventory -count=1` passed on the LF archive. The default root command is `go run ./tools/wireinventory`; its tests compile the CLI/example probe modules and invoke that exact command without an absolute-root override. Nested closed-enum tests include an exported helper accepting arbitrary `dependencymodels.LoginCloudRequestMethod` and assigning it to generated `LoginCloudRequest.Method` (`TestPackageScalarProvenanceRejectsUnvalidatedClosedEnumCallerValues`). Positive controls retain caller-defined open alias values.
- A separate TEMP consumer module importing `pkg/tplink` and `pkg/tplinkmodels`, with a local replacement to this exact SDK tree, passed `go mod tidy` and `go test ./...`.
- Exact-head CI metadata: Docs run `37255156075` succeeded; CI run `37255156141` failed both Windows SDK matrix legs (`windows-latest`, Go 1.24.x and 1.26.x) in the default wire-inventory checks. The failure says the anonymous model inventory is stale. I reproduced it in the TEMP clone by converting only `docs/wire-model-inventory.md` to CRLF and running `go run ./tools/wireinventory`; it then fails with the same stale-inventory error. Restoring the file returned it to its original SHA256 `0FEE12B6F273595437966150FAF0A6E4DB1F6D7071A315D720844B24A5ED19CF`.
- Docs artifact from run `37255156075` is in `C:\Users\andre\AppData\Local\Temp\go-tplink-pages-artifact-r1-804b98d\rendered`. I checked all 13 rendered HTML pages for internal links after removing the `/go-tplink` Pages base prefix: zero missing targets. The API page’s visible-text extraction is `...\api-text.txt`; a local static server was available, but this reviewer’s CUA reported no browser surfaces, so I could not activate client-side selectors. The API HTML contains a real request-schema `combobox` defaulting to `LoginCloudRequest` and a request-example `combobox` defaulting to “Login request (synthetic example)”; the option choices are not present in server-rendered visible text.
- External destinations checked included the rendered `pkg.go.dev` link (200), README badge/report targets (200), and release-note target `https://portpowered.github.io/go-tplink/docs/guides/upgrading/` (currently 404 on the live Pages site). The exact PR artifact includes the upgrading guide. Live Pages/Go proxy publication proof is therefore still pending main merge and deployment.

#### ALL16 verdicts

##### 1. Application independence — PASS

`README.md` presents a standalone provider SDK. The public `pkg/tplink` package takes request-scoped account auth; the example only lists devices; the separate CLI lives under `cmd/go-tplink`. No consuming-application adapters or rollout plan were found in the SDK surface.

##### 2. Operation docs, schema examples, and evidence classes — OPEN (source blocker)

Guides cover authentication, devices, plug operations, lighting, errors, upgrades, and CLI usage. The OpenAPI route has complete outer request and response examples and nested passthrough strings, while the contributor docs classify fixtures as synthetic and the schema describes the API as implementation-derived.

The checklist additionally requires every canonical example to be validated against its owning schema in CI. At this SHA `.github/workflows/ci.yml` schema-generation only runs `make generate-api` and checks generated-file drift; the Docs job renders schemas and replay coverage; root `make check` has no example-schema validator. There is no validator target/job for the outer examples or JSON encoded inside `requestData`/`responseData`. This is an unmet CI requirement even if examples are manually well-formed.

##### 3. README badges — PASS

`README.md` contains the required Go version, CI, replay coverage, release, Go Reference, license, and documentation badges with repository-specific targets. The tested badge/report endpoints responded successfully.

##### 4. Schema-backed wire/model inventory and discoverable API reference — OPEN (source blocker)

The source implementation has substantial schema coverage: generated endpoint/envelope and nested command DTOs, separate public projection models, a checked-in component/primitive/anonymous-object inventory, and package-wide provenance and network-edge gates. `tools/wireinventory/main.go` scans SDK packages, the standalone CLI, and `examples`. Its compile-valid default-command probes cover CLI/example HTTP escapes, nested relay/reboot enum values, and an aliased transport helper. The tests also cover recursive closed enums and caller-open positive cases. The current HTTP surface has one inventoried JSON `POST /`; source/import review found no additional production socket, WebSocket, MQTT, or third-party network call site.

The rendered Fumadocs API page does not expose the known passthrough payload fields in its visible output. `api-text.txt` shows one request model, `LoginCloudRequest`, and one login request example; counts for `GetDeviceListCloudRequest`, `PassthroughCloudRequest`, `PassthroughCommand`, and `requestData` are all zero in the visible page text. The bundled raw schema contains the oneOf members, but `PassthroughParams.requestData` is an encoded string with `contentSchema: PassthroughCommand`, and the rendered page does not show the nested command/result fields or variants. The schema and example controls are present as comboboxes, but I could not activate them in this environment to establish whether their options expose the missing variants. Under the checklist’s explicit requirement to inspect usable controls and rendered nested fields, item 4 cannot be signed off at 804.

##### 5. Offline build, lint, race, replay, and blocking CI — OPEN (source blocker)

Local `make lint` and `make check` passed with pinned golangci-lint v2.14.0 and `linters.default: all` in both configs. CI pins v2.14.0 and runs blocking full-repository jobs. The exact-head Windows SDK jobs fail `go test -race ./...` because `TestProductionWireInventoryPasses`, `TestGeneratedAnonymousModelInventoryIncludesNestedCommandObjects`, and `TestWireInventoryDefaultCommandScansShippedModules` observe the anonymous-model inventory as stale after Windows CRLF checkout. A default command that fails on supported CI platforms is a blocker despite the LF local pass.

##### 6. Deterministic fixtures and measured coverage — PASS

Replay fixtures are explicitly synthetic, pair complete request identity with response/failure outcomes, and assert consumption. `make check` and CI measure SDK, projection-model, and cloud-transport coverage and enforce the 80% floor. The exact local combined result was 95.5% with the generated exclusion reported.

##### 7. Package boundaries and public imports — PASS

Wire DTOs are under `pkg/dependencymodels`, caller projections under `pkg/tplinkmodels`, public operations under `pkg/tplink`, and HTTP transport under `pkg/dependencies/cloud`; the CLI is a separate module. A separate consumer module compiled the current public `pkg/tplink` and `pkg/tplinkmodels` imports against the frozen tree. Published-version proxy checks remain a later release proof.

##### 8. Functional options and configuration validation — PASS

`NewClient` uses options with a generated default base URL and default HTTP client. `WithBaseURL` validation rejects unsupported schemes, empty authority, credentials, query, and fragment. `WithHTTPClient` rejects nil implementations. Credentials and tokens stay on individual auth/login requests rather than reusable client options.

##### 9. Stateless sessions and injected cookie state — PASS

The reusable client stores no account token. It returns the login result and accepts `AuthContext` per operation. `NewClient` rejects an effective `*http.Client` with a non-nil `Jar` as a typed configuration error and snapshots the accepted client, including the default client. `TestReusableClientKeepsTwoAccountsCookieFree` simulates a Set-Cookie from one account, mutates the caller’s client to add a jar after construction, then asserts both complete requests are cookie-free and the second has only the other account’s token. Docs state caller-owned custom Doers must avoid cross-account cookie state.

##### 10. Injection at every network edge — PASS

The SDK’s only production network edge is the injected HTTPDoer used by `pkg/dependencies/cloud`. The wire gate rejects raw HTTP constructors and direct network primitives from every shipped production directory, including CLI and examples. Import review found no additional socket or network-using runtime dependency needing a connection-producing seam.

##### 11. Token exchange and renewal obligations — PASS

`Login` is explicit and returns current account/token data; each authenticated call receives the caller’s token. No refresh operation is claimed or implemented. README and authentication/error/CLI guides require explicit reauthentication and explain caller storage and expiry behavior.

##### 12. MDX guides, generated reference, and links — PASS for source artifact; publication proof pending

Customer guides are MDX under `docs/guides/` and link to the generated operation reference. The Docs CI artifact contains all guides and the API page; all 13 rendered HTML pages have no unresolved internal links after accounting for the project Pages prefix. The schema has no `externalDocs`; the rendered Go reference destination responded 200. The release workflow’s upgrading-guide URL currently returns 404 on the live Pages site because this PR’s guide is not deployed yet; the PR artifact has the page. Final live Pages verification must follow main merge/deploy.

##### 13. Documentation audience and copy — PASS for reviewed source; publication proof pending

The README stays focused on installation, an authenticated SDK example, supported surface, caller configuration/lifecycle, and links. Customer workflows live under `docs/guides/`; architecture, fixture provenance, protocol notes, schema instructions, and the wire inventory remain contributor material. `docs/protocol.md` labels its content implementation observations rather than vendor specification; the guides label the contract implementation-derived. No redundant customer-facing repository Markdown or obvious duplicate process guide was found. The current live upgrading link is pending the Pages deployment described under item 12. The withheld review record’s audience/staleness appendix will be audited only after this initial report is delivered.

##### 14. Independent two-reviewer record and final-commit recheck — OPEN

This initial blind report is signed independently and includes all 16 verdicts, but `docs/independent-review.md` was deliberately withheld from content review until delivery. The placeholder in the sanitized archive is not the source record. This review cannot confirm the record’s current audience, links, staleness, or appendix, and the exact SHA still has open findings in items 2, 4, 5, and 16. Both reviewers must verify all fixes at the final commit and place their separate evidence in one current review document before item 14 can pass.

##### 15. Request identity, framing, secret-safe errors, and paired replay — PASS

Replay identity checks reject URL user information, a Host override, opaque URLs, malformed queries, and mismatched method/origin/path, and ensure diagnostics omit URL credentials. The request mutation gate covers route and header changes, body replacement, `GetBody`, `ContentLength`, `TransferEncoding`, request/URL/header aliases, clones, helper escapes, and mutation of the byte slice backing `bytes.NewReader`; an unchanged body buffer is a positive control. Paired fixtures compare method, origin, path, query, headers, and body. These tests passed in the exact local `make check` run.

##### 16. Standalone customer CLI — OPEN (source test blocker; release proof pending)

The separate CLI consumes the SDK, has named auth/discovery/read/control commands, supports prompt/environment/stdin/credential-file login, stores only a protected token, offers explicit export/logout, uses returned device IDs, and documents an ordered install/login/discovery/operation flow. OAuth callback/PKCE is not part of the only supported implementation-derived login contract at this SHA: the modeled request is direct `login` with `cloudUserName` and `cloudPassword`; no authorization URL, redirect, callback, or activation contract is present.

The checklist explicitly requires offline verification of the documented sequence. Existing tests cover successful login/storage, failed auth/redaction/cleanup, device listing, plug reads and controls, cancellation, and close behavior, but do not run the full ordered flow from successful CLI login through the same saved-token store, device listing, selection of an ID actually returned by that list, and a named operation. `TestDevicesListAndPlugStateConsumePairedExchanges` starts by calling `saveTestToken` and uses independent transports, while login persistence is a separate test. Add a meaningful paired sequence test before signoff. The standalone CLI’s v0.3 tag and Go proxy install verification also remain pending; the source `cmd/go-tplink/go.mod` still requires SDK v0.2.1, so the CLI’s published dependency/version relationship must be resolved during the staged release after the SDK module is available.

#### Overall disposition at 804b

**Implementation merge eligibility: not ready at this SHA.** Source blockers remain in items 2 (example-schema CI validation), 4 (rendered nested variant discoverability), 5 (Windows default inventory gate), and 16 (ordered CLI workflow test). Items 14 and all publication-only evidence also remain open by definition.

**Final publication proof: pending.** SDK v0.3 and CLI v0.3 are not published; current Pages does not yet serve the upgrading guide, although the exact Docs artifact renders it. Verify the SDK consumer through the Go proxy, install the tagged CLI module, confirm the Pages deployment/release-note link, and then re-run both independent reviews on the exact final source SHA.

#### Post-delivery appendix — Reviewer 1 follow-up at source 804

**Reviewer:** `/root/tplink_7b_r1` — independent follow-up, no source authorship. **Date:** 2026-10-05 UTC.

After delivering the blind report, I rechecked item 15 against the two defects in reviewer 2's source-baseline report using only temporary tests in the immutable 804 archive. This supersedes item 15's initial PASS above: item 15 is **OPEN (source blocker)**.

- **Secret-safe diagnostic failure reproduced independently.** In a temporary `tests/replay/z_r1_probe_test.go`, I called `newReplayTransport`, set `expectSequence(replayDeviceListMethod)`, then submitted a correct device-list body with query token `r1-sensitive-token-987`. `go test ./tests/replay -run '^TestR1ProbeReplayLeaksMismatchedToken' -count=1` failed because the diagnostic included `query token[0] = "r1-sensitive-token-987", want "test-token"`. This is emitted by `matchFixtureQuery` / `compareFixtureValues` in `tests/replay/transport_test.go`; both query length mismatch and value mismatch formatting can print credential values.
- **Framing mismatch accepted independently.** In temporary `TestR1ProbeReplayAcceptsContentLengthAndTransferEncodingMismatch`, I set `request.ContentLength = 0` and `request.TransferEncoding = []string{"chunked"}` on the normal paired device-list request. `go test ./tests/replay -run '^TestR1ProbeReplayAcceptsContentLengthAndTransferEncodingMismatch$' -count=1 -v` passed: `RoundTrip` returned the fixture response despite the changed framing. Individual probes also showed acceptance for changed ContentLength, TransferEncoding, Close, RequestURI, and GetBody. The relevant framing fields alter how a real HTTP transport emits the body. `matchFixtureRequest` currently compares URL identity/query, headers, operation, and body, but not these request fields.
- The probe file was removed after each run. No tracked file in the source repository was changed. `GetBody`, aliases, and framing are separately checked by the wire inventory's production-constructor mutation tests; the finding here concerns what the paired replay validator itself accepts.

I also inspected the canonical `C:\Users\andre\work\portos\go-tplink\docs\independent-review.md` after the parent explicitly supplied that docs-only file. It contains the corrected R1 provenance text and both initial source-baseline reports. Its top-level status is OPEN and its summary lists the two report sets' source blockers, including replay diagnostics/framing. The snapshot checklist links this canonical file as the current review record. The R1/R2 statements that the document was withheld are accurate historical descriptions of their initial delivery boundary; I found no claim that the placeholder was source content. R2's item-13 PENDING was correct for its initial blind report; this R1 follow-up has now read the canonical record as the requested appendix audit. The record should retain the initial reports as historical evidence while making this item-15 correction explicit before final signoff.

**Current Reviewer 1 source-baseline disposition:** Items 2, 4, 5, 15, and 16 are OPEN at 804; item 14 remains OPEN. SDK/CLI publication and live Pages/proxy proof are pending separately. This is not final-source approval.

**Signed:** Reviewer 1, 2026-10-05 UTC.

## Reviewer 2 — initial all-16 audit

### Reviewer 2: independent checklist audit

**Reviewer:** Codex, independent reviewer 2; did not implement the change
**Audit date:** 2026-10-05 UTC
**Audited source:** go-tplink `804b98d1d4ada73fc1b8820646dfc5be4c47d281` (the isolated archive source verified by the parent manifest)
**Scope:** `docs/template-checklist.md` and the source tree at the audited SHA. This is the initial source-baseline report. I did not inspect `docs/independent-review.md`, other review reports, the original repository/history, or a later implementation snapshot, as instructed.

#### Verdict

Items 1, 3, 6, 8–11 pass on the audited source. Items 2, 4, 5, and 15 have open findings. Items 7, 12, and 16 pass their source requirements with publication-only evidence pending. Item 13 is pending because the current review record was intentionally not read and release publication has not occurred. Item 14 remains open until both reviewers have checked the final implementation and publication evidence. No item should be marked complete from this source-baseline audit while the listed findings and publication evidence remain open.

The source checkout was not edited or committed. Reproductions were performed in isolated TEMP copies. The task checkout is a fresh archive with a synthetic local Git identity; I relied on the parent-provided manifest for source SHA verification and did not fetch old objects or remotes.

#### Item-by-item evidence

##### 1. Independent public library — PASS

The public package is `pkg/tplink`; the README and published-guide sources describe a reusable TP-Link cloud client. I found no consuming-application adapter or rollout-specific integration in the public package, README, examples, or site sources. The repository has a standalone CLI under `cmd/go-tplink`, separated into its own module.

##### 2. Customer documentation and schema-validated examples — OPEN

The README and MDX guides cover supported operations, authentication, caller-supplied `AuthContext`, transport injection, errors, token renewal by explicit login, and secret handling. The API schema has request and response examples, and contributor docs distinguish synthetic fixtures from captures.

The required CI validation of every example against its owning schema is absent. `Makefile`, `.github/workflows/ci.yml`, and `.github/workflows/docs.yml` have no schema-example validator or target. The API schema includes inner JSON examples encoded in string fields such as `requestData`; merely generating and rendering the outer schema cannot validate those inner payload examples. This leaves example/schema agreement unproven.

##### 3. README badges — PASS

`README.md` has badges for Go version, CI, replay coverage, release, Go Reference, license, and documentation. Their targets use `portpowered/go-tplink`, the module, or the live Pages coverage/documentation paths; I found no leftover example repository value.

##### 4. Complete schemas, generated wire models, reference discoverability, and gates — OPEN

The source has an OpenAPI contract for the implemented cloud POST route and query token, generated dependency wire types/constants, public semantic projections, and separate schemas for authentication, devices, passthrough, and related models. `tools/wireinventory` is included in `make check` and CI, and its local default command passes. The source includes compile-valid negative controls for CLI/examples and nested enum cases. The checked-in inventory includes anonymous models.

The rendered Pages artifact for exact run `37255156075`, artifact `11321753722`, shows the request body as a discriminated outer request type. Its visible default is `LoginCloudRequest`; the accessibility tree also exposes a request-type combo box and a separate example selector. I therefore do not treat the default SSR selection alone as proof that other outer oneOf variants cannot be selected. I could not change that selector through the isolated subagent browser controls, so I make no stronger claim about its available options.

There is a definite nested-payload discoverability gap regardless of outer selector behavior. In `api/openapi.yaml`, `PassthroughParams.requestData` is a JSON-encoded `string` with `contentSchema: PassthroughCommand`; `PassthroughResult.responseData` is likewise a JSON-encoded `string` with `contentSchema: PassthroughCommandResult`. The rendered request field tree shows the login envelope (`method`, `params` / `LoginParams`) and does not render the nested `system.set_relay_state`, `system.get_sysinfo`, or lighting fields from `contentSchema`. The generated reference thus does not expose the known nested command parameters and result shape at the actual wire boundary as required. The guides partially explain these payloads, but that does not supply the required generated reference fields and snippets.

The independent example-validation gap in item 2 also prevents signoff for this item’s schema/example requirements. The route/model gate’s default Linux run passed, but the exact-commit Windows CI evidence is separately failing under item 5.

##### 5. Blocking all-linter CI and exact-commit checks — OPEN

`.golangci.yml` sets literal `linters.default: all`; CI pins golangci-lint `v2.14.0`, uses blocking action steps, and does not set an ignored issues exit code. On this Windows host, Go `1.26.8`, pinned linter `2.14.0`, and the provided wrapper (which inserts only `--allow-parallel-runners`), local `make lint` passed and `make check` completed its root and CLI lint/build/race/vet/tidy/format/coverage/inventory targets. Local replay coverage was `tplink` 269/286 (94.1%), `tplinkmodels` 78/81 (96.3%), cloud 81/81 (100%), combined 428/448 (95.5%); one generated file was excluded.

Exact-SHA CI run `37255156141` is not green: both Windows SDK verify jobs (Go 1.24 and 1.26) fail `TestWireInventoryDefaultCommandScansShippedModules` and `TestProductionWireInventoryPasses` with `anonymous generated model inventory is incomplete or stale`. Other listed CI jobs, including lint, CLI checks, Linux/macOS SDK checks, and schema generation, pass. A passing local run does not resolve the exact-commit blocking Windows failures. The separate docs PR build `37255156075` completed and produced the Pages artifact.

##### 6. Synthetic paired fixtures and measured coverage — PASS

`tests/replay/fixtures/synthetic` contains deterministic synthetic request/response pairs and fault/error outcomes; the fixture README documents their synthetic status and placeholder values. The replay gate measures public and transport package coverage plus combined non-generated production coverage, enforces 80%, and records a 90% target. The local `make check` metrics above exceed the target and identify the generated exclusion and uncovered statement totals.

##### 7. Package boundaries and consumer import — PASS; publication evidence pending

The source follows `pkg/tplink`, `pkg/dependencymodels`, `pkg/tplinkmodels`, and `pkg/dependencies/cloud`; API and generator outputs are split by responsibility, with compatibility/projection definitions separated from provider wire DTOs. A separate TEMP consumer module imported `github.com/portpowered/go-tplink/pkg/tplink`, constructed a client using `WithBaseURL`, closed it, and passed `go test ./...` with a local replace to the audited source.

The SDK/CLI 0.3 release and a consumer install against published tags are planned but unpublished at this source snapshot. The source-level import smoke does not replace the required published-version installation evidence.

##### 8. Functional options and validation — PASS

`pkg/tplink/client.go` exposes `NewClient(options ...Option)`, `WithHTTPClient`, and `WithBaseURL`; defaults and URL/transport validation are covered by tests. Credentials are not stored in client configuration.

##### 9. Account/stateless lifecycle and cookie isolation — PASS

The reusable client stores transport/base URL state, not account credentials or tokens. Login returns an explicit `LoginResult`; subsequent calls take request-scoped `AuthContext`. A standard injected `*http.Client` with a non-nil `Jar` is rejected with a configuration error, and accepted clients are snapshotted. Synthetic tests reuse a client for two account tokens and assert the outbound requests, including token placement and cookie isolation. No event/socket/RTC lifecycle is currently exposed.

##### 10. Injectable network edges — PASS for implemented scope

The inspected production scope uses HTTP cloud requests through the `HTTPDoer` seam, with `WithHTTPClient` injection. `tools/wireinventory` scans the shipped SDK, CLI, and example production modules and its default root command passes locally. I found no implemented WebSocket, MQTT, RTC, or socket edge requiring a separate connection hook. The audited pinned dependencies supply encoding/runtime helpers; no dependency-owned outbound edge was identified in the shipped call paths.

##### 11. Token renewal — PASS

The provider flow has no supported refresh operation in the checked-in contract. Login returns the token to the caller, subsequent operations accept it explicitly, and README, architecture, and authentication guides tell the caller to reauthenticate after expiry. No automatic refresh or token retention in the reusable client was found.

##### 12. Guides and rendered Pages links — PASS for source artifact; publication evidence pending

Customer guides are MDX under `docs/guides/` and link to the generated reference. Exact docs run `37255156075` produced the Pages artifact `11321753722`. I inspected the rendered root, guides, and API operation page; all 13 rendered `index.html` pages passed the internal-anchor target audit. The guide destinations and expected content were present. The schema has no `externalDocs` entries; the rendered API page’s external Go Reference link targets `pkg.go.dev`.

The run was a PR build with deployment skipped. The actual published Pages state for release 0.3 is therefore publication-only evidence still pending.

##### 13. Documentation review — PENDING

The README, architecture, protocol, fixture/testing, wire inventory, API/CLI contributor notes, and customer MDX guides were inspected, along with rendered pages. Their audience separation is generally clear: README for adoption, guides for users, inventories and fixture details for contributors.

I intentionally did not inspect `docs/independent-review.md` or any other review report before delivering this initial report, per the isolation instruction. Since item 13 requires every tracked documentation file to be reviewed, I cannot mark it complete in this initial pass. Published release-note copy/URLs also remain pending the planned publication.

##### 14. Two independent final approvals — PENDING / MUST REMAIN OPEN

This report is my independent source-baseline audit as reviewer 2. The checklist requires two separate reviewers to verify every fix and publication artifact at the final implementation commit. The audited source has open findings (items 2, 4, 5, 15), and publication evidence remains pending. Do not check item 14 until those conditions are resolved and both reviewers independently approve the final SHA. I have not inspected the deferred review record.

##### 15. Paired replay, secret-safe diagnostics, and HTTP framing — OPEN; reproduced

The replay suite pairs requests and responses, checks method/origin/path/query/headers/body, rejects unexpected exchanges, and has identity tests for URL userinfo, `Request.Host`, opaque URLs, and malformed queries. Existing client tests cover response-body read/close failures and transport-error response cleanup.

Two failures were reproduced in a TEMP-only copy at `C:/Users/andre/AppData/Local/Temp/tplink-review-repro`, containing the source `pkg` and `tests/replay` trees plus a new probe test; the audited checkout was not edited. Command used: `GOTOOLCHAIN=go1.26.8`, TEMP-scoped `GOMODCACHE`/`GOCACHE`, `go test ./tests/replay -run '^TestReviewProbe' -count=1`.

1. **Token leak on mismatch:** changing the actual token to `probe-actual-session-secret` makes replay fail with `request mismatch: query token[0] = "probe-actual-session-secret", want "test-token"`. The actual credential is copied into the diagnostic by `matchFixtureQuery` / `compareFixtureValues`.
2. **Framing mismatch accepted:** setting `request.ContentLength = 0` and `request.TransferEncoding = []string{"chunked"}` still receives the fixture response. The replay matcher does not compare these HTTP framing fields, even though they can change emitted body framing.

Both probes failed as expected: the first exposes a secret in the diagnostic, and the second shows an unexpected request is accepted. These are concrete item 15 blockers.

##### 16. Standalone customer CLI — PASS for source behavior; publication evidence pending

The CLI is in the separate `cmd/go-tplink` module, consumes the SDK, and has a customer guide. Source/tests cover login and auth failure, device enumeration, plug/bulb reads and controls, explicit credential export/logout, JSON output, documented env/stdin/file credentials, redaction, cancellation, and client cleanup. Exact CI run `37255156141` passes CLI Linux/Windows jobs and CLI lint. The documented routine device flow uses named commands and device IDs.

The provider’s implemented authorization is cloud username/password login; no browser/OAuth callback or PKCE contract was found, so those OAuth-specific clauses do not apply to this provider surface. `cmd/go-tplink/go.mod` currently requires SDK `v0.2.1`; the planned SDK/CLI 0.3 module tags and separate install from published CLI tags are not yet available. Treat those as publication-only evidence pending, not a source-level test result.

#### Publication-only evidence still pending

- SDK and CLI `0.3` tags/releases and a clean consumer installation from the published SDK and CLI modules.
- GitHub Pages deployment for the release snapshot; the audited docs result is a successful PR artifact build with deployment skipped.
- Final independent review of the exact release SHA after the current open findings are fixed.

**Signed:** Codex, independent reviewer 2 (initial all-16 source-baseline report), 2026-10-05 UTC.

## Deferred record audit and verdict corrections

Both reviewers inspected this actual record only after delivering their independent source-804 reports. Reviewer 1 independently reproduced the token leak and HTTP framing acceptance; its initial item 15 PASS is superseded by OPEN in the signed appendix above. Reviewer 2 confirmed that its initial item 16 source PASS was too broad: the missing ordered login/store/discovery/operation test found by Reviewer 1 keeps item 16 OPEN. Neither appendix approves the moving source or publication.
