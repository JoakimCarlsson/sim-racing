# sim-racing realtime protocol

**Protocol version:** 1
**Transport:** WebSocket (RFC 6455), **binary frames**, little-endian packed payloads.
**Spec status:** target schema. Authoritative encoder/decoder lives at `internal/protocol/` (Go) and `web/src/protocol/messages.ts` (TypeScript). Golden binary fixtures under `internal/protocol/testdata/` are the source of truth — both implementations test against them.

---

## Why binary

This is a 60Hz input channel and a 30Hz snapshot channel. JSON would burn CPU, bandwidth, and allocations. Every message is a fixed/predictable layout encoded with `encoding/binary` (Go) and `DataView` (TypeScript), zero allocations in the hot path.

---

## Anti-cheat invariants

These properties are non-negotiable and define what the protocol *cannot* express:

1. **Clients send inputs only.** There is no field through which a client can transmit position, velocity, lap time, sector split, or any other piece of authoritative state.
2. **Servers send snapshots only.** Snapshots reflect the server's authoritative simulation. Clients render predicted state locally but reconcile against snapshots.
3. **Every accepted input is sequence-numbered and rate-limited.** Replay, reorder, and flood are detected at `internal/realtime`.

If a future change would let a client tell the server "I just crossed the finish line," that change must be refused. The line crossing is detected server-side, full stop.

---

## Transport

Connect with a standard WebSocket upgrade:

```
GET /ws  HTTP/1.1
Upgrade: websocket
```

There are no query parameters. Identity is established by `ClientHello` over the upgraded socket (carrying an optional persistent token from `localStorage`).

The server upgrades the connection and immediately sends a `ServerHello` frame.

---

## Frame layout

Every frame begins with a 2-byte header:

| Offset | Size | Field    | Notes                          |
|-------:|-----:|----------|--------------------------------|
| 0      | 1    | `type`   | Message type tag (see below).  |
| 1      | 1    | `version`| Protocol version. `1` today.   |

The body that follows depends on `type`. All multi-byte integers are **little-endian**. All floats are IEEE-754 `float32`. Fixed-width strings are UTF-8, null-padded.

---

## Message types

| Tag    | Name              | Direction       | Description |
|-------:|-------------------|-----------------|-------------|
| `0x01` | `ClientInput`     | client → server | One input frame at 60 Hz. |
| `0x02` | `ServerSnapshot`  | server → client | Authoritative state of every active car at 30 Hz. |
| `0x03` | `ServerHello`     | server → client | Sent once on connect. Assigns `PlayerID`, declares tick rates and physics constants. |
| `0x04` | `ServerShutdown`  | server → client | Server is going away (graceful shutdown). Body holds a short reason string. |
| `0x05` | `ClientHello`     | client → server | Optional, sent right after connect to reattach an existing identity. |

Tags `0x10+` are reserved for future non-realtime messages (chat, settings, etc.).

---

## `ClientInput` (0x01)

| Offset | Size | Field          | Notes                                  |
|-------:|-----:|----------------|----------------------------------------|
| 2      | 4    | `seq`          | `uint32`, monotonic. Reused across reconnects must continue forward. |
| 6      | 4    | `client_ms`    | `uint32`, client wall-clock ms. Informational only. |
| 10     | 4    | `throttle`     | `float32` in `[0, 1]`. |
| 14     | 4    | `brake`        | `float32` in `[0, 1]`. |
| 18     | 4    | `steer`        | `float32` in `[-1, 1]`. |
| 22     | 1    | `gear`         | `int8`, range `[-1, MaxGear]` (`-1` = reverse, `0` = neutral). |
| 23     | 1    | `flags`        | bit 0: handbrake. bits 1-7 reserved (must be 0). |

Total: **24 bytes**.

Validation gates at `internal/realtime/sanity.go`:

- `throttle`, `brake` in `[0, 1]`. `steer` in `[-1, 1]`.
- `gear` in declared range.
- `seq` strictly greater than the last accepted `seq` (small reorder window allowed; replays rejected).
- Maximum one accepted input per **8 ms** wall clock per connection (≈120 Hz hard cap).
- Failure increments a per-connection rejection counter; sustained abuse closes the connection with a logged reason.

---

## `ServerSnapshot` (0x02)

| Offset | Size | Field          | Notes |
|-------:|-----:|----------------|-------|
| 2      | 4    | `server_ms`    | `uint32`, server wall-clock ms (monotonic basis). |
| 6      | 4    | `last_acked_seq` | `uint32`, the latest `ClientInput.seq` the server has applied for this recipient. |
| 10     | 2    | `car_count`    | `uint16`. |
| 12     | …    | `cars[]`       | `car_count` × `CarState` (see below). |

The per-recipient header (`server_ms`, `last_acked_seq`) is written separately for each connection; the shared `cars[]` payload is encoded once per tick and reused across recipients.

### `CarState`

| Offset | Size | Field          | Notes |
|-------:|-----:|----------------|-------|
| 0      | 2    | `player_id`    | `uint16`. |
| 2      | 16   | `nickname`     | UTF-8, null-padded, ≤ 16 bytes. |
| 18     | 12   | `position`     | `float32[3]`, world-space metres. |
| 30     | 16   | `orientation`  | `float32[4]`, quaternion (x, y, z, w). |
| 46     | 12   | `linear_vel`   | `float32[3]`, m/s. |
| 58     | 12   | `angular_vel`  | `float32[3]`, rad/s. |
| 70     | 4    | `rpm`          | `float32`. |
| 74     | 1    | `gear`         | `int8`. |
| 75     | 1    | `grounded`     | `uint8` bitmask of 4 wheels. |
| 76     | 16   | `wheel_load`   | `float32[4]`, normal force in N. |
| 92     | 16   | `wheel_slip`   | `float32[4]`, normalized slip magnitude. |

Total per car: **108 bytes**. A 32-player snapshot ≈ 32 × 108 + 12 ≈ 3.5 KB outbound at 30 Hz.

---

## `ServerHello` (0x03)

| Offset | Size | Field           | Notes |
|-------:|-----:|-----------------|-------|
| 2      | 2    | `player_id`     | `uint16`, assigned by server. |
| 4      | 16   | `nickname`      | UTF-8, null-padded, ≤ 16 bytes. |
| 20     | 32   | `token`         | base64url-encoded 24-byte raw token (newly minted if `ClientHello` did not present one). |
| 52     | 1    | `tick_hz`       | `uint8`, sim tick rate (60). |
| 53     | 1    | `snapshot_hz`   | `uint8`, snapshot rate (30). |
| 54     | …    | `constants`     | Packed `physics.Constants`. Exact layout matches `internal/protocol.MarshalConstants`. |

The client stores `token` in `localStorage` and sends it back via `ClientHello` on reconnect.

---

## `ServerShutdown` (0x04)

| Offset | Size | Field           | Notes |
|-------:|-----:|-----------------|-------|
| 2      | 16   | `reason`        | UTF-8, null-padded, human-readable. |

The server broadcasts this immediately before closing connections on `SIGTERM`. Clients should show a "server restarting" banner and reconnect with backoff.

---

## `ClientHello` (0x05)

| Offset | Size | Field           | Notes |
|-------:|-----:|-----------------|-------|
| 2      | 32   | `token`         | base64url, the value previously received in `ServerHello`. All-zero means "no prior identity." |
| 34     | 16   | `nickname_req`  | UTF-8, null-padded. Server validates `^[a-z0-9-]{3,16}$`; invalid → ignored. |

If the token matches a known player, the server reattaches that identity (keeps `players.id`, nickname). If not, the server mints a fresh identity.

---

## Heartbeat

The server sends a WebSocket-level ping frame every **30 seconds**. If the pong is not received within **5 seconds**, the server cancels the connection context and closes the socket. Browsers honour WS pings automatically.

Read deadline per frame: **60 seconds**. A connection that sends nothing for 60 s is dropped.

There is no application-level ping/pong — the sub-second snapshot rate already provides a continuous liveness signal.

---

## Close codes

| Code | Meaning |
|------|---------|
| 1000 `StatusNormalClosure` | Clean disconnect by either side. |
| 1008 `StatusPolicyViolation` | Sanity-gate violation (out-of-range input, flood, message size cap, bad origin). |
| 1011 `StatusInternalError` | Unexpected server error. |
| 4001 (app) | Connection rate limit per IP (`429`-equivalent over WS). |

The reason field is short, human-readable, and logged server-side with the connection id; it is *not* a stable identifier and clients must not branch on its text.

---

## Origin and rate limits

In production, the server enforces:

- `Origin` header in `SIM_ALLOWED_ORIGINS` (comma-separated allowlist) — required for the upgrade.
- Per-IP WS upgrade rate: **6 new connections per minute** (token bucket).
- Per-connection inbound message size cap: **256 bytes** (well above `ClientInput`'s 24).

---

## Example session

```
Client                                    Server
  ── GET /ws ────────────────────────────►
  ◄── 101 Switching Protocols ────────────
  [optional] ── ClientHello(token,nick) ─►
  ◄── ServerHello(player_id=42, token, constants)
  ── ClientInput(seq=1, throttle=1)  ────►   [60 Hz, applied at next tick]
  ── ClientInput(seq=2, throttle=1)  ────►
  ...
  ◄── ServerSnapshot(server_ms=…, cars=[…])  [30 Hz]
  ◄── ServerSnapshot(…)
  ...
  ◄── ServerShutdown("restart")              [on SIGTERM]
  ◄── [close 1000]
```

---

## Versioning policy

- `version` byte in the header is bumped only on **breaking** changes (field added in the middle of an existing message, semantics change, type tag reassigned).
- Adding a new message type with an unused tag is **not** a breaking change.
- The server refuses connections whose first frame uses a `version` it does not recognize, with close code 1008.
