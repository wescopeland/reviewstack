package reviewer

import "fmt"

type State int

const (
	StatePending State = iota
	StateRunning
	StateDone
	StateFailed
	StateKilled
)

func (s State) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StateRunning:
		return "running"
	case StateDone:
		return "done"
	case StateFailed:
		return "failed"
	case StateKilled:
		return "killed"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

func ParseState(v string) (State, bool) {
	switch v {
	case "pending":
		return StatePending, true
	case "running":
		return StateRunning, true
	case "done":
		return StateDone, true
	case "failed":
		return StateFailed, true
	case "killed":
		return StateKilled, true
	default:
		return StatePending, false
	}
}

// Transition returns the next state after an event. Returns false if invalid.
func Transition(from State, event Event) (State, bool) {
	switch event {
	case EventStart:
		return StateRunning, from == StatePending
	case EventComplete:
		return StateDone, from == StateRunning
	case EventFail:
		return StateFailed, from == StateRunning || from == StatePending
	case EventKill:
		return StateKilled, from == StateRunning || from == StatePending
	case EventReset:
		return StatePending, from == StateDone || from == StateFailed || from == StateKilled
	default:
		return from, false
	}
}

type Event int

const (
	EventStart Event = iota
	EventComplete
	EventFail
	EventKill
	EventReset
)
