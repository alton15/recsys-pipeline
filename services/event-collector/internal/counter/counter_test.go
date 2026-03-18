package counter_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/recsys-pipeline/event-collector/internal/counter"
)

func TestCounter_IncrementAndGet(t *testing.T) {
	c := counter.New()

	c.Increment("u-1", "click")
	c.Increment("u-1", "click")
	c.Increment("u-1", "view")

	if got := c.Get("u-1", "click"); got != 2 {
		t.Errorf("expected click count 2, got %d", got)
	}
	if got := c.Get("u-1", "view"); got != 1 {
		t.Errorf("expected view count 1, got %d", got)
	}
	if got := c.Get("u-1", "purchase"); got != 0 {
		t.Errorf("expected purchase count 0, got %d", got)
	}
}

func TestCounter_ConcurrentAccess(t *testing.T) {
	c := counter.New()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			userID := fmt.Sprintf("u-%d", id%5)
			c.Increment(userID, "click")
		}(i)
	}

	wg.Wait()

	var total int64
	for i := 0; i < 5; i++ {
		total += c.Get(fmt.Sprintf("u-%d", i), "click")
	}

	if total != goroutines {
		t.Errorf("expected total %d, got %d", goroutines, total)
	}
}
