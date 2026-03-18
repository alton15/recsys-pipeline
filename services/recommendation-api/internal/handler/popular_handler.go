package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

// PopularStore is the interface for retrieving popular items.
type PopularStore interface {
	GetPopularItems(limit int) ([]tier.Recommendation, error)
}

// PopularResponse is the JSON envelope returned for the popular items endpoint.
type PopularResponse struct {
	Items []tier.Recommendation `json:"items"`
	Tier  string                `json:"tier"`
}

// PopularHandler handles HTTP requests for popular item listings (Tier 0).
type PopularHandler struct {
	store PopularStore
}

// NewPopularHandler creates a PopularHandler with the given store.
func NewPopularHandler(s PopularStore) *PopularHandler {
	return &PopularHandler{store: s}
}

// HandlePopular serves GET /api/v1/popular.
// Query params: category (optional), limit (optional, default 20).
// Sets CDN-cacheable headers for edge caching.
func (h *PopularHandler) HandlePopular(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := defaultLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	items, err := h.store.GetPopularItems(limit)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if items == nil {
		items = []tier.Recommendation{}
	}

	resp := PopularResponse{
		Items: items,
		Tier:  "tier0_popular",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	w.Header().Set("Vary", "X-Category")
	json.NewEncoder(w).Encode(resp)
}
