package builder

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
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

// CheckGARAuth checks if GAR authentication is set up
func CheckGARAuth() error {
	cmd := exec.Command("gcloud", "auth", "print-access-token")
	return cmd.Run()
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
	if task.Config.UseGAR {
		imageFullName = fmt.Sprintf("%s-docker.pkg.dev/%s/%s/%s:%s",
			task.Config.Region, task.Config.Project, task.Config.GAR, task.ImageName, task.Tag)
	} else {
		imageFullName = fmt.Sprintf("%s:%s", task.ImageName, task.Tag)
	}

	result.Image = imageFullName

	log.Printf("Building image for %s: %s", task.ServicePath, imageFullName)

	// Build the image
	var buildCmd *exec.Cmd
	cacheType := task.Config.CacheType
	if cacheType == "" {
		cacheType = "inline" // Default fallback
	}

	if task.Config.EnableBuildKit && cacheType != "none" {
		// Use BuildKit for better caching and performance
		args := []string{"buildx", "build", "--load", "--progress=plain"}

		switch cacheType {
		case "inline":
			args = append(args,
				"--cache-from=type=registry,ref="+imageFullName,
				"--cache-to=type=inline")
		case "registry":
			args = append(args,
				"--cache-from=type=registry,ref="+imageFullName,
				"--cache-to=type=registry,ref="+imageFullName+",mode=max")
		}

		args = append(args, "-t", imageFullName, ".")
		buildCmd = exec.Command("docker", args...)
		buildCmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1", "BUILDKIT_PROGRESS=plain")
		log.Printf("Building %s with BuildKit (cache: %s)", imageFullName, cacheType)
	} else {
		// Use traditional docker build
		buildCmd = exec.Command("docker", "build", "-t", imageFullName, ".")
		log.Printf("Building %s with traditional docker build", imageFullName)
	}
	
	buildCmd.Dir = task.ServicePath
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		log.Printf("Failed to build %s", imageFullName)
		result.Status = "failed"
		result.BuildOutput = err.Error()
		result.EndTime = time.Now()
		return result
	}

	log.Printf("Successfully built %s", imageFullName)
	result.Status = "success"
	result.EndTime = time.Now()

	// Push to GAR if enabled and build was successful
	if task.Config.UseGAR && task.Config.PushToGAR && result.Status == "success" {
		log.Printf("Queueing push to GAR: %s", imageFullName)
		result.PushStatus = "queued"
	}

	return result
}
