# sim-racing

A persistent multiplayer time-trial racing game. A Go server (minmux router, SQLite, WebSocket realtime) and a Bun + Vite + TypeScript + three.js client. Players connect to a single always-on track, drive laps, and chase the leaderboard. Other cars appear as ghost cars (no collisions). The server is fully authoritative; clients send inputs only, never positions.

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | >= 1.26 | https://go.dev/dl/ |
| Bun | latest | https://bun.sh |
| golangci-lint | latest | https://golangci-lint.run/welcome/install/ |
| golines | latest | `go install github.com/golangci/golines@latest` |

## Quickstart

```bash
# Clone
git clone https://github.com/JoakimCarlsson/sim-racing.git
cd sim-racing

# Install frontend dependencies
cd web && bun install && cd ..

# Start server + frontend dev server in parallel
make dev
```

The server listens on `:8080` by default. The frontend dev server listens on `:5173`.

## Make targets

| Target | What it does |
|--------|-------------|
| `make backend` | `go build ./...` |
| `make backend-test` | `go test ./...` |
| `make backend-vet` | `go vet ./...` |
| `make backend-fmt` | `gofmt -w` + `golines -m 80 -w` over every Go package |
| `make backend-lint` | `golangci-lint run ./...` |
| `make frontend` | Vite production bundle (`cd web && bun run build`) |
| `make frontend-typecheck` | `tsc --noEmit` |
| `make frontend-fmt` | Prettier write over `web/src` |
| `make frontend-lint` | ESLint over `web/src` |
| `make fmt` | `backend-fmt` + `frontend-fmt` — format everything |
| `make lint` | `backend-lint` + `frontend-lint` — lint everything |
| `make dev` | Server + frontend dev server in parallel |
| `make ci` | Full gate: vet, test, lint (backend + frontend), build |

### Common workflows

```bash
# Format all code
make fmt

# Lint everything (non-zero exit if any violation)
make lint

# Run the full CI gate locally
make ci

# Verify nothing changed after formatting
make fmt && git diff --exit-code
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DB_PATH` | `./sim.db` | SQLite file path |
| `SIM_ALLOWED_ORIGINS` | (none) | Comma-separated allowed WebSocket origins (required in prod) |
| `LOG_FORMAT` | `text` | Log format: `text` (dev) or `json` (prod) |
| `VITE_SERVER_WS_URL` | `ws://localhost:8080/ws` | Override WebSocket URL for frontend dev |

## Architecture

- **Go server** — `cmd/server/`, `internal/` — bounded-context packages
- **Frontend** — `web/` — Bun + Vite + TypeScript + three.js
- **Physics** — `internal/physics/` — pure deterministic sim, compiles to WASM

See [docs/backend-architecture.md](docs/backend-architecture.md) for full architecture notes.

## Links

- [Backend architecture](docs/backend-architecture.md)
- [Wire protocol](docs/protocol.md)
- [Security model](docs/security.md)
- [Track format](docs/track-format.md)
