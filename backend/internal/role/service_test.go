package role

import "testing"

// TestUniqueInt64s_DeduplicatesPreservingOrder covers
// SetRoleWarehouseAccess's dedup step: a client sending the same
// warehouse_id twice (a double-click, a stale re-submit) must not reach
// ReplaceRoleWarehouseAccess's INSERT as two rows for the same
// (role_id, warehouse_id) pair, which would violate
// PK_role_warehouse_access. Everything else SetRoleWarehouseAccess does
// (existence checks, the FindRoleByID lookup, the transactional
// replace) goes through the concrete *Repository and is verified live
// against real SQL Server instead, the same tradeoff every other
// service in this codebase makes (see PROJECT_STATUS.md).
func TestUniqueInt64s_DeduplicatesPreservingOrder(t *testing.T) {
	got := uniqueInt64s([]int64{3, 1, 3, 2, 1, 1})
	want := []int64{3, 1, 2}

	if len(got) != len(want) {
		t.Fatalf("uniqueInt64s(...) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("uniqueInt64s(...) = %v, want %v", got, want)
		}
	}
}

func TestUniqueInt64s_EmptyInput(t *testing.T) {
	got := uniqueInt64s(nil)
	if len(got) != 0 {
		t.Fatalf("uniqueInt64s(nil) = %v, want empty", got)
	}
}
