package generator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/recsys-pipeline/traffic-simulator/internal/generator"
)

func TestGenerateUsers_Count(t *testing.T) {
	users := generator.GenerateUsers(1000)

	if len(users) != 1000 {
		t.Fatalf("expected 1000 users, got %d", len(users))
	}

	for i, u := range users {
		expectedID := fmt.Sprintf("u_%05d", i)
		if u.ID != expectedID {
			t.Errorf("user %d: expected ID %q, got %q", i, expectedID, u.ID)
		}
		if len(u.CategoryPrefs) < 3 || len(u.CategoryPrefs) > 10 {
			t.Errorf("user %s: expected 3-10 category prefs, got %d", u.ID, len(u.CategoryPrefs))
		}
	}
}

func TestGenerateItems_Categories(t *testing.T) {
	numCategories := 50
	items := generator.GenerateItems(10000, numCategories)

	if len(items) != 10000 {
		t.Fatalf("expected 10000 items, got %d", len(items))
	}

	// Verify all categories are represented
	categorySeen := make(map[string]bool)
	for _, item := range items {
		categorySeen[item.CategoryID] = true
	}

	for i := 0; i < numCategories; i++ {
		catID := fmt.Sprintf("cat_%03d", i)
		if !categorySeen[catID] {
			t.Errorf("category %s not represented in items", catID)
		}
	}

	// Verify item IDs and prices
	for i, item := range items {
		expectedID := fmt.Sprintf("i_%06d", i)
		if item.ID != expectedID {
			t.Errorf("item %d: expected ID %q, got %q", i, expectedID, item.ID)
		}
		if item.Price < 5000 || item.Price > 200000 {
			t.Errorf("item %s: price %d out of range [5000, 200000]", item.ID, item.Price)
		}
	}
}

func TestSeedDragonfly_PopulatesKeys(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	users := generator.GenerateUsers(100)
	items := generator.GenerateItems(1000, 50)

	if err := generator.SeedDragonfly(client, users, items); err != nil {
		t.Fatalf("SeedDragonfly failed: %v", err)
	}

	ctx := context.Background()

	// Verify rec:popular:top_k exists and is valid JSON array
	popularRaw, err := client.Get(ctx, "rec:popular:top_k").Result()
	if err != nil {
		t.Fatalf("rec:popular:top_k not found: %v", err)
	}
	var popularRecs []generator.Recommendation
	if err := json.Unmarshal([]byte(popularRaw), &popularRecs); err != nil {
		t.Fatalf("rec:popular:top_k is not valid JSON: %v", err)
	}
	if len(popularRecs) != 100 {
		t.Errorf("expected 100 popular items, got %d", len(popularRecs))
	}

	// Verify stock:next_bit_pos = "1000"
	nextBitPos, err := client.Get(ctx, "stock:next_bit_pos").Result()
	if err != nil {
		t.Fatalf("stock:next_bit_pos not found: %v", err)
	}
	if nextBitPos != "1000" {
		t.Errorf("expected stock:next_bit_pos = %q, got %q", "1000", nextBitPos)
	}

	// Verify rec:u_00000:top_k exists
	userRecRaw, err := client.Get(ctx, "rec:u_00000:top_k").Result()
	if err != nil {
		t.Fatalf("rec:u_00000:top_k not found: %v", err)
	}
	var userRecs []generator.Recommendation
	if err := json.Unmarshal([]byte(userRecRaw), &userRecs); err != nil {
		t.Fatalf("rec:u_00000:top_k is not valid JSON: %v", err)
	}
	if len(userRecs) != 100 {
		t.Errorf("expected 100 user recs, got %d", len(userRecs))
	}

	// Verify stock:id_map:i_000000 exists
	_, err = client.Get(ctx, "stock:id_map:i_000000").Result()
	if err != nil {
		t.Fatalf("stock:id_map:i_000000 not found: %v", err)
	}
}
