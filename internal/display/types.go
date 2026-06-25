package display

import "time"

// ServiceState tracks a single service's build progress.
type ServiceState int

const (
	StateQueued   ServiceState = iota // Waiting to build
	StateBuilding                     // Build in progress
	StatePushing                      // Push in progress (after build)
	StateDone                         // Completed successfully
	StateFailed                       // Failed
)

// ServiceStatus holds the live status of one service during a build.
type ServiceStatus struct {
	Name       string
	State      ServiceState
	DockerLine string        // latest --progress=plain output line (Building/Pushing only)
	Elapsed    time.Duration // time since current state began
	BuildTime  time.Duration // total build duration (for done services)
	PushTime   time.Duration // total push duration (for pushed services)
	Image      string
	Error      string
}

// StatusIcon returns a single visual character for the state.
func (s ServiceState) StatusIcon() string {
	switch s {
	case StateQueued:
		return "·"
	case StateBuilding:
		return "▸"
	case StatePushing:
		return "▴"
	case StateDone:
		return "✓"
	case StateFailed:
		return "✗"
	default:
		return "?"
	}
}

// Label returns a compact human-readable label for the state.
func (s ServiceState) Label() string {
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

// ServiceLine represents the accumulated result of one service for the final summary.
type ServiceLine struct {
	Name      string
	Image     string
	Status    string // "success", "failed", "skipped"
	BuildTime time.Duration
	PushTime  time.Duration
}
