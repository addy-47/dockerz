package builder

import (
	"time"

	"github.com/addy-47/dockerz/internal/config"
)

// ProgressCallback is called for each line of Docker build --progress=plain
// output when a live status display is active.
type ProgressCallback func(serviceName, line string)

// BuildTask represents a single build task
type BuildTask struct {
	ServicePath  string
	ServiceName  string           // YAML name field (used for display identification)
	ImageName    string           // Docker image name (may differ from ServiceName)
	Tag          string
	Config       *config.Config
	CurrentHash  string
	ChangedFiles []string
	NeedsBuild   bool
	Quiet        bool             // Suppress verbose output (used with progress display)
	ProgressCb   ProgressCallback // called for each Docker output line (Quiet mode)
}

// BuildResult represents the result of a build operation
type BuildResult struct {
	Service     string    `json:"service"`
	ServiceName string    `json:"service_name,omitempty"` // YAML name field (for display)
	ImageName   string    `json:"image_name,omitempty"`   // Docker image name
	Image       string    `json:"image"`
	Status      string    `json:"status"`
	BuildOutput string    `json:"build_output,omitempty"`
	PushStatus  string    `json:"push_status,omitempty"`
	PushOutput  string    `json:"push_output,omitempty"`
	StartTime   time.Time `json:"-"`
	EndTime     time.Time `json:"-"`
}

// Summary represents the build summary
type Summary struct {
	TotalServices    int
	SuccessfulBuilds int
	FailedBuilds     int
	FailedPushes     int
	Duration         time.Duration
}
