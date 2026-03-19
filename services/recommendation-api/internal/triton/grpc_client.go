package triton

import (
	"context"
	"fmt"
	"log"
)

// grpcClient is a concrete implementation of GRPCInferenceClient that
// connects to the Triton Inference Server via gRPC.
//
// NOTE: This is a stub that logs the target address and returns an error
// indicating that real gRPC inference is not yet wired. When the Triton
// proto stubs are generated, replace the body of ModelInfer with a real
// gRPC call. The important thing is that the address is captured and the
// plumbing is in place.
type grpcClient struct {
	addr string
}

// NewGRPCClient creates a GRPCInferenceClient targeting the given address.
func NewGRPCClient(addr string) GRPCInferenceClient {
	log.Printf("triton gRPC client targeting %s (stub — replace with real proto client)", addr)
	return &grpcClient{addr: addr}
}

// ModelInfer sends an inference request to Triton.
// TODO: Replace with real gRPC call once proto stubs are generated.
func (c *grpcClient) ModelInfer(_ context.Context, _ *InferRequest) (*InferResponse, error) {
	return nil, fmt.Errorf("triton gRPC inference not yet implemented (target: %s)", c.addr)
}
