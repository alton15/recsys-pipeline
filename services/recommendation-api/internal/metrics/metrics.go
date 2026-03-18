package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// RequestsTotal counts total requests by tier.
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "recsys_requests_total",
			Help: "Total requests by tier",
		},
		[]string{"tier"},
	)

	// RequestDuration observes request latency by tier.
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "recsys_request_duration_seconds",
			Help:    "Request latency by tier",
			Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.05, 0.1},
		},
		[]string{"tier"},
	)

	// ErrorsTotal counts total errors by tier.
	ErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "recsys_errors_total",
			Help: "Total errors by tier",
		},
		[]string{"tier"},
	)

	// CircuitBreakerState tracks circuit breaker state (0=closed,1=open,2=half-open).
	CircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "recsys_circuit_breaker_state",
			Help: "Circuit breaker state (0=closed,1=open,2=half-open)",
		},
		[]string{"tier"},
	)
)

func init() {
	prometheus.MustRegister(RequestsTotal, RequestDuration, ErrorsTotal, CircuitBreakerState)
}
