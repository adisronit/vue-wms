package inventory

import (
	"errors"
	"testing"
)

// validateStockChange and MoveStock's upfront checks run before any
// repository access, so they're testable without a database — the
// actual balance/ledger/rollback behavior (which needs real SQL Server
// transactional semantics) was verified live; see PROJECT_STATUS.md.

func TestValidateStockChange_RequiresProductAndLocation(t *testing.T) {
	err := validateStockChange(StockChangeRequest{Quantity: 1, Type: TransactionAdjust})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateStockChange_RejectsZeroQuantity(t *testing.T) {
	err := validateStockChange(StockChangeRequest{ProductID: 1, LocationID: 1, Quantity: 0, Type: TransactionAdjust})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateStockChange_RejectsNegativeQuantity(t *testing.T) {
	err := validateStockChange(StockChangeRequest{ProductID: 1, LocationID: 1, Quantity: -5, Type: TransactionAdjust})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateStockChange_RejectsInvalidType(t *testing.T) {
	err := validateStockChange(StockChangeRequest{ProductID: 1, LocationID: 1, Quantity: 1, Type: "NOT_REAL"})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateStockChange_AcceptsValidRequest(t *testing.T) {
	err := validateStockChange(StockChangeRequest{ProductID: 1, LocationID: 1, Quantity: 10, Type: TransactionReceive})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMoveStock_RejectsSameSourceAndDestination(t *testing.T) {
	svc := &Service{}

	err := svc.MoveStock(nil, MoveStockRequest{
		ProductID: 1, FromLocationID: 5, ToLocationID: 5, Quantity: 1,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestMoveStock_RejectsZeroQuantity(t *testing.T) {
	svc := &Service{}

	err := svc.MoveStock(nil, MoveStockRequest{
		ProductID: 1, FromLocationID: 5, ToLocationID: 6, Quantity: 0,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestMoveStock_RejectsMissingIDs(t *testing.T) {
	svc := &Service{}

	err := svc.MoveStock(nil, MoveStockRequest{Quantity: 1})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestTransactionType_IsValid(t *testing.T) {
	cases := map[TransactionType]bool{
		TransactionReceive: true, TransactionPutaway: true, TransactionMove: true,
		TransactionPick: true, TransactionShip: true, TransactionAdjust: true,
		TransactionReturn: true, TransactionCount: true,
		"NOT_REAL": false, "": false,
	}
	for typ, want := range cases {
		if got := typ.IsValid(); got != want {
			t.Errorf("TransactionType(%q).IsValid() = %v, want %v", typ, got, want)
		}
	}
}

func TestInventory_Available(t *testing.T) {
	inv := Inventory{Quantity: 140, ReservedQuantity: 20}
	if got := inv.Available(); got != 120 {
		t.Errorf("Available() = %v, want 120", got)
	}
}

// validateReserveRequest runs before any repository access, so it's
// testable without a database — same rationale as validateStockChange's
// tests above. The actual guarded-UPDATE concurrency behavior of
// ReserveStockTx/ReleaseReservationTx/ConsumeReservationTx (which needs
// real SQL Server transactional semantics) was verified live; see
// PROJECT_STATUS.md's Phase 12 section.

func TestValidateReserveRequest_RequiresProductAndLocation(t *testing.T) {
	err := validateReserveRequest(ReserveRequest{Quantity: 1})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateReserveRequest_RejectsZeroQuantity(t *testing.T) {
	err := validateReserveRequest(ReserveRequest{ProductID: 1, LocationID: 1, Quantity: 0})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateReserveRequest_RejectsNegativeQuantity(t *testing.T) {
	err := validateReserveRequest(ReserveRequest{ProductID: 1, LocationID: 1, Quantity: -5})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateReserveRequest_AcceptsValidRequest(t *testing.T) {
	err := validateReserveRequest(ReserveRequest{ProductID: 1, LocationID: 1, Quantity: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReserveStockTx_RejectsInvalidRequest(t *testing.T) {
	svc := &Service{}

	_, err := svc.ReserveStockTx(nil, nil, ReserveRequest{Quantity: 1})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestReleaseReservationTx_RejectsInvalidRequest(t *testing.T) {
	svc := &Service{}

	_, err := svc.ReleaseReservationTx(nil, nil, ReserveRequest{Quantity: 1})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestConsumeReservationTx_RejectsInvalidRequest(t *testing.T) {
	svc := &Service{}

	_, err := svc.ConsumeReservationTx(nil, nil, StockChangeRequest{ProductID: 1, LocationID: 1, Quantity: 0, Type: TransactionPick})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}
