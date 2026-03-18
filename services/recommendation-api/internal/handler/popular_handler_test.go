package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/recsys-pipeline/recommendation-api/internal/handler"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

// mockPopularStore implements handler.PopularStore for testing.
type mockPopularStore struct {
	items []tier.Recommendation
	err   error
}

func (m *mockPopularStore) GetPopularItems(limit int) ([]tier.Recommendation, error) {
	if m.err != nil {
		return nil, m.err
	}
	if limit < len(m.items) {
		return m.items[:limit], nil
	}
	return m.items, nil
}

func TestPopularHandler_SetsCacheHeaders(t *testing.T) {
	store := &mockPopularStore{
		items: []tier.Recommendation{{ItemID: "pop1", Score: 100}},
	}
	h := handler.NewPopularHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	w := httptest.NewRecorder()

	h.HandlePopular(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	cc := w.Header().Get("Cache-Control")
	expected := "public, max-age=30, stale-while-revalidate=60"
	if cc != expected {
		t.Errorf("expected Cache-Control %q, got %q", expected, cc)
	}

	vary := w.Header().Get("Vary")
	if vary != "X-Category" {
		t.Errorf("expected Vary 'X-Category', got %q", vary)
	}
}

func TestPopularHandler_ReturnsItems(t *testing.T) {
	store := &mockPopularStore{
		items: []tier.Recommendation{
			{ItemID: "pop1", Score: 100},
			{ItemID: "pop2", Score: 90},
		},
	}
	h := handler.NewPopularHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular?limit=10", nil)
	w := httptest.NewRecorder()

	h.HandlePopular(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.PopularResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Tier != "tier0_popular" {
		t.Errorf("expected tier 'tier0_popular', got %q", resp.Tier)
	}
}

func TestPopularHandler_DefaultLimit(t *testing.T) {
	items := make([]tier.Recommendation, 30)
	for i := range items {
		items[i] = tier.Recommendation{ItemID: "item", Score: float64(30 - i)}
	}
	store := &mockPopularStore{items: items}
	h := handler.NewPopularHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	w := httptest.NewRecorder()

	h.HandlePopular(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.PopularResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Items) != 20 {
		t.Errorf("expected default limit 20 items, got %d", len(resp.Items))
	}
}
