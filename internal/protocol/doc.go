// Package protocol defines the binary wire format for the sim-racing realtime
// channel. All multi-byte integers and floats are little-endian.
//
// Message envelope
//
//	byte 0: MsgType  (0x01 ClientInput | 0x02 ServerSnapshot | 0x03 ServerHello)
//	byte 1: ProtocolVersion (currently 1)
//	bytes 2+: message payload
//
// See docs/protocol.md for per-message byte-layout tables.
package protocol
