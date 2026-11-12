// Command worker runs the RabbitMQ consumer process. It starts and
// deploys independently from the API server and shares the same
// internal/ packages (config, logging, database, rabbitmq) rather than
// duplicating any of that plumbing, per CLAUDE.md's modular-monolith
// guidance.
//
// Phase 21 added this worker's first real business consumer:
// wms.stock_adjustment.pending_reminder, published by the server's
// overdue-adjustment scanner (see cmd/server/main.go) and consumed here
// to persist a notification per eligible approver (internal/notification).
// Phase 22 adds a second: wms.report.export, generating the CSV file
// for an async export job (internal/report) and marking it COMPLETE/
// FAILED. It still also consumes the development wms.system.health
// queue, unchanged, proving the original consumer foundation continues
// to work alongside the two real ones.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"wms-backend/config"
	"wms-backend/internal/inventory"
	"wms-backend/internal/messaging/rabbitmq"
	"wms-backend/internal/notification"
	"wms-backend/internal/platform/database"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/platform/logging"
	"wms-backend/internal/report"
	"wms-backend/internal/stockadjustment"
	"wms-backend/internal/user"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString("configuration error: " + err.Error() + "\n")
		os.Exit(1)
	}

	logger := logging.New(cfg.AppEnv, "worker")
	slog.SetDefault(logger)

	// Phase 21's reminder consumer runs a real business mutation
	// (notification insert) through internal/platform/dbctx, the same
	// as the server — see that package's own doc.
	dbctx.SetDefaultTimeout(cfg.DBTransactionTimeout)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbManager := database.NewManager(cfg.DB, logger)
	if err := dbManager.Connect(ctx, config.DefaultStartupRetry); err != nil {
		logger.Error("database not reachable at startup, continuing in degraded mode", slog.String("error", err.Error()))
	}
	dbManager.StartReconnectLoop(ctx, 10*time.Second)

	mqManager := rabbitmq.NewManager(cfg.RabbitMQ, logger)
	if err := mqManager.Connect(ctx, config.DefaultStartupRetry); err != nil {
		logger.Error("rabbitmq not reachable at startup, continuing in degraded mode", slog.String("error", err.Error()))
	}
	mqManager.StartReconnectLoop(ctx, 10*time.Second)

	userRepo := user.NewRepository(dbManager.DB())
	notificationRepo := notification.NewRepository(dbManager.DB())
	notificationService := notification.NewService(notificationRepo, userRepo)

	inventoryService := inventory.NewService(inventory.NewRepository(dbManager.DB()))
	reportRepo := report.NewRepository(dbManager.DB())
	// publisher is nil here: ProcessJob (the only report.Service method
	// this worker ever calls) never publishes — only
	// CreateInventoryTransactionsJob does, and that only ever runs
	// server-side from the HTTP handler. A nil messagePublisher is safe
	// as long as that invariant holds, since Go never dereferences it
	// unless a method on it is actually called.
	reportService := report.NewService(reportRepo, inventoryService, nil, "", "", cfg.ExportDir)

	startConsumingWhenReady(ctx, mqManager, notificationService, reportService, logger)

	logger.Info("worker started")
	<-ctx.Done()
	logger.Info("shutdown signal received")

	// Give in-flight deliveries a moment to finish their handler before
	// the connection is torn down.
	time.Sleep(1 * time.Second)

	if err := dbManager.Close(); err != nil {
		logger.Error("database close error", slog.String("error", err.Error()))
	}
	if err := mqManager.Close(); err != nil {
		logger.Error("rabbitmq close error", slog.String("error", err.Error()))
	}

	logger.Info("worker stopped")
}

// startConsumingWhenReady declares topology (idempotent, harmless if the
// server already did it) and starts consuming the development
// wms.system.health queue, Phase 21's wms.stock_adjustment.pending_reminder
// queue, and Phase 22's wms.report.export queue as soon as RabbitMQ is
// reachable, retrying every 5s until it is.
func startConsumingWhenReady(ctx context.Context, mq *rabbitmq.Manager, notifications *notification.Service, reports *report.Service, logger *slog.Logger) {
	consumer := rabbitmq.NewConsumer(mq, logger)

	start := func() bool {
		ch, err := mq.OpenChannel()
		if err != nil {
			return false
		}
		if err := rabbitmq.DeclareTopology(ch); err != nil {
			_ = ch.Close()
			logger.Error("rabbitmq topology declare failed", slog.String("error", err.Error()))
			return false
		}
		_ = ch.Close()

		err = consumer.Consume(ctx, rabbitmq.SystemHealthQueue, 5, func(_ context.Context, d amqp.Delivery) error {
			var msg rabbitmq.DevHeartbeat
			if jsonErr := json.Unmarshal(d.Body, &msg); jsonErr != nil {
				logger.Warn("dev heartbeat: could not decode body", slog.String("error", jsonErr.Error()))
				return nil // malformed dev message — ack and move on, not a real failure
			}

			logger.Info("dev heartbeat received",
				slog.String("message_id", msg.MessageID),
				slog.String("event_type", msg.EventType),
				slog.String("correlation_id", msg.CorrelationID),
				slog.String("source", msg.Payload.Source),
			)
			return nil
		})
		if err != nil {
			logger.Error("failed to start consuming", slog.String("queue", rabbitmq.SystemHealthQueue), slog.String("error", err.Error()))
			return false
		}
		logger.Info("consuming queue", slog.String("queue", rabbitmq.SystemHealthQueue))

		err = consumer.Consume(ctx, rabbitmq.StockAdjustmentPendingReminderQueue, 5, stockAdjustmentReminderHandler(notifications, logger))
		if err != nil {
			logger.Error("failed to start consuming", slog.String("queue", rabbitmq.StockAdjustmentPendingReminderQueue), slog.String("error", err.Error()))
			return false
		}
		logger.Info("consuming queue", slog.String("queue", rabbitmq.StockAdjustmentPendingReminderQueue))

		err = consumer.Consume(ctx, rabbitmq.ReportExportQueue, 2, reportExportHandler(reports, logger))
		if err != nil {
			logger.Error("failed to start consuming", slog.String("queue", rabbitmq.ReportExportQueue), slog.String("error", err.Error()))
			return false
		}
		logger.Info("consuming queue", slog.String("queue", rabbitmq.ReportExportQueue))

		return true
	}

	if start() {
		return
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if start() {
					return
				}
			}
		}
	}()
}

// stockAdjustmentReminderHandler decodes one stock_adjustment.
// pending_reminder delivery and persists a notification per eligible
// approver via notification.Service — idempotently, by message_id (see
// NotifyStockAdjustmentReminder's own doc), so a RabbitMQ redelivery
// (connection blip before ack, or this worker restarting mid-delivery)
// never produces a duplicate notification for the same recipient.
// Returning nil ACKs (per rabbitmq.Handler's own contract, only once
// this function's DB writes have actually succeeded — never before);
// returning an error NACKs without requeue, the same no-DLQ shape this
// codebase's one other consumer already uses, so a permanently
// malformed message cannot loop forever.
func stockAdjustmentReminderHandler(notifications *notification.Service, logger *slog.Logger) rabbitmq.Handler {
	return func(ctx context.Context, d amqp.Delivery) error {
		var msg stockadjustment.ReminderMessage
		if err := json.Unmarshal(d.Body, &msg); err != nil {
			logger.Warn("stock adjustment reminder: could not decode body", slog.String("error", err.Error()))
			return nil // malformed message — ack and move on, retrying would never succeed
		}

		err := notifications.NotifyStockAdjustmentReminder(ctx, msg.MessageID, notification.StockAdjustmentReminderPayload{
			AdjustmentID: msg.Payload.AdjustmentID,
			AdjustmentNo: msg.Payload.AdjustmentNo,
			CreatedBy:    msg.Payload.CreatedBy,
			PendingSince: msg.Payload.PendingSince,
			AgeHours:     msg.Payload.AgeHours,
		})
		if err != nil {
			logger.Error("stock adjustment reminder: notify failed",
				slog.String("adjustment_no", msg.Payload.AdjustmentNo),
				slog.String("message_id", msg.MessageID),
				slog.String("error", err.Error()),
			)
			return err
		}

		logger.Info("stock adjustment reminder delivered",
			slog.String("adjustment_no", msg.Payload.AdjustmentNo),
			slog.String("message_id", msg.MessageID),
			slog.Int("age_hours", msg.Payload.AgeHours),
		)
		return nil
	}
}

// reportExportHandler decodes one report.export delivery and runs
// report.Service.ProcessJob — idempotently, via the job's own claim
// (see Repository.Claim's doc), so a RabbitMQ redelivery never
// regenerates a file for a job already completed or in flight.
// Returning nil ACKs only once ProcessJob has actually finished (either
// completed the job or durably recorded a failure via MarkFailed —
// both are "handled", so both ACK); returning an error (a failure that
// happened before either of those could even be recorded, e.g. the DB
// itself being unreachable) NACKs without requeue, the same no-DLQ
// shape this codebase's other consumers already use.
func reportExportHandler(reports *report.Service, logger *slog.Logger) rabbitmq.Handler {
	return func(ctx context.Context, d amqp.Delivery) error {
		var msg report.Message
		if err := json.Unmarshal(d.Body, &msg); err != nil {
			logger.Warn("report export: could not decode body", slog.String("error", err.Error()))
			return nil // malformed message — ack and move on, retrying would never succeed
		}

		if err := reports.ProcessJob(ctx, msg.Payload.JobID); err != nil {
			logger.Error("report export: processing failed",
				slog.Int64("job_id", msg.Payload.JobID),
				slog.String("message_id", msg.MessageID),
				slog.String("error", err.Error()),
			)
			return err
		}

		logger.Info("report export processed",
			slog.Int64("job_id", msg.Payload.JobID),
			slog.String("message_id", msg.MessageID),
		)
		return nil
	}
}
