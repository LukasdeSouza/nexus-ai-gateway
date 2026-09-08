// Package usage defines event contracts and emitters for asynchronous metering.
package usage

import (
	"context"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
)

// Producer abstracts publishing UsageEvents to an event stream (e.g. Kafka).
type Producer interface {
	Emit(ctx context.Context, event *domain.UsageEvent) error
	Close() error
}

// NoopProducer is a non-blocking in-memory/noop emitter used for local testing or before Kafka is wired.
type NoopProducer struct{}

// NewNoopProducer returns a no-op producer.
func NewNoopProducer() *NoopProducer {
	return &NoopProducer{}
}

// Emit discards or logs the event without blocking.
func (p *NoopProducer) Emit(_ context.Context, _ *domain.UsageEvent) error {
	return nil
}

// Close is a no-op.
func (p *NoopProducer) Close() error {
	return nil
}
