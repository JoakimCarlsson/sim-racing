// Package physics implements the authoritative deterministic vehicle simulation.
//
// # Purity contract
//
// This package is intentionally pure: no I/O, no logging, no goroutines, and
// no non-stdlib imports. It must compile without modification for both the
// native server binary and the Go-to-WASM client build target
// (cmd/physicswasm). Breaking any of these constraints invalidates the
// Go<->WASM byte-identical determinism guarantee.
//
// # Units
//
// All physical quantities use SI base units unless documented otherwise on the
// individual field:
//   - Distance / position: metres (m)
//   - Velocity:            metres per second (m/s)
//   - Angular velocity:    radians per second (rad/s)
//   - Mass:                kilograms (kg)
//   - Force / torque:      newtons (N) / newton-metres (N·m)
//   - Time step dt:        seconds (s)
//   - Angles:              radians (rad)
//
// # Coordinate system
//
// Two frames are used; fields are labelled accordingly in State:
//
// World frame (inertial, fixed to the track):
//   - State.Position is in world frame (metres from track origin).
//   - State.Orientation is a unit quaternion [x, y, z, w] rotating from body
//     frame to world frame.
//
// Body frame (vehicle-space, attached to and moving with the car):
//   - State.LinearVel is in body frame: X right, Y up, Z forward (m/s).
//   - State.AngularVel is in body frame: X pitch, Y yaw, Z roll (rad/s).
//
// Both frames are right-handed, Y up, Z forward when the vehicle faces its
// default heading:
//
//	+Y  up
//	|
//	+---+X  right
//	/
//	+Z  forward (into the track)
//
// Step integrates LinearVel in body frame, then rotates the displacement into
// world frame to advance Position. Orientation is integrated via quaternion
// derivative from AngularVel each tick and renormalised to prevent drift.
//
// # float32 rationale
//
// All simulation quantities use float32 (single-precision IEEE 754) rather than
// float64. The primary reasons are:
//  1. WASM SIMD and GPU interop both favour 32-bit floats; keeping a single
//     numeric type avoids silent widening casts on the boundary.
//  2. The physics tick runs at 60 Hz and feeds directly into the rendering
//     pipeline; float32 precision is adequate for the distances and velocities
//     encountered in a single lap.
//  3. Binary-serialised snapshots (internal/protocol) are 4 bytes per float,
//     halving bandwidth compared with float64.
//
// All arithmetic in Step must be strictly deterministic: the output State for a
// given (State, Input, Constants, dt) triple must be identical bit-for-bit on
// every platform and every invocation.
package physics
