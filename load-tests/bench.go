package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	recommendURL := getEnv("RECOMMEND_URL", "http://localhost:8090")
	eventURL := getEnv("EVENT_URL", "http://localhost:8080")
	vus := getEnvInt("VUS", 100)
	duration := getEnvInt("DURATION_SECS", 30)

	fmt.Printf("=== Recommendation System Benchmark ===\n")
	fmt.Printf("Recommend API: %s\n", recommendURL)
	fmt.Printf("Event Collector: %s\n", eventURL)
	fmt.Printf("Virtual Users: %d, Duration: %ds\n\n", vus, duration)

	// Phase 1: Recommendation API benchmark
	fmt.Println("--- Phase 1: Recommendation API ---")
	recResult := runBenchmark(vus, time.Duration(duration)*time.Second, func(client *http.Client) (time.Duration, error) {
		userID := fmt.Sprintf("u_%06d", rand.Intn(100000))
		sessionID := ""
		if rand.Float32() < 0.6 {
			sessionID = fmt.Sprintf("&session_id=sess_%010d", rand.Intn(1000000000))
		}
		url := fmt.Sprintf("%s/api/v1/recommend?user_id=%s&limit=20%s", recommendURL, userID, sessionID)

		start := time.Now()
		resp, err := client.Get(url)
		elapsed := time.Since(start)
		if err != nil {
			return elapsed, err
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return elapsed, fmt.Errorf("status %d", resp.StatusCode)
		}
		return elapsed, nil
	})
	printResult(recResult)

	// Phase 2: Event Collector benchmark
	fmt.Println("\n--- Phase 2: Event Collector ---")
	eventTypes := []string{"click", "view", "purchase", "search", "add_to_cart"}
	evtResult := runBenchmark(vus, time.Duration(duration)*time.Second, func(client *http.Client) (time.Duration, error) {
		evt := map[string]interface{}{
			"event_type": eventTypes[rand.Intn(len(eventTypes))],
			"user_id":    fmt.Sprintf("u_%06d", rand.Intn(100000)),
			"item_id":    fmt.Sprintf("i_%06d", rand.Intn(1000000)),
		}
		body, _ := json.Marshal(evt)

		start := time.Now()
		resp, err := client.Post(eventURL+"/api/v1/events", "application/json",
			newReader(body))
		elapsed := time.Since(start)
		if err != nil {
			return elapsed, err
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return elapsed, fmt.Errorf("status %d", resp.StatusCode)
		}
		return elapsed, nil
	})
	printResult(evtResult)

	// Phase 3: Mixed workload (80% recommend, 20% events)
	fmt.Println("\n--- Phase 3: Mixed Workload (80%% recommend, 20%% events) ---")
	mixResult := runBenchmark(vus, time.Duration(duration)*time.Second, func(client *http.Client) (time.Duration, error) {
		if rand.Float32() < 0.8 {
			userID := fmt.Sprintf("u_%06d", rand.Intn(100000))
			url := fmt.Sprintf("%s/api/v1/recommend?user_id=%s&limit=20", recommendURL, userID)
			start := time.Now()
			resp, err := client.Get(url)
			elapsed := time.Since(start)
			if err != nil {
				return elapsed, err
			}
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				return elapsed, fmt.Errorf("status %d", resp.StatusCode)
			}
			return elapsed, nil
		}
		evt := map[string]interface{}{
			"event_type": eventTypes[rand.Intn(len(eventTypes))],
			"user_id":    fmt.Sprintf("u_%06d", rand.Intn(100000)),
			"item_id":    fmt.Sprintf("i_%06d", rand.Intn(1000000)),
		}
		body, _ := json.Marshal(evt)
		start := time.Now()
		resp, err := client.Post(eventURL+"/api/v1/events", "application/json", newReader(body))
		elapsed := time.Since(start)
		if err != nil {
			return elapsed, err
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return elapsed, fmt.Errorf("status %d", resp.StatusCode)
		}
		return elapsed, nil
	})
	printResult(mixResult)

	// Summary
	fmt.Println("\n=== 50M DAU Scalability Analysis ===")
	analyzeScalability(recResult, evtResult)
}

type benchResult struct {
	totalRequests int64
	errors        int64
	latencies     []time.Duration
	elapsed       time.Duration
}

func runBenchmark(vus int, duration time.Duration, reqFn func(*http.Client) (time.Duration, error)) benchResult {
	var totalReqs atomic.Int64
	var totalErrors atomic.Int64
	var mu sync.Mutex
	var allLatencies []time.Duration

	start := time.Now()
	done := make(chan struct{})
	time.AfterFunc(duration, func() { close(done) })

	var wg sync.WaitGroup
	for i := range vus {
		_ = i
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{
				Timeout: 5 * time.Second,
				Transport: &http.Transport{
					MaxIdleConns:        200,
					MaxIdleConnsPerHost: 200,
					IdleConnTimeout:     90 * time.Second,
				},
			}
			var localLatencies []time.Duration

			for {
				select {
				case <-done:
					mu.Lock()
					allLatencies = append(allLatencies, localLatencies...)
					mu.Unlock()
					return
				default:
				}

				lat, err := reqFn(client)
				totalReqs.Add(1)
				localLatencies = append(localLatencies, lat)
				if err != nil {
					totalErrors.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	return benchResult{
		totalRequests: totalReqs.Load(),
		errors:        totalErrors.Load(),
		latencies:     allLatencies,
		elapsed:       elapsed,
	}
}

func printResult(r benchResult) {
	rps := float64(r.totalRequests) / r.elapsed.Seconds()
	errorRate := float64(r.errors) / float64(r.totalRequests) * 100

	sort.Slice(r.latencies, func(i, j int) bool { return r.latencies[i] < r.latencies[j] })

	p50 := percentile(r.latencies, 0.50)
	p95 := percentile(r.latencies, 0.95)
	p99 := percentile(r.latencies, 0.99)
	maxLat := r.latencies[len(r.latencies)-1]

	fmt.Printf("  Total Requests: %d\n", r.totalRequests)
	fmt.Printf("  RPS:            %.0f req/s\n", rps)
	fmt.Printf("  Error Rate:     %.2f%%\n", errorRate)
	fmt.Printf("  Latency p50:    %s\n", p50)
	fmt.Printf("  Latency p95:    %s\n", p95)
	fmt.Printf("  Latency p99:    %s\n", p99)
	fmt.Printf("  Latency max:    %s\n", maxLat)
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func analyzeScalability(recResult, evtResult benchResult) {
	recRPS := float64(recResult.totalRequests) / recResult.elapsed.Seconds()
	evtRPS := float64(evtResult.totalRequests) / evtResult.elapsed.Seconds()

	// 50M DAU assumptions
	const (
		dau             = 50_000_000
		peakMultiplier  = 10.0  // peak is 10x average
		reqsPerUserDay  = 20.0  // avg requests per user per day
		secondsPerDay   = 86400
	)

	avgRPS := float64(dau) * reqsPerUserDay / secondsPerDay
	peakRPS := avgRPS * peakMultiplier

	fmt.Printf("\n  Target: 50M DAU\n")
	fmt.Printf("  Assumed %0.f requests/user/day\n", reqsPerUserDay)
	fmt.Printf("  Average RPS needed: %.0f\n", avgRPS)
	fmt.Printf("  Peak RPS needed (10x):  %.0f\n", peakRPS)

	fmt.Printf("\n  Current single-instance performance:\n")
	fmt.Printf("    Recommendation API: %.0f RPS\n", recRPS)
	fmt.Printf("    Event Collector:    %.0f RPS\n", evtRPS)

	recInstances := peakRPS / recRPS
	evtInstances := peakRPS / evtRPS

	fmt.Printf("\n  Estimated instances needed for peak:\n")
	fmt.Printf("    Recommendation API: %.0f instances (Helm production: 250 base, 1000 max)\n", recInstances)
	fmt.Printf("    Event Collector:    %.0f instances (Helm production: 50 base, 200 max)\n", evtInstances)

	sort.Slice(recResult.latencies, func(i, j int) bool { return recResult.latencies[i] < recResult.latencies[j] })
	recP99 := percentile(recResult.latencies, 0.99)

	fmt.Printf("\n  SLA Compliance:\n")
	fmt.Printf("    Recommendation p99 < 200ms: %v (actual: %s)\n", recP99 < 200*time.Millisecond, recP99)
	fmt.Printf("    Error rate < 1%%:            %v (actual: %.2f%%)\n",
		float64(recResult.errors)/float64(recResult.totalRequests)*100 < 1,
		float64(recResult.errors)/float64(recResult.totalRequests)*100)
}

func newReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}
