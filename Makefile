# Panvara
# Makefile    2026-07-14
#
# @link    : https://github.com/shezw/panvara
# @author  : shezw
# @email   : hello@shezw.com

GO ?= go
GOFMT ?= gofmt
BINARY := bin/panvara
GO_PACKAGES := ./...
GO_FILES := $(shell find . -type f -name '*.go' -not -path './.git/*')

.PHONY: help fmt fmt-check test test-race test-integration test-server-smoke test-e2e vet build verify run run-server infra-up infra-down clean

help:
	@echo "Panvara development commands"
	@echo "  make run               Run the Lite profile"
	@echo "  make run-server        Run Server from exported PANVARA_* variables"
	@echo "  make verify            Run the Docker-free pull-request gate"
	@echo "  make test-integration  Require PostgreSQL 18.4 integration tests"
	@echo "  make test-server-smoke Require the Server HTTP persistence smoke test"
	@echo "  make test-e2e          Run all required alpha.2 integration tests"
	@echo "  make infra-up          Start local PostgreSQL"
	@echo "  make infra-down        Stop local PostgreSQL"

fmt:
	$(GOFMT) -w $(GO_FILES)

fmt-check:
	@test -z "$$($(GOFMT) -l $(GO_FILES))" || { echo "Go files need formatting:"; $(GOFMT) -l $(GO_FILES); exit 1; }

test:
	$(GO) test -shuffle=on -count=1 $(GO_PACKAGES)

test-race:
	$(GO) test -race -shuffle=on -count=1 $(GO_PACKAGES)

test-integration:
	PANVARA_REQUIRE_DOCKER=1 $(GO) test -tags=integration -shuffle=on -count=1 ./tests/integration

test-server-smoke:
	PANVARA_REQUIRE_DOCKER=1 $(GO) test -tags=integration -shuffle=on -count=1 -run '^TestServerProfileHTTPPersistenceLifecycle$$' ./cmd/panvara

test-e2e: test-integration test-server-smoke

vet:
	$(GO) vet $(GO_PACKAGES)

build:
	mkdir -p bin
	$(GO) build -trimpath -o $(BINARY) ./cmd/panvara

verify: fmt-check vet test test-race build

run:
	$(GO) run ./cmd/panvara --profile=lite

run-server:
	@test -n "$$PANVARA_ADMIN_TOKEN" || { echo "PANVARA_ADMIN_TOKEN must be exported; generate at least 32 random bytes" >&2; exit 1; }
	$(GO) run ./cmd/panvara --profile=server

infra-up:
	docker compose -f deploy/compose/compose.yaml up -d --wait

infra-down:
	docker compose -f deploy/compose/compose.yaml down

clean:
	rm -rf bin
