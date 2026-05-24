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
	return s
}
