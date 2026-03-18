package experiment

import "hash/fnv"

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
type Router struct {
	store Store
}

// NewRouter creates a Router backed by the given experiment store.
func NewRouter(store Store) *Router {
	return &Router{store: store}
}

// GetExperiment determines which experiment (if any) the user is assigned to.
// Returns nil, nil when the user falls into the control group (no experiment).
func (r *Router) GetExperiment(userID string) (*Experiment, error) {
	experiments, err := r.store.ListActiveExperiments()
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

// hashBucket maps a user ID to a bucket in [0, 100) using FNV-1a.
func hashBucket(userID string) int {
	h := fnv.New32a()
	h.Write([]byte(userID))
	return int(h.Sum32() % 100)
}
