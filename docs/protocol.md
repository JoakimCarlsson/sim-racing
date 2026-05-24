# Wire Protocol

sim-racing uses a compact binary, little-endian protocol over WebSocket frames.
Every message starts with a 2-byte header followed by a message-specific
payload. There are no JSON or length-framing layers; the WebSocket frame
boundary IS the message boundary.

## MsgType registry

| Hex  | Name           | Direction         |
|------|----------------|-------------------|
| 0x01 | ClientInput    | client → server   |
| 0x02 | ServerSnapshot | server → client   |
| 0x03 | ServerHello    | server → client   |

Reserved for future use: 0x10 and above.

## Common header (all messages)

| Offset | Size | Type  | Field           | Notes                    |
|--------|------|-------|-----------------|--------------------------|
| 0      | 1    | uint8 | MsgType         | See registry above       |
| 1      | 1    | uint8 | ProtocolVersion | Currently 1              |

## ClientInput (0x01) — 24 bytes

Sent by the client each input frame. **Anti-cheat invariant:** this message
MUST NOT contain any position, velocity, lap time, or checkpoint field. The
server is the only source of truth for vehicle state.

| Offset | Size | Type    | Field        | Notes                            |
|--------|------|---------|--------------|----------------------------------|
| 0      | 1    | uint8   | MsgType      | 0x01                             |
| 1      | 1    | uint8   | Version      | 1                                |
| 2      | 4    | uint32  | Seq          | Monotonically increasing per connection |
| 6      | 4    | uint32  | ClientTimeMs | Client-local millisecond clock   |
| 10     | 4    | float32 | Throttle     | [0, 1]                           |
| 14     | 4    | float32 | Brake        | [0, 1]                           |
| 18     | 4    | float32 | Steer        | [-1, 1]; negative = left         |
| 22     | 1    | int8    | Gear         | -1 reverse, 0 neutral, 1..N fwd  |
| 23     | 1    | uint8   | Handbrake    | 0 = off, 1 = on                  |

Total: **24 bytes**.

## ServerSnapshot (0x02) — 11 + N × 100 bytes

Sent by the server at SnapshotHz (default 20 Hz). Contains the authoritative
state of every connected car.

### Envelope

| Offset | Size | Type   | Field        | Notes                        |
|--------|------|--------|--------------|------------------------------|
| 0      | 1    | uint8  | MsgType      | 0x02                         |
| 1      | 1    | uint8  | Version      | 1                            |
| 2      | 4    | uint32 | ServerTickMs | Server monotonic clock (ms)  |
| 6      | 4    | uint32 | LastAckedSeq | Last ClientInput.Seq seen    |
| 10     | 1    | uint8  | NumCars      | Number of CarState entries   |
| 11     | …    | —      | Cars[0..N-1] | N × CarState (100 bytes each)|

### CarState (100 bytes per car)

| Offset | Size | Type    | Field    | Notes                             |
|--------|------|---------|----------|-----------------------------------|
| 0      | 2    | uint16  | PlayerID | Stable server-assigned ID         |
| 2      | 8    | [8]byte | Nickname | UTF-8, null-padded; over-long input silently truncated |
| 10     | 90   | State   | State    | See physics.State layout below    |

### physics.State (90 bytes, packed, no alignment padding)

Field order matches the canonical order documented in
`web/src/physics/parity.test.ts:23-29`.

| Offset | Size | Type    | Field          | Notes                         |
|--------|------|---------|----------------|-------------------------------|
| 0      | 12   | f32[3]  | Position       | World-space CoM (m)           |
| 12     | 16   | f32[4]  | Orientation    | Unit quaternion [x, y, z, w]  |
| 28     | 12   | f32[3]  | LinearVel      | Vehicle-space CoM vel (m/s)   |
| 40     | 12   | f32[3]  | AngularVel     | Vehicle-space angular vel (rad/s) |
| 52     | 4    | float32 | RPM            | Engine crankshaft speed       |
| 56     | 1    | int8    | Gear           | -1 reverse, 0 neutral, 1..N  |
| 57     | 16   | f32[4]  | WheelLoad      | Normal force FL,FR,RL,RR (N)  |
| 73     | 16   | f32[4]  | WheelSlip      | Combined slip FL,FR,RL,RR     |
| 89     | 1    | uint8   | Grounded       | Bitmask bit0=FL,1=FR,2=RL,3=RR|

Total State: **90 bytes**.

## ServerHello (0x03) — 211 bytes for DefaultConstants

Sent once by the server immediately after the WebSocket handshake. Contains
the player's assigned ID and the fixed vehicle constants for this session.

### Envelope

| Offset | Size | Type   | Field        | Notes                       |
|--------|------|--------|--------------|-----------------------------|
| 0      | 1    | uint8  | MsgType      | 0x03                        |
| 1      | 1    | uint8  | Version      | 1                           |
| 2      | 2    | uint16 | PlayerID     | Stable server-assigned ID   |
| 4      | 1    | uint8  | ServerTickHz | Physics tick rate (e.g. 60) |
| 5      | 1    | uint8  | SnapshotHz   | Snapshot broadcast rate     |
| 6      | 172  | —      | Constants    | Fixed constants (see below) |
| 178    | 1    | uint8  | GearRatiosLen| Number of gear ratio entries |
| 179    | N×4  | f32[N] | GearRatios   | One float32 per gear        |

Total for DefaultConstants (8 gear ratios): **211 bytes**.

### physics.Constants fixed block (172 bytes, starting at offset 6)

| Offset | Size | Type     | Field                   | Notes                         |
|--------|------|----------|-------------------------|-------------------------------|
| 0      | 4    | float32  | Mass                    | kg                            |
| 4      | 4    | float32  | Wheelbase               | m                             |
| 8      | 4    | float32  | TrackWidth              | m                             |
| 12     | 4    | float32  | MaxEngineTorque         | N·m fallback                  |
| 16     | 4    | float32  | DragCoeff               | dimensionless                 |
| 20     | 4    | float32  | DownforceCoeff          | dimensionless                 |
| 24     | 16   | f32[4]   | TireGripCoeffs          | FL,FR,RL,RR peak friction     |
| 40     | 4    | float32  | MaxSteerAngle           | rad                           |
| 44     | 64   | 8×(2×f32)| TorqueCurve             | 8 entries of (RPM, Torque)    |
| 108    | 4    | float32  | FinalDrive              | differential ratio            |
| 112    | 4    | float32  | DrivetrainEfficiency    | 0-1                           |
| 116    | 4    | float32  | WheelRadius             | m                             |
| 120    | 4    | float32  | RollingResistCoeff      | Crr                           |
| 124    | 4    | float32  | FrontalArea             | m²                            |
| 128    | 4    | float32  | AirDensity              | kg/m³                         |
| 132    | 4    | float32  | MaxBrakeForce           | N                             |
| 136    | 4    | float32  | IdleRPM                 | rev/min                       |
| 140    | 4    | float32  | RedlineRPM              | rev/min                       |
| 144    | 4    | float32  | PacejkaB                | stiffness factor              |
| 148    | 4    | float32  | PacejkaC                | shape factor                  |
| 152    | 4    | float32  | PacejkaD                | peak factor                   |
| 156    | 4    | float32  | YawInertia              | kg·m²                         |
| 160    | 4    | float32  | WeightDistributionFront | 0-1                           |
| 164    | 4    | float32  | CGHeight                | m                             |
| 168    | 4    | float32  | RollStiffnessFront      | 0-1                           |

## Implementation files

| Side       | Path                                      |
|------------|-------------------------------------------|
| Go         | `internal/protocol/messages.go`           |
| TypeScript | `web/src/protocol/messages.ts`            |
| Go tests   | `internal/protocol/messages_test.go`      |
| TS tests   | `web/src/protocol/messages.test.ts`       |
| Goldens    | `internal/protocol/testdata/golden_*.bin` |

## Anti-cheat invariants

1. `ClientInput` (0x01) carries **inputs only**. Any server code that receives
   a 0x01 frame and finds position, velocity, lap time, or sector crossing data
   in it is seeing a forged frame and must drop the connection.
2. The server-side `internal/realtime` ingestion layer applies range and
   monotonic-sequence sanity gates before forwarding inputs to the physics
   step. See `docs/security.md`.

## Wire format stability

The protocol version byte allows future incompatible changes. Implementations
must return `ErrUnsupportedVersion` (Go) or `UnsupportedVersionError` (TS)
when they receive a version other than the one they were compiled against.
Reserved MsgTypes 0x10 and above are for future extensions; receivers must
return `ErrUnknownMsgType` / `UnknownMsgTypeError` rather than silently
dropping unknown frames.
