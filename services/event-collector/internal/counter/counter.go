package counter

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Counter tracks event counts per user and event type using thread-safe primitives.
type Counter struct {
	store sync.Map // key: "userID:eventType" → *atomic.Int64
}

// New creates a new Counter.
func New() *Counter {
	return &Counter{}
}

func key(userID, eventType string) string {
	return fmt.Sprintf("%s:%s", userID, eventType)
}

// Increment atomically increments the count for the given user and event type,
// returning the new value.
func (c *Counter) Increment(userID, eventType string) int64 {
	k := key(userID, eventType)
	val, _ := c.store.LoadOrStore(k, &atomic.Int64{})
	return val.(*atomic.Int64).Add(1)
}

// Get returns the current count for the given user and event type.
// Returns 0 if no events have been recorded.
func (c *Counter) Get(userID, eventType string) int64 {
	k := key(userID, eventType)
	val, ok := c.store.Load(k)
	if !ok {
		return 0
	}
	return val.(*atomic.Int64).Load()
}
