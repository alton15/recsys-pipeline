package triton

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- Mock gRPC Inference Client ---

type mockInferResult struct {
	scores []float32
	err    error
}

type mockGRPCInferenceClient struct {
	result   mockInferResult
	called   bool
	reqCheck func(req *InferRequest)
}

func (m *mockGRPCInferenceClient) ModelInfer(ctx context.Context, req *InferRequest) (*InferResponse, error) {
	m.called = true
	if m.reqCheck != nil {
		m.reqCheck(req)
	}
	if m.result.err != nil {
		return nil, m.result.err
	}
	return &InferResponse{Scores: m.result.scores}, nil
}

// --- Tests ---

func TestNewClient_DefaultTimeout(t *testing.T) {
	mock := &mockGRPCInferenceClient{}
	c := NewClient(mock, 0)

	if c.timeout != DefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultTimeout, c.timeout)
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	mock := &mockGRPCInferenceClient{}
	c := NewClient(mock, 50*time.Millisecond)

	if c.timeout != 50*time.Millisecond {
		t.Errorf("expected 50ms timeout, got %v", c.timeout)
	}
}

func TestClient_Score_CorrectRequestConstruction(t *testing.T) {
	userEmb := make([]float32, UserEmbeddingDim)
	itemEmb := make([]float32, ItemEmbeddingDim)
	ctxFeat := make([]float32, ContextFeatureDim)

	for i := range userEmb {
		userEmb[i] = float32(i) * 0.01
	}
	for i := range itemEmb {
		itemEmb[i] = float32(i) * 0.02
	}
	for i := range ctxFeat {
		ctxFeat[i] = float32(i) * 0.1
	}

	var capturedReq *InferRequest
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.85}},
		reqCheck: func(req *InferRequest) {
			capturedReq = req
		},
	}

	c := NewClient(mock, 100*time.Millisecond)
	_, err := c.Score(context.Background(), userEmb, itemEmb, ctxFeat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq == nil {
		t.Fatal("request was not captured")
	}
	if capturedReq.ModelName != ModelName {
		t.Errorf("expected model name %s, got %s", ModelName, capturedReq.ModelName)
	}
	if len(capturedReq.UserEmbedding) != UserEmbeddingDim {
		t.Errorf("expected user embedding dim %d, got %d", UserEmbeddingDim, len(capturedReq.UserEmbedding))
	}
	if len(capturedReq.ItemEmbedding) != ItemEmbeddingDim {
		t.Errorf("expected item embedding dim %d, got %d", ItemEmbeddingDim, len(capturedReq.ItemEmbedding))
	}
	if len(capturedReq.ContextFeatures) != ContextFeatureDim {
		t.Errorf("expected context features dim %d, got %d", ContextFeatureDim, len(capturedReq.ContextFeatures))
	}
}

func TestClient_Score_ReturnsScore(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.92}},
	}

	c := NewClient(mock, 100*time.Millisecond)
	score, err := c.Score(
		context.Background(),
		make([]float32, UserEmbeddingDim),
		make([]float32, ItemEmbeddingDim),
		make([]float32, ContextFeatureDim),
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0.92 {
		t.Errorf("expected score 0.92, got %f", score)
	}
}

func TestClient_Score_ErrorHandling(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{err: errors.New("triton unavailable")},
	}

	c := NewClient(mock, 100*time.Millisecond)
	_, err := c.Score(
		context.Background(),
		make([]float32, UserEmbeddingDim),
		make([]float32, ItemEmbeddingDim),
		make([]float32, ContextFeatureDim),
	)

	if err == nil {
		t.Fatal("expected error when triton is unavailable")
	}
}

func TestClient_Score_TimeoutHandling(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.5}},
	}

	c := NewClient(mock, 100*time.Millisecond)

	// Create an already-cancelled context to simulate timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Score(
		ctx,
		make([]float32, UserEmbeddingDim),
		make([]float32, ItemEmbeddingDim),
		make([]float32, ContextFeatureDim),
	)

	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestClient_Score_InvalidDimensions(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.5}},
	}

	c := NewClient(mock, 100*time.Millisecond)

	tests := []struct {
		name    string
		userEmb []float32
		itemEmb []float32
		ctxFeat []float32
	}{
		{
			name:    "wrong user embedding dim",
			userEmb: make([]float32, 64),
			itemEmb: make([]float32, ItemEmbeddingDim),
			ctxFeat: make([]float32, ContextFeatureDim),
		},
		{
			name:    "wrong item embedding dim",
			userEmb: make([]float32, UserEmbeddingDim),
			itemEmb: make([]float32, 64),
			ctxFeat: make([]float32, ContextFeatureDim),
		},
		{
			name:    "wrong context features dim",
			userEmb: make([]float32, UserEmbeddingDim),
			itemEmb: make([]float32, ItemEmbeddingDim),
			ctxFeat: make([]float32, 16),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Score(context.Background(), tt.userEmb, tt.itemEmb, tt.ctxFeat)
			if err == nil {
				t.Error("expected validation error for wrong dimensions")
			}
		})
	}
}

func TestClient_ScoreBatch_MultipleItems(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.9, 0.7}},
	}

	c := NewClient(mock, 100*time.Millisecond)

	items := []BatchItem{
		{
			UserEmbedding:   make([]float32, UserEmbeddingDim),
			ItemEmbedding:   make([]float32, ItemEmbeddingDim),
			ContextFeatures: make([]float32, ContextFeatureDim),
		},
		{
			UserEmbedding:   make([]float32, UserEmbeddingDim),
			ItemEmbedding:   make([]float32, ItemEmbeddingDim),
			ContextFeatures: make([]float32, ContextFeatureDim),
		},
	}

	scores, err := c.ScoreBatch(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != 2 {
		t.Errorf("expected 2 scores, got %d", len(scores))
	}
	if scores[0] != 0.9 {
		t.Errorf("expected first score 0.9, got %f", scores[0])
	}
	if scores[1] != 0.7 {
		t.Errorf("expected second score 0.7, got %f", scores[1])
	}
}

func TestClient_ScoreBatch_BatchedRequest(t *testing.T) {
	// Verify that ScoreBatch sends a single batched request, not per-item calls.
	callCount := 0
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.8, 0.6, 0.4}},
		reqCheck: func(req *InferRequest) {
			callCount++
			// The batched request should contain concatenated embeddings.
			if len(req.UserEmbedding) != 3*UserEmbeddingDim {
				t.Errorf("expected batched user embedding len %d, got %d", 3*UserEmbeddingDim, len(req.UserEmbedding))
			}
			if len(req.ItemEmbedding) != 3*ItemEmbeddingDim {
				t.Errorf("expected batched item embedding len %d, got %d", 3*ItemEmbeddingDim, len(req.ItemEmbedding))
			}
		},
	}

	c := NewClient(mock, 100*time.Millisecond)

	items := make([]BatchItem, 3)
	for i := range items {
		items[i] = BatchItem{
			UserEmbedding:   make([]float32, UserEmbeddingDim),
			ItemEmbedding:   make([]float32, ItemEmbeddingDim),
			ContextFeatures: make([]float32, ContextFeatureDim),
		}
	}

	scores, err := c.ScoreBatch(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != 3 {
		t.Errorf("expected 3 scores, got %d", len(scores))
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 gRPC call for batched inference, got %d", callCount)
	}
}

func TestClient_ScoreBatch_InvalidDimensions(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.5}},
	}

	c := NewClient(mock, 100*time.Millisecond)

	items := []BatchItem{
		{
			UserEmbedding:   make([]float32, 64), // wrong dimension
			ItemEmbedding:   make([]float32, ItemEmbeddingDim),
			ContextFeatures: make([]float32, ContextFeatureDim),
		},
	}

	_, err := c.ScoreBatch(context.Background(), items)
	if err == nil {
		t.Fatal("expected validation error for wrong dimensions in batch")
	}
}

func TestClient_ScoreBatch_EmptyInput(t *testing.T) {
	mock := &mockGRPCInferenceClient{}

	c := NewClient(mock, 100*time.Millisecond)

	scores, err := c.ScoreBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("expected 0 scores for empty input, got %d", len(scores))
	}
}

func TestClient_Score_EmptyResponse(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{}},
	}

	c := NewClient(mock, 100*time.Millisecond)
	_, err := c.Score(
		context.Background(),
		make([]float32, UserEmbeddingDim),
		make([]float32, ItemEmbeddingDim),
		make([]float32, ContextFeatureDim),
	)

	if err == nil {
		t.Fatal("expected error for empty response")
	}
}
