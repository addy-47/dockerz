package builder

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/addy-47/dockerz/internal/config"
	"github.com/addy-47/dockerz/internal/logging"
)

var logger *logging.Logger

// SetLogger sets the package-level logger for the builder package
func SetLogger(l *logging.Logger) {
	logger = l
}

// GetGitCommitID fetches the short Git commit ID for default tagging
func GetGitCommitID() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		if logger != nil {
			logger.Error(logging.CATEGORY_GIT, "Failed to fetch Git commit ID. Ensure this is a Git repository.")
		}
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// CheckRegistryAuth verifies Docker can authenticate to the configured registry.
// It checks that Docker is running and prints a registry-specific hint if auth fails.
func CheckRegistryAuth(cfg *config.Config) error {
	if cfg.RegistryURL == "" {
		return nil // No registry configured, nothing to check
	}

	// Try docker info as a basic connectivity check
	dockerCmd := exec.Command("docker", "info")
	if err := dockerCmd.Run(); err != nil {
		return fmt.Errorf("Docker daemon is not available: %w", err)
	}

	// Try docker manifest inspect to verify registry auth.
	// Use a non-existent tag to avoid pulling. A "no such manifest" error
	// means auth is fine (image doesn't exist). Other errors mean auth failed.
	testImage := cfg.RegistryURL + "/dockerz-auth-check:test"
	testCmd := exec.Command("docker", "manifest", "inspect", testImage)
	output, err := testCmd.CombinedOutput()
	if err != nil {
		errMsg := strings.ToLower(string(output))
		// "no such manifest" / "manifest unknown" = auth OK, image doesn't exist
		if strings.Contains(errMsg, "no such manifest") || strings.Contains(errMsg, "manifest unknown") || strings.Contains(errMsg, "not found") {
			return nil
		}
		// Real auth failure — show registry-specific hint
		hint := registryAuthHint(cfg.RegistryURL)
		return fmt.Errorf("registry authentication failed: %s. %s", strings.TrimSpace(string(output)), hint)
	}
	return nil
}

// registryAuthHint returns a shell command hint for authenticating to a registry
// based on the URL pattern.
func registryAuthHint(registryURL string) string {
	switch {
	case strings.Contains(registryURL, "pkg.dev"):
		// Google Artifact Registry
		host := strings.SplitN(registryURL, "/", 2)[0]
		return fmt.Sprintf("Run: gcloud auth configure-docker %s", host)
	case strings.Contains(registryURL, "dkr.ecr"):
		// AWS ECR
		parts := strings.SplitN(registryURL, ".", 2)
		region := ""
		if len(parts) >= 2 {
			subParts := strings.SplitN(parts[1], ".", 3)
			if len(subParts) >= 2 {
				region = subParts[0]
			}
		}
		if region != "" {
			return fmt.Sprintf("Run: aws ecr get-login-password --region %s | docker login --username AWS --password-stdin %s", region, strings.SplitN(registryURL, "/", 2)[0])
		}
		return fmt.Sprintf("Run: aws ecr get-login-password | docker login --username AWS --password-stdin %s", strings.SplitN(registryURL, "/", 2)[0])
	case strings.Contains(registryURL, "azurecr.io"):
		// Azure Container Registry
		return fmt.Sprintf("Run: az acr login --name %s", strings.SplitN(registryURL, ".", 2)[0])
	default:
		// Generic Docker registry
		host := strings.SplitN(registryURL, "/", 2)[0]
		return fmt.Sprintf("Run: docker login %s", host)
	}
}

// BuildDockerImage builds a single Docker image
func BuildDockerImage(task BuildTask) BuildResult {
	result := BuildResult{
		Service:   task.ServicePath,
		StartTime: time.Now(),
	}

	// Skip build if smart features indicate it shouldn't be built
	if !task.NeedsBuild {
		result.Status = "skipped"
		result.EndTime = time.Now()
		if logger != nil {
			logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Skipping build for %s (smart orchestration)", task.ServicePath))
		}
		return result
	}

	// Construct full image name
	var imageFullName string
	if task.Config.RegistryURL != "" {
		imageFullName = fmt.Sprintf("%s/%s:%s", task.Config.RegistryURL, task.ImageName, task.Tag)
	} else {
		imageFullName = fmt.Sprintf("%s:%s", task.ImageName, task.Tag)
	}

	result.Image = imageFullName
	result.ImageName = task.ImageName
	result.ServiceName = task.ServiceName
	// Use ServiceName for display identification (may differ from ImageName)

	if !task.Quiet && logger != nil {
		logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Building image for %s: %s", task.ServicePath, imageFullName))
	}

	// Build the image
	var buildCmd *exec.Cmd
	cacheType := task.Config.CacheType
	if cacheType == "" {
		cacheType = "inline" // Default fallback
	}

	if task.Config.EnableBuildKit && cacheType != "none" {
		// Use BuildKit for better caching and performance.
		// When a progress callback is set (live display active), always use
		// --progress=plain so we can capture real Docker step output.
		// Fall back to --progress=quiet only when there's no display.
		progressFlag := "plain"
		if task.Quiet && task.ProgressCb == nil {
			progressFlag = "quiet"
		}
		args := []string{"buildx", "build", "--load", "--progress=" + progressFlag}

		switch cacheType {
		case "inline":
			// Inline cache is stored in the image itself.
			// No --cache-from needed — Docker uses it automatically
			// from the local layer cache on subsequent builds.
			// Adding --cache-from=type=registry here would attempt
			// a remote pull that fails when the image doesn't exist.
			args = append(args, "--cache-to=type=inline")
		case "registry":
			args = append(args,
				"--cache-from=type=registry,ref="+imageFullName,
				"--cache-to=type=registry,ref="+imageFullName+",mode=max")
		}

		args = append(args, "-t", imageFullName, ".")
		buildCmd = exec.Command("docker", args...)
		buildCmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1", "BUILDKIT_PROGRESS=plain")
		if !task.Quiet && logger != nil {
			logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Building %s with BuildKit (cache: %s)", imageFullName, cacheType))
		}
	} else {
		// Use traditional docker build
		buildCmd = exec.Command("docker", "build", "-t", imageFullName, ".")
		if !task.Quiet && logger != nil {
			logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Building %s with traditional docker build", imageFullName))
		}
	}

	buildCmd.Dir = task.ServicePath

	// Capture build output: buffer for build.log (on failure) and optionally
	// feed Docker --progress=plain lines to the live display via ProgressCb.
	// Three modes:
	//   1. Quiet + ProgressCb → read piped output line-by-line, display Docker steps
	//   2. Quiet alone          → capture to buffer only (legacy)
	//   3. Not quiet            → tee to terminal AND buffer
	var buildBuf bytes.Buffer
	switch {
	case task.Quiet && task.ProgressCb != nil:
		// Live display mode: pipe stdout+stderr, read line-by-line.
		stdout, _ := buildCmd.StdoutPipe()
		stderr, _ := buildCmd.StderrPipe()

		var wg sync.WaitGroup
		wg.Add(2)

		readPipe := func(rc io.ReadCloser) {
			defer wg.Done()
			scanner := bufio.NewScanner(rc)
			scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
			for scanner.Scan() {
				line := scanner.Text()
				buildBuf.WriteString(line + "\n")
				// Use ServiceName for display identification
				task.ProgressCb(task.ServiceName, line)
			}
			rc.Close()
		}

		go readPipe(stdout)
		go readPipe(stderr)

		if err := buildCmd.Start(); err != nil {
			result.Status = "failed"
			result.BuildOutput = err.Error()
			result.EndTime = time.Now()
			return result
		}
		wg.Wait()
		if err := buildCmd.Wait(); err != nil {
			result.Status = "failed"
			if buildBuf.Len() > 0 {
				result.BuildOutput = strings.TrimSpace(buildBuf.String())
			} else {
				result.BuildOutput = err.Error()
			}
			result.EndTime = time.Now()
			return result
		}

	case task.Quiet:
		// Quiet mode without display — capture to buffer only.
		buildCmd.Stdout = &buildBuf
		buildCmd.Stderr = &buildBuf
		if err := buildCmd.Run(); err != nil {
			result.Status = "failed"
			if buildBuf.Len() > 0 {
				result.BuildOutput = strings.TrimSpace(buildBuf.String())
			} else {
				result.BuildOutput = err.Error()
			}
			result.EndTime = time.Now()
			return result
		}

	default:
		// Not quiet — tee to terminal and buffer.
		buildCmd.Stdout = io.MultiWriter(os.Stdout, &buildBuf)
		buildCmd.Stderr = io.MultiWriter(os.Stderr, &buildBuf)
		if err := buildCmd.Run(); err != nil {
			if logger != nil {
				logger.Error(logging.CATEGORY_BUILD, fmt.Sprintf("Failed to build %s", imageFullName))
			}
			result.Status = "failed"
			if buildBuf.Len() > 0 {
				result.BuildOutput = strings.TrimSpace(buildBuf.String())
			} else {
				result.BuildOutput = err.Error()
			}
			result.EndTime = time.Now()
			return result
		}
		if logger != nil {
			logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Successfully built %s", imageFullName))
		}
	}

	result.Status = "success"
	result.EndTime = time.Now()

	// Push to registry if enabled and build was successful
	if task.Config.RegistryURL != "" && task.Config.PushToRegistry && result.Status == "success" {
		if !task.Quiet && logger != nil {
			logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Queueing push to registry: %s", imageFullName))
		}
		result.PushStatus = "queued"
	}

	return result
}
