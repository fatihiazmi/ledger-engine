package publisher

import (
	"context"
	"log"
)

// LogPublisher is a dev/debug publisher that logs events to stdout.
// Swap with Kafka/NATS publisher in production.
type LogPublisher struct{}

func NewLogPublisher() *LogPublisher {
	return &LogPublisher{}
}

func (p *LogPublisher) Publish(_ context.Context, eventType, aggregateID string, payload []byte) error {
	log.Printf("[EVENT] type=%s aggregate=%s payload=%s", eventType, aggregateID, string(payload))
	return nil
}
