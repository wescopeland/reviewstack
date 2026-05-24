package reviewer_test

import (
	"testing"

	"github.com/wescopeland/reviewstack/internal/reviewer"
)

func TestTransitionStart(t *testing.T) {
	next, ok := reviewer.Transition(reviewer.StatePending, reviewer.EventStart)
	if !ok || next != reviewer.StateRunning {
		t.Fatalf("Transition(start) = (%v, %v), want (running, true)", next, ok)
	}
}

func TestTransitionComplete(t *testing.T) {
	next, ok := reviewer.Transition(reviewer.StateRunning, reviewer.EventComplete)
	if !ok || next != reviewer.StateDone {
		t.Fatalf("Transition(complete) = (%v, %v)", next, ok)
	}
}

func TestTransitionFailFromRunning(t *testing.T) {
	next, ok := reviewer.Transition(reviewer.StateRunning, reviewer.EventFail)
	if !ok || next != reviewer.StateFailed {
		t.Fatalf("Transition(fail) = (%v, %v)", next, ok)
	}
}

func TestTransitionKill(t *testing.T) {
	next, ok := reviewer.Transition(reviewer.StateRunning, reviewer.EventKill)
	if !ok || next != reviewer.StateKilled {
		t.Fatalf("Transition(kill) = (%v, %v)", next, ok)
	}
}

func TestTransitionReset(t *testing.T) {
	for _, from := range []reviewer.State{reviewer.StateDone, reviewer.StateFailed, reviewer.StateKilled} {
		next, ok := reviewer.Transition(from, reviewer.EventReset)
		if !ok || next != reviewer.StatePending {
			t.Fatalf("Transition(reset from %s) = (%v, %v)", from, next, ok)
		}
	}
}

func TestTransitionInvalid(t *testing.T) {
	_, ok := reviewer.Transition(reviewer.StateDone, reviewer.EventComplete)
	if ok {
		t.Fatal("expected invalid transition from done -> complete")
	}
}

func TestStateString(t *testing.T) {
	cases := map[reviewer.State]string{
		reviewer.StatePending: "pending",
		reviewer.StateRunning: "running",
		reviewer.StateDone:    "done",
		reviewer.StateFailed:  "failed",
		reviewer.StateKilled:  "killed",
	}
	for st, want := range cases {
		if st.String() != want {
			t.Fatalf("%v.String() = %q, want %q", st, st.String(), want)
		}
	}
}
