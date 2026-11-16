package loading

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/packing"
	"wms-backend/internal/platform/database/sqlerr"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/shipping"
)

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) — the same pattern
// internal/shipping's own repository.go uses.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrShipmentNotFound = errors.New("loading: shipment not found")
	// ErrShipmentNotReady covers a shipment that is not currently READY
	// — either not yet reached READY, or already dispatched/delivered/
	// issue-flagged/cancelled. Only a READY shipment may be assigned to
	// a trip.
	ErrShipmentNotReady = errors.New("loading: shipment is not READY")
	// ErrShipmentAlreadyAssigned covers a shipment already assigned to
	// another ACTIVE (non-CANCELLED) trip — see
	// Repository.FindActiveTripIDForShipment's own doc on the "no
	// double-claim" shape this mirrors from picking_batch_orders.
	ErrShipmentAlreadyAssigned = errors.New("loading: shipment is already assigned to an active loading trip")
	ErrPackingJobNotFound      = errors.New("loading: packing job not found")
	// ErrNoCartons covers a packing job with zero cartons at all — see
	// package doc: in practice unreachable once a shipment exists at
	// all (Shipping.Create itself requires at least one packed line,
	// which requires at least one carton), but checked explicitly
	// rather than silently creating an empty, un-confirmable trip
	// assignment.
	ErrNoCartons         = errors.New("loading: shipment's packing job has no cartons to load")
	ErrInvalidTransition = errors.New("loading: trip is not in a state that allows this action")
	// ErrCartonNotPlannedForTrip covers BOTH "this carton belongs to a
	// different trip" and "this carton was never planned for any trip
	// at all" — Repository.FindTripCarton scopes its lookup to
	// (trip_id, carton_id, tenant_id) together, so either case surfaces
	// identically, the same "wrong-trip loading" prevention this
	// phase's own instructions ask for.
	ErrCartonNotPlannedForTrip = errors.New("loading: carton is not planned for this trip")
	// ErrCartonAlreadyLoaded is the duplicate-scan-prevention error —
	// returned instead of a silent no-op, per this phase's own
	// instructions.
	ErrCartonAlreadyLoaded = errors.New("loading: carton has already been scanned loaded on this trip")
	// ErrCartonAlreadyResolved covers re-scanning a carton that was
	// already flagged SHORT — also terminal, also rejected rather than
	// silently re-resolved.
	ErrCartonAlreadyResolved = errors.New("loading: carton has already been resolved (SHORT) on this trip")
	// ErrNotReadyToConfirm covers the trip-level confirm readiness gate
	// — see Service.ConfirmTrip's own doc for the exact rule.
	ErrNotReadyToConfirm = errors.New("loading: every planned carton on this trip must be LOADED or flagged SHORT before it can be confirmed")
)

// shippingChecker is the narrow slice of internal/shipping's own
// Service this package needs — satisfied structurally by
// *shipping.Service, the same narrow-interface pattern internal/
// packing's own pickingChecker and internal/shipping's own
// packingChecker already established. Get is read-only (resolves a
// shipment's own Status/PackingJobID); MarkLoaded is the one and only
// write this package ever performs against internal/shipping's own
// data, and the one and only reason this package depends on that one
// at all (see package doc for the full dependency-direction
// reasoning). internal/loading never reads or writes any other
// shipping table/column directly.
type shippingChecker interface {
	Get(ctx context.Context, tenantID, id int64) (*shipping.Response, error)
	MarkLoaded(ctx context.Context, tenantID, id, userID int64) error
}

// packingChecker is the narrow slice of internal/packing's own Service
// this package needs — read-only, to resolve a shipment's own packing
// job and its cartons (CartonResponse.LPN/Status) when a shipment is
// first assigned to a trip. Satisfied structurally by *packing.Service.
type packingChecker interface {
	Get(ctx context.Context, tenantID, id int64) (*packing.Response, error)
}

// Service holds the Loading workflow's business rules. It deliberately
// has NO dependency on internal/inventory at all (grepped to confirm)
// — this package never mutates a stock balance; it is a pure physical-
// handling/traceability record of which cartons were loaded onto which
// vehicle. See package doc for the full one-way dependency shape.
type Service struct {
	repo    *Repository
	shippng shippingChecker
	packing packingChecker
}

func NewService(repo *Repository, shippingSvc shippingChecker, packingSvc packingChecker) *Service {
	return &Service{repo: repo, shippng: shippingSvc, packing: packingSvc}
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

// Sources returns READY shipments not yet assigned to any active trip
// — see Repository.Sources' own doc.
func (s *Service) Sources(ctx context.Context, tenantID int64) ([]SourceResponse, error) {
	return s.repo.Sources(ctx, tenantID)
}

// Get assembles the full trip detail — header, assigned shipments (with
// their own carton progress), and the full carton worklist — for GET
// /loading-trips/{id}.
func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	header, err := s.repo.FindDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	shipments, err := s.repo.ListTripShipments(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	cartons, err := s.repo.ListTripCartons(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	loaded, short := 0, 0
	for _, c := range cartons {
		switch c.Status {
		case CartonStatusLoaded:
			loaded++
		case CartonStatusShort:
			short++
		}
	}

	return &Response{
		ID: header.ID, TripNo: header.TripNo, VehicleNo: header.VehicleNo, DriverName: header.DriverName,
		Carrier: header.Carrier, LoadingDock: header.LoadingDock, Route: header.Route,
		ScheduledLoadingTime: header.ScheduledLoadingTime, ActualLoadingStartAt: header.ActualLoadingStartAt,
		ActualLoadingEndAt: header.ActualLoadingEndAt, VehicleSeal: header.VehicleSeal, Status: Status(header.Status),
		Remark: header.Remark, PlannedCount: len(cartons), LoadedCount: loaded, ShortCount: short,
		Shipments: shipments, Cartons: cartons,
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt, CompletedAt: header.CompletedAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
	}, nil
}

// CreateTrip opens a new trip header — every field is optional (see
// CreateRequest's own doc); the trip exists as a real, assignable
// PLANNED trip as soon as it has an id/trip_no.
func (s *Service) CreateTrip(ctx context.Context, tenantID int64, req CreateRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var tripID int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		trip := &Trip{
			VehicleNo: req.VehicleNo, DriverName: req.DriverName, Carrier: req.Carrier,
			LoadingDock: req.LoadingDock, Route: req.Route, ScheduledLoadingTime: req.ScheduledLoadingTime,
			VehicleSeal: req.VehicleSeal, Status: StatusPlanned, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateTrip(ctx, tenantID, trip); err != nil {
			return err
		}
		tripID = trip.ID
		return nil
	})
	dbctx.LogTransaction(ctx, "loading.create_trip", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, tripID)
}

// AssignShipment adds a READY shipment to a trip and auto-populates the
// trip's own scan-to-load worklist (loading_trip_cartons) from that
// shipment's own packing job's own cartons — "the cartons belonging to
// a shipment" is, as of Phase 33D.1, exactly "the cartons belonging to
// that shipment's own packing_job_id" (see package doc).
//
// Validates: the trip is in an assignable status (CanAssignShipment),
// the shipment exists and belongs to tenantID (via shippingChecker.Get
// — a cross-tenant id is rejected as ErrShipmentNotFound exactly like a
// nonexistent one, never revealing whether the id exists in another
// tenant), the shipment is READY (ErrShipmentNotReady — already
// dispatched/loaded, or not yet packed/shipped, cannot be assigned),
// and the shipment is not already assigned to another ACTIVE trip
// (ErrShipmentAlreadyAssigned — see Repository.
// FindActiveTripIDForShipment's own doc). The shipment's own packing
// job is then resolved via packingChecker.Get and every one of its
// cartons becomes one new PLANNED loading_trip_cartons row.
func (s *Service) AssignShipment(ctx context.Context, tenantID, tripID int64, req AssignShipmentRequest, userID int64) (*Response, error) {
	trip, err := s.repo.FindTripByID(ctx, tenantID, tripID)
	if err != nil {
		return nil, err
	}
	if !trip.Status.CanAssignShipment() {
		return nil, ErrInvalidTransition
	}

	shipment, err := s.shippng.Get(ctx, tenantID, req.ShipmentID)
	if err != nil {
		return nil, ErrShipmentNotFound
	}
	if shipment.Status != shipping.StatusReady {
		return nil, ErrShipmentNotReady
	}

	activeTripID, err := s.repo.FindActiveTripIDForShipment(ctx, tenantID, req.ShipmentID)
	if err != nil {
		return nil, err
	}
	if activeTripID != 0 {
		return nil, ErrShipmentAlreadyAssigned
	}

	job, err := s.packing.Get(ctx, tenantID, shipment.PackingJobID)
	if err != nil {
		return nil, ErrPackingJobNotFound
	}
	if len(job.Cartons) == 0 {
		return nil, ErrNoCartons
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		if err := txRepo.CreateTripShipment(ctx, tenantID, &TripShipment{
			TripID: tripID, ShipmentID: req.ShipmentID,
		}); err != nil {
			return err
		}
		for _, carton := range job.Cartons {
			if err := txRepo.CreateTripCarton(ctx, tenantID, &TripCarton{
				TripID: tripID, CartonID: carton.ID, ShipmentID: req.ShipmentID, Status: CartonStatusPlanned,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "loading.assign_shipment", start, err)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrShipmentAlreadyAssigned
		}
		return nil, err
	}
	return s.Get(ctx, tenantID, tripID)
}

// ScanCarton is the ordinary "scan this carton as loaded" action.
// Resolves the scanned LPN against packing's own cartons table (tenant-
// scoped), then requires THIS trip to already have a PLANNED worklist
// row for that carton — a carton belonging to a different trip, or
// never planned for any trip at all, is rejected as
// ErrCartonNotPlannedForTrip (the "wrong-trip loading" prevention this
// phase's own instructions ask for). A carton already LOADED is
// rejected as ErrCartonAlreadyLoaded (duplicate-scan prevention,
// returned rather than silently no-op'd); already SHORT is rejected as
// ErrCartonAlreadyResolved. The actual status flip is an atomic guarded
// UPDATE (Repository.MarkCartonLoaded) so two concurrent scans for the
// same carton can never both succeed — see that method's own doc.
func (s *Service) ScanCarton(ctx context.Context, tenantID, tripID int64, req ScanCartonRequest, userID int64) (*Response, error) {
	trip, err := s.repo.FindTripByID(ctx, tenantID, tripID)
	if err != nil {
		return nil, err
	}
	if !trip.Status.CanScan() {
		return nil, ErrInvalidTransition
	}

	carton, err := s.repo.FindCartonByLPN(ctx, tenantID, req.LPN)
	if err != nil {
		return nil, err
	}

	tc, err := s.repo.FindTripCarton(ctx, tenantID, tripID, carton.ID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrCartonNotPlannedForTrip
	}
	if err != nil {
		return nil, err
	}
	switch tc.Status {
	case CartonStatusLoaded:
		return nil, ErrCartonAlreadyLoaded
	case CartonStatusShort:
		return nil, ErrCartonAlreadyResolved
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		rows, err := txRepo.MarkCartonLoaded(ctx, tenantID, tc.ID, userID)
		if err != nil {
			return err
		}
		if rows == 0 {
			// Lost a race against a concurrent scan/short-flag for the
			// very same carton — re-fetch to report the real, now-
			// current reason rather than guessing.
			latest, ferr := txRepo.FindTripCarton(ctx, tenantID, tripID, carton.ID)
			if ferr != nil {
				return ferr
			}
			if latest.Status == CartonStatusShort {
				return ErrCartonAlreadyResolved
			}
			return ErrCartonAlreadyLoaded
		}
		return s.transitionToLoadingIfNeeded(ctx, txRepo, tenantID, trip)
	})
	dbctx.LogTransaction(ctx, "loading.scan_carton", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, tripID)
}

// ShortCarton is ScanCarton's sibling for the "unable to load"
// exception (damaged/missing) — same wrong-trip/not-planned/already-
// resolved guards, same atomic-guarded-UPDATE race safety
// (Repository.MarkCartonShort).
func (s *Service) ShortCarton(ctx context.Context, tenantID, tripID int64, req ShortCartonRequest, userID int64) (*Response, error) {
	if !ShortReason(req.Reason).IsValid() {
		return nil, ErrValidation{Message: "reason must be one of DAMAGED, MISSING, OTHER"}
	}

	trip, err := s.repo.FindTripByID(ctx, tenantID, tripID)
	if err != nil {
		return nil, err
	}
	if !trip.Status.CanScan() {
		return nil, ErrInvalidTransition
	}

	carton, err := s.repo.FindCartonByLPN(ctx, tenantID, req.LPN)
	if err != nil {
		return nil, err
	}

	tc, err := s.repo.FindTripCarton(ctx, tenantID, tripID, carton.ID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrCartonNotPlannedForTrip
	}
	if err != nil {
		return nil, err
	}
	switch tc.Status {
	case CartonStatusLoaded:
		return nil, ErrCartonAlreadyLoaded
	case CartonStatusShort:
		return nil, ErrCartonAlreadyResolved
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		rows, err := txRepo.MarkCartonShort(ctx, tenantID, tc.ID, req.Reason, req.Remark)
		if err != nil {
			return err
		}
		if rows == 0 {
			latest, ferr := txRepo.FindTripCarton(ctx, tenantID, tripID, carton.ID)
			if ferr != nil {
				return ferr
			}
			if latest.Status == CartonStatusLoaded {
				return ErrCartonAlreadyLoaded
			}
			return ErrCartonAlreadyResolved
		}
		return s.transitionToLoadingIfNeeded(ctx, txRepo, tenantID, trip)
	})
	dbctx.LogTransaction(ctx, "loading.short_carton", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, tripID)
}

// transitionToLoadingIfNeeded flips a still-PLANNED trip to LOADING and
// stamps ActualLoadingStartAt, the first time any carton on it is
// resolved — called from inside ScanCarton/ShortCarton's own
// transaction, after their own carton-level UPDATE has already
// succeeded. A no-op once the trip has already moved past PLANNED. Uses
// the same CAS (UpdateTripStatusCAS) as every other trip-status write,
// so a concurrent Cancel racing the very first scan can never leave the
// trip in an inconsistent state — whichever one wins the CAS, the other
// simply sees 0 rows affected and is ignored here (the carton itself
// was already durably marked LOADED/SHORT either way; only the header's
// own derived status/timestamp could theoretically lag by one action,
// which the next scan/the eventual ConfirmTrip call corrects).
func (s *Service) transitionToLoadingIfNeeded(ctx context.Context, txRepo *Repository, tenantID int64, trip *Trip) error {
	if trip.Status != StatusPlanned {
		return nil
	}
	_, err := txRepo.TransitionToLoading(ctx, tenantID, trip.ID, time.Now().UTC())
	return err
}

// ConfirmTrip is the explicit completion action: requires the trip to
// already be LOADING (CanConfirm) AND every one of its own planned
// cartons to be resolved (LOADED or SHORT) — ties and ends are the
// same: a trip with any carton still PLANNED cannot be confirmed
// (ErrNotReadyToConfirm), and neither can a trip with zero cartons at
// all (also ErrNotReadyToConfirm — nothing to confirm).
//
// On success, calls shipping.Service.MarkLoaded for EVERY shipment
// assigned to this trip (the simpler, documented default this phase's
// own instructions recommend over a per-shipment variant) BEFORE
// committing the trip's own transition to COMPLETED — see the ordering
// note below for why.
//
// Known limitation (documented, not an outbox pattern — see
// CLAUDE.md's own guidance that the Outbox Pattern is for RabbitMQ
// message-loss risk, which does not apply here at all; this is a
// same-process, same-database, synchronous call): MarkLoaded is called
// once per shipment as a SEPARATE, already-committing call into
// internal/shipping's own Repository.Transaction, not nested inside
// this trip's own transaction (nesting would require sharing one
// *gorm.DB across two packages' own Repository types, which no
// existing module in this codebase does either). MarkLoaded calls run
// BEFORE this trip's own COMPLETED transition is committed specifically
// so that if any one of them fails partway through (e.g. a transient DB
// error), the trip is left exactly as it was (still LOADING, safely
// retryable) rather than showing COMPLETED while a shipment is not yet
// marked loaded. MarkLoaded itself is idempotent (re-stamping
// loaded_at/loaded_by is harmless), so retrying ConfirmTrip after a
// partial failure is always safe.
func (s *Service) ConfirmTrip(ctx context.Context, tenantID, tripID int64, req ConfirmRequest, userID int64) (*Response, error) {
	trip, err := s.repo.FindTripByID(ctx, tenantID, tripID)
	if err != nil {
		return nil, err
	}
	if !trip.Status.CanConfirm() {
		return nil, ErrInvalidTransition
	}

	total, unresolved, err := s.repo.CountUnresolvedCartons(ctx, tenantID, tripID)
	if err != nil {
		return nil, err
	}
	if total == 0 || unresolved > 0 {
		return nil, ErrNotReadyToConfirm
	}

	shipments, err := s.repo.ListTripShipments(ctx, tenantID, tripID)
	if err != nil {
		return nil, err
	}
	for _, sh := range shipments {
		if err := s.shippng.MarkLoaded(ctx, tenantID, sh.ShipmentID, userID); err != nil {
			return nil, err
		}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		rows, err := txRepo.CompleteTrip(ctx, tenantID, tripID, time.Now().UTC())
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrInvalidTransition
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "loading.confirm_trip", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, tripID)
}

// CancelTrip is allowed only before completion (Status.CanCancel) —
// mirrors every other module's own Cancel shape. Never calls
// shipping.Service.MarkLoaded — a cancelled trip's own shipments simply
// remain not-loaded (and therefore still blocked from dispatch), and
// remain free to be assigned to a brand new trip afterward (see
// Repository.FindActiveTripIDForShipment's own doc on why a CANCELLED
// trip's own loading_trip_shipments rows are excluded from the "already
// assigned" check).
func (s *Service) CancelTrip(ctx context.Context, tenantID, tripID int64, req CancelRequest, userID int64) (*Response, error) {
	trip, err := s.repo.FindTripByID(ctx, tenantID, tripID)
	if err != nil {
		return nil, err
	}
	if !trip.Status.CanCancel() {
		return nil, ErrInvalidTransition
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		rows, err := txRepo.CancelTripRow(ctx, tenantID, tripID, trip.Status, userID, req.Remark, time.Now().UTC())
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrInvalidTransition
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "loading.cancel_trip", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, tripID)
}
