// Package track defines the canonical track schema and validator.
// This package is pure domain — no net/http, no WebSocket framing.
package track

import "fmt"

// Vec3 is a 3-component float32 vector [x, y, z].
type Vec3 [3]float32

// Vec2 is a 2-component float32 vector [x, z] used for the 2-D limits polygon.
type Vec2 [2]float32

// Quat is a unit quaternion [x, y, z, w].
type Quat [4]float32

// Plane is a straight boundary defined by two endpoints and an outward normal.
// P0 and P1 are the left and right endpoints of the gate line.
// Normal is the unit normal pointing in the forward direction of travel
// (e.g. [0,0,1] for a plane facing +Z).
type Plane struct {
	P0     Vec3
	P1     Vec3
	Normal Vec3
}

// Polygon2D is the flat 2-D outline of the driveable surface, expressed
// as a sequence of (x,z) vertices in counter-clockwise order.
// At least 3 vertices are required.
type Polygon2D []Vec2

// SpawnPoint is a position + orientation from which a car can enter the track.
type SpawnPoint struct {
	Pos Vec3
	Rot Quat
}

// Track is the canonical in-memory representation of a track.json document.
type Track struct {
	ID            string
	Name          string
	SpawnPoints   []SpawnPoint
	StartFinish   Plane
	Sectors       []Plane
	LimitsPolygon Polygon2D
}

// Validate returns a descriptive error if the track is invalid, or nil.
// Rules:
//   - id and name must be non-empty.
//   - spawnPoints must contain at least one entry.
//   - limitsPolygon must have at least 3 vertices.
//   - startFinish.Normal must be non-zero.
//   - every sector's Normal must be non-zero.
func (t Track) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("track: id must not be empty")
	}
	if t.Name == "" {
		return fmt.Errorf("track: name must not be empty")
	}
	if len(t.SpawnPoints) == 0 {
		return fmt.Errorf(
			"track: spawnPoints must contain at least one entry",
		)
	}
	if len(t.LimitsPolygon) < 3 {
		return fmt.Errorf(
			"track: limitsPolygon must have at least 3 vertices, got %d",
			len(t.LimitsPolygon),
		)
	}
	if err := validateNormal(t.StartFinish.Normal, "startFinish.normal"); err != nil {
		return err
	}
	for i, s := range t.Sectors {
		label := fmt.Sprintf("sectors[%d].normal", i)
		if err := validateNormal(s.Normal, label); err != nil {
			return err
		}
	}
	return nil
}

// validateNormal returns an error when the given Vec3 is a zero vector.
func validateNormal(v Vec3, label string) error {
	if v[0] == 0 && v[1] == 0 && v[2] == 0 {
		return fmt.Errorf("track: %s must not be a zero vector", label)
	}
	return nil
}
