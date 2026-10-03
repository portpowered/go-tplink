GO ?= go
GOLANGCI_LINT ?= golangci-lint
export GOWORK := off
PUBLIC_MODULE ?= github.com/portpowered/go-tplink
PUBLIC_PACKAGES ?= pkg/tplink,pkg/tplinkmodels

.DEFAULT_GOAL := check
.PHONY: check build test vet lint fmt format-check tidy-check replay-coverage api-compatibility generate-api

check: lint build test vet tidy-check format-check replay-coverage

build:
	$(GO) build ./...
	$(MAKE) -C cmd/go-tplink build

test:
	$(GO) test -race ./...
	$(MAKE) -C cmd/go-tplink test

lint:
	$(GOLANGCI_LINT) run ./...
	$(MAKE) -C cmd/go-tplink lint

vet:
	$(GO) vet ./...
	$(MAKE) -C cmd/go-tplink vet

tidy-check:
	$(GO) mod tidy
	git diff --exit-code -- go.mod go.sum
	$(MAKE) -C cmd/go-tplink tidy-check

ifeq ($(OS),Windows_NT)
format-check:
	@powershell -NoProfile -Command "$$files = git ls-files -- '*.go'; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }; $$unformatted = gofmt -l $$files; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }; if ($$unformatted) { Write-Output 'Unformatted Go files:'; $$unformatted; exit 1 }"
	$(MAKE) -C cmd/go-tplink format-check
else
format-check:
	@unformatted="$$(gofmt -l $$(git ls-files -- '*.go'))" || exit $$?; \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted Go files:"; echo "$$unformatted"; exit 1; \
	fi
	$(MAKE) -C cmd/go-tplink format-check
endif

replay-coverage:
	$(GO) run ./tools/replaycoverage

fmt:
	$(GO) fmt ./...
	$(MAKE) -C cmd/go-tplink fmt

api-compatibility:
	$(GO) run ./tools/compatibility -policy report -base previous-release -module "$(PUBLIC_MODULE)" -packages "$(PUBLIC_PACKAGES)"

# oapi-codegen v2.8.0 requires Go 1.25+; GOTOOLCHAIN=auto downloads it when needed.
generate-api:
	$(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config pkg/generatedwire/config.yaml api/openapi.yaml
	$(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml
	$(GO) fmt ./pkg/generatedwire ./pkg/tplinkmodels
