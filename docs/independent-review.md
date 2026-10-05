# Independent review record

Status: **open; fixes and two final independent approvals are required**.

These repair reports audit `7b1742fb0efbfe441f31c3a0317e63584988f516`.
The current [checklist](template-checklist.md) pins shared template
`585b5677caebd4e6d58db215e5278d35a4a7aadf`.
The reports include separate audits of that standards clarification.

## Findings being resolved

- A caller-supplied value can bypass a closed wire enum constraint.
- An alias to the cloud send helper can bypass actual target validation.
- Request body backing bytes can change after construction.
- A network call in a shipped example can evade the root inventory gate.
- An injected shared cookie jar can transfer cookies between accounts.
- Contributor documentation has stale configuration and check descriptions.

The second reviewer disclosed seeing historical review snippets before delivering
this report. Its reproduced findings are repair evidence; a replacement blind
reviewer will provide final approval. Prior reports remain in Git history.
The first reviewer completed a blind all-item audit and its post-delivery
document appendix. Neither this record nor passing CI closes any unresolved
checklist item.

## First review: independent findings

# Blind checklist audit — go-tplink PR 1 (Reviewer 1)

**Reviewed source:** `7b1742fb0efbfe441f31c3a0317e63584988f516`
**Canonical review:** [PR 1](https://github.com/portpowered/go-tplink/pull/1)
**Review date:** 2026-10-04
**Scope:** independent read-only audit of all 16 checklist items against the frozen source. The source checkout used for probes was a clean detached TEMP clone at the reviewed SHA. I made no source changes and did not read `docs/independent-review.md` before delivering this initial report.
**Reviewer:** Reviewer 1 — independent Codex agent

## Findings affecting merge eligibility

- **F1 — Item 4, closed enum provenance:** the wire gate accepts caller-controlled `LoginCloudRequestMethod` values in the closed `LoginCloudRequest.Method` enum. The schema permits only `login`; an arbitrary value of the defined string type compiles and is emitted. The committed caller-positive probe explicitly blesses this case.
- **F2 — Item 4, request body backing bytes:** the request mutation gate tracks the `http.Request` and its URL/header/body fields, but not the byte slice retained by `bytes.NewReader`. A valid mutation of `bodyBytes` after request construction compiles and passes the default inventory gate.
- **F3 — Item 4, shipped example scan:** `productionGoFiles` scans SDK packages and the nested CLI, but omits the shipped `examples/list-devices` source. A compile-valid unregistered `http.Get` added there compiles and passes the default inventory gate. A corresponding CLI injection is rejected by that gate, but there is no committed CLI-specific negative that locks this behavior through the default root command.
- **F4 — Items 2, 9 and 15, injected cookie state:** `WithHTTPClient` accepts a reusable `*http.Client` with a shared `CookieJar` without rejecting it or isolating cookie state. Its cookies can cross account calls to the same cloud origin. The architecture guide says a client can be shared across accounts, without this transport-state qualification, and the replay fixtures do not assert two-account cookie isolation.

These findings mean the frozen implementation is **not merge eligible** under the checklist. Passing CI does not resolve them.

## Checklist verdicts

1. **PASS — application-independent SDK.** The reusable API is under `pkg/tplink`; the read-only example in `examples/list-devices`, README, and customer guides describe the provider client generically. No consuming-application adapter or rollout plan appears in the public client/docs.

2. **PARTIAL — user documentation matches most of the API, but the account-sharing statement is incomplete.** README and MDX guides describe the exported `Login`, `AuthContext`, client options, supported operations, typed errors, token handling, and synthetic evidence. The authentication guide includes a compiling-shape `WithBaseURL`/`WithHTTPClient` example. However, `docs/architecture.md:20` says a client can be shared across accounts, while `pkg/tplink/client.go:87-125` accepts an HTTP client carrying shared account cookies without an isolation rule. See F4.

3. **PASS — README badges.** README contains Go version, CI, replay coverage, release, Go Reference, license, and documentation badges. Targets resolve to the repository workflows/metadata, pkg.go.dev, license, Pages, and coverage report. The current coverage artifact reports 95.5%.

4. **FAIL — API, wire inventory, provenance, and network-edge gates.** Positive evidence: the OpenAPI schemas split the single implemented cloud `POST /` route and its login/device/passthrough payloads; the CLI contracts and public projection have separate schemas; nested command payloads and known keys are generated; `docs/wire-model-inventory.md` inventories route, models, nested fields and primitive values; model and request tests cover anonymous/unreferenced handwritten JSON shapes, callback and named-result provenance, sibling-package helpers, recursive and long helper chains, aliases, URL user information, `GetBody`, content length, transfer encoding, clones and helper escapes. CI regenerates schemas and runs the Fumadocs API action. No other active non-HTTP network edge or network-using third-party dependency was found in the shipped root/CLI modules.

   Concrete failures:
   - **F1:** `api/authentication.openapi.yaml:18-21` constrains `LoginCloudRequest.Method` to `login`. `tools/wireinventory/package_provenance_test.go:88-121` accepts `BuildFromCaller(value dm.LoginCloudRequestMethod)` and forwards that value into `dm.LoginCloudRequest{Method: ...}`; lines 233-259 repeat a caller-positive through a named-result helper. `tools/wireinventory/scalar_package_provenance.go:305-313` treats every parameter of an exported function as caller-safe. This permits a caller-supplied string such as `"invented"` at a closed schema enum field.
   - **F2:** `pkg/dependencies/cloud/cloud.go:66-83` marshals into `bodyBytes`, passes it to `bytes.NewReader`, then sends the request. In the clean TEMP clone, I inserted `bodyBytes[0] = 'x'` after request construction and before `Do`. `go test -run '^$' ./pkg/dependencies/cloud` compiled; the exact default root command `go run ./tools/wireinventory` exited 0. No after-construction backing-byte negative or safe immutable-body positive exists.
   - **F3:** `tools/wireinventory/main.go:172-206` scans `pkg/...` and `cmd/go-tplink`, not `examples/list-devices`. I added a compile-valid temporary example function that called `http.Get("https://unregistered.invalid")`; `go test ./examples/list-devices` compiled it and `go run ./tools/wireinventory` exited 0. The equivalent CLI probe exited 1 with “HTTP constructor or convenience request is outside the registered cloud transport.” That demonstrates CLI inclusion, but no committed CLI-specific negative exercises it through the root command.

5. **PASS — blocking all-linter CI and offline checks.** `.golangci.yml` uses literal `linters.default: all`; CI and release workflows pin golangci-lint v2.14.0 and run full root and CLI modules. The exact-head PR checks show build, lint, CLI lint, root/CLI verify matrices (Linux, Windows, macOS as configured), API compatibility, and schema generation all successful. No continue-on-error or zero issues exit-code bypass is configured. Root `Makefile` default is `check`, which runs lint, build, race tests, vet, module metadata, formatting, replay coverage, and wire inventory; it also delegates build/test/lint/vet/metadata/format to the CLI module.

6. **PASS — deterministic paired replay and coverage.** Replay fixtures are explicitly synthetic; the fixture README says no live-capture provenance is claimed. Pair tests assert request matching before response, request/body/query/header fields, status and response, call order and exhaustion. Coverage is measured over public client, projection, and HTTP transport packages, excludes generated files, enforces 80% and targets 90%; exact documentation artifact coverage is 95.5%.

7. **PASS — package/schema boundaries and public import path.** Provider wire contracts are in `pkg/dependencymodels`, transport in `pkg/dependencies/cloud`, client in `pkg/tplink`, and semantic projections in `pkg/tplinkmodels`; CLI local models have their own schema/generated file. The complete inventory maps schema components and call sites. I created a separate TEMP consumer module with a local replace to this frozen SDK and imported `pkg/tplink` and `pkg/tplinkmodels`; `go test ./...` passed, including `ClientInterface`, options and token model references. Public proxy verification for a future tagged SDK is a post-merge release gate, not evidence available from this PR artifact.

8. **PASS — explicit options and defaults.** `NewClient` installs the default base URL and `http.DefaultClient`; `WithHTTPClient` rejects nil implementations, and `WithBaseURL` validates scheme, host, userinfo, query and fragment. Account credentials/tokens are supplied per operation rather than stored in client configuration.

9. **FAIL — injected CookieJar account isolation.** `pkg/tplink/client.go:53-60, 87-125` defaults to `http.DefaultClient` and stores any provided `HTTPDoer` directly. `pkg/dependencies/cloud/cloud.go:42-60` calls `Do` without handling cookie state. Reusing `http.Client{Jar: sharedJar}` lets response cookies from one account be sent on a later account’s request to the same endpoint. No distinguishable configuration error, explicit per-account cookie session, or two-account complete-request test exists. Stateless custom transports remain injectable.

10. **PASS — applicable network edges are injectable.** The shipped SDK has one active HTTP cloud endpoint; no WebSocket, MQTT, RTC, event, or raw-socket edge is present. The client exposes the `HTTPDoer` seam and paired tests use injected doers. Network-primitive gate controls cover convenience HTTP calls, `Do`, method values/expressions and raw `net.Dial`. CookieJar state is separately tracked under item 9.

11. **PASS — explicit token lifecycle.** `Login` returns a `LoginResult` containing the opaque token; authenticated operations take request-scoped `AuthContext`. The docs say there is no supported refresh operation and callers must authenticate again when the provider expires a token. The client does not silently retain updated tokens.

12. **PASS for the PR build; publication pending merge.** The docs workflow uses the shared Fumadocs action on the schemas and `docs/guides`; exact PR artifact run 37231606453 succeeded through build, replay report and Pages artifact upload. Its deployment job is skipped on a pull request. The artifact contained all expected route pages and the generated `POST /` reference. I checked 133 local page links across rendered HTML, including site root and generated reference: zero broken internal links. The OpenAPI files contain no `externalDocs` links. The upgrade guide route used by release notes exists. Main Pages publication remains post-merge proof.

13. **PENDING — post-delivery appendix required.** I reviewed README, examples README, API README, architecture, protocol, fixture/testing docs, synthetic fixture README, wire inventory, checklist, and all guide MDX for audience, purpose, evidence labels, duplicate links and concise scope. README is focused on install, short authenticated example, capabilities, configuration/lifecycle and user-guide links; inventory, fixtures, coverage and generation detail stay in maintainer docs. The task instruction withheld inspection of `docs/independent-review.md` until this initial blind report was delivered. I will append its audience/staleness assessment after delivery; this is not a finding that item 13 is blocked.

14. **OPEN — no overall sign-off.** This audit is one independent reviewer, F1–F4 remain open, and the checklist itself requires two reviewers to verify every item at the final implementation commit after fixes. Do not check item 14 from this report.

15. **PARTIAL — paired synthetic HTTP exchanges are strong, with cookie-isolation gap.** `tests/replay/transport_pair_test.go` and synthetic JSON fixtures capture expected request and response halves, match before response, reject unexpected calls, and assert sequence exhaustion. CLI integration tests also use paired requests for login, errors, device workflows, cancellation, and cleanup. No test stores a response cookie then asserts full outbound requests for two accounts sharing the injected client, so the current supported `CookieJar` behavior is not verified; see F4.

16. **PASS for CLI implementation; published-proxy proof pending.** `cmd/go-tplink` is a separate nested module, uses the public SDK API, has help, JSON reads, nonzero errors, cancellation, explicit commands for changes, secret input options, protected token storage/export, and cleanup. CLI integration tests exercise paired HTTP exchanges including auth failure and client close; CI has pinned lint/build/race/test/vet/format/tidy checks for the CLI across its configured matrix. The MDX guide documents checkout use now and states the first standalone release is pending. The release workflow has a later tag job that installs the published module through the Go proxy and checks `--help`; that final install/release proof has not run at this pre-merge SHA.

## Implementation merge status and final publication proof

**Implementation merge eligibility at frozen SHA: NO.** F1–F4 are unresolved checklist findings despite all exact-head CI checks passing. Re-audit the fixes at the final implementation SHA with both independent reviewers.

**Final publication proof: PENDING.** The Pages deploy job is skipped on PRs; SDK/CLI tags and public Go proxy consumer/install checks only run after the main merge and tag events.

---

**Signed:** Reviewer 1 — independent Codex agent
**Reviewed commit:** `7b1742fb0efbfe441f31c3a0317e63584988f516`



## Post-delivery appendix — review-record audience and staleness

After delivery of the initial blind report, I read `docs/independent-review.md` from the same immutable TEMP clone at the reviewed SHA. This supersedes item 13's initial “pending appendix” note with the verdict below; the file was not used to form the earlier blind findings.

- **Audience and purpose:** the file is maintainer review evidence, linked from `docs/template-checklist.md` and `docs/fixtures-and-testing.md`, and is not included in the customer Pages navigation. It keeps separate reviewer sections, exact reviewed commit identifiers, check run/artifact evidence, item findings, and dispositions. It is the single tracked independent-review record; prior scoped reports are linked as historical Git references, not duplicated as tracked documents.
- **Staleness:** both review sections explicitly identify implementation commit `7688030c012429bd16da752a0cf3e5696b31d739` and the top status says the record is pending two independent reviews at the final implementation commit. Their reported CI run, 94.0% coverage, unpublished CLI route, findings, and checklist observations are therefore historical evidence for that SHA. They must not be presented as current evidence for `7b1742fb...`: this audit independently verified that the coverage population, schema/model inventory, CLI-guide/API link, and checklist link changed by 7b. The record should append the two new final-SHA audits and their finding dispositions before item 14 is checked; keeping the old findings is appropriate as history.
- **Item 13 update:** **PASS for documentation audience, purpose, and retention.** The review record is maintainer-facing, its historical evidence is commit-scoped, and it is not a duplicate customer document. The absent final-SHA reports are a separate item 14 completion requirement and do not keep item 13 blocked. The exact 7b record still needs the final reports appended before overall review sign-off.

**Appendix signed:** Reviewer 1 — independent Codex agent; inspected only the immutable `7b1742fb0efbfe441f31c3a0317e63584988f516` clone.

## Second review: repair evidence

# Independent checklist audit — go-tplink

**Reviewer:** Codex agent /root/tplink_7b_r2 (R2)
**Commit:** 7b1742fb0efbfe441f31c3a0317e63584988f516
**Method:** Read-only audit in clean detached temp clone. No repository source edits. All negative probes below compiled in that clone, executed, then removed; clone returned clean at the reviewed SHA. Exact-SHA CI was already green, so I did not repeat the full baseline suite.

## Protocol exception

Before this initial report was delivered, a broad rg command included docs and inadvertently printed portions of docs/independent-review.md. The source review and reproductions below were independently developed; I did not rely on, cite, or incorporate that report. Strict blindness was compromised. I disclosed this to the coordinating reviewer. This report is useful repair evidence, but is **not final independent sign-off**; a replacement blind reviewer will be used after fixes.

## Verdicts

1. **PASS** — Public SDK and docs are independent of a consuming application.
2. **PASS** — Supported operations, auth, errors, injection, and evidence status are documented consistently.
3. **PASS** — README contains Go, CI, coverage, release, Go Reference, license, and docs badges linked to live project endpoints.
4. **FAIL** — Source gate has bypasses; examples omitted; anonymous nested wire objects not individually inventoried. Details below.
5. **PASS** — Exact-SHA blocking CI and pinned literal-all lint configuration passed.
6. **PASS** — Deterministic synthetic paired fixtures and 95.5 percent combined non-generated coverage (423/443) exceed threshold and target.
7. **FAIL** — Required anonymous-object model inventory is incomplete; package boundaries otherwise match.
8. **PASS** — Functional options, defaults, HTTP injection, and base URL validation are present.
9. **FAIL** — A shared injected HTTP CookieJar leaks one account's cookie into another account's requests.
10. **PASS for implemented edges** — SDK HTTP edge is injected; no other supported SDK/dependency network edge found.
11. **OPEN** — Login returns credentials explicitly and client does not rotate/store them, but no explicit token-refresh operation or supported refresh contract exists.
12. **PASS for artifact / publication pending** — MDX guides and generated reference rendered with valid internal links; verify Pages deployment after merge.
13. **OPEN** — Complete audience/incoming-link appendix is pending; do not call blocked merely because the report was withheld.
14. **OPEN** — Findings remain; both reviewers must verify fixes at final SHA.
15. **PASS for supported HTTP exchanges** — Synthetic request/response pairs validate requests and call order.
16. **OPEN for release proof; implementation merge eligible** — CLI is separate and tested, but first nested module tag and consumer installation remain pending.

## Detailed evidence

### 1 — PASS

Reusable API is in pkg/tplink; device transport and models are separately packaged. Examples and the standalone CLI do not introduce an application adapter or rollout plan into the public SDK. README and Pages explain library usage.

### 2 — PASS

Customer guides cover authentication and supported device workflows using exported API shapes. WithHTTPClient and caller-provided AuthContext are documented. The API/site title says implementation-derived; synthetic examples and historical/captured evidence are distinguished. I found no provider-guarantee overclaim in the reviewed customer content.

### 3 — PASS

README has the seven required badges: Go version, CI, replay coverage, release, Go Reference, license, and docs. Targets use this repository's live endpoints; no template repository values remain. README focuses on install, a short authenticated example, capabilities, caller obligations, and guide links.

### 4 — FAIL

**Mutable request body backing storage bypass.** pkg/dependencies/cloud/cloud.go newRequest marshals into bodyBytes and passes bytes.NewReader(bodyBytes) to http.NewRequestWithContext (around lines 65–89). In the temp clone I inserted bodyBytes[0] = 'X' after request construction and before Do. Since the reader aliases the backing array, the emitted body can change. Both GOTOOLCHAIN=go1.26.8 GOWORK=off go build ./... and default-root go run ./tools/wireinventory passed. The gate lacks the addendum-required mutable-slice negative and immutable-body positive controls.

**cloud.Send function-value alias bypass.** tools/wireinventory/main.go:255–293 counts direct cloud.Send calls; tools/wireinventory/model_gates.go:1185+ resolves the direct call. I added var aliasSend = cloud.Send, left a dead direct call under if false so the direct-call count stayed satisfied, and sent the live request through aliasSend with &url.URL{Scheme:"https", Host:"attacker.invalid"}. Build and default root wireinventory both passed. Existing tests exercise direct-call receiver/authority substitution but not local or file-scope method-value aliases.

**Example outbound edge excluded.** tools/wireinventory/main.go:175–184 enumerates SDK, dependency, generated-wire, and CLI directories but omits examples. A compile-valid http.Get("https://attacker.invalid") in examples/list-devices/main.go passed go build ./... and default root wireinventory. The same probe in cmd/go-tplink/main.go is rejected by the root command. The addendum requires all shipped modules, including examples, in source inventory.

**Anonymous nested generated objects absent from the complete model inventory.** pkg/dependencymodels/passthrough.gen.go has anonymous nested structs in command DTOs including SystemGetSysInfoCommand, SystemRebootCommand, SystemSetDevAliasCommand, SystemSetRelayStateCommand, and lighting commands. docs/wire-model-inventory.md records named parent models and scalar enums but not each inline object with schema owner, generated declaration, generator, and conversion/use site. Named-component validation does not establish the required anonymous-object population.

**Docs build evidence.** The docs workflow uses portpowered/api-docs-website-github-action@v0.3.0, api/openapi.yaml, docs/guides, and /go-tplink. Exact-SHA docs build succeeded. I inspected the downloaded artifact, including root, guides, and API reference, and checked internal rendered anchors; no broken internal targets were found. PR deploy skip is expected. This does not cure the source-gate failures or substitute for post-merge publication proof.

Required controls: mutable body alias rejected and immutable body accepted; function-value cloud.Send alias plus dead direct-call bypass rejected; example network edge rejected by default root command; anonymous model entries verified against schema, generated Go, and actual use.

### 5 — PASS

Exact-SHA CI run 37231606460 succeeded (SDK verify matrix, schema generation, CLI verify, lint, compatibility). Docs run 37231606453 built the artifact; deploy was skipped for the PR. Both lint configs say literal linters.default: all; CI pins golangci-lint v2.14.0, runs full repository, and does not zero lint exit status, continue after failures, or limit to new issues. Root make check covers lint/build/race tests/vet/tidy/format/replay coverage/wire inventory and CLI checks; CLI has a separate blocking check. I found no intentional persisted JSON-key migration in this snapshot, so missing regression lock for such a migration is not a present style-fix blocker.
Links: https://github.com/portpowered/go-tplink/actions/runs/37231606460 and https://github.com/portpowered/go-tplink/actions/runs/37231606453.

### 6 — PASS

tests/replay/transport_pair_test.go checks paired request matching before response return, order/exhaustion, mismatches, and unexpected or duplicate calls. Fixtures are labeled synthetic. CI coverage report: combined non-generated 95.5 percent (423/443); pkg/tplink 94.0 percent; pkg/tplinkmodels 96.3 percent; cloud 100 percent. One generated file is excluded and identified.

### 7 — FAIL

Package locations meet the requested separation: pkg/tplink public SDK, pkg/tplinkmodels semantic projection, pkg/dependencymodels provider wire models, pkg/dependencies/cloud HTTP behavior. Schemas and generated files are split by responsibility, compatibility aliases/projections are used, and CI includes public API compatibility/consumer verification. However the inventory does not list anonymous nested generated wire structs individually. This violates the complete model inventory requirement despite the correct architecture.

### 8 — PASS

NewClient(options ...Option) sets default HTTP client and base URL. WithBaseURL validates HTTP(S), host, and rejects user info/query/fragment. WithHTTPClient accepts HTTPDoer and rejects nil-like clients. Tests cover invalid configuration. Credentials are per-operation AuthContext, not reusable client options. CookieJar leak is item 9.

### 9 — FAIL

Client stores reusable transport/base URL and close state; caller supplies auth. But WithHTTPClient accepts *http.Client unchanged, including a shared mutable Jar. Runtime temp-clone probe used one SDK client with http.Client{Jar: cookiejar.New(nil)}. Fake account-A login set account-session=account-A. A later GetDevices request for account B carried token-B and account-A's cookie:

    second account token="token-B" cookie="account-session=account-A"
    COOKIE_CROSSED_ACCOUNTS

The addendum requires rejecting a shared unsafe jar with a distinguishable error or explicit isolated session cookie storage, and testing two accounts through the same reusable client.

### 10 — PASS for current implementation

Only SDK network edge found is cloud.Send calling injected HTTPDoer.Do; WithHTTPClient installs the doer and replay tests substitute offline transports. No SDK websocket/MQTT/RTC/raw-socket edge or active network call in pinned dependencies was found. The example-scan omission is a separate item-4 inventory defect.

### 11 — OPEN

Login is explicit and returns AuthContext; protected calls receive it from the caller. Client does not retain/rotate credentials. No refresh operation is implemented and checked-in schemas do not establish provider refresh support. Because checklist item 11 literally requires token exchange and refresh operations, sign-off needs either supported refresh evidence and an explicit API or an agreed contract that refresh is unsupported. Existing docs correctly leave token storage/renewal to the caller, but do not prove the explicit refresh requirement.

### 12 — PASS for artifact; publication proof pending

Customer guides are MDX under docs/guides and link to generated reference pages. Exact-SHA Fumadocs artifact renders expected guide/API content and internal links resolve. Static schema review found no externalDocs link left unchecked. PR artifact is not main Pages deployment proof; inspect after merge.

### 13 — OPEN

README and guides appear focused; contributor inventories and fixture details are separated from customer navigation. This item requires every tracked doc, duplicate/internal-doc and incoming-link review, plus rendered site and release-note review. Complete that appendix after initial report delivery. Do not mark blocked solely because independent-review.md had been withheld.

### 14 — OPEN

Findings in items 4, 7, 9 remain; item 11 and 13 remain open. Both reviewers need separate evidence for all 16 and must recheck the final implementation SHA. This report is not a final two-reviewer sign-off, also due to the disclosure above.

### 15 — PASS for supported HTTP exchanges

Synthetic replay tests match method/full URL/body/relevant headers before paired response, reject mismatches/extras, and enforce ordered consumption where needed. Error, CLI auth, and cancellation paths use injected transports. No capture provenance is claimed and no other supported transport requires transcript handling. Add a specific two-account CookieJar regression under item 9.

### 16 — OPEN for final release proof; implementation merge eligible

cmd/go-tplink is a separate module consuming the SDK. Its module has pinned all-linter/build/race-test/vet/module checks. Offline tests cover auth failure, discovery/control, output, cancellation, and cleanup using paired injected transports. Credentials use prompt/environment/stdin/file inputs; export is explicit; secrets are redacted; device changes require commands. Customer CLI guide is MDX and says first standalone CLI release is pending. Implementation is merge eligible after source/model/account-isolation fixes; first CLI nested-module tag and separate consumer installation must follow SDK/CLI release.

## Separate addendum audit

I separately audited the parent-provided 23-line clarification at template main commit 585b5677caebd4e6d58db215e5278d35a4a7aadf (not the frozen target checklist source):
- Mutable body backing-buffer protection: FAIL, reproduced above.
- All shipped modules/examples included in network source inventory: FAIL, reproduced above.
- Injected CookieJar account isolation: FAIL, reproduced above.
- The CLI unregistered-call probe is rejected by the actual default root gate, but the test suite lacks a compile-valid CLI network mutation that runs that exact root/default command. Add an end-to-end negative test so later scope changes cannot bypass root CI.

## Disposition

**Not merge eligible yet** because of confirmed wire-gate bypasses, omitted example sources, incomplete anonymous-object inventory, and cross-account cookie leakage. Re-run affected default-root gate tests and account-isolation tests after fixes. Resolve item 11 refresh contract. After implementation blockers are fixed, merge eligibility is distinct from post-merge Pages deployment and post-tag CLI consumer proof. I did not merge or release.

**Signed:** Codex agent /root/tplink_7b_r2 (R2), 2026-10-04


## Post-delivery documentation audience and freshness appendix (item 13)

After delivering the initial report, I reviewed all tracked Markdown/MDX documentation in the clone, including docs/independent-review.md, README, api/README, examples/README, contributor guides, every customer guide, and the synthetic-fixture README; I also checked the rendered artifact targets and release workflow URLs.

Audience separation is mostly sound: protocol, fixture provenance, architecture, schema inventory, checklist, and review evidence are contributor material; operation/authentication/device/CLI/error guides are MDX customer pages; README is focused on install, authenticated use, capabilities, caller obligations, and guide links. The checklist links the review record at docs/template-checklist.md:10, and contributor fixture notes link to it too. The CLI guide accurately says the first standalone release is pending and places its go install @latest instruction under “After publication”; it gives a checkout command for the interim. The release workflow’s notes link targets the existing upgrading guide route. The docs artifact build and internal target check passed.

**Concrete item-13 freshness issue:** docs/architecture.md says that a client can be shared across accounts without changing shared token state. At the reviewed SHA, WithHTTPClient accepts a standard http.Client with a shared CookieJar, and my item-9 runtime probe demonstrates account-A cookies sent with account-B token. That customer-facing claim is too broad and should either be corrected or made conditional on cookie-jar isolation; resolving item 9 should include this doc.

**Review-record staleness:** docs/independent-review.md contains R1/R2 sections for 7688030..., not this reviewed 7b1742... SHA. The SHA labels make those historical results identifiable, and the file’s top status says two final-commit reviews are pending, but no current-commit disposition matrix is present. Several statements are stale against this tree: the older reports say the checklist does not link to the review record (the current frozen checklist does); they describe pre-split schemas and old coverage scope, whereas api/README and docs/fixtures-and-testing.md now describe separate schemas and combined coverage; and they repeat old README/install/lint claims that current README and CLI guide no longer make. Keep those sections clearly historical and, when recording current reviews, add explicit per-finding resolution/supersession dispositions instead of carrying old “open” items forward as current.

**Small accuracy issue:** docs/fixtures-and-testing.md’s make check table says it “Runs lint, build, and test,” although root Makefile check additionally runs vet, tidy, format, replay-coverage, wire-inventory, and CLI checks. This is not a broken link or customer-guide issue, but should be updated to avoid under-describing the command.

Verdict 13 remains OPEN pending correction of the cross-account architecture claim with the item-9 fix and a current-review-record update after final reviewers. No additional broken internal rendered targets were found. The documentation audit does not make item 13 blocked merely because the current report had initially been withheld.
