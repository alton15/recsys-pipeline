package degradation

import "sync/atomic"

// Level represents the current degradation level of the system.
type Level int32

const (
	Normal    Level = 0 // All tiers available
	Warning   Level = 1 // 150%+ load → disable Tier 3
	Critical  Level = 2 // 200%+ load → disable Tier 2 & 3
	Emergency Level = 3 // 300%+ load → CDN-only fallback
)

// Manager tracks and enforces system degradation levels using lock-free atomics.
type Manager struct {
	level atomic.Int32
}

// NewManager creates a Manager starting at Normal level.
func NewManager() *Manager {
	return &Manager{}
}

// SetLevel explicitly sets the degradation level.
func (m *Manager) SetLevel(l Level) {
	m.level.Store(int32(l))
}

// CurrentLevel returns the current degradation level.
func (m *Manager) CurrentLevel() Level {
	return Level(m.level.Load())
}

// ReportLoad auto-escalates or de-escalates based on the load ratio.
//   - ratio >= 3.0 → Emergency
//   - ratio >= 2.0 → Critical
//   - ratio >= 1.5 → Warning
//   - otherwise    → Normal
func (m *Manager) ReportLoad(ratio float64) {
	var newLevel Level
	switch {
	case ratio >= 3.0:
		newLevel = Emergency
	case ratio >= 2.0:
		newLevel = Critical
	case ratio >= 1.5:
		newLevel = Warning
	default:
		newLevel = Normal
	}
	m.level.Store(int32(newLevel))
}

// IsTierAllowed returns whether the given tier is allowed at the current degradation level.
//   - tier3: blocked at Warning and above
//   - tier2: blocked at Critical and above
//   - tier1: blocked at Emergency
func (m *Manager) IsTierAllowed(tierName string) bool {
	current := m.CurrentLevel()
	switch tierName {
	case "tier3":
		return current < Warning
	case "tier2":
		return current < Critical
	case "tier1":
		return current < Emergency
	default:
		return true
	}
}
