package physics

// Step advances the vehicle simulation by dt seconds given the current State s,
// driver Input in, and vehicle Constants c.
//
// The function is pure: it has no side effects and produces the same output for
// identical inputs on every platform (determinism contract). Do not add I/O,
// logging, goroutines, or non-stdlib imports to this file.
//
// The returned State is a new value; s is not mutated.
func Step(s State, in Input, c Constants, dt float32) State {
	next := s

	// Commit the requested gear from the driver input.
	next.Gear = in.Gear

	// Forward speed along the vehicle's longitudinal axis (m/s).
	// LinearVel[2] is the Z (forward) component in vehicle-space.
	v := s.LinearVel[2]

	// Resolve the transmission ratio for the current gear.
	gearRatio := gearTransmissionRatio(in.Gear, c.GearRatios)

	// Compute engine torque via piecewise-linear interpolation of the curve.
	engineTorque := lookupTorque(c.TorqueCurve, s.RPM) * in.Throttle

	// Drive force at the contact patch (N). Zero in neutral (gearRatio == 0).
	var driveForce float32
	if gearRatio != 0 && c.WheelRadius > 0 {
		driveForce = engineTorque * gearRatio * c.FinalDrive *
			c.DrivetrainEfficiency / c.WheelRadius
	}

	// Aerodynamic drag: opposes velocity, proportional to v².
	// F_drag = -sign(v) * 0.5 * rho * Cd * A * v²
	dragForce := -sign32(v) * 0.5 * c.AirDensity * c.DragCoeff *
		c.FrontalArea * v * v

	// Rolling resistance: opposes velocity (N = Crr * m * g).
	const g = 9.81
	rollForce := -sign32(v) * c.RollingResistCoeff * c.Mass * g

	// Brake force: always opposes current velocity or drive force; never
	// reverses the car through braking.
	brakeForce := -sign32(v) * in.Brake * c.MaxBrakeForce
	// If the car is nearly stopped and braking, prevent the brake from
	// creating backward motion.
	if v == 0 {
		brakeForce = 0
	}

	// Net longitudinal force.
	netForce := driveForce + dragForce + rollForce + brakeForce

	// Semi-implicit Euler integration of forward velocity.
	accel := netForce / c.Mass
	vNew := v + accel*dt

	// Clamp: braking cannot reverse the car (wheels lock before reversal).
	if in.Brake > 0 && sign32(vNew) != sign32(v) && v != 0 {
		vNew = 0
	}

	next.LinearVel[2] = vNew

	// Update forward position.
	next.Position[2] = s.Position[2] + vNew*dt

	// Derive engine RPM from wheel speed and gear kinematics.
	// RPM = (|v_new| / wheelRadius) * |gearRatio| * finalDrive * (60 / 2π)
	const radsPerRevPerMin = 60.0 / (2.0 * 3.141592653589793)
	if c.WheelRadius > 0 && gearRatio != 0 {
		wheelAngularSpeed := abs32(vNew) / c.WheelRadius // rad/s
		engineAngularSpeed := wheelAngularSpeed * abs32(
			gearRatio,
		) * c.FinalDrive
		next.RPM = engineAngularSpeed * radsPerRevPerMin
	}

	// Clamp RPM to [idle, redline].
	next.RPM = clamp32(next.RPM, c.IdleRPM, c.RedlineRPM)

	return next
}

// gearTransmissionRatio resolves the GearRatios entry for the requested gear.
//
// GearRatios layout: index 0 = reverse, index 1 = neutral, index 2..N = forward.
// Input.Gear encoding: -1 = reverse, 0 = neutral, 1..N = forward gear number.
func gearTransmissionRatio(gear int8, ratios []float32) float32 {
	var idx int
	switch {
	case gear < 0:
		idx = 0 // reverse
	case gear == 0:
		idx = 1 // neutral
	default:
		idx = int(gear) + 1 // forward: gear 1 → index 2, etc.
	}
	if idx < 0 || idx >= len(ratios) {
		return 0
	}
	return ratios[idx]
}

// lookupTorque returns the interpolated engine torque (N·m) at the given RPM
// using the piecewise-linear curve stored in c.TorqueCurve.
// The curve must be sorted in ascending RPM order.
// The function performs zero heap allocations.
func lookupTorque(curve [8]TorqueSample, rpm float32) float32 {
	n := len(curve)
	if n == 0 {
		return 0
	}

	// Below first sample: clamp to first torque value.
	if rpm <= curve[0].RPM {
		return curve[0].Torque
	}

	// Above last sample: clamp to last torque value.
	if rpm >= curve[n-1].RPM {
		return curve[n-1].Torque
	}

	// Linear interpolation between surrounding samples.
	for i := 1; i < n; i++ {
		if rpm <= curve[i].RPM {
			lo := curve[i-1]
			hi := curve[i]
			t := (rpm - lo.RPM) / (hi.RPM - lo.RPM)
			return lo.Torque + t*(hi.Torque-lo.Torque)
		}
	}

	return curve[n-1].Torque
}

// sign32 returns +1 if x > 0, -1 if x < 0, and 0 if x == 0.
func sign32(x float32) float32 {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}

// abs32 returns the absolute value of x.
func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// clamp32 clamps x to [lo, hi].
func clamp32(x, lo, hi float32) float32 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}
