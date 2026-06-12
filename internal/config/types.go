package config

import (
	"time"
)

// Service represents a service configuration
type Service struct {
	Name      string `yaml:"name" mapstructure:"name"`
	ImageName string `yaml:"image_name,omitempty" mapstructure:"image_name"`
	Tag       string `yaml:"tag,omitempty" mapstructure:"tag"`
}

// Config represents the main configuration structure
type Config struct {
	ServicesDir []string `yaml:"services_dir" mapstructure:"services_dir"`

	// Registry configuration
	// Use registry_url for any OCI-compatible registry (GAR, ECR, Docker Hub, self-hosted, etc.)
	// Examples:
	//   GAR:           us-central1-docker.pkg.dev/my-project/my-repo
	//   AWS ECR:       123456.dkr.ecr.us-east-1.amazonaws.com/my-repo
	//   Docker Hub:    docker.io/myuser
	//   Self-hosted:   registry.example.com/my-project
	// Leave empty for local-only builds (image:tag naming).
	RegistryURL string `yaml:"registry_url,omitempty" mapstructure:"registry_url"`

	// Deprecated: Use registry_url instead. These GAR-specific fields are kept
	// for backward compatibility. If project + gar + region are set without
	// registry_url, they will be auto-constructed into a registry_url value.
	Project string `yaml:"project,omitempty" mapstructure:"project"`
	GAR     string `yaml:"gar,omitempty" mapstructure:"gar"`
	Region  string `yaml:"region,omitempty" mapstructure:"region"`

	GlobalTag    string `yaml:"global_tag,omitempty" mapstructure:"global_tag"`
	MaxProcesses int    `yaml:"max_processes,omitempty" mapstructure:"max_processes"`

	// Resource-aware scheduling configuration
	EnableResourceMonitoring bool      `yaml:"enable_resource_monitoring,omitempty" mapstructure:"enable_resource_monitoring"`
	MaxCPUThreshold          float64   `yaml:"max_cpu_threshold,omitempty" mapstructure:"max_cpu_threshold"`
	MaxMemoryThreshold       float64   `yaml:"max_memory_threshold,omitempty" mapstructure:"max_memory_threshold"`
	MaxDiskThreshold         float64   `yaml:"max_disk_threshold,omitempty" mapstructure:"max_disk_threshold"`
	UseGAR                   bool      `yaml:"use_gar,omitempty" mapstructure:"use_gar"`         // Deprecated: use registry_url
	PushToGAR                bool      `yaml:"push_to_gar,omitempty" mapstructure:"push_to_gar"` // Deprecated: use push_to_registry
	PushToRegistry           bool      `yaml:"push_to_registry,omitempty" mapstructure:"push_to_registry"`
	Services                 []Service `yaml:"services,omitempty" mapstructure:"services"`

	// Smart features configuration
	Smart                 bool   `yaml:"smart" mapstructure:"smart"`
	GitTrack              bool   `yaml:"git_track" mapstructure:"git_track"`
	GitTrackDepth         int    `yaml:"git_track_depth" mapstructure:"git_track_depth"`
	Cache                 bool   `yaml:"cache" mapstructure:"cache"`
	Force                 bool   `yaml:"force" mapstructure:"force"`
	InputChangedServices  string `yaml:"input_changed_services" mapstructure:"input_changed_services"`
	OutputChangedServices string `yaml:"output_changed_services" mapstructure:"output_changed_services"`

	// BuildKit configuration
	EnableBuildKit bool `yaml:"enable_buildkit,omitempty" mapstructure:"enable_buildkit"`

	// Push configuration
	PushConcurrency int    `yaml:"push_concurrency,omitempty" mapstructure:"push_concurrency"`
	CacheType       string `yaml:"cache_type,omitempty" mapstructure:"cache_type"`

	// Cache TTL configuration (Go duration strings like "5m", "24h")
	GitCacheTTL string `yaml:"git_cache_ttl,omitempty" mapstructure:"git_cache_ttl"`
	CacheTTL    string `yaml:"cache_ttl,omitempty" mapstructure:"cache_ttl"`
}

// BuildResult represents the result of a build operation
type BuildResult struct {
	Service     string `json:"service"`
	Image       string `json:"image"`
	Status      string `json:"status"`
	BuildOutput string `json:"build_output,omitempty"`
	PushStatus  string `json:"push_status,omitempty"`
	PushOutput  string `json:"push_output,omitempty"`
	StartTime   time.Time
	EndTime     time.Time
}
