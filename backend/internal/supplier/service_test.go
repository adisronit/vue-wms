package supplier

import "testing"

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusActive: true, StatusInactive: true,
		"NOT_REAL": false, "": false,
	}
	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}
