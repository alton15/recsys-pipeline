package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/recsys-pipeline/recommendation-api/internal/metrics"
)

func TestMetrics_RequestsTotal(t *testing.T) {
	counter, err := metrics.RequestsTotal.GetMetricWithLabelValues("tier1_precomputed")
	if err != nil {
		t.Fatalf("failed to get counter: %v", err)
	}

	counter.Inc()
	counter.Inc()

	var m dto.Metric
	if err := counter.Write(&m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}

	got := m.GetCounter().GetValue()
	if got != 2 {
		t.Errorf("expected counter value 2, got %f", got)
	}
}

func TestMetrics_RequestDuration(t *testing.T) {
	observer, err := metrics.RequestDuration.GetMetricWithLabelValues("tier1_precomputed")
	if err != nil {
		t.Fatalf("failed to get histogram: %v", err)
	}

	observer.Observe(0.005)
	observer.Observe(0.015)

	// Verify the histogram is registered and observable.
	ch := make(chan prometheus.Metric, 10)
	metrics.RequestDuration.Collect(ch)

	select {
	case metric := <-ch:
		var m dto.Metric
		if err := metric.Write(&m); err != nil {
			t.Fatalf("failed to write metric: %v", err)
		}
		hist := m.GetHistogram()
		if hist.GetSampleCount() != 2 {
			t.Errorf("expected 2 samples, got %d", hist.GetSampleCount())
		}
	default:
		t.Fatal("no metric collected from histogram")
	}
}
