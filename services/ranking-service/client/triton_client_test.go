package client

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

func TestNewTritonClient_DefaultTimeout(t *testing.T) {
	mock := &mockGRPCInferenceClient{}
	c := NewTritonClient(mock, 0)

	if c.timeout != DefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultTimeout, c.timeout)
	}
}

func TestNewTritonClient_CustomTimeout(t *testing.T) {
	mock := &mockGRPCInferenceClient{}
	c := NewTritonClient(mock, 50*time.Millisecond)

	if c.timeout != 50*time.Millisecond {
		t.Errorf("expected 50ms timeout, got %v", c.timeout)
	}
}

func TestTritonClient_Score_CorrectRequestConstruction(t *testing.T) {
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

	c := NewTritonClient(mock, 100*time.Millisecond)
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

func TestTritonClient_Score_ReturnsScore(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.92}},
	}

	c := NewTritonClient(mock, 100*time.Millisecond)
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

func TestTritonClient_Score_ErrorHandling(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{err: errors.New("triton unavailable")},
	}

	c := NewTritonClient(mock, 100*time.Millisecond)
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

func TestTritonClient_Score_TimeoutHandling(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.5}},
	}

	c := NewTritonClient(mock, 100*time.Millisecond)

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

func TestTritonClient_Score_InvalidDimensions(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.5}},
	}

	c := NewTritonClient(mock, 100*time.Millisecond)

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

func TestTritonClient_ScoreBatch_MultipleItems(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{0.9}},
	}

	c := NewTritonClient(mock, 100*time.Millisecond)

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
}

func TestTritonClient_ScoreBatch_EmptyInput(t *testing.T) {
	mock := &mockGRPCInferenceClient{}

	c := NewTritonClient(mock, 100*time.Millisecond)

	scores, err := c.ScoreBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("expected 0 scores for empty input, got %d", len(scores))
	}
}

func TestTritonClient_Score_EmptyResponse(t *testing.T) {
	mock := &mockGRPCInferenceClient{
		result: mockInferResult{scores: []float32{}},
	}

	c := NewTritonClient(mock, 100*time.Millisecond)
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
