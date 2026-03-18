package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/recsys-pipeline/event-collector/internal/handler"
	"github.com/recsys-pipeline/shared/event"
)

// mockProducer implements handler.Producer for testing.
type mockProducer struct {
	mu     sync.Mutex
	events []event.Event
	err    error
}

func (m *mockProducer) Publish(e event.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, e)
	return nil
}

func (m *mockProducer) Close() error { return nil }

func (m *mockProducer) Events() []event.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	dst := make([]event.Event, len(m.events))
	copy(dst, m.events)
	return dst
}

func TestHandleEvent_ValidClick(t *testing.T) {
	mp := &mockProducer{}
	h := handler.New(mp)

	body := map[string]interface{}{
		"event_type": "click",
		"user_id":    "u-123",
		"item_id":    "i-456",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleEvent(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", rec.Code)
	}

	events := mp.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(events))
	}
	if events[0].UserID != "u-123" {
		t.Errorf("expected user_id u-123, got %s", events[0].UserID)
	}
	if events[0].EventType != event.Click {
		t.Errorf("expected event_type click, got %s", events[0].EventType)
	}
}

func TestHandleEvent_InvalidType(t *testing.T) {
	mp := &mockProducer{}
	h := handler.New(mp)

	body := map[string]interface{}{
		"event_type": "invalid",
		"user_id":    "u-123",
		"item_id":    "i-456",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleEvent(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}

	if len(mp.Events()) != 0 {
		t.Error("expected no events published for invalid type")
	}
}

func TestHandleEvent_MissingUserID(t *testing.T) {
	mp := &mockProducer{}
	h := handler.New(mp)

	body := map[string]interface{}{
		"event_type": "click",
		"user_id":    "",
		"item_id":    "i-456",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleEvent(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}
}

func TestHandleEvent_InvalidJSON(t *testing.T) {
	mp := &mockProducer{}
	h := handler.New(mp)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader([]byte("{bad json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleEvent(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}
}

func TestHandleEvent_MethodNotAllowed(t *testing.T) {
	mp := &mockProducer{}
	h := handler.New(mp)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	h.HandleEvent(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", rec.Code)
	}
}
