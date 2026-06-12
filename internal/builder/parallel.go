package builder

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/addy-47/dockerz/internal/config"
	"github.com/addy-47/dockerz/internal/discovery"
	"github.com/addy-47/dockerz/internal/renderer"
)

// ResourceAwareConfig holds configuration for resource-aware scheduling
type ResourceAwareConfig struct {
	EnableResourceMonitoring bool
	MaxCPUThreshold          float64
	MaxMemoryThreshold       float64
	MaxDiskThreshold         float64
	MonitorInterval          time.Duration
}

// BuildImages builds Docker images for discovered services in parallel
func BuildImages(cfg *config.Config, discoveryResult *discovery.DiscoveryResult, maxProcesses int, reporter ...*renderer.ProgressRenderer) ([]BuildResult, Summary) {
	startTime := time.Now()

	// Get the progress reporter if provided
	var progressReporter *renderer.ProgressRenderer
	if len(reporter) > 0 {
		progressReporter = reporter[0]
	}

	// Create build.log file
	logFile, err := os.OpenFile("build.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Warning: Failed to create build.log file: %v", err)
	} else {
		defer logFile.Close()
		// Write header to log file
		fmt.Fprintf(logFile, "=== Dockerz Build Log ===\n")
		fmt.Fprintf(logFile, "Started at: %s\n", startTime.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(logFile, "Services to build: %d\n", len(discoveryResult.Services))
		fmt.Fprintf(logFile, "Max processes: %d\n\n", maxProcesses)
	}

	// Resource-aware scheduling configuration from config
	resourceConfig := ResourceAwareConfig{
		EnableResourceMonitoring: cfg.EnableResourceMonitoring,
		MaxCPUThreshold:          cfg.MaxCPUThreshold,
		MaxMemoryThreshold:       cfg.MaxMemoryThreshold,
		MaxDiskThreshold:         cfg.MaxDiskThreshold,
		MonitorInterval:          2 * time.Second,
	}

	// Initialize resource monitor
	var resourceMonitor *ResourceMonitor
	if resourceConfig.EnableResourceMonitoring {
		monitorConfig := ResourceMonitorConfig{
			MaxCPUThreshold:    resourceConfig.MaxCPUThreshold,
			MaxMemoryThreshold: resourceConfig.MaxMemoryThreshold,
			MaxDiskThreshold:   resourceConfig.MaxDiskThreshold,
			CheckInterval:      resourceConfig.MonitorInterval,
		}
		resourceMonitor = NewResourceMonitor(monitorConfig)
		resourceMonitor.Start()
		defer resourceMonitor.Stop()

		log.Printf("Resource-aware scheduling enabled: CPU<%.0f%%, Memory<%.0f%%, Disk<%.0f%%",
			resourceConfig.MaxCPUThreshold, resourceConfig.MaxMemoryThreshold, resourceConfig.MaxDiskThreshold)
		log.Printf("System info: %s", GetSystemInfo())
	}

	// Initialize push manager if pushing is enabled
	var pushMgr *PushManager
	var mapMu sync.Mutex
	pushResultsMap := make(map[string]chan PushResult)
	if cfg.RegistryURL != "" && cfg.PushToRegistry {
		pushConcurrency := cfg.PushConcurrency
		if pushConcurrency == 0 {
			pushConcurrency = 2
		}
		pushMgr = NewPushManager(cfg, pushConcurrency)
		pushMgr.Start()
		log.Printf("Parallel pushes enabled: max_concurrent=%d", pushConcurrency)
	}

	// Prepare build tasks
	tasks := make([]BuildTask, 0, len(discoveryResult.Services))
	for _, service := range discoveryResult.Services {
		task := BuildTask{
			ServicePath: service.Path,
			ImageName:   service.ImageName,
			Tag:         service.Tag,
			Config:      cfg,
			NeedsBuild:  service.NeedsBuild,
			Quiet:       progressReporter != nil,
		}
		tasks = append(tasks, task)
	}

	log.Printf("Starting parallel builds for %d services with max_processes=%d", len(tasks), maxProcesses)
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting parallel builds for %d services with max_processes=%d\n", len(tasks), maxProcesses)
	}

	// Channel to receive results
	resultsChan := make(chan BuildResult, len(tasks))

	// Semaphore to limit concurrent goroutines
	sem := make(chan struct{}, maxProcesses)

	// WaitGroup to wait for all goroutines to complete
	var wg sync.WaitGroup

	// Task queue for resource-aware scheduling
	taskQueue := make(chan BuildTask, len(tasks))
	for _, task := range tasks {
		taskQueue <- task
	}
	close(taskQueue)

	// Worker pool with resource-aware scheduling
	for i := 0; i < maxProcesses; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for task := range taskQueue {
				// Resource-aware scheduling: wait for resources to be available
				if resourceMonitor != nil {
					for {
						if resourceMonitor.CanSchedule() {
							break
						}
						time.Sleep(500 * time.Millisecond) // Wait before retrying
					}
				}

				sem <- struct{}{} // Acquire semaphore

				// Report build start to progress renderer
				if progressReporter != nil {
					progressReporter.ServiceStarted(task.ImageName)
				}

				if !task.Quiet {
					log.Printf("Worker %d: Starting build for %s", workerID, task.ServicePath)
				}
				result := BuildDockerImage(task)

				// Queue push if build was successful and push is requested
				if result.Status == "success" && result.PushStatus == "queued" && pushMgr != nil {
					if progressReporter != nil {
						progressReporter.ServicePushStarted(task.ImageName)
					}
					mapMu.Lock()
					pushResultsMap[result.Image] = pushMgr.QueuePush(result.Image, task.ServicePath)
					mapMu.Unlock()
				}

				<-sem // Release semaphore

				resultsChan <- result
				if !task.Quiet {
					log.Printf("Worker %d: Completed build for %s (status: %s)", workerID, task.ServicePath, result.Status)
				}
			}
		}(i)
	}

	// Close results channel when all goroutines are done
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	results := make([]BuildResult, 0, len(tasks))
	for result := range resultsChan {
		results = append(results, result)
		// Log individual build result to file
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Service: %s, Image: %s, Status: %s", time.Now().Format("15:04:05"), result.Service, result.Image, result.Status)
			if result.Status == "failed" {
				fmt.Fprintf(logFile, ", Build Output: %s", result.BuildOutput)
			}
			fmt.Fprintf(logFile, "\n")
		}
	}

	// Wait for all pushes and update results
	if pushMgr != nil {
		log.Printf("Waiting for all pushes to complete...")
		pushMgr.Stop() // This waits for all queued pushes to finish

		for i, result := range results {
			if resChan, exists := pushResultsMap[result.Image]; exists {
				pushRes := <-resChan
				results[i].PushStatus = pushRes.Status
				results[i].PushOutput = pushRes.Output
			}
		}
	}

	// Report completion/failure to progress renderer (after pushes resolved)
	if progressReporter != nil {
		for _, result := range results {
			// Use short image name for progress events, falling back to path if empty
			svcName := result.ImageName
			if svcName == "" {
				// Extract short name from path as fallback
				if idx := strings.LastIndex(result.Service, "/"); idx >= 0 {
					svcName = result.Service[idx+1:]
				} else {
					svcName = result.Service
				}
			}
			buildTime := result.EndTime.Sub(result.StartTime)
			switch result.Status {
			case "success":
				progressReporter.ServiceDone(svcName, result.Image, buildTime, 0)
			case "failed":
				errMsg := result.BuildOutput
				if errMsg == "" {
					errMsg = "build failed"
				}
				progressReporter.ServiceFailed(svcName, errMsg)
			default:
				// skipped — no report needed
			}
		}
	}

	// Calculate summary
	totalDuration := time.Since(startTime)
	successfulBuilds := 0
	failedBuilds := 0
	failedPushes := 0

	for _, result := range results {
		if result.Status == "success" {
			successfulBuilds++
		} else {
			failedBuilds++
		}
		if result.PushStatus == "failed" {
			failedPushes++
		}
	}

	summary := Summary{
		TotalServices:    len(tasks),
		SuccessfulBuilds: successfulBuilds,
		FailedBuilds:     failedBuilds,
		FailedPushes:     failedPushes,
		Duration:         totalDuration,
	}

	// Console summary is provided by main.go's structured logger
	// or by the progress renderer's visual summary in TTY mode.
	// build.log gets its own summary below.

	// Write final summary to log file
	if logFile != nil {
		fmt.Fprintf(logFile, "\n=== Build Summary ===\n")
		fmt.Fprintf(logFile, "Total services: %d\n", summary.TotalServices)
		fmt.Fprintf(logFile, "Successful builds: %d\n", summary.SuccessfulBuilds)
		fmt.Fprintf(logFile, "Failed builds: %d\n", summary.FailedBuilds)
		fmt.Fprintf(logFile, "Duration: %v\n", summary.Duration)
		fmt.Fprintf(logFile, "Completed at: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		if summary.FailedBuilds > 0 {
			fmt.Fprintf(logFile, "\nFailed builds:\n")
			for _, result := range results {
				if result.Status == "failed" {
					fmt.Fprintf(logFile, "- %s: %s\n", result.Service, result.Image)
				}
			}
		}
		if summary.FailedPushes > 0 {
			fmt.Fprintf(logFile, "\nFailed pushes:\n")
			for _, result := range results {
				if result.PushStatus == "failed" {
					fmt.Fprintf(logFile, "- %s: %s\n", result.Service, result.Image)
				}
			}
		}
	}

	return results, summary
}
