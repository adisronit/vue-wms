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
	"wms-backend/internal/brand"
	"wms-backend/internal/customer"
	"wms-backend/internal/customerdeliverylocation"
	"wms-backend/internal/dashboard"
	"wms-backend/internal/docrender"
	"wms-backend/internal/health"
	"wms-backend/internal/inbounddoc"
	"wms-backend/internal/inventory"
	"wms-backend/internal/inventoryreport"
	"wms-backend/internal/loading"
	"wms-backend/internal/location"
	"wms-backend/internal/messaging/rabbitmq"
	"wms-backend/internal/notification"
	"wms-backend/internal/order"
	"wms-backend/internal/packing"
	"wms-backend/internal/picking"
	"wms-backend/internal/pickingbatch"
	"wms-backend/internal/pickingkpi"
	"wms-backend/internal/pickingreport"
	"wms-backend/internal/platform/database"
	"wms-backend/internal/platform/dbctx"
	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/logging"
	"wms-backend/internal/platform/middleware"
	"wms-backend/internal/platformadmin"
	"wms-backend/internal/product"
	"wms-backend/internal/productcategory"
	"wms-backend/internal/productfamily"
	"wms-backend/internal/putaway"
	"wms-backend/internal/qcreport"
	"wms-backend/internal/qualitycheck"
	"wms-backend/internal/receiving"
	"wms-backend/internal/receivingreport"
	"wms-backend/internal/relocation"
	"wms-backend/internal/replenishment"
	"wms-backend/internal/report"
	"wms-backend/internal/role"
	"wms-backend/internal/shipping"
	"wms-backend/internal/stockadjustment"
	"wms-backend/internal/stockcount"
	"wms-backend/internal/stockcountreport"
	"wms-backend/internal/supplier"
	"wms-backend/internal/tenant"
	"wms-backend/internal/transferreceipt"
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

	// tenantService resolves and re-verifies active tenant membership
	// (Phase 29B) — used by auth for login/switch-tenant/me, and by
	// RequireTenantContext below. Constructed ahead of roleRepo/role's own
	// handler registration (moved up from its original position after
	// user/auth wiring) because role.Handler.Register now needs it as its
	// own membershipChecker — WMS-29E was the first route owner to wire
	// RequireTenantContext in ahead of its own handlers; roles joined it
	// once roles.tenant_id made GET /roles's IDOR exploitable. tenantRepo
	// is passed to NewService twice — once per capability interface
	// (membershipRepository, managementRepository) — because WMS-29's
	// Platform Admin tenant-management API (registered below) reuses
	// the same repository rather than duplicating it.
	tenantRepo := tenant.NewRepository(dbManager.DB())
	tenantService := tenant.NewService(tenantRepo, tenantRepo)

	roleRepo := role.NewRepository(dbManager.DB())
	roleService := role.NewService(roleRepo, warehouseRepo)
	role.NewHandler(roleService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	userRepo := user.NewRepository(dbManager.DB())
	userService := user.NewService(userRepo, roleRepo, warehouseRepo)
	user.NewHandler(userService).Register(v1, cfg.JWTSecret)

	// Phase 33C.3 -- Document History / Reprint. docrenderService is
	// constructed early (same section as tenantRepo/userRepo above)
	// because every PDF-rendering module below (order/picking/
	// pickingbatch/packing) is wired with it via SetDocRender right
	// after each of their own PDFService constructions -- see each
	// one's own comment below for why that is additive, optional wiring
	// rather than a constructor-signature change.
	docrenderRepo := docrender.NewRepository(dbManager.DB())
	docrenderService := docrender.NewService(docrenderRepo, userRepo)
	docrender.NewHandler(docrenderService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// membershipService (WMS-29E) implements Tenant User & Membership
	// Management's business rules, shared by both the Platform Admin and
	// Tenant Admin handlers below — see MembershipService's own doc for
	// why one Service type serves both personas.
	membershipService := tenant.NewMembershipService(tenantRepo, userRepo, roleRepo, tenantRepo)

	authService := auth.NewService(userRepo, roleRepo, tenantService, cfg.JWTSecret, cfg.JWTExpiresIn)
	auth.NewHandler(authService).Register(v1, cfg.JWTSecret)

	// Platform Admin tenant-management + tenant-membership-management API
	// (WMS-29, WMS-29E): deliberately authorized on platform_admins alone
	// (middleware.RequirePlatformAdmin), never on tenant-level RBAC — see
	// that middleware's own doc for why, and docs/proposals/wms-saas/
	// tenant-isolation-security.md §2-3 for the design this follows ahead
	// of a full separate PLATFORM_ADMIN session_type login flow
	// (saas-module-roadmap.md's Phase 33, not yet built).
	platformAdminRepo := platformadmin.NewRepository(dbManager.DB())
	tenant.NewHandler(tenantService, membershipService).Register(v1, cfg.JWTSecret, platformAdminRepo, logger)

	// Tenant Admin membership-management API (WMS-29E): the
	// RequireJWT -> RequireTenantContext -> RequirePermission chain,
	// scoped to the caller's own re-verified active tenant only — see
	// TenantAdminHandler's own doc for why this is a separate route
	// prefix/type from the Platform Admin handler above rather than the
	// same routes with a looser check.
	tenant.NewTenantAdminHandler(membershipService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 29D: the one capability check the frontend needs to decide
	// whether to show Platform Administration navigation at all — see
	// that handler's own doc for why it cannot just reuse the route
	// group above.
	platformadmin.NewHandler(platformAdminRepo).Register(v1, cfg.JWTSecret)

	// Warehouse -> Zone -> Location: each service takes a narrow
	// interface satisfied by the next package's Repository, so the
	// dependency only ever points one way (warehouse <- zone <-
	// location) and none of the three import each other in a cycle.
	zoneRepo := zone.NewRepository(dbManager.DB())
	locationRepo := location.NewRepository(dbManager.DB())
	// inventoryRepo is constructed here, ahead of its own service/handler
	// registration below, because Location Master Management's
	// locationService needs it (CountByLocation) for the stock-
	// referential-safety check on Delete — the same "construct early for
	// a later dependency" shape warehouseRepo/productCategoryRepo above
	// already use.
	inventoryRepo := inventory.NewRepository(dbManager.DB())

	warehouseService := warehouse.NewService(warehouseRepo, zoneRepo)
	warehouse.NewHandler(warehouseService).Register(v1, cfg.JWTSecret)

	zoneService := zone.NewService(zoneRepo, warehouseRepo, locationRepo)
	zone.NewHandler(zoneService).Register(v1, cfg.JWTSecret)

	// Location Master Management: Register now uses the RequireJWT ->
	// RequireTenantContext -> RequirePermission chain (same
	// tenantService/cfg.TenantMembershipCacheTTL as the Tenant Admin
	// membership API and Master Data Expansion's new modules above) —
	// see internal/location/handler.go's doc for why this package is
	// now held to that stricter standard while warehouse/zone's own
	// routes above are deliberately left unchanged.
	locationService := location.NewService(locationRepo, zoneRepo, inventoryRepo)
	location.NewHandler(locationService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Master Data Expansion: Product Category, Customer, Supplier, plus
	// (EWTA Product Master enhancement) Brand and Product Family.
	// supplierRepo/brandRepo/familyRepo are constructed ahead of their
	// own handler registration below because productService.Create/
	// Update now need them (alongside productCategoryRepo) as checkers
	// to validate optional default_supplier_id/brand_id/family_id — see
	// internal/product/service.go's own checker interface docs. Each
	// handler's Register uses the RequireJWT -> RequireTenantContext ->
	// RequirePermission chain (membershipChecker/cacheTTL are the same
	// tenantService/cfg.TenantMembershipCacheTTL the Tenant Admin
	// membership API above already uses) — product.Handler.Register now
	// uses this chain too (EWTA closes its own pre-existing,
	// RequireJWT-only gap; see internal/product/repository.go's doc).
	productCategoryRepo := productcategory.NewRepository(dbManager.DB())
	supplierRepo := supplier.NewRepository(dbManager.DB())
	brandRepo := brand.NewRepository(dbManager.DB())
	familyRepo := productfamily.NewRepository(dbManager.DB())

	productRepo := product.NewRepository(dbManager.DB())
	productService := product.NewService(
		productRepo, productCategoryRepo, brandRepo, familyRepo, supplierRepo, warehouseRepo, zoneRepo,
		cfg.ImageStorageDir, cfg.ImageMaxUploadBytes,
	)
	product.NewHandler(productService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	productCategoryService := productcategory.NewService(productCategoryRepo, productRepo)
	productcategory.NewHandler(productCategoryService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	customerRepo := customer.NewRepository(dbManager.DB())
	customerDeliveryLocationRepo := customerdeliverylocation.NewRepository(dbManager.DB())
	customerService := customer.NewService(customerRepo, customerDeliveryLocationRepo)
	customer.NewHandler(customerService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	customerDeliveryLocationService := customerdeliverylocation.NewService(customerDeliveryLocationRepo, customerRepo)
	customerdeliverylocation.NewHandler(customerDeliveryLocationService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	supplierService := supplier.NewService(supplierRepo)
	supplier.NewHandler(supplierService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	brandService := brand.NewService(brandRepo)
	brand.NewHandler(brandService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	familyService := productfamily.NewService(familyRepo)
	productfamily.NewHandler(familyService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 30 Batch 1: inventoryService now threads a verified tenantID
	// through every query (List/FindByID/Summary/Transactions/
	// AdjustBalance/AdjustReservation/...), and verifies productRepo/
	// locationRepo ownership before any mutation — see
	// internal/inventory/service.go's own package doc for the full
	// rationale. Its own Register now uses the RequireJWT ->
	// RequireTenantContext -> RequirePermission chain, same as
	// location/customer/supplier above.
	inventoryService := inventory.NewService(inventoryRepo, productRepo, locationRepo)
	inventory.NewHandler(inventoryService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 31A: receivingService is now itself tenant-scoped, so it uses
	// productRepo/warehouseRepo/locationRepo's real tenant-scoped lookups
	// (FindByIDForTenant/FindByIDForTenant/FindByID) directly, not
	// locationUnscoped — see internal/receiving/service.go's own doc for
	// why Receiving was the last inbound/outbound module on the old
	// existence-only checks.
	receivingRepo := receiving.NewRepository(dbManager.DB())
	receivingService := receiving.NewService(receivingRepo, productRepo, warehouseRepo, locationRepo, supplierRepo, inventoryService)
	receiving.NewHandler(receivingService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 31B: qualitycheckService is tenant-scoped from day one (see
	// its own package doc) — it reads receipts/receipt_item_splits
	// directly via raw SQL rather than depending on receivingService's
	// Go API, so it has no constructor dependency on it.
	qualitycheckRepo := qualitycheck.NewRepository(dbManager.DB())
	qualitycheckService := qualitycheck.NewService(qualitycheckRepo, warehouseRepo, inventoryService)
	qualitycheck.NewHandler(qualitycheckService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 31C: putawayService is now itself tenant-scoped (the last
	// inbound/outbound module on the old chain), so it uses productRepo/
	// warehouseRepo's real tenant-scoped FindByIDForTenant lookups and
	// receivingService's own verified tenantID — see
	// internal/putaway/service.go's own doc.
	putawayRepo := putaway.NewRepository(dbManager.DB())
	putawayService := putaway.NewService(putawayRepo, productRepo, warehouseRepo, receivingService, inventoryService)
	putaway.NewHandler(putawayService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 32: relocation is a standalone internal transfer workflow —
	// architecturally a clone of putaway above (same tenant-scoped,
	// hold-aware MoveAvailableStockTx usage) but with no dependency on
	// receivingService, since it moves stock already in the warehouse
	// rather than stock a receipt just brought in. Its own PDF
	// (RenderTransferNote) is composed locally in internal/relocation,
	// not folded into inbounddoc — a Relocation Transfer Note is not an
	// inbound document.
	relocationRepo := relocation.NewRepository(dbManager.DB())
	relocationService := relocation.NewService(relocationRepo, productRepo, warehouseRepo, inventoryService)
	relocationPDFService := relocation.NewPDFService(relocationService, inventoryService, locationRepo, tenantRepo, userRepo)
	relocation.NewHandler(relocationService, relocationPDFService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 31D: inbounddoc composes the three Services above (plus
	// inventoryService for its own Final Stock Position section and
	// tenantRepo/userRepo for the PDF header/footer) into read-only PDF
	// generation — see its own package doc for why GRN/Completion/
	// Discrepancy/QC Report/Putaway Task Sheet all live in one new
	// package rather than split across receiving/qualitycheck/putaway's
	// own handlers (receiving cannot import qualitycheck or putaway
	// without a cycle).
	inbounddocService := inbounddoc.NewService(receivingService, qualitycheckService, putawayService, inventoryService, locationRepo, tenantRepo, userRepo)
	inbounddoc.NewHandler(inbounddocService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 34B: receivingreport reads receipts/receipt_items/
	// receipt_item_splits/qc_inspection_items/qc_dispositions/
	// putaway_task_items directly via raw SQL, the same "no Go
	// dependency on the owning write-side package" shape qualitycheck
	// established above — see its own package doc. Gated on the Phase
	// 34A report module, not receiving.*/qualitycheck.*/putaway.*.
	receivingReportRepo := receivingreport.NewRepository(dbManager.DB())
	receivingReportService := receivingreport.NewService(receivingReportRepo, tenantRepo, userRepo)
	receivingreport.NewHandler(receivingReportService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 34C: stockcountreport mirrors receivingreport's own shape —
	// raw SQL against stock_counts/stock_count_items/stock_adjustments,
	// no Go dependency on internal/stockcount/internal/stockadjustment.
	stockCountReportRepo := stockcountreport.NewRepository(dbManager.DB())
	stockCountReportService := stockcountreport.NewService(stockCountReportRepo, tenantRepo, userRepo)
	stockcountreport.NewHandler(stockCountReportService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	pickingRepo := picking.NewRepository(dbManager.DB())
	pickingService := picking.NewService(pickingRepo, productRepo, warehouseRepo, inventoryService)
	pickingService.SetUsers(userRepo)
	pickingPDFService := picking.NewPDFService(pickingService, tenantRepo, userRepo)
	pickingPDFService.SetDocRender(docrenderService)
	picking.NewHandler(pickingService, pickingPDFService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 34D: pickingreport mirrors receivingreport/stockcountreport's
	// own shape — raw SQL against picking_tasks/picking_task_items/
	// picking_batches/picking_batch_allocations/shipment_items, no Go
	// dependency on internal/picking/internal/pickingbatch/internal/
	// shipping.
	pickingReportRepo := pickingreport.NewRepository(dbManager.DB())
	pickingReportService := pickingreport.NewService(pickingReportRepo, tenantRepo, userRepo)
	pickingreport.NewHandler(pickingReportService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 34E: inventoryreport mirrors every other Phase 34 report
	// package's own shape — raw SQL against inventory/
	// inventory_transactions/locations/zones/warehouses/products, no Go
	// dependency on internal/inventory.
	inventoryReportRepo := inventoryreport.NewRepository(dbManager.DB())
	inventoryReportService := inventoryreport.NewService(inventoryReportRepo, tenantRepo, userRepo)
	inventoryreport.NewHandler(inventoryReportService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 34F: pickingkpi is a KPI dashboard, not a detail-row report
	// — it computes real aggregates (never fabricated) from the same
	// picking_tasks/picking_task_items/picking_scan_log/picking_batches
	// tables pickingreport (34D) already reads, via its own raw SQL, no
	// Go dependency on internal/picking or internal/pickingreport.
	pickingKPIRepo := pickingkpi.NewRepository(dbManager.DB())
	pickingKPIService := pickingkpi.NewService(pickingKPIRepo, tenantRepo, userRepo)
	pickingkpi.NewHandler(pickingKPIService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 34G: qcreport mirrors receivingreport's own shape — reads
	// qc_inspections/qc_inspection_items/qc_dispositions/receipts/
	// receipt_item_splits/inventory directly via raw SQL, no Go
	// dependency on internal/qualitycheck.
	qcReportRepo := qcreport.NewRepository(dbManager.DB())
	qcReportService := qcreport.NewService(qcReportRepo, tenantRepo, userRepo)
	qcreport.NewHandler(qcReportService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 33C — Packing: a pure physical-handling/traceability layer
	// over already-picked stock, sitting between Picking and Shipping.
	// Deliberately has NO dependency on inventoryService at all (unlike
	// every module above) — it never mutates a stock balance; the
	// pickingService dependency is read-only (pickingChecker.Get) for
	// copying an already-terminal picking task's already-picked lines
	// verbatim. Does not gate shippingService.Create — that DB-level
	// pack/load dispatch gate is explicitly deferred to Phase 33D.
	packingRepo := packing.NewRepository(dbManager.DB())
	packingService := packing.NewService(packingRepo, pickingService)
	packingService.SetUsers(userRepo)
	packingPDFService := packing.NewPDFService(packingService, tenantRepo, userRepo)
	packingPDFService.SetProductInfo(productRepo, brandRepo)
	packingPDFService.SetDocRender(docrenderService)
	packing.NewHandler(packingService, packingPDFService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 33D.1 — Shipping re-keyed off packing_job_id instead of
	// picking_task_id: Create's own pack-validation gate (the linked
	// packing job must already be COMPLETED, i.e. checker-confirmed) now
	// goes through packingService's own already-correct, order-scoped
	// abstraction rather than pickingService directly — internal/shipping
	// no longer depends on internal/picking at all.
	shippingRepo := shipping.NewRepository(dbManager.DB())
	shippingService := shipping.NewService(shippingRepo, packingService)
	// Phase 33D.3 -- Delivery Note.
	shippingPDFService := shipping.NewPDFService(shippingService, packingService, tenantRepo, userRepo)
	shippingPDFService.SetDocRender(docrenderService)
	shipping.NewHandler(shippingService, shippingPDFService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 33D.2 — Loading: the "load" half of the dispatch
	// re-sequencing gate. Depends on shippingService (read-only Get,
	// plus the one narrow write — MarkLoaded) and packingService
	// (read-only Get, to resolve a shipment's own packing job's
	// cartons) — a strictly one-way dependency (loading -> shipping ->
	// packing -> picking); shippingService itself is never modified to
	// call into this package at all. See internal/loading's own package
	// doc for the full reasoning and internal/shipping's own
	// Service.MarkLoaded doc for why this is safe against an import
	// cycle.
	loadingRepo := loading.NewRepository(dbManager.DB())
	loadingService := loading.NewService(loadingRepo, shippingService, packingService)
	// Phase 33D.3 -- Loading Manifest.
	loadingPDFService := loading.NewPDFService(loadingService, tenantRepo, userRepo)
	loadingPDFService.SetDocRender(docrenderService)
	loading.NewHandler(loadingService, loadingPDFService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Batch 1a (tenant isolation): stockadjustment now takes productRepo/
	// locationRepo's own real tenant-scoped lookups directly instead of
	// locationUnscoped — see internal/receiving/service.go's own doc for
	// the identical reasoning.
	stockAdjustmentRepo := stockadjustment.NewRepository(dbManager.DB())
	stockAdjustmentService := stockadjustment.NewService(stockAdjustmentRepo, productRepo, locationRepo, inventoryService, cfg.StockAdjustmentPendingWarningHours)
	stockadjustment.NewHandler(stockAdjustmentService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Batch 1b (tenant isolation): stockcount now takes locationRepo's own
	// real tenant-scoped FindByID directly too — locationUnscoped's last
	// caller was this line, so that adapter variable is now removed
	// entirely from this file (location.UnscopedAdapter's own type/
	// constructor in internal/location is left untouched for any future
	// caller).
	stockCountRepo := stockcount.NewRepository(dbManager.DB())
	stockCountService := stockcount.NewService(stockCountRepo, warehouseRepo, zoneRepo, productRepo, locationRepo, inventoryService, stockAdjustmentService)
	stockcount.NewHandler(stockCountService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

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

	// Batch 1c (tenant isolation): productRepo/warehouseRepo already
	// implement FindByIDForTenant (used by stockadjustment/stockcount
	// above), so no new adapter is needed here — only Register's own
	// signature gained the tenant-context middleware arguments.
	replenishmentRepo := replenishment.NewRepository(dbManager.DB())
	replenishmentService := replenishment.NewService(replenishmentRepo, productRepo, warehouseRepo, inventoryService)
	replenishment.NewHandler(replenishmentService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	orderRepo := order.NewRepository(dbManager.DB())
	orderService := order.NewService(orderRepo, productRepo, warehouseRepo, customerRepo, customerDeliveryLocationRepo, pickingService, shippingService, inventoryService)
	orderPDFService := order.NewPDFService(orderService, tenantRepo, userRepo)
	orderPDFService.SetDocRender(docrenderService)
	order.NewHandler(orderService, orderPDFService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 33E.2 — Destination Receiving: closes the inter-warehouse
	// transfer loop 33E.1 proved. Depends only on existing read methods
	// of shippingService (resolve a shipment's own items/order) and
	// orderRepo (resolve that order's own order_type/destination_
	// warehouse_id by order_no) — neither internal/shipping nor
	// internal/order is modified by this phase. Never a direct
	// dependency on orderService/pickingService/packingService: this
	// module only ever reads a shipment already dispatched by the
	// existing pipeline, it never orchestrates any part of it.
	transferReceiptRepo := transferreceipt.NewRepository(dbManager.DB())
	transferReceiptService := transferreceipt.NewService(transferReceiptRepo, shippingService, orderRepo, inventoryService)
	// Phase 33E.3 -- Reconciliation + Inter-Warehouse Transfer Note PDF.
	// shippingService/orderRepo/warehouseRepo are the exact same
	// instances already wired above -- this PDF generator never opens a
	// second dependency graph, only reads through the same already-
	// correct services/repositories every other module already shares.
	transferReceiptPDFService := transferreceipt.NewPDFService(transferReceiptService, shippingService, orderRepo, warehouseRepo, tenantRepo, userRepo)
	transferReceiptPDFService.SetProductInfo(productRepo, brandRepo)
	transferReceiptPDFService.SetExpiryInfo(pickingService)
	transferReceiptPDFService.SetDocRender(docrenderService)
	transferreceipt.NewHandler(transferReceiptService, transferReceiptPDFService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 33C.1 — Batch Picking / Wave Management. Shares the exact
	// same pickingService (and its own CreateTx/SuggestLots/Get) every
	// other orchestrator above already uses — never a second
	// picking.Service instance, never a direct call into
	// inventoryService.
	pickingBatchRepo := pickingbatch.NewRepository(dbManager.DB())
	pickingBatchService := pickingbatch.NewService(pickingBatchRepo, warehouseRepo, pickingService)
	// Phase 33C.3 -- Consolidated Batch Picking List.
	pickingBatchPDFService := pickingbatch.NewPDFService(pickingBatchService, pickingService, tenantRepo, userRepo)
	pickingBatchPDFService.SetProductInfo(productRepo, brandRepo)
	pickingBatchPDFService.SetDocRender(docrenderService)
	pickingbatch.NewHandler(pickingBatchService, pickingBatchPDFService).Register(v1, cfg.JWTSecret, tenantService, cfg.TenantMembershipCacheTTL)

	// Phase 33C.2 — Sortation + Packing Integration. Wired AFTER
	// pickingBatchService exists (SetBatches, not the constructor, the
	// same "set after construction" shape SetUsers above already uses)
	// so packingService.Create can detect whether a source picking task
	// is batch-sourced — a one-way read-only dependency packing ->
	// pickingbatch; pickingbatch itself never calls into packing.
	packingService.SetBatches(pickingBatchService)

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
