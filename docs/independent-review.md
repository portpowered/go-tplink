# Independent review record

Status: pending two independent reviews of every item in the
[current checklist](template-checklist.md) at the final implementation commit.
Record each reviewer's separate verdicts, evidence, findings, and dispositions
here, including exact CI, documentation, and published CLI installation results.

## Historical scope

Earlier reviews covered item 5 at `65c2626a1f88e1d275163552fefd817bdab01773`
and item 15 at `2298ea5`. Their records remain in Git history:
[lint review](https://github.com/portpowered/go-tplink/blob/b4a683e697a17f18e5b20d6a6c0adaf22c59e15e/docs/all-linter-review.md)
and [paired replay review](https://github.com/portpowered/go-tplink/blob/b4a683e697a17f18e5b20d6a6c0adaf22c59e15e/docs/paired-replay-review.md).
They do not certify expanded model-generation, documentation, or CLI requirements.

## Reviewer 1 — TP-Link full checklist review

**Reviewed implementation commit:** `7688030c012429bd16da752a0cf3e5696b31d739`.
**Checklist/standards baseline:** `go-third-party-template` commit
`987b9c34a6b927472c21604617b6842a4238746b`; reviewed the library, client design,
verification, website, and release standards as well as this repository's expanded
16-item checklist. These are Reviewer 1 dispositions at this SHA, not final checklist
sign-off; Reviewer 2 and any later implementation fixes still need verification.

**Build and publication evidence:** CI run
[37164564131](https://github.com/portpowered/go-tplink/actions/runs/37164564131)
completed successfully on the reviewed SHA. Its full-repository and standalone CLI
all-linter jobs use golangci-lint v2.14.0; build, race tests, vet, module/format checks,
schema generation, replay coverage, and the wire-inventory command also passed across
the configured Linux/macOS/Windows matrix. Documentation run
[37164564119](https://github.com/portpowered/go-tplink/actions/runs/37164564119)
completed its build and uploaded Pages artifact `11289430900`; its deploy job was
skipped for the pull request. I inspected the artifact's 14 HTML pages and checked
their local targets and fragments: no broken internal links were found, and the
rendered pages contained no external links. The current live site root returns 200,
but its `/go-tplink/docs/guides/cli/` route returns 404, so this candidate's guide is
not published. GitHub lists SDK `v0.2.1` as the latest release; the CLI tag
`cmd/go-tplink/v0.1.0` is absent (`gh api` returns 404 and `go list -m` reports an
unknown revision). No source files were changed by this review.

### Item verdicts

1. **PASS (Reviewer 1).** `pkg/tplink` is a standalone provider client, examples
   consume only its public package, and the customer guides describe generic caller
   responsibilities. A search of README, guides, SDK, examples, and CLI found no
   consuming-application adapter or rollout-specific dependency.

2. **PASS (Reviewer 1).** README examples and the MDX authentication, devices,
   errors, lighting, plugs, and CLI guides match the exported client operations.
   The guides explain request-scoped authentication and errors; `docs/architecture.md`
   documents `WithHTTPClient`; OpenAPI and fixture docs repeatedly label the contract
   and examples implementation-derived/synthetic rather than vendor-verified.

3. **PASS (Reviewer 1).** README provides Go-version, CI, replay-coverage, release,
   Go Reference, license, and documentation badges. The live coverage JSON, Pages
   root, and pkg.go.dev URLs returned HTTP 200 during this review. The repository and
   module values match `go.mod`.

4. **OPEN — F-R1-1 through F-R1-5.** The schema and written inventory cover the
   implemented single `POST /` cloud route, nested command JSON, responses, and
   generated constants, but the source gate is not a complete wire/model inventory.
   `tools/wireinventory.productionGoFiles` scans only `pkg/tplink`,
   `pkg/tplinkmodels`, and `pkg/dependencies`; it neither inspects the full generated
   model population nor verifies a schema-bound method/path at the actual
   `http.NewRequestWithContext`/`HTTPDoer.Do` call site. The gate tests do not add an
   unreferenced exported JSON struct, an anonymous nested wire object, or an
   unschematized route. `checkRawWireStrings` rejects only string literals already
   present in generated constants; its negative test uses the known value `"system"`,
   so a novel fixed string/key is accepted, and numeric/boolean literals such as a
   handwritten relay or delay value are not checked. Caller-open `SetLightState`
   tracking seeds only the parameter name `request` in `checkFunctionBody`; renaming
   that parameter bypasses the alias rule. `isOpenMapSource` does not recognize
   address-taking (`&request.State`), allowing a pointer alias and later indexed write
   to evade the mutation/escape checks. The current map gate also does not bind
   query/header map keys or method/path values to generated declarations at send.
   Finally, `.github/workflows/ci.yml` uses `git diff --exit-code` on fixed generated
   paths after generation, which misses newly untracked generated files.

   The independent model search found two omitted exported compatibility helpers:
   `dependencymodels.SendCloudRequestParams` and
   `SendCloudRequestJSONRequestBody`, re-exported from `pkg/generatedwire/compat.gen.go`,
   are absent from `docs/wire-model-inventory.md`'s complete inventory. The same
   inventory says all capability flags and relay states are referenced at their
   interpretation sites, but generated `DeviceCapabilityDisabled` and
   `DeviceStateOff` have no production use; only the enabled/on variants are used.
   Record these as unused exports/values or correct the usage claim. These missing
   proofs keep the item open even though the current schema generation and inventory
   commands pass.

5. **OPEN — F-R1-6.** The exact reviewed-commit CI run passed its blocking root and CLI
   full-linter jobs with `linters.default: all` and pinned v2.14.0, as well as build,
   race, and replay jobs. However, the pinned release standard requires the SDK tag
   workflow itself to rerun the full all-linter gate, generation/drift, endpoint
   inventory, and coverage before publication. `.github/workflows/release.yml`'s SDK
   `verify-and-publish` job runs compatibility, `go test -race`, replay coverage, vet,
   and a consumer check, but does not run the SDK all-linter, `go build`, schema
   generation/drift, or `tools/wireinventory`. No SDK release workflow has run on
   this candidate. Keep the release gate open until those required checks are covered.

6. **OPEN — F-R1-7.** `tools/replaycoverage` sets `-coverpkg` to only
   `github.com/portpowered/go-tplink/pkg/tplink`; the generated report says 94.0%, and
   CI enforces a 90% minimum for that package. `pkg/tplinkmodels` and
   `pkg/dependencies/cloud` are not measured separately or together with the public
   client, while the standard requires public and transport coverage and a combined
   non-generated total. The current number therefore does not establish the required
   coverage population or report its exclusions/uncovered behavior.

7. **OPEN — F-R1-8.** The required package boundaries exist (`pkg/tplink`,
   `pkg/dependencymodels`, `pkg/dependencies/cloud`, and separate public projections),
   and compatibility aliases are generated. The provider wire contract still puts
   login, device-list, and passthrough/feature payload responsibilities in one
   `api/openapi.yaml`, with a single generated `pkg/dependencymodels/models.gen.go`,
   rather than separate schemas/generated files by API responsibility as required by
   the pinned standard. The candidate SDK import path has not been tested from a
   clean consumer against this unpublished version: the release workflow consumer
   runs only on a future SDK tag, and the nested CLI currently pins public SDK v0.2.1.

8. **PASS (Reviewer 1).** `tplink.NewClient` takes functional options, provides
   defaults, and validates its HTTP client/base URL. `WithHTTPClient` and
   `WithBaseURL` are documented. Account credentials are supplied only to `Login`,
   rather than stored in reusable client options.

9. **PASS (Reviewer 1).** The reusable client stores no account credentials or
   session token; each operation receives `AuthContext`, and login returns the token
   in `LoginResult`. The only network lifecycle is caller-owned HTTP. `Close` marks
   the client unusable and does not retain or close the caller's transport. No event,
   socket, or other stateful connection operation is claimed.

10. **PASS (Reviewer 1).** The only active outbound network edge found is
    `pkg/dependencies/cloud.Send` calling the injected `HTTPDoer.Do`; callers install
    it through `WithHTTPClient`, and replay tests substitute offline transports.
    Production SDK searches found no WebSocket, MQTT, RTC, raw socket, or dependency-
    initiated network edge needing another injection seam.

11. **PASS (Reviewer 1).** `Login` explicitly exchanges email/password and returns
    the token; later calls require the caller to supply it through `AuthContext`.
    There is no automatic refresh or internal token update, and the authentication
    guide tells callers to log in again when a token expires.

12. **OPEN — F-R1-9.** Customer guides are MDX under `docs/guides/`, and the exact
    Docs run successfully rendered the API reference and guides. The artifact's 14
    HTML files had no broken internal targets in my check. The PR's Pages deploy job
    was skipped, and the currently live site returns 404 for the new CLI guide route;
    there is no publication proof for this candidate. Build success alone does not
    satisfy the publish requirement.

13. **OPEN — F-R1-10.** The README includes contributor evidence and maintenance
    material that the checklist assigns elsewhere: “Testing and evidence,” the
    make-command/coverage table, lint and CI implementation details, schema-generation
    and release-workflow explanation, and direct navigation to protocol, fixtures,
    and architecture notes. It also says the SDK uses golangci-lint v2.3.0 while
    current CI pins v2.14.0, installs SDK v0.1.0 while v0.2.1 is latest, and instructs
    `go install .../cmd/go-tplink@v0.1.0` although that CLI tag does not exist. This
    conflicts with the CLI guide's accurate “first release pending” status. The
    tracked docs include distinct customer MDX and contributor architecture,
    protocol, fixture, inventory, checklist, and review material; the README should
    remain customer-focused and those conflicting/stale claims should be corrected.

14. **OPEN.** This is the first independent section for SHA `7688030`; Reviewer 2's
    separate all-16 review is pending. Items 4–7 and 12–13, 15–16 have concrete open
    findings, so the two-reviewer/current-final-commit requirement is not met.
    Reviewer 2 and both reviewers must recheck any corrected final SHA.

15. **OPEN — F-R1-11.** The paired synthetic HTTP suite matches before returning the
    response, checks method/origin/escaped path, exact query value sets and body,
    rejects unpaired/extra calls, and checks full sequence consumption. It covers all
    public client operations and labels fixtures synthetic; the supported surface has
    no OAuth, CSRF, OTP, signaling, or socket flow. Two request-match gaps remain:
    `matchFixtureHeaders` iterates expected header names without rejecting additional
    actual headers, so adding a new authentication/security header does not fail the
    pair. Also, the OpenAPI route is `/` but every synthetic fixture stores `path: ""`;
    the matcher compares `URL.EscapedPath()` before the real HTTP transport normalizes
    an empty path to `/`. The stored path is therefore not the actual escaped wire
    path. Fix the header-set and root-path expectations before closing item 15.

16. **OPEN — F-R1-12.** `cmd/go-tplink` is a separate module with no local `replace`,
    its own full CLI lint/build/race/vet/module CI, and offline paired tests for
    authentication failure, commands, cancellation, and close. Its `go.mod` consumes
    public SDK v0.2.1. There is no published CLI tag or clean public install proof:
    GitHub returns 404 for `cmd/go-tplink/v0.1.0`, and
    `go list -m github.com/portpowered/go-tplink/cmd/go-tplink@v0.1.0` fails with
    `unknown revision`. The CLI README install command is consequently unusable.
    The current CLI guide correctly says the first release is pending; release/tag
    both modules in the required order and verify `go install` from the public proxy.

### Reviewer 1 finding disposition at 7688030

The following findings remain **OPEN** at the reviewed commit: F-R1-1 (incomplete
wire/model/route gate), F-R1-2 (no novel fixed primitive/key controls), F-R1-3
(caller-open map parameter/address escapes), F-R1-4 (untracked generated-file drift
and missing model/primitive inventory entries), F-R1-5 (route/query/header and
call-site provenance gaps), F-R1-6 (SDK release workflow omits required gates),
F-R1-7 (coverage scope), F-R1-8 (schema split and candidate consumer proof), F-R1-9
(candidate Pages publication), F-R1-10 (README audience/stale claims), F-R1-11
(paired header/path expectations), and F-R1-12 (published SDK/CLI evidence). None is
resolved by this review record or by the successful PR CI/Docs builds.

## Reviewer 2 — Independent audit at 7688030

**Reviewed implementation commit:** `7688030c012429bd16da752a0cf3e5696b31d739`.
**Checklist and standards baseline:** the checklist pins
`go-third-party-template` commit `987b9c34a6b927472c21604617b6842a4238746b`.
I independently inspected the public SDK, generated schemas/models, source gates,
paired replay code, CLI module, tracked Markdown/MDX files, CI and release workflows,
and the rendered documentation artifact.

**Exact-commit evidence:** CI run
[37164564131](https://github.com/portpowered/go-tplink/actions/runs/37164564131)
completed successfully on this SHA, including blocking root and CLI golangci-lint
v2.14.0 jobs and the configured build, race, vet, module, generation, inventory,
and replay checks. Documentation run
[37164564119](https://github.com/portpowered/go-tplink/actions/runs/37164564119)
also succeeded for this SHA and uploaded artifact `11289430900`; its PR deploy
job was skipped. I inspected the rendered artifact: internal page targets resolve,
but the generated API endpoint's visible main content does not show that the
contract is implementation-derived (the caveat is only in schema/page data).
The live site root returned HTTP 200, while
[the CLI guide route](https://portpowered.github.io/go-tplink/docs/guides/cli/)
returned HTTP 404. GitHub lists SDK `v0.2.1` as latest and no
`cmd/go-tplink/v*` tag exists. The checked-in CLI module requires public SDK
`v0.2.1`; no clean install from a published CLI version can be demonstrated.

### Item verdicts

1. **PASS (Reviewer 2).** The public client is in `pkg/tplink`; the CLI is a
   separate nested module and examples import the library package. I found no
   consuming-application adapter or rollout-specific dependency in the SDK,
   examples, or customer guides.

2. **PASS (Reviewer 2).** The authenticated README example and MDX guides cover
   login, request-scoped tokens, device discovery, plug and bulb workflows, and
   errors using the exported API. The architecture guide documents transport
   injection. The docs and fixture notes distinguish the implementation-derived
   contract and synthetic fixtures from vendor-verified behavior.

3. **PASS (Reviewer 2).** README includes Go version, CI, replay coverage, release,
   Go Reference, license, and documentation badges. The linked Go-version, CI,
   coverage JSON, release, pkg.go.dev, license, and Pages endpoints each returned
   HTTP 200 during this audit.

4. **OPEN — F-R2-1.** The written inventory covers the single cloud `POST /`
   route and known payloads, but it is not a complete generated model and
   call-site inventory. `tools/wireinventory` scans handwritten files only in
   `pkg/tplink`, `pkg/tplinkmodels`, and `pkg/dependencies`; it checks a list of
   required constants, raw literals already found in the generated constants,
   and selected map constructions. It does not enumerate all generated wire
   models or pair each outbound network primitive with its schema method/path at
   the actual send. For example, exported generated
   `dependencymodels.SendCloudRequestParams` and alias
   `SendCloudRequestJSONRequestBody` are missing from
   `docs/wire-model-inventory.md`. The generated
   `DeviceCapabilityDisabled` and `DeviceStateOff` constants have no production
   use, while the inventory says capability flags and relay states are referenced
   at their interpretation sites. The required model negative for an unreferenced
   exported handwritten JSON struct and anonymous nested wire object is absent.
   These gaps leave the item open despite the passing inventory command.

5. **OPEN — F-R2-2.** The exact pull-request CI run is green and its blocking
   full-repository lint configuration uses literal `linters.default: all` with
   pinned golangci-lint v2.14.0. The pinned release guide additionally requires
   the SDK tag workflow to rerun the full all-linter, build, schema drift,
   endpoint inventory, and coverage gates on the tag itself. In
   `.github/workflows/release.yml`, the SDK job runs compatibility, race tests,
   replay coverage, vet, and a public consumer check, but omits those release-time
   linter, build, generation/drift, and wire-inventory checks. No SDK release run
   exists for this candidate.

6. **OPEN — F-R2-3.** `tools/replaycoverage` sets `-coverpkg` to only
   `github.com/portpowered/go-tplink/pkg/tplink` and enforces a 90% threshold
   for that package. It does not measure `pkg/tplinkmodels` and
   `pkg/dependencies/cloud` separately and together with the public package, or
   report the combined non-generated coverage population and exclusions required
   by the standard. The reported 94.0% therefore does not establish the required
   combined 80% floor.

7. **OPEN — F-R2-4.** The package boundaries and separate public projection schema
   exist, but the provider wire schema keeps login, device-list, passthrough, and
   nested feature payloads together in `api/openapi.yaml`, and code generation
   emits one `pkg/dependencymodels/models.gen.go`. The pinned standard requires
   responsibility-specific schemas and generated Go files rather than this
   monolithic wire schema/model file. The missing generated model population noted
   in F-R2-1 also prevents a complete inventory.

8. **PASS (Reviewer 2).** `tplink.NewClient` accepts functional options, validates
   configuration, and supplies defaults. `WithHTTPClient` and `WithBaseURL`
   provide the documented injection/configuration points; credentials are passed
   to `Login`, not stored in client options.

9. **PASS (Reviewer 2).** The SDK returns the login token to the caller and
   requires an `AuthContext` for later calls. The client holds no account token,
   and `Close` only ends the local client lifecycle. The implemented surface has
   no event or socket session to expose or close.

10. **PASS (Reviewer 2).** The production network search found one active edge:
    `pkg/dependencies/cloud.Send` calls the injected `HTTPDoer.Do`, supplied
    through `WithHTTPClient`. Replay tests substitute offline transports. I found
    no additional WebSocket, MQTT, raw socket, or dependency-initiated network
    edge requiring an injection seam.

11. **PASS (Reviewer 2).** `Login` is an explicit credential exchange that
    returns the session token. Authenticated calls receive the caller-owned token
    through `AuthContext`. The SDK does not silently refresh or retain a token;
    the docs state that the provider surface has no refresh operation and callers
    must log in again after expiration.

12. **OPEN — F-R2-5.** The customer guides are MDX under `docs/guides/`, and the
    Docs artifact builds with no broken internal targets. However, guide pages do
    not link to the matching generated API reference page, as required. The PR
    deploy is skipped and the current live CLI guide route returns 404, so this
    candidate's guide is not published. The successful artifact build does not
    satisfy publication.

13. **OPEN — F-R2-6.** The README has accurate customer-facing SDK usage, but it
    also contains contributor material (“Testing and evidence,” the make-command
    table, replay coverage mechanics, lint/CI details, and schema/release workflow
    explanation). It advertises SDK install `@v0.1.0` although `v0.2.1` is
    latest, CLI install `@v0.1.0` although no nested CLI tag exists, and
    golangci-lint v2.3.0 although CI pins v2.14.0. In the rendered API reference,
    the endpoint's visible page content labels synthetic examples but does not
    display the contract's implementation-derived/not-vendor-verified status;
    that caveat is present only in embedded schema/page data. These are stale
    claims and evidence/audience gaps under the documentation standard. I inspected
    the tracked README, API README, examples README, contributor notes, checklist,
    review record, and all customer MDX guides; no additional standalone historical
    report is currently tracked.

14. **OPEN.** The record now contains separate R1 and R2 sections for this SHA,
    but the checklist does not link to `docs/independent-review.md`, and items
    4–7, 12–13, 15, and 16 remain open. The standard requires the item to stay
    unchecked while any finding or other checklist item is open, then requires
    both reviewers to recheck the corrected final commit.

15. **OPEN — F-R2-7.** The synthetic replay suite pairs requests and responses,
    matches method/origin/escaped path/query/body before returning responses, and
    checks response metadata and full sequence consumption. The matcher compares
    repeated query values in order and rejects extra calls. Two gaps remain:
    `matchFixtureHeaders` checks only names present in the expected header map,
    so an unexpected added header is accepted; and fixtures store `path: ""` for
    the schema route `/`, while the matcher compares the pre-transport
    `URL.EscapedPath()` directly. The replay evidence therefore does not assert
    the schema route's slash representation at the wire boundary. Both cases need
    exact paired expectations before item 15 can pass.

16. **OPEN — F-R2-8.** `cmd/go-tplink` is a standalone module that imports the
    public SDK, has blocking pinned all-linter/build/test/module CI, and uses
    offline paired command tests for authentication success/failure, read/control
    commands, cancellation, and cleanup. Its module has no local replacement.
    However, there is no published nested CLI tag or consumer install result;
    `git ls-remote` returns no `cmd/go-tplink/v*` tags. README's
    `go install ...@v0.1.0` command is therefore not installable, and the live
    CLI guide is unavailable. Keep the item open until the SDK is released first,
    the CLI tag is published, and `go install` succeeds from a clean consumer
    through the public proxy.

### Reviewer 2 finding disposition at 7688030

F-R2-1 (wire/model/network inventory), F-R2-2 (SDK release gates), F-R2-3
(coverage population), F-R2-4 (responsibility-specific schema/model layout),
F-R2-5 (published/reference-linked guides), F-R2-6 (README and visible evidence
copy), F-R2-7 (paired header/path assertions), and F-R2-8 (published CLI tag and
consumer install) remain **OPEN** at this exact SHA. Successful PR CI and Docs
artifact builds do not resolve these findings. No SDK/CLI source or release was
changed by this audit.
