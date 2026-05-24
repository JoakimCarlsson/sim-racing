# Physics model

`internal/physics` is the authoritative vehicle simulation used by both the
Go server and the Go-compiled-to-WASM client predictor. This document covers
the determinism contract, the golden-hash policy, the architecture-scoping
rationale, and how to regenerate the golden when tuning changes physics
intentionally.

---

## Determinism contract

`internal/physics` is a **pure package**:

- No I/O (no `os`, `net`, `log` calls)
- No non-stdlib imports
- No goroutines
- No calls to `time.Now` or `math/rand`
- No map iteration over values that affect output
- No global mutable state

The single entry-point is:

```go
func Step(s State, in Input, c Constants, dt float32) State
```

For identical `(s, in, c, dt)` inputs the function returns an identical
`State` output — run after run, process after process — on the **same
architecture and OS**. This property is what the golden-hash test guards.

---

## What `TestReplayDeterminism` protects against

`TestReplayDeterminism` (in `internal/physics/replay_test.go`) replays a
pre-recorded 30-second input fixture (`testdata/lap_inputs.bin`) through
`Step` at 60 Hz using `DefaultConstants`, samples every 60th `State` (1 Hz),
and SHA-256 hashes the resulting state stream. The digest is compared against
a committed golden (`testdata/lap_inputs.golden`).

A failing `golden_hash` sub-test means one of:

1. A physics constant or formula was changed — regenerate the golden
   intentionally (see below).
2. A non-deterministic element was accidentally introduced (clock, rand,
   map iteration, goroutine, external I/O) — fix the regression.

The `structural_invariants` sub-test runs on every architecture and checks
that sampled states are physically sane (finite values, unit quaternion within
1e-5, non-negative wheel loads, RPM within `[IdleRPM, RedlineRPM]`).

---

## Architecture-scoping rationale

The Go documentation for package `math` states explicitly:

> "This package does not guarantee bit-identical results across architectures."

Functions like `math.Sin`, `math.Atan2`, `math.Tan`, and `math.Sqrt` — all
used inside `Step` — may produce different lowest-order bits on arm64, 386,
wasm, etc. This is not a bug; it is a documented property of IEEE 754
floating-point on heterogeneous hardware.

Consequences for this codebase:

| Concern | Resolution |
|---|---|
| golden_hash sub-test | Skipped on all non-amd64 arches with an explanatory message |
| structural_invariants sub-test | Runs everywhere; catches NaN / Inf / negative-load regressions |
| Canonical platform | **linux/amd64** (CI host) |

---

## Canonical platform

The committed `testdata/lap_inputs.golden` is produced on **linux/amd64**.
The first CI run on that platform confirms the golden. If the development host
is Windows/amd64 or macOS/amd64, the digest should match; if it does not,
the CI run surfaces the discrepancy and the developer must regenerate from the
linux/amd64 host.

See also: AGENTS.md hard rule #4 ("A physics change without a regenerated,
reviewed golden is not done.") and issue #10 (WASM parity test).

---

## Regeneration procedure

When a physics constant or formula is **intentionally changed**, update the
golden in the **same commit** as the physics change:

```bash
# 1. Make your physics change in internal/physics/.

# 2. Regenerate fixture + golden from the canonical platform.
go run ./cmd/replaygen

# 3. Verify the test passes (and is stable).
go test -run TestReplayDeterminism -count=100 ./internal/physics/

# 4. Commit lap_inputs.bin, lap_inputs.golden, and the physics source
#    together in a single commit.
git add internal/physics/testdata/lap_inputs.bin \
        internal/physics/testdata/lap_inputs.golden \
        internal/physics/*.go
git commit -m "feat(physics): <describe the change>"
```

**Do not** commit a golden change without a corresponding physics-source
change in the same commit — reviewers use the diff to audit the intent.

### Flags for cmd/replaygen

```
go run ./cmd/replaygen [--out PATH] [--golden PATH] [--seconds N] [--hz N]
```

| Flag | Default | Purpose |
|---|---|---|
| `--out` | `internal/physics/testdata/lap_inputs.bin` | fixture output path |
| `--golden` | `internal/physics/testdata/lap_inputs.golden` | golden output path |
| `--seconds` | `30` | scenario duration |
| `--hz` | `60` | simulation frequency |

### Verifying a regenerated fixture is consistent

```bash
go run ./cmd/replaygen --out /tmp/lap_inputs.bin --golden /tmp/lap_inputs.golden
diff /tmp/lap_inputs.bin  internal/physics/testdata/lap_inputs.bin   # must be empty
diff /tmp/lap_inputs.golden internal/physics/testdata/lap_inputs.golden  # must be empty
```

---

## WASM build and Go↔WASM parity

### Building physics.wasm

```bash
./scripts/build-wasm.sh
# or: make wasm
```

This produces two artefacts under `web/public/` (both gitignored):

| File | Purpose |
|---|---|
| `physics.wasm` | Go-compiled WASM binary (~2 MB before Brotli) |
| `wasm_exec.js` | Go runtime JS shim copied from `$(go env GOROOT)/lib/wasm/` |

**TinyGo is explicitly rejected.** TinyGo substitutes its own math and runtime
packages, which breaks the bit-identical parity guarantee between the Go server
and the WASM client predictor. Always use the standard Go toolchain
(`GOOS=js GOARCH=wasm go build`).

### JS bridge surface

`web/src/physics/wasm.ts` exposes:

```ts
loadPhysics(wasmUrl: string, wasmExecUrl: string): Promise<Physics>
```

The returned `Physics` object has two methods:

```ts
interface Physics {
  // Advance the simulation by dt seconds.
  // stateBuf: 96-byte encoded current state (see buffer layout below).
  // inputBuf: 20-byte encoded driver input.
  // Returns: 96-byte encoded next state.
  step(stateBuf: Uint8Array, inputBuf: Uint8Array, dt: number): Uint8Array;

  // Returns DefaultConstants decoded into a typed object.
  defaultConstants(): PhysicsConstants;
}
```

`cmd/physicswasm/main.go` also registers these directly on `globalThis.simPhysics`
for use from vanilla JS:

```js
globalThis.simPhysics.step(stateBuf, inputBuf, dt);   // → Uint8Array
globalThis.simPhysics.defaultConstants();              // → Uint8Array
```

#### stateBuf layout (96 bytes, little-endian)

| Offset | Field | Type |
|---|---|---|
| 0–11 | Position[3] | float32[3] |
| 12–27 | Orientation[4] | float32[4] |
| 28–39 | LinearVel[3] | float32[3] |
| 40–51 | AngularVel[3] | float32[3] |
| 52–55 | RPM | float32 |
| 56 | Gear | int8 (as uint8) |
| 57 | padding | — |
| 58–73 | WheelLoad[4] | float32[4] |
| 74–89 | WheelSlip[4] | float32[4] |
| 90 | Grounded | uint8 |
| 91–95 | padding | — |

**SYNC NOTE:** This field order must stay identical to `hashStateStream` in
`internal/physics/replay_test.go` and `decodeState`/`encodeState` in
`cmd/physicswasm/main.go`. If you change the order in any one file, update
all three.

### Parity contract

`web/src/physics/parity.test.ts` (run via `bun test` / `make parity`) replays
`internal/physics/testdata/lap_inputs.bin` through the WASM binary, samples
every 60th tick at 60 Hz, SHA-256 hashes the state stream using the same
field order and serialisation as `hashStateStream`, and asserts:

```
digest == 9b4d7906439c46568a1f7cd85f82e5164f89e27aedf383a70bbfd1350ec52e37
```

A failing parity test means one of:

1. The WASM binary is stale — rebuild with `make wasm`.
2. The state buffer layout drifted between Go and TypeScript — check the
   SYNC NOTE field order.
3. The golden was regenerated (intentional physics change) without updating
   this file.

```bash
make parity   # build wasm + run bun test parity.test.ts
```

CI runs this check on every PR in the `wasm-parity` job
(`.github/workflows/ci.yml`).

---

## Cross-references

- AGENTS.md hard rule #4 — physics golden enforcement
- Issue #10 — Go↔WASM parity test (separate concern from the golden hash)
- `internal/physics/replay_test.go` — `TestReplayDeterminism`
- `cmd/replaygen/main.go` — fixture + golden generator
- `cmd/physicswasm/main.go` — WASM entry-point
- `web/src/physics/wasm.ts` — TypeScript bridge
- `web/src/physics/parity.test.ts` — parity test
