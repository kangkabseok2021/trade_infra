package gate_test

import (
	"testing"
	"time"

	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/gate"
)

type mockRisk struct{ exposure float64 }

func (m *mockRisk) LatestNetExposure(_ string) (float64, error) { return m.exposure, nil }

type mockSignals struct{ latest *time.Time }

func (m *mockSignals) LatestSubmitted(_, _ string) (*time.Time, error) { return m.latest, nil }

func TestGate_AllowsWhenUnderLimit(t *testing.T) {
	g := gate.New(&mockRisk{exposure: 10.0}, &mockSignals{}, 40.0, 30)
	allowed, reason := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if !allowed {
		t.Errorf("want allowed, got skipped: %s", reason)
	}
}

func TestGate_BlocksWhenAtLimit(t *testing.T) {
	// 36 + 5 = 41 >= 40 → blocked
	g := gate.New(&mockRisk{exposure: 36.0}, &mockSignals{}, 40.0, 30)
	allowed, reason := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if allowed {
		t.Error("want blocked at limit")
	}
	if reason != "risk_limit" {
		t.Errorf("want reason=risk_limit got %s", reason)
	}
}

func TestGate_BlocksOnActiveCooldown(t *testing.T) {
	recent := time.Now().Add(-10 * time.Second) // 10s ago, cooldown=30
	g := gate.New(&mockRisk{exposure: 5.0}, &mockSignals{latest: &recent}, 40.0, 30)
	allowed, reason := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if allowed {
		t.Error("want blocked on cooldown")
	}
	if reason != "cooldown" {
		t.Errorf("want reason=cooldown got %s", reason)
	}
}

func TestGate_AllowsAfterCooldownExpires(t *testing.T) {
	old := time.Now().Add(-60 * time.Second) // 60s ago, cooldown=30 → expired
	g := gate.New(&mockRisk{exposure: 5.0}, &mockSignals{latest: &old}, 40.0, 30)
	allowed, _ := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if !allowed {
		t.Error("want allowed after cooldown expires")
	}
}

func TestGate_AllowsWithNoSubmittedHistory(t *testing.T) {
	g := gate.New(&mockRisk{exposure: 0.0}, &mockSignals{latest: nil}, 40.0, 30)
	allowed, _ := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if !allowed {
		t.Error("want allowed with no prior submissions")
	}
}
