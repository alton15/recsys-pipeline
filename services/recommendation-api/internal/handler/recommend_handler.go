package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/recsys-pipeline/recommendation-api/internal/experiment"
	"github.com/recsys-pipeline/recommendation-api/internal/metrics"
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

// ExperimentAssigner assigns users to A/B experiments.
type ExperimentAssigner interface {
	GetExperiment(userID string) (*experiment.Experiment, error)
}

// HealthPinger checks the health of a backing store.
type HealthPinger interface {
	Ping(ctx context.Context) error
}

// RecommendResponse is the JSON envelope returned to clients.
type RecommendResponse struct {
	Items        []tier.Recommendation `json:"items"`
	Tier         string                `json:"tier"`
	ExperimentID string                `json:"experiment_id,omitempty"`
	ModelVersion string                `json:"model_version,omitempty"`
}

// RecommendHandler handles HTTP requests for recommendations.
type RecommendHandler struct {
	recommender  Recommender
	experimenter ExperimentAssigner
	pinger       HealthPinger
}

// NewRecommendHandler creates a RecommendHandler with the given recommender.
func NewRecommendHandler(r Recommender) *RecommendHandler {
	return &RecommendHandler{recommender: r}
}

// WithExperiments returns a new RecommendHandler with experiment routing enabled.
func (h *RecommendHandler) WithExperiments(e ExperimentAssigner) *RecommendHandler {
	return &RecommendHandler{
		recommender:  h.recommender,
		experimenter: e,
		pinger:       h.pinger,
	}
}

// WithPinger returns a new RecommendHandler with a health pinger for dependency checks.
func (h *RecommendHandler) WithPinger(p HealthPinger) *RecommendHandler {
	return &RecommendHandler{
		recommender:  h.recommender,
		experimenter: h.experimenter,
		pinger:       p,
	}
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

	start := time.Now()
	items, level, err := h.recommender.Recommend(userID, sessionID, limit)
	duration := time.Since(start).Seconds()

	tierLabel := string(level)
	if tierLabel == "" {
		tierLabel = "unknown"
	}

	metrics.RequestsTotal.WithLabelValues(tierLabel).Inc()
	metrics.RequestDuration.WithLabelValues(tierLabel).Observe(duration)

	if err != nil {
		metrics.ErrorsTotal.WithLabelValues(tierLabel).Inc()
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

	// Assign A/B experiment if experimenter is configured.
	if h.experimenter != nil {
		exp, expErr := h.experimenter.GetExperiment(userID)
		if expErr != nil {
			log.Printf("experiment assignment error for user %s: %v", userID, expErr)
		}
		if exp != nil {
			resp.ExperimentID = exp.ID
			resp.ModelVersion = exp.ModelVersion
			log.Printf("experiment assignment: user=%s experiment=%s model=%s", userID, exp.ID, exp.ModelVersion)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HealthResponse is the JSON body returned by the health endpoint.
type HealthResponse struct {
	Status    string `json:"status"`
	Dragonfly string `json:"dragonfly"`
}

// HandleHealth checks DragonflyDB connectivity and returns the health status.
// Returns 200 if healthy, 503 if DragonflyDB is unreachable.
func (h *RecommendHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.pinger != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := h.pinger.Ping(ctx); err != nil {
			log.Printf("health check failed: dragonfly ping error: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(HealthResponse{
				Status:    "unhealthy",
				Dragonfly: "disconnected",
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HealthResponse{
		Status:    "ok",
		Dragonfly: "connected",
	})
}
