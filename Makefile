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

.PHONY: help fmt fmt-check test test-race vet build verify run infra-up infra-down clean

help:
	@echo "Panvara development commands"
	@echo "  make run        Run the lite profile"
	@echo "  make verify     Run the complete pull-request gate"
	@echo "  make infra-up   Start local PostgreSQL"
	@echo "  make infra-down Stop local PostgreSQL"

fmt:
	$(GOFMT) -w $(GO_FILES)

fmt-check:
	@test -z "$$($(GOFMT) -l $(GO_FILES))" || { echo "Go files need formatting:"; $(GOFMT) -l $(GO_FILES); exit 1; }

test:
	$(GO) test -shuffle=on -count=1 $(GO_PACKAGES)

test-race:
	$(GO) test -race -shuffle=on -count=1 $(GO_PACKAGES)

vet:
	$(GO) vet $(GO_PACKAGES)

build:
	mkdir -p bin
	$(GO) build -trimpath -o $(BINARY) ./cmd/panvara

verify: fmt-check vet test test-race build

run:
	$(GO) run ./cmd/panvara --profile=lite

infra-up:
	docker compose -f deploy/compose/compose.yaml up -d --wait

infra-down:
	docker compose -f deploy/compose/compose.yaml down

clean:
	rm -rf bin
