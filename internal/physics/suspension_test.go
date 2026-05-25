package physics_test

// Suspension and ground-contact tests.
//
// AC1: GroundSampler interface defined; FlatGround satisfies it.
// AC2: Suspension settles in <1 second from a 1 m drop (damped response).
// AC3: On flat ground at rest, sum(WheelLoad) ≈ mass * g ± 1%.
// AC4: Airborne ↔ grounded transitions work cleanly; Grounded bitmask reflects
//      per-wheel contact state.

import (
	"math"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

const suspG = float32(9.81)

// TestFlatGroundSatisfiesGroundSampler is a compile-time + runtime assertion
// that FlatGround() returns a valid GroundSampler (AC1).
func TestFlatGroundSatisfiesGroundSampler(t *testing.T) {
	// Compile-time: ensure FlatGround returns physics.GroundSampler.
	gs := physics.FlatGround(5.0)
	_ = gs

	// Runtime: Sample on flat ground always returns ok=true and correct values.
	g := physics.FlatGround(3.14)
	h, n, ok := g.Sample(0, 0)
	if !ok {
		t.Error("FlatGround.Sample: ok=false, want true")
	}
	if h != 3.14 {
		t.Errorf("FlatGround(3.14).Sample height = %v, want 3.14", h)
	}
	wantNormal := [3]float32{0, 1, 0}
	if n != wantNormal {
		t.Errorf("FlatGround.Sample normal = %v, want %v", n, wantNormal)
	}

	// Sample at arbitrary XZ — should always be ok.
	_, _, ok2 := g.Sample(1234.5, -999.9)
	if !ok2 {
		t.Error("FlatGround.Sample at far XZ: ok=false, want true")
	}
}

// suspEqY returns the suspension-equilibrium CoM height (world Y) for
// DefaultConstants on FlatGround(groundH). At this height the spring force
// per corner equals mg/4 and vertAccel = 0.
//
//	Y_eq = groundH + cgH + restLen - mass*g / (4*k)
func suspEqY(groundH float32) float32 {
	c := physics.DefaultConstants
	k := c.SuspensionSpringK
	if k <= 0 {
		k = 40000
	}
	return groundH + c.CGHeight + c.SuspensionRestLength - c.Mass*suspG/(4*k)
}

// TestFlatGroundRestLoadSumsToWeight verifies that on flat ground at rest,
// with the car at equilibrium height and zero velocity, sum(WheelLoad) ≈ mg
// within 1% (AC3).
func TestFlatGroundRestLoadSumsToWeight(t *testing.T) {
	c := physics.DefaultConstants
	sampler := physics.FlatGround(0)

	// Start at equilibrium height with zero velocity.
	s := physics.State{
		Position:    [3]float32{0, suspEqY(0), 0},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        0,
	}

	// Run for 3 seconds to let transients settle.
	dt := float32(1.0 / 60.0)
	for i := 0; i < 3*60; i++ {
		s = physics.Step(s, physics.Input{Gear: 0}, c, sampler, dt)
	}

	wlSum := s.WheelLoad[0] + s.WheelLoad[1] + s.WheelLoad[2] + s.WheelLoad[3]
	mg := c.Mass * suspG

	relErr := float64(math.Abs(float64(wlSum-mg))) / float64(mg)
	const tol = 0.01 // 1% tolerance
	if relErr > tol {
		t.Errorf(
			"sum(WheelLoad) = %.2f N, mg = %.2f N, rel-err = %.4f > %.2f",
			wlSum, mg, relErr, tol,
		)
	}

	// All 4 wheels must be grounded at rest on flat ground.
	const allGrounded = uint8(0b00001111)
	if s.Grounded != allGrounded {
		t.Errorf(
			"Grounded = 0b%04b, want 0b%04b (all 4 wheels grounded at rest)",
			s.Grounded, allGrounded,
		)
	}
}

// TestSuspensionSettlesUnder1SecondFrom1mDrop verifies that a car dropped
// from 1 m above the suspension equilibrium height settles within 1 second
// (AC2). "Settling" is defined as |LinearVel[1]| < 0.1 m/s (body nearly at
// rest vertically).
func TestSuspensionSettlesUnder1SecondFrom1mDrop(t *testing.T) {
	c := physics.DefaultConstants
	sampler := physics.FlatGround(0)

	// Drop height: 1 m above the equilibrium CoM position.
	dropHeight := suspEqY(0) + 1.0

	s := physics.State{
		Position:    [3]float32{0, dropHeight, 0},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        0,
	}

	const settleVelThresh = float32(0.1) // m/s vertical
	const maxSecs = 1.0
	dt := float32(1.0 / 60.0)
	ticks := int(maxSecs / float64(dt))

	settled := false
	for i := 0; i < ticks; i++ {
		s = physics.Step(s, physics.Input{Gear: 0}, c, sampler, dt)
		vy := s.LinearVel[1]
		if vy < 0 {
			vy = -vy
		}
		// Require the car to be grounded AND vertical velocity small.
		if s.Grounded != 0 && vy < settleVelThresh {
			settled = true
			t.Logf(
				"settled at tick %d (%.3f s): LinearVel[1]=%.4f m/s",
				i+1, float64(i+1)*float64(dt), s.LinearVel[1],
			)
			break
		}
	}

	if !settled {
		t.Errorf(
			"suspension did not settle within %.1f s: final LinearVel[1]=%.4f m/s, Grounded=0b%04b",
			maxSecs,
			s.LinearVel[1],
			s.Grounded,
		)
	}

	// Also verify no NaN or Inf in state after settling.
	for i, v := range s.Position {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Errorf("Position[%d] = %v after drop test (want finite)", i, v)
		}
	}
}

// TestAirborneGroundedTransition verifies that Grounded transitions cleanly
// between 0 (airborne) and non-zero (contact) without NaN or popping (AC4).
func TestAirborneGroundedTransition(t *testing.T) {
	c := physics.DefaultConstants
	sampler := physics.FlatGround(0)

	// Start with the car well above the ground (fully airborne).
	// The car falls, hits the ground, bounces, settles.
	startY := suspEqY(0) + 5.0 // 5 m above equilibrium

	s := physics.State{
		Position:    [3]float32{0, startY, 0},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        0,
	}

	dt := float32(1.0 / 60.0)
	sawAirborne := false
	sawGrounded := false
	nanDetected := false

	for i := 0; i < 5*60; i++ { // 5 seconds
		s = physics.Step(s, physics.Input{Gear: 0}, c, sampler, dt)

		// Check for NaN / Inf.
		for _, v := range s.Position {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				nanDetected = true
			}
		}
		for _, v := range s.LinearVel {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				nanDetected = true
			}
		}
		for _, v := range s.SuspensionCompression {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				nanDetected = true
			}
		}

		if s.Grounded == 0 {
			sawAirborne = true
		} else {
			sawGrounded = true
		}
	}

	if nanDetected {
		t.Error(
			"NaN or Inf detected in state during airborne/grounded transition",
		)
	}
	if !sawAirborne {
		t.Error("expected to see airborne state (Grounded==0) during drop test")
	}
	if !sawGrounded {
		t.Error("expected to see grounded state (Grounded!=0) after car lands")
	}
}

// TestGroundedBitmaskPerWheel verifies that the Grounded bitmask reflects
// per-wheel contact state correctly (AC4).
//
// Strategy: place the car at height where only the front wheels can reach the
// ground by tilting the simulated ground up at the front. We use a custom
// GroundSampler that returns different heights for front vs rear wheel
// positions.
func TestGroundedBitmaskPerWheel(t *testing.T) {
	// A tilted ground sampler: front of track is higher than rear.
	// Front wheel Z offset ≈ +wheelbase/2 = +1.3 m.
	// We set ground height so that at the default Y position:
	//   - front wheels (Z ≈ +1.3) have compression > 0 → grounded
	//   - rear wheels  (Z ≈ -1.3) have compression <= 0 → airborne
	c := physics.DefaultConstants
	halfWB := c.Wheelbase / 2.0

	// At a given position, the car is at equilibrium for the front wheels.
	// Front ground height = groundH_front, rear ground height = 0.
	// Choose groundH_front such that front wheels are grounded and rear are not.
	//
	// At equilibrium Y (for front wheels, groundH = heightFront):
	//   compression_front = restLen - (wheelBaseY - heightFront) > 0 ✓
	//   compression_rear  = restLen - (wheelBaseY - 0) < 0          ✓
	//
	// Using wheelBaseY = Position[1] - cgH:
	//   At suspEqY(0): wheelBaseY = 0.776 - 0.55 = 0.226
	//   compression_rear = restLen - (0.226 - 0) = 0.30 - 0.226 = 0.074 > 0  (still grounded)
	//
	// Need rear compression <= 0:
	//   0.30 - (wheelBaseY - 0) <= 0 → wheelBaseY >= 0.30
	//   → Position[1] >= cgH + restLen = 0.55 + 0.30 = 0.85 m
	//
	// Set Position[1] = 0.9 m so rear is airborne, front is grounded (with elevated ground).
	posY := float32(0.90)
	wheelBaseY := posY - c.CGHeight // = 0.90 - 0.55 = 0.35

	// Front ground height = restLen + wheelBaseY - epsilon (just barely grounded).
	heightFront := c.SuspensionRestLength + wheelBaseY - 0.02
	// Rear ground height = 0 (below the wheel → airborne since wheelBaseY > restLen).

	tiltedSampler := tiltedGround{
		halfWB:      halfWB,
		heightFront: heightFront,
		heightRear:  0,
	}

	s := physics.State{
		Position:    [3]float32{0, posY, 0},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        0,
	}

	next := physics.Step(
		s,
		physics.Input{Gear: 0},
		c,
		tiltedSampler,
		float32(1.0/60.0),
	)

	// FL=bit0, FR=bit1 should be set (front grounded).
	// RL=bit2, RR=bit3 should be clear (rear airborne).
	const wantFrontBits = uint8(0b00000011)
	const wantRearBits = uint8(0b00001100)

	frontGrounded := next.Grounded & wantFrontBits
	rearGrounded := next.Grounded & wantRearBits

	if frontGrounded != wantFrontBits {
		t.Errorf(
			"expected front wheels grounded (bits 0,1 set), got Grounded=0b%08b",
			next.Grounded,
		)
	}
	if rearGrounded != 0 {
		t.Errorf(
			"expected rear wheels airborne (bits 2,3 clear), got Grounded=0b%08b",
			next.Grounded,
		)
	}
}

// tiltedGround is a test GroundSampler that returns different heights for
// front vs rear wheel positions (identified by Z coordinate).
type tiltedGround struct {
	halfWB      float32
	heightFront float32
	heightRear  float32
}

func (tg tiltedGround) Sample(_, z float32) (float32, [3]float32, bool) {
	normal := [3]float32{0, 1, 0}
	// Front wheels have positive Z offset from CoM; rear have negative.
	if z >= 0 {
		return tg.heightFront, normal, true
	}
	return tg.heightRear, normal, true
}
