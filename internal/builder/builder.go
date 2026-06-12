package builder

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/addy-47/dockerz/internal/config"
)

// GetGitCommitID fetches the short Git commit ID for default tagging
func GetGitCommitID() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to fetch Git commit ID. Ensure this is a Git repository.")
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

	// Try docker manifest inspect to verify registry auth
	// Use a non-existent tag to avoid pulling, we just care about auth status
	testImage := cfg.RegistryURL + "/dockerz-auth-check:test"
	testCmd := exec.Command("docker", "manifest", "inspect", testImage)
	if err := testCmd.Run(); err != nil {
		// Auth failed — show registry-specific hint based on URL pattern
		hint := registryAuthHint(cfg.RegistryURL)
		return fmt.Errorf("registry authentication failed. %s", hint)
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
		log.Printf("Skipping build for %s (smart orchestration)", task.ServicePath)
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

	if !task.Quiet {
		log.Printf("Building image for %s: %s", task.ServicePath, imageFullName)
	}

	// Build the image
	var buildCmd *exec.Cmd
	cacheType := task.Config.CacheType
	if cacheType == "" {
		cacheType = "inline" // Default fallback
	}

	if task.Config.EnableBuildKit && cacheType != "none" {
		// Use BuildKit for better caching and performance
		progressFlag := "plain"
		if task.Quiet {
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
		if !task.Quiet {
			log.Printf("Building %s with BuildKit (cache: %s)", imageFullName, cacheType)
		}
	} else {
		// Use traditional docker build
		buildCmd = exec.Command("docker", "build", "-t", imageFullName, ".")
		if !task.Quiet {
			log.Printf("Building %s with traditional docker build", imageFullName)
		}
	}

	buildCmd.Dir = task.ServicePath

	// Capture build output: tee to terminal AND buffer (for build.log on failure)
	var buildBuf bytes.Buffer
	if task.Quiet {
		// During progress mode, suppress Docker output from terminal
		// (still captured in buffer for build.log on failure)
		buildCmd.Stdout = &buildBuf
		buildCmd.Stderr = &buildBuf
	} else {
		buildCmd.Stdout = io.MultiWriter(os.Stdout, &buildBuf)
		buildCmd.Stderr = io.MultiWriter(os.Stderr, &buildBuf)
	}

	if err := buildCmd.Run(); err != nil {
		if !task.Quiet {
			log.Printf("Failed to build %s", imageFullName)
		}
		result.Status = "failed"
		// Capture actual Docker stderr/stdout output (not just Go exit error)
		if buildBuf.Len() > 0 {
			result.BuildOutput = strings.TrimSpace(buildBuf.String())
		} else {
			result.BuildOutput = err.Error()
		}
		result.EndTime = time.Now()
		return result
	}

	if !task.Quiet {
		log.Printf("Successfully built %s", imageFullName)
	}
	result.Status = "success"
	result.EndTime = time.Now()

	// Push to registry if enabled and build was successful
	if task.Config.RegistryURL != "" && task.Config.PushToRegistry && result.Status == "success" {
		if !task.Quiet {
			log.Printf("Queueing push to registry: %s", imageFullName)
		}
		result.PushStatus = "queued"
	}

	return result
}
