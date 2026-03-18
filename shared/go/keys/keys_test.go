package keys_test

import (
	"testing"

	"github.com/recsys-pipeline/shared/keys"
)

func TestRecTopK(t *testing.T) {
	got := keys.RecTopK("u123")
	want := "rec:u123:top_k"
	if got != want {
		t.Errorf("RecTopK(u123) = %q, want %q", got, want)
	}
}

func TestRecTTL(t *testing.T) {
	got := keys.RecTTL("u123")
	want := "rec:u123:ttl"
	if got != want {
		t.Errorf("RecTTL(u123) = %q, want %q", got, want)
	}
}

func TestFeatUserClicks1H(t *testing.T) {
	got := keys.FeatUserClicks1H("u456")
	want := "feat:user:u456:clicks_1h"
	if got != want {
		t.Errorf("FeatUserClicks1H(u456) = %q, want %q", got, want)
	}
}

func TestFeatUserViews1D(t *testing.T) {
	got := keys.FeatUserViews1D("u456")
	want := "feat:user:u456:views_1d"
	if got != want {
		t.Errorf("FeatUserViews1D(u456) = %q, want %q", got, want)
	}
}

func TestFeatUserRecentItems(t *testing.T) {
	got := keys.FeatUserRecentItems("u789")
	want := "feat:user:u789:recent_items"
	if got != want {
		t.Errorf("FeatUserRecentItems(u789) = %q, want %q", got, want)
	}
}

func TestFeatItemCTR7D(t *testing.T) {
	got := keys.FeatItemCTR7D("item-100")
	want := "feat:item:item-100:ctr_7d"
	if got != want {
		t.Errorf("FeatItemCTR7D(item-100) = %q, want %q", got, want)
	}
}

func TestFeatItemPopularity(t *testing.T) {
	got := keys.FeatItemPopularity("item-200")
	want := "feat:item:item-200:popularity"
	if got != want {
		t.Errorf("FeatItemPopularity(item-200) = %q, want %q", got, want)
	}
}

func TestStockBitmap(t *testing.T) {
	got := keys.StockBitmap()
	want := "stock:bitmap"
	if got != want {
		t.Errorf("StockBitmap() = %q, want %q", got, want)
	}
}

func TestStockIDMap(t *testing.T) {
	got := keys.StockIDMap("item-300")
	want := "stock:id_map:item-300"
	if got != want {
		t.Errorf("StockIDMap(item-300) = %q, want %q", got, want)
	}
}

func TestSessionEvents(t *testing.T) {
	got := keys.SessionEvents("sess-abc")
	want := "session:sess-abc:events"
	if got != want {
		t.Errorf("SessionEvents(sess-abc) = %q, want %q", got, want)
	}
}

func TestExperimentConfig(t *testing.T) {
	got := keys.ExperimentConfig("exp-001")
	want := "experiment:exp-001"
	if got != want {
		t.Errorf("ExperimentConfig(exp-001) = %q, want %q", got, want)
	}
}
