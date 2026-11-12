package rabbitmq

import (
	"context"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher is the generic publish-side foundation. Business packages in
// later phases will wrap this with domain-specific message construction
// (see CLAUDE.md's recommended message envelope: message_id, event_type,
// created_at, correlation_id, user_id, payload) — none of that exists yet
// because no business event is produced in this phase.
type Publisher struct {
	manager *Manager
}

// NewPublisher builds a Publisher bound to a connection Manager.
func NewPublisher(manager *Manager) *Publisher {
	return &Publisher{manager: manager}
}

// Publish sends a persistent message to the given exchange/routing key.
// It opens and closes a short-lived channel per call, which is the safe
// default for the low, bursty publish volume expected here; a pooled
// channel can be introduced later if throughput requires it.
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, contentType string, body []byte) error {
	ch, err := p.manager.OpenChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return ch.PublishWithContext(publishCtx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  contentType,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
}
