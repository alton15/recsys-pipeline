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

// FilterOutOfStock removes out-of-stock items from the slice.
// On per-item errors, the item is included (fail-open).
func (c *BitmapChecker) FilterOutOfStock(items []tier.Recommendation) ([]tier.Recommendation, error) {
	var result []tier.Recommendation
	for _, item := range items {
		oos, err := c.IsOutOfStock(item.ItemID)
		if err != nil {
			// Fail open: include item if stock check fails.
			log.Printf("stock check failed for %s, including item: %v", item.ItemID, err)
			result = append(result, item)
			continue
		}
		if !oos {
			result = append(result, item)
		}
	}
	return result, nil
}

// Close shuts down the underlying Redis client.
func (c *BitmapChecker) Close() error {
	return c.client.Close()
}
