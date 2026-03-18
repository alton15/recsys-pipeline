package rerank

import (
	"sort"

	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

// Weights controls how session signals adjust recommendation scores.
type Weights struct {
	BaseScore      float64 // multiplier for the original score (default 1.0)
	ClickedPenalty float64 // additive penalty for already-clicked items (default -0.5)
	ViewedPenalty  float64 // additive penalty for already-viewed items (default -0.2)
}

// DefaultWeights returns production-ready default weights.
func DefaultWeights() Weights {
	return Weights{
		BaseScore:      1.0,
		ClickedPenalty: -0.5,
		ViewedPenalty:  -0.2,
	}
}

// WeightedScorer applies session-aware penalties to recommendation scores.
type WeightedScorer struct {
	weights Weights
}

// NewWeightedScorer creates a WeightedScorer with the given weights.
func NewWeightedScorer(w Weights) *WeightedScorer {
	return &WeightedScorer{weights: w}
}

// Rerank produces a new slice of recommendations sorted by adjusted scores.
// The input slice is never modified (immutable pattern).
func (s *WeightedScorer) Rerank(items []tier.Recommendation, features *SessionFeatures) []tier.Recommendation {
	type scored struct {
		rec        tier.Recommendation
		finalScore float64
	}

	entries := make([]scored, len(items))
	for i, item := range items {
		finalScore := item.Score * s.weights.BaseScore

		if features.ClickedItems[item.ItemID] {
			finalScore += s.weights.ClickedPenalty
		}
		if features.ViewedItems[item.ItemID] {
			finalScore += s.weights.ViewedPenalty
		}

		entries[i] = scored{
			rec:        tier.Recommendation{ItemID: item.ItemID, Score: item.Score},
			finalScore: finalScore,
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].finalScore > entries[j].finalScore
	})

	result := make([]tier.Recommendation, len(entries))
	for i, e := range entries {
		result[i] = tier.Recommendation{
			ItemID: e.rec.ItemID,
			Score:  e.finalScore,
		}
	}

	return result
}
