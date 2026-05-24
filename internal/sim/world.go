package sim

import (
	"sync"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
)

// PlayerID is the on-wire player identifier.
type PlayerID = uint16

// InputSource is satisfied by any type that can drain buffered client inputs
// into the provided slice. realtime.Conn satisfies this structurally.
type InputSource interface {
	DrainInputs(dst []protocol.ClientInput) int
}

// PlayerSim holds the mutable simulation state for a single connected player.
type PlayerSim struct {
	ID             PlayerID
	Source         InputSource
	State          physics.State
	Constants      physics.Constants
	LastAppliedSeq uint32
}

// World holds the collection of all active PlayerSims and the simulation
// configuration.
type World struct {
	mu               sync.Mutex
	players          map[PlayerID]*PlayerSim
	TickHz           int
	MaxInputsPerTick int
}

// Snapshot is an immutable copy of a PlayerSim used for broadcasting.
type Snapshot struct {
	ID    PlayerID
	State physics.State
}

// New creates a World with default parameters (60 Hz, 16 inputs/tick).
func New() *World {
	return &World{
		players:          make(map[PlayerID]*PlayerSim),
		TickHz:           60,
		MaxInputsPerTick: 16,
	}
}

// AddPlayer registers a new player in the world. If a player with the same ID
// already exists it is replaced.
func (w *World) AddPlayer(p *PlayerSim) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.players[p.ID] = p
}

// RemovePlayer removes a player by ID. It is a no-op when the ID is not
// present.
func (w *World) RemovePlayer(id PlayerID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.players, id)
}

// Get returns the PlayerSim for id, or nil when not found. The returned
// pointer must not be retained by the caller after releasing the lock (this
// helper is intended for tests only).
func (w *World) Get(id PlayerID) *PlayerSim {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.players[id]
}

// Snapshot returns a slice of immutable snapshots for all current players.
// The slice is freshly allocated on each call.
func (w *World) Snapshots() []Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Snapshot, 0, len(w.players))
	for _, p := range w.players {
		out = append(out, Snapshot{ID: p.ID, State: p.State})
	}
	return out
}
