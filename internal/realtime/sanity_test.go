package realtime

import (
	"math"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
)

// makeInput is a helper that builds a valid ClientInput at the given seq.
func makeInput(seq uint32) protocol.ClientInput {
	return protocol.ClientInput{
		Seq:          seq,
		ClientTimeMs: seq * 16,
		Input: physics.Input{
			Throttle: 0.5,
			Brake:    0.0,
			Steer:    0.0,
			Gear:     1,
		},
	}
}

// TestValidateInput_Table covers AC1: every documented bad case returns the
// expected InvalidReason (not ReasonNone).
func TestValidateInput_Table(t *testing.T) {
	const maxGear int8 = 6
	const minIntervalNs int64 = 8_000_000 // 8 ms

	prev := makeInput(1)
	prevWallNs := int64(0)
	// "normal" now = 20 ms after prev (well above interval floor).
	normalNowNs := prevWallNs + 20_000_000

	nanF32 := float32(math.NaN())

	cases := []struct {
		name       string
		in         protocol.ClientInput
		nowNs      int64
		wantReason InvalidReason
	}{
		// ---- throttle ----
		{
			name: "throttle_negative",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Throttle = -0.1
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadThrottle,
		},
		{
			name: "throttle_above_one",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Throttle = 1.1
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadThrottle,
		},
		{
			name: "throttle_nan",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Throttle = nanF32
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadThrottle,
		},
		// ---- brake ----
		{
			name: "brake_negative",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Brake = -0.01
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadBrake,
		},
		{
			name: "brake_above_one",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Brake = 1.5
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadBrake,
		},
		{
			name: "brake_nan",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Brake = nanF32
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadBrake,
		},
		// ---- steer ----
		{
			name: "steer_too_low",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Steer = -1.1
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadSteer,
		},
		{
			name: "steer_too_high",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Steer = 1.01
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadSteer,
		},
		{
			name: "steer_nan",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Steer = nanF32
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadSteer,
		},
		// ---- gear ----
		{
			name: "gear_below_minus_one",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Gear = -2
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadGear,
		},
		{
			name: "gear_above_max",
			in: func() protocol.ClientInput {
				c := makeInput(2)
				c.Input.Gear = maxGear + 1
				return c
			}(),
			nowNs:      normalNowNs,
			wantReason: ReasonBadGear,
		},
		// ---- seq monotonicity ----
		{
			name:       "replayed_seq_same",
			in:         makeInput(1), // same as prev
			nowNs:      normalNowNs,
			wantReason: ReasonReplayedSeq,
		},
		{
			name:       "seq_backward",
			in:         makeInput(0), // less than prev.Seq=1
			nowNs:      normalNowNs,
			wantReason: ReasonReplayedSeq,
		},
		// ---- seq jump ----
		{
			name:       "seq_jump_too_large",
			in:         makeInput(prev.Seq + maxSeqJump + 1),
			nowNs:      normalNowNs,
			wantReason: ReasonSeqJump,
		},
		// ---- rate limit ----
		{
			name:       "rate_limit_too_fast",
			in:         makeInput(2),
			nowNs:      prevWallNs + minIntervalNs - 1, // just under the floor
			wantReason: ReasonRateLimit,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, _ := validateInput(
				&prev, prevWallNs, tc.in, tc.nowNs, maxGear, minIntervalNs,
			)
			if reason != tc.wantReason {
				t.Errorf(
					"validateInput %q: got %v, want %v",
					tc.name,
					reason,
					tc.wantReason,
				)
			}
		})
	}
}

// TestValidateInput_Valid verifies a well-formed next input returns ReasonNone
// (covers AC2 positive path).
func TestValidateInput_Valid(t *testing.T) {
	const maxGear int8 = 6
	const minIntervalNs int64 = 8_000_000

	prev := makeInput(1)
	prevWallNs := int64(0)
	next := makeInput(2)
	nowNs := prevWallNs + minIntervalNs + 1

	reason, err := validateInput(
		&prev,
		prevWallNs,
		next,
		nowNs,
		maxGear,
		minIntervalNs,
	)
	if reason != ReasonNone {
		t.Errorf("expected ReasonNone, got %v (err=%v)", reason, err)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// TestValidateInputZeroAllocs verifies validateInput is allocation-free (AC2,
// LEARNINGS #63).
func TestValidateInputZeroAllocs(t *testing.T) {
	const maxGear int8 = 6
	const minIntervalNs int64 = 8_000_000

	prev := makeInput(1)
	prevWallNs := int64(0)
	next := makeInput(2)
	nowNs := prevWallNs + minIntervalNs + 1

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = validateInput(
			&prev,
			prevWallNs,
			next,
			nowNs,
			maxGear,
			minIntervalNs,
		)
	})
	if allocs != 0 {
		t.Errorf("validateInput allocs = %.1f, want 0", allocs)
	}
}
