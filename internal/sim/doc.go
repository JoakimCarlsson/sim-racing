// Package sim implements the server-authoritative 60 Hz tick loop and world
// state for sim-racing.
//
// Architecture rule: this package MUST NOT import internal/realtime. The
// InputSource interface allows realtime.Conn to satisfy the contract
// structurally without creating a hard dependency.
package sim
