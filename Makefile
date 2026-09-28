GO ?= go
PYTHON ?= python
export GOWORK := off
PUBLIC_MODULE ?= github.com/portpowered/go-tplink
PUBLIC_PACKAGES ?= pkg/tplink,pkg/tplinkmodels

.DEFAULT_GOAL := check
.PHONY: check build test lint fmt replay-coverage api-compatibility docs-check

check: lint build test replay-coverage

build:
	$(GO) build ./...

test:
	$(GO) test -race ./...

lint:
	$(GO) vet ./...

replay-coverage:
	$(GO) run ./tools/replaycoverage

fmt:
	$(GO) fmt ./...

api-compatibility:
	$(GO) run ./tools/compatibility -policy report -base previous-release -module "$(PUBLIC_MODULE)" -packages "$(PUBLIC_PACKAGES)"

docs-check:
	$(PYTHON) -m mkdocs build --strict
	$(PYTHON) tools/render_replay_coverage.py
