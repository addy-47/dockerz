package smart

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/addy-47/dockerz/internal/cache"
	"github.com/addy-47/dockerz/internal/config"
	"github.com/addy-47/dockerz/internal/discovery"
	"github.com/addy-47/dockerz/internal/git"
	"github.com/addy-47/dockerz/internal/logging"
)

// Orchestrator handles smart build decisions
type Orchestrator struct {
	config     *SmartConfig
	cacheMgr   cache.CacheManager
	gitTracker *git.Tracker
	logger     *logging.Logger
}

// NewOrchestrator creates a new smart orchestrator
func NewOrchestrator(config *SmartConfig) *Orchestrator {
	var cacheMgr cache.CacheManager
	switch config.CacheLevel {
	case cache.LayerCacheLevel:
		cacheMgr = cache.NewLayerCache(&cache.CacheConfig{
			Enabled: config.CacheEnabled,
			Level:   config.CacheLevel,
			TTL:     config.CacheTTL,
		})
	case cache.RegistryCacheLevel:
		cacheMgr = cache.NewRegistryCache(&cache.CacheConfig{
			Enabled: config.CacheEnabled,
			Level:   config.CacheLevel,
			TTL:     config.CacheTTL,
		})
	case cache.DistributedCacheLevel:
		cacheMgr = cache.NewDistributedCache(&cache.CacheConfig{
			Enabled: config.CacheEnabled,
			Level:   config.CacheLevel,
			TTL:     config.CacheTTL,
		})
	default:
		cacheMgr = cache.NewDistributedCache(&cache.CacheConfig{
			Enabled: config.CacheEnabled,
			Level:   config.CacheLevel,
			TTL:     config.CacheTTL,
		})
	}

	return &Orchestrator{
		config:     config,
		cacheMgr:   cacheMgr,
		gitTracker: git.NewTracker(),
		logger:     nil, // Will be set by caller
	}
}

// SetLogger sets the logger for the orchestrator
func (o *Orchestrator) SetLogger(logger *logging.Logger) {
	o.logger = logger
	// Also set logger for cache manager if it supports it
	switch cacheMgr := o.cacheMgr.(type) {
	case *cache.RegistryCache:
		cacheMgr.SetLogger(logger)
	case *cache.DistributedCache:
		cacheMgr.SetLogger(logger)
	}
}

// OrchestrateBuilds analyzes services and makes smart build decisions
func (o *Orchestrator) OrchestrateBuilds(cfg *config.Config, services []discovery.DiscoveredService) (*OrchestrationResult, error) {
	result := &OrchestrationResult{
		ServiceStates: make([]ServiceState, 0, len(services)),
		Decisions:     make(map[string]BuildDecision),
		TotalServices: len(services),
	}

	if !o.config.Enabled {
		// If smart features are disabled, build everything
		for _, service := range services {
			result.Decisions[service.Name] = ForceBuild
			result.BuildCount++
		}
		return result, nil
	}

	// Preload all git changes once if tracking is enabled
	if o.config.GitTracking {
		depth := o.config.GitTrackDepth
		if depth == 0 {
			depth = 2
		}
		if err := o.gitTracker.PreloadChanges(depth); err != nil {
			if o.logger != nil {
				o.logger.Warn(logging.CATEGORY_SMART, fmt.Sprintf("Failed to preload git changes: %v. Falling back to sequential checks.", err))
			}
		}
	}

	// Analyze each service
	for _, service := range services {
		state, decision := o.analyzeService(cfg, service)
		result.ServiceStates = append(result.ServiceStates, state)

		switch decision {
		case SkipBuild:
			result.SkipCount++
		case ForceBuild, ConditionalBuild:
			result.BuildCount++
		}

		result.Decisions[service.Name] = decision
	}

	return result, nil
}

// analyzeService determines if a service needs to be built
func (o *Orchestrator) analyzeService(cfg *config.Config, service discovery.DiscoveredService) (ServiceState, BuildDecision) {
	state := ServiceState{
		ServiceName: service.Name,
	}

	// Log analysis start
	if o.logger != nil {
		o.logger.Debug(logging.CATEGORY_SMART, fmt.Sprintf("Analyzing service: %s", service.Name))
	}

	// Force rebuild takes highest priority
	if o.config.ForceRebuild {
		if o.logger != nil {
			o.logger.Info(logging.CATEGORY_SMART, fmt.Sprintf("%s: FORCE_BUILD - force rebuild enabled", service.Name))
		}
		return state, ForceBuild
	}

	// If git tracking is disabled, always build
	if !o.config.GitTracking {
		if o.logger != nil {
			o.logger.Info(logging.CATEGORY_SMART, fmt.Sprintf("%s: CONDITIONAL_BUILD - git tracking disabled", service.Name))
		}
		return state, ConditionalBuild
	}

	// Git tracking is enabled - trust Git over cache
	depth := o.config.GitTrackDepth
	if depth == 0 {
		depth = 2 // Default depth
	}
	
	if o.logger != nil {
		o.logger.Debug(logging.CATEGORY_GIT, fmt.Sprintf("Checking git changes for %s (depth: %d)", service.Name, depth))
	}

	// 1. Check if image already exists in registry (Registry-First Strategy)
	// Even if there are git changes, if the image for this tag already exists, we can skip.
	if cfg.UseGAR {
		var imageFullName string
		imageFullName = fmt.Sprintf("%s-docker.pkg.dev/%s/%s/%s:%s",
			cfg.Region, cfg.Project, cfg.GAR, service.ImageName, service.Tag)

		if o.logger != nil {
			o.logger.Debug(logging.CATEGORY_SMART, fmt.Sprintf("Checking registry for %s", imageFullName))
		}

		if o.CheckGARImageExists(imageFullName) {
			if o.logger != nil {
				o.logger.Info(logging.CATEGORY_SMART, fmt.Sprintf("%s: SKIP_BUILD - image already exists in registry: %s", service.Name, imageFullName))
			}
			state.GARImageExists = true
			state.GARConfigured = true
			state.GARReachable = true
			return state, SkipBuild
		}
	}

	changedFiles, err := o.gitTracker.GetChangedFiles(service.Path, depth)
	if err != nil {
		if o.logger != nil {
			o.logger.Warn(logging.CATEGORY_GIT, fmt.Sprintf("Failed to get git changes for %s: %v", service.Name, err))
		}
		// Git failed - always build
		if o.logger != nil {
			o.logger.Info(logging.CATEGORY_SMART, fmt.Sprintf("%s: CONDITIONAL_BUILD - git check failed", service.Name))
		}
		return state, ConditionalBuild
	}

	state.ChangedFiles = changedFiles
	if len(changedFiles) > 0 {
		if o.logger != nil {
			o.logger.Info(logging.CATEGORY_GIT, fmt.Sprintf("Git changes detected for %s: %d files changed", service.Name, len(changedFiles)))
			for _, file := range changedFiles {
				o.logger.Debug(logging.CATEGORY_GIT, fmt.Sprintf("  %s: %s", service.Name, file))
			}
		}
		// Git detected changes - build
		if o.logger != nil {
			o.logger.Info(logging.CATEGORY_SMART, fmt.Sprintf("%s: CONDITIONAL_BUILD - git changes detected", service.Name))
		}
		return state, ConditionalBuild
	}

	// Git says no changes - skip build (trust Git over cache)
	if o.logger != nil {
		o.logger.Info(logging.CATEGORY_GIT, fmt.Sprintf("Git reports no changes for %s", service.Name))
		o.logger.Info(logging.CATEGORY_SMART, fmt.Sprintf("%s: SKIP_BUILD - no git changes", service.Name))
	}
	return state, SkipBuild
}

// UpdateCache updates the cache with new build results
func (o *Orchestrator) UpdateCache(serviceName, imageHash string) error {
	entry := &cache.CacheEntry{
		ServiceName: serviceName,
		ImageHash:   imageHash,
		Timestamp:   time.Now(),
		TTL:         o.config.CacheTTL,
	}

	return o.cacheMgr.Set(entry)
}

// Cleanup performs cache cleanup
func (o *Orchestrator) Cleanup() error {
	return o.cacheMgr.Cleanup()
}

// GetStats returns orchestration statistics
func (o *Orchestrator) GetStats(result *OrchestrationResult) string {
	return fmt.Sprintf("Smart Orchestration: %d total, %d to build, %d skipped",
		result.TotalServices, result.BuildCount, result.SkipCount)
}

// GAR Integration Framework (Phase 3 placeholders)

// CheckGARConnectivity checks if GAR registry is configured and reachable
func (o *Orchestrator) CheckGARConnectivity(serviceName string) (configured, reachable bool) {
	// TODO: Phase 3 - Implement actual GAR connectivity checks
	// For now, return placeholders
	log.Printf("GAR connectivity check for %s: placeholder implementation", serviceName)
	return false, false
}

// CheckGARImageExists checks if the service image exists in GAR using docker manifest inspect
func (o *Orchestrator) CheckGARImageExists(imageFullName string) bool {
	// Use 'docker manifest inspect' to check for image existence without pulling.
	// This requires the registry to be authenticated (e.g. via gcloud auth configure-docker).
	cmd := exec.Command("docker", "manifest", "inspect", imageFullName)
	
	// We don't want to pollute stdout with manifest JSON, so we discard it.
	// We only care about the exit code.
	err := cmd.Run()
	return err == nil
}

// UpdateGARState updates the service state with GAR information
func (o *Orchestrator) UpdateGARState(cfg *config.Config, state *ServiceState, service discovery.DiscoveredService) {
	state.GARConfigured, state.GARReachable = o.CheckGARConnectivity(service.Name)
	if state.GARConfigured && state.GARReachable {
		imageFullName := fmt.Sprintf("%s-docker.pkg.dev/%s/%s/%s:%s",
			cfg.Region, cfg.Project, cfg.GAR, service.ImageName, service.Tag)
		state.GARImageExists = o.CheckGARImageExists(imageFullName)
	}
}
