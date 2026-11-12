package dbctx

import (
	"context"
	"testing"
	"time"
)

func TestDetach_SurvivesParentCancellation(t *testing.T) {
	SetDefaultTimeout(50 * time.Millisecond)
	defer SetDefaultTimeout(0)

	parent, cancelParent := context.WithCancel(context.Background())
	detached, cancel := Detach(parent)
	defer cancel()

	cancelParent()

	select {
	case <-detached.Done():
		t.Fatal("detached context was cancelled when its parent was cancelled — Detach should be immune to parent cancellation")
	case <-time.After(10 * time.Millisecond):
		// still alive, as expected
	}
}

func TestDetach_HasItsOwnBound(t *testing.T) {
	SetDefaultTimeout(20 * time.Millisecond)
	defer SetDefaultTimeout(0)

	detached, cancel := Detach(context.Background())
	defer cancel()

	select {
	case <-detached.Done():
		if detached.Err() != context.DeadlineExceeded {
			t.Fatalf("Err() = %v, want context.DeadlineExceeded", detached.Err())
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("detached context never expired — Detach should apply its own bounded timeout")
	}
}

func TestDetach_FallsBackWhenUnconfigured(t *testing.T) {
	SetDefaultTimeout(0)

	if got := currentDefault(); got != fallbackTimeout {
		t.Errorf("currentDefault() = %v, want fallback %v when never configured", got, fallbackTimeout)
	}
}

func TestDetach_PreservesContextValues(t *testing.T) {
	SetDefaultTimeout(time.Second)
	defer SetDefaultTimeout(0)

	parent := WithRequestID(context.Background(), "req-123")
	detached, cancel := Detach(parent)
	defer cancel()

	if got := RequestIDFromContext(detached); got != "req-123" {
		t.Errorf("RequestIDFromContext(detached) = %q, want %q — Detach must preserve context values, only strip cancellation", got, "req-123")
	}
}

func TestWithRequestID_EmptyIsNoop(t *testing.T) {
	ctx := WithRequestID(context.Background(), "")
	if got := RequestIDFromContext(ctx); got != "" {
		t.Errorf("RequestIDFromContext = %q, want empty for an empty input id", got)
	}
}

func TestRequestIDFromContext_UnsetReturnsEmpty(t *testing.T) {
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Errorf("RequestIDFromContext(plain context) = %q, want empty", got)
	}
}
