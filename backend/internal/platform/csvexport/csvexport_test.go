package csvexport

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteTo(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTo(&buf, []string{"a", "b"}, [][]string{{"1", "2"}, {"3", "4"}})
	if err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "a,b") || !strings.Contains(got, "1,2") || !strings.Contains(got, "3,4") {
		t.Errorf("WriteTo() output = %q, missing expected rows", got)
	}
}

func TestSafeFilename(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"already safe", "receiving_history_20260101.csv", "receiving_history_20260101.csv"},
		{"spaces replaced", "my report.csv", "my_report.csv"},
		{"path traversal stripped", "../../etc/passwd", ".._.._etc_passwd"},
		{"empty falls back", "", "export.csv"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SafeFilename(tc.input); got != tc.want {
				t.Errorf("SafeFilename(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatFloat(t *testing.T) {
	cases := map[float64]string{
		100:    "100",
		10.5:   "10.5",
		0:      "0",
		-5.25:  "-5.25",
		1000.1: "1000.1",
	}
	for in, want := range cases {
		if got := FormatFloat(in); got != want {
			t.Errorf("FormatFloat(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	if got := FormatTime(nil); got != "" {
		t.Errorf("FormatTime(nil) = %q, want empty string", got)
	}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := FormatTime(&when); got != "2026-01-02T03:04:05Z" {
		t.Errorf("FormatTime(&when) = %q, want 2026-01-02T03:04:05Z", got)
	}
}

func TestFormatString(t *testing.T) {
	if got := FormatString(nil); got != "" {
		t.Errorf("FormatString(nil) = %q, want empty string", got)
	}
	s := "hello"
	if got := FormatString(&s); got != "hello" {
		t.Errorf("FormatString(&s) = %q, want hello", got)
	}
}

func TestFormatID(t *testing.T) {
	if got := FormatID(nil); got != "" {
		t.Errorf("FormatID(nil) = %q, want empty string", got)
	}
	id := int64(42)
	if got := FormatID(&id); got != "42" {
		t.Errorf("FormatID(&id) = %q, want 42", got)
	}
}
