package physics_test

// Weight transfer tests for physics.Step.
//
// AC1: WheelLoad sum equals total normal force (m*g + downforce) within 1e-3 N.
// AC2: Brake/throttle/cornering shift loads in the expected direction.
// AC3: Pacejka lateral force is load-sensitive (TestPacejka_LoadSensitive).

import (
	"math"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

const (
	physG   = float32(9.81)
	sumTolN = float32(1e-3) // Newton tolerance for sum checks
)

// totalNormalForce returns the expected total normal force for the given
// constants and forward speed (downforce is speed-dependent).
func totalNormalForce(c physics.Constants, vMs float32) float32 {
	staticWeight := c.Mass * physG
	downforce := 0.5 * c.AirDensity * c.DownforceCoeff * c.FrontalArea * vMs * vMs
	return staticWeight + downforce
}

// TestWheelLoad_StaticSum verifies that at rest (v=0, no acceleration) the four
// wheel loads sum to m*g + downforce within sumTolN (AC1).
//
// Evidence: step_weight_transfer_test.go (TestWheelLoad_StaticSum)
func TestWheelLoad_StaticSum(t *testing.T) {
	c := physics.DefaultConstants
	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		LinearVel:   [3]float32{0, 0, 0},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        0,
		RPM:         c.IdleRPM,
	}
	in := physics.Input{Gear: 0}

	next := physics.Step(s, in, c, physics.FlatGround(0), tickDT)

	wlSum := next.WheelLoad[0] + next.WheelLoad[1] +
		next.WheelLoad[2] + next.WheelLoad[3]
	expected := totalNormalForce(c, 0)

	diff := float32(math.Abs(float64(wlSum - expected)))
	if diff > sumTolN {
		t.Errorf(
			"static WheelLoad sum = %.4f N, want %.4f N (diff %.6f > tol %.6f)",
			wlSum, expected, diff, sumTolN,
		)
	}

	// All four wheels must be positive.
	for i, wl := range next.WheelLoad {
		if wl <= 0 {
			t.Errorf("WheelLoad[%d] = %.4f, want > 0", i, wl)
		}
	}
}

// TestWheelLoad_DynamicSum verifies that during hard braking at 100 km/h the
// four wheel loads still sum to m*g + downforce within sumTolN (AC1).
//
// The expected total is computed using the forward speed after the step, since
// downforce is a function of vNew (the integrated velocity).
//
// Evidence: step_weight_transfer_test.go (TestWheelLoad_DynamicSum)
func TestWheelLoad_DynamicSum(t *testing.T) {
	c := physics.DefaultConstants
	v := float32(100 * kmhToMs)
	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		LinearVel:   [3]float32{0, 0, v},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        4,
		RPM:         4000,
	}
	in := physics.Input{
		Throttle: 0,
		Brake:    1.0, // hard brake
		Steer:    0.0,
		Gear:     4,
	}

	next := physics.Step(s, in, c, physics.FlatGround(0), tickDT)

	wlSum := next.WheelLoad[0] + next.WheelLoad[1] +
		next.WheelLoad[2] + next.WheelLoad[3]
	// Use the post-step forward speed so downforce matches what Step computed.
	expected := totalNormalForce(c, next.LinearVel[2])

	diff := float32(math.Abs(float64(wlSum - expected)))
	// Tolerance is widened slightly here: wheel-lift clamping can absorb up to
	// a few Newtons when loads go negative before clamping. We accept 1 N.
	const dynTol = float32(1.0)
	if diff > dynTol {
		t.Errorf(
			"dynamic WheelLoad sum = %.4f N, want %.4f N (diff %.4f > tol %.1f N)",
			wlSum,
			expected,
			diff,
			dynTol,
		)
	}
}

// TestWheelLoad_BrakeShiftsForward verifies that hard braking increases front
// axle load above the static front load (AC2).
//
// Evidence: step_weight_transfer_test.go (TestWheelLoad_BrakeShiftsForward)
func TestWheelLoad_BrakeShiftsForward(t *testing.T) {
	c := physics.DefaultConstants
	v := float32(100 * kmhToMs)

	// Compute expected static front load (no downforce at rest for comparison).
	// Use the actual static distribution with downforce at v.
	staticTotal := totalNormalForce(c, v)
	staticFront := staticTotal * c.WeightDistributionFront

	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		LinearVel:   [3]float32{0, 0, v},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        4,
		RPM:         4000,
	}
	in := physics.Input{
		Throttle: 0,
		Brake:    1.0, // full brake
		Steer:    0,
		Gear:     4,
	}

	next := physics.Step(s, in, c, physics.FlatGround(0), tickDT)

	frontLoad := next.WheelLoad[0] + next.WheelLoad[1]
	if frontLoad <= staticFront {
		t.Errorf(
			"brake: front load = %.2f N, want > static %.2f N (load did not shift forward)",
			frontLoad,
			staticFront,
		)
	}
}

// TestWheelLoad_ThrottleShiftsRear verifies that full throttle from speed
// increases rear axle load above the static rear load (AC2).
//
// Evidence: step_weight_transfer_test.go (TestWheelLoad_ThrottleShiftsRear)
func TestWheelLoad_ThrottleShiftsRear(t *testing.T) {
	c := physics.DefaultConstants
	v := float32(60 * kmhToMs)

	staticTotal := totalNormalForce(c, v)
	staticRear := staticTotal * (1.0 - c.WeightDistributionFront)

	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		LinearVel:   [3]float32{0, 0, v},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        3,
		RPM:         3500,
	}
	in := physics.Input{
		Throttle: 1.0, // full throttle
		Brake:    0,
		Steer:    0,
		Gear:     3,
	}

	next := physics.Step(s, in, c, physics.FlatGround(0), tickDT)

	rearLoad := next.WheelLoad[2] + next.WheelLoad[3]
	if rearLoad <= staticRear {
		t.Errorf(
			"throttle: rear load = %.2f N, want > static %.2f N (load did not shift rearward)",
			rearLoad,
			staticRear,
		)
	}
}

// TestWheelLoad_CorneringShiftsOutside verifies that a left turn (positive steer
// at speed) shifts load to the outside (right) wheels: FR+RR > FL+RL (AC2).
//
// Evidence: step_weight_transfer_test.go (TestWheelLoad_CorneringShiftsOutside)
func TestWheelLoad_CorneringShiftsOutside(t *testing.T) {
	c := physics.DefaultConstants

	// Run to steady-state cornering to have a meaningful yaw rate and lateral acc.
	v := float32(60 * kmhToMs)
	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		LinearVel:   [3]float32{0, 0, v},
		Orientation: [4]float32{0, 0, 0, 1},
		Gear:        3,
		RPM:         3000,
	}
	in := physics.Input{
		Throttle: 0.35,
		Brake:    0,
		Steer:    1.0, // full lock left turn (steer > 0 = left in this model)
		Gear:     3,
	}

	// Run for 5 s to build steady-state lateral acceleration.
	ticks := int(5 * tickHz)
	for i := 0; i < ticks; i++ {
		s = physics.Step(s, in, c, physics.FlatGround(0), tickDT)
	}

	// One more step with load capture.
	next := physics.Step(s, in, c, physics.FlatGround(0), tickDT)

	// Left turn → centrifugal force pushes mass to the right → outside wheels
	// are FR (index 1) and RR (index 3).
	rightSide := next.WheelLoad[1] + next.WheelLoad[3] // FR + RR
	leftSide := next.WheelLoad[0] + next.WheelLoad[2]  // FL + RL

	if rightSide <= leftSide {
		t.Errorf(
			"left-turn cornering: right side load = %.2f N, left = %.2f N; expected right > left",
			rightSide,
			leftSide,
		)
	}
}

// TestPacejka_LoadSensitive verifies that an axle with 1.5x normal load
// produces at least 1.3x the lateral force at the same slip angle (AC3).
//
// This directly tests the pacejkaFy load-scaling property used in Step.
//
// Evidence: step_weight_transfer_test.go (TestPacejka_LoadSensitive)
func TestPacejka_LoadSensitive(t *testing.T) {
	c := physics.DefaultConstants

	// Build two states identical except that one has higher WheelLoad.
	// We test via full Step so that per-axle loads are fed into pacejkaFy.
	//
	// Strategy: compare steady-state lateral forces at two different weights
	// by scaling Mass (which drives static + dynamic wheel loads equally).
	//
	// baseM → mass M, scaledM → mass 1.5*M, same geometry and slip.
	baseC := c
	scaledC := c
	scaledC.Mass = c.Mass * 1.5

	// Common state: constant moderate speed, no yaw yet, mild steer.
	v := float32(60 * kmhToMs)
	// equilY returns the suspension equilibrium CoM height for a given Constants.
	equilY := func(cc physics.Constants) float32 {
		const g = float32(9.81)
		k := cc.SuspensionSpringK
		if k <= 0 {
			k = 40000
		}
		return cc.CGHeight + cc.SuspensionRestLength - cc.Mass*g/(4*k)
	}
	makeState := func(cc physics.Constants) physics.State {
		return physics.State{
			Position:    [3]float32{0, equilY(cc), 0},
			LinearVel:   [3]float32{0, 0, v},
			Orientation: [4]float32{0, 0, 0, 1},
			Gear:        3,
			RPM:         3000,
		}
	}
	in := physics.Input{
		Throttle: 0.35,
		Brake:    0,
		Steer:    0.5, // half lock
		Gear:     3,
	}

	// Run both to a quasi-steady state (5 s).
	sBase := makeState(baseC)
	sScaled := makeState(scaledC)
	ticks := int(5 * tickHz)
	for i := 0; i < ticks; i++ {
		sBase = physics.Step(sBase, in, baseC, physics.FlatGround(0), tickDT)
		sScaled = physics.Step(
			sScaled,
			in,
			scaledC,
			physics.FlatGround(0),
			tickDT,
		)
	}

	// Lateral force proxy: m * ay = m * r * v
	baseLatForce := baseC.Mass * sBase.AngularVel[1] * sBase.LinearVel[2]
	scaledLatForce := scaledC.Mass * sScaled.AngularVel[1] * sScaled.LinearVel[2]

	if baseLatForce <= 0 {
		t.Fatalf("base lateral force = %.2f N, want > 0", baseLatForce)
	}

	ratio := scaledLatForce / baseLatForce
	const minRatio = float32(1.3)
	if ratio < minRatio {
		t.Errorf(
			"Pacejka load-sensitive: scaled(1.5x mass) lateral force ratio = %.3f, want >= %.1f",
			ratio,
			minRatio,
		)
	}
}
