package packing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("packing: not found")
var ErrItemNotFound = errors.New("packing: item not found")
var ErrCartonNotFound = errors.New("packing: carton not found")
var ErrDuplicateIdempotencyKey = errors.New("packing: idempotency key already used")

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the pack/confirm workflow) live in Service. Every method
// takes an explicit tenantID and applies it to every query, mirroring
// internal/relocation's own discipline from the start (this package has
// no pre-tenant-scoping history to retrofit).
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both the
// raw *gorm.DB and a Repository bound to it — matches picking.Repository.
// Transaction/relocation.Repository.Transaction exactly.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateJob inserts the header, then derives job_no from the new
// IDENTITY id in the same transaction — the same race-free-by-
// construction approach as pick_no/relocation_no.
func (r *Repository) CreateJob(ctx context.Context, tenantID int64, job *Job) error {
	job.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(job).Error; err != nil {
		return err
	}
	job.JobNo = fmt.Sprintf("PACK-%06d", job.ID)
	return r.db.WithContext(ctx).Model(&Job{}).Where("id = ? AND tenant_id = ?", job.ID, tenantID).
		Update("job_no", job.JobNo).Error
}

func (r *Repository) FindJobByID(ctx context.Context, tenantID, id int64) (*Job, error) {
	var job Job
	err := r.db.WithContext(ctx).First(&job, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// FindByPickingTaskID returns the packing job already linked to a
// picking task, if any — used by Create for a friendly pre-check ahead
// of the UNIQUE constraint (migrations/0087) that makes "one packing
// job per picking task" authoritative regardless, the same pre-check
// pattern shipping.Repository.FindByPickingTaskID established.
func (r *Repository) FindByPickingTaskID(ctx context.Context, tenantID, pickingTaskID int64) (*Job, error) {
	var job Job
	err := r.db.WithContext(ctx).First(&job, "picking_task_id = ? AND tenant_id = ?", pickingTaskID, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// FindByPickingTaskAndOrder is FindByPickingTaskID's own order-aware
// generalization (Phase 33C.2) — the friendly pre-check ahead of
// migrations/0102's own dual filtered-unique-index scheme. orderID ==
// nil matches the ordinary (non-batch) "order_id IS NULL" rule exactly;
// a non-nil orderID matches the new per-order rule for a batch-sourced
// job.
func (r *Repository) FindByPickingTaskAndOrder(ctx context.Context, tenantID, pickingTaskID int64, orderID *int64) (*Job, error) {
	q := r.db.WithContext(ctx).Where("picking_task_id = ? AND tenant_id = ?", pickingTaskID, tenantID)
	if orderID == nil {
		q = q.Where("order_id IS NULL")
	} else {
		q = q.Where("order_id = ?", *orderID)
	}
	var job Job
	err := q.First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

type headerRow struct {
	ID                  int64
	JobNo               string
	PickingTaskID       int64
	PickNo              string
	OrderID             *int64
	OrderNo             string
	Customer            *string
	CustomerID          *int64
	ShipToID            *int64
	ShipToName          *string
	ShipToAddress       *string
	ShipToProvince      *string
	ShipToDistrict      *string
	ShipToPostalCode    *string
	ShipToContactPerson *string
	ShipToContactPhone  *string
	WarehouseID         int64
	WarehouseCode       string
	Status              string
	AssignedPackerID    *int64
	AssignedPackerName  *string
	AssignedAt          *time.Time
	CheckerID           *int64
	CheckerName         *string
	CheckerConfirmedAt  *time.Time
	Remark              *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	CompletedAt         *time.Time
	CancelledAt         *time.Time
	CancelledBy         *int64
	CancelledByName     *string
	CancelRemark        *string
}

// FindJobDetail returns the header with warehouse/picking/user display
// fields resolved, for GET /packing-jobs/{id}'s response.
func (r *Repository) FindJobDetail(ctx context.Context, tenantID, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pj.id AS id, pj.job_no AS job_no, pj.picking_task_id AS picking_task_id, pt.pick_no AS pick_no,
			pj.order_id AS order_id,
			pj.order_no AS order_no, pj.customer AS customer, pj.customer_id AS customer_id, pj.ship_to_id AS ship_to_id,
			pj.ship_to_name AS ship_to_name, pj.ship_to_address AS ship_to_address,
			pj.ship_to_province AS ship_to_province, pj.ship_to_district AS ship_to_district,
			pj.ship_to_postal_code AS ship_to_postal_code,
			pj.ship_to_contact_person AS ship_to_contact_person, pj.ship_to_contact_phone AS ship_to_contact_phone,
			pj.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			pj.status AS status,
			pj.assigned_packer_id AS assigned_packer_id, apu.full_name AS assigned_packer_name, pj.assigned_at AS assigned_at,
			pj.checker_id AS checker_id, cku.full_name AS checker_name, pj.checker_confirmed_at AS checker_confirmed_at,
			pj.remark AS remark, pj.created_at AS created_at, pj.updated_at AS updated_at, pj.completed_at AS completed_at,
			pj.cancelled_at AS cancelled_at, pj.cancelled_by AS cancelled_by, clu.full_name AS cancelled_by_name,
			pj.cancel_remark AS cancel_remark
		FROM packing_jobs pj
		JOIN warehouses w ON w.id = pj.warehouse_id
		JOIN picking_tasks pt ON pt.id = pj.picking_task_id
		LEFT JOIN users apu ON apu.id = pj.assigned_packer_id
		LEFT JOIN users cku ON cku.id = pj.checker_id
		LEFT JOIN users clu ON clu.id = pj.cancelled_by
		WHERE pj.id = ? AND pj.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// UpdateJob writes the fields PackItem/FlagException derive after
// recomputing the header status — deliberately excludes the checker/
// cancellation audit columns, which have their own dedicated writers
// below (matching picking/relocation's own CancelTask-is-separate-from-
// UpdateTask convention).
func (r *Repository) UpdateJob(ctx context.Context, tenantID int64, job *Job) error {
	result := r.db.WithContext(ctx).Model(&Job{}).Where("id = ? AND tenant_id = ?", job.ID, tenantID).Updates(map[string]any{
		"status":       job.Status,
		"remark":       job.Remark,
		"completed_at": job.CompletedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignPacker writes assigned_packer_id/assigned_at together — mirrors
// picking.Repository.AssignPicker exactly.
func (r *Repository) AssignPacker(ctx context.Context, tenantID, jobID, packerID int64, assignedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&Job{}).Where("id = ? AND tenant_id = ?", jobID, tenantID).Updates(map[string]any{
		"assigned_packer_id": packerID,
		"assigned_at":        assignedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CheckerConfirmJob writes the exact columns a successful CheckerConfirm
// derives — status, completed_at, checker_id, checker_confirmed_at — in
// one explicit UPDATE, deliberately separate from UpdateJob.
func (r *Repository) CheckerConfirmJob(ctx context.Context, tenantID, jobID, checkerID int64, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&Job{}).Where("id = ? AND tenant_id = ?", jobID, tenantID).Updates(map[string]any{
		"status":               StatusCompleted,
		"completed_at":         now,
		"checker_id":           checkerID,
		"checker_confirmed_at": now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CancelJob writes the exact columns Cancel derives — matches
// picking.Repository.CancelTask/relocation.Repository.CancelTask.
func (r *Repository) CancelJob(ctx context.Context, tenantID int64, job *Job) error {
	result := r.db.WithContext(ctx).Model(&Job{}).Where("id = ? AND tenant_id = ?", job.ID, tenantID).Updates(map[string]any{
		"status":        job.Status,
		"cancelled_at":  job.CancelledAt,
		"cancelled_by":  job.CancelledBy,
		"cancel_remark": job.CancelRemark,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

type listRow struct {
	ID                 int64
	JobNo              string
	PickingTaskID      int64
	PickNo             string
	OrderID            *int64
	OrderNo            string
	Customer           *string
	WarehouseCode      string
	Status             string
	ItemCount          int
	PickedTotal        float64
	PackedTotal        float64
	AssignedPackerName *string
	CreatedAt          time.Time
}

// List returns job-level rows (one row per job, with an aggregated item
// count and progress percentage) — mirrors picking.Repository.List's
// own task-level shape.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM packing_jobs pj
		JOIN warehouses w ON w.id = pj.warehouse_id
		LEFT JOIN users apu ON apu.id = pj.assigned_packer_id
		LEFT JOIN packing_job_items pji ON pji.packing_job_id = pj.id
		LEFT JOIN products p ON p.id = pji.product_id
	`
	where := `WHERE pj.tenant_id = ?
		AND (? = '' OR pj.job_no LIKE ? OR pj.order_no LIKE ? OR pj.customer LIKE ? OR p.sku LIKE ? OR p.name LIKE ?)
		AND (? = '' OR pj.status = ?)
		AND (? = 0 OR pj.warehouse_id = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like, like, like, like,
		string(params.Status), string(params.Status),
		params.WarehouseID, params.WarehouseID,
	}

	countSQL := "SELECT COUNT(DISTINCT pj.id) " + base + where
	var total int64
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT pj.id AS id, pj.job_no AS job_no, pj.picking_task_id AS picking_task_id, '' AS pick_no,
			pj.order_id AS order_id,
			pj.order_no AS order_no, pj.customer AS customer, w.code AS warehouse_code, pj.status AS status,
			apu.full_name AS assigned_packer_name, pj.created_at AS created_at,
			COUNT(DISTINCT pji.id) AS item_count,
			COALESCE(SUM(pji.picked_qty), 0) AS picked_total,
			COALESCE(SUM(pji.packed_qty), 0) AS packed_total
	` + base + where + `
		GROUP BY pj.id, pj.job_no, pj.picking_task_id, pj.order_id, pj.order_no, pj.customer, w.code, pj.status,
			apu.full_name, pj.created_at
		ORDER BY pj.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		progress := 0.0
		if row.PickedTotal > 0 {
			progress = (row.PackedTotal / row.PickedTotal) * 100
		} else if row.PackedTotal > 0 {
			progress = 100
		}
		responses = append(responses, ListResponse{
			ID: row.ID, JobNo: row.JobNo, PickingTaskID: row.PickingTaskID, OrderID: row.OrderID,
			OrderNo: row.OrderNo, Customer: row.Customer, WarehouseCode: row.WarehouseCode,
			Status: Status(row.Status), ItemCount: row.ItemCount, ProgressPct: progress,
			AssignedPackerName: row.AssignedPackerName, CreatedAt: row.CreatedAt,
		})
	}
	return responses, total, nil
}

type itemRow struct {
	ID                int64
	PickingTaskItemID int64
	ProductID         int64
	SKU               string
	ProductName       string
	LotNo             string
	LocationID        int64
	LocationCode      string
	OrderedQty        float64
	PickedQty         float64
	PackedQty         float64
	UOM               string
	Status            string
	ExceptionFlag     bool
	ExceptionReason   *string
	ExceptionQty      *float64
	Remark            *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ListItems resolves display fields for one job's lines. Joins through
// to picking_task_items/locations (Phase 33D.1) to resolve the FROM-
// location a packing_job_items row itself has no concept of — see
// ItemResponse's own doc.
func (r *Repository) ListItems(ctx context.Context, tenantID, jobID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pji.id AS id, pji.picking_task_item_id AS picking_task_item_id,
			pji.product_id AS product_id, p.sku AS sku, p.name AS product_name, pji.lot_no AS lot_no,
			pti.location_id AS location_id, l.code AS location_code,
			pji.ordered_qty AS ordered_qty, pji.picked_qty AS picked_qty, pji.packed_qty AS packed_qty, pji.uom AS uom,
			pji.status AS status, pji.exception_flag AS exception_flag, pji.exception_reason AS exception_reason,
			pji.exception_qty AS exception_qty, pji.remark AS remark,
			pji.created_at AS created_at, pji.updated_at AS updated_at
		FROM packing_job_items pji
		JOIN products p ON p.id = pji.product_id
		JOIN picking_task_items pti ON pti.id = pji.picking_task_item_id
		JOIN locations l ON l.id = pti.location_id
		WHERE pji.packing_job_id = ? AND pji.tenant_id = ?
		ORDER BY pji.id ASC
	`, jobID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	cartonsByItem, err := r.cartonLPNsByItem(ctx, tenantID, jobID)
	if err != nil {
		return nil, err
	}

	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, PickingTaskItemID: row.PickingTaskItemID,
			ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName, LotNo: row.LotNo,
			LocationID: row.LocationID, LocationCode: row.LocationCode,
			OrderedQty: row.OrderedQty, PickedQty: row.PickedQty, PackedQty: row.PackedQty, UOM: row.UOM,
			Status: ItemStatus(row.Status), ExceptionFlag: row.ExceptionFlag, ExceptionReason: row.ExceptionReason,
			ExceptionQty: row.ExceptionQty, Remark: row.Remark,
			CartonLPNs: cartonsByItem[row.ID],
			CreatedAt:  row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return items, nil
}

// cartonLPNsByItem returns, for every packing_job_item in jobID, the
// distinct set of carton LPNs it has been packed into — used by both
// ListItems (API display) and the Packing List PDF ("which carton(s)").
func (r *Repository) cartonLPNsByItem(ctx context.Context, tenantID, jobID int64) (map[int64][]string, error) {
	type row struct {
		PackingJobItemID int64
		LPN              string
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT cc.packing_job_item_id AS packing_job_item_id, c.lpn AS lpn
		FROM carton_contents cc
		JOIN cartons c ON c.id = cc.carton_id
		JOIN packing_job_items pji ON pji.id = cc.packing_job_item_id
		WHERE pji.packing_job_id = ? AND cc.tenant_id = ?
		ORDER BY c.lpn ASC
	`, jobID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[int64][]string)
	for _, r := range rows {
		result[r.PackingJobItemID] = append(result[r.PackingJobItemID], r.LPN)
	}
	return result, nil
}

func (r *Repository) CreateItem(ctx context.Context, tenantID int64, item *Item) error {
	item.TenantID = tenantID
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *Repository) FindItemByID(ctx context.Context, tenantID, id int64) (*Item, error) {
	var item Item
	err := r.db.WithContext(ctx).First(&item, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// FindItemsByJob returns the raw Item rows — used internally by
// PackItem/FlagException/CheckerConfirm to compute the header's new
// status/eligibility after applying deltas.
func (r *Repository) FindItemsByJob(ctx context.Context, tenantID, jobID int64) ([]Item, error) {
	var items []Item
	err := r.db.WithContext(ctx).Where("packing_job_id = ? AND tenant_id = ?", jobID, tenantID).Find(&items).Error
	return items, err
}

// AdjustPackedQty atomically increments a line's packed_qty by delta in
// one UPDATE statement, bounded by picked_qty in the very same WHERE
// clause — this is the concurrency-safe overpacking guard itself: two
// concurrent pack actions against the same line can never together pack
// more than picked_qty, because the arithmetic and the "does this fit"
// check are one atomic SQL statement, not a Go read-then-write. Returns
// RowsAffected == 0 when the bound would be exceeded (the caller
// interprets that as ErrOverpack and rejects the whole action), matching
// internal/picking.Repository.AdjustPickedQty's own atomic-guarded-UPDATE
// shape.
func (r *Repository) AdjustPackedQty(ctx context.Context, tenantID, itemID int64, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE packing_job_items
		SET packed_qty = packed_qty + ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND packed_qty + ? <= picked_qty
	`, delta, itemID, tenantID, delta)
	return result.RowsAffected, result.Error
}

// UpdateItemAfterPack writes the fields PackItem derives after calling
// AdjustPackedQty — deliberately excludes packed_qty itself so this
// write can never clobber a concurrent transaction's own atomic
// increment with a value computed from a stale read. Matches
// picking.Repository.UpdateItemAfterConfirm's own split.
func (r *Repository) UpdateItemAfterPack(ctx context.Context, tenantID int64, item *Item) error {
	result := r.db.WithContext(ctx).Model(&Item{}).Where("id = ? AND tenant_id = ?", item.ID, tenantID).Updates(map[string]any{
		"status": item.Status,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// FlagExceptionItem writes the fields a successful FlagException
// derives — exception_flag/exception_reason/exception_qty/remark/status
// all together, since they are only ever set as one atomic business
// event.
func (r *Repository) FlagExceptionItem(ctx context.Context, tenantID int64, item *Item) error {
	result := r.db.WithContext(ctx).Model(&Item{}).Where("id = ? AND tenant_id = ?", item.ID, tenantID).Updates(map[string]any{
		"status":           item.Status,
		"exception_flag":   item.ExceptionFlag,
		"exception_reason": item.ExceptionReason,
		"exception_qty":    item.ExceptionQty,
		"remark":           item.Remark,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// itemScanContext is what PackItem needs to validate a scan against —
// the line's own assigned product SKU/lot, adapted from
// picking.itemScanContext minus the location concept (a carton has no
// location).
type itemScanContext struct {
	PackingJobID int64
	SKU          string
	LotNo        string
}

// FindItemScanContext resolves itemID's own expected scan identity.
func (r *Repository) FindItemScanContext(ctx context.Context, tenantID, itemID int64) (*itemScanContext, error) {
	var ctxRow itemScanContext
	err := r.db.WithContext(ctx).Raw(`
		SELECT pji.packing_job_id AS packing_job_id, p.sku AS sku, pji.lot_no AS lot_no
		FROM packing_job_items pji
		JOIN products p ON p.id = pji.product_id
		WHERE pji.id = ? AND pji.tenant_id = ?
	`, itemID, tenantID).Scan(&ctxRow).Error
	if err != nil {
		return nil, err
	}
	if ctxRow.PackingJobID == 0 {
		return nil, ErrItemNotFound
	}
	return &ctxRow, nil
}

// CreateCarton inserts the header, then derives lpn from the new
// IDENTITY id in the same transaction — same race-free pattern as
// job_no/pick_no.
func (r *Repository) CreateCarton(ctx context.Context, tenantID int64, carton *Carton) error {
	carton.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(carton).Error; err != nil {
		return err
	}
	carton.LPN = fmt.Sprintf("CTN-%06d", carton.ID)
	return r.db.WithContext(ctx).Model(&Carton{}).Where("id = ? AND tenant_id = ?", carton.ID, tenantID).
		Update("lpn", carton.LPN).Error
}

func (r *Repository) FindCartonByID(ctx context.Context, tenantID, id int64) (*Carton, error) {
	var carton Carton
	err := r.db.WithContext(ctx).First(&carton, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCartonNotFound
	}
	if err != nil {
		return nil, err
	}
	return &carton, nil
}

func (r *Repository) FindCartonsByJob(ctx context.Context, tenantID, jobID int64) ([]Carton, error) {
	var cartons []Carton
	err := r.db.WithContext(ctx).Where("packing_job_id = ? AND tenant_id = ?", jobID, tenantID).Order("id ASC").Find(&cartons).Error
	return cartons, err
}

// UpdateCarton writes the fields UpdateCarton derives — packaging/
// weight/dimensions/status/remark. lpn/packing_job_id are immutable once
// created and are never part of this UPDATE.
func (r *Repository) UpdateCarton(ctx context.Context, tenantID int64, carton *Carton) error {
	result := r.db.WithContext(ctx).Model(&Carton{}).Where("id = ? AND tenant_id = ?", carton.ID, tenantID).Updates(map[string]any{
		"packaging_type": carton.PackagingType,
		"gross_weight":   carton.GrossWeight,
		"net_weight":     carton.NetWeight,
		"weight_unit":    carton.WeightUnit,
		"carton_length":  carton.CartonLength,
		"carton_width":   carton.CartonWidth,
		"carton_height":  carton.CartonHeight,
		"dimension_unit": carton.DimensionUnit,
		"status":         carton.Status,
		"remark":         carton.Remark,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCartonNotFound
	}
	return nil
}

func (r *Repository) CreateCartonContent(ctx context.Context, tenantID int64, content *CartonContent) error {
	content.TenantID = tenantID
	return r.db.WithContext(ctx).Create(content).Error
}

// ContentsByCarton returns every carton_contents row for cartonID,
// joined with product/lot display fields, for CartonResponse.Contents
// and the Carton Label PDF's "contents summary".
func (r *Repository) ContentsByCarton(ctx context.Context, tenantID, cartonID int64) ([]ContentResponse, error) {
	type row struct {
		ID               int64
		PackingJobItemID int64
		SKU              string
		ProductName      string
		LotNo            string
		PackedQty        float64
		UOM              string
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT cc.id AS id, cc.packing_job_item_id AS packing_job_item_id, p.sku AS sku, p.name AS product_name,
			pji.lot_no AS lot_no, cc.packed_qty AS packed_qty, pji.uom AS uom
		FROM carton_contents cc
		JOIN packing_job_items pji ON pji.id = cc.packing_job_item_id
		JOIN products p ON p.id = pji.product_id
		WHERE cc.carton_id = ? AND cc.tenant_id = ?
		ORDER BY cc.id ASC
	`, cartonID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	resp := make([]ContentResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, ContentResponse{
			ID: row.ID, PackingJobItemID: row.PackingJobItemID, SKU: row.SKU, ProductName: row.ProductName,
			LotNo: row.LotNo, PackedQty: row.PackedQty, UOM: row.UOM,
		})
	}
	return resp, nil
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0091's doc. Must be called inside the same transaction as
// AdjustPackedQty/CreateCartonContent.
func (r *Repository) InsertConfirmation(ctx context.Context, tenantID int64, idempotencyKey string, jobID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, TenantID: tenantID, PackingJobID: jobID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

type sourceRow struct {
	PickingTaskID int64
	PickNo        string
	OrderNo       string
	Customer      *string
	ItemCount     int
	BatchID       *int64
}

// Sources returns completed (or partially-completed, i.e. SHORT_PICK
// with at least one line that has a nonzero PickedQty) picking tasks
// that still have at least one packable packing job left to create —
// powers GET /packing-jobs/sources, mirroring shipping.Repository.
// Sources' own query shape (NOT EXISTS against this package's own join
// table) for the ordinary (non-batch) case.
//
// Phase 33C.2: a batch-sourced task (picking_batches.picking_task_id =
// pt.id) is ALWAYS included here regardless of how many packing jobs
// already exist against it — "no job exists yet at all" would wrongly
// stop surfacing a batch task the moment its FIRST order got a job,
// even though its other orders still need their own. For each such row
// this additionally resolves PendingOrders (see that field's own doc);
// a batch-sourced row whose every order already has its own job is
// filtered back out in Go, since nothing is left to create for it.
// Scoped to tenantID throughout.
func (r *Repository) Sources(ctx context.Context, tenantID int64) ([]SourceResponse, error) {
	var rows []sourceRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pt.id AS picking_task_id, pt.pick_no AS pick_no, pt.order_no AS order_no, pt.customer AS customer,
			COUNT(DISTINCT pti.id) AS item_count, pb.id AS batch_id
		FROM picking_tasks pt
		JOIN picking_task_items pti ON pti.picking_task_id = pt.id AND pti.picked_qty > 0
		LEFT JOIN picking_batches pb ON pb.picking_task_id = pt.id AND pb.tenant_id = pt.tenant_id
		WHERE pt.tenant_id = ?
			AND pt.status IN ('COMPLETED', 'SHORT_PICK')
			AND (
				pb.id IS NOT NULL
				OR NOT EXISTS (SELECT 1 FROM packing_jobs pj WHERE pj.picking_task_id = pt.id)
			)
		GROUP BY pt.id, pt.pick_no, pt.order_no, pt.customer, pb.id
		ORDER BY pt.id DESC
	`, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	sources := make([]SourceResponse, 0, len(rows))
	for _, row := range rows {
		if row.BatchID == nil {
			sources = append(sources, SourceResponse{
				PickingTaskID: row.PickingTaskID, PickNo: row.PickNo, OrderNo: row.OrderNo, Customer: row.Customer,
				ItemCount: row.ItemCount,
			})
			continue
		}

		pending, err := r.pendingBatchOrders(ctx, tenantID, *row.BatchID, row.PickingTaskID)
		if err != nil {
			return nil, err
		}
		if len(pending) == 0 {
			// Every order in this batch already has its own job —
			// nothing left to create.
			continue
		}
		sources = append(sources, SourceResponse{
			PickingTaskID: row.PickingTaskID, PickNo: row.PickNo, OrderNo: row.OrderNo, Customer: row.Customer,
			ItemCount: row.ItemCount, IsBatch: true, BatchID: row.BatchID, PendingOrders: pending,
		})
	}
	return sources, nil
}

// pendingBatchOrders resolves, for a batch-sourced source task, exactly
// which of the batch's own included orders do not yet have their own
// packing job against pickingTaskID.
func (r *Repository) pendingBatchOrders(ctx context.Context, tenantID, batchID, pickingTaskID int64) ([]PendingOrderResponse, error) {
	type row struct {
		OrderID  int64
		OrderNo  string
		Customer *string
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT o.id AS order_id, o.order_no AS order_no, o.customer AS customer
		FROM picking_batch_orders pbo
		JOIN orders o ON o.id = pbo.order_id
		WHERE pbo.batch_id = ? AND pbo.tenant_id = ?
			AND NOT EXISTS (
				SELECT 1 FROM packing_jobs pj
				WHERE pj.picking_task_id = ? AND pj.tenant_id = ? AND pj.order_id = o.id
			)
		ORDER BY o.id ASC
	`, batchID, tenantID, pickingTaskID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]PendingOrderResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, PendingOrderResponse{OrderID: r.OrderID, OrderNo: r.OrderNo, Customer: r.Customer})
	}
	return out, nil
}
