# AGENTS.md — sim-racing contributor and agent contract

Read this document **first** before planning, implementing, or reviewing sim-racing work. It is the canonical source for architecture rules and mandatory verification.

## Project summary

sim-racing is a **persistent multiplayer time-trial racing game**: a **Go server** (minmux router, SQLite, WebSocket realtime) and a **Bun + Vite + TypeScript + three.js** client under `web/`. Players connect to a single always-on track, drive laps, and chase the leaderboard. Other cars are visible as **ghost cars** (no collisions).

The server is **fully authoritative**: clients send inputs only, never positions. The simulation runs in Go on the server and as **Go-compiled-to-WASM** on the client for prediction. The same physics binary produces byte-identical state on both sides, so lap times cannot be forged.

## Hard rules — verification (non-negotiable)

1. **Everything observable must be verified end-to-end.** Unit tests alone are not enough for HTTP APIs, WebSocket flows, or the running server.
2. **New or changed HTTP endpoints:** start the server (`go run ./cmd/server`, `docker compose up`, or equivalent), then **`curl` every new/changed route** — assert status code and response body match acceptance criteria.
3. **New or changed WebSocket behaviour:** drive a synthetic client (`websocat`, or a small Go/TS smoke client) and assert the binary frames and ordering.
4. **New or changed physics:** re-run the determinism golden hash (`go test -run TestReplayDeterminism -count=100 ./internal/physics/`) AND the Go↔WASM parity test. A physics change without a regenerated, reviewed golden is not done.
5. **Delivery agents** follow `.claude/agents/` role definitions and the verification commands documented per subsystem.
6. **Coder** must list every new/changed route, WS message type, and physics-affecting change in `HANDOFF:IMPLEMENTATION` → `smoke_endpoints` / `smoke_steps` with concrete `expect` values.
7. **Smoke-tester** must not mark pass without live evidence when the issue touches HTTP, WS, or physics.

### Smoke example

Start the server:

```bash
go run ./cmd/server
# or: docker compose up --build
```

Verify health:

```bash
curl -s http://localhost:8080/healthz
```

Expected: `{"status":"ok","version":"...","uptime_seconds":N}` (HTTP 200).

Verify the realtime channel:

```bash
websocat ws://localhost:8080/ws
# Server sends a binary ServerHello frame on connect (msg type 0x03).
```

Verify a track-bound flow (e.g., a leaderboard route after laps exist):

```bash
curl -s 'http://localhost:8080/api/tracks/circuit01/leaderboard?limit=10' | jq
```

## Hard rules — architecture (Go)

Package **by subsystem (bounded context)**, not by technical layer. Top-level directories under `internal/` are domain slices. Prefer **flat packages** with many `.go` files; add sub-folders only when a sub-feature has its own vocabulary and roughly **10+ files**.

**Do not introduce:**

```
internal/
  controllers/
  services/
  repositories/
  models/
```

Full detail, health-endpoint template, and minmux notes: **[docs/backend-architecture.md](docs/backend-architecture.md)**.

| Rule | Detail |
|------|--------|
| Domain purity | `internal/<subsystem>/` — business logic only; no `net/http`, no HTTP request DTOs, no WS framing |
| HTTP | `internal/http/*_endpoint.go` per subsystem; routes via [minmux](https://github.com/JoakimCarlsson/minmux) |
| WebSocket | WS upgrade in `internal/http/realtime_endpoint.go`; connection lifecycle owned by `internal/realtime` |
| SQL | `internal/<subsystem>/store.go` — not `internal/repositories/` |
| Shared DB | `internal/store/` — SQLite pool and embedded migration runner only |
| Physics | `internal/physics/` is **pure**: no I/O, no logging, no goroutines, no non-stdlib imports; compiles to WASM |
| Protocol | `internal/protocol/` owns wire format; mirrored at `web/src/protocol/messages.ts` |
| `main.go` | Wiring only — env, store, `http.NewHandler`, listen |

When adding a subsystem: domain package → optional `store.go` → `http/<name>_endpoint.go` → wire in `NewHandler`. Mirror `internal/health` + `health_endpoint.go`.

## Repo layout

| Path | Purpose |
|------|---------|
| `cmd/server/` | Server entrypoint (HTTP + WS + sim loop) |
| `cmd/bake-track/` | Track-asset baker (heightmap from glTF) |
| `cmd/laggy/` | Dev-only latency/jitter/loss WS proxy |
| `cmd/physicswasm/` | WASM build target for `internal/physics` |
| `internal/health/` | Liveness ping |
| `internal/realtime/` | WS connection lifecycle, input ingestion, sanity gates |
| `internal/protocol/` | Wire-format messages (binary, little-endian) |
| `internal/physics/` | Pure deterministic sim (`Step(state, input, c, dt)`) — compiles to WASM |
| `internal/sim/` | 60Hz tick loop, snapshot broadcaster, world state |
| `internal/track/` | Track loader, heightmap, 2D limits polygon, sector planes |
| `internal/lap/` | Lap state machine, validity, persistence |
| `internal/identity/` | Persistent anonymous player tokens |
| `internal/store/` | SQLite pool + embedded migration runner |
| `internal/metrics/` | Prometheus-format `/metrics` |
| `internal/http/` | One `*_endpoint.go` per subsystem; minmux routes |
| `migrations/` | Embedded `.sql` migrations applied at boot |
| `assets/tracks/<id>/` | Track assets: `track.gltf`, `track.height.bin`, `track.json` |
| `web/` | Bun + Vite + TS + three.js client |
| `web/src/protocol/` | TypeScript mirror of `internal/protocol` |
| `web/src/physics/` | WASM loader + JS bridge + parity test |
| `web/public/physics.wasm` | Built artefact (gitignored; produced by `scripts/build-wasm.sh`) |
| `docker-compose.yml` | Local server + Caddy (for TLS testing) |
| `deploy/Caddyfile` | Reverse-proxy + auto-TLS config |
| `docs/backend-architecture.md` | Backend architecture reference |
| `docs/protocol.md` | Wire-format byte layout |
| `docs/physics.md` | Physics model, determinism policy, WASM build |
| `docs/track-format.md` | `track.json` schema |
| `docs/security.md` | Threat model + anti-cheat invariants |

## Clone

```bash
git clone https://github.com/JoakimCarlsson/sim-racing.git
cd sim-racing
```

minmux is consumed via `go.mod` directly (no submodule).

## How to run

### Docker Compose (server + Caddy)

```bash
cp .env.example .env
docker compose up --build
```

- Server: [http://localhost:8080](http://localhost:8080) (`PORT` overrides)
- Caddy fronts TLS in production; locally it can be skipped

### Local server

```bash
go run ./cmd/server
```

- Listens on `:8080` by default (`PORT`)
- SQLite file path via `DB_PATH` (default `./sim.db`)
- Allowed Origins via `SIM_ALLOWED_ORIGINS` (comma-separated; required in prod)
- Log format via `LOG_FORMAT` (`text` for dev, `json` for prod)

### Frontend dev server

```bash
cd web
bun install
bun run dev
```

Vite runs on [http://localhost:5173](http://localhost:5173). Point at a non-default backend with `VITE_SERVER_WS_URL=ws://localhost:9090/ws bun run dev` (e.g., when using `cmd/laggy`).

### Build the WASM physics

```bash
./scripts/build-wasm.sh
```

Produces `web/public/physics.wasm` (plus `wasm_exec.js` if standard Go). The Go↔WASM parity test (`bun test parity` in `web/`) refuses to pass unless this binary matches the Go-native golden hash.

## Dev tooling (Makefile)

Run **`make install` once** before `make lint` (installs `golangci-lint`, `goimports`, `golines` into `$(go env GOPATH)/bin`).

| Target | Purpose |
|--------|---------|
| `make install` | Install Go lint/format tools (once) — includes `go install github.com/golangci/golines@latest` |
| `make backend` | `go build ./...` |
| `make backend-test` | `go test ./...` |
| `make backend-vet` | `go vet ./...` |
| `make backend-fmt` | `gofmt -w` + `golines -m 80 -w` over every Go package |
| `make backend-lint` | `golangci-lint run ./...` |
| `make web-install` | `bun install` in `web/` |
| `make web-fmt` | Prettier write in `web/` |
| `make web-lint` | ESLint in `web/` |
| `make web-typecheck` | `tsc --noEmit` in `web/` |
| `make web-build` | Vite production bundle |
| `make wasm` | Build `internal/physics` to `web/public/physics.wasm` |
| `make fmt` | `backend-fmt` + `web-fmt` |
| `make lint` | `backend-lint` + `web-lint` |
| `make dev` | Run server + frontend dev server in parallel |
| `make dev-laggy` | Run server behind `cmd/laggy` (simulated latency/jitter/loss) |
| `make ci` | `backend-vet` + `backend-test` + `backend-lint` + `web-typecheck` + `web-lint` + `web-build` + `wasm` + parity test |
| `make help` | List all targets |

Migrations are embedded in the server binary via `embed.FS` and applied automatically on boot; there is no separate migration CLI.

`golangci-lint` uses **v2** config (`.golangci.yml`, `version: "2"`).

## Agent workflow

Delivery pipeline: **planner → red-team → coder → smoke-tester → reviewer** (see `.claude/agents/`). Every handoff between stages is a typed YAML block conforming to **[docs/pipeline-handoff-schema.md](docs/pipeline-handoff-schema.md)** — that schema is the contract, not the prose in any single agent file.

| Resource | Purpose |
|----------|---------|
| `.claude/agents/` | Agent role definitions |
| `.claude/commands/pipeline.md` | Orchestrator — validates handoffs, hashes failure signatures, enforces token budget, writes `.pipeline-runs/<issue>/<run-id>.jsonl` |
| `docs/pipeline-handoff-schema.md` | Canonical HANDOFF YAML contract for all five blocks (PLAN, IMPLEMENTATION, VERIFIED, FIX, APPROVED) |
| `docs/pipeline-observability.md` | `run.jsonl` row schema + recipes |

### Structured plan contract

Every plan emits a `HANDOFF:PLAN` block with **stable AC ids** (`AC1`, `AC2`, …), `files_touched[]`, `interfaces[]`, `test_cases[]` (≥1 per AC), `non_goals[]`, and `assumptions[]`. The coder refuses to start if any AC lacks a mapped `test_case`. The reviewer's `HANDOFF:APPROVED` must include `spec_conformance[]` with `status: MET` and a `file:line` `evidence` cite for every AC id.

### Enforced lessons

`LEARNINGS.md` is appended on every clean PR. When the same lesson appears twice or more, the reviewer **promotes** it to this file AND, where the lesson is mechanical, also creates a deterministic enforcement artifact in the same fix cycle:

| Lesson shape | Enforcement artifact |
|---|---|
| "Don't import X from Y" | `golangci-lint` rule or `scripts/` grep pre-commit |
| "Always assert response body, not just status" | smoke-tester test helper |
| "Forgot to register route in NewHandler" | integration test that fails on missing route |
| "Physics change without golden refresh" | `make ci` runs `TestReplayDeterminism -count=100` and the Go↔WASM parity test |
| "Client sent a position field" | protocol-level lint: `web/src/protocol/messages.ts` forbids non-input fields client→server |

Prose lessons rot. Enforced lessons compound. Always pair the promotion with the artifact.

## Security and anti-cheat

Server is the only source of truth. Inviolable invariants:

- **Clients send inputs only.** No position, velocity, lap time, or "I crossed the finish line" field exists in any client→server message.
- **Server runs the only authoritative physics.** Client prediction is cosmetic.
- **Every accepted lap stores its input stream** for server-side replay verification.
- **Sanity gates on every input** before it reaches the simulator (range, monotonic seq, rate limit).
- **Per-IP connection rate limit** and per-connection inbound size cap on the WS upgrade.
- **WSS only in production.** `SIM_ALLOWED_ORIGINS` enforced.
- **Token hash stored, never the cleartext token.** Logs never contain a token.

Threat model and non-goals are documented in `docs/security.md`.

Do **not** commit secrets, `.env` files with real credentials, or API keys. Use `.env.example` for documented placeholders.

## Quick reference

```bash
make install                          # once
make fmt && make lint
make web-install
make wasm                             # build physics.wasm
make ci                               # full gate
go test ./...
go test -run TestReplayDeterminism -count=100 ./internal/physics/
```
