# Bastion conventions (all delivery agents)

Read repo-root **`AGENTS.md` first**, then `docs/backend-architecture.md`, then `LEARNINGS.md`, before planning or implementing.

## Verification — mandatory E2E

1. **Everything observable must be verified end-to-end.** Unit tests alone are not enough for HTTP APIs or running services.
2. **New or changed API routes:** start the API (`go run ./cmd/api`, `docker compose up`, or `commands_to_verify.serve`), then **`curl` every new/changed endpoint** — record status and body; compare to acceptance criteria.
3. **smoke-tester** must not mark pass without live endpoint evidence when the issue touches HTTP.
4. **coder** must list every new/changed route in its handoff with concrete `expect` values.

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

## Pipeline note

Claude Code has no auto-handoff buttons. Chaining is done by `/pipeline` (see `.claude/commands/pipeline.md`), which invokes each agent in sequence via the `Agent` tool and routes the next stage based on the verdict in each agent's final output. Agents end with a structured `HANDOFF:*` block defined in **`docs/pipeline-handoff-schema.md`** — same shape across all three homes — and `/pipeline` parses that block to validate, log, and decide what to do next.

The full chain is **planner → red-team → coder → smoke-tester → reviewer**. The red-team subagent walks every `assumptions[]` entry in the plan; on `RED-TEAM:REFUTED` the plan goes back to the user (ambiguity gate), not to the coder. The orchestrator additionally enforces a **failure-signature circuit breaker** (repeat hash = stop), a **per-issue token budget** (default 400000, override via `BASTION_PIPELINE_BUDGET`), and writes a **per-run JSONL log** under `.pipeline-runs/<issue>/<run-id>.jsonl` — see `docs/pipeline-observability.md`.
