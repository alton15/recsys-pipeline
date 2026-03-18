package rerank_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/recsys-pipeline/recommendation-api/internal/rerank"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return mr, client
}

func seedEvent(t *testing.T, mr *miniredis.Miniredis, sessionID string, ts float64, eventType, itemID string) {
	t.Helper()
	evt := map[string]string{
		"event_type": eventType,
		"item_id":    itemID,
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	key := "session:" + sessionID + ":events"
	mr.ZAdd(key, ts, string(data))
}

func TestSessionFeatures_ExtractsClickedItems(t *testing.T) {
	mr, client := setupMiniredis(t)

	now := float64(time.Now().Unix())
	seedEvent(t, mr, "sess1", now-10, "click", "item_a")
	seedEvent(t, mr, "sess1", now-5, "click", "item_b")

	extractor := rerank.NewSessionFeatureExtractor(client)
	features, err := extractor.Extract("sess1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(features.ClickedItems) != 2 {
		t.Errorf("expected 2 clicked items, got %d", len(features.ClickedItems))
	}
	if !features.ClickedItems["item_a"] {
		t.Error("expected item_a in ClickedItems")
	}
	if !features.ClickedItems["item_b"] {
		t.Error("expected item_b in ClickedItems")
	}
	if features.SessionLength != 2 {
		t.Errorf("expected SessionLength 2, got %d", features.SessionLength)
	}
}

func TestSessionFeatures_EmptySession(t *testing.T) {
	_, client := setupMiniredis(t)

	extractor := rerank.NewSessionFeatureExtractor(client)
	features, err := extractor.Extract("empty_session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(features.ClickedItems) != 0 {
		t.Errorf("expected empty ClickedItems, got %d", len(features.ClickedItems))
	}
	if len(features.ViewedItems) != 0 {
		t.Errorf("expected empty ViewedItems, got %d", len(features.ViewedItems))
	}
	if features.SessionLength != 0 {
		t.Errorf("expected SessionLength 0, got %d", features.SessionLength)
	}
}

func TestSessionFeatures_MixedEvents(t *testing.T) {
	mr, client := setupMiniredis(t)

	now := float64(time.Now().Unix())
	seedEvent(t, mr, "sess2", now-30, "view", "item_x")
	seedEvent(t, mr, "sess2", now-20, "click", "item_y")
	seedEvent(t, mr, "sess2", now-10, "view", "item_z")

	extractor := rerank.NewSessionFeatureExtractor(client)
	features, err := extractor.Extract("sess2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(features.ClickedItems) != 1 {
		t.Errorf("expected 1 clicked item, got %d", len(features.ClickedItems))
	}
	if !features.ClickedItems["item_y"] {
		t.Error("expected item_y in ClickedItems")
	}
	if len(features.ViewedItems) != 2 {
		t.Errorf("expected 2 viewed items, got %d", len(features.ViewedItems))
	}
	if !features.ViewedItems["item_x"] {
		t.Error("expected item_x in ViewedItems")
	}
	if !features.ViewedItems["item_z"] {
		t.Error("expected item_z in ViewedItems")
	}
	if features.SessionLength != 3 {
		t.Errorf("expected SessionLength 3, got %d", features.SessionLength)
	}
}
