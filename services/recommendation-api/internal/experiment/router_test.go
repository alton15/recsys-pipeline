package experiment_test

import (
	"errors"
	"math"
	"testing"

	"github.com/recsys-pipeline/recommendation-api/internal/experiment"
)

// mockStore implements experiment.Store for testing.
type mockStore struct {
	experiments []experiment.Experiment
	err         error
}

func (m *mockStore) ListActiveExperiments() ([]experiment.Experiment, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return a copy to preserve immutability.
	out := make([]experiment.Experiment, len(m.experiments))
	copy(out, m.experiments)
	return out, nil
}

func TestHashBucket_ConsistentForSameUser(t *testing.T) {
	store := &mockStore{
		experiments: []experiment.Experiment{
			{ID: "exp-1", ModelVersion: "v1", TrafficPct: 50},
			{ID: "exp-2", ModelVersion: "v2", TrafficPct: 50},
		},
	}
	router := experiment.NewRouter(store)

	// Same user should always get the same experiment.
	first, err := router.GetExperiment("user-abc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 100; i++ {
		got, err := router.GetExperiment("user-abc-123")
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
		if first == nil && got != nil || first != nil && got == nil {
			t.Fatalf("inconsistent nil result on iteration %d", i)
		}
		if first != nil && got.ID != first.ID {
			t.Fatalf("expected experiment %s, got %s on iteration %d", first.ID, got.ID, i)
		}
	}
}

func TestTrafficSplit_ApproximatelyEven(t *testing.T) {
	store := &mockStore{
		experiments: []experiment.Experiment{
			{ID: "exp-a", ModelVersion: "v1", TrafficPct: 50},
			{ID: "exp-b", ModelVersion: "v2", TrafficPct: 50},
		},
	}
	router := experiment.NewRouter(store)

	counts := map[string]int{}
	totalUsers := 10000

	for i := 0; i < totalUsers; i++ {
		userID := "user-" + itoa(i)
		exp, err := router.GetExperiment(userID)
		if err != nil {
			t.Fatalf("unexpected error for user %s: %v", userID, err)
		}
		if exp == nil {
			counts["control"]++
		} else {
			counts[exp.ID]++
		}
	}

	// With 50/50 split over 10k users, each experiment should have roughly 5000.
	// Allow 10% tolerance.
	for _, id := range []string{"exp-a", "exp-b"} {
		ratio := float64(counts[id]) / float64(totalUsers)
		if math.Abs(ratio-0.5) > 0.1 {
			t.Errorf("experiment %s got %.2f%% (expected ~50%%)", id, ratio*100)
		}
	}
}

func TestNoExperiments_ReturnsNil(t *testing.T) {
	store := &mockStore{experiments: []experiment.Experiment{}}
	router := experiment.NewRouter(store)

	exp, err := router.GetExperiment("user-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp != nil {
		t.Errorf("expected nil for control group, got %+v", exp)
	}
}

func TestStoreError_ReturnsError(t *testing.T) {
	storeErr := errors.New("connection refused")
	store := &mockStore{err: storeErr}
	router := experiment.NewRouter(store)

	exp, err := router.GetExperiment("user-xyz")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("expected store error, got: %v", err)
	}
	if exp != nil {
		t.Errorf("expected nil experiment on error, got %+v", exp)
	}
}

func TestFullTraffic_AllUsersAssigned(t *testing.T) {
	store := &mockStore{
		experiments: []experiment.Experiment{
			{ID: "exp-full", ModelVersion: "v3", TrafficPct: 100},
		},
	}
	router := experiment.NewRouter(store)

	for i := 0; i < 1000; i++ {
		userID := "user-" + itoa(i)
		exp, err := router.GetExperiment(userID)
		if err != nil {
			t.Fatalf("unexpected error for user %s: %v", userID, err)
		}
		if exp == nil {
			t.Fatalf("expected all users assigned, but user %s got nil", userID)
		}
		if exp.ID != "exp-full" {
			t.Errorf("expected exp-full, got %s", exp.ID)
		}
	}
}

// itoa converts an int to a string without importing strconv to keep test deps minimal.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
