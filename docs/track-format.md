# track.json schema

This document defines the canonical schema for `track.json` — the metadata
file that describes a racing circuit's spawn points, start/finish plane,
sector planes, and the 2-D limits polygon.

## Coordinate convention

- **Right-handed Y-up** world space (three.js default).
- X points right, Y points up, Z points toward the viewer (−Z is forward in
  track-local convention for the default spawn).
- All distances are in **metres**.
- Angles are stored as **unit quaternions** (XYZW).

## Units

| Field | Unit |
|---|---|
| `pos`, `p0`, `p1`, polygon vertices | metres |
| `rot` | dimensionless unit quaternion |
| `normal` | dimensionless unit vector |

## Schema

```json
{
  "id":            "<string>",
  "name":          "<string>",
  "spawnPoints":   [ <SpawnPoint>, … ],
  "startFinish":   <Plane>,
  "sectors":       [ <Plane>, … ],
  "limitsPolygon": [ <Vec2>, … ]
}
```

### Top-level fields

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier for the track (e.g. `"circuit01"`). Must not be empty. |
| `name` | string | yes | Human-readable display name. Must not be empty. |
| `spawnPoints` | SpawnPoint[] | yes | At least one spawn point is required. |
| `startFinish` | Plane | yes | The start/finish line. |
| `sectors` | Plane[] | yes | Zero or more sector boundary planes. Empty array is valid. |
| `limitsPolygon` | Vec2[] | yes | 2-D outline of the driveable surface. At least 3 vertices required. |

### Vec2

A 2-element JSON array `[x, z]` representing a point on the XZ plane (Y is
always 0 for 2-D limits).

```json
[-50.0, -50.0]
```

### Vec3

A 3-element JSON array `[x, y, z]`.

```json
[0.0, 0.0, 1.0]
```

### Quat

A 4-element JSON array `[x, y, z, w]` (unit quaternion).
Identity rotation is `[0, 0, 0, 1]`.

```json
[0.0, 0.0, 0.0, 1.0]
```

### SpawnPoint

```json
{
  "pos": [0.0, 0.0, 0.0],
  "rot": [0.0, 0.0, 0.0, 1.0]
}
```

| Field | Type | Description |
|---|---|---|
| `pos` | Vec3 | World-space position where the car spawns. |
| `rot` | Quat | Orientation the car faces on spawn. Identity = facing +Z. |

### Plane

A boundary defined by two endpoints and an outward normal.

```json
{
  "p0": [-5.0, 0.0, 0.0],
  "p1": [5.0,  0.0, 0.0],
  "normal": [0.0, 0.0, 1.0]
}
```

| Field | Type | Description |
|---|---|---|
| `p0` | Vec3 | Left endpoint of the gate line. |
| `p1` | Vec3 | Right endpoint of the gate line. |
| `normal` | Vec3 | Unit vector perpendicular to the gate, pointing in the direction of valid crossing. Must not be a zero vector. |

### Sector ordering

The `sectors` array must be ordered by traversal sequence: `sectors[0]` is the
first sector boundary a car encounters after crossing the start/finish plane,
`sectors[1]` is the second, and so on.  The lap state machine (M5) depends on
this ordering to validate lap splits.

Gates that are crossed in a direction opposite to their `normal` are still
detected; the crossing direction can be inferred from the sign of
`dot(motion, plane.Normal)` — positive means forward (same direction as the
normal), negative means reverse.

### limitsPolygon

A flat ordered sequence of `[x, z]` vertices that define the driveable
boundary. Vertices should be given in counter-clockwise order when viewed
from above (+Y looking down). At least **3** vertices are required.

```json
[
  [-50.0, -50.0],
  [ 50.0, -50.0],
  [ 50.0,  50.0],
  [-50.0,  50.0]
]
```

## Plane.Crossed — boundary semantics

`Plane.Crossed(prev, cur Vec3) (crossed bool, t float32)` detects whether the
segment from `prev` to `cur` crosses the gate in the XZ plane (Y is ignored;
all gates are assumed to lie on Y=0).

The fractional crossing position `t ∈ (0, 1]` measures how far along the tick's
motion vector the crossing occurred.  `t=0.5` means the car was halfway through
its step when it reached the gate.

### Boundary policy

| Condition | Outcome | Rationale |
|---|---|---|
| `signedPrev == 0` | `crossed = false` | The previous tick already sat exactly on the plane.  Counting this tick as a fresh crossing would double-fire the event. |
| `signedCur == 0` with `signedPrev != 0` | `crossed = true`, `t = 1.0` | The car arrives exactly on the plane this tick; count it once, here. |
| Same-side (`signedPrev` and `signedCur` same sign) | `crossed = false` | No straddle; no crossing detected. |
| Lateral miss (crossing point outside gate segment) | `crossed = false` | The car passed the infinite plane but not through the physical gate. |

These rules are deterministic: the same `(prev, cur)` pair always produces the
same result regardless of tick rate or floating-point rounding order.

## Validation rules

Both the Go loader (`internal/track`) and the TypeScript loader
(`web/src/track`) enforce the following rules and reject documents that
violate any of them:

| Rule | Error |
|---|---|
| `id` must be a non-empty string | `id must not be empty` |
| `name` must be a non-empty string | `name must not be empty` |
| `spawnPoints` must contain at least one entry | `spawnPoints must contain at least one entry` |
| `limitsPolygon` must have ≥ 3 vertices | `limitsPolygon must have at least 3 vertices` |
| `startFinish.normal` must not be `[0,0,0]` | `startFinish.normal must not be a zero vector` |
| Each `sectors[i].normal` must not be `[0,0,0]` | `sectors[i].normal must not be a zero vector` |
| Unknown top-level JSON fields are rejected | `unknown field: "<key>"` |

## File locations

| Path | Purpose |
|---|---|
| `assets/tracks/<id>/track.json` | Server-side source of truth |
| `assets/tracks/<id>/track.gltf` | 3-D model asset for the track surface |
| `web/public/tracks/<id>/track.json` | Byte-identical copy served to clients |
| `web/public/tracks/<id>/track.gltf` | Byte-identical copy served to clients |

> Note: the `assets/` and `web/public/` copies are kept in sync manually.
> No auto-sync tooling is provided at this milestone.

## Heightmap (.height.bin)

Each track directory may contain a `track.height.bin` file alongside
`track.json`.  The heightmap is a regular-grid elevation map on the XZ
plane, used by the server for terrain queries.  It is generated offline by
`cmd/bake-track` and is **optional** at runtime — the server logs a warning
and continues if the file is absent.

### Binary layout

All fields are **little-endian**.

| Offset | Size | Type | Field | Value |
|--------|------|------|-------|-------|
| 0 | 4 | bytes | magic | `SRHM` |
| 4 | 2 | uint16 | version | `1` |
| 6 | 2 | uint16 | reserved | `0` |
| 8 | 4 | float32 | originX | world X of cell (0,0) |
| 12 | 4 | float32 | originZ | world Z of cell (0,0) |
| 16 | 4 | float32 | cellSize | metres per cell edge |
| 20 | 4 | uint32 | width | columns (X-dimension) |
| 24 | 4 | uint32 | depth | rows (Z-dimension) |
| 28 | W×D×4 | float32[] | heights | row-major heights |
| 28+W×D×4 | 3×W×D×4 | float32[] | normals | packed XYZ normals, row-major |

- **magic** must be exactly `SRHM`; files with any other magic are rejected.
- **version** must be `1`; other values are rejected.
- **cellSize** and both dimensions must be positive; zero or negative values are rejected.
- Heights and normals are stored **row-major** with Z as the outer dimension:
  index = row × width + col, where col ∈ [0, width) maps to X and
  row ∈ [0, depth) maps to Z.
- Normals are packed XYZ triples: `normals[3×i]`, `normals[3×i+1]`,
  `normals[3×i+2]` for the i-th grid point.

### Grid boundary

The grid covers:

- X ∈ `[originX, originX + (width-1) × cellSize]`
- Z ∈ `[originZ, originZ + (depth-1) × cellSize]`

Queries outside this rectangle return `ok=false`.

### Generating a heightmap

```bash
go run ./cmd/bake-track \
  --in  assets/tracks/circuit01/track.gltf \
  --out assets/tracks/circuit01/track.height.bin \
  --cell-size 0.5 \
  --pad 5.0
```

For a flat glTF (Y=0 everywhere) this emits a constant-height grid with
normals `[0,1,0]`.

## Example: circuit01

```json
{
  "id": "circuit01",
  "name": "Circuit 01",
  "spawnPoints": [
    { "pos": [0, 0, 0], "rot": [0, 0, 0, 1] }
  ],
  "startFinish": {
    "p0": [-5, 0, 0],
    "p1": [5, 0, 0],
    "normal": [0, 0, 1]
  },
  "sectors": [
    { "p0": [6, 0, -5],   "p1": [14, 0, -5],  "normal": [0, 0, -1] },
    { "p0": [-14, 0, -12],"p1": [14, 0, -12], "normal": [-1, 0, 0] },
    { "p0": [-14, 0, -5], "p1": [-6, 0, -5],  "normal": [0, 0, 1]  }
  ],
  "limitsPolygon": [
    [-50, -50],
    [50, -50],
    [50, 50],
    [-50, 50]
  ]
}
```
