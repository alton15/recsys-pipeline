package tier_test

import (
	"errors"
	"testing"

	"github.com/recsys-pipeline/recommendation-api/internal/degradation"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

// --- Mock Ranker ---

type mockRanker struct {
	called bool
	recs   []tier.Recommendation
	err    error
}

func (m *mockRanker) Rank(userID string, items []tier.Recommendation) ([]tier.Recommendation, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	if m.recs != nil {
		return m.recs, nil
	}
	// Default: return items with updated scores.
	result := make([]tier.Recommendation, len(items))
	for i, item := range items {
		result[i] = tier.Recommendation{ItemID: item.ItemID, Score: item.Score + 0.1}
	}
	return result, nil
}

func TestRouter_Tier3_CacheMissNoReranker_CallsRanker(t *testing.T) {
	store := &mockStore{
		recs: nil, // cache miss
		popular: []tier.Recommendation{
			{ItemID: "pop1", Score: 100},
			{ItemID: "pop2", Score: 90},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}
	ranker := &mockRanker{}

	router := tier.NewRouterWithRanker(store, checker, nil, nil, ranker)
	items, level, err := router.Recommend("user1", "", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ranker.called {
		t.Error("ranker should have been called on cache miss")
	}
	if level != tier.Tier3 {
		t.Errorf("expected tier %s, got %s", tier.Tier3, level)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestRouter_Tier3_CacheMissWithSession_CallsRankerAfterRerank(t *testing.T) {
	store := &mockStore{
		recs: nil,
		popular: []tier.Recommendation{
			{ItemID: "pop1", Score: 100},
			{ItemID: "pop2", Score: 90},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}
	reranker := &mockReranker{}
	ranker := &mockRanker{}

	router := tier.NewRouterWithRanker(store, checker, reranker, nil, ranker)
	items, level, err := router.Recommend("user1", "session1", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ranker.called {
		t.Error("ranker should have been called on cache miss")
	}
	if level != tier.Tier3 {
		t.Errorf("expected tier %s, got %s", tier.Tier3, level)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestRouter_Tier3_RankerError_FallsToPopular(t *testing.T) {
	store := &mockStore{
		recs: nil,
		popular: []tier.Recommendation{
			{ItemID: "pop1", Score: 100},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}
	ranker := &mockRanker{err: errors.New("triton circuit open")}

	router := tier.NewRouterWithRanker(store, checker, nil, nil, ranker)
	items, level, err := router.Recommend("user1", "", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != tier.Fallback {
		t.Errorf("expected fallback on ranker error, got %s", level)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 popular item, got %d", len(items))
	}
}

func TestRouter_Tier3_DegradationWarning_SkipsRanker(t *testing.T) {
	store := &mockStore{
		recs: nil,
		popular: []tier.Recommendation{
			{ItemID: "pop1", Score: 100},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}
	ranker := &mockRanker{}

	dm := degradation.NewManager()
	dm.SetLevel(degradation.Warning) // Warning disables Tier 3

	router := tier.NewRouterWithRanker(store, checker, nil, dm, ranker)
	items, level, err := router.Recommend("user1", "", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ranker.called {
		t.Error("ranker should NOT have been called at Warning level")
	}
	if level != tier.Fallback {
		t.Errorf("expected fallback level, got %s", level)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 popular item, got %d", len(items))
	}
}

func TestRouter_Tier3_CacheHit_SkipsRanker(t *testing.T) {
	store := &mockStore{
		recs: []tier.Recommendation{
			{ItemID: "item1", Score: 0.9},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}
	ranker := &mockRanker{}

	router := tier.NewRouterWithRanker(store, checker, nil, nil, ranker)
	_, level, err := router.Recommend("user1", "", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ranker.called {
		t.Error("ranker should NOT have been called on cache hit")
	}
	if level != tier.Tier1 {
		t.Errorf("expected tier1, got %s", level)
	}
}
