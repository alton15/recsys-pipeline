package circuitbreaker_test

import (
	"testing"
	"time"

	"github.com/recsys-pipeline/recommendation-api/internal/circuitbreaker"
)

func TestBreaker_ClosedAllowsRequests(t *testing.T) {
	b := circuitbreaker.New("test", 5, 1*time.Second)

	if !b.Allow() {
		t.Fatal("expected Allow() to return true when circuit is closed")
	}

	if b.GetState() != circuitbreaker.Closed {
		t.Fatalf("expected state Closed, got %v", b.GetState())
	}
}

func TestBreaker_OpensAfterThreshold(t *testing.T) {
	b := circuitbreaker.New("test", 5, 1*time.Second)

	for i := int64(0); i < 5; i++ {
		b.RecordFailure()
	}

	if b.GetState() != circuitbreaker.Open {
		t.Fatalf("expected state Open after 5 failures, got %v", b.GetState())
	}
}

func TestBreaker_RejectsWhenOpen(t *testing.T) {
	b := circuitbreaker.New("test", 5, 1*time.Second)

	for i := int64(0); i < 5; i++ {
		b.RecordFailure()
	}

	if b.Allow() {
		t.Fatal("expected Allow() to return false when circuit is open")
	}
}

func TestBreaker_TransitionsToHalfOpen(t *testing.T) {
	b := circuitbreaker.New("test", 5, 50*time.Millisecond)

	for i := int64(0); i < 5; i++ {
		b.RecordFailure()
	}

	if b.GetState() != circuitbreaker.Open {
		t.Fatalf("expected state Open, got %v", b.GetState())
	}

	time.Sleep(60 * time.Millisecond)

	if !b.Allow() {
		t.Fatal("expected Allow() to return true after resetTimeout (should transition to HalfOpen)")
	}

	if b.GetState() != circuitbreaker.HalfOpen {
		t.Fatalf("expected state HalfOpen, got %v", b.GetState())
	}
}

func TestBreaker_ClosesOnSuccessFromHalfOpen(t *testing.T) {
	b := circuitbreaker.New("test", 5, 50*time.Millisecond)

	for i := int64(0); i < 5; i++ {
		b.RecordFailure()
	}

	time.Sleep(60 * time.Millisecond)

	// Transition to HalfOpen by calling Allow
	if !b.Allow() {
		t.Fatal("expected Allow() to return true (transition to HalfOpen)")
	}

	b.RecordSuccess()

	if b.GetState() != circuitbreaker.Closed {
		t.Fatalf("expected state Closed after success in HalfOpen, got %v", b.GetState())
	}
}
