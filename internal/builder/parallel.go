package builder

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/addy-47/dockerz/internal/config"
	"github.com/addy-47/dockerz/internal/discovery"
)

// ResourceAwareConfig holds configuration for resource-aware scheduling
type ResourceAwareConfig struct {
	EnableResourceMonitoring bool
	MaxCPUThreshold         float64
	MaxMemoryThreshold      float64
	MaxDiskThreshold        float64
	MonitorInterval         time.Duration
}

// BuildImages builds Docker images for discovered services in parallel
func BuildImages(cfg *config.Config, discoveryResult *discovery.DiscoveryResult, maxProcesses int) ([]BuildResult, Summary) {
	startTime := time.Now()

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
		MaxCPUThreshold:         cfg.MaxCPUThreshold,
		MaxMemoryThreshold:      cfg.MaxMemoryThreshold,
		MaxDiskThreshold:        cfg.MaxDiskThreshold,
		MonitorInterval:         2 * time.Second,
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
	if cfg.PushToGAR {
		pushMgr = NewPushManager(cfg, 2) // Default to 2 concurrent pushes
		pushMgr.Start()
		log.Printf("Parallel pushes enabled: max_concurrent=2")
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
				
				log.Printf("Worker %d: Starting build for %s", workerID, task.ServicePath)
				result := BuildDockerImage(task)
				
				// Queue push if build was successful and push is requested
				if result.Status == "success" && result.PushStatus == "queued" && pushMgr != nil {
					mapMu.Lock()
					pushResultsMap[result.Image] = pushMgr.QueuePush(result.Image, task.ServicePath)
					mapMu.Unlock()
				}
				
				<-sem // Release semaphore
				
				resultsChan <- result
				log.Printf("Worker %d: Completed build for %s (status: %s)", workerID, task.ServicePath, result.Status)
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

	// Print summary
	log.Printf("\nBuild Summary:")
	log.Printf("Total services: %d", summary.TotalServices)
	log.Printf("Successful builds: %d", summary.SuccessfulBuilds)
	log.Printf("Failed builds: %d", summary.FailedBuilds)
	if summary.FailedBuilds > 0 {
		log.Printf("Failed builds:")
		for _, result := range results {
			if result.Status == "failed" {
				log.Printf("- %s: %s", result.Service, result.Image)
			}
		}
	}
	if summary.FailedPushes > 0 {
		log.Printf("Failed pushes:")
		for _, result := range results {
			if result.PushStatus == "failed" {
				log.Printf("- %s: %s", result.Service, result.Image)
			}
		}
	}

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