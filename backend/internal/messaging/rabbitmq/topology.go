package rabbitmq

import amqp "github.com/rabbitmq/amqp091-go"

// Topology names. wms.events is the single topic exchange every future
// business queue will bind to (see the integration analysis' RabbitMQ
// architecture section) — routing keys will mirror queue names.
const (
	EventsExchange = "wms.events"

	// SystemHealthQueue is a DEVELOPMENT / INFRASTRUCTURE-ONLY queue.
	// It exists solely so this phase can prove the publisher, consumer
	// and topology foundations work end-to-end (server publishes a
	// heartbeat, worker consumes it) before any real business queue
	// exists. It is not one of the five business queues from the
	// integration analysis and should be removed once a real queue
	// takes over verifying the pipeline.
	SystemHealthQueue      = "wms.system.health"
	SystemHealthRoutingKey = "system.health"

	// StockAdjustmentPendingReminderQueue is Phase 21's first real
	// business queue: the server's reminder scanner (cmd/server/main.go)
	// publishes here for every overdue PENDING stock adjustment that
	// clears its cooldown (see internal/stockadjustment's
	// ClaimAndListOverdueForReminder); the worker consumes it and
	// persists a notification per eligible approver (internal/notification).
	StockAdjustmentPendingReminderQueue      = "wms.stock_adjustment.pending_reminder"
	StockAdjustmentPendingReminderRoutingKey = "stock_adjustment.pending_reminder"

	// ReportExportQueue is Phase 22's async export-job queue —
	// internal/report's handler publishes here after durably creating a
	// PENDING export_jobs row; the worker consumes it, generates the
	// file, and marks the job COMPLETED/FAILED. One queue serves every
	// async report type this phase (today, just
	// INVENTORY_TRANSACTIONS) or a future phase adds — job_type lives on
	// the export_jobs row itself, not the routing key, so a new async
	// report never needs a new queue.
	ReportExportQueue      = "wms.report.export"
	ReportExportRoutingKey = "report.export"
)

// DeclareTopology declares the exchange, the development health queue,
// Phase 21's stock-adjustment-reminder queue, and Phase 22's report-
// export queue. Both server and worker call this on startup so either
// one can be first without a declare-order dependency — AMQP declare is
// idempotent.
func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(
		EventsExchange,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		return err
	}

	if err := declareAndBind(ch, SystemHealthQueue, SystemHealthRoutingKey); err != nil {
		return err
	}
	if err := declareAndBind(ch, StockAdjustmentPendingReminderQueue, StockAdjustmentPendingReminderRoutingKey); err != nil {
		return err
	}
	return declareAndBind(ch, ReportExportQueue, ReportExportRoutingKey)
}

// declareAndBind declares one durable queue and binds it to
// EventsExchange under routingKey — the shared shape every queue this
// topology owns uses.
func declareAndBind(ch *amqp.Channel, queue, routingKey string) error {
	q, err := ch.QueueDeclare(
		queue,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		return err
	}

	return ch.QueueBind(
		q.Name,
		routingKey,
		EventsExchange,
		false,
		nil,
	)
}
