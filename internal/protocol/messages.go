package protocol

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

// MsgType is the first byte of every wire message.
type MsgType uint8

const (
	MsgClientInput    MsgType = 0x01
	MsgServerSnapshot MsgType = 0x02
	MsgServerHello    MsgType = 0x03
)

// ProtocolVersion is the current wire-format version (byte 1 of every message).
const ProtocolVersion uint8 = 1

// Sentinel errors.
var (
	ErrShortBuffer        = errors.New("protocol: buffer too short")
	ErrUnknownMsgType     = errors.New("protocol: unknown message type")
	ErrUnsupportedVersion = errors.New("protocol: unsupported protocol version")
)

// ---- size constants ---------------------------------------------------------

// headerSize is the 2-byte envelope present in every message.
const headerSize = 2

// stateSize is the packed byte size of physics.State on the wire.
// Field order mirrors the canonical order in docs/protocol.md:
//
//	Position[3]    3×float32 = 12
//	Orientation[4] 4×float32 = 16
//	LinearVel[3]   3×float32 = 12
//	AngularVel[3]  3×float32 = 12
//	RPM            float32   =  4
//	Gear           int8      =  1
//	WheelLoad[4]   4×float32 = 16
//	WheelSlip[4]   4×float32 = 16
//	Grounded       uint8     =  1
//	                       ------
//	                           90 bytes (packed, no alignment padding)
const stateSize = 90

// carStateSize is the packed size of a single CarState on the wire.
//
//	PlayerID  uint16  = 2
//	Nickname  [8]byte = 8
//	State             = 90
//	                  ---
//	                  100
const carStateSize = 2 + 8 + stateSize // 100

// ClientInputSize is the fixed byte length of a marshalled ClientInput.
//
//	header        = 2
//	Seq   uint32  = 4
//	ClientTimeMs uint32 = 4
//	Throttle float32 = 4
//	Brake    float32 = 4
//	Steer    float32 = 4
//	Gear     int8    = 1
//	Handbrake uint8  = 1
//	              ------
//	              24 bytes
const ClientInputSize = headerSize + 4 + 4 + 4 + 4 + 4 + 1 + 1 // 24

// serverSnapshotHeaderSize is the fixed portion of a ServerSnapshot frame.
//
//	header          = 2
//	ServerTickMs uint32 = 4
//	LastAckedSeq uint32 = 4
//	NumCars      uint8  = 1
//	                  ---
//	                  11 bytes
const serverSnapshotHeaderSize = headerSize + 4 + 4 + 1 // 11

// ServerSnapshotSize returns the total marshalled size for n cars.
func ServerSnapshotSize(n int) int {
	return serverSnapshotHeaderSize + n*carStateSize
}

// serverHelloFixedSize is the fixed part of ServerHello before GearRatios.
//
//	header             =  2
//	PlayerID   uint16  =  2
//	ServerTickHz uint8 =  1
//	SnapshotHz   uint8 =  1
//	Constants (fixed)  = 172
//	GearRatios len uint8 = 1
//	                    ---
//	                    179 + N×4
//
// Fixed Constants layout (172 bytes):
//
//	Mass               float32 =  4
//	Wheelbase          float32 =  4
//	TrackWidth         float32 =  4
//	MaxEngineTorque    float32 =  4
//	DragCoeff          float32 =  4
//	DownforceCoeff     float32 =  4
//	TireGripCoeffs[4]  4×f32  = 16
//	MaxSteerAngle      float32 =  4
//	TorqueCurve[8]     8×(2×f32)=64
//	FinalDrive         float32 =  4
//	DrivetrainEfficiency f32   =  4
//	WheelRadius        float32 =  4
//	RollingResistCoeff float32 =  4
//	FrontalArea        float32 =  4
//	AirDensity         float32 =  4
//	MaxBrakeForce      float32 =  4
//	IdleRPM            float32 =  4
//	RedlineRPM         float32 =  4
//	PacejkaB           float32 =  4
//	PacejkaC           float32 =  4
//	PacejkaD           float32 =  4
//	YawInertia         float32 =  4
//	WeightDistributionFront f32= 4
//	CGHeight           float32 =  4
//	RollStiffnessFront float32 =  4
//	                         ---
//	                         172
const constantsFixedSize = 172

// ServerHelloSize returns the total marshalled size for the given constants.
func ServerHelloSize(c *physics.Constants) int {
	return headerSize + 2 + 1 + 1 + constantsFixedSize + 1 + len(c.GearRatios)*4
}

// ---- types ------------------------------------------------------------------

// ClientInput is a single driver input frame sent from client to server.
type ClientInput struct {
	Seq          uint32
	ClientTimeMs uint32
	Input        physics.Input
}

// CarState is a single car's state as transmitted inside a ServerSnapshot.
type CarState struct {
	PlayerID uint16
	Nickname [8]byte
	State    physics.State
}

// ServerSnapshot is sent from server to client at SnapshotHz.
type ServerSnapshot struct {
	ServerTickMs uint32
	LastAckedSeq uint32
	Cars         []CarState
}

// ServerHello is sent from server to client on WebSocket connect.
type ServerHello struct {
	PlayerID     uint16
	ServerTickHz uint8
	SnapshotHz   uint8
	Constants    physics.Constants
}

// ---- helpers ----------------------------------------------------------------

// le is a package-level alias for the binary byte order used everywhere.
var le = binary.LittleEndian

func putF32(b []byte, v float32) {
	le.PutUint32(b, math.Float32bits(v))
}

func getF32(b []byte) float32 {
	return math.Float32frombits(le.Uint32(b))
}

// marshalState writes a physics.State into dst (must be ≥ stateSize bytes).
// Returns the number of bytes written (always stateSize).
func marshalState(dst []byte, s *physics.State) int {
	off := 0
	putF32(dst[off:], s.Position[0])
	off += 4
	putF32(dst[off:], s.Position[1])
	off += 4
	putF32(dst[off:], s.Position[2])
	off += 4
	putF32(dst[off:], s.Orientation[0])
	off += 4
	putF32(dst[off:], s.Orientation[1])
	off += 4
	putF32(dst[off:], s.Orientation[2])
	off += 4
	putF32(dst[off:], s.Orientation[3])
	off += 4
	putF32(dst[off:], s.LinearVel[0])
	off += 4
	putF32(dst[off:], s.LinearVel[1])
	off += 4
	putF32(dst[off:], s.LinearVel[2])
	off += 4
	putF32(dst[off:], s.AngularVel[0])
	off += 4
	putF32(dst[off:], s.AngularVel[1])
	off += 4
	putF32(dst[off:], s.AngularVel[2])
	off += 4
	putF32(dst[off:], s.RPM)
	off += 4
	dst[off] = uint8(s.Gear)
	off++
	putF32(dst[off:], s.WheelLoad[0])
	off += 4
	putF32(dst[off:], s.WheelLoad[1])
	off += 4
	putF32(dst[off:], s.WheelLoad[2])
	off += 4
	putF32(dst[off:], s.WheelLoad[3])
	off += 4
	putF32(dst[off:], s.WheelSlip[0])
	off += 4
	putF32(dst[off:], s.WheelSlip[1])
	off += 4
	putF32(dst[off:], s.WheelSlip[2])
	off += 4
	putF32(dst[off:], s.WheelSlip[3])
	off += 4
	dst[off] = s.Grounded
	off++
	return off // == stateSize
}

// unmarshalState reads a physics.State from src (must be ≥ stateSize bytes).
// Returns the number of bytes consumed (always stateSize).
func unmarshalState(src []byte, s *physics.State) int {
	off := 0
	s.Position[0] = getF32(src[off:])
	off += 4
	s.Position[1] = getF32(src[off:])
	off += 4
	s.Position[2] = getF32(src[off:])
	off += 4
	s.Orientation[0] = getF32(src[off:])
	off += 4
	s.Orientation[1] = getF32(src[off:])
	off += 4
	s.Orientation[2] = getF32(src[off:])
	off += 4
	s.Orientation[3] = getF32(src[off:])
	off += 4
	s.LinearVel[0] = getF32(src[off:])
	off += 4
	s.LinearVel[1] = getF32(src[off:])
	off += 4
	s.LinearVel[2] = getF32(src[off:])
	off += 4
	s.AngularVel[0] = getF32(src[off:])
	off += 4
	s.AngularVel[1] = getF32(src[off:])
	off += 4
	s.AngularVel[2] = getF32(src[off:])
	off += 4
	s.RPM = getF32(src[off:])
	off += 4
	s.Gear = int8(src[off])
	off++
	s.WheelLoad[0] = getF32(src[off:])
	off += 4
	s.WheelLoad[1] = getF32(src[off:])
	off += 4
	s.WheelLoad[2] = getF32(src[off:])
	off += 4
	s.WheelLoad[3] = getF32(src[off:])
	off += 4
	s.WheelSlip[0] = getF32(src[off:])
	off += 4
	s.WheelSlip[1] = getF32(src[off:])
	off += 4
	s.WheelSlip[2] = getF32(src[off:])
	off += 4
	s.WheelSlip[3] = getF32(src[off:])
	off += 4
	s.Grounded = src[off]
	off++
	return off // == stateSize
}

// ---- ClientInput ------------------------------------------------------------

// Marshal encodes m into buf. buf must be at least ClientInputSize bytes.
// Returns the number of bytes written and nil on success.
func (m *ClientInput) Marshal(buf []byte) (int, error) {
	if len(buf) < ClientInputSize {
		return 0, ErrShortBuffer
	}
	buf[0] = byte(MsgClientInput)
	buf[1] = ProtocolVersion
	le.PutUint32(buf[2:], m.Seq)
	le.PutUint32(buf[6:], m.ClientTimeMs)
	putF32(buf[10:], m.Input.Throttle)
	putF32(buf[14:], m.Input.Brake)
	putF32(buf[18:], m.Input.Steer)
	buf[22] = uint8(m.Input.Gear)
	if m.Input.Handbrake {
		buf[23] = 1
	} else {
		buf[23] = 0
	}
	return ClientInputSize, nil
}

// Unmarshal decodes a ClientInput from buf.
func (m *ClientInput) Unmarshal(buf []byte) error {
	if len(buf) < 2 {
		return ErrShortBuffer
	}
	if MsgType(buf[0]) != MsgClientInput {
		return ErrUnknownMsgType
	}
	if buf[1] != ProtocolVersion {
		return ErrUnsupportedVersion
	}
	if len(buf) < ClientInputSize {
		return ErrShortBuffer
	}
	m.Seq = le.Uint32(buf[2:])
	m.ClientTimeMs = le.Uint32(buf[6:])
	m.Input.Throttle = getF32(buf[10:])
	m.Input.Brake = getF32(buf[14:])
	m.Input.Steer = getF32(buf[18:])
	m.Input.Gear = int8(buf[22])
	m.Input.Handbrake = buf[23] != 0
	return nil
}

// ---- ServerSnapshot ---------------------------------------------------------

// Marshal encodes m into buf. buf must be at least ServerSnapshotSize(len(m.Cars)).
func (m *ServerSnapshot) Marshal(buf []byte) (int, error) {
	need := ServerSnapshotSize(len(m.Cars))
	if len(buf) < need {
		return 0, ErrShortBuffer
	}
	buf[0] = byte(MsgServerSnapshot)
	buf[1] = ProtocolVersion
	le.PutUint32(buf[2:], m.ServerTickMs)
	le.PutUint32(buf[6:], m.LastAckedSeq)
	buf[10] = uint8(len(m.Cars))
	off := serverSnapshotHeaderSize
	for i := range m.Cars {
		car := &m.Cars[i]
		le.PutUint16(buf[off:], car.PlayerID)
		off += 2
		copy(buf[off:off+8], car.Nickname[:])
		off += 8
		off += marshalState(buf[off:], &car.State)
	}
	return off, nil
}

// Unmarshal decodes a ServerSnapshot from buf.
// The caller must pre-size m.Cars to the expected number of cars to avoid
// allocations; Unmarshal will decode exactly min(numCars, len(m.Cars)) entries.
func (m *ServerSnapshot) Unmarshal(buf []byte) error {
	if len(buf) < 2 {
		return ErrShortBuffer
	}
	if MsgType(buf[0]) != MsgServerSnapshot {
		return ErrUnknownMsgType
	}
	if buf[1] != ProtocolVersion {
		return ErrUnsupportedVersion
	}
	if len(buf) < serverSnapshotHeaderSize {
		return ErrShortBuffer
	}
	m.ServerTickMs = le.Uint32(buf[2:])
	m.LastAckedSeq = le.Uint32(buf[6:])
	numCars := int(buf[10])
	need := serverSnapshotHeaderSize + numCars*carStateSize
	if len(buf) < need {
		return ErrShortBuffer
	}
	off := serverSnapshotHeaderSize
	decode := numCars
	if len(m.Cars) < decode {
		decode = len(m.Cars)
	}
	for i := 0; i < decode; i++ {
		car := &m.Cars[i]
		car.PlayerID = le.Uint16(buf[off:])
		off += 2
		copy(car.Nickname[:], buf[off:off+8])
		off += 8
		off += unmarshalState(buf[off:], &car.State)
	}
	return nil
}

// ---- ServerHello ------------------------------------------------------------

// Marshal encodes m into buf.
func (m *ServerHello) Marshal(buf []byte) (int, error) {
	need := ServerHelloSize(&m.Constants)
	if len(buf) < need {
		return 0, ErrShortBuffer
	}
	buf[0] = byte(MsgServerHello)
	buf[1] = ProtocolVersion
	le.PutUint16(buf[2:], m.PlayerID)
	buf[4] = m.ServerTickHz
	buf[5] = m.SnapshotHz

	// Constants (fixed 172 bytes).
	off := 6
	c := &m.Constants
	putF32(buf[off:], c.Mass)
	off += 4
	putF32(buf[off:], c.Wheelbase)
	off += 4
	putF32(buf[off:], c.TrackWidth)
	off += 4
	putF32(buf[off:], c.MaxEngineTorque)
	off += 4
	putF32(buf[off:], c.DragCoeff)
	off += 4
	putF32(buf[off:], c.DownforceCoeff)
	off += 4
	putF32(buf[off:], c.TireGripCoeffs[0])
	off += 4
	putF32(buf[off:], c.TireGripCoeffs[1])
	off += 4
	putF32(buf[off:], c.TireGripCoeffs[2])
	off += 4
	putF32(buf[off:], c.TireGripCoeffs[3])
	off += 4
	putF32(buf[off:], c.MaxSteerAngle)
	off += 4
	for _, ts := range c.TorqueCurve {
		putF32(buf[off:], ts.RPM)
		off += 4
		putF32(buf[off:], ts.Torque)
		off += 4
	}
	putF32(buf[off:], c.FinalDrive)
	off += 4
	putF32(buf[off:], c.DrivetrainEfficiency)
	off += 4
	putF32(buf[off:], c.WheelRadius)
	off += 4
	putF32(buf[off:], c.RollingResistCoeff)
	off += 4
	putF32(buf[off:], c.FrontalArea)
	off += 4
	putF32(buf[off:], c.AirDensity)
	off += 4
	putF32(buf[off:], c.MaxBrakeForce)
	off += 4
	putF32(buf[off:], c.IdleRPM)
	off += 4
	putF32(buf[off:], c.RedlineRPM)
	off += 4
	putF32(buf[off:], c.PacejkaB)
	off += 4
	putF32(buf[off:], c.PacejkaC)
	off += 4
	putF32(buf[off:], c.PacejkaD)
	off += 4
	putF32(buf[off:], c.YawInertia)
	off += 4
	putF32(buf[off:], c.WeightDistributionFront)
	off += 4
	putF32(buf[off:], c.CGHeight)
	off += 4
	putF32(buf[off:], c.RollStiffnessFront)
	off += 4

	// GearRatios: uint8 length prefix + N×float32.
	n := len(c.GearRatios)
	if n > 255 {
		n = 255
	}
	buf[off] = uint8(n)
	off++
	for i := 0; i < n; i++ {
		putF32(buf[off:], c.GearRatios[i])
		off += 4
	}
	return off, nil
}

// Unmarshal decodes a ServerHello from buf.
func (m *ServerHello) Unmarshal(buf []byte) error {
	if len(buf) < 2 {
		return ErrShortBuffer
	}
	if MsgType(buf[0]) != MsgServerHello {
		return ErrUnknownMsgType
	}
	if buf[1] != ProtocolVersion {
		return ErrUnsupportedVersion
	}
	// Minimum: header(2) + playerID(2) + tickHz(1) + snapHz(1) +
	//          constants(172) + gearRatiosLen(1) = 179 bytes.
	if len(buf) < 179 {
		return ErrShortBuffer
	}
	m.PlayerID = le.Uint16(buf[2:])
	m.ServerTickHz = buf[4]
	m.SnapshotHz = buf[5]

	off := 6
	c := &m.Constants
	c.Mass = getF32(buf[off:])
	off += 4
	c.Wheelbase = getF32(buf[off:])
	off += 4
	c.TrackWidth = getF32(buf[off:])
	off += 4
	c.MaxEngineTorque = getF32(buf[off:])
	off += 4
	c.DragCoeff = getF32(buf[off:])
	off += 4
	c.DownforceCoeff = getF32(buf[off:])
	off += 4
	c.TireGripCoeffs[0] = getF32(buf[off:])
	off += 4
	c.TireGripCoeffs[1] = getF32(buf[off:])
	off += 4
	c.TireGripCoeffs[2] = getF32(buf[off:])
	off += 4
	c.TireGripCoeffs[3] = getF32(buf[off:])
	off += 4
	c.MaxSteerAngle = getF32(buf[off:])
	off += 4
	for i := range c.TorqueCurve {
		c.TorqueCurve[i].RPM = getF32(buf[off:])
		off += 4
		c.TorqueCurve[i].Torque = getF32(buf[off:])
		off += 4
	}
	c.FinalDrive = getF32(buf[off:])
	off += 4
	c.DrivetrainEfficiency = getF32(buf[off:])
	off += 4
	c.WheelRadius = getF32(buf[off:])
	off += 4
	c.RollingResistCoeff = getF32(buf[off:])
	off += 4
	c.FrontalArea = getF32(buf[off:])
	off += 4
	c.AirDensity = getF32(buf[off:])
	off += 4
	c.MaxBrakeForce = getF32(buf[off:])
	off += 4
	c.IdleRPM = getF32(buf[off:])
	off += 4
	c.RedlineRPM = getF32(buf[off:])
	off += 4
	c.PacejkaB = getF32(buf[off:])
	off += 4
	c.PacejkaC = getF32(buf[off:])
	off += 4
	c.PacejkaD = getF32(buf[off:])
	off += 4
	c.YawInertia = getF32(buf[off:])
	off += 4
	c.WeightDistributionFront = getF32(buf[off:])
	off += 4
	c.CGHeight = getF32(buf[off:])
	off += 4
	c.RollStiffnessFront = getF32(buf[off:])
	off += 4

	n := int(buf[off])
	off++
	need := off + n*4
	if len(buf) < need {
		return ErrShortBuffer
	}
	c.GearRatios = make([]float32, n)
	for i := range c.GearRatios {
		c.GearRatios[i] = getF32(buf[off:])
		off += 4
	}
	return nil
}
