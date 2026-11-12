package rabbitmq

import (
	"context"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Handler processes one delivery. Returning nil ACKs the message;
// returning an error NACKs it without requeue. Real retry/DLQ routing
// (per CLAUDE.md's retry-queue-with-TTL pattern) belongs to each business
// queue's own worker in a later phase — this foundation only guarantees
// "never ACK before the handler succeeds," which is the one rule that
// must hold from day one.
type Handler func(ctx context.Context, d amqp.Delivery) error

// Consumer is the generic consume-side foundation shared by every future
// worker consumer.
type Consumer struct {
	manager *Manager
	logger  *slog.Logger
}

// NewConsumer builds a Consumer bound to a connection Manager.
func NewConsumer(manager *Manager, logger *slog.Logger) *Consumer {
	return &Consumer{manager: manager, logger: logger}
}

// Consume opens a dedicated channel, sets a conservative prefetch so one
// slow message can't starve the others, and runs handler for every
// delivery until ctx is cancelled.
func (c *Consumer) Consume(ctx context.Context, queue string, prefetch int, handler Handler) error {
	ch, err := c.manager.OpenChannel()
	if err != nil {
		return err
	}

	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		return err
	}

	deliveries, err := ch.Consume(
		queue,
		"",    // consumer tag (auto-generated)
		false, // auto-ack — we ack manually after the handler succeeds
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		_ = ch.Close()
		return err
	}

	go func() {
		defer ch.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}

				if err := handler(ctx, d); err != nil {
					c.logger.Error("message handler failed",
						slog.String("queue", queue),
						slog.String("error", err.Error()),
					)
					_ = d.Nack(false, false)
					continue
				}

				_ = d.Ack(false)
			}
		}
	}()

	return nil
}
