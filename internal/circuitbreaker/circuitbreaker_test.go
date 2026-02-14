package circuitbreaker

import (
	"testing"
	"time"
)

func TestCircuitBreaker_StartsAsClosed(t *testing.T) {
	cb := New("test", 3, 2, 100*time.Millisecond)
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed, got %s", cb.State())
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := New("test", 3, 2, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed after 2 failures, got %s", cb.State())
	}

	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen after 3 failures, got %s", cb.State())
	}
}

func TestCircuitBreaker_DeniesInOpenState(t *testing.T) {
	cb := New("test", 2, 2, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen, got %s", cb.State())
	}

	if cb.Allow() {
		t.Fatal("expected Allow() to return false in Open state")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := New("test", 2, 2, 50*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen, got %s", cb.State())
	}

	time.Sleep(60 * time.Millisecond)

	if !cb.Allow() {
		t.Fatal("expected Allow() to return true after timeout")
	}
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %s", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterSuccesses(t *testing.T) {
	cb := New("test", 2, 2, 50*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)
	cb.Allow() // transition to HalfOpen

	cb.RecordSuccess()
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen after 1 success, got %s", cb.State())
	}

	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed after 2 successes, got %s", cb.State())
	}
}

func TestCircuitBreaker_ReopensOnHalfOpenFailure(t *testing.T) {
	cb := New("test", 2, 2, 50*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)
	cb.Allow() // transition to HalfOpen

	if cb.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %s", cb.State())
	}

	// failureCount is still 2 (not reset on Open→HalfOpen transition),
	// so one more failure meets the threshold and reopens the circuit.
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen after failure in HalfOpen, got %s", cb.State())
	}
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	cb := New("test-cb", 2, 1, 50*time.Millisecond)

	var transitions []struct{ from, to State }
	cb.SetOnStateChange(func(name string, from, to State) {
		if name != "test-cb" {
			t.Errorf("expected name 'test-cb', got %q", name)
		}
		transitions = append(transitions, struct{ from, to State }{from, to})
	})

	// Closed -> Open
	cb.RecordFailure()
	cb.RecordFailure()

	// Open -> HalfOpen
	time.Sleep(60 * time.Millisecond)
	cb.Allow()

	// HalfOpen -> Closed
	cb.RecordSuccess()

	if len(transitions) != 3 {
		t.Fatalf("expected 3 transitions, got %d", len(transitions))
	}

	expected := []struct{ from, to State }{
		{StateClosed, StateOpen},
		{StateOpen, StateHalfOpen},
		{StateHalfOpen, StateClosed},
	}
	for i, e := range expected {
		if transitions[i].from != e.from || transitions[i].to != e.to {
			t.Errorf("transition %d: expected %s->%s, got %s->%s",
				i, e.from, e.to, transitions[i].from, transitions[i].to)
		}
	}
}

func TestCircuitBreaker_SuccessResetFailureCount(t *testing.T) {
	cb := New("test", 3, 2, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	// 2 failures, below threshold of 3

	cb.RecordSuccess() // should reset failureCount to 0

	cb.RecordFailure()
	cb.RecordFailure()
	// only 2 failures again after reset, should still be closed
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed after success reset, got %s", cb.State())
	}

	cb.RecordFailure()
	// now 3 failures, should open
	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen after 3 consecutive failures, got %s", cb.State())
	}
}
