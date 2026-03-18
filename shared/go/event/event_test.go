package event_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/recsys-pipeline/shared/event"
)

func TestEventTypes(t *testing.T) {
	tests := []struct {
		eventType event.Type
		expected  string
	}{
		{event.Click, "click"},
		{event.View, "view"},
		{event.Purchase, "purchase"},
		{event.Search, "search"},
		{event.AddToCart, "add_to_cart"},
		{event.RemoveFromCart, "remove_from_cart"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.eventType) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.eventType))
			}
		})
	}
}

func TestEventMarshalJSON(t *testing.T) {
	ts := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	e := event.Event{
		EventID:    "evt-001",
		UserID:     "user-123",
		EventType:  event.Click,
		ItemID:     "item-456",
		CategoryID: "cat-789",
		Timestamp:  ts,
		SessionID:  "sess-abc",
		Metadata: event.Metadata{
			Query:    "shoes",
			Position: 3,
			Price:    29900,
			Source:   "search",
		},
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal event JSON: %v", err)
	}

	if decoded["event_id"] != "evt-001" {
		t.Errorf("expected event_id=evt-001, got %v", decoded["event_id"])
	}
	if decoded["user_id"] != "user-123" {
		t.Errorf("expected user_id=user-123, got %v", decoded["user_id"])
	}
	if decoded["event_type"] != "click" {
		t.Errorf("expected event_type=click, got %v", decoded["event_type"])
	}
	if decoded["item_id"] != "item-456" {
		t.Errorf("expected item_id=item-456, got %v", decoded["item_id"])
	}

	meta, ok := decoded["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("metadata is not an object")
	}
	if meta["query"] != "shoes" {
		t.Errorf("expected metadata.query=shoes, got %v", meta["query"])
	}
	if meta["position"] != float64(3) {
		t.Errorf("expected metadata.position=3, got %v", meta["position"])
	}
	if meta["price"] != float64(29900) {
		t.Errorf("expected metadata.price=29900, got %v", meta["price"])
	}
}

func TestEventUnmarshalJSON(t *testing.T) {
	raw := `{
		"event_id": "evt-002",
		"user_id": "user-456",
		"event_type": "purchase",
		"item_id": "item-789",
		"category_id": "cat-001",
		"timestamp": "2026-03-18T12:00:00Z",
		"session_id": "sess-xyz",
		"metadata": {"price": 50000, "source": "recommendation"}
	}`

	var e event.Event
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if e.EventID != "evt-002" {
		t.Errorf("expected event_id=evt-002, got %s", e.EventID)
	}
	if e.EventType != event.Purchase {
		t.Errorf("expected event_type=purchase, got %s", e.EventType)
	}
	if e.Metadata.Price != 50000 {
		t.Errorf("expected metadata.price=50000, got %d", e.Metadata.Price)
	}
}

func TestMetadataOmitsEmptyFields(t *testing.T) {
	m := event.Metadata{Price: 10000}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal metadata: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, exists := decoded["query"]; exists {
		t.Error("expected query to be omitted when empty")
	}
	if _, exists := decoded["source"]; exists {
		t.Error("expected source to be omitted when empty")
	}
}
