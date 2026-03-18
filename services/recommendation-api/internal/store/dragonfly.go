package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/recsys-pipeline/recommendation-api/internal/experiment"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
	"github.com/recsys-pipeline/shared/keys"
)

// DragonflyStore reads pre-computed recommendations from DragonflyDB.
type DragonflyStore struct {
	client *redis.Client
}

// NewDragonflyStore creates a DragonflyStore connected to the given address.
func NewDragonflyStore(addr string) *DragonflyStore {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		ReadTimeout:  10 * time.Millisecond,
		WriteTimeout: 10 * time.Millisecond,
		PoolSize:     100,
	})
	return &DragonflyStore{client: client}
}

// GetRecommendations retrieves cached top-K recommendations for a user.
// Returns nil, nil on cache miss.
func (s *DragonflyStore) GetRecommendations(userID string) ([]tier.Recommendation, error) {
	ctx := context.Background()
	key := keys.RecTopK(userID)

	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var recs []tier.Recommendation
	if err := json.Unmarshal(data, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}

// GetPopularItems retrieves the global popular items list, truncated to limit.
// Returns nil, nil on cache miss.
func (s *DragonflyStore) GetPopularItems(limit int) ([]tier.Recommendation, error) {
	ctx := context.Background()
	key := keys.RecTopK("popular")

	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var recs []tier.Recommendation
	if err := json.Unmarshal(data, &recs); err != nil {
		return nil, err
	}

	if len(recs) > limit {
		recs = recs[:limit]
	}
	return recs, nil
}

// ListActiveExperiments retrieves all active A/B experiment configurations
// from DragonflyDB. Experiment IDs are stored in a set at "experiment:active",
// and each experiment config is stored as JSON at "experiment:<id>".
func (s *DragonflyStore) ListActiveExperiments() ([]experiment.Experiment, error) {
	ctx := context.Background()

	ids, err := s.client.SMembers(ctx, keys.ExperimentActiveList()).Result()
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return []experiment.Experiment{}, nil
	}

	// Fetch each experiment config.
	expKeys := make([]string, len(ids))
	for i, id := range ids {
		expKeys[i] = keys.ExperimentConfig(id)
	}

	vals, err := s.client.MGet(ctx, expKeys...).Result()
	if err != nil {
		return nil, err
	}

	experiments := make([]experiment.Experiment, 0, len(vals))
	for _, val := range vals {
		if val == nil {
			continue
		}
		str, ok := val.(string)
		if !ok {
			continue
		}
		var exp experiment.Experiment
		if err := json.Unmarshal([]byte(str), &exp); err != nil {
			continue
		}
		experiments = append(experiments, exp)
	}

	return experiments, nil
}

// Close shuts down the underlying Redis client.
func (s *DragonflyStore) Close() error {
	return s.client.Close()
}
