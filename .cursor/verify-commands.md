# Verification commands

Run from the repository root unless noted.

## Submodule

```bash
git submodule update --init --recursive
```

**Expected:** `deps/minmux` is populated (e.g. `deps/minmux/router/go.mod` exists).

## Go build

```bash
go build ./cmd/api
go build ./cmd/migrate
```

**Expected:** exits 0; produces `api` and `migrate` binaries (or `.exe` on Windows) in the current directory.

Optional:

```bash
go build -o bin/api ./cmd/api
go build -o bin/migrate ./cmd/migrate
```

## Go test

```bash
go test ./...
```

**Expected:** all unit tests pass without Docker.

Optional integration (requires live Postgres and `DATABASE_URL`):

```bash
go test -tags=integration ./internal/store/...
```

## Docker Compose

```bash
cp .env.example .env
docker compose up --build
```

**Expected:**

- `db` service becomes healthy (`pg_isready` passes).
- `api` service starts, applies migrations, and logs `listening on :8080`.
- API port `${API_PORT:-8080}` is reachable on the host.

Stop with `Ctrl+C`, then:

```bash
docker compose down
```

Optional database check while compose is up:

```bash
docker compose exec db pg_isready -U bastion -d bastion
docker compose exec db psql -U bastion -d bastion -c '\dt'
```

**Expected:** `pg_isready` reports `accepting connections`; `\dt` lists `schema_migrations` and `bastion_schema_marker`.

## Migrations (Makefile)

Export `DATABASE_URL` first (use `localhost` as host when running from the host against compose `db`):

```bash
# bash
set -a; source .env; set +a
# then override host for host-side commands:
export DATABASE_URL=postgres://bastion:bastion@localhost:5432/bastion?sslmode=disable

make migrate-version
make migrate-up
```

**Expected:** `migrate-version` prints `version: 1 dirty: false` (or higher if already migrated); `migrate-up` succeeds with no pending migrations.

Dev-only rollback:

```bash
make migrate-down
make migrate-up
```

**Expected:** down removes `bastion_schema_marker`; up restores it.

## Layout checks

```bash
# internal/ must not contain layered dirs
test ! -d internal/controllers
test ! -d internal/services
test ! -d internal/repositories
test ! -d internal/models
```

**Expected:** all commands succeed (exit 0).

Required stubs:

```bash
test -d internal/health
test -d internal/http
test -d internal/store
test -f docs/backend-architecture.md
test -f .env.example
```

**Expected:** all paths exist.

## Makefile

Run **`make install` once** before `make lint` (installs golangci-lint v2, goimports, golines).

```bash
make help
make install
make workspace
make fmt
make lint
make web-install
make web-lint
make check
make migrate-version
```

**Expected:**

- `help` lists dev, web, and migration targets.
- `install` exits 0 and places tools in `$(go env GOPATH)/bin`.
- `workspace` creates `go.work` from `go.work.example` when missing.
- `fmt`, `lint`, `web-lint`, and `check` exit 0 on a clean tree.
- `migrate-version` requires `DATABASE_URL` and prints version/dirty (see Migrations section).

## Frontend (web/)

From `web/` after `bun install`:

```bash
cd web
bun run build
bun run lint
bun run typecheck
bun run test
```

**Expected:** build writes `dist/index.html` and assets; lint, typecheck, and test all exit 0. Test output should show 4 test files, 83+ tests passed.

With API running and `web/dist` built:

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/
curl -s http://localhost:8080/health
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/ready
curl -s http://localhost:8080/ready
```

**Expected:** `/` returns 200 (HTML); `/health` returns JSON `{"status":"ok",...}`; `/ready` returns HTTP 200 with `{"status":"ready"}` when the database is up.

## Manual play smoke (/play)

Run with `bun run dev` in `web/` (and optionally `go run ./cmd/api` for health status):

1. Open http://localhost:5173/play in a browser.
2. Confirm HUD shows: Wave 1/3, Gold 100, Base HP 20, Phase prep.
3. Click **New game** — verify state resets (gold back to 100, waveIndex back to 0, phase back to prep).
4. Select the **Archer** tower from the tower bar.
5. Click a buildable cell on the map — confirm a tower appears on the canvas and gold decreases by 25.
6. Click **Start wave** — button should become disabled; Phase changes to combat; enemies appear and begin moving.
7. Watch enemies move along the path. When towers fire and kill enemies, gold should increase (10g per goblin kill).
8. After all enemies die, phase returns to **prep**; wave counter advances to 2/3.
9. Repeat steps 5–8 for waves 2 and 3.
10. After clearing wave 3: a **Victory** overlay appears over the canvas showing "You cleared all 3 waves!". Confirm Start wave button is disabled.
11. Click **Restart** in the overlay — confirm state resets (gold 100, baseHp 20, phase prep, wave 1/3, canvas clears).
12. Alternative loss path: place no towers (or few), start a wave, let goblins reach the end — Base HP drops. When it hits 0, a **Game Over** overlay appears showing the wave reached and a **Restart** button.
13. Click **Restart** — state resets cleanly.
14. Open the **How to play** collapsible at the bottom — confirm it lists controls for placing towers, starting waves, earning gold, and the game-over condition.
15. From the home page (http://localhost:5173/), confirm a "Start a single-player run →" link navigates to /play.
16. Verify no console errors throughout.
