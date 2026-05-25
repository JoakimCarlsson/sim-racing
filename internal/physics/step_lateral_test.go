package physics_test

// Lateral dynamics tests for physics.Step.
//
// AC1: Skidpad steady-state test — constant throttle + constant steer reaches
//      steady-state lateral acceleration within the 0.8-1.1 g band.
// AC2: Yaw-decay test — with zero steer input, AngularVel about the vertical
//      (Y) axis decays to ~0 rad/s.

import (
	"math"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

const lateralG = float32(9.81)

// TestLateral_SkidpadSteadyState drives at constant throttle and constant
// maximum steer for 10 s, then measures the centripetal (lateral) acceleration
// derived from yaw rate and forward speed. It must fall within [0.8, 1.1] g
// (AC1).
//
// Evidence: step_lateral_test.go:46 (TestLateral_SkidpadSteadyState)
func TestLateral_SkidpadSteadyState(t *testing.T) {
	t.Helper()

	c := physics.DefaultConstants

	// Seed: moving at 60 km/h forward, gear 3, no yaw.
	startV := float32(60 * kmhToMs)
	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		LinearVel:   [3]float32{0, 0, startV},
		Orientation: [4]float32{0, 0, 0, 1}, // identity quaternion
		Gear:        3,
		RPM:         3000,
	}

	in := physics.Input{
		Throttle: 0.35,
		Brake:    0.0,
		Steer:    1.0, // full lock right
		Gear:     3,
	}

	// Run for 10 s to reach steady state.
	ticks := int(10 * tickHz)
	for i := 0; i < ticks; i++ {
		s = physics.Step(s, in, c, physics.FlatGround(0), tickDT)
	}

	// Lateral acceleration = yaw_rate * v_long (centripetal, body frame).
	// AngularVel[1] is yaw rate (rad/s, body Y axis), LinearVel[2] is v_long.
	yawRate := s.AngularVel[1]
	vLong := s.LinearVel[2]
	latAccel := yawRate * vLong // m/s²

	latAccelG := latAccel / lateralG

	const minG, maxG = 0.8, 1.1
	if latAccelG < minG || latAccelG > maxG {
		t.Errorf(
			"skidpad steady-state lateral accel = %.3f g (yaw=%.3f rad/s, v=%.2f m/s), want [%.1f, %.1f] g",
			latAccelG,
			yawRate,
			vLong,
			minG,
			maxG,
		)
	}
}

// TestLateral_Slalom checks symmetric response: identical magnitude steer in
// both directions produces equal magnitude lateral acceleration (AC1 secondary,
// symmetry guard).
//
// Evidence: step_lateral_test.go:84 (TestLateral_Slalom)
func TestLateral_Slalom(t *testing.T) {
	c := physics.DefaultConstants

	runSteadyState := func(steer float32) float32 {
		startV := float32(60 * kmhToMs)
		s := physics.State{
			Position:    [3]float32{0, suspGroundY, 0},
			LinearVel:   [3]float32{0, 0, startV},
			Orientation: [4]float32{0, 0, 0, 1},
			Gear:        3,
			RPM:         3000,
		}
		in := physics.Input{
			Throttle: 0.35,
			Brake:    0.0,
			Steer:    steer,
			Gear:     3,
		}
		ticks := int(10 * tickHz)
		for i := 0; i < ticks; i++ {
			s = physics.Step(s, in, c, physics.FlatGround(0), tickDT)
		}
		yawRate := s.AngularVel[1]
		vLong := s.LinearVel[2]
		return yawRate * vLong
	}

	rightAccel := runSteadyState(1.0)
	leftAccel := runSteadyState(-1.0)

	// The magnitudes must match within 1%.
	diff := float64(
		rightAccel + leftAccel,
	) // they should cancel (opposite signs)
	magRight := math.Abs(float64(rightAccel))
	magLeft := math.Abs(float64(leftAccel))
	if magRight < 0.01 || magLeft < 0.01 {
		t.Errorf(
			"lateral acceleration too small: right=%.4f, left=%.4f",
			rightAccel,
			leftAccel,
		)
		return
	}
	relErr := math.Abs(diff) / ((magRight + magLeft) / 2)
	if relErr > 0.01 {
		t.Errorf(
			"asymmetric lateral response: right=%.4f m/s², left=%.4f m/s², rel-err=%.4f",
			rightAccel,
			leftAccel,
			relErr,
		)
	}
}

// TestLateral_YawDecay seeds the vehicle with a yaw rate of 1.0 rad/s and
// zero steer input, then verifies that within 5 s the yaw rate decays below
// 0.05 rad/s (AC2).
//
// Evidence: step_lateral_test.go:125 (TestLateral_YawDecay)
func TestLateral_YawDecay(t *testing.T) {
	c := physics.DefaultConstants

	// Seed: moving at 40 km/h with an initial yaw rate of 1.0 rad/s.
	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		LinearVel:   [3]float32{0, 0, float32(40 * kmhToMs)},
		AngularVel:  [3]float32{0, 1.0, 0}, // 1 rad/s yaw
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        2,
		RPM:         2500,
	}

	in := physics.Input{
		Throttle: 0.2,
		Brake:    0.0,
		Steer:    0.0, // no steer
		Gear:     2,
	}

	const maxSecs = 5.0
	ticks := int(maxSecs * tickHz)
	for i := 0; i < ticks; i++ {
		s = physics.Step(s, in, c, physics.FlatGround(0), tickDT)
	}

	finalYawRate := s.AngularVel[1]
	if finalYawRate < 0 {
		finalYawRate = -finalYawRate
	}

	const threshold = float32(0.05)
	if finalYawRate >= threshold {
		t.Errorf(
			"yaw rate did not decay: final = %.4f rad/s, want < %.2f rad/s within %.0f s",
			finalYawRate,
			threshold,
			maxSecs,
		)
	}
}
