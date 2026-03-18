package tier

// Router orchestrates the multi-tier recommendation pipeline.
type Router struct {
	store   Store
	stock   StockChecker
	reranker Reranker
}

// NewRouter creates a Router. The reranker parameter may be nil if Tier 2
// re-ranking is not yet available.
func NewRouter(store Store, stock StockChecker, reranker Reranker) *Router {
	return &Router{
		store:    store,
		stock:    stock,
		reranker: reranker,
	}
}

// Recommend returns up to `limit` recommendations for the given user,
// along with the tier that produced the result.
func (r *Router) Recommend(userID, sessionID string, limit int) ([]Recommendation, Level, error) {
	// Step 1: Try pre-computed recommendations (Tier 1).
	recs, err := r.store.GetRecommendations(userID)
	if err != nil {
		return nil, "", err
	}

	level := Tier1

	// Step 2: If no pre-computed recs, fall back to popular items.
	if len(recs) == 0 {
		recs, err = r.store.GetPopularItems(limit)
		if err != nil {
			return nil, "", err
		}
		level = Fallback
	}

	// Step 3: Filter out-of-stock items.
	recs, err = r.stock.FilterOutOfStock(recs)
	if err != nil {
		return nil, "", err
	}

	// Step 4: If session context is available and reranker is configured, re-rank.
	if sessionID != "" && r.reranker != nil {
		recs, err = r.reranker.Rerank(userID, sessionID, recs)
		if err != nil {
			return nil, "", err
		}
		if level == Tier1 {
			level = Tier2
		}
	}

	// Step 5: Truncate to the requested limit.
	if len(recs) > limit {
		recs = recs[:limit]
	}

	return recs, level, nil
}
