//go:build js && wasm

// cmd/physicswasm exposes internal/physics to JavaScript via syscall/js.
//
// # Exported globals (on globalThis.simPhysics)
//
//   - step(stateBuf Uint8Array, inputBuf Uint8Array, dt float64) Uint8Array
//     Advances the vehicle simulation by dt seconds.  The returned buffer
//     encodes the next State in the same layout as stateBuf.
//
//   - defaultConstants(constsBuf Uint8Array) void
//     Fills constsBuf with the DefaultConstants encoding so the JS caller
//     does not need to hard-code values.
//
// # Buffer layouts (all values little-endian unless noted)
//
// stateBuf (96 bytes):
//
//	[  0: 12]  Position    [3]float32
//	[ 12: 28]  Orientation [4]float32
//	[ 28: 40]  LinearVel   [3]float32
//	[ 40: 52]  AngularVel  [3]float32
//	[ 52: 56]  RPM         float32
//	[ 56]      Gear        int8  (stored as uint8)
//	[ 57: 57]  _padding    (1 byte, ignored on read)
//	[ 58: 74]  WheelLoad   [4]float32
//	[ 74: 90]  WheelSlip   [4]float32
//	[ 90]      Grounded    uint8
//	[ 91: 96]  _padding    (5 bytes)
//
// inputBuf (20 bytes):
//
//	[  0:  4]  Seq       uint32 (ignored by Step; present for protocol compat)
//	[  4:  8]  Throttle  float32
//	[  8: 12]  Brake     float32
//	[ 12: 16]  Steer     float32
//	[ 16]      Gear      int8  (stored as uint8)
//	[ 17]      Handbrake uint8
//	[ 18: 20]  _padding
//
// SYNC NOTE: stateBuf field order MUST mirror hashStateStream in
// internal/physics/replay_test.go:
// Position[3], Orientation[4], LinearVel[3], AngularVel[3], RPM, Gear,
// WheelLoad[4], WheelSlip[4], Grounded.
package main

import (
	"encoding/binary"
	"math"
	"syscall/js"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

func main() {
	obj := js.Global().Get("Object").New()

	obj.Set("step", js.FuncOf(jsStep))
	obj.Set("defaultConstants", js.FuncOf(jsDefaultConstants))

	js.Global().Set("simPhysics", obj)

	// Block forever — the WASM module must stay alive.
	<-make(chan struct{})
}

// jsStep implements globalThis.simPhysics.step(stateBuf, inputBuf, dt).
// Returns a new Uint8Array containing the next state.
func jsStep(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		panic("simPhysics.step: expected (stateBuf, inputBuf, dt)")
	}
	stateBuf := jsUint8Array(args[0])
	inputBuf := jsUint8Array(args[1])
	dt := float32(args[2].Float())

	s := decodeState(stateBuf)
	in := decodeInput(inputBuf)

	next := physics.Step(s, in, physics.DefaultConstants, physics.FlatGround(0), dt)

	out := make([]byte, 96)
	encodeState(out, next)
	return uint8ArrayFromBytes(out)
}

// jsDefaultConstants implements globalThis.simPhysics.defaultConstants(constsBuf).
// Writes DefaultConstants into the provided Uint8Array.
// constsBuf must be at least 52 bytes.
//
// Layout (all float32 LE, except GearRatios count uint32 and GearRatios[8]):
//
//	[  0:  4]  Mass
//	[  4:  8]  Wheelbase
//	[  8: 12]  TrackWidth
//	[ 12: 16]  MaxEngineTorque
//	[ 16: 20]  DragCoeff
//	[ 20: 24]  DownforceCoeff
//	[ 24: 28]  MaxSteerAngle
//	[ 28: 32]  FinalDrive
//	[ 32: 36]  DrivetrainEfficiency
//	[ 36: 40]  WheelRadius
//	[ 40: 44]  RollingResistCoeff
//	[ 44: 48]  FrontalArea
//	[ 48: 52]  AirDensity
//	[ 52: 56]  MaxBrakeForce
//	[ 56: 60]  IdleRPM
//	[ 60: 64]  RedlineRPM
//	[ 64: 68]  PacejkaB
//	[ 68: 72]  PacejkaC
//	[ 72: 76]  PacejkaD
//	[ 76: 80]  YawInertia
//	[ 80: 84]  WeightDistributionFront
//	[ 84: 88]  CGHeight
//	[ 88: 92]  RollStiffnessFront
//	[ 92: 96]  GearRatios count (uint32)
//	[ 96:128]  GearRatios[8] float32 (padded with zeros if shorter)
func jsDefaultConstants(_ js.Value, args []js.Value) any {
	c := physics.DefaultConstants

	out := make([]byte, 128)
	putF32 := func(off int, v float32) {
		binary.LittleEndian.PutUint32(out[off:], math.Float32bits(v))
	}
	putU32 := func(off int, v uint32) {
		binary.LittleEndian.PutUint32(out[off:], v)
	}

	putF32(0, c.Mass)
	putF32(4, c.Wheelbase)
	putF32(8, c.TrackWidth)
	putF32(12, c.MaxEngineTorque)
	putF32(16, c.DragCoeff)
	putF32(20, c.DownforceCoeff)
	putF32(24, c.MaxSteerAngle)
	putF32(28, c.FinalDrive)
	putF32(32, c.DrivetrainEfficiency)
	putF32(36, c.WheelRadius)
	putF32(40, c.RollingResistCoeff)
	putF32(44, c.FrontalArea)
	putF32(48, c.AirDensity)
	putF32(52, c.MaxBrakeForce)
	putF32(56, c.IdleRPM)
	putF32(60, c.RedlineRPM)
	putF32(64, c.PacejkaB)
	putF32(68, c.PacejkaC)
	putF32(72, c.PacejkaD)
	putF32(76, c.YawInertia)
	putF32(80, c.WeightDistributionFront)
	putF32(84, c.CGHeight)
	putF32(88, c.RollStiffnessFront)

	n := len(c.GearRatios)
	if n > 8 {
		n = 8
	}
	putU32(92, uint32(len(c.GearRatios)))
	for i := 0; i < n; i++ {
		putF32(96+i*4, c.GearRatios[i])
	}

	// Write to the provided buffer if given, always return the bytes too.
	if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
		jsTA := args[0]
		for i, b := range out {
			jsTA.SetIndex(i, b)
		}
	}
	return uint8ArrayFromBytes(out)
}

// decodeState reads a physics.State from a 96-byte little-endian buffer.
//
// Field order (SYNC with hashStateStream):
//
//	Position[3], Orientation[4], LinearVel[3], AngularVel[3],
//	RPM, Gear, WheelLoad[4], WheelSlip[4], Grounded.
func decodeState(buf []byte) physics.State {
	var s physics.State
	getF32 := func(off int) float32 {
		return math.Float32frombits(binary.LittleEndian.Uint32(buf[off:]))
	}
	s.Position[0] = getF32(0)
	s.Position[1] = getF32(4)
	s.Position[2] = getF32(8)
	s.Orientation[0] = getF32(12)
	s.Orientation[1] = getF32(16)
	s.Orientation[2] = getF32(20)
	s.Orientation[3] = getF32(24)
	s.LinearVel[0] = getF32(28)
	s.LinearVel[1] = getF32(32)
	s.LinearVel[2] = getF32(36)
	s.AngularVel[0] = getF32(40)
	s.AngularVel[1] = getF32(44)
	s.AngularVel[2] = getF32(48)
	s.RPM = getF32(52)
	s.Gear = int8(buf[56])
	// byte 57 = padding
	s.WheelLoad[0] = getF32(58)
	s.WheelLoad[1] = getF32(62)
	s.WheelLoad[2] = getF32(66)
	s.WheelLoad[3] = getF32(70)
	s.WheelSlip[0] = getF32(74)
	s.WheelSlip[1] = getF32(78)
	s.WheelSlip[2] = getF32(82)
	s.WheelSlip[3] = getF32(86)
	s.Grounded = buf[90]
	return s
}

// encodeState writes a physics.State into a 96-byte little-endian buffer.
// Field order matches decodeState and hashStateStream.
func encodeState(buf []byte, s physics.State) {
	putF32 := func(off int, v float32) {
		binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(v))
	}
	putF32(0, s.Position[0])
	putF32(4, s.Position[1])
	putF32(8, s.Position[2])
	putF32(12, s.Orientation[0])
	putF32(16, s.Orientation[1])
	putF32(20, s.Orientation[2])
	putF32(24, s.Orientation[3])
	putF32(28, s.LinearVel[0])
	putF32(32, s.LinearVel[1])
	putF32(36, s.LinearVel[2])
	putF32(40, s.AngularVel[0])
	putF32(44, s.AngularVel[1])
	putF32(48, s.AngularVel[2])
	putF32(52, s.RPM)
	buf[56] = uint8(s.Gear)
	buf[57] = 0 // padding
	putF32(58, s.WheelLoad[0])
	putF32(62, s.WheelLoad[1])
	putF32(66, s.WheelLoad[2])
	putF32(70, s.WheelLoad[3])
	putF32(74, s.WheelSlip[0])
	putF32(78, s.WheelSlip[1])
	putF32(82, s.WheelSlip[2])
	putF32(86, s.WheelSlip[3])
	buf[90] = s.Grounded
	// bytes 91-95 = padding
}

// decodeInput reads a physics.Input from a 20-byte little-endian buffer
// (same layout as the fixture record in internal/physics/replay_test.go).
func decodeInput(buf []byte) physics.Input {
	var in physics.Input
	getF32 := func(off int) float32 {
		return math.Float32frombits(binary.LittleEndian.Uint32(buf[off:]))
	}
	// buf[0:4] = Seq (ignored)
	in.Throttle = getF32(4)
	in.Brake = getF32(8)
	in.Steer = getF32(12)
	in.Gear = int8(buf[16])
	in.Handbrake = buf[17] != 0
	return in
}

// jsUint8Array copies a JS Uint8Array into a Go []byte.
func jsUint8Array(v js.Value) []byte {
	n := v.Length()
	buf := make([]byte, n)
	js.CopyBytesToGo(buf, v)
	return buf
}

// uint8ArrayFromBytes creates a JS Uint8Array from a Go []byte.
func uint8ArrayFromBytes(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}
