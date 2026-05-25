// Package track_test exercises the track domain loader and validator.
package track_test

import (
	"strings"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

// TestLoad_Circuit01_Golden loads the shipped circuit01/track.json and
// asserts it parses cleanly with the expected field values.
func TestLoad_Circuit01_Golden(t *testing.T) {
	t.Helper()

	tr, err := track.LoadFile("../../assets/tracks/circuit01/track.json")
	if err != nil {
		t.Fatalf("LoadFile returned unexpected error: %v", err)
	}

	if tr.ID != "circuit01" {
		t.Errorf("ID: want %q, got %q", "circuit01", tr.ID)
	}
	if tr.Name == "" {
		t.Error("Name must not be empty")
	}
	if len(tr.SpawnPoints) == 0 {
		t.Error("SpawnPoints must not be empty")
	}
	if len(tr.LimitsPolygon) < 3 {
		t.Errorf(
			"LimitsPolygon must have >=3 vertices, got %d",
			len(tr.LimitsPolygon),
		)
	}

	// StartFinish normal must be non-zero.
	n := tr.StartFinish.Normal
	if n[0] == 0 && n[1] == 0 && n[2] == 0 {
		t.Error("StartFinish.Normal must not be zero vector")
	}

	// circuit01 must have exactly 3 sector planes (AC4).
	if len(tr.Sectors) != 3 {
		t.Errorf("Sectors: want 3, got %d", len(tr.Sectors))
	}
}

// TestValidate_RejectsBadInputs is table-driven and checks that Validate and
// Load return errors for every class of invalid input described in AC2.
func TestValidate_RejectsBadInputs(t *testing.T) {
	// A valid minimal JSON document — each case mutates one aspect.
	const goodJSON = `{
		"id": "test",
		"name": "Test Track",
		"spawnPoints": [{"pos":[0,0,0],"rot":[0,0,0,1]}],
		"startFinish": {"p0":[-5,0,0],"p1":[5,0,0],"normal":[0,0,1]},
		"sectors": [],
		"limitsPolygon": [[-50,-50],[50,-50],[50,50],[-50,50]]
	}`

	t.Run("good_input_passes", func(t *testing.T) {
		_, err := track.Load(strings.NewReader(goodJSON))
		if err != nil {
			t.Fatalf("expected no error for valid JSON, got: %v", err)
		}
	})

	cases := []struct {
		name    string
		json    string
		wantSub string // substring expected in error message
	}{
		{
			name:    "malformed_json",
			json:    `{not valid json`,
			wantSub: "", // any error
		},
		{
			name: "unknown_field",
			json: `{
				"id":"test","name":"Test","unknownField":true,
				"spawnPoints":[{"pos":[0,0,0],"rot":[0,0,0,1]}],
				"startFinish":{"p0":[-5,0,0],"p1":[5,0,0],"normal":[0,0,1]},
				"sectors":[],"limitsPolygon":[[-50,-50],[50,-50],[50,50]]
			}`,
			wantSub: "unknown",
		},
		{
			name: "missing_id",
			json: `{
				"name":"Test",
				"spawnPoints":[{"pos":[0,0,0],"rot":[0,0,0,1]}],
				"startFinish":{"p0":[-5,0,0],"p1":[5,0,0],"normal":[0,0,1]},
				"sectors":[],"limitsPolygon":[[-50,-50],[50,-50],[50,50]]
			}`,
			wantSub: "id",
		},
		{
			name: "missing_name",
			json: `{
				"id":"test",
				"spawnPoints":[{"pos":[0,0,0],"rot":[0,0,0,1]}],
				"startFinish":{"p0":[-5,0,0],"p1":[5,0,0],"normal":[0,0,1]},
				"sectors":[],"limitsPolygon":[[-50,-50],[50,-50],[50,50]]
			}`,
			wantSub: "name",
		},
		{
			name: "empty_spawn_points",
			json: `{
				"id":"test","name":"Test",
				"spawnPoints":[],
				"startFinish":{"p0":[-5,0,0],"p1":[5,0,0],"normal":[0,0,1]},
				"sectors":[],"limitsPolygon":[[-50,-50],[50,-50],[50,50]]
			}`,
			wantSub: "spawnpoints",
		},
		{
			name: "polygon_too_few_vertices",
			json: `{
				"id":"test","name":"Test",
				"spawnPoints":[{"pos":[0,0,0],"rot":[0,0,0,1]}],
				"startFinish":{"p0":[-5,0,0],"p1":[5,0,0],"normal":[0,0,1]},
				"sectors":[],"limitsPolygon":[[-50,-50],[50,-50]]
			}`,
			wantSub: "limitspolygon",
		},
		{
			name: "zero_normal_startfinish",
			json: `{
				"id":"test","name":"Test",
				"spawnPoints":[{"pos":[0,0,0],"rot":[0,0,0,1]}],
				"startFinish":{"p0":[-5,0,0],"p1":[5,0,0],"normal":[0,0,0]},
				"sectors":[],"limitsPolygon":[[-50,-50],[50,-50],[50,50]]
			}`,
			wantSub: "normal",
		},
		{
			name: "zero_normal_sector",
			json: `{
				"id":"test","name":"Test",
				"spawnPoints":[{"pos":[0,0,0],"rot":[0,0,0,1]}],
				"startFinish":{"p0":[-5,0,0],"p1":[5,0,0],"normal":[0,0,1]},
				"sectors":[{"p0":[-5,0,10],"p1":[5,0,10],"normal":[0,0,0]}],
				"limitsPolygon":[[-50,-50],[50,-50],[50,50]]
			}`,
			wantSub: "normal",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := track.Load(strings.NewReader(tc.json))
			if err == nil {
				t.Fatal("expected an error but got nil")
			}
			if tc.wantSub != "" &&
				!strings.Contains(strings.ToLower(err.Error()), tc.wantSub) {
				t.Errorf(
					"error %q does not contain %q",
					err.Error(),
					tc.wantSub,
				)
			}
		})
	}
}
