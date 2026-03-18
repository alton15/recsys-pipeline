package store_test

import (
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/recsys-pipeline/recommendation-api/internal/store"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *store.DragonflyStore) {
	t.Helper()
	mr := miniredis.RunT(t)
	s := store.NewDragonflyStore(mr.Addr())
	return mr, s
}

func TestDragonflyStore_GetRecommendations(t *testing.T) {
	mr, s := setupMiniredis(t)

	recs := []tier.Recommendation{
		{ItemID: "item1", Score: 0.95},
		{ItemID: "item2", Score: 0.80},
		{ItemID: "item3", Score: 0.65},
	}
	data, _ := json.Marshal(recs)
	mr.Set("rec:user123:top_k", string(data))

	got, err := s.GetRecommendations("user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0].ItemID != "item1" {
		t.Errorf("expected first item item1, got %s", got[0].ItemID)
	}
	if got[0].Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", got[0].Score)
	}
}

func TestDragonflyStore_GetRecommendations_Miss(t *testing.T) {
	_, s := setupMiniredis(t)

	got, err := s.GetRecommendations("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error on cache miss: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil on cache miss, got %v", got)
	}
}

func TestDragonflyStore_GetPopularItems(t *testing.T) {
	mr, s := setupMiniredis(t)

	popular := []tier.Recommendation{
		{ItemID: "pop1", Score: 1000},
		{ItemID: "pop2", Score: 900},
		{ItemID: "pop3", Score: 800},
		{ItemID: "pop4", Score: 700},
		{ItemID: "pop5", Score: 600},
	}
	data, _ := json.Marshal(popular)
	mr.Set("rec:popular:top_k", string(data))

	got, err := s.GetPopularItems(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 items after truncation, got %d", len(got))
	}
	if got[2].ItemID != "pop3" {
		t.Errorf("expected third item pop3, got %s", got[2].ItemID)
	}
}

func TestDragonflyStore_GetPopularItems_Miss(t *testing.T) {
	_, s := setupMiniredis(t)

	got, err := s.GetPopularItems(10)
	if err != nil {
		t.Fatalf("unexpected error on cache miss: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil on cache miss, got %v", got)
	}
}
