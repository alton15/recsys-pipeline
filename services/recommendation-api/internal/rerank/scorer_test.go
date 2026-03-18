package rerank_test

import (
	"testing"

	"github.com/recsys-pipeline/recommendation-api/internal/rerank"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

func TestWeightedScorer_PenalizesClickedItems(t *testing.T) {
	scorer := rerank.NewWeightedScorer(rerank.DefaultWeights())

	items := []tier.Recommendation{
		{ItemID: "clicked_item", Score: 0.9},
		{ItemID: "fresh_item", Score: 0.8},
	}
	features := &rerank.SessionFeatures{
		ClickedItems:  map[string]bool{"clicked_item": true},
		ViewedItems:   map[string]bool{},
		SessionLength: 1,
	}

	result := scorer.Rerank(items, features)

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	// The clicked item (0.9 + penalty) should rank below the fresh item (0.8)
	if result[0].ItemID != "fresh_item" {
		t.Errorf("expected fresh_item first after penalizing clicked, got %s", result[0].ItemID)
	}
	if result[1].ItemID != "clicked_item" {
		t.Errorf("expected clicked_item second, got %s", result[1].ItemID)
	}
}

func TestWeightedScorer_PreservesOrderWithoutSession(t *testing.T) {
	scorer := rerank.NewWeightedScorer(rerank.DefaultWeights())

	items := []tier.Recommendation{
		{ItemID: "a", Score: 0.9},
		{ItemID: "b", Score: 0.8},
		{ItemID: "c", Score: 0.7},
	}
	features := &rerank.SessionFeatures{
		ClickedItems:  map[string]bool{},
		ViewedItems:   map[string]bool{},
		SessionLength: 0,
	}

	result := scorer.Rerank(items, features)

	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	if result[0].ItemID != "a" || result[1].ItemID != "b" || result[2].ItemID != "c" {
		t.Errorf("expected order [a, b, c], got [%s, %s, %s]",
			result[0].ItemID, result[1].ItemID, result[2].ItemID)
	}
}

func TestWeightedScorer_ImmutableInput(t *testing.T) {
	scorer := rerank.NewWeightedScorer(rerank.DefaultWeights())

	items := []tier.Recommendation{
		{ItemID: "clicked", Score: 0.9},
		{ItemID: "fresh", Score: 0.8},
	}
	features := &rerank.SessionFeatures{
		ClickedItems:  map[string]bool{"clicked": true},
		ViewedItems:   map[string]bool{},
		SessionLength: 1,
	}

	// Capture original state.
	origFirst := items[0].ItemID
	origSecond := items[1].ItemID
	origScore0 := items[0].Score
	origScore1 := items[1].Score

	_ = scorer.Rerank(items, features)

	// Verify the original slice was not mutated.
	if items[0].ItemID != origFirst || items[1].ItemID != origSecond {
		t.Error("original items slice was mutated (order changed)")
	}
	if items[0].Score != origScore0 || items[1].Score != origScore1 {
		t.Error("original items slice was mutated (scores changed)")
	}
}
