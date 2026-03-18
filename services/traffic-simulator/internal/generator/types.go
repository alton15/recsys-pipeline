package generator

// User represents a simulated user with category preferences.
type User struct {
	ID            string   `json:"user_id"`
	CategoryPrefs []string `json:"category_prefs"`
}

// Item represents a catalog item with category and price.
type Item struct {
	ID         string `json:"item_id"`
	CategoryID string `json:"category_id"`
	Price      int64  `json:"price"`
}

// Recommendation represents a single item recommendation with a relevance score.
type Recommendation struct {
	ItemID string  `json:"item_id"`
	Score  float64 `json:"score"`
}
