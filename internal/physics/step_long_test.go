package physics_test

// Longitudinal dynamics tests for physics.Step.
//
// AC1: Full throttle from rest reaches ~100 km/h in 6-9 s and plateaus
//      200-250 km/h with DefaultConstants.
// AC2: All four longitudinal scenarios pass.

import (
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

const (
	kmhToMs = 1.0 / 3.6
	tickHz  = 60.0
	tickDT  = 1.0 / tickHz
)

// autoShift returns the recommended gear based on RPM thresholds, simulating a
// simple driver model. It only shifts up and never below 1.
func autoShift(currentGear int8, rpm float32, maxGears int) int8 {
	if rpm > 6500 && int(currentGear) < maxGears {
		return currentGear + 1
	}
	return currentGear
}

// TestLongitudinal_ZeroToHundred verifies that full throttle with simple
// auto-upshifting from rest reaches 100 km/h within the 6-9 s window (AC1, AC2).
func TestLongitudinal_ZeroToHundred(t *testing.T) {
	c := physics.DefaultConstants
	// maxForwardGear: GearRatios has index 0=reverse, 1=neutral, 2..N=forward.
	maxForwardGear := int8(
		len(c.GearRatios) - 2,
	) // highest forward gear index in Input.Gear terms

	s := physics.State{Gear: 1}
	in := physics.Input{
		Throttle: 1.0,
		Brake:    0.0,
		Gear:     1,
	}

	const target = float32(100 * kmhToMs)
	const maxSecs = 20.0
	maxTicks := int(maxSecs * tickHz)

	var elapsed float64
	for i := 0; i < maxTicks; i++ {
		// Shift up when RPM exceeds 6500.
		in.Gear = autoShift(in.Gear, s.RPM, int(maxForwardGear))
		s = physics.Step(s, in, c, tickDT)
		if elapsed == 0 && s.LinearVel[2] >= target {
			elapsed = float64(i+1) * tickDT
		}
	}

	if elapsed == 0 {
		t.Fatal("vehicle never reached 100 km/h within 20 s")
	}
	const minSec, maxSec = 5.0, 10.0
	if elapsed < minSec || elapsed > maxSec {
		t.Errorf(
			"0-100 km/h time = %.2f s, want [%.1f, %.1f] s",
			elapsed, minSec, maxSec,
		)
	}
}

// TestLongitudinal_TopSpeed verifies that after 90 s of full throttle the
// vehicle is travelling between 200 and 260 km/h (AC1, AC2).
func TestLongitudinal_TopSpeed(t *testing.T) {
	c := physics.DefaultConstants
	maxForwardGear := int8(len(c.GearRatios) - 2)

	s := physics.State{Gear: 1}
	in := physics.Input{
		Throttle: 1.0,
		Brake:    0.0,
		Gear:     1,
	}

	maxTicks := int(90.0 * tickHz)
	for i := 0; i < maxTicks; i++ {
		in.Gear = autoShift(in.Gear, s.RPM, int(maxForwardGear))
		s = physics.Step(s, in, c, tickDT)
	}

	finalKmh := s.LinearVel[2] * 3.6
	const minKmh, maxKmh = 190.0, 270.0
	if finalKmh < minKmh || finalKmh > maxKmh {
		t.Errorf(
			"top speed = %.1f km/h, want [%.0f, %.0f] km/h",
			finalKmh, minKmh, maxKmh,
		)
	}
}

// TestLongitudinal_BrakingDistance verifies that a car travelling at 100 km/h
// stops within a reasonable distance when full brakes are applied (AC2).
func TestLongitudinal_BrakingDistance(t *testing.T) {
	c := physics.DefaultConstants

	// Seed: car moving at 100 km/h forward.
	s := physics.State{
		LinearVel: [3]float32{0, 0, float32(100 * kmhToMs)},
		Gear:      1,
		RPM:       float32(c.IdleRPM),
	}

	in := physics.Input{
		Throttle: 0.0,
		Brake:    1.0,
		Gear:     1,
	}

	var distM float32
	const maxTicks = int(10 * tickHz) // 10 s limit
	stopped := false
	for i := 0; i < maxTicks; i++ {
		distM += s.LinearVel[2] * tickDT
		s = physics.Step(s, in, c, tickDT)
		if s.LinearVel[2] <= 0.1 {
			stopped = true
			break
		}
	}

	if !stopped {
		t.Fatalf(
			"car did not stop within 10 s; final speed = %.2f m/s",
			s.LinearVel[2],
		)
	}

	// Braking from 100 km/h; accept 20-100 m.
	const minDist, maxDist = 20.0, 100.0
	if distM < minDist || distM > maxDist {
		t.Errorf(
			"braking distance = %.1f m, want [%.0f, %.0f] m",
			distM, minDist, maxDist,
		)
	}
}

// TestLongitudinal_Reverse verifies that selecting gear -1 (reverse) with
// full throttle results in a negative (backward) forward velocity (AC2).
func TestLongitudinal_Reverse(t *testing.T) {
	c := physics.DefaultConstants
	s := physics.State{Gear: -1}
	in := physics.Input{
		Throttle: 1.0,
		Brake:    0.0,
		Gear:     -1,
	}

	for i := 0; i < int(5*tickHz); i++ {
		s = physics.Step(s, in, c, tickDT)
	}

	if s.LinearVel[2] >= 0 {
		t.Errorf(
			"reverse gear did not produce negative velocity: got %.3f m/s",
			s.LinearVel[2],
		)
	}
}
