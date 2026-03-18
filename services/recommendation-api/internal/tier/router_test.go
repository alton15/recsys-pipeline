package tier_test

import (
	"errors"
	"testing"

	"github.com/recsys-pipeline/recommendation-api/internal/degradation"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

// --- Mock Store ---

type mockStore struct {
	recs    []tier.Recommendation
	recsErr error
	popular []tier.Recommendation
	popErr  error
}

func (m *mockStore) GetRecommendations(_ string) ([]tier.Recommendation, error) {
	return m.recs, m.recsErr
}

func (m *mockStore) GetPopularItems(limit int) ([]tier.Recommendation, error) {
	if m.popErr != nil {
		return nil, m.popErr
	}
	if limit < len(m.popular) {
		return m.popular[:limit], nil
	}
	return m.popular, nil
}

// --- Mock StockChecker ---

type mockStockChecker struct {
	oosItems map[string]bool
}

func (m *mockStockChecker) IsOutOfStock(itemID string) (bool, error) {
	return m.oosItems[itemID], nil
}

func (m *mockStockChecker) FilterOutOfStock(items []tier.Recommendation) ([]tier.Recommendation, error) {
	var result []tier.Recommendation
	for _, item := range items {
		if !m.oosItems[item.ItemID] {
			result = append(result, item)
		}
	}
	return result, nil
}

// --- Mock Reranker ---

type mockReranker struct {
	called bool
}

func (m *mockReranker) Rerank(_ string, _ string, items []tier.Recommendation) ([]tier.Recommendation, error) {
	m.called = true
	// Reverse order to prove reranking happened.
	result := make([]tier.Recommendation, len(items))
	for i, item := range items {
		result[len(items)-1-i] = item
	}
	return result, nil
}

func TestRouter_Tier1_PreComputedHit(t *testing.T) {
	store := &mockStore{
		recs: []tier.Recommendation{
			{ItemID: "item1", Score: 0.9},
			{ItemID: "item2", Score: 0.8},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}

	router := tier.NewRouter(store, checker, nil)
	items, level, err := router.Recommend("user1", "", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != tier.Tier1 {
		t.Errorf("expected tier %s, got %s", tier.Tier1, level)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestRouter_Tier1_FiltersOutOfStock(t *testing.T) {
	store := &mockStore{
		recs: []tier.Recommendation{
			{ItemID: "item1", Score: 0.9},
			{ItemID: "item2", Score: 0.8},
			{ItemID: "item3", Score: 0.7},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{"item2": true}}

	router := tier.NewRouter(store, checker, nil)
	items, _, err := router.Recommend("user1", "", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items after filtering, got %d", len(items))
	}
	for _, item := range items {
		if item.ItemID == "item2" {
			t.Error("out-of-stock item2 should have been filtered")
		}
	}
}

func TestRouter_CacheMiss_FallsToPopular(t *testing.T) {
	store := &mockStore{
		recs: nil,
		popular: []tier.Recommendation{
			{ItemID: "pop1", Score: 100},
			{ItemID: "pop2", Score: 90},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}

	router := tier.NewRouter(store, checker, nil)
	items, level, err := router.Recommend("user1", "", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != tier.Fallback {
		t.Errorf("expected tier %s, got %s", tier.Fallback, level)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 popular items, got %d", len(items))
	}
}

func TestRouter_Tier2_WithSession(t *testing.T) {
	store := &mockStore{
		recs: []tier.Recommendation{
			{ItemID: "item1", Score: 0.9},
			{ItemID: "item2", Score: 0.8},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}
	reranker := &mockReranker{}

	router := tier.NewRouter(store, checker, reranker)
	items, level, err := router.Recommend("user1", "session123", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != tier.Tier2 {
		t.Errorf("expected tier %s, got %s", tier.Tier2, level)
	}
	if !reranker.called {
		t.Error("reranker should have been called")
	}
	// Verify reranking reversed the order.
	if items[0].ItemID != "item2" {
		t.Errorf("expected first item to be item2 after reranking, got %s", items[0].ItemID)
	}
}

func TestRouter_Truncate(t *testing.T) {
	recs := make([]tier.Recommendation, 100)
	for i := range recs {
		recs[i] = tier.Recommendation{ItemID: "item", Score: float64(100 - i)}
	}
	store := &mockStore{recs: recs}
	checker := &mockStockChecker{oosItems: map[string]bool{}}

	router := tier.NewRouter(store, checker, nil)
	items, _, err := router.Recommend("user1", "", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 10 {
		t.Errorf("expected 10 items after truncation, got %d", len(items))
	}
}

func TestRouter_StoreError(t *testing.T) {
	store := &mockStore{recsErr: errors.New("connection refused")}
	checker := &mockStockChecker{oosItems: map[string]bool{}}

	router := tier.NewRouter(store, checker, nil)
	_, _, err := router.Recommend("user1", "", 10)

	if err == nil {
		t.Error("expected error when store fails")
	}
}

func TestRouter_EmergencySkipsTiers(t *testing.T) {
	store := &mockStore{
		recs: []tier.Recommendation{{ItemID: "item1", Score: 0.9}},
		popular: []tier.Recommendation{
			{ItemID: "pop1", Score: 100},
			{ItemID: "pop2", Score: 90},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}
	reranker := &mockReranker{}

	dm := degradation.NewManager()
	dm.SetLevel(degradation.Emergency)

	router := tier.NewRouterWithDegradation(store, checker, reranker, dm)
	items, level, err := router.Recommend("user1", "session1", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != tier.Fallback {
		t.Errorf("expected fallback level, got %s", level)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 popular items, got %d", len(items))
	}
	if reranker.called {
		t.Error("reranker should NOT have been called in emergency mode")
	}
}

func TestRouter_CriticalSkipsTier2(t *testing.T) {
	store := &mockStore{
		recs: []tier.Recommendation{
			{ItemID: "item1", Score: 0.9},
			{ItemID: "item2", Score: 0.8},
		},
	}
	checker := &mockStockChecker{oosItems: map[string]bool{}}
	reranker := &mockReranker{}

	dm := degradation.NewManager()
	dm.SetLevel(degradation.Critical)

	router := tier.NewRouterWithDegradation(store, checker, reranker, dm)
	items, level, err := router.Recommend("user1", "session1", 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should stay Tier1 since Tier2 reranking is blocked.
	if level != tier.Tier1 {
		t.Errorf("expected tier1 level, got %s", level)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if reranker.called {
		t.Error("reranker should NOT have been called at Critical level")
	}
}
