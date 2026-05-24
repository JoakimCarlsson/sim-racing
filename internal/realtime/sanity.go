package realtime

import (
	"fmt"
	"math"

	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
)

// InvalidReason identifies why a ClientInput was rejected by validateInput.
// The zero value (ReasonNone) means the input is valid.
type InvalidReason int

const (
	// ReasonNone means the input passed all sanity checks.
	ReasonNone InvalidReason = iota

	// ReasonBadThrottle is returned when Input.Throttle is outside [0, 1] or NaN/Inf.
	ReasonBadThrottle

	// ReasonBadBrake is returned when Input.Brake is outside [0, 1] or NaN/Inf.
	ReasonBadBrake

	// ReasonBadSteer is returned when Input.Steer is outside [-1, 1] or NaN/Inf.
	ReasonBadSteer

	// ReasonBadGear is returned when Input.Gear is outside [-1, maxGear].
	ReasonBadGear

	// ReasonReplayedSeq is returned when in.Seq <= prev.Seq (replayed or
	// out-of-order packet).
	ReasonReplayedSeq

	// ReasonSeqJump is returned when in.Seq - prev.Seq > maxSeqJump (implausibly
	// large gap, possible clock skew attack or client bug).
	ReasonSeqJump

	// ReasonRateLimit is returned when the wall-clock interval since the previous
	// accepted input is shorter than minIntervalNs.
	ReasonRateLimit
)

//go:generate stringer -type=InvalidReason

func (r InvalidReason) String() string {
	switch r {
	case ReasonNone:
		return "ReasonNone"
	case ReasonBadThrottle:
		return "ReasonBadThrottle"
	case ReasonBadBrake:
		return "ReasonBadBrake"
	case ReasonBadSteer:
		return "ReasonBadSteer"
	case ReasonBadGear:
		return "ReasonBadGear"
	case ReasonReplayedSeq:
		return "ReasonReplayedSeq"
	case ReasonSeqJump:
		return "ReasonSeqJump"
	case ReasonRateLimit:
		return "ReasonRateLimit"
	default:
		return fmt.Sprintf("InvalidReason(%d)", int(r))
	}
}

// maxSeqJump is the maximum allowed gap between consecutive accepted sequence
// numbers.  A jump larger than this is treated as a possible replay or clock
// attack.  Value chosen to allow up to ~5 seconds of packet loss at 60 Hz
// (60*5 = 300) with headroom.
const maxSeqJump uint32 = 512

// validateInput checks a newly received ClientInput against the previously
// accepted input and the current wall-clock time.
//
// Parameters:
//
//	prev          – pointer to the last accepted input (must not be nil).
//	prevWallNs    – nanosecond wall-clock timestamp when prev was accepted.
//	in            – the candidate input to validate.
//	nowNs         – current nanosecond wall-clock timestamp.
//	maxGear       – maximum forward gear (e.g. 6); reverse is always -1.
//	minIntervalNs – minimum nanoseconds that must have elapsed since prevWallNs.
//
// Returns (ReasonNone, nil) on success; otherwise a non-zero reason and a
// descriptive error (never allocates on the hot path — fmt.Errorf is only
// reached on the rejection path).
//
// This function is pure (no I/O, no goroutines) and allocation-free on the
// valid-input path.
func validateInput(
	prev *protocol.ClientInput,
	prevWallNs int64,
	in protocol.ClientInput,
	nowNs int64,
	maxGear int8,
	minIntervalNs int64,
) (InvalidReason, error) {
	// --- rate limit (wall-clock) ---
	if nowNs-prevWallNs < minIntervalNs {
		return ReasonRateLimit, fmt.Errorf(
			"rate limit: interval %dns < min %dns",
			nowNs-prevWallNs, minIntervalNs,
		)
	}

	// --- sequence monotonicity ---
	if in.Seq <= prev.Seq {
		return ReasonReplayedSeq, fmt.Errorf(
			"replayed seq: got %d, prev %d", in.Seq, prev.Seq,
		)
	}
	if delta := in.Seq - prev.Seq; delta > maxSeqJump {
		return ReasonSeqJump, fmt.Errorf(
			"seq jump: delta %d > max %d", delta, maxSeqJump,
		)
	}

	// --- range checks (NaN/Inf treated as out-of-range) ---
	t32 := float64(in.Input.Throttle)
	if math.IsNaN(t32) || math.IsInf(t32, 0) || t32 < 0 || t32 > 1 {
		return ReasonBadThrottle, fmt.Errorf(
			"throttle %v out of [0,1]", in.Input.Throttle,
		)
	}

	b32 := float64(in.Input.Brake)
	if math.IsNaN(b32) || math.IsInf(b32, 0) || b32 < 0 || b32 > 1 {
		return ReasonBadBrake, fmt.Errorf(
			"brake %v out of [0,1]", in.Input.Brake,
		)
	}

	s32 := float64(in.Input.Steer)
	if math.IsNaN(s32) || math.IsInf(s32, 0) || s32 < -1 || s32 > 1 {
		return ReasonBadSteer, fmt.Errorf(
			"steer %v out of [-1,1]", in.Input.Steer,
		)
	}

	if in.Input.Gear < -1 || in.Input.Gear > maxGear {
		return ReasonBadGear, fmt.Errorf(
			"gear %d out of [-1,%d]", in.Input.Gear, maxGear,
		)
	}

	return ReasonNone, nil
}
