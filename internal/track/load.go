package track

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// trackJSON is the on-wire JSON representation.  Field names are camelCase so
// the Go types can keep idiomatic PascalCase names while still matching the
// canonical schema documented in docs/track-format.md.
type trackJSON struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	SpawnPoints   []spawnJSON  `json:"spawnPoints"`
	StartFinish   planeJSON    `json:"startFinish"`
	Sectors       []planeJSON  `json:"sectors"`
	LimitsPolygon [][2]float32 `json:"limitsPolygon"`
}

type spawnJSON struct {
	Pos [3]float32 `json:"pos"`
	Rot [4]float32 `json:"rot"`
}

type planeJSON struct {
	P0     [3]float32 `json:"p0"`
	P1     [3]float32 `json:"p1"`
	Normal [3]float32 `json:"normal"`
}

// Load decodes a track.json document from r, validates it, and returns the
// resulting Track.  Unknown JSON fields are rejected (DisallowUnknownFields).
func Load(r io.Reader) (Track, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var raw trackJSON
	if err := dec.Decode(&raw); err != nil {
		return Track{}, fmt.Errorf("track: JSON decode: %w", err)
	}

	spawns := make([]SpawnPoint, len(raw.SpawnPoints))
	for i, s := range raw.SpawnPoints {
		spawns[i] = SpawnPoint{Pos: s.Pos, Rot: s.Rot}
	}

	sectors := make([]Plane, len(raw.Sectors))
	for i, s := range raw.Sectors {
		sectors[i] = Plane{P0: s.P0, P1: s.P1, Normal: s.Normal}
	}

	poly := make(Polygon2D, len(raw.LimitsPolygon))
	for i, v := range raw.LimitsPolygon {
		poly[i] = Vec2(v)
	}

	t := Track{
		ID:          raw.ID,
		Name:        raw.Name,
		SpawnPoints: spawns,
		StartFinish: Plane{
			P0:     raw.StartFinish.P0,
			P1:     raw.StartFinish.P1,
			Normal: raw.StartFinish.Normal,
		},
		Sectors:       sectors,
		LimitsPolygon: poly,
	}

	if err := t.Validate(); err != nil {
		return Track{}, err
	}
	return t, nil
}

// LoadFile is a convenience wrapper around Load that opens the named file.
func LoadFile(path string) (Track, error) {
	f, err := os.Open(path)
	if err != nil {
		return Track{}, fmt.Errorf("track: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return Load(f)
}
