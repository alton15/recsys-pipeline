package tier

// Store retrieves pre-computed recommendations from the backing cache.
type Store interface {
	GetRecommendations(userID string) ([]Recommendation, error)
	GetPopularItems(limit int) ([]Recommendation, error)
}

// StockChecker determines whether items are currently in stock.
type StockChecker interface {
	IsOutOfStock(itemID string) (bool, error)
	FilterOutOfStock(items []Recommendation) ([]Recommendation, error)
}

// Reranker adjusts recommendation ordering based on session context.
type Reranker interface {
	Rerank(userID string, sessionID string, items []Recommendation) ([]Recommendation, error)
}
