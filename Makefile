GO ?= go
export GOWORK := off
PUBLIC_MODULE ?= github.com/portpowered/go-tplink
PUBLIC_PACKAGES ?= pkg/tplink,pkg/tplinkmodels

.DEFAULT_GOAL := check
.PHONY: check build test lint fmt replay-coverage api-compatibility generate-api

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

# oapi-codegen v2.8.0 requires Go 1.25+; GOTOOLCHAIN=auto downloads it when needed.
generate-api:
	$(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config pkg/generatedwire/config.yaml api/openapi.yaml
	$(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config pkg/tplinkmodels/config.yaml api/client-models.openapi.yaml
	$(GO) fmt ./pkg/generatedwire ./pkg/tplinkmodels
