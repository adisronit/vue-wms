package sqlerr

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mssql "github.com/microsoft/go-mssqldb"
)

func TestIsUniqueViolation(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"primary key violation (2627)":  {mssql.Error{Number: 2627, Message: "Violation of PRIMARY KEY constraint"}, true},
		"unique key violation (2627)":   {mssql.Error{Number: 2627, Message: "Violation of UNIQUE KEY constraint"}, true},
		"duplicate key on index (2601)": {mssql.Error{Number: 2601, Message: "Cannot insert duplicate key row"}, true},
		"fk violation is not unique":    {mssql.Error{Number: 547, Message: "FOREIGN KEY constraint"}, false},
		"non-sql error":                 {errors.New("connection reset"), false},
		"wrapped sql error":             {fmt.Errorf("query failed: %w", mssql.Error{Number: 2627}), true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsUniqueViolation(tc.err); got != tc.want {
				t.Errorf("IsUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsForeignKeyViolation(t *testing.T) {
	if !IsForeignKeyViolation(mssql.Error{Number: 547}) {
		t.Error("expected 547 to be classified as a foreign key violation")
	}
	if IsForeignKeyViolation(mssql.Error{Number: 2627}) {
		t.Error("expected 2627 (unique violation) not to be classified as a foreign key violation")
	}
}

func TestIsDeadlock(t *testing.T) {
	if !IsDeadlock(mssql.Error{Number: 1205}) {
		t.Error("expected 1205 to be classified as a deadlock")
	}
	if IsDeadlock(errors.New("some other error")) {
		t.Error("expected a non-SQL error not to be classified as a deadlock")
	}
}

func TestIsCancelled(t *testing.T) {
	if !IsCancelled(context.Canceled) {
		t.Error("expected context.Canceled to be classified as cancelled")
	}
	if !IsCancelled(context.DeadlineExceeded) {
		t.Error("expected context.DeadlineExceeded to be classified as cancelled")
	}
	if !IsCancelled(fmt.Errorf("query failed: %w", context.DeadlineExceeded)) {
		t.Error("expected a wrapped context.DeadlineExceeded to be classified as cancelled")
	}
	if IsCancelled(mssql.Error{Number: 2627}) {
		t.Error("expected a genuine SQL error not to be classified as cancelled")
	}
}

func TestAsSQLError(t *testing.T) {
	sqlErr, ok := AsSQLError(mssql.Error{Number: 2627, Message: "test"})
	if !ok {
		t.Fatal("AsSQLError should succeed for a real mssql.Error")
	}
	if sqlErr.Number != 2627 {
		t.Errorf("Number = %d, want 2627", sqlErr.Number)
	}

	if _, ok := AsSQLError(errors.New("not a sql error")); ok {
		t.Error("AsSQLError should fail for a non-SQL error")
	}
}
