package stock

import (
	"context"
	"log"
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
	"github.com/recsys-pipeline/shared/keys"
)

// BitmapChecker determines stock availability using a DragonflyDB bitmap.
// Each item is mapped to a bit position via stock:id_map:{item_id}.
// A set bit (1) at that position in stock:bitmap means out-of-stock.
type BitmapChecker struct {
	client *redis.Client
}

// NewBitmapChecker creates a BitmapChecker connected to the given address.
func NewBitmapChecker(addr string) *BitmapChecker {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		PoolSize: 100,
	})
	return &BitmapChecker{client: client}
}

// IsOutOfStock checks whether the given item is out of stock.
// Returns false (in-stock) for unknown items (fail-open).
func (c *BitmapChecker) IsOutOfStock(itemID string) (bool, error) {
	ctx := context.Background()

	// Look up the bitmap offset for this item.
	posStr, err := c.client.Get(ctx, keys.StockIDMap(itemID)).Result()
	if err == redis.Nil {
		// Unknown item → assume in stock.
		return false, nil
	}
	if err != nil {
		return false, err
	}

	pos, err := strconv.ParseInt(posStr, 10, 64)
	if err != nil {
		// Invalid offset → fail open, assume in stock.
		return false, nil
	}

	// Check the bit: 1 = out of stock.
	bit, err := c.client.GetBit(ctx, keys.StockBitmap(), pos).Result()
	if err != nil {
		return false, err
	}

	return bit == 1, nil
}

// FilterOutOfStock removes out-of-stock items from the slice using Redis
// pipelining to minimize round-trips. Two pipeline executions are used:
// one for id_map lookups and one for GETBIT checks.
// On per-item errors, the item is included (fail-open).
func (c *BitmapChecker) FilterOutOfStock(items []tier.Recommendation) ([]tier.Recommendation, error) {
	if len(items) == 0 {
		return items, nil
	}

	ctx := context.Background()

	// Pipeline 1: Batch all id_map lookups.
	idMapPipe := c.client.Pipeline()
	idMapCmds := make([]*redis.StringCmd, len(items))
	for i, item := range items {
		idMapCmds[i] = idMapPipe.Get(ctx, keys.StockIDMap(item.ItemID))
	}
	_, _ = idMapPipe.Exec(ctx) // Errors are checked per-command below.

	// Collect valid offsets for GETBIT pipeline.
	type offsetEntry struct {
		index  int
		offset int64
	}
	var validOffsets []offsetEntry

	// Track which items should be included (default: true, fail-open).
	include := make([]bool, len(items))
	for i := range include {
		include[i] = true
	}

	for i, cmd := range idMapCmds {
		posStr, err := cmd.Result()
		if err == redis.Nil {
			// Unknown item -> assume in stock (fail-open).
			continue
		}
		if err != nil {
			log.Printf("stock id_map lookup failed for %s, including item: %v", items[i].ItemID, err)
			continue
		}
		pos, err := strconv.ParseInt(posStr, 10, 64)
		if err != nil {
			// Invalid offset -> fail open.
			continue
		}
		validOffsets = append(validOffsets, offsetEntry{index: i, offset: pos})
	}

	// Pipeline 2: Batch all GETBIT checks.
	if len(validOffsets) > 0 {
		bitPipe := c.client.Pipeline()
		bitCmds := make([]*redis.IntCmd, len(validOffsets))
		bitmapKey := keys.StockBitmap()
		for j, entry := range validOffsets {
			bitCmds[j] = bitPipe.GetBit(ctx, bitmapKey, entry.offset)
		}
		_, _ = bitPipe.Exec(ctx) // Errors are checked per-command below.

		for j, cmd := range bitCmds {
			bit, err := cmd.Result()
			if err != nil {
				log.Printf("stock getbit failed for %s, including item: %v", items[validOffsets[j].index].ItemID, err)
				continue
			}
			if bit == 1 {
				include[validOffsets[j].index] = false
			}
		}
	}

	// Build result slice with only in-stock items.
	result := make([]tier.Recommendation, 0, len(items))
	for i, item := range items {
		if include[i] {
			result = append(result, item)
		}
	}
	return result, nil
}

// Close shuts down the underlying Redis client.
func (c *BitmapChecker) Close() error {
	return c.client.Close()
}
