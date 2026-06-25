package config

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// defaultThresholds returns platform-aware resource monitoring thresholds.
// Containers/VMs get more conservative thresholds; bare metal gets higher ones.
func defaultThresholds() (cpu, mem, disk float64) {
	// Defaults for bare metal / developer machines
	cpuDefault := 80.0
	memDefault := 85.0
	diskDefault := 90.0

	// Detect containerized environments
	if isRunningInContainer() {
		// More conservative thresholds for resource-constrained environments
		cpuDefault = 65.0
		memDefault = 70.0
		diskDefault = 75.0
	}
	// Check for WSL (Windows Subsystem for Linux) - moderate thresholds
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/version"); err == nil {
			if strings.Contains(strings.ToLower(string(data)), "microsoft") ||
				strings.Contains(strings.ToLower(string(data)), "wsl") {
				cpuDefault = 70.0
				memDefault = 75.0
				diskDefault = 80.0
			}
		}
	}

	return cpuDefault, memDefault, diskDefault
}

// isRunningInContainer checks if the process is running inside a container
func isRunningInContainer() bool {
	// Check for Docker-specific file at root
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check Kubernetes environment variable
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}

	// Check cgroup for container indicators
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		cgroupContent := string(data)
		if strings.Contains(cgroupContent, "docker") ||
			strings.Contains(cgroupContent, "kubepods") ||
			strings.Contains(cgroupContent, "containerd") ||
			strings.Contains(cgroupContent, "garden") {
			return true
		}
	}

	// Check for /.containerenv (Podman/CRI-O)
	if _, err := os.Stat("/.containerenv"); err == nil {
		return true
	}

	return false
}

// ValidateTxtFile validates that the file path has a .txt extension
func ValidateTxtFile(filePath string) error {
	if filePath == "" {
		return nil // Empty is allowed
	}
	if !strings.HasSuffix(strings.ToLower(filePath), ".txt") {
		return fmt.Errorf("file '%s' must have a .txt extension", filePath)
	}
	return nil
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	// Validate that the config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file %s does not exist", configPath)
	}

	// Set up viper
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Environment variables not supported - use CLI flags instead

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Handle backward compatibility for services_dir (can be string or []string)
	if servicesDirRaw := viper.Get("services_dir"); servicesDirRaw != nil {
		switch v := servicesDirRaw.(type) {
		case string:
			// Handle comma-separated string or single directory
			if v != "" {
				// Split by comma and trim spaces
				dirs := strings.Split(v, ",")
				for i, dir := range dirs {
					dirs[i] = strings.TrimSpace(dir)
				}
				config.ServicesDir = dirs
			}
		case []interface{}:
			// Already a slice, convert to []string
			dirs := make([]string, len(v))
			for i, item := range v {
				if str, ok := item.(string); ok {
					dirs[i] = str
				}
			}
			config.ServicesDir = dirs
		}
	}

	// No environment variable overrides for use_gar and push_to_gar - use CLI flags instead

	// Set defaults
	if config.MaxProcesses == 0 {
		config.MaxProcesses = 4 // Default to 4 parallel processes
	}

	// Set defaults for resource-aware scheduling (platform-aware)
	cpuDef, memDef, diskDef := defaultThresholds()
	if config.MaxCPUThreshold == 0 {
		config.MaxCPUThreshold = cpuDef
	}
	if config.MaxMemoryThreshold == 0 {
		config.MaxMemoryThreshold = memDef
	}
	if config.MaxDiskThreshold == 0 {
		config.MaxDiskThreshold = diskDef
	}

	// Set default for BuildKit (enabled by default for better performance)
	if !config.EnableBuildKit {
		config.EnableBuildKit = true
	}

	// Set default for push concurrency
	if config.PushConcurrency == 0 {
		config.PushConcurrency = 2 // Default to 2 concurrent pushes
	}

	// Set default for cache type
	if config.CacheType == "" {
		config.CacheType = "inline" // Default to BuildKit inline cache
	}

	// Set defaults for cache TTLs
	if config.GitCacheTTL == "" {
		config.GitCacheTTL = "5m" // Default: 5 minutes
	}
	if config.CacheTTL == "" {
		config.CacheTTL = "24h" // Default: 24 hours
	}

	// Validate cache TTLs can be parsed
	if _, err := time.ParseDuration(config.GitCacheTTL); err != nil {
		return nil, fmt.Errorf("invalid git_cache_ttl '%s': %w", config.GitCacheTTL, err)
	}
	if _, err := time.ParseDuration(config.CacheTTL); err != nil {
		return nil, fmt.Errorf("invalid cache_ttl '%s': %w", config.CacheTTL, err)
	}

	// Ensure smart features are disabled by default for basic builds
	if !config.Smart {
		config.Smart = false
	}

	// Backward compatibility: auto-construct registry_url from GAR-specific fields
	if config.RegistryURL == "" && config.UseGAR {
		if config.Project != "" && config.Region != "" && config.GAR != "" {
			config.RegistryURL = fmt.Sprintf("%s-docker.pkg.dev/%s/%s", config.Region, config.Project, config.GAR)
		} else {
			return nil, fmt.Errorf("use_gar is set but missing required GAR fields: project, gar, region")
		}
	}

	// Map push_to_gar to push_to_registry for GAR users
	if config.PushToGAR && !config.PushToRegistry {
		config.PushToRegistry = true
	}

	// Validate registry_url if set (basic format check)
	if config.RegistryURL != "" {
		if strings.Contains(config.RegistryURL, " ") {
			return nil, fmt.Errorf("registry_url must not contain spaces")
		}
	}

	// Validate cache type
	if config.CacheType != "" {
		switch config.CacheType {
		case "none", "inline", "registry":
			// Valid
		default:
			return nil, fmt.Errorf("invalid cache_type '%s': must be one of: none, inline, registry", config.CacheType)
		}
	}

	// Validate changed services file paths
	if err := ValidateTxtFile(config.InputChangedServices); err != nil {
		return nil, fmt.Errorf("invalid input_changed_services: %w", err)
	}
	if err := ValidateTxtFile(config.OutputChangedServices); err != nil {
		return nil, fmt.Errorf("invalid output_changed_services: %w", err)
	}

	return &config, nil
}

// SaveSampleConfig creates a sample build.yaml file if it doesn't already exist
func SaveSampleConfig(filename string) error {
	// Check if file already exists
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("config file %s already exists", filename)
	}

	sampleYAML := `# Dockerz Configuration File
# This file configures how Dockerz builds and manages your microservices.
# All paths are relative to the project root unless specified as absolute paths.

# Directory to scan for services (leave empty for auto-discovery of all subdirectories)
# Can be overridden with --services-dir flag (supports comma-separated paths)
# Example: --services-dir=backend,frontend/src,api/services
# Note: Auto-discovery excludes common build/dependency directories like debian/, node_modules/, .git/, etc.
services_dir:

# ===== REGISTRY CONFIGURATION =====
# Configure your OCI-compatible container registry (optional)
# Leave empty for local-only builds (image:tag naming)
# Supports: GAR, AWS ECR, Docker Hub, Azure ACR, self-hosted, etc.
# Override with --registry-url flag
#
# Examples:
#   GAR:           us-central1-docker.pkg.dev/my-project/my-repo
#   AWS ECR:       123456.dkr.ecr.us-east-1.amazonaws.com/my-repo
#   Docker Hub:    docker.io/myuser
#   Self-hosted:   registry.example.com/my-project
registry_url:

# GAR-specific fields (alternative to registry_url for Google Artifact Registry)
# If registry_url is empty and use_gar is true, these are combined into:
#   <region>-docker.pkg.dev/<project>/<gar>
project: my-gcp-project
gar: my-artifact-registry
region: us-central1

# ===== BUILD CONFIGURATION =====

# Global Docker tag applied to all services (defaults to Git commit hash if not set)
# Common values: 'latest', 'v1.0.0', or leave empty for auto-generated tags
# Override with --tag flag
global_tag: latest

# Maximum number of parallel Docker builds (0 = use CPU core count / 2)
# Override with --max-processes flag
max_processes: 4

# Maximum number of concurrent image pushes to registry
# Override with --push-concurrency flag
push_concurrency: 2

# BuildKit cache mode for builds: none, inline, or registry
# 'inline' embeds cache in the image, 'registry' pushes cache to registry
# Override with --cache-type flag
cache_type: inline

# Whether to push built images to the configured registry after building
# Only effective when registry_url is set
# Override with --push-to-registry flag
push_to_registry: true

# Alternative: use GAR-specific fields (use_gar + project/gar/region + push_to_gar)
# use_gar: true
# push_to_gar: true

# ===== SMART BUILD FEATURES (v2.0) =====
# Advanced features for optimizing CI/CD pipelines - disabled by default

# Enable smart build orchestration (analyzes dependencies and build order)
# Use --smart flag to enable in CI/CD
smart: false

# Enable git change detection to only rebuild modified services
# Requires git repository - tracks file changes between commits
# Use --git-track flag to enable
git_track: false

# Git tracking depth: how many recent commits to analyze for changes
# Default: 2 (checks last 2 commits: HEAD and HEAD~1)
# Can be set with --depth <number>
git_track_depth: 2

# Enable build caching to speed up rebuilds of unchanged services
# Use --cache flag to enable
cache: false

# Force rebuild all services regardless of smart features
# Useful for clean builds or when cache is corrupted
# Use --force flag to enable
force: false

# ===== CACHE TTL CONFIGURATION =====
# How long to cache git operation results (Go duration format)
git_cache_ttl: 5m

# How long to cache build results before considering them stale (Go duration format)
cache_ttl: 24h

# ===== CHANGE DETECTION FILES =====
# File paths for storing lists of changed services (used with git_track)

# Input file containing list of services that have changed (for CI/CD input)
# Used when you want to specify changed services externally
input_changed_services:

# Output file where Dockerz will write the list of detected changed services
# Useful for subsequent CI/CD steps or debugging
output_changed_services:

# ===== SERVICE DEFINITIONS =====
# Explicitly define services to build (leave empty for auto-discovery)
# Auto-discovery scans services_dir for directories containing Dockerfiles
#
# Behavior depends on git_track setting:
# - If services is empty AND git_track is false: Builds all services with Dockerfiles
# - If services is empty AND git_track is true: Builds only changed services from last commit
#   (if no changes detected, logs clear message to user)
# - If services is defined: Only builds the explicitly listed services
#
# Auto-discovery excludes common directories: debian/, node_modules/, .git/, internal/, vendor/, etc.
# This prevents conflicts in CI/CD where you can't modify this config file
#
# Each service can have:
# - name: Path to service directory (relative to project root)
# - image_name: Custom Docker image name (optional, defaults to service name)
# - tag: Service-specific tag (optional, overrides global_tag)

services:
  # Examples (uncomment and modify as needed):

  # - name: services/api
  #   image_name: my-api-service    # Optional custom image name
  #   tag: v1.0.0                   # Optional service-specific tag

  # - name: services/web-frontend
  #   image_name: my-web-app        # Optional custom image name

  # - name: microservices/user-service

# ===== USAGE EXAMPLES =====
#
# Basic build (auto-discover all services):
#   dockerz build
#
# Build with custom settings:
#   dockerz build --registry-url us-east1-docker.pkg.dev/my-prod-project/my-repo --global-tag v2.1.0
#   dockerz build --registry-url 123456.dkr.ecr.us-east-1.amazonaws.com/my-repo --push-to-registry
#
# Smart build with git tracking (CI/CD):
#   dockerz build --smart --git-track --cache --output-changed-services changed.txt
#
# Build specific services only:
#   dockerz build --services-dir=backend,frontend
#
# Force rebuild everything:
#   dockerz build --force
`

	if err := os.WriteFile(filename, []byte(sampleYAML), 0644); err != nil {
		return fmt.Errorf("failed to write sample config file: %w", err)
	}

	return nil
}
