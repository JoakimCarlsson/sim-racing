package health_test

import (
	"testing"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/health"
)

func TestStatus_ReturnsOK(t *testing.T) {
	svc := health.New(time.Now(), "v1.2.3")
	result := svc.Status()

	if !result.OK {
		t.Error("expected OK=true")
	}
	if result.Version != "v1.2.3" {
		t.Errorf("expected version v1.2.3, got %q", result.Version)
	}
}

func TestStatus_UptimeSeconds_NonNegative(t *testing.T) {
	svc := health.New(time.Now().Add(-5*time.Second), "dev")
	result := svc.Status()

	if result.UptimeSeconds < 0 {
		t.Errorf("expected uptime >= 0, got %d", result.UptimeSeconds)
	}
}

func TestStatus_UptimeSeconds_Increases(t *testing.T) {
	startedAt := time.Now().Add(-10 * time.Second)
	svc := health.New(startedAt, "dev")
	result := svc.Status()

	if result.UptimeSeconds < 9 {
		t.Errorf(
			"expected uptime >= 9s for 10s-old service, got %d",
			result.UptimeSeconds,
		)
	}
}
