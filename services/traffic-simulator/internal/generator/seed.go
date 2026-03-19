package generator

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	batchSize  = 1000
	recTTL     = 7 * 24 * time.Hour
	topKCount  = 100
	popularKey = "rec:popular:top_k"
	nextBitKey = "stock:next_bit_pos"
)

// SeedDragonfly populates DragonflyDB with stock mappings, popular items,
// and per-user recommendations using pipelined writes in batches of 1000.
func SeedDragonfly(client *redis.Client, users []User, items []Item) error {
	ctx := context.Background()

	// Build category → items index for recommendation generation.
	catItems := buildCategoryIndex(items)

	// Phase 1: Seed stock bitmap mappings.
	if err := seedStockMappings(ctx, client, items); err != nil {
		return fmt.Errorf("seed stock mappings: %w", err)
	}

	// Phase 2: Seed popular items.
	if err := seedPopularItems(ctx, client, items); err != nil {
		return fmt.Errorf("seed popular items: %w", err)
	}

	// Phase 3: Seed per-user recommendations.
	if err := seedUserRecommendations(ctx, client, users, catItems); err != nil {
		return fmt.Errorf("seed user recommendations: %w", err)
	}

	return nil
}

func seedStockMappings(ctx context.Context, client *redis.Client, items []Item) error {
	pipe := client.Pipeline()
	for i, item := range items {
		pipe.Set(ctx, fmt.Sprintf("stock:id_map:%s", item.ID), i, 0)

		if (i+1)%batchSize == 0 || i == len(items)-1 {
			if _, err := pipe.Exec(ctx); err != nil {
				return fmt.Errorf("exec stock batch at %d: %w", i, err)
			}
			pipe = client.Pipeline()
		}
	}

	// Set next_bit_pos to total item count.
	return client.Set(ctx, nextBitKey, len(items), 0).Err()
}

func seedPopularItems(ctx context.Context, client *redis.Client, items []Item) error {
	count := topKCount
	if len(items) < count {
		count = len(items)
	}

	// Pick first `count` items and assign descending scores.
	recs := make([]Recommendation, count)
	for i := range count {
		recs[i] = Recommendation{
			ItemID: items[i].ID,
			Score:  float64(count - i),
		}
	}

	data, err := json.Marshal(recs)
	if err != nil {
		return fmt.Errorf("marshal popular recs: %w", err)
	}

	return client.Set(ctx, popularKey, data, 0).Err()
}

func seedUserRecommendations(ctx context.Context, client *redis.Client, users []User, catItems map[string][]Item) error {
	pipe := client.Pipeline()
	for i, user := range users {
		recs := generateUserRecs(user, catItems)
		data, err := json.Marshal(recs)
		if err != nil {
			return fmt.Errorf("marshal recs for %s: %w", user.ID, err)
		}

		key := fmt.Sprintf("rec:%s:top_k", user.ID)
		pipe.Set(ctx, key, data, recTTL)

		if (i+1)%batchSize == 0 || i == len(users)-1 {
			if _, err := pipe.Exec(ctx); err != nil {
				return fmt.Errorf("exec user rec batch at %d: %w", i, err)
			}
			pipe = client.Pipeline()
		}
	}
	return nil
}

// generateUserRecs builds up to topKCount recommendations for a user, preferring
// items that match the user's category preferences. When preferred categories
// don't provide enough candidates, items from other categories are added to
// fill the remaining slots.
func generateUserRecs(user User, catItems map[string][]Item) []Recommendation {
	// Collect candidate items from preferred categories.
	prefSet := make(map[string]struct{}, len(user.CategoryPrefs))
	var candidates []Item
	for _, cat := range user.CategoryPrefs {
		prefSet[cat] = struct{}{}
		candidates = append(candidates, catItems[cat]...)
	}

	// If preferred categories don't provide enough, supplement from other categories.
	if len(candidates) < topKCount {
		for cat, items := range catItems {
			if _, preferred := prefSet[cat]; preferred {
				continue
			}
			candidates = append(candidates, items...)
			if len(candidates) >= topKCount {
				break
			}
		}
	}

	// Shuffle and deduplicate (categories might overlap in theory, but IDs are unique per category).
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	count := topKCount
	if len(candidates) < count {
		count = len(candidates)
	}

	recs := make([]Recommendation, count)
	for i := range count {
		recs[i] = Recommendation{
			ItemID: candidates[i].ID,
			Score:  float64(count-i) / float64(count), // 1.0 → 0.01
		}
	}

	// Sort descending by score for consistency.
	sort.Slice(recs, func(i, j int) bool {
		return recs[i].Score > recs[j].Score
	})

	return recs
}

// buildCategoryIndex groups items by their CategoryID.
func buildCategoryIndex(items []Item) map[string][]Item {
	idx := make(map[string][]Item)
	for _, item := range items {
		idx[item.CategoryID] = append(idx[item.CategoryID], item)
	}
	return idx
}
