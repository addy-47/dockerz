package renderer

import "time"

// ProgressState tracks a single service's build progress
type ProgressState int

const (
	StateQueued   ProgressState = iota // Waiting to build
	StateBuilding                      // Build in progress
	StatePushing                       // Push in progress (after build)
	StateDone                          // Completed successfully
	StateFailed                        // Failed
)

// String returns a compact human-readable label for the state.
// All states use plain ASCII — no emoji — for clean terminal display.
func (s ProgressState) String() string {
	switch s {
	case StateQueued:
		return "queued"
	case StateBuilding:
		return "building"
	case StatePushing:
		return "pushing"
	case StateDone:
		return "done"
	case StateFailed:
		return "failed"
	default:
		return "?"
	}
}

// StatusIcon returns a single visual character for the state.
func (s ProgressState) StatusIcon() string {
	switch s {
	case StateQueued:
		return "·"
	case StateBuilding:
		return ">"
	case StatePushing:
		return "^"
	case StateDone:
		return "✓"
	case StateFailed:
		return "✗"
	default:
		return "?"
	}
}

// ServiceLine represents the accumulated result of one service for the final summary
type ServiceLine struct {
	Name      string
	Image     string
	Status    string // "success", "failed", "skipped"
	BuildTime time.Duration
	PushTime  time.Duration
}
