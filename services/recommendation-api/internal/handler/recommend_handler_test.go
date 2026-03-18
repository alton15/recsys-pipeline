package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/recsys-pipeline/recommendation-api/internal/handler"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

// mockRouter implements a minimal recommendation router for testing.
type mockRouter struct {
	items []tier.Recommendation
	level tier.Level
	err   error
}

func (m *mockRouter) Recommend(_, _ string, limit int) ([]tier.Recommendation, tier.Level, error) {
	if m.err != nil {
		return nil, "", m.err
	}
	items := m.items
	if len(items) > limit {
		items = items[:limit]
	}
	return items, m.level, nil
}

func TestHandleRecommend_Success(t *testing.T) {
	router := &mockRouter{
		items: []tier.Recommendation{
			{ItemID: "item1", Score: 0.9},
			{ItemID: "item2", Score: 0.8},
		},
		level: tier.Tier1,
	}
	h := handler.NewRecommendHandler(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommend?user_id=user1", nil)
	w := httptest.NewRecorder()

	h.HandleRecommend(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.RecommendResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Tier != string(tier.Tier1) {
		t.Errorf("expected tier %s, got %s", tier.Tier1, resp.Tier)
	}
}

func TestHandleRecommend_MissingUserID(t *testing.T) {
	h := handler.NewRecommendHandler(&mockRouter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommend", nil)
	w := httptest.NewRecorder()

	h.HandleRecommend(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRecommend_DefaultLimit(t *testing.T) {
	items := make([]tier.Recommendation, 30)
	for i := range items {
		items[i] = tier.Recommendation{ItemID: "item", Score: float64(30 - i)}
	}
	router := &mockRouter{items: items, level: tier.Tier1}
	h := handler.NewRecommendHandler(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommend?user_id=user1", nil)
	w := httptest.NewRecorder()

	h.HandleRecommend(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.RecommendResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Items) != 20 {
		t.Errorf("expected default limit 20 items, got %d", len(resp.Items))
	}
}

func TestHandleRecommend_MaxLimit(t *testing.T) {
	router := &mockRouter{items: nil, level: tier.Fallback}
	h := handler.NewRecommendHandler(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommend?user_id=user1&limit=999", nil)
	w := httptest.NewRecorder()

	h.HandleRecommend(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// The handler should cap at 100 internally; we just verify no error.
}

func TestHandleRecommend_MethodNotAllowed(t *testing.T) {
	h := handler.NewRecommendHandler(&mockRouter{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/recommend?user_id=user1", nil)
	w := httptest.NewRecorder()

	h.HandleRecommend(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleHealth(t *testing.T) {
	h := handler.NewRecommendHandler(&mockRouter{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", w.Body.String())
	}
}
