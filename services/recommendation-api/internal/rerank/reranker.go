package rerank

import (
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

// SessionReranker implements the tier.Reranker interface by combining session
// feature extraction with weighted scoring.
type SessionReranker struct {
	extractor *SessionFeatureExtractor
	scorer    *WeightedScorer
}

// NewSessionReranker creates a SessionReranker with the given dependencies.
func NewSessionReranker(extractor *SessionFeatureExtractor, scorer *WeightedScorer) *SessionReranker {
	return &SessionReranker{
		extractor: extractor,
		scorer:    scorer,
	}
}

// Rerank extracts session features and applies weighted re-ranking.
// Satisfies the tier.Reranker interface.
func (r *SessionReranker) Rerank(userID, sessionID string, items []tier.Recommendation) ([]tier.Recommendation, error) {
	features, err := r.extractor.Extract(sessionID)
	if err != nil {
		return nil, err
	}
	return r.scorer.Rerank(items, features), nil
}
