package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/recsys-pipeline/event-collector/internal/handler"
	"github.com/recsys-pipeline/event-collector/internal/producer"
)

func main() {
	brokers := getEnv("REDPANDA_BROKERS", "localhost:9092")
	topic := getEnv("EVENTS_TOPIC", "user-events")

	p, err := producer.NewRedpanda(strings.Split(brokers, ","), topic)
	if err != nil {
		log.Fatalf("failed to create producer: %v", err)
	}
	defer p.Close()

	h := handler.New(p)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/events", h.HandleEvent)
	mux.HandleFunc("/health", h.HandleHealth)

	appServer := &http.Server{
		Addr:         ":8080",
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
		log.Printf("event-collector listening on :8080 (brokers=%s, topic=%s)", brokers, topic)
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

	if err := appServer.Shutdown(ctx); err != nil {
		log.Printf("app server shutdown error: %v", err)
	}
	if err := metricsServer.Shutdown(ctx); err != nil {
		log.Printf("metrics server shutdown error: %v", err)
	}
	log.Println("server stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
