# Makefile — sim-racing dev tooling
# Targets: backend, backend-test, backend-vet, backend-fmt, backend-lint,
#          frontend, frontend-typecheck, frontend-fmt, frontend-lint,
#          wasm, parity, fmt, lint, dev, ci

GOPATH_FWD ?= $(shell go env GOPATH)
GOLINES    := $(GOPATH_FWD)/bin/golines
GO_PKG_DIRS := $(shell go list -f '{{.Dir}}' ./...)

.PHONY: backend backend-test backend-vet backend-fmt backend-lint \
        frontend frontend-typecheck frontend-fmt frontend-lint \
        wasm parity \
        web-install web-fmt web-lint web-typecheck web-build \
        fmt lint dev ci help install

# ---------------------------------------------------------------------------
# Backend
# ---------------------------------------------------------------------------

backend:
	go build ./...

backend-test:
	go test ./...

backend-vet:
	go vet ./...

backend-fmt:
	gofmt -w $(GO_PKG_DIRS)
	$(GOLINES) -m 80 -w $(GO_PKG_DIRS)

backend-lint:
	golangci-lint run ./...

install:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golines@latest

# ---------------------------------------------------------------------------
# Frontend (aliases and canonical targets)
# ---------------------------------------------------------------------------

frontend:
	cd web && bun run build

frontend-typecheck:
	cd web && bun run typecheck

frontend-fmt:
	cd web && bun run format

frontend-lint:
	cd web && bun run lint

web-install:
	cd web && bun install

web-build: frontend

web-fmt: frontend-fmt

web-lint: frontend-lint

web-typecheck: frontend-typecheck

# ---------------------------------------------------------------------------
# WASM build and parity test
# ---------------------------------------------------------------------------

## wasm: compile internal/physics to web/public/physics.wasm
wasm:
	bash scripts/build-wasm.sh

## parity: run the Go<->WASM parity test (requires web/public/physics.wasm)
parity: wasm
	cd web && bun test src/physics/parity.test.ts

# ---------------------------------------------------------------------------
# Aggregates
# ---------------------------------------------------------------------------

fmt: backend-fmt frontend-fmt

lint: backend-lint frontend-lint

dev:
	@echo "Starting server and frontend dev server in parallel…"
	go run ./cmd/server & cd web && bun run dev

## ci: full gate — vet, test, lint, typecheck, build, wasm, parity
ci: backend-vet backend-test backend-lint frontend-typecheck frontend-lint frontend wasm parity

## help: list all targets
help:
	@grep -E '^##' Makefile | sed 's/^## //'
