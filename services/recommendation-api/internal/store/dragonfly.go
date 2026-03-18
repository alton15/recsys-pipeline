package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
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

// Close shuts down the underlying Redis client.
func (s *DragonflyStore) Close() error {
	return s.client.Close()
}
