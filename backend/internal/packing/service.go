package packing

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/picking"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/user"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrPickingTaskNotFound = errors.New("packing: picking task not found")
	ErrNotPackable         = errors.New("packing: picking task is not in a packable state")
	ErrNoPackableItems     = errors.New("packing: picking task has no picked lines to pack")
	ErrAlreadyPacked       = errors.New("packing: picking task already has a packing job")
	ErrInvalidTransition   = errors.New("packing: job is not in a state that allows this action")
	ErrNotAssignable       = errors.New("packing: job is not in an assignable status")
	ErrNotReadyToComplete  = errors.New("packing: every line must be packed or flagged with an exception before checker confirmation")
	ErrNotCancellable      = errors.New("packing: job is not in a cancellable status")
	ErrScanMismatch        = errors.New("packing: scanned product/lot does not match this line")
	ErrCartonNotOpen       = errors.New("packing: carton is not open for packing")
	ErrOverpack            = errors.New("packing: packed quantity would exceed this line's picked quantity")
	ErrPackerNotFound      = errors.New("packing: packer user not found")
	ErrItemNotInJob        = errors.New("packing: item does not belong to this job")
	ErrCartonNotInJob      = errors.New("packing: carton does not belong to this job")
)

// pickingChecker is the narrow slice of internal/picking's own Service
// this package needs — satisfied structurally by *picking.Service, the
// same narrow-interface pattern internal/shipping's own pickingChecker
// established for the identical "read a terminal picking task and copy
// its already-picked lines verbatim" need. Packing never writes back to
// internal/picking's own tables through this interface or any other —
// Get is read-only.
type pickingChecker interface {
	Get(ctx context.Context, tenantID, id int64) (*picking.Response, error)
}

// userChecker is the narrow slice of internal/user's own Repository
// AssignPacker needs — satisfied structurally by *user.Repository,
// mirroring picking.userChecker exactly, including its own "set via
// SetUsers, not the constructor" wiring convention and the same
// deliberately-unscoped (not tenant-scoped) existence check, since
// users are not a tenant-scoped entity in this codebase.
type userChecker interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// Service holds the Packing workflow's business rules. It deliberately
// has NO dependency on internal/inventory at all — grepped to confirm
// this package does not import it anywhere: Picking already removed the
// stock and wrote the ledger when it was picked (see package doc); this
// package only ever records how those already-picked quantities get
// boxed into cartons.
type Service struct {
	repo    *Repository
	picking pickingChecker
	// users is set via SetUsers, not the constructor, for the same
	// reason documented on picking.Service.users — AssignPacker is the
	// only method that needs it, and it is nil-checked there.
	users userChecker
}

func NewService(repo *Repository, pickingSvc pickingChecker) *Service {
	return &Service{repo: repo, picking: pickingSvc}
}

// SetUsers wires the packer-existence checker AssignPacker needs —
// called once by cmd/server/main.go's own DI wiring after NewService,
// mirroring picking.Service.SetUsers exactly.
func (s *Service) SetUsers(users userChecker) {
	s.users = users
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	params.Page, params.Limit = page, limit
	return s.repo.List(ctx, tenantID, params)
}

// ListForExport mirrors picking/relocation's own CSV-export path.
func (s *Service) ListForExport(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, error) {
	params.Page = 1
	params.Limit = csvexport.MaxExportRows
	rows, _, err := s.repo.List(ctx, tenantID, params)
	return rows, err
}

// Sources returns completed/short-pick picking tasks not yet packed —
// see Repository.Sources' own doc.
func (s *Service) Sources(ctx context.Context, tenantID int64) ([]SourceResponse, error) {
	return s.repo.Sources(ctx, tenantID)
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	header, err := s.repo.FindJobDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	cartons, err := s.repo.FindCartonsByJob(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	cartonResponses := make([]CartonResponse, 0, len(cartons))
	for _, c := range cartons {
		contents, err := s.repo.ContentsByCarton(ctx, tenantID, c.ID)
		if err != nil {
			return nil, err
		}
		cartonResponses = append(cartonResponses, cartonToResponse(c, contents))
	}
	return &Response{
		ID: header.ID, JobNo: header.JobNo, PickingTaskID: header.PickingTaskID, PickNo: header.PickNo,
		OrderNo: header.OrderNo, Customer: header.Customer, CustomerID: header.CustomerID, ShipToID: header.ShipToID,
		ShipToName: header.ShipToName, ShipToAddress: header.ShipToAddress,
		ShipToProvince: header.ShipToProvince, ShipToDistrict: header.ShipToDistrict,
		ShipToPostalCode:    header.ShipToPostalCode,
		ShipToContactPerson: header.ShipToContactPerson, ShipToContactPhone: header.ShipToContactPhone,
		WarehouseID: header.WarehouseID, WarehouseCode: header.WarehouseCode, Status: Status(header.Status),
		AssignedPackerID: header.AssignedPackerID, AssignedPackerName: header.AssignedPackerName, AssignedAt: header.AssignedAt,
		CheckerID: header.CheckerID, CheckerName: header.CheckerName, CheckerConfirmedAt: header.CheckerConfirmedAt,
		Remark: header.Remark, CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt, CompletedAt: header.CompletedAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
		Items:        items, Cartons: cartonResponses,
	}, nil
}

// Create validates the linked picking task and copies its already-
// picked lines' quantities verbatim — the client never supplies a
// per-line quantity at all (CreateRequest has no Items field), so
// "packing picked_qty does not exceed the picking task's own PickedQty"
// is enforced by construction, not by a range check on client input.
// Rejects a picking task that is not yet terminal (COMPLETED or
// SHORT_PICK — see ErrNotPackable) or that already has a packing job
// (one-to-one, ErrAlreadyPacked, matching shipping's own one-shipment-
// per-picking-task precedent). Never calls any internal/inventory
// function.
//
// Lines are copied for every item with PickedQty > 0 regardless of
// whether that line's own picking status is COMPLETED or SHORT — a
// short-picked line can still have a nonzero partial PickedQty that
// physically left the shelf and must still be packed; only lines with
// nothing actually picked (PickedQty == 0) are excluded.
func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest, userID int64) (*Response, error) {
	task, err := s.picking.Get(ctx, tenantID, req.PickingTaskID)
	if err != nil {
		return nil, ErrPickingTaskNotFound
	}
	if task.Status != picking.StatusCompleted && task.Status != picking.StatusShortPick {
		return nil, ErrNotPackable
	}

	var packableItems []picking.ItemResponse
	for _, it := range task.Items {
		if it.PickedQty > 0 {
			packableItems = append(packableItems, it)
		}
	}
	if len(packableItems) == 0 {
		return nil, ErrNoPackableItems
	}

	// The duplicate-job check is the first thing here that touches the
	// database — everything above is pure validation against the
	// already-fetched picking task.
	if _, err := s.repo.FindByPickingTaskID(ctx, tenantID, req.PickingTaskID); err == nil {
		return nil, ErrAlreadyPacked
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var jobID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		job := &Job{
			PickingTaskID: req.PickingTaskID, OrderNo: task.OrderNo, Customer: task.Customer,
			CustomerID: task.CustomerID, ShipToID: task.ShipToID,
			ShipToName: task.ShipToName, ShipToAddress: task.ShipToAddress,
			ShipToProvince: task.ShipToProvince, ShipToDistrict: task.ShipToDistrict,
			ShipToPostalCode:    task.ShipToPostalCode,
			ShipToContactPerson: task.ShipToContactPerson, ShipToContactPhone: task.ShipToContactPhone,
			WarehouseID: task.WarehouseID, Status: StatusDraft, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateJob(ctx, tenantID, job); err != nil {
			return err
		}
		jobID = job.ID

		for _, it := range packableItems {
			if err := txRepo.CreateItem(ctx, tenantID, &Item{
				PackingJobID: job.ID, PickingTaskItemID: it.ID, ProductID: it.ProductID,
				LotNo: it.LotNo, OrderedQty: it.OrderedQty, PickedQty: it.PickedQty, PackedQty: 0,
				UOM: it.UOM, Status: ItemStatusPending,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "packing.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, jobID)
}

// AssignPacker claims (or reassigns) a job to a packer — a planning/
// ownership annotation, the same authority level as Create, never
// touching reservation or packed-quantity state. Mirrors
// picking.Service.AssignPicker exactly.
func (s *Service) AssignPacker(ctx context.Context, tenantID, jobID, packerUserID, actingUserID int64) (*Response, error) {
	if s.users != nil {
		if _, err := s.users.FindByID(ctx, packerUserID); err != nil {
			return nil, ErrPackerNotFound
		}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	now := time.Now().UTC()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		job, err := txRepo.FindJobByID(ctx, tenantID, jobID)
		if err != nil {
			return err
		}
		if !job.Status.CanAssign() {
			return ErrNotAssignable
		}
		return txRepo.AssignPacker(ctx, tenantID, jobID, packerUserID, now)
	})
	dbctx.LogTransaction(ctx, "packing.assign", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, jobID)
}

// PackItem is the atomic core of this phase: validate -> scan-verify
// (product SKU + lot, adapted from picking.ScanConfirm's shape minus
// the location concept) -> atomically increment packed_qty bounded by
// picked_qty in the same guarded UPDATE (AdjustPackedQty — the
// concurrency-safe overpacking guard itself, not merely an
// application-level check) -> append a carton_contents row -> recompute
// line and job status -> commit. Any failure rolls back everything.
// NEVER calls any internal/inventory function — this is the only
// mutation PackItem performs, and it mutates this package's own tables
// exclusively.
//
// Overpacking (packed quantity that would exceed the line's own frozen
// PickedQty) is rejected outright (ErrOverpack, 422) — never silently
// clamped to whatever room remains.
func (s *Service) PackItem(ctx context.Context, tenantID, jobID int64, req PackItemRequest, userID int64) (*Response, error) {
	if req.Quantity <= 0 {
		return nil, ErrValidation{Message: "quantity must be greater than zero"}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the job exists BEFORE claiming the idempotency key:
		// packing_confirmations.packing_job_id is a real FK
		// (migrations/0091), so inserting the guard for a nonexistent id
		// would raise a raw FK-violation 500 instead of a clean 404 — the
		// same ordering picking/relocation's own Confirm already apply.
		job, err := txRepo.FindJobByID(ctx, tenantID, jobID)
		if err != nil {
			return err
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the PRIMARY KEY constraint and everything below is
		// skipped and rolled back.
		if err := txRepo.InsertConfirmation(ctx, tenantID, req.IdempotencyKey, jobID); err != nil {
			return err
		}

		if !job.Status.CanPack() {
			return ErrInvalidTransition
		}

		item, err := txRepo.FindItemByID(ctx, tenantID, req.ItemID)
		if err != nil {
			return err
		}
		if item.PackingJobID != jobID {
			return ErrItemNotInJob
		}
		if item.Status == ItemStatusException {
			return ErrValidation{Message: "line has already been resolved with an exception"}
		}

		carton, err := txRepo.FindCartonByID(ctx, tenantID, req.CartonID)
		if err != nil {
			return err
		}
		if carton.PackingJobID != jobID {
			return ErrCartonNotInJob
		}
		if carton.Status != CartonStatusOpen {
			return ErrCartonNotOpen
		}

		scanCtx, err := txRepo.FindItemScanContext(ctx, tenantID, req.ItemID)
		if err != nil {
			return err
		}
		if !equalFold(scanCtx.SKU, req.ScannedSKU) {
			return ErrScanMismatch
		}
		if req.ScannedLotNo != "" && !equalFold(scanCtx.LotNo, req.ScannedLotNo) {
			return ErrScanMismatch
		}

		// The concurrency-safe overpacking guard itself — see
		// AdjustPackedQty's own doc. affected == 0 means the WHERE
		// clause's own packed_qty + delta <= picked_qty bound rejected
		// this call; nothing was written.
		affected, err := txRepo.AdjustPackedQty(ctx, tenantID, item.ID, req.Quantity)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrOverpack
		}

		if err := txRepo.CreateCartonContent(ctx, tenantID, &CartonContent{
			CartonID: carton.ID, PackingJobItemID: item.ID, PackedQty: req.Quantity, CreatedBy: &userID,
		}); err != nil {
			return err
		}

		// Re-read after the atomic increment so status derivation uses the
		// row's true, post-increment total — matches
		// picking.Service.Confirm's own re-read-after-AdjustPickedQty
		// convention.
		updated, err := txRepo.FindItemByID(ctx, tenantID, item.ID)
		if err != nil {
			return err
		}
		updated.Status = itemStatus(updated.PickedQty, updated.PackedQty, false)
		if err := txRepo.UpdateItemAfterPack(ctx, tenantID, updated); err != nil {
			return err
		}

		allItems, err := txRepo.FindItemsByJob(ctx, tenantID, jobID)
		if err != nil {
			return err
		}
		job.Status = computeJobStatus(allItems)
		return txRepo.UpdateJob(ctx, tenantID, job)
	})
	dbctx.LogTransaction(ctx, "packing.pack", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a double pack.
		return s.Get(ctx, tenantID, jobID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, jobID)
}

// FlagException records a packing-stage damage/shortage/wrong-item
// exception on a line, terminating it regardless of its current
// packed_qty — mirrors the SHORT branch of picking.Service.Confirm's
// shape, minus any reservation release (Packing holds no reservation to
// release in the first place).
func (s *Service) FlagException(ctx context.Context, tenantID, jobID, itemID int64, req FlagExceptionRequest, userID int64) (*Response, error) {
	reason := ExceptionReason(req.Reason)
	if !reason.IsValid() {
		return nil, ErrValidation{Message: "reason must be one of DAMAGED, SHORTAGE, WRONG_ITEM, OTHER"}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		job, err := txRepo.FindJobByID(ctx, tenantID, jobID)
		if err != nil {
			return err
		}
		if !job.Status.CanPack() {
			return ErrInvalidTransition
		}

		item, err := txRepo.FindItemByID(ctx, tenantID, itemID)
		if err != nil {
			return err
		}
		if item.PackingJobID != jobID {
			return ErrItemNotInJob
		}
		if item.Status == ItemStatusException {
			return ErrValidation{Message: "line has already been flagged with an exception"}
		}

		reasonStr := string(reason)
		item.ExceptionFlag = true
		item.ExceptionReason = &reasonStr
		if req.Quantity > 0 {
			qty := req.Quantity
			item.ExceptionQty = &qty
		}
		if req.Remark != nil && *req.Remark != "" {
			item.Remark = req.Remark
		}
		item.Status = itemStatus(item.PickedQty, item.PackedQty, true)
		if err := txRepo.FlagExceptionItem(ctx, tenantID, item); err != nil {
			return err
		}

		allItems, err := txRepo.FindItemsByJob(ctx, tenantID, jobID)
		if err != nil {
			return err
		}
		job.Status = computeJobStatus(allItems)
		return txRepo.UpdateJob(ctx, tenantID, job)
	})
	dbctx.LogTransaction(ctx, "packing.flag_exception", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, jobID)
}

// CreateCarton adds a new, empty, OPEN carton to a job — container
// prep, allowed any time the job itself is still DRAFT or IN_PROGRESS.
func (s *Service) CreateCarton(ctx context.Context, tenantID, jobID int64, req CreateCartonRequest, userID int64) (*CartonResponse, error) {
	packagingType := PackagingType(req.PackagingType)
	if packagingType == "" {
		packagingType = PackagingBox
	}
	if !packagingType.IsValid() {
		return nil, ErrValidation{Message: "packaging_type must be one of BOX, PALLET, BAG, CRATE, OTHER"}
	}
	weightUnit := WeightUnit(req.WeightUnit)
	if weightUnit == "" {
		weightUnit = WeightKG
	}
	if !weightUnit.IsValid() {
		return nil, ErrValidation{Message: "weight_unit must be KG or LB"}
	}
	dimensionUnit := DimensionUnit(req.DimensionUnit)
	if dimensionUnit == "" {
		dimensionUnit = DimensionCM
	}
	if !dimensionUnit.IsValid() {
		return nil, ErrValidation{Message: "dimension_unit must be CM or IN"}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var cartonID int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		job, err := txRepo.FindJobByID(ctx, tenantID, jobID)
		if err != nil {
			return err
		}
		if !job.Status.CanAddCarton() {
			return ErrInvalidTransition
		}
		carton := &Carton{
			PackingJobID: jobID, PackagingType: packagingType, GrossWeight: req.GrossWeight, NetWeight: req.NetWeight,
			WeightUnit: weightUnit, CartonLength: req.CartonLength, CartonWidth: req.CartonWidth, CartonHeight: req.CartonHeight,
			DimensionUnit: dimensionUnit, Status: CartonStatusOpen, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateCarton(ctx, tenantID, carton); err != nil {
			return err
		}
		cartonID = carton.ID
		return nil
	})
	dbctx.LogTransaction(ctx, "packing.create_carton", start, err)
	if err != nil {
		return nil, err
	}
	return s.getCartonResponse(ctx, tenantID, cartonID)
}

// UpdateCarton updates a carton's descriptive metadata and/or closes it
// (Status "CLOSED" stops PackItem from targeting it further).
func (s *Service) UpdateCarton(ctx context.Context, tenantID, jobID, cartonID int64, req UpdateCartonRequest) (*CartonResponse, error) {
	packagingType := PackagingType(req.PackagingType)
	if !packagingType.IsValid() {
		return nil, ErrValidation{Message: "packaging_type must be one of BOX, PALLET, BAG, CRATE, OTHER"}
	}
	weightUnit := WeightUnit(req.WeightUnit)
	if !weightUnit.IsValid() {
		return nil, ErrValidation{Message: "weight_unit must be KG or LB"}
	}
	dimensionUnit := DimensionUnit(req.DimensionUnit)
	if !dimensionUnit.IsValid() {
		return nil, ErrValidation{Message: "dimension_unit must be CM or IN"}
	}
	status := CartonStatus(req.Status)
	if !status.IsValid() {
		return nil, ErrValidation{Message: "status must be OPEN or CLOSED"}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		if _, err := txRepo.FindJobByID(ctx, tenantID, jobID); err != nil {
			return err
		}
		carton, err := txRepo.FindCartonByID(ctx, tenantID, cartonID)
		if err != nil {
			return err
		}
		if carton.PackingJobID != jobID {
			return ErrCartonNotInJob
		}
		carton.PackagingType = packagingType
		carton.GrossWeight = req.GrossWeight
		carton.NetWeight = req.NetWeight
		carton.WeightUnit = weightUnit
		carton.CartonLength = req.CartonLength
		carton.CartonWidth = req.CartonWidth
		carton.CartonHeight = req.CartonHeight
		carton.DimensionUnit = dimensionUnit
		carton.Status = status
		carton.Remark = req.Remark
		return txRepo.UpdateCarton(ctx, tenantID, carton)
	})
	dbctx.LogTransaction(ctx, "packing.update_carton", start, err)
	if err != nil {
		return nil, err
	}
	return s.getCartonResponse(ctx, tenantID, cartonID)
}

// CheckerConfirm is Phase 33C's second-person verification step —
// transitions IN_PROGRESS -> COMPLETED, requiring every line to already
// be terminal (PACKED or EXCEPTION). Never DRAFT (nothing yet to
// verify) and never re-runnable once COMPLETED/CANCELLED. No quantity
// mutation of any kind — a pure status transition, naturally idempotent-
// safe under a duplicate/retry via the status guard alone (see
// migrations/0091's own doc for why no idempotency table backs this
// action).
func (s *Service) CheckerConfirm(ctx context.Context, tenantID, jobID int64, req CheckerConfirmRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	now := time.Now().UTC()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		job, err := txRepo.FindJobByID(ctx, tenantID, jobID)
		if err != nil {
			return err
		}
		if !job.Status.CanCheckerConfirm() {
			return ErrInvalidTransition
		}
		items, err := txRepo.FindItemsByJob(ctx, tenantID, jobID)
		if err != nil {
			return err
		}
		if !allItemsTerminal(items) {
			return ErrNotReadyToComplete
		}
		if err := txRepo.CheckerConfirmJob(ctx, tenantID, jobID, userID, now); err != nil {
			return err
		}
		if req.Remark != nil && *req.Remark != "" {
			job.Status = StatusCompleted
			job.CompletedAt = &now
			job.Remark = req.Remark
			return txRepo.UpdateJob(ctx, tenantID, job)
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "packing.checker_confirm", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, jobID)
}

// Cancel marks the job CANCELLED. Allowed from DRAFT or IN_PROGRESS.
// Never calls any internal/inventory function — Packing holds no
// reservation to release and has removed no stock, so cancellation is a
// pure status-only transition; whatever has already been packed into a
// carton is left exactly as it is.
func (s *Service) Cancel(ctx context.Context, tenantID, jobID int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		job, err := txRepo.FindJobByID(ctx, tenantID, jobID)
		if err != nil {
			return err
		}
		if !job.Status.CanCancel() {
			return ErrNotCancellable
		}
		job.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelJob(ctx, tenantID, job)
	})
	dbctx.LogTransaction(ctx, "packing.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, jobID)
}

func (s *Service) getCartonResponse(ctx context.Context, tenantID, cartonID int64) (*CartonResponse, error) {
	carton, err := s.repo.FindCartonByID(ctx, tenantID, cartonID)
	if err != nil {
		return nil, err
	}
	contents, err := s.repo.ContentsByCarton(ctx, tenantID, cartonID)
	if err != nil {
		return nil, err
	}
	resp := cartonToResponse(*carton, contents)
	return &resp, nil
}

func cartonToResponse(c Carton, contents []ContentResponse) CartonResponse {
	return CartonResponse{
		ID: c.ID, PackingJobID: c.PackingJobID, LPN: c.LPN, PackagingType: c.PackagingType,
		GrossWeight: c.GrossWeight, NetWeight: c.NetWeight, WeightUnit: c.WeightUnit,
		CartonLength: c.CartonLength, CartonWidth: c.CartonWidth, CartonHeight: c.CartonHeight,
		DimensionUnit: c.DimensionUnit, Status: c.Status, Remark: c.Remark, Contents: contents,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func equalFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
