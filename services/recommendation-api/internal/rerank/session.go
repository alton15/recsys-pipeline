package rerank

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/recsys-pipeline/shared/keys"
)

// SessionFeatures holds extracted features from a user's session events.
type SessionFeatures struct {
	ClickedItems  map[string]bool
	ViewedItems   map[string]bool
	SessionLength int
	LastEventAge  time.Duration
}

// sessionEvent represents a single event stored in the session sorted set.
type sessionEvent struct {
	EventType string `json:"event_type"`
	ItemID    string `json:"item_id"`
}

// SessionFeatureExtractor reads session events from DragonflyDB and produces
// SessionFeatures for use by the weighted scorer.
type SessionFeatureExtractor struct {
	client *redis.Client
}

// NewSessionFeatureExtractor creates a new extractor backed by the given Redis client.
func NewSessionFeatureExtractor(client *redis.Client) *SessionFeatureExtractor {
	return &SessionFeatureExtractor{client: client}
}

// Extract reads the most recent 50 session events and populates SessionFeatures.
func (e *SessionFeatureExtractor) Extract(sessionID string) (*SessionFeatures, error) {
	ctx := context.Background()
	key := keys.SessionEvents(sessionID)

	results, err := e.client.ZRevRangeWithScores(ctx, key, 0, 49).Result()
	if err != nil {
		return nil, err
	}

	features := &SessionFeatures{
		ClickedItems: make(map[string]bool),
		ViewedItems:  make(map[string]bool),
	}

	if len(results) == 0 {
		return features, nil
	}

	features.SessionLength = len(results)

	// The first result is the most recent event (ZREVRANGE).
	latestTimestamp := results[0].Score
	features.LastEventAge = time.Since(time.Unix(int64(latestTimestamp), 0))

	for _, z := range results {
		member, ok := z.Member.(string)
		if !ok {
			continue
		}

		var evt sessionEvent
		if err := json.Unmarshal([]byte(member), &evt); err != nil {
			continue
		}

		switch evt.EventType {
		case "click":
			features.ClickedItems[evt.ItemID] = true
		case "view":
			features.ViewedItems[evt.ItemID] = true
		}
	}

	return features, nil
}
