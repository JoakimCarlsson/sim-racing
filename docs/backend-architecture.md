# Backend architecture

sim-racing organizes Go code **by subsystem (bounded context)**, not by technical layer. Each top-level directory under `internal/` is a domain slice with a flat package layout.

## Layout

```
sim-racing/
  cmd/
    server/main.go            # wiring only — env, store, http handler, sim loop, listen
    bake-track/main.go        # offline: produce track.height.bin from glTF
    laggy/main.go             # dev-only WS proxy with latency/jitter/loss
    physicswasm/main.go       # WASM build target for internal/physics
  migrations/                 # embedded .sql files applied at server boot
  assets/tracks/<id>/         # track.gltf, track.height.bin, track.json
  internal/
    health/                   # liveness ping (pure domain)
    realtime/                 # WS connection lifecycle, input ingestion, sanity gates
    protocol/                 # binary wire format (mirrored at web/src/protocol/)
    physics/                  # pure deterministic sim — compiles to WASM
    sim/                      # 60Hz tick loop, snapshot broadcaster, world state
    track/                    # track loader, heightmap, 2D limits, sector planes
    lap/                      # lap state machine, validity, persistence
    identity/                 # persistent anonymous player tokens
    store/                    # SQLite pool + embedded migration runner
    metrics/                  # Prometheus-format /metrics
    http/                     # minmux routes; one *_endpoint.go per subsystem
  web/                        # Bun + Vite + TS + three.js client
    src/protocol/             # TypeScript mirror of internal/protocol
    src/physics/              # WASM loader + JS bridge + parity test
    public/physics.wasm       # built artefact (gitignored)
  go.mod
```

## Rules

### Package by subsystem

- Top-level dirs under `internal/` are bounded contexts (e.g. `realtime`, `sim`, `lap`, `track`).
- Prefer **flat packages** with many `.go` files in one directory.
- Add a sub-folder only when a sub-feature has its own vocabulary and roughly **10+ files**.

### Forbidden layered layout

Do **not** introduce:

```
internal/
  controllers/
  services/
  repositories/
  models/
```

### Domain purity

- Domain packages (`internal/<subsystem>/`) contain business logic and types only.
- **No** `net/http` imports in domain packages.
- **No** WebSocket framing in domain packages — that lives in `internal/realtime`.
- **No** request/response DTOs tied to HTTP in domain packages.
- `internal/physics` is *strictly* pure: no I/O, no logging, no goroutines, no non-stdlib imports. It must compile to WASM and produce byte-identical state to its native build.

### HTTP adapters

- All HTTP wiring lives in `internal/http/`.
- One file per subsystem endpoint group: `*_endpoint.go` (e.g. `health_endpoint.go`, `lap_endpoint.go`, `realtime_endpoint.go`).
- Routes are registered with **[minmux](https://github.com/JoakimCarlsson/minmux)** (`github.com/joakimcarlsson/minmux/router`).
- The WS upgrade lives in `internal/http/realtime_endpoint.go`; after upgrade the connection is handed to `internal/realtime`.
- `cmd/server/main.go` stays thin: read config, open `store.DB`, run migrations, construct `sim.World`, call `http.NewHandler`, listen.

### Data access

- Per-subsystem SQL lives in `internal/<subsystem>/store.go`, not in a shared `repositories/` tree.
- Shared connection pooling and migration runner live in `internal/store/`.
- SQLite is the only database. Driver: `modernc.org/sqlite` (pure Go, no cgo).
- Migrations are embedded via `embed.FS` and applied at server boot, in numeric order, tracked in a `_migrations` table by filename.

### Sim and physics separation

- `internal/physics` exposes a single pure function: `Step(state, input, constants, dt) State`. Same code compiles to WASM for client-side prediction.
- `internal/sim` owns the running world: a map of `PlayerSim`, the 60Hz tick loop, and the 30Hz snapshot broadcaster. It calls `physics.Step` but never embeds physics math.
- `internal/sim` imports `internal/physics` and `internal/track`; it does **not** import `internal/realtime`. Connection lifecycle is delivered to `sim` via callbacks, so `sim` is testable headlessly.

### Static SPA

- Frontend source is under `web/` (Bun + Vite + TypeScript + three.js).
- Built assets go to `web/dist/`.
- The server serves the SPA via `router.SPA()` from `web/dist` once `web/dist/index.html` exists.
- Do **not** mount the SPA until that file is present — the router expects a valid index.

### minmux

- minmux is consumed via `go.mod` directly. No submodule.

## Adding a new subsystem

1. Create `internal/<name>/` with domain types and logic (no HTTP, no WS framing).
2. Add `internal/<name>/store.go` if the subsystem needs SQL, and a numbered migration under `migrations/`.
3. Add `internal/http/<name>_endpoint.go` to register minmux routes.
4. Wire dependencies in `http.NewHandler` — not in `main.go` beyond construction.

## Reference: health endpoint

`GET /healthz` is the template for every subsystem HTTP surface.

| File | Responsibility |
|------|----------------|
| `internal/health/health.go` | Domain logic — `Status()` returns `Result{OK, Version, UptimeSeconds}`; no HTTP imports |
| `internal/http/health_endpoint.go` | HTTP adapter — maps domain result to JSON DTO, registers `r.Get("/healthz", ...)` |
| `internal/http/handler.go` | Router assembly — global middleware (recover, logging, slog, CORS), calls every `register*` |
| `cmd/server/main.go` | Wiring only — env config, `store.Open(path)`, `sim.New(...)`, `http.NewHandler(deps)` |

Request flow:

```
GET /healthz
  → router middleware (Recover → logging → CORS)
  → healthHandler (internal/http)
  → health.Status() (internal/health)
  → JSON {"status":"ok","version":"...","uptime_seconds":N}
```

When adding a subsystem, mirror this split: domain package, `*_endpoint.go` registration, wire in `NewHandler`.

## Reference: realtime endpoint

`GET /ws` is the only WebSocket route. It is the **only** ingress for player inputs.

| File | Responsibility |
|------|----------------|
| `internal/realtime/connection.go` | Domain — `Conn` wraps a WS, owns reader/writer goroutines, the per-conn input ring buffer, and lifecycle hooks. No `net/http`. |
| `internal/realtime/sanity.go` | Validation gates: range, monotonic seq, rate limit, message size cap. Bad inputs increment a counter and may close the conn. |
| `internal/protocol/messages.go` | Wire-format (un)marshallers; pure, allocation-free in the hot path. |
| `internal/http/realtime_endpoint.go` | HTTP adapter — `r.Get("/ws", ...)` performs the WS upgrade and hands the connection to `internal/realtime`. |
| `internal/sim/world.go` | Subscribes to realtime's `AddPlayer` / `RemovePlayer` hooks; drains per-conn input queues each tick. |

Invariants:

- The client→server channel carries only the message types defined in `internal/protocol`. There is no field through which a client can send a position, velocity, or claimed lap time.
- Server→client snapshots are the only authoritative state. Clients render predicted state for local feel but reconcile against snapshots.
- The wire format is binary, little-endian, packed, versioned by a single byte after the type tag.

See [docs/realtime-protocol.md](realtime-protocol.md) for the byte layout.
