// Package triton provides a client for NVIDIA Triton Inference Server.
// This file contains the Ranker adapter that bridges the Triton client
// with the tier.Ranker interface, adding circuit breaker protection.
package triton

import (
	"context"
	"fmt"

	"github.com/recsys-pipeline/recommendation-api/internal/circuitbreaker"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

// ProtectedRanker implements tier.Ranker by calling the Triton client
// for batch scoring, guarded by a circuit breaker.
type ProtectedRanker struct {
	client  *Client
	breaker *circuitbreaker.Breaker
}

// NewProtectedRanker creates a Ranker that wraps a Triton client with
// circuit breaker protection. If the breaker is open, Rank returns an
// error immediately without calling Triton.
func NewProtectedRanker(client *Client, breaker *circuitbreaker.Breaker) *ProtectedRanker {
	return &ProtectedRanker{
		client:  client,
		breaker: breaker,
	}
}

// Rank scores candidate items via Triton inference, protected by a circuit
// breaker. On success, items are returned sorted by descending score.
// On circuit-open or inference failure, an error is returned so the caller
// can fall back gracefully.
func (r *ProtectedRanker) Rank(userID string, items []tier.Recommendation) ([]tier.Recommendation, error) {
	if !r.breaker.Allow() {
		return nil, fmt.Errorf("triton circuit breaker is open")
	}

	// Build batch items. In a production system, user/item embeddings would
	// be fetched from the feature store. Here we use zero-valued placeholder
	// embeddings to satisfy the interface contract.
	batchItems := make([]BatchItem, len(items))
	userEmb := make([]float32, UserEmbeddingDim)
	for i := range batchItems {
		batchItems[i] = BatchItem{
			UserEmbedding:   userEmb,
			ItemEmbedding:   make([]float32, ItemEmbeddingDim),
			ContextFeatures: make([]float32, ContextFeatureDim),
		}
	}

	scores, err := r.client.ScoreBatch(context.Background(), batchItems)
	if err != nil {
		r.breaker.RecordFailure()
		return nil, fmt.Errorf("triton rank failed: %w", err)
	}

	r.breaker.RecordSuccess()

	// Build result with updated scores, preserving item metadata.
	result := make([]tier.Recommendation, len(items))
	for i, item := range items {
		result[i] = tier.Recommendation{
			ItemID: item.ItemID,
			Score:  float64(scores[i]),
		}
	}

	// Sort by descending score.
	sortByScoreDesc(result)

	return result, nil
}

// sortByScoreDesc sorts recommendations by score in descending order.
func sortByScoreDesc(items []tier.Recommendation) {
	// Simple insertion sort — efficient for typical candidate set sizes (<200).
	for i := 1; i < len(items); i++ {
		key := items[i]
		j := i - 1
		for j >= 0 && items[j].Score < key.Score {
			items[j+1] = items[j]
			j--
		}
		items[j+1] = key
	}
}
