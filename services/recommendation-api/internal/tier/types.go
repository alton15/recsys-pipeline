package tier

// Level indicates which tier of the recommendation pipeline served the result.
type Level string

const (
	Tier1    Level = "tier1_precomputed"
	Tier2    Level = "tier2_rerank"
	Tier3    Level = "tier3_inference"
	Fallback Level = "fallback_popular"
)

// Recommendation represents a single recommended item with its relevance score.
type Recommendation struct {
	ItemID string  `json:"item_id"`
	Score  float64 `json:"score"`
}
