# Fixtures and testing

Normal tests are designed to run offline and without TP-Link credentials.
They exercise the client at the HTTP boundary and check provider request
encoding, response decoding, and typed errors.

## Fixture provenance

The earlier client test suite contained JSON response files with placeholder
account and device identifiers. They had no adjacent capture notes, collection
date, source, or redaction record. Their origin cannot be verified, so they
are classified as **synthetic**, not captured. They test expected wire shapes
and error handling, but they do not establish that a live TP-Link service
returned those bytes.

Synthetic fixtures belong under
`tests/replay/fixtures/synthetic/` and their names or neighboring notes should
keep that classification visible. Do not move these files into a `captured/`
directory or describe them as sanitized live captures.

The synthetic HTTP fixtures now pair each response with a request envelope:
method, origin, path, exact synthetic query token, relevant headers, operation,
and complete synthetic JSON request body variants. Tests select one request
variant for each expected step. The transport rejects a request outside that
pair before returning its response. Failure outcomes, including nil and broken
HTTP responses, have stored request and response halves in
`paired_outcomes.synthetic.json`. Tests must declare the expected sequence;
repeated operations require repeated steps. They reject extra calls and assert
that every declared step was consumed. The public-method inventory fails when
a client operation has no paired fixture. An independent item-15 review must
recheck this implementation at the final commit before sign-off.

If a future live or instrumented capture is added, keep it separately under
`tests/replay/fixtures/captured/`. Add a neighboring provenance note with the
operation, UTC capture date, source category, capture boundary, behavior
preserved, and redactions. Remove credentials, tokens, cookies, email addresses,
account identifiers, device IDs, MAC addresses, and other personal or
account-linked data before committing it.

## Local checks

Run these from the repository root:

| Purpose | Command | Credentials | Notes |
| --- | --- | --- | --- |
| Build | `make build` | No | Builds all module packages |
| Test | `make test` | No | Runs `go test -race ./...`; use only synthetic/offline test data |
| Format | `make fmt` | No | Runs `go fmt ./...` |
| Lint | `make lint` | No | Runs golangci-lint v2.3.0 with every linter enabled by `linters.default: all` |
| Combined check | `make check` | No | Runs lint, build, and test |
| API compatibility report | `make api-compatibility` | No | Compares exported API with the previous release when one exists |

The CI workflow installs golangci-lint v2.3.0 and runs the full-repository
lint command as a blocking job. The configuration does not disable
linters or filter findings through a baseline.

## Replay coverage

`make check` and CI run the offline replay suite against `pkg/tplink` with Go
statement coverage enabled. The gate requires at least 90% of client package
statements to execute; the profile is written to a temporary directory and
removed after the check. The documentation workflow generates a standalone
HTML report and Shields endpoint JSON from the same replay suite, then includes
both files in the GitHub Pages site.

There is no separate integration-test or live-test target: the module currently
has no live-account test harness, and ordinary checks must remain independent
of real accounts and devices. The documentation workflow builds the API
reference from `api/openapi.yaml` with the shared Fumadocs action. A separate
read-only device-list example requires user-provided credentials and is
documented in the README; it is not part of CI.
