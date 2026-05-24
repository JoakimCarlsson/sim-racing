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
  "sectors": [],
  "limitsPolygon": [
    [-50, -50],
    [50, -50],
    [50, 50],
    [-50, 50]
  ]
}
```
