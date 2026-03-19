package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/recsys-pipeline/shared/event"
	"github.com/twmb/franz-go/pkg/kgo"
)

// RedpandaProducer publishes events to a Redpanda topic using franz-go.
type RedpandaProducer struct {
	client      *kgo.Client
	topic       string
	produceErrs atomic.Int64
}

// NewRedpanda creates a new RedpandaProducer connected to the given brokers.
func NewRedpanda(brokers []string, topic string) (*RedpandaProducer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.ProducerBatchMaxBytes(1_000_000),
		kgo.ProducerLinger(5*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("creating kafka client: %w", err)
	}

	return &RedpandaProducer{
		client: client,
		topic:  topic,
	}, nil
}

// Publish serializes the event to JSON and produces it asynchronously
// with the user_id as the partition key.
func (p *RedpandaProducer) Publish(e event.Event) error {
	value, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	record := &kgo.Record{
		Key:   []byte(e.UserID),
		Value: value,
	}

	p.client.Produce(context.Background(), record, func(_ *kgo.Record, err error) {
		if err != nil {
			count := p.produceErrs.Add(1)
			log.Printf("produce error for user %s (total_errors=%d): %v", e.UserID, count, err)
		}
	})

	return nil
}

// ProduceErrors returns the total number of async produce errors since startup.
func (p *RedpandaProducer) ProduceErrors() int64 {
	return p.produceErrs.Load()
}

// Close flushes pending records and closes the client.
func (p *RedpandaProducer) Close() error {
	p.client.Close()
	return nil
}
