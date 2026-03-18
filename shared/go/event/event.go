package event

import "time"

// Type represents the kind of user interaction event.
type Type string

const (
	Click         Type = "click"
	View          Type = "view"
	Purchase      Type = "purchase"
	Search        Type = "search"
	AddToCart     Type = "add_to_cart"
	RemoveFromCart Type = "remove_from_cart"
)

// Metadata holds optional context about an event.
type Metadata struct {
	Query    string `json:"query,omitempty"`
	Position int    `json:"position,omitempty"`
	Price    int64  `json:"price,omitempty"`
	Source   string `json:"source,omitempty"`
}

// Event represents a single user interaction in the commerce platform.
type Event struct {
	EventID    string   `json:"event_id"`
	UserID     string   `json:"user_id"`
	EventType  Type     `json:"event_type"`
	ItemID     string   `json:"item_id"`
	CategoryID string   `json:"category_id"`
	Timestamp  time.Time `json:"timestamp"`
	SessionID  string   `json:"session_id"`
	Metadata   Metadata `json:"metadata"`
}
