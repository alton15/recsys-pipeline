package tier

import "github.com/recsys-pipeline/recommendation-api/internal/degradation"

// DegradationChecker allows the router to query the current degradation state.
type DegradationChecker interface {
	IsTierAllowed(tierName string) bool
}

// Router orchestrates the multi-tier recommendation pipeline.
type Router struct {
	store       Store
	stock       StockChecker
	reranker    Reranker
	degradation DegradationChecker
	ranker      Ranker
}

// NewRouter creates a Router. The reranker parameter may be nil if Tier 2
// re-ranking is not yet available. The degradation parameter may be nil to
// allow all tiers unconditionally.
func NewRouter(store Store, stock StockChecker, reranker Reranker) *Router {
	return &Router{
		store:    store,
		stock:    stock,
		reranker: reranker,
	}
}

// NewRouterWithDegradation creates a Router with a degradation manager.
func NewRouterWithDegradation(store Store, stock StockChecker, reranker Reranker, dm *degradation.Manager) *Router {
	return &Router{
		store:       store,
		stock:       stock,
		reranker:    reranker,
		degradation: dm,
	}
}

// NewRouterWithRanker creates a Router with all tiers including Tier 3 inference.
// The ranker parameter may be nil if Tier 3 is not available.
func NewRouterWithRanker(store Store, stock StockChecker, reranker Reranker, dm *degradation.Manager, ranker Ranker) *Router {
	r := &Router{
		store:    store,
		stock:    stock,
		reranker: reranker,
		ranker:   ranker,
	}
	if dm != nil {
		r.degradation = dm
	}
	return r
}

// isTierAllowed checks whether the given tier is allowed. If no degradation
// checker is configured, all tiers are allowed.
func (r *Router) isTierAllowed(tierName string) bool {
	if r.degradation == nil {
		return true
	}
	return r.degradation.IsTierAllowed(tierName)
}

// Recommend returns up to `limit` recommendations for the given user,
// along with the tier that produced the result.
func (r *Router) Recommend(userID, sessionID string, limit int) ([]Recommendation, Level, error) {
	// Emergency: skip all tiers, return popular items directly.
	if !r.isTierAllowed("tier1") {
		recs, err := r.store.GetPopularItems(limit)
		if err != nil {
			return nil, "", err
		}
		if recs == nil {
			recs = []Recommendation{}
		}
		if len(recs) > limit {
			recs = recs[:limit]
		}
		return recs, Fallback, nil
	}

	// Step 1: Try pre-computed recommendations (Tier 1).
	recs, err := r.store.GetRecommendations(userID)
	if err != nil {
		return nil, "", err
	}

	level := Tier1
	cacheMiss := len(recs) == 0

	// Step 2: If no pre-computed recs, fall back to popular items as candidates.
	if cacheMiss {
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

	// Step 4: If session context is available, reranker is configured, and tier2 is allowed, re-rank.
	if sessionID != "" && r.reranker != nil && r.isTierAllowed("tier2") {
		recs, err = r.reranker.Rerank(userID, sessionID, recs)
		if err != nil {
			return nil, "", err
		}
		if level == Tier1 {
			level = Tier2
		}
	}

	// Step 5: Tier 3 — model-based ranking via inference server.
	// Only triggered on cache miss when ranker is available and tier3 is allowed.
	if cacheMiss && r.ranker != nil && r.isTierAllowed("tier3") {
		ranked, rankErr := r.ranker.Rank(userID, recs)
		if rankErr != nil {
			// On Tier 3 failure, keep current fallback candidates (graceful degradation).
			level = Fallback
		} else {
			recs = ranked
			level = Tier3
		}
	}

	// Step 6: Truncate to the requested limit.
	if len(recs) > limit {
		recs = recs[:limit]
	}

	return recs, level, nil
}
