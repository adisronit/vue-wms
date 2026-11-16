package loading

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/packing"
)

var ErrNotFound = errors.New("loading: not found")
var ErrCartonNotFound = errors.New("loading: carton not found")

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the confirm workflow) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both the
// raw *gorm.DB and a Repository bound to it — the same pattern every
// other workflow package in this codebase establishes.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateTrip stamps tenantID onto the header, inserts it, then derives
// trip_no from the new IDENTITY id in the same transaction — the same
// race-free-by-construction approach as every other *_no column in this
// codebase.
func (r *Repository) CreateTrip(ctx context.Context, tenantID int64, t *Trip) error {
	t.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return err
	}
	t.TripNo = fmt.Sprintf("TRIP-%06d", t.ID)
	return r.db.WithContext(ctx).Model(&Trip{}).Where("id = ? AND tenant_id = ?", t.ID, tenantID).
		Update("trip_no", t.TripNo).Error
}

// FindTripByID scopes to tenantID — a cross-tenant id returns
// ErrNotFound exactly like a nonexistent one, matching every other
// module's own FindByID precedent.
func (r *Repository) FindTripByID(ctx context.Context, tenantID, id int64) (*Trip, error) {
	var t Trip
	err := r.db.WithContext(ctx).First(&t, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// casUpdate is the shared compare-and-swap primitive every trip-status
// write below uses: the UPDATE only applies, and reports success via
// rows-affected, when the row's CURRENT status still equals fromStatus
// at the moment the UPDATE runs — the same atomic-guarded-UPDATE
// pattern packing_job_items.packed_qty (AdjustPackedQty) and this
// package's own MarkCartonLoaded/MarkCartonShort use, adapted to a
// status column instead of a quantity column. Prevents two concurrent
// status-changing calls (e.g. Confirm racing a Cancel) from both
// succeeding against the same trip. Each caller below passes only the
// columns ITS OWN transition actually needs, so e.g. cancelling a
// LOADING trip can never accidentally null out an already-stamped
// actual_loading_start_at.
func (r *Repository) casUpdate(ctx context.Context, tenantID, id int64, fromStatus Status, fields map[string]any) (int64, error) {
	result := r.db.WithContext(ctx).Model(&Trip{}).
		Where("id = ? AND tenant_id = ? AND status = ?", id, tenantID, string(fromStatus)).
		Updates(fields)
	return result.RowsAffected, result.Error
}

// TransitionToLoading flips a PLANNED trip to LOADING and stamps
// actual_loading_start_at — the first time any carton on it is
// resolved. See Service.transitionToLoadingIfNeeded's own doc.
func (r *Repository) TransitionToLoading(ctx context.Context, tenantID, tripID int64, startedAt time.Time) (int64, error) {
	return r.casUpdate(ctx, tenantID, tripID, StatusPlanned, map[string]any{
		"status":                  string(StatusLoading),
		"actual_loading_start_at": startedAt,
	})
}

// CompleteTrip flips a LOADING trip to COMPLETED and stamps
// actual_loading_end_at/completed_at — see Service.ConfirmTrip's own
// doc.
func (r *Repository) CompleteTrip(ctx context.Context, tenantID, tripID int64, completedAt time.Time) (int64, error) {
	return r.casUpdate(ctx, tenantID, tripID, StatusLoading, map[string]any{
		"status":                string(StatusCompleted),
		"actual_loading_end_at": completedAt,
		"completed_at":          completedAt,
	})
}

// CancelTripRow flips a PLANNED or LOADING trip to CANCELLED and stamps
// the Phase-18-style cancellation audit columns — see Service.
// CancelTrip's own doc. fromStatus is the trip's own current status, as
// already read by the caller (CanCancel already proved it is PLANNED or
// LOADING).
func (r *Repository) CancelTripRow(ctx context.Context, tenantID, tripID int64, fromStatus Status, userID int64, remark *string, cancelledAt time.Time) (int64, error) {
	fields := map[string]any{
		"status":       string(StatusCancelled),
		"cancelled_at": cancelledAt,
		"cancelled_by": userID,
	}
	if remark != nil && *remark != "" {
		fields["cancel_remark"] = *remark
	}
	return r.casUpdate(ctx, tenantID, tripID, fromStatus, fields)
}

type tripDetailRow struct {
	ID                   int64
	TripNo               string
	VehicleNo            *string
	DriverName           *string
	Carrier              *string
	LoadingDock          *string
	Route                *string
	ScheduledLoadingTime *time.Time
	ActualLoadingStartAt *time.Time
	ActualLoadingEndAt   *time.Time
	VehicleSeal          *string
	Status               string
	Remark               *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	CompletedAt          *time.Time
	CancelledAt          *time.Time
	CancelledBy          *int64
	CancelledByName      *string
	CancelRemark         *string
}

// FindDetail returns the header with cancelled_by resolved to a display
// name (the same LEFT JOIN users pattern internal/shipping's own
// FindDetail uses) — for GET /loading-trips/{id}'s response. Scoped to
// tenantID.
func (r *Repository) FindDetail(ctx context.Context, tenantID, id int64) (*tripDetailRow, error) {
	var row tripDetailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT t.id AS id, t.trip_no AS trip_no, t.vehicle_no AS vehicle_no, t.driver_name AS driver_name,
			t.carrier AS carrier, t.loading_dock AS loading_dock, t.route AS route,
			t.scheduled_loading_time AS scheduled_loading_time, t.actual_loading_start_at AS actual_loading_start_at,
			t.actual_loading_end_at AS actual_loading_end_at, t.vehicle_seal AS vehicle_seal, t.status AS status,
			t.remark AS remark, t.created_at AS created_at, t.updated_at AS updated_at, t.completed_at AS completed_at,
			t.cancelled_at AS cancelled_at, t.cancelled_by AS cancelled_by, u.full_name AS cancelled_by_name,
			t.cancel_remark AS cancel_remark
		FROM loading_trips t
		LEFT JOIN users u ON u.id = t.cancelled_by
		WHERE t.id = ? AND t.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

type tripListRow struct {
	ID                   int64
	TripNo               string
	VehicleNo            *string
	DriverName           *string
	Carrier              *string
	Status               string
	ScheduledLoadingTime *time.Time
	CreatedAt            time.Time
	ShipmentCount        int
	PlannedCount         int
	LoadedCount          int
	ShortCount           int
}

// List returns trip-level rows with shipment/carton progress counts —
// matches the Loading Trip List screen's table. Scoped to tenantID.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `FROM loading_trips t`
	where := `WHERE t.tenant_id = ?
		AND (? = '' OR t.trip_no LIKE ? OR t.vehicle_no LIKE ? OR t.driver_name LIKE ?)
		AND (? = '' OR t.status = ?)`
	like := "%" + params.Search + "%"
	args := []any{tenantID, params.Search, like, like, like, string(params.Status), string(params.Status)}

	countSQL := "SELECT COUNT(*) " + base + " " + where
	var total int64
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT t.id AS id, t.trip_no AS trip_no, t.vehicle_no AS vehicle_no, t.driver_name AS driver_name,
			t.carrier AS carrier, t.status AS status, t.scheduled_loading_time AS scheduled_loading_time,
			t.created_at AS created_at,
			(SELECT COUNT(*) FROM loading_trip_shipments s WHERE s.trip_id = t.id) AS shipment_count,
			(SELECT COUNT(*) FROM loading_trip_cartons c WHERE c.trip_id = t.id) AS planned_count,
			(SELECT COUNT(*) FROM loading_trip_cartons c WHERE c.trip_id = t.id AND c.status = 'LOADED') AS loaded_count,
			(SELECT COUNT(*) FROM loading_trip_cartons c WHERE c.trip_id = t.id AND c.status = 'SHORT') AS short_count
	` + base + " " + where + `
		ORDER BY t.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []tripListRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, ListResponse{
			ID: row.ID, TripNo: row.TripNo, VehicleNo: row.VehicleNo, DriverName: row.DriverName,
			Carrier: row.Carrier, Status: Status(row.Status), ScheduledLoadingTime: row.ScheduledLoadingTime,
			ShipmentCount: row.ShipmentCount, PlannedCount: row.PlannedCount, LoadedCount: row.LoadedCount,
			ShortCount: row.ShortCount, CreatedAt: row.CreatedAt,
		})
	}
	return responses, total, nil
}

// CreateTripShipment inserts the join row — Service.AssignShipment has
// already verified the shipment is READY, owned by tenantID, and not
// already assigned to another active trip before calling this.
func (r *Repository) CreateTripShipment(ctx context.Context, tenantID int64, ts *TripShipment) error {
	ts.TenantID = tenantID
	return r.db.WithContext(ctx).Create(ts).Error
}

// FindActiveTripIDForShipment returns the id of the non-CANCELLED trip
// (if any) this shipment is already assigned to — mirrors
// picking_batch_orders' own documented "no double-claim" application-
// layer check (migrations/0095/0110's own doc) exactly: a CANCELLED
// trip's own rows are deliberately excluded, so a shipment that was
// only ever assigned to a since-cancelled trip is free to join a new
// one. Returns (0, nil) if no active assignment exists.
func (r *Repository) FindActiveTripIDForShipment(ctx context.Context, tenantID, shipmentID int64) (int64, error) {
	var tripID int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT ts.trip_id FROM loading_trip_shipments ts
		JOIN loading_trips t ON t.id = ts.trip_id
		WHERE ts.tenant_id = ? AND ts.shipment_id = ? AND t.status <> 'CANCELLED'
	`, tenantID, shipmentID).Scan(&tripID).Error
	if err != nil {
		return 0, err
	}
	return tripID, nil
}

type tripShipmentRow struct {
	ShipmentID   int64
	ShipmentNo   string
	OrderNo      string
	Customer     *string
	PackingJobID int64
	JobNo        string
	PlannedCount int
	LoadedCount  int
	ShortCount   int
}

// ListTripShipments returns every shipment assigned to this trip with
// its own carton progress — used by GET /loading-trips/{id}.
func (r *Repository) ListTripShipments(ctx context.Context, tenantID, tripID int64) ([]ShipmentSummaryResponse, error) {
	var rows []tripShipmentRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS shipment_id, s.shipment_no AS shipment_no, pj.order_no AS order_no, pj.customer AS customer,
			s.packing_job_id AS packing_job_id, pj.job_no AS job_no,
			(SELECT COUNT(*) FROM loading_trip_cartons c WHERE c.trip_id = ts.trip_id AND c.shipment_id = s.id) AS planned_count,
			(SELECT COUNT(*) FROM loading_trip_cartons c WHERE c.trip_id = ts.trip_id AND c.shipment_id = s.id AND c.status = 'LOADED') AS loaded_count,
			(SELECT COUNT(*) FROM loading_trip_cartons c WHERE c.trip_id = ts.trip_id AND c.shipment_id = s.id AND c.status = 'SHORT') AS short_count
		FROM loading_trip_shipments ts
		JOIN shipments s ON s.id = ts.shipment_id
		JOIN packing_jobs pj ON pj.id = s.packing_job_id
		WHERE ts.tenant_id = ? AND ts.trip_id = ?
		ORDER BY s.id ASC
	`, tenantID, tripID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]ShipmentSummaryResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, ShipmentSummaryResponse{
			ShipmentID: row.ShipmentID, ShipmentNo: row.ShipmentNo, OrderNo: row.OrderNo, Customer: row.Customer,
			PackingJobID: row.PackingJobID, JobNo: row.JobNo, PlannedCount: row.PlannedCount,
			LoadedCount: row.LoadedCount, ShortCount: row.ShortCount,
		})
	}
	return out, nil
}

// CreateTripCarton inserts one scan-to-load worklist row —
// Service.AssignShipment calls this once per carton belonging to the
// shipment's own packing job, immediately after CreateTripShipment, in
// the same transaction.
func (r *Repository) CreateTripCarton(ctx context.Context, tenantID int64, tc *TripCarton) error {
	tc.TenantID = tenantID
	return r.db.WithContext(ctx).Create(tc).Error
}

type tripCartonRow struct {
	ID           int64
	CartonID     int64
	LPN          string
	ShipmentID   int64
	ShipmentNo   string
	Status       string
	LoadedAt     *time.Time
	LoadedBy     *int64
	LoadedByName *string
	ShortReason  *string
	ShortRemark  *string
	CreatedAt    time.Time
}

// ListTripCartons returns the full scan-to-load worklist for a trip —
// used by GET /loading-trips/{id} and by Service's own "every carton
// resolved" readiness check ahead of ConfirmTrip.
func (r *Repository) ListTripCartons(ctx context.Context, tenantID, tripID int64) ([]CartonResponse, error) {
	var rows []tripCartonRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT tc.id AS id, tc.carton_id AS carton_id, c.lpn AS lpn, tc.shipment_id AS shipment_id,
			s.shipment_no AS shipment_no, tc.status AS status, tc.loaded_at AS loaded_at, tc.loaded_by AS loaded_by,
			u.full_name AS loaded_by_name, tc.short_reason AS short_reason, tc.short_remark AS short_remark,
			tc.created_at AS created_at
		FROM loading_trip_cartons tc
		JOIN cartons c ON c.id = tc.carton_id
		JOIN shipments s ON s.id = tc.shipment_id
		LEFT JOIN users u ON u.id = tc.loaded_by
		WHERE tc.tenant_id = ? AND tc.trip_id = ?
		ORDER BY tc.id ASC
	`, tenantID, tripID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]CartonResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, CartonResponse{
			ID: row.ID, CartonID: row.CartonID, LPN: row.LPN, ShipmentID: row.ShipmentID, ShipmentNo: row.ShipmentNo,
			Status: CartonStatus(row.Status), LoadedAt: row.LoadedAt, LoadedBy: row.LoadedBy,
			LoadedByName: row.LoadedByName, ShortReason: row.ShortReason, ShortRemark: row.ShortRemark,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

// FindCartonByLPN resolves packing's own cartons table by LPN, scoped
// to tenantID — a read-only lookup of packing's own exported Carton GORM
// model (the same cross-package "query another module's own table
// directly via its exported model/raw SQL" pattern internal/shipping's
// own FindDetail/Sources already use against packing_jobs/cartons).
// Loading never writes to this table.
func (r *Repository) FindCartonByLPN(ctx context.Context, tenantID int64, lpn string) (*packing.Carton, error) {
	var c packing.Carton
	err := r.db.WithContext(ctx).First(&c, "lpn = ? AND tenant_id = ?", lpn, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCartonNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// FindTripCarton looks up this trip's own worklist row for a given
// carton — scoped to BOTH tripID and tenantID, so a carton that belongs
// to a different trip (or was never planned for any trip at all) is
// rejected as ErrNotFound exactly like a nonexistent one, which Service
// turns into the "wrong-trip loading" / "not planned" error.
func (r *Repository) FindTripCarton(ctx context.Context, tenantID, tripID, cartonID int64) (*TripCarton, error) {
	var tc TripCarton
	err := r.db.WithContext(ctx).First(&tc, "trip_id = ? AND carton_id = ? AND tenant_id = ?", tripID, cartonID, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tc, nil
}

// MarkCartonLoaded is the atomic guarded UPDATE that actually resolves a
// carton as LOADED — the WHERE status = 'PLANNED' predicate is what
// makes two concurrent scans for the very same carton race-safe: SQL
// Server applies each UPDATE statement atomically, so at most one of two
// concurrent callers can ever see rows-affected = 1; the loser sees 0
// and Service reports ErrCartonAlreadyLoaded. Mirrors packing's own
// AdjustPackedQty guarded-UPDATE pattern (overpack prevention),
// specialized to a status column instead of a running quantity.
func (r *Repository) MarkCartonLoaded(ctx context.Context, tenantID, tripCartonID, userID int64) (int64, error) {
	result := r.db.WithContext(ctx).Model(&TripCarton{}).
		Where("id = ? AND tenant_id = ? AND status = ?", tripCartonID, tenantID, string(CartonStatusPlanned)).
		Updates(map[string]any{
			"status":    string(CartonStatusLoaded),
			"loaded_at": time.Now().UTC(),
			"loaded_by": userID,
		})
	return result.RowsAffected, result.Error
}

// MarkCartonShort is MarkCartonLoaded's sibling for the short-loading
// exception — same atomic-guarded-UPDATE race safety.
func (r *Repository) MarkCartonShort(ctx context.Context, tenantID, tripCartonID int64, reason string, remark *string) (int64, error) {
	result := r.db.WithContext(ctx).Model(&TripCarton{}).
		Where("id = ? AND tenant_id = ? AND status = ?", tripCartonID, tenantID, string(CartonStatusPlanned)).
		Updates(map[string]any{
			"status":       string(CartonStatusShort),
			"short_reason": reason,
			"short_remark": remark,
		})
	return result.RowsAffected, result.Error
}

// CountUnresolvedCartons returns how many of this trip's own planned
// cartons are still PLANNED (neither LOADED nor SHORT) — Service's own
// ConfirmTrip readiness gate requires this to be exactly 0, and also
// requires at least one carton to exist at all (an empty trip has
// nothing to confirm).
func (r *Repository) CountUnresolvedCartons(ctx context.Context, tenantID, tripID int64) (total, unresolved int64, err error) {
	if err = r.db.WithContext(ctx).Model(&TripCarton{}).
		Where("tenant_id = ? AND trip_id = ?", tenantID, tripID).Count(&total).Error; err != nil {
		return 0, 0, err
	}
	if err = r.db.WithContext(ctx).Model(&TripCarton{}).
		Where("tenant_id = ? AND trip_id = ? AND status = ?", tenantID, tripID, string(CartonStatusPlanned)).
		Count(&unresolved).Error; err != nil {
		return 0, 0, err
	}
	return total, unresolved, nil
}

type sourceRow struct {
	ShipmentID   int64
	ShipmentNo   string
	OrderNo      string
	Customer     *string
	PackingJobID int64
	JobNo        string
	CartonCount  int
}

// Sources returns READY shipments that are not yet assigned to any
// active (non-CANCELLED) trip — powers GET /loading-trips/sources, the
// "assign shipment to trip" picker. Scoped to tenantID throughout.
func (r *Repository) Sources(ctx context.Context, tenantID int64) ([]SourceResponse, error) {
	var rows []sourceRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS shipment_id, s.shipment_no AS shipment_no, pj.order_no AS order_no, pj.customer AS customer,
			s.packing_job_id AS packing_job_id, pj.job_no AS job_no,
			(SELECT COUNT(*) FROM cartons c WHERE c.packing_job_id = pj.id) AS carton_count
		FROM shipments s
		JOIN packing_jobs pj ON pj.id = s.packing_job_id
		WHERE s.tenant_id = ?
			AND s.status = 'READY'
			AND NOT EXISTS (
				SELECT 1 FROM loading_trip_shipments ts
				JOIN loading_trips t ON t.id = ts.trip_id
				WHERE ts.shipment_id = s.id AND t.status <> 'CANCELLED'
			)
		ORDER BY s.id DESC
	`, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]SourceResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, SourceResponse{
			ShipmentID: row.ShipmentID, ShipmentNo: row.ShipmentNo, OrderNo: row.OrderNo, Customer: row.Customer,
			PackingJobID: row.PackingJobID, JobNo: row.JobNo, CartonCount: row.CartonCount,
		})
	}
	return out, nil
}
