// Package health provides the liveness-check domain logic.
// This package has no net/http dependency — it is pure domain.
package health

import "time"

// Result is the data returned by a health status check.
type Result struct {
	OK            bool
	Version       string
	UptimeSeconds int64
}

// Service computes server health at any point in time.
type Service struct {
	startedAt time.Time
	version   string
}

// New constructs a Service. startedAt is typically time.Now() at server boot.
func New(startedAt time.Time, version string) *Service {
	return &Service{startedAt: startedAt, version: version}
}

// Status returns the current health result.
func (s *Service) Status() Result {
	return Result{
		OK:            true,
		Version:       s.version,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
	}
}
