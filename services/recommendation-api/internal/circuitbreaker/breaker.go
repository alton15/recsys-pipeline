package circuitbreaker

import (
	"sync/atomic"
	"time"
)

// State represents the circuit breaker state.
type State int32

const (
	Closed   State = 0 // normal, requests flow
	Open     State = 1 // tripped, requests fail fast
	HalfOpen State = 2 // probing, limited requests
)

// Breaker is a simple circuit breaker with three states.
type Breaker struct {
	name         string
	state        atomic.Int32
	failCount    atomic.Int64
	successCount atomic.Int64
	threshold    int64
	resetTimeout time.Duration
	lastTripped  atomic.Int64 // unix nano timestamp
	probing      atomic.Bool  // ensures only one probe in HalfOpen
}

// New creates a Breaker that opens after `threshold` failures and
// transitions to HalfOpen after `resetTimeout`.
func New(name string, threshold int64, resetTimeout time.Duration) *Breaker {
	return &Breaker{
		name:         name,
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

// GetState returns the current state of the circuit breaker.
func (b *Breaker) GetState() State {
	return State(b.state.Load())
}

// Allow returns true if the request should be allowed through.
// Closed -> always true.
// Open -> check if resetTimeout elapsed, if so transition to HalfOpen and allow one probe.
// HalfOpen -> only one concurrent probe request is allowed.
func (b *Breaker) Allow() bool {
	switch State(b.state.Load()) {
	case Closed:
		return true
	case Open:
		trippedAt := b.lastTripped.Load()
		if time.Since(time.Unix(0, trippedAt)) >= b.resetTimeout {
			// CAS to HalfOpen: only one goroutine wins the transition.
			if b.state.CompareAndSwap(int32(Open), int32(HalfOpen)) {
				b.probing.Store(true)
			}
			// The transitioning goroutine and all others compete for the single probe slot.
			return b.tryProbe()
		}
		return false
	case HalfOpen:
		return b.tryProbe()
	default:
		return true
	}
}

// tryProbe allows exactly one probe request in HalfOpen state.
func (b *Breaker) tryProbe() bool {
	return b.probing.CompareAndSwap(true, false)
}

// RecordSuccess increments the success counter. If the breaker is in
// HalfOpen state, it transitions back to Closed and resets the fail count.
func (b *Breaker) RecordSuccess() {
	b.successCount.Add(1)
	if b.state.CompareAndSwap(int32(HalfOpen), int32(Closed)) {
		b.failCount.Store(0)
	}
}

// RecordFailure increments the fail counter. If the fail count reaches
// the threshold, the breaker transitions to Open and records the trip time.
func (b *Breaker) RecordFailure() {
	current := State(b.state.Load())

	// In HalfOpen, a failure immediately re-opens the breaker.
	if current == HalfOpen {
		b.state.Store(int32(Open))
		b.lastTripped.Store(time.Now().UnixNano())
		return
	}

	newCount := b.failCount.Add(1)
	if newCount >= b.threshold {
		b.state.Store(int32(Open))
		b.lastTripped.Store(time.Now().UnixNano())
	}
}
