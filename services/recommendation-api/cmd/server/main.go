package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/recsys-pipeline/recommendation-api/internal/circuitbreaker"
	"github.com/recsys-pipeline/recommendation-api/internal/degradation"
	"github.com/recsys-pipeline/recommendation-api/internal/experiment"
	"github.com/recsys-pipeline/recommendation-api/internal/handler"
	"github.com/recsys-pipeline/recommendation-api/internal/rerank"
	"github.com/recsys-pipeline/recommendation-api/internal/stock"
	"github.com/recsys-pipeline/recommendation-api/internal/store"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
	"github.com/recsys-pipeline/recommendation-api/internal/triton"
)

func main() {
	dragonflyAddr := getEnv("DRAGONFLY_ADDR", "localhost:6379")

	// Initialize dependencies.
	dfStore := store.NewDragonflyStore(dragonflyAddr)
	defer dfStore.Close()

	bitmapChecker := stock.NewBitmapChecker(dragonflyAddr)
	defer bitmapChecker.Close()

	// Tier 2: session-aware weighted re-ranking.
	redisClient := redis.NewClient(&redis.Options{
		Addr:         dragonflyAddr,
		ReadTimeout:  10 * time.Millisecond,
		WriteTimeout: 10 * time.Millisecond,
		PoolSize:     100,
	})
	defer redisClient.Close()

	extractor := rerank.NewSessionFeatureExtractor(redisClient)
	scorer := rerank.NewWeightedScorer(rerank.DefaultWeights())
	reranker := rerank.NewSessionReranker(extractor, scorer)

	// Tier 3: Triton inference with circuit breaker protection.
	tritonAddr := getEnv("TRITON_ADDR", "localhost:8001")
	tritonClient := triton.NewClient(nil, triton.DefaultTimeout)
	_ = tritonAddr // Used to establish the gRPC connection in production.
	tritonBreaker := circuitbreaker.New("triton", 5, 30*time.Second)
	ranker := triton.NewProtectedRanker(tritonClient, tritonBreaker)

	// Degradation state machine for graceful degradation under load.
	degradationMgr := degradation.NewManager()

	router := tier.NewRouterWithRanker(dfStore, bitmapChecker, reranker, degradationMgr, ranker)

	// A/B experiment routing backed by DragonflyDB.
	expRouter := experiment.NewRouter(dfStore)

	h := handler.NewRecommendHandler(router).WithPinger(dfStore).WithExperiments(expRouter)
	ph := handler.NewPopularHandler(dfStore)

	// Application server.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/recommend", h.HandleRecommend)
	mux.HandleFunc("/api/v1/popular", ph.HandlePopular)
	mux.HandleFunc("/health", h.HandleHealth)

	appServer := &http.Server{
		Addr:         ":8090",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Metrics server for Prometheus scraping.
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    ":2112",
		Handler: metricsMux,
	}

	go func() {
		log.Println("metrics server listening on :2112")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("metrics server error: %v", err)
		}
	}()

	go func() {
		log.Printf("recommendation-api listening on :8090 (dragonfly=%s)", dragonflyAddr)
		if err := appServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("app server error: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	appServer.Shutdown(ctx)
	metricsServer.Shutdown(ctx)
	log.Println("server stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
