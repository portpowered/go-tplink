GO ?= go
GOLANGCI_LINT ?= golangci-lint
export GOWORK := off

.DEFAULT_GOAL := check
.PHONY: check lint build test vet tidy-check format-check fmt

check: lint build test vet tidy-check format-check

lint:
	$(GOLANGCI_LINT) run --config .golangci.yml --allow-parallel-runners ./...

build:
	$(GO) build ./...

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

tidy-check:
	$(GO) mod tidy
	git diff --exit-code -- go.mod go.sum

ifeq ($(OS),Windows_NT)
format-check:
	@powershell -NoProfile -Command "$$files = Get-ChildItem -Path . -Recurse -Filter '*.go' | ForEach-Object { $$_.FullName }; $$unformatted = gofmt -l $$files; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }; if ($$unformatted) { Write-Output 'Unformatted Go files:'; $$unformatted; exit 1 }"
else
format-check:
	@unformatted="$$(gofmt -l *.go)" || exit $$?; \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted Go files:"; echo "$$unformatted"; exit 1; \
	fi
endif

fmt:
	$(GO) fmt ./...
