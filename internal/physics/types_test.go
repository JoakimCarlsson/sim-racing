package physics_test

import (
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

// TestDefaultConstantsValid asserts that DefaultConstants has sensible non-zero
// values for all scalar fields, including the new longitudinal parameters.
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
		t.Errorf(
			"DefaultConstants.MaxEngineTorque = %v, want > 0",
			c.MaxEngineTorque,
		)
	}
	if c.DragCoeff <= 0 {
		t.Errorf("DefaultConstants.DragCoeff = %v, want > 0", c.DragCoeff)
	}
	if c.DownforceCoeff <= 0 {
		t.Errorf(
			"DefaultConstants.DownforceCoeff = %v, want > 0",
			c.DownforceCoeff,
		)
	}
	if len(c.GearRatios) == 0 {
		t.Error("DefaultConstants.GearRatios must be non-empty")
	}
	if c.MaxSteerAngle <= 0 {
		t.Errorf(
			"DefaultConstants.MaxSteerAngle = %v, want > 0",
			c.MaxSteerAngle,
		)
	}

	// Longitudinal parameter checks.
	if c.FinalDrive <= 0 {
		t.Errorf("DefaultConstants.FinalDrive = %v, want > 0", c.FinalDrive)
	}
	if c.DrivetrainEfficiency <= 0 || c.DrivetrainEfficiency > 1 {
		t.Errorf(
			"DefaultConstants.DrivetrainEfficiency = %v, want (0, 1]",
			c.DrivetrainEfficiency,
		)
	}
	if c.WheelRadius <= 0 {
		t.Errorf("DefaultConstants.WheelRadius = %v, want > 0", c.WheelRadius)
	}
	if c.RollingResistCoeff <= 0 {
		t.Errorf(
			"DefaultConstants.RollingResistCoeff = %v, want > 0",
			c.RollingResistCoeff,
		)
	}
	if c.FrontalArea <= 0 {
		t.Errorf("DefaultConstants.FrontalArea = %v, want > 0", c.FrontalArea)
	}
	if c.AirDensity <= 0 {
		t.Errorf("DefaultConstants.AirDensity = %v, want > 0", c.AirDensity)
	}
	if c.MaxBrakeForce <= 0 {
		t.Errorf(
			"DefaultConstants.MaxBrakeForce = %v, want > 0",
			c.MaxBrakeForce,
		)
	}
	if c.IdleRPM <= 0 {
		t.Errorf("DefaultConstants.IdleRPM = %v, want > 0", c.IdleRPM)
	}
	if c.RedlineRPM <= c.IdleRPM {
		t.Errorf(
			"DefaultConstants.RedlineRPM = %v, want > IdleRPM (%v)",
			c.RedlineRPM, c.IdleRPM,
		)
	}

	// Torque curve must be in ascending RPM order and have non-zero torques.
	for i, s := range c.TorqueCurve {
		if s.Torque <= 0 {
			t.Errorf("TorqueCurve[%d].Torque = %v, want > 0", i, s.Torque)
		}
		if i > 0 && s.RPM <= c.TorqueCurve[i-1].RPM {
			t.Errorf(
				"TorqueCurve not in ascending RPM order at index %d: %.0f <= %.0f",
				i,
				s.RPM,
				c.TorqueCurve[i-1].RPM,
			)
		}
	}
}

// TestZeroValueTypes verifies that zero-value Input and State can be constructed
// without panics.
func TestZeroValueTypes(t *testing.T) {
	var in physics.Input
	var s physics.State
	_ = in
	_ = s
}
