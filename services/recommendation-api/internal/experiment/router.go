package experiment

import (
	"hash/fnv"
	"sync"
	"time"
)

// Experiment represents an A/B test experiment configuration.
type Experiment struct {
	ID           string  `json:"id"`
	ModelVersion string  `json:"model_version"`
	TrafficPct   float64 `json:"traffic_pct"`
	TierConfig   string  `json:"tier_config,omitempty"`
}

// Store is the interface for retrieving active experiment configurations.
type Store interface {
	ListActiveExperiments() ([]Experiment, error)
}

// Router assigns users to experiments using consistent hashing.
// Experiment configurations are cached and refreshed periodically to
// avoid hitting DragonflyDB on every recommendation request.
type Router struct {
	store    Store
	cacheTTL time.Duration

	mu          sync.RWMutex
	cached      []Experiment
	lastFetched time.Time
}

// NewRouter creates a Router backed by the given experiment store.
// Experiments are cached for 30 seconds by default.
func NewRouter(store Store) *Router {
	return &Router{
		store:    store,
		cacheTTL: 30 * time.Second,
	}
}

// GetExperiment determines which experiment (if any) the user is assigned to.
// Returns nil, nil when the user falls into the control group (no experiment).
func (r *Router) GetExperiment(userID string) (*Experiment, error) {
	experiments, err := r.loadExperiments()
	if err != nil {
		return nil, err
	}

	bucket := hashBucket(userID) // 0-99

	cumulative := 0.0
	for _, exp := range experiments {
		cumulative += exp.TrafficPct
		if float64(bucket) < cumulative {
			// Return a copy to avoid callers mutating shared state.
			result := exp
			return &result, nil
		}
	}

	return nil, nil // control group
}

// loadExperiments returns cached experiments if still fresh, otherwise
// fetches from the store and updates the cache.
func (r *Router) loadExperiments() ([]Experiment, error) {
	r.mu.RLock()
	if r.cached != nil && time.Since(r.lastFetched) < r.cacheTTL {
		experiments := r.cached
		r.mu.RUnlock()
		return experiments, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check: another goroutine may have refreshed while we waited.
	if r.cached != nil && time.Since(r.lastFetched) < r.cacheTTL {
		return r.cached, nil
	}

	experiments, err := r.store.ListActiveExperiments()
	if err != nil {
		// On error, return stale cache if available.
		if r.cached != nil {
			return r.cached, nil
		}
		return nil, err
	}

	r.cached = experiments
	r.lastFetched = time.Now()
	return experiments, nil
}

// hashBucket maps a user ID to a bucket in [0, 100) using FNV-1a.
func hashBucket(userID string) int {
	h := fnv.New32a()
	h.Write([]byte(userID))
	return int(h.Sum32() % 100)
}
