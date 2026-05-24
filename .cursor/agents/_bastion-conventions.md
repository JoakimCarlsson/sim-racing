# Bastion conventions (all delivery agents)

Read repo-root **`AGENTS.md` first**, then `docs/backend-architecture.md`, then **`docs/pipeline-handoff-schema.md`** (canonical HANDOFF contract — every agent in the chain conforms to it), before planning or implementing.

## Pipeline shape

The full chain is **planner → red-team → coder → smoke-tester → reviewer**. The red-team agent walks every `assumptions[]` entry in the plan; on `RED-TEAM:REFUTED` the plan goes back to the user (ambiguity gate), not to the coder. Cursor uses `handoffs:` frontmatter to fire each next stage automatically.

## Verification — mandatory E2E

1. **Everything observable must be verified end-to-end.** Unit tests alone are not enough for HTTP APIs or running services.
2. **New or changed API routes:** start the API (`go run ./cmd/api`, `docker compose up`, or `commands_to_verify.serve`), then **`curl` every new/changed endpoint** — record status and body; compare to acceptance criteria.
3. **Smoke-tester** must not mark pass without live endpoint evidence when the issue touches HTTP.
4. **Coder** must list every new/changed route in `HANDOFF:IMPLEMENTATION` → `smoke_endpoints` with concrete `expect` values.
5. Use `.cursor/verify-commands.md` for project-specific commands.

## Architecture (Go)

Package **by subsystem (bounded context)**, not by layer. Top-level dirs under `internal/` are domain slices. Flat packages with many `.go` files; sub-folders only when a sub-feature has its own vocabulary and ~10+ files.

**Forbidden:**

```
internal/
  controllers/
  services/
  repositories/
  models/
```

**Layout:**

```
bastion/
  cmd/api/main.go
  deps/minmux/              # git submodule
  migrations/
  docs/backend-architecture.md
  internal/
    health/                 # pure domain
    http/                   # minmux routes, static SPA
    store/                  # DB pool + migrate
  web/                      # Bun + React SPA — all frontend here
```

| Rule | Detail |
|------|--------|
| Domain purity | `internal/<subsystem>/` — no `net/http`, no HTTP request DTOs |
| HTTP | `internal/http/*_endpoint.go` per subsystem; minmux routes |
| SQL | `internal/<subsystem>/store.go` — not `internal/repositories/` |
| Shared DB | `internal/store/` — pool + migrate only |
| `main.go` | Wiring only — env, pool, `http.NewHandler`, listen |

When adding a subsystem: domain package → optional `store.go` → `http/<name>_endpoint.go` → wire in `NewHandler`. Mirror `internal/health` + `health_endpoint.go`.
