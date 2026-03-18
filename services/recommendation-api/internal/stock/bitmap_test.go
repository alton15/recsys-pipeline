package stock_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/recsys-pipeline/recommendation-api/internal/stock"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client, *stock.BitmapChecker) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := stock.NewBitmapChecker(mr.Addr())
	return mr, rdb, c
}

func TestBitmapChecker_InStockItem(t *testing.T) {
	_, rdb, c := setupMiniredis(t)
	ctx := context.Background()

	// Offset 5, bit=0 (default) → in stock.
	rdb.Set(ctx, "stock:id_map:item1", "5", 0)

	oos, err := c.IsOutOfStock("item1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oos {
		t.Error("expected item to be in stock (bit=0)")
	}
}

func TestBitmapChecker_OutOfStockItem(t *testing.T) {
	_, rdb, c := setupMiniredis(t)
	ctx := context.Background()

	// Offset 3, bit=1 → out of stock.
	rdb.Set(ctx, "stock:id_map:item2", "3", 0)
	rdb.SetBit(ctx, "stock:bitmap", 3, 1)

	oos, err := c.IsOutOfStock("item2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !oos {
		t.Error("expected item to be out of stock (bit=1)")
	}
}

func TestBitmapChecker_UnknownItem(t *testing.T) {
	_, _, c := setupMiniredis(t)

	// No mapping exists → assume in stock.
	oos, err := c.IsOutOfStock("unknown_item")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oos {
		t.Error("expected unknown item to be assumed in stock")
	}
}

func TestBitmapChecker_FilterOutOfStock(t *testing.T) {
	_, rdb, c := setupMiniredis(t)
	ctx := context.Background()

	// item_a at offset 0, in stock (bit=0).
	rdb.Set(ctx, "stock:id_map:item_a", "0", 0)

	// item_b at offset 1, out of stock (bit=1).
	rdb.Set(ctx, "stock:id_map:item_b", "1", 0)
	rdb.SetBit(ctx, "stock:bitmap", 1, 1)

	items := []tier.Recommendation{
		{ItemID: "item_a", Score: 0.9},
		{ItemID: "item_b", Score: 0.8},
	}

	got, err := c.FilterOutOfStock(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 item after filtering, got %d", len(got))
	}
	if got[0].ItemID != "item_a" {
		t.Errorf("expected item_a, got %s", got[0].ItemID)
	}
}

func TestBitmapChecker_FilterOutOfStock_FailOpen(t *testing.T) {
	_, rdb, c := setupMiniredis(t)
	ctx := context.Background()

	// item_a has a valid mapping, in stock.
	rdb.Set(ctx, "stock:id_map:item_a", "0", 0)

	// item_b has an invalid (non-numeric) mapping → fail open, include it.
	rdb.Set(ctx, "stock:id_map:item_b", "not_a_number", 0)

	items := []tier.Recommendation{
		{ItemID: "item_a", Score: 0.9},
		{ItemID: "item_b", Score: 0.8},
	}

	got, err := c.FilterOutOfStock(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 items (fail-open), got %d", len(got))
	}
}
