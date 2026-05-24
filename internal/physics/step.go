package physics

import "math"

// Step advances the vehicle simulation by dt seconds given the current State s,
// driver Input in, and vehicle Constants c.
//
// The function is pure: it has no side effects and produces the same output for
// identical inputs on every platform (determinism contract). Do not add I/O,
// logging, goroutines, or non-stdlib imports to this file.
//
// The returned State is a new value; s is not mutated.
//
// # Coordinate convention
//
// LinearVel and AngularVel are in vehicle-space (body frame):
//
//	LinearVel[0] = lateral   (X, rightward)
//	LinearVel[1] = vertical  (Y, upward)
//	LinearVel[2] = forward   (Z, forward)
//	AngularVel[0] = pitch
//	AngularVel[1] = yaw  (positive = turning left when viewed from above in Y-up convention)
//	AngularVel[2] = roll
//
// Position and Orientation are in world frame.
func Step(s State, in Input, c Constants, dt float32) State {
	next := s

	// Commit the requested gear from the driver input.
	next.Gear = in.Gear

	// -------------------------------------------------------------------------
	// Longitudinal dynamics
	// -------------------------------------------------------------------------

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

	// -------------------------------------------------------------------------
	// Lateral dynamics — sideslip + yaw rate (bicycle model)
	// -------------------------------------------------------------------------
	//
	// State representation: (β, r) where β = body sideslip angle (rad) and
	// r = yaw rate (rad/s).  This avoids accumulating an unbounded lateral
	// velocity and gives a numerically stable first-order system.
	//
	// Equations of motion:
	//   m * v * (dβ/dt + r) = Fy_total          [lateral Newton]
	//   Iz * dr/dt           = N_z               [yaw moment]
	//
	// where v = total longitudinal speed, Fy_total = sum of per-axle lateral
	// tire forces (Pacejka), N_z = yaw torque.

	// Guard: Pacejka and slip-angle math require meaningful forward speed.
	// Below epsilon we zero out lateral dynamics to avoid divide-by-zero.
	const vLongEps = float32(0.5) // m/s

	// Total speed (use absolute forward velocity).
	vTotal := abs32(vNew)

	// Steer angle at the front wheels (rad).
	steerAngle := in.Steer * c.MaxSteerAngle

	// Weight distribution (default 0.5 if unset).
	wdf := c.WeightDistributionFront
	if wdf <= 0 || wdf >= 1 {
		wdf = 0.5
	}
	totalNormalForce := c.Mass * g
	frontLoad := totalNormalForce * wdf
	rearLoad := totalNormalForce * (1.0 - wdf)

	// Yaw rate and sideslip angle from previous state.
	yawRate := s.AngularVel[1]
	// Sideslip angle: atan2(v_lat, v_long).
	// Reconstruct from stored LinearVel (body frame).
	vLat := s.LinearVel[0]
	var beta float32
	if vTotal >= vLongEps {
		beta = atan32(vLat, abs32(s.LinearVel[2]))
	}

	// Axle distances from CoM (50/50 => Lf = Lr = Wheelbase/2).
	halfWB := c.Wheelbase / 2.0

	var Fy_front, Fy_rear float32
	var nextYawRate float32
	if vTotal >= vLongEps {
		// Per-axle slip angles using sideslip angle + yaw rate.
		// α_f = δ - β - (Lf * r) / v
		// α_r =     -β + (Lr * r) / v
		alphaFront := steerAngle - beta - (halfWB*yawRate)/vTotal
		alphaRear := -beta + (halfWB*yawRate)/vTotal

		Fy_front = pacejkaFy(
			alphaFront,
			frontLoad,
			c.PacejkaB,
			c.PacejkaC,
			c.PacejkaD,
		)
		Fy_rear = pacejkaFy(
			alphaRear,
			rearLoad,
			c.PacejkaB,
			c.PacejkaC,
			c.PacejkaD,
		)

		// Net lateral force and yaw torque.
		fyTotal := Fy_front + Fy_rear
		yawTorque := halfWB*Fy_front - halfWB*Fy_rear

		// Lateral equation: dβ/dt = Fy_total / (m * v) - r
		dBeta := fyTotal/(c.Mass*vTotal) - yawRate
		betaNew := beta + dBeta*dt

		// Update lateral velocity from new sideslip angle.
		next.LinearVel[0] = float32(math.Tan(float64(betaNew))) * vTotal

		// Yaw rate integration.
		iz := c.YawInertia
		if iz <= 0 {
			iz = 2000
		}
		yawAccel := yawTorque / iz
		nextYawRate = yawRate + yawAccel*dt
	} else {
		// Below speed threshold: zero lateral dynamics.
		next.LinearVel[0] = 0
		nextYawRate = 0
	}
	next.AngularVel[1] = nextYawRate

	// -------------------------------------------------------------------------
	// Orientation integration (quaternion)
	// -------------------------------------------------------------------------
	// dq/dt = 0.5 * q ⊗ [0, ω_x, ω_y, ω_z]
	// where ω is in body frame: pitch=AngularVel[0], yaw=AngularVel[1], roll=AngularVel[2].
	// We use the yaw component only (no pitch/roll in this model).
	qx := s.Orientation[0]
	qy := s.Orientation[1]
	qz := s.Orientation[2]
	qw := s.Orientation[3]

	wx := s.AngularVel[0]
	wy := nextYawRate
	wz := s.AngularVel[2]

	// Quaternion derivative: dq = 0.5 * q ⊗ ω_quat
	// ω_quat = [wx, wy, wz, 0] (pure quaternion)
	// q ⊗ p: [qw*px+qz*py-qy*pz+qx*pw,
	//         -qz*px+qw*py+qx*pz+qy*pw,
	//          qy*px-qx*py+qw*pz+qz*pw,
	//         -qx*px-qy*py-qz*pz+qw*pw]
	dqx := 0.5 * (qw*wx + qz*wy - qy*wz)
	dqy := 0.5 * (-qz*wx + qw*wy + qx*wz)
	dqz := 0.5 * (qy*wx - qx*wy + qw*wz)
	dqw := 0.5 * (-qx*wx - qy*wy - qz*wz)

	nqx := qx + dqx*dt
	nqy := qy + dqy*dt
	nqz := qz + dqz*dt
	nqw := qw + dqw*dt

	// Renormalise to prevent quaternion drift.
	qMag := float32(math.Sqrt(float64(nqx*nqx + nqy*nqy + nqz*nqz + nqw*nqw)))
	if qMag > 0 {
		invMag := 1.0 / qMag
		nqx *= invMag
		nqy *= invMag
		nqz *= invMag
		nqw *= invMag
	} else {
		// Degenerate: reset to identity.
		nqx, nqy, nqz, nqw = 0, 0, 0, 1
	}

	next.Orientation[0] = nqx
	next.Orientation[1] = nqy
	next.Orientation[2] = nqz
	next.Orientation[3] = nqw

	// -------------------------------------------------------------------------
	// World-frame position update
	// -------------------------------------------------------------------------
	// Rotate body-frame velocity through orientation quaternion to get
	// world-frame displacement: p_world += q * v_body * q*
	// For a unit quaternion q = [x, y, z, w], the rotation of vector v is:
	//   v' = q ⊗ [vx, vy, vz, 0] ⊗ q*
	// Computed efficiently as the sandwich product.
	vbx := next.LinearVel[0]
	vby := next.LinearVel[1]
	vbz := next.LinearVel[2]

	// t = 2 * cross(q.xyz, v)
	tx := 2 * (nqy*vbz - nqz*vby)
	ty := 2 * (nqz*vbx - nqx*vbz)
	tz := 2 * (nqx*vby - nqy*vbx)

	// v' = v + w * t + cross(q.xyz, t)  where w = nqw
	wxWorld := vbx + nqw*tx + (nqy*tz - nqz*ty)
	wyWorld := vby + nqw*ty + (nqz*tx - nqx*tz)
	wzWorld := vbz + nqw*tz + (nqx*ty - nqy*tx)

	next.Position[0] = s.Position[0] + wxWorld*dt
	next.Position[1] = s.Position[1] + wyWorld*dt
	next.Position[2] = s.Position[2] + wzWorld*dt

	return next
}

// pacejkaFy computes the lateral tyre force (N) for a single axle using the
// simplified Pacejka magic formula:
//
//	Fy = normalLoad * D * sin(C * atan(B * alpha))
//
// alpha is the slip angle in radians, normalLoad is the axle normal force (N).
// B, C, D are the stiffness, shape, and peak factors respectively.
func pacejkaFy(alpha, normalLoad, B, C, D float32) float32 {
	// Use stdlib math for sin/atan (float64 then cast back).
	x := float64(B * alpha)
	sinTerm := float32(math.Sin(float64(C) * math.Atan(x)))
	return normalLoad * D * sinTerm
}

// atan32 computes atan2(y, x) as float32 via stdlib math.
func atan32(y, x float32) float32 {
	return float32(math.Atan2(float64(y), float64(x)))
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
