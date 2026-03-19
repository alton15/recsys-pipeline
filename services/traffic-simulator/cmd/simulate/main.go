package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// event mirrors the shared Event struct for JSON serialization.
type event struct {
	EventID    string   `json:"event_id"`
	UserID     string   `json:"user_id"`
	EventType  string   `json:"event_type"`
	ItemID     string   `json:"item_id"`
	CategoryID string   `json:"category_id"`
	Timestamp  string   `json:"timestamp"`
	SessionID  string   `json:"session_id"`
	Metadata   metadata `json:"metadata"`
}

type metadata struct {
	Position int    `json:"position,omitempty"`
	Price    int64  `json:"price,omitempty"`
	Source   string `json:"source,omitempty"`
}

var eventTypes = []string{"click", "view", "purchase", "search", "add_to_cart", "remove_from_cart"}

func main() {
	collectorURL := os.Getenv("EVENT_COLLECTOR_URL")
	if collectorURL == "" {
		collectorURL = "http://localhost:8080"
	}

	totalEvents := 1000
	if v := os.Getenv("TOTAL_EVENTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid TOTAL_EVENTS: %v", err)
		}
		totalEvents = n
	}

	concurrency := 10
	if v := os.Getenv("CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid CONCURRENCY: %v", err)
		}
		concurrency = n
	}

	log.Printf("simulating %d events against %s (concurrency=%d)", totalEvents, collectorURL, concurrency)

	client := &http.Client{Timeout: 5 * time.Second}
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	var (
		mu       sync.Mutex
		sent     int
		failures int
	)

	start := time.Now()

	for i := range totalEvents {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			evt := randomEvent(idx)
			body, err := json.Marshal(evt)
			if err != nil {
				log.Printf("marshal error: %v", err)
				mu.Lock()
				failures++
				mu.Unlock()
				return
			}

			resp, err := client.Post(collectorURL+"/api/v1/events", "application/json", bytes.NewReader(body))
			if err != nil {
				mu.Lock()
				failures++
				mu.Unlock()
				return
			}
			resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				mu.Lock()
				sent++
				mu.Unlock()
			} else {
				mu.Lock()
				failures++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	log.Printf("simulation complete: sent=%d failures=%d elapsed=%s rps=%.1f",
		sent, failures, elapsed, float64(sent)/elapsed.Seconds())
}

func randomEvent(idx int) event {
	userID := fmt.Sprintf("user_%06d", rand.Intn(100000))
	itemID := fmt.Sprintf("item_%07d", rand.Intn(1000000))
	categoryID := fmt.Sprintf("cat_%03d", rand.Intn(50))
	sessionID := fmt.Sprintf("sess_%010d", rand.Intn(1000000000))
	eventType := eventTypes[rand.Intn(len(eventTypes))]

	return event{
		EventID:    fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), idx),
		UserID:     userID,
		EventType:  eventType,
		ItemID:     itemID,
		CategoryID: categoryID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		SessionID:  sessionID,
		Metadata: metadata{
			Position: rand.Intn(50) + 1,
			Price:    int64(rand.Intn(100000) + 100),
			Source:   "traffic-simulator",
		},
	}
}
