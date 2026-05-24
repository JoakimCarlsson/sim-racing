# Makefile — sim-racing dev tooling
# Targets: backend, backend-test, backend-vet, backend-fmt, backend-lint,
#          frontend, frontend-typecheck, frontend-fmt, frontend-lint,
#          fmt, lint, dev, ci

GOPATH_FWD ?= $(shell go env GOPATH)
GOLINES    := $(GOPATH_FWD)/bin/golines
GO_PKG_DIRS := $(shell go list -f '{{.Dir}}' ./...)

.PHONY: backend backend-test backend-vet backend-fmt backend-lint \
        frontend frontend-typecheck frontend-fmt frontend-lint \
        fmt lint dev ci

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

# ---------------------------------------------------------------------------
# Frontend
# ---------------------------------------------------------------------------

frontend:
	cd web && bun run build

frontend-typecheck:
	cd web && bun run typecheck

frontend-fmt:
	cd web && bun run format

frontend-lint:
	cd web && bun run lint

# ---------------------------------------------------------------------------
# Aggregates
# ---------------------------------------------------------------------------

fmt: backend-fmt frontend-fmt

lint: backend-lint frontend-lint

dev:
	@echo "Starting server and frontend dev server in parallel…"
	go run ./cmd/server & cd web && bun run dev

ci: backend-vet backend-test backend-lint frontend-typecheck frontend-lint frontend
