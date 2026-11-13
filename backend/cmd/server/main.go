// Command server runs the WMS REST API.
//
//	@title			Smart WMS API
//	@version		1.0
//	@description	Warehouse Management System backend. Phase 11 adds Stock Adjustment (a controlled correction document that only ever mutates inventory at Approve time, via AddStock/RemoveStock) and Stock Count (freezes an expected quantity from live inventory, then applies the counted-vs-expected variance through the same Inventory Core primitives on Confirm, with a stale-balance conflict check) — on top of Phase 10's Shipping, Phase 9's Picking, Phase 8's transaction-context hardening, Phase 7's Putaway, Phase 6's Inbound Receiving, Phase 5's Inventory Core, Phase 4's Product Master, Phase 3's Warehouse Setup, Phase 2's authentication/RBAC, and Phase 1's infrastructure foundation.
//	@BasePath		/api/v1
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and the JWT. Not yet required by any endpoint in this phase — reserved for the auth module.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"wms-backend/config"
	_ "wms-backend/docs"
	"wms-backend/internal/auth"
	"wms-backend/internal/dashboard"
	"wms-backend/internal/health"
	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/messaging/rabbitmq"
	"wms-backend/internal/notification"
	"wms-backend/internal/order"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/database"
	"wms-backend/internal/platform/dbctx"
	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/logging"
	"wms-backend/internal/platform/middleware"
	"wms-backend/internal/platformadmin"
	"wms-backend/internal/product"
	"wms-backend/internal/putaway"
	"wms-backend/internal/receiving"
	"wms-backend/internal/replenishment"
	"wms-backend/internal/report"
	"wms-backend/internal/role"
	"wms-backend/internal/shipping"
	"wms-backend/internal/stockadjustment"
	"wms-backend/internal/stockcount"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Configuration is invalid before a logger even exists to log
		// through, so this is the one place a plain stderr write is
		// correct instead of structured logging.
		os.Stderr.WriteString("configuration error: " + err.Error() + "\n")
		os.Exit(1)
	}

	logger := logging.New(cfg.AppEnv, "server")
	slog.SetDefault(logger)

	// Phase 8: the process-wide bound every durable business mutation's
	// detached transaction uses (see internal/platform/dbctx) — set once
	// here, the same way slog.SetDefault configures the logger above.
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
	declareTopologyWhenReady(ctx, mqManager, logger, func() {
		startDevHeartbeat(ctx, mqManager, logger)
	})

	// Passed into NewEngine so it can apply them before creating the
	// /api/v1 group — see that function's doc for why the ordering
	// matters (a real, previously-undetected bug: middleware added via
	// a bare engine.Use() call here, after the group already existed,
	// never actually ran for any /api/v1/* route).
	engine, v1 := apihttp.NewEngine(
		middleware.Recovery(logger),
		middleware.RequestID(),
		middleware.RequestLogging(logger),
		middleware.CORS(os.Getenv("CORS_ALLOWED_ORIGINS")),
	)

	healthHandler := health.NewHandler(health.NewChecker(dbManager, mqManager))
	healthHandler.Register(v1)

	// warehouseRepo is constructed here, ahead of its own handler
	// registration below, because both role.NewService (Phase 27's
	// Warehouse Access) and user.NewService (Phase 26) also need it — the
	// same narrow FindByID/FindByIDs lookups Warehouse Setup itself uses.
	warehouseRepo := warehouse.NewRepository(dbManager.DB())

	roleRepo := role.NewRepository(dbManager.DB())
	roleService := role.NewService(roleRepo, warehouseRepo)
	role.NewHandler(roleService).Register(v1, cfg.JWTSecret)

	userRepo := user.NewRepository(dbManager.DB())
	userService := user.NewService(userRepo, roleRepo, warehouseRepo)
	user.NewHandler(userService).Register(v1, cfg.JWTSecret)

	// tenantService resolves and re-verifies active tenant membership
	// (Phase 29B) — used by auth for login/switch-tenant/me, and
	// available for RequireTenantContext (internal/platform/middleware)
	// once a route owner wires that middleware in ahead of its own
	// handlers, the same "built, tested, not yet mounted anywhere" shape
	// RequireJWT itself started in during Phase 1. tenantRepo is passed
	// to NewService twice — once per capability interface
	// (membershipRepository, managementRepository) — because WMS-29's
	// Platform Admin tenant-management API (registered below) reuses
	// the same repository rather than duplicating it.
	tenantRepo := tenant.NewRepository(dbManager.DB())
	tenantService := tenant.NewService(tenantRepo, tenantRepo)

	authService := auth.NewService(userRepo, roleRepo, tenantService, cfg.JWTSecret, cfg.JWTExpiresIn)
	auth.NewHandler(authService).Register(v1, cfg.JWTSecret)

	// Platform Admin tenant-management API (WMS-29): deliberately
	// authorized on platform_admins alone (middleware.RequirePlatformAdmin),
	// never on tenant-level RBAC — see that middleware's own doc for why,
	// and docs/proposals/wms-saas/tenant-isolation-security.md §2-3 for
	// the design this follows ahead of a full separate
	// PLATFORM_ADMIN session_type login flow (saas-module-roadmap.md's
	// Phase 33, not yet built).
	platformAdminRepo := platformadmin.NewRepository(dbManager.DB())
	tenant.NewHandler(tenantService).Register(v1, cfg.JWTSecret, platformAdminRepo, logger)

	// Warehouse -> Zone -> Location: each service takes a narrow
	// interface satisfied by the next package's Repository, so the
	// dependency only ever points one way (warehouse <- zone <-
	// location) and none of the three import each other in a cycle.
	zoneRepo := zone.NewRepository(dbManager.DB())
	locationRepo := location.NewRepository(dbManager.DB())

	warehouseService := warehouse.NewService(warehouseRepo, zoneRepo)
	warehouse.NewHandler(warehouseService).Register(v1, cfg.JWTSecret)

	zoneService := zone.NewService(zoneRepo, warehouseRepo, locationRepo)
	zone.NewHandler(zoneService).Register(v1, cfg.JWTSecret)

	locationService := location.NewService(locationRepo, zoneRepo)
	location.NewHandler(locationService).Register(v1, cfg.JWTSecret)

	productRepo := product.NewRepository(dbManager.DB())
	productService := product.NewService(productRepo)
	product.NewHandler(productService).Register(v1, cfg.JWTSecret)

	inventoryRepo := inventory.NewRepository(dbManager.DB())
	inventoryService := inventory.NewService(inventoryRepo)
	inventory.NewHandler(inventoryService).Register(v1, cfg.JWTSecret)

	receivingRepo := receiving.NewRepository(dbManager.DB())
	receivingService := receiving.NewService(receivingRepo, productRepo, warehouseRepo, locationRepo, inventoryService)
	receiving.NewHandler(receivingService).Register(v1, cfg.JWTSecret)

	putawayRepo := putaway.NewRepository(dbManager.DB())
	putawayService := putaway.NewService(putawayRepo, productRepo, warehouseRepo, receivingService, inventoryService)
	putaway.NewHandler(putawayService).Register(v1, cfg.JWTSecret)

	pickingRepo := picking.NewRepository(dbManager.DB())
	pickingService := picking.NewService(pickingRepo, productRepo, warehouseRepo, inventoryService)
	picking.NewHandler(pickingService).Register(v1, cfg.JWTSecret)

	shippingRepo := shipping.NewRepository(dbManager.DB())
	shippingService := shipping.NewService(shippingRepo, pickingService)
	shipping.NewHandler(shippingService).Register(v1, cfg.JWTSecret)

	stockAdjustmentRepo := stockadjustment.NewRepository(dbManager.DB())
	stockAdjustmentService := stockadjustment.NewService(stockAdjustmentRepo, productRepo, locationRepo, inventoryService, cfg.StockAdjustmentPendingWarningHours)
	stockadjustment.NewHandler(stockAdjustmentService).Register(v1, cfg.JWTSecret)

	stockCountRepo := stockcount.NewRepository(dbManager.DB())
	stockCountService := stockcount.NewService(stockCountRepo, warehouseRepo, zoneRepo, productRepo, locationRepo, inventoryService, stockAdjustmentService)
	stockcount.NewHandler(stockCountService).Register(v1, cfg.JWTSecret)

	notificationRepo := notification.NewRepository(dbManager.DB())
	notificationService := notification.NewService(notificationRepo, userRepo)
	notification.NewHandler(notificationService).Register(v1, cfg.JWTSecret)

	startStockAdjustmentReminderScanner(ctx, mqManager, stockAdjustmentService,
		time.Duration(cfg.StockAdjustmentReminderIntervalMinutes)*time.Minute, logger)

	reportPublisher := rabbitmq.NewPublisher(mqManager)
	reportRepo := report.NewRepository(dbManager.DB())
	reportService := report.NewService(reportRepo, inventoryService, reportPublisher,
		rabbitmq.EventsExchange, rabbitmq.ReportExportRoutingKey, cfg.ExportDir)
	report.NewHandler(reportService).Register(v1, cfg.JWTSecret)

	dashboardService := dashboard.NewService(receivingService, pickingService, inventoryService)
	dashboard.NewHandler(dashboardService).Register(v1, cfg.JWTSecret)

	replenishmentRepo := replenishment.NewRepository(dbManager.DB())
	replenishmentService := replenishment.NewService(replenishmentRepo, productRepo, warehouseRepo, inventoryService)
	replenishment.NewHandler(replenishmentService).Register(v1, cfg.JWTSecret)

	orderRepo := order.NewRepository(dbManager.DB())
	orderService := order.NewService(orderRepo, productRepo, warehouseRepo, pickingService, shippingService)
	order.NewHandler(orderService).Register(v1, cfg.JWTSecret)

	if cfg.SwaggerEnabled {
		engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
		logger.Info("swagger ui enabled", slog.String("url", "/swagger/index.html"))
	}

	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server starting", slog.String("port", cfg.AppPort), slog.String("env", cfg.AppEnv))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", slog.String("error", err.Error()))
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	// A handler mid-mutation may be running on a dbctx.Detach'd
	// transaction context that is deliberately immune to ctx above being
	// cancelled (that's the whole point — see internal/platform/dbctx).
	// srv.Shutdown stops accepting new requests immediately but waits
	// for in-flight handlers to return; giving it less time than the
	// transaction bound itself would mean dbManager.Close() below can
	// yank the connection pool out from under a transaction that was
	// still running legitimately, rather than letting it reach a real
	// commit/rollback on its own. A small buffer on top covers response
	// serialization after the transaction itself completes.
	shutdownTimeout := cfg.DBTransactionTimeout + 5*time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", slog.String("error", err.Error()))
	}

	if err := dbManager.Close(); err != nil {
		logger.Error("database close error", slog.String("error", err.Error()))
	}

	if err := mqManager.Close(); err != nil {
		logger.Error("rabbitmq close error", slog.String("error", err.Error()))
	}

	logger.Info("server stopped")
}

// declareTopologyWhenReady declares the exchange and the development
// health queue as soon as RabbitMQ is reachable, whether that happens
// immediately or only after the reconnect loop catches up. Declaring
// topology is idempotent, so a single successful declare is enough — this
// does not need to re-run on every reconnect. onReady runs once,
// immediately after the first successful declare.
func declareTopologyWhenReady(ctx context.Context, mq *rabbitmq.Manager, logger *slog.Logger, onReady func()) {
	declare := func() bool {
		ch, err := mq.OpenChannel()
		if err != nil {
			return false
		}
		defer ch.Close()

		if err := rabbitmq.DeclareTopology(ch); err != nil {
			logger.Error("rabbitmq topology declare failed", slog.String("error", err.Error()))
			return false
		}

		logger.Info("rabbitmq topology declared",
			slog.String("exchange", rabbitmq.EventsExchange),
			slog.String("dev_queue", rabbitmq.SystemHealthQueue),
		)
		return true
	}

	if declare() {
		onReady()
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
				if declare() {
					onReady()
					return
				}
			}
		}
	}()
}

// startDevHeartbeat is a DEVELOPMENT / INFRASTRUCTURE-ONLY publisher: it
// periodically publishes a heartbeat message so the worker's consumer
// foundation has something to receive, proving the full publish →
// exchange → queue → consume pipeline works end to end before any real
// business event exists. Remove this once a real queue takes over
// verifying the pipeline.
func startDevHeartbeat(ctx context.Context, mq *rabbitmq.Manager, logger *slog.Logger) {
	publisher := rabbitmq.NewPublisher(mq)

	publish := func() {
		body, err := rabbitmq.NewDevHeartbeat("server").Marshal()
		if err != nil {
			logger.Error("dev heartbeat: marshal failed", slog.String("error", err.Error()))
			return
		}

		if err := publisher.Publish(ctx, rabbitmq.EventsExchange, rabbitmq.SystemHealthRoutingKey, "application/json", body); err != nil {
			logger.Warn("dev heartbeat: publish failed", slog.String("error", err.Error()))
		}
	}

	go func() {
		publish()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				publish()
			}
		}
	}()
}

// startStockAdjustmentReminderScanner is Phase 21's server-owned
// periodic producer: every interval, it asks stockAdjustmentService for
// every overdue PENDING adjustment that has cleared its reminder
// cooldown (ClaimAndListOverdueForReminder — the claim itself is what
// guarantees this scanner never re-publishes for the same adjustment
// every single tick, see that method's own doc) and publishes one
// ReminderMessage per candidate to
// rabbitmq.StockAdjustmentPendingReminderQueue. A publish failure is
// logged and the loop continues — the next scan (or the adjustment's
// next cooldown-eligible cycle) will retry, the same tolerant-of-
// transient-RabbitMQ-unavailability shape startDevHeartbeat's own
// publish loop already uses.
func startStockAdjustmentReminderScanner(ctx context.Context, mq *rabbitmq.Manager, svc *stockadjustment.Service, interval time.Duration, logger *slog.Logger) {
	publisher := rabbitmq.NewPublisher(mq)

	scan := func() {
		candidates, err := svc.ClaimAndListOverdueForReminder(ctx)
		if err != nil {
			logger.Error("stock adjustment reminder scan failed", slog.String("error", err.Error()))
			return
		}

		for _, candidate := range candidates {
			body, err := stockadjustment.NewReminderMessage(candidate).Marshal()
			if err != nil {
				logger.Error("stock adjustment reminder: marshal failed",
					slog.String("adjustment_no", candidate.AdjustmentNo), slog.String("error", err.Error()))
				continue
			}

			if err := publisher.Publish(ctx, rabbitmq.EventsExchange, rabbitmq.StockAdjustmentPendingReminderRoutingKey, "application/json", body); err != nil {
				logger.Warn("stock adjustment reminder: publish failed",
					slog.String("adjustment_no", candidate.AdjustmentNo), slog.String("error", err.Error()))
				continue
			}

			logger.Info("stock adjustment reminder published",
				slog.String("adjustment_no", candidate.AdjustmentNo), slog.Int("age_hours", candidate.AgeHours))
		}
	}

	go func() {
		scan()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				scan()
			}
		}
	}()
}
