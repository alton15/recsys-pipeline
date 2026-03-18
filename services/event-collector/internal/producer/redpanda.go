package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/recsys-pipeline/shared/event"
	"github.com/twmb/franz-go/pkg/kgo"
)

// RedpandaProducer publishes events to a Redpanda topic using franz-go.
type RedpandaProducer struct {
	client *kgo.Client
	topic  string
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
			// In production, this would feed into a dead-letter queue or metrics.
			fmt.Printf("produce error for user %s: %v\n", e.UserID, err)
		}
	})

	return nil
}

// Close flushes pending records and closes the client.
func (p *RedpandaProducer) Close() error {
	p.client.Close()
	return nil
}
