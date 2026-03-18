// Package client provides a Go gRPC client for NVIDIA Triton Inference Server.
// This file is the standalone client for the ranking-service.
// The recommendation-api imports its own copy at internal/triton/client.go.
package client

import (
	"context"
	"fmt"
	"time"
)

const (
	// ModelName is the Triton model repository name for DCN-V2.
	ModelName = "dcn_v2"

	// UserEmbeddingDim is the expected dimension for user embeddings.
	UserEmbeddingDim = 128

	// ItemEmbeddingDim is the expected dimension for item embeddings.
	ItemEmbeddingDim = 128

	// ContextFeatureDim is the expected dimension for context features.
	ContextFeatureDim = 32

	// DefaultTimeout is the default inference timeout for Tier 3 SLA (80ms).
	DefaultTimeout = 80 * time.Millisecond
)

// InferRequest represents a model inference request to Triton.
type InferRequest struct {
	ModelName       string
	UserEmbedding   []float32
	ItemEmbedding   []float32
	ContextFeatures []float32
}

// InferResponse represents the inference result from Triton.
type InferResponse struct {
	Scores []float32
}

// BatchItem holds the input data for a single item in a batch inference.
type BatchItem struct {
	UserEmbedding   []float32
	ItemEmbedding   []float32
	ContextFeatures []float32
}

// GRPCInferenceClient abstracts the Triton gRPC inference API for testability.
type GRPCInferenceClient interface {
	ModelInfer(ctx context.Context, req *InferRequest) (*InferResponse, error)
}

// TritonClient wraps a Triton gRPC connection with timeout and validation.
type TritonClient struct {
	grpc    GRPCInferenceClient
	timeout time.Duration
}

// NewTritonClient creates a new Triton client. If timeout is zero, DefaultTimeout is used.
func NewTritonClient(grpc GRPCInferenceClient, timeout time.Duration) *TritonClient {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &TritonClient{
		grpc:    grpc,
		timeout: timeout,
	}
}

// Score sends a single inference request and returns the relevance score.
// It validates input dimensions and enforces the configured timeout.
func (c *TritonClient) Score(ctx context.Context, userEmb, itemEmb []float32, ctxFeat []float32) (float32, error) {
	if err := validateDimensions(userEmb, itemEmb, ctxFeat); err != nil {
		return 0, err
	}

	inferCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Check if the parent context is already cancelled.
	if err := inferCtx.Err(); err != nil {
		return 0, fmt.Errorf("triton inference context error: %w", err)
	}

	req := &InferRequest{
		ModelName:       ModelName,
		UserEmbedding:   userEmb,
		ItemEmbedding:   itemEmb,
		ContextFeatures: ctxFeat,
	}

	resp, err := c.grpc.ModelInfer(inferCtx, req)
	if err != nil {
		return 0, fmt.Errorf("triton inference failed: %w", err)
	}

	if len(resp.Scores) == 0 {
		return 0, fmt.Errorf("triton returned empty scores")
	}

	return resp.Scores[0], nil
}

// ScoreBatch scores multiple items and returns their relevance scores.
func (c *TritonClient) ScoreBatch(ctx context.Context, items []BatchItem) ([]float32, error) {
	if len(items) == 0 {
		return []float32{}, nil
	}

	scores := make([]float32, len(items))
	for i, item := range items {
		score, err := c.Score(ctx, item.UserEmbedding, item.ItemEmbedding, item.ContextFeatures)
		if err != nil {
			return nil, fmt.Errorf("batch item %d: %w", i, err)
		}
		scores[i] = score
	}

	return scores, nil
}

// validateDimensions checks that input vectors have the expected dimensions.
func validateDimensions(userEmb, itemEmb []float32, ctxFeat []float32) error {
	if len(userEmb) != UserEmbeddingDim {
		return fmt.Errorf("user embedding: expected dim %d, got %d", UserEmbeddingDim, len(userEmb))
	}
	if len(itemEmb) != ItemEmbeddingDim {
		return fmt.Errorf("item embedding: expected dim %d, got %d", ItemEmbeddingDim, len(itemEmb))
	}
	if len(ctxFeat) != ContextFeatureDim {
		return fmt.Errorf("context features: expected dim %d, got %d", ContextFeatureDim, len(ctxFeat))
	}
	return nil
}
