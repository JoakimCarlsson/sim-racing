package physics

// Input holds a single driver input frame sent from the client.
// All values are in normalised ranges unless otherwise noted.
type Input struct {
	// Throttle is the normalised throttle position in [0, 1].
	Throttle float32 `json:"throttle"`
	// Brake is the normalised brake pressure in [0, 1].
	Brake float32 `json:"brake"`
	// Steer is the normalised steering position in [-1, 1]; negative is left.
	Steer float32 `json:"steer"`
	// Gear is the currently requested gear: -1 reverse, 0 neutral, 1..N forward.
	Gear int8 `json:"gear"`
	// Handbrake indicates whether the handbrake is applied.
	Handbrake bool `json:"handbrake"`
}

// State is the complete, deterministic vehicle state at a single physics tick.
// Fields are laid out for direct binary serialisation (no padding reorder).
type State struct {
	// Position is the world-space position of the vehicle centre of mass (m).
	Position [3]float32 `json:"position"`
	// Orientation is a unit quaternion [x, y, z, w] representing the vehicle
	// orientation in world space.
	Orientation [4]float32 `json:"orientation"`
	// LinearVel is the centre-of-mass velocity in world space (m/s).
	LinearVel [3]float32 `json:"linearVel"`
	// AngularVel is the angular velocity about each world axis (rad/s).
	AngularVel [3]float32 `json:"angularVel"`
	// RPM is the engine crankshaft rotational speed (rev/min).
	RPM float32 `json:"rpm"`
	// Gear is the currently engaged gear: -1 reverse, 0 neutral, 1..N forward.
	Gear int8 `json:"gear"`
	// WheelLoad is the normal force on each wheel in order [FL, FR, RL, RR] (N).
	WheelLoad [4]float32 `json:"wheelLoad"`
	// WheelSlip is the combined slip ratio for each wheel [FL, FR, RL, RR].
	WheelSlip [4]float32 `json:"wheelSlip"`
	// Grounded is a bitmask: bit 0=FL, 1=FR, 2=RL, 3=RR; set when the wheel
	// is in contact with the track surface.
	Grounded uint8 `json:"grounded"`
}

// TorqueSample is a single (RPM, torque in N·m) point on the engine's
// power curve. The curve must be provided in ascending RPM order.
type TorqueSample struct {
	RPM    float32
	Torque float32
}

// Constants holds the vehicle parameters that are fixed for the lifetime of a
// session. Treat as read-only inside Step.
type Constants struct {
	// Mass is the total vehicle mass including driver (kg).
	Mass float32 `json:"mass"`
	// Wheelbase is the distance between front and rear axle centrelines (m).
	Wheelbase float32 `json:"wheelbase"`
	// TrackWidth is the lateral distance between left and right contact patches
	// on the same axle (m).
	TrackWidth float32 `json:"trackWidth"`
	// MaxEngineTorque is the peak crankshaft torque (N·m). Used as a fallback
	// when TorqueCurve is empty.
	MaxEngineTorque float32 `json:"maxEngineTorque"`
	// DragCoeff is the aerodynamic drag coefficient (dimensionless Cd).
	DragCoeff float32 `json:"dragCoeff"`
	// DownforceCoeff is the aerodynamic downforce coefficient (dimensionless Cl).
	DownforceCoeff float32 `json:"downforceCoeff"`
	// TireGripCoeffs contains the peak friction coefficient for each tyre in
	// order [FL, FR, RL, RR].
	TireGripCoeffs [4]float32 `json:"tireGripCoeffs"`
	// GearRatios contains the overall drive ratio for each gear. Index 0 is
	// reverse, index 1 is neutral, indices 2..N are forward gears.
	GearRatios []float32 `json:"gearRatios"`
	// MaxSteerAngle is the maximum front-wheel steer angle at the tyre (rad).
	MaxSteerAngle float32 `json:"maxSteerAngle"`

	// --- Longitudinal dynamics fields ---

	// TorqueCurve is an 8-entry piecewise-linear engine torque map indexed by
	// RPM. Entries must be in ascending RPM order. Step uses linear
	// interpolation between adjacent samples and clamps outside the range.
	// Using a fixed-size array keeps Step allocation-free.
	TorqueCurve [8]TorqueSample `json:"torqueCurve"`

	// FinalDrive is the differential/final-drive ratio (dimensionless).
	FinalDrive float32 `json:"finalDrive"`

	// DrivetrainEfficiency is the fraction of engine torque that reaches the
	// wheels, accounting for drivetrain losses (dimensionless, 0-1).
	DrivetrainEfficiency float32 `json:"drivetrainEfficiency"`

	// WheelRadius is the loaded tyre radius (m).
	WheelRadius float32 `json:"wheelRadius"`

	// RollingResistCoeff is the coefficient of rolling resistance (Crr,
	// dimensionless, typically 0.01-0.02 for road tyres).
	RollingResistCoeff float32 `json:"rollingResistCoeff"`

	// FrontalArea is the vehicle's frontal cross-section (m²) used in the
	// aerodynamic drag equation.
	FrontalArea float32 `json:"frontalArea"`

	// AirDensity is the ambient air density (kg/m³). Standard sea level is
	// 1.225 kg/m³.
	AirDensity float32 `json:"airDensity"`

	// MaxBrakeForce is the peak braking force deliverable at all four wheels
	// combined (N).
	MaxBrakeForce float32 `json:"maxBrakeForce"`

	// IdleRPM is the minimum engine speed at idle (rev/min).
	IdleRPM float32 `json:"idleRPM"`

	// RedlineRPM is the maximum safe engine speed (rev/min). RPM is clamped
	// to this value.
	RedlineRPM float32 `json:"redlineRPM"`
}

// DefaultConstants provides a reasonable starting-point vehicle configuration
// suitable for a ~1200 kg sports car on a dry circuit.
//
// Tuning targets:
//   - 0-100 km/h: 6-9 s (with simple up-shift at 6500 RPM)
//   - Top speed:  200-250 km/h
var DefaultConstants = Constants{
	Mass:            1200,
	Wheelbase:       2.6,
	TrackWidth:      1.55,
	MaxEngineTorque: 310,
	DragCoeff:       0.32,
	DownforceCoeff:  0.15,
	TireGripCoeffs:  [4]float32{1.4, 1.4, 1.4, 1.4},
	// Index 0 = reverse, index 1 = neutral, indices 2..7 = forward gears 1-6.
	GearRatios:    []float32{-2.9, 0, 2.5, 1.7, 1.2, 0.9, 0.72, 0.58},
	MaxSteerAngle: 0.6,

	// 8-sample torque curve; ascending RPM, peak ~310 N·m at 3500 RPM.
	TorqueCurve: [8]TorqueSample{
		{RPM: 900, Torque: 210},
		{RPM: 1500, Torque: 255},
		{RPM: 2500, Torque: 285},
		{RPM: 3500, Torque: 310},
		{RPM: 4500, Torque: 298},
		{RPM: 5500, Torque: 268},
		{RPM: 6500, Torque: 225},
		{RPM: 7000, Torque: 170},
	},

	FinalDrive:           3.0,
	DrivetrainEfficiency: 0.85,
	WheelRadius:          0.31,
	RollingResistCoeff:   0.015,
	FrontalArea:          2.2,
	AirDensity:           1.225,
	MaxBrakeForce:        14000, // ~1.2 g deceleration at 1200 kg
	IdleRPM:              900,
	RedlineRPM:           7000,
}
