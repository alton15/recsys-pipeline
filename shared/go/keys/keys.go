package keys

import "fmt"

// RecTopK returns the DragonflyDB key for a user's top-K recommendations.
func RecTopK(userID string) string { return fmt.Sprintf("rec:%s:top_k", userID) }

// RecTTL returns the DragonflyDB key for a user's recommendation TTL tracking.
func RecTTL(userID string) string { return fmt.Sprintf("rec:%s:ttl", userID) }

// FeatUserClicks1H returns the key for a user's click count in the last hour.
func FeatUserClicks1H(userID string) string {
	return fmt.Sprintf("feat:user:%s:clicks_1h", userID)
}

// FeatUserViews1D returns the key for a user's view count in the last day.
func FeatUserViews1D(userID string) string {
	return fmt.Sprintf("feat:user:%s:views_1d", userID)
}

// FeatUserRecentItems returns the key for a user's recently interacted items.
func FeatUserRecentItems(userID string) string {
	return fmt.Sprintf("feat:user:%s:recent_items", userID)
}

// FeatItemCTR7D returns the key for an item's 7-day click-through rate.
func FeatItemCTR7D(itemID string) string {
	return fmt.Sprintf("feat:item:%s:ctr_7d", itemID)
}

// FeatItemPopularity returns the key for an item's popularity score.
func FeatItemPopularity(itemID string) string {
	return fmt.Sprintf("feat:item:%s:popularity", itemID)
}

// StockBitmap returns the key for the global stock availability bitmap.
func StockBitmap() string { return "stock:bitmap" }

// StockIDMap returns the key for mapping an item ID to its bitmap offset.
func StockIDMap(itemID string) string { return fmt.Sprintf("stock:id_map:%s", itemID) }

// SessionEvents returns the key for storing events within a session.
func SessionEvents(sessionID string) string {
	return fmt.Sprintf("session:%s:events", sessionID)
}

// ExperimentConfig returns the key for an A/B experiment configuration.
func ExperimentConfig(expID string) string { return fmt.Sprintf("experiment:%s", expID) }

// ExperimentActiveList returns the key for the set of active experiment IDs.
func ExperimentActiveList() string { return "experiment:active" }
