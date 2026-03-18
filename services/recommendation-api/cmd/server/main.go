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
	"github.com/recsys-pipeline/recommendation-api/internal/handler"
	"github.com/recsys-pipeline/recommendation-api/internal/stock"
	"github.com/recsys-pipeline/recommendation-api/internal/store"
	"github.com/recsys-pipeline/recommendation-api/internal/tier"
)

func main() {
	dragonflyAddr := getEnv("DRAGONFLY_ADDR", "localhost:6379")

	// Initialize dependencies.
	dfStore := store.NewDragonflyStore(dragonflyAddr)
	defer dfStore.Close()

	bitmapChecker := stock.NewBitmapChecker(dragonflyAddr)
	defer bitmapChecker.Close()

	// No reranker for Tier 1 — will be added in Task 6.
	router := tier.NewRouter(dfStore, bitmapChecker, nil)

	h := handler.NewRecommendHandler(router)

	// Application server.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/recommend", h.HandleRecommend)
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
