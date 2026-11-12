// Phase 29 addition: a shared httptest router builder for the tenant-
// isolation security test suite (backend/test/isolation). It wires the
// same foundation + master-data modules cmd/server/main.go wires —
// auth, user, role, warehouse, zone, location, product — against a
// caller-supplied *gorm.DB, so the isolation tests exercise the real
// handler/service/repository stack (not a mock) over a real SQL Server
// connection, exactly like cmd/server does in production.
//
// Deliberately narrower than main.go: it omits every RabbitMQ-dependent
// module (report, notification's reminder scanner) and every module
// outside Phase 29's own scope (inventory, receiving, putaway, picking,
// shipping, order, stockadjustment, stockcount, replenishment,
// dashboard) per saas-module-roadmap.md's phased rollout — Phase 29 is
// scoped to "auth, user, role, rbac, warehouse, zone, location,
// product" and this harness mirrors that boundary rather than dragging
// in infrastructure (a live RabbitMQ broker) the isolation suite does
// not need. Nothing here ships in any built binary — like testsupport's
// db.go, it exists purely for `go test`.
//
// Lives in its own package, separate from internal/platform/testsupport,
// because it imports product/warehouse/tenant to wire their handlers —
// and those same packages have their own tenant-scoping integration
// tests that need testsupport.ConnectDB/BeginRollbackTx. Keeping this
// router builder out of the testsupport package itself is what avoids
// an import cycle (product -> testsupport -> product).
package testrouter

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wms-backend/internal/auth"
	"wms-backend/internal/location"
	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
	"wms-backend/internal/product"
	"wms-backend/internal/role"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// RouterDeps exposes the narrow repositories the isolation suite's
// fixture builders need to seed data through the same code paths the
// HTTP layer itself uses (e.g. resolving a seeded role's ID), without
// exporting the whole dependency graph BuildTestRouter wires internally.
type RouterDeps struct {
	Engine        *gin.Engine
	WarehouseRepo *warehouse.Repository
	ZoneRepo      *zone.Repository
	LocationRepo  *location.Repository
	ProductRepo   *product.Repository
	UserRepo      *user.Repository
	RoleRepo      *role.Repository
}

// BuildTestRouter wires a gin.Engine against db using the production
// Register(...) calls each module already exposes — the same wiring
// cmd/server/main.go performs — so a test hitting this router observes
// exactly the same route tree, middleware chain, and request/response
// shapes production traffic does.
func BuildTestRouter(db *gorm.DB, jwtSecret string) RouterDeps {
	gin.SetMode(gin.TestMode)

	engine, v1 := apihttp.NewEngine(middleware.Recovery(slog.Default()))

	warehouseRepo := warehouse.NewRepository(db)
	roleRepo := role.NewRepository(db)
	roleService := role.NewService(roleRepo, warehouseRepo)
	role.NewHandler(roleService).Register(v1, jwtSecret)

	userRepo := user.NewRepository(db)
	userService := user.NewService(userRepo, roleRepo, warehouseRepo)
	user.NewHandler(userService).Register(v1, jwtSecret)

	tenantService := tenant.NewService(tenant.NewRepository(db))

	authService := auth.NewService(userRepo, roleRepo, tenantService, jwtSecret, time.Hour)
	auth.NewHandler(authService).Register(v1, jwtSecret)

	zoneRepo := zone.NewRepository(db)
	locationRepo := location.NewRepository(db)

	warehouseService := warehouse.NewService(warehouseRepo, zoneRepo)
	warehouse.NewHandler(warehouseService).Register(v1, jwtSecret)

	zoneService := zone.NewService(zoneRepo, warehouseRepo, locationRepo)
	zone.NewHandler(zoneService).Register(v1, jwtSecret)

	locationService := location.NewService(locationRepo, zoneRepo)
	location.NewHandler(locationService).Register(v1, jwtSecret)

	productRepo := product.NewRepository(db)
	productService := product.NewService(productRepo)
	product.NewHandler(productService).Register(v1, jwtSecret)

	return RouterDeps{
		Engine:        engine,
		WarehouseRepo: warehouseRepo,
		ZoneRepo:      zoneRepo,
		LocationRepo:  locationRepo,
		ProductRepo:   productRepo,
		UserRepo:      userRepo,
		RoleRepo:      roleRepo,
	}
}
