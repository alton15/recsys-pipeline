package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/recsys-pipeline/shared/event"
)

// validTypes is the set of accepted event types.
var validTypes = map[event.Type]struct{}{
	event.Click:         {},
	event.View:          {},
	event.Purchase:      {},
	event.Search:        {},
	event.AddToCart:     {},
	event.RemoveFromCart: {},
}

// Producer abstracts message publishing so it can be mocked in tests.
type Producer interface {
	Publish(e event.Event) error
	Close() error
}

// Handler handles HTTP requests for the event collector.
type Handler struct {
	producer Producer
}

// New creates a new Handler with the given producer.
func New(p Producer) *Handler {
	return &Handler{producer: p}
}

// eventRequest represents the incoming JSON body for event ingestion.
type eventRequest struct {
	EventType  event.Type     `json:"event_type"`
	UserID     string         `json:"user_id"`
	ItemID     string         `json:"item_id"`
	CategoryID string         `json:"category_id,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	Metadata   event.Metadata `json:"metadata,omitempty"`
}

// HandleEvent processes POST /api/v1/events requests.
func (h *Handler) HandleEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if _, ok := validTypes[req.EventType]; !ok {
		http.Error(w, "invalid event_type", http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	if req.ItemID == "" {
		http.Error(w, "item_id is required", http.StatusBadRequest)
		return
	}

	evt := event.Event{
		EventID:    uuid.New().String(),
		UserID:     req.UserID,
		EventType:  req.EventType,
		ItemID:     req.ItemID,
		CategoryID: req.CategoryID,
		Timestamp:  time.Now().UTC(),
		SessionID:  req.SessionID,
		Metadata:   req.Metadata,
	}

	if err := h.producer.Publish(evt); err != nil {
		http.Error(w, "failed to publish event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "accepted",
		"event_id": evt.EventID,
	})
}

// HandleHealth returns 200 OK for health checks.
func (h *Handler) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
