package physics_test

import (
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

// TestDefaultConstantsValid asserts that DefaultConstants has sensible non-zero
// values for all scalar fields (AC1).
func TestDefaultConstantsValid(t *testing.T) {
	c := physics.DefaultConstants
	if c.Mass <= 0 {
		t.Errorf("DefaultConstants.Mass = %v, want > 0", c.Mass)
	}
	if c.Wheelbase <= 0 {
		t.Errorf("DefaultConstants.Wheelbase = %v, want > 0", c.Wheelbase)
	}
	if c.TrackWidth <= 0 {
		t.Errorf("DefaultConstants.TrackWidth = %v, want > 0", c.TrackWidth)
	}
	if c.MaxEngineTorque <= 0 {
		t.Errorf("DefaultConstants.MaxEngineTorque = %v, want > 0", c.MaxEngineTorque)
	}
	if c.DragCoeff <= 0 {
		t.Errorf("DefaultConstants.DragCoeff = %v, want > 0", c.DragCoeff)
	}
	if c.DownforceCoeff <= 0 {
		t.Errorf("DefaultConstants.DownforceCoeff = %v, want > 0", c.DownforceCoeff)
	}
	if len(c.GearRatios) == 0 {
		t.Error("DefaultConstants.GearRatios must be non-empty")
	}
	if c.MaxSteerAngle <= 0 {
		t.Errorf("DefaultConstants.MaxSteerAngle = %v, want > 0", c.MaxSteerAngle)
	}
}

// TestZeroValueTypes verifies that zero-value Input and State can be constructed
// without panics, satisfying AC1 structural completeness.
func TestZeroValueTypes(t *testing.T) {
	var in physics.Input
	var s physics.State
	_ = in
	_ = s
}

// TestStepReturnsInputUnchanged asserts that Step(s, in, c, dt) == s for all
// zero inputs, exercising AC2.
func TestStepReturnsInputUnchanged(t *testing.T) {
	var s physics.State
	s.Position = [3]float32{1.0, 2.0, 3.0}
	s.RPM = 3000
	s.Gear = 2

	var in physics.Input
	c := physics.DefaultConstants
	dt := float32(1.0 / 60.0)

	got := physics.Step(s, in, c, dt)

	if got.Position != s.Position {
		t.Errorf("Step changed Position: got %v, want %v", got.Position, s.Position)
	}
	if got.RPM != s.RPM {
		t.Errorf("Step changed RPM: got %v, want %v", got.RPM, s.RPM)
	}
	if got.Gear != s.Gear {
		t.Errorf("Step changed Gear: got %v, want %v", got.Gear, s.Gear)
	}
	if got.Orientation != s.Orientation {
		t.Errorf("Step changed Orientation: got %v, want %v", got.Orientation, s.Orientation)
	}
	if got.LinearVel != s.LinearVel {
		t.Errorf("Step changed LinearVel: got %v, want %v", got.LinearVel, s.LinearVel)
	}
	if got.AngularVel != s.AngularVel {
		t.Errorf("Step changed AngularVel: got %v, want %v", got.AngularVel, s.AngularVel)
	}
	if got.WheelLoad != s.WheelLoad {
		t.Errorf("Step changed WheelLoad: got %v, want %v", got.WheelLoad, s.WheelLoad)
	}
	if got.WheelSlip != s.WheelSlip {
		t.Errorf("Step changed WheelSlip: got %v, want %v", got.WheelSlip, s.WheelSlip)
	}
	if got.Grounded != s.Grounded {
		t.Errorf("Step changed Grounded: got %v, want %v", got.Grounded, s.Grounded)
	}
}
