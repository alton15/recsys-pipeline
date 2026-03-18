package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Recommender is the interface the handler depends on for serving recommendations.
type Recommender interface {
	Recommend(userID, sessionID string, limit int) ([]tier.Recommendation, tier.Level, error)
}

// RecommendResponse is the JSON envelope returned to clients.
type RecommendResponse struct {
	Items []tier.Recommendation `json:"items"`
	Tier  string                `json:"tier"`
}

// RecommendHandler handles HTTP requests for recommendations.
type RecommendHandler struct {
	recommender Recommender
}

// NewRecommendHandler creates a RecommendHandler with the given recommender.
func NewRecommendHandler(r Recommender) *RecommendHandler {
	return &RecommendHandler{recommender: r}
}

// HandleRecommend serves GET /api/v1/recommend.
// Query params: user_id (required), session_id (optional), limit (optional, default 20, max 100).
func (h *RecommendHandler) HandleRecommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	sessionID := r.URL.Query().Get("session_id")

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

	items, level, err := h.recommender.Recommend(userID, sessionID, limit)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if items == nil {
		items = []tier.Recommendation{}
	}

	resp := RecommendResponse{
		Items: items,
		Tier:  string(level),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleHealth returns a simple 200 OK for liveness probes.
func (h *RecommendHandler) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
