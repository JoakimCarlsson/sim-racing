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
	// MaxEngineTorque is the peak crankshaft torque (N·m).
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
}

// DefaultConstants provides a reasonable starting-point vehicle configuration
// suitable for a light sports car on a dry circuit.
var DefaultConstants = Constants{
	Mass:            1200,
	Wheelbase:       2.6,
	TrackWidth:      1.55,
	MaxEngineTorque: 400,
	DragCoeff:       0.32,
	DownforceCoeff:  0.15,
	TireGripCoeffs:  [4]float32{1.4, 1.4, 1.4, 1.4},
	GearRatios:      []float32{-3.2, 0, 3.5, 2.1, 1.4, 1.0, 0.8},
	MaxSteerAngle:   0.6,
}
