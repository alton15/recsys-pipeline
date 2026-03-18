package degradation_test

import (
	"testing"

	"github.com/recsys-pipeline/recommendation-api/internal/degradation"
)

func TestDegradation_NormalAllowsAllTiers(t *testing.T) {
	m := degradation.NewManager()
	m.SetLevel(degradation.Normal)

	for _, tier := range []string{"tier1", "tier2", "tier3"} {
		if !m.IsTierAllowed(tier) {
			t.Errorf("Normal level should allow %s", tier)
		}
	}
}

func TestDegradation_WarningDisablesTier3(t *testing.T) {
	m := degradation.NewManager()
	m.SetLevel(degradation.Warning)

	if !m.IsTierAllowed("tier1") {
		t.Error("Warning level should allow tier1")
	}
	if !m.IsTierAllowed("tier2") {
		t.Error("Warning level should allow tier2")
	}
	if m.IsTierAllowed("tier3") {
		t.Error("Warning level should disable tier3")
	}
}

func TestDegradation_CriticalDisablesTier2And3(t *testing.T) {
	m := degradation.NewManager()
	m.SetLevel(degradation.Critical)

	if !m.IsTierAllowed("tier1") {
		t.Error("Critical level should allow tier1")
	}
	if m.IsTierAllowed("tier2") {
		t.Error("Critical level should disable tier2")
	}
	if m.IsTierAllowed("tier3") {
		t.Error("Critical level should disable tier3")
	}
}

func TestDegradation_EmergencyDisablesAll(t *testing.T) {
	m := degradation.NewManager()
	m.SetLevel(degradation.Emergency)

	for _, tier := range []string{"tier1", "tier2", "tier3"} {
		if m.IsTierAllowed(tier) {
			t.Errorf("Emergency level should disable %s", tier)
		}
	}
}

func TestDegradation_AutoEscalatesOnLoad(t *testing.T) {
	m := degradation.NewManager()

	m.ReportLoad(1.0)
	if m.CurrentLevel() != degradation.Normal {
		t.Errorf("load 1.0 should be Normal, got %d", m.CurrentLevel())
	}

	m.ReportLoad(1.5)
	if m.CurrentLevel() != degradation.Warning {
		t.Errorf("load 1.5 should be Warning, got %d", m.CurrentLevel())
	}

	m.ReportLoad(2.0)
	if m.CurrentLevel() != degradation.Critical {
		t.Errorf("load 2.0 should be Critical, got %d", m.CurrentLevel())
	}

	m.ReportLoad(3.0)
	if m.CurrentLevel() != degradation.Emergency {
		t.Errorf("load 3.0 should be Emergency, got %d", m.CurrentLevel())
	}

	// De-escalate back to Normal.
	m.ReportLoad(0.5)
	if m.CurrentLevel() != degradation.Normal {
		t.Errorf("load 0.5 should de-escalate to Normal, got %d", m.CurrentLevel())
	}
}
