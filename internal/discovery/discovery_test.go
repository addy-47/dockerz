package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/addy-47/dockerz/internal/config"
)

func TestNormalizeImageName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"already lowercase", "myservice", "myservice"},
		{"mixed case", "MyService", "myservice"},
		{"underscores", "my_service", "my-service"},
		{"spaces", "my service", "my-service"},
		{"mixed case with spaces", "API Gateway", "api-gateway"},
		{"leading hyphen", "-service", "service"},
		{"trailing hyphen", "service-", "service"},
		{"both sides hyphen", "-service-", "service"},
		{"special characters", "test@#$service", "test-service"},
		{"multiple hyphens", "my---service", "my-service"},
		{"all special chars", "@#$%", "service-"},
		{"already kebab case", "my-service", "my-service"},
		{"numbers", "service2-api", "service2-api"},
		{"dots", "my.service", "my.service"},
		{"underscores and spaces", "my_ cool service", "my-cool-service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeImageName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeImageName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateImageName(t *testing.T) {
	tests := []struct {
		name      string
		imageName string
		wantErr   bool
	}{
		{"valid simple", "myservice", false},
		{"valid with hyphens", "my-service", false},
		{"valid with underscores", "my_service", false},
		{"valid with numbers", "my-service-2", false},
		{"valid with dots", "my.service", false},
		{"valid starts with number", "2service", false},
		{"valid all combined", "my.service-2_v3", false},
		{"uppercase", "MyService", true},
		{"special chars", "my@service", true},
		{"empty string", "", true},
		{"starts with hyphen", "-service", true},
		{"starts with underscore", "_service", true},
		{"starts with dot", ".service", true},
		{"contains spaces", "my service", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageName(tt.imageName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateImageName(%q) error = %v, wantErr = %v", tt.imageName, err, tt.wantErr)
			}
		})
	}
}

func TestIsExcludedDirectory(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{"excluded debian", "debian", true},
		{"excluded node_modules", "node_modules", true},
		{"excluded vendor", "vendor", true},
		{"excluded __pycache__", "__pycache__", true},
		{"excluded .git", ".git", true},
		{"excluded .svn", ".svn", true},
		{"excluded .vscode", ".vscode", true},
		{"excluded .idea", ".idea", true},
		{"excluded build", "build", true},
		{"excluded dist", "dist", true},
		{"excluded target", "target", true},
		{"excluded hidden dir", ".hidden", true},
		{"excluded .DS_Store", ".DS_Store", true},
		{"excluded Thumbs.db", "Thumbs.db", true},
		{"not excluded src", "src", false},
		{"not excluded services", "services", false},
		{"not excluded myapp", "myapp", false},
		{"not excluded api", "api", false},
		{"not excluded backend", "backend", false},
		{"not excluded frontend", "frontend", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcludedDirectory(tt.dir)
			if got != tt.want {
				t.Errorf("isExcludedDirectory(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestDeduplicateServices(t *testing.T) {
	tests := []struct {
		name     string
		services []DiscoveredService
		wantLen  int
	}{
		{
			name:     "empty slice",
			services: []DiscoveredService{},
			wantLen:  0,
		},
		{
			name: "no duplicates",
			services: []DiscoveredService{
				{Path: "services/api", Name: "api"},
				{Path: "services/backend", Name: "backend"},
				{Path: "services/frontend", Name: "frontend"},
			},
			wantLen: 3,
		},
		{
			name: "with duplicates",
			services: []DiscoveredService{
				{Path: "services/api", Name: "api"},
				{Path: "services/backend", Name: "backend"},
				{Path: "services/api", Name: "api"}, // duplicate
				{Path: "services/frontend", Name: "frontend"},
				{Path: "services/backend", Name: "backend"}, // duplicate
			},
			wantLen: 3,
		},
		{
			name: "all duplicates",
			services: []DiscoveredService{
				{Path: "services/api", Name: "api"},
				{Path: "services/api", Name: "api"},
				{Path: "services/api", Name: "api"},
			},
			wantLen: 1,
		},
		{
			name: "same path different name",
			services: []DiscoveredService{
				{Path: "services/api", Name: "api"},
				{Path: "services/api", Name: "api-v2"}, // same path, should dedup
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateServices(tt.services)
			if len(got) != tt.wantLen {
				t.Errorf("deduplicateServices() returned %d services, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestValidateDockerfile(t *testing.T) {
	t.Run("file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create a Dockerfile
		if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM alpine"), 0644); err != nil {
			t.Fatalf("failed to create Dockerfile: %v", err)
		}

		err := ValidateDockerfile(tmpDir)
		if err != nil {
			t.Errorf("ValidateDockerfile() unexpected error: %v", err)
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		// No Dockerfile created

		err := ValidateDockerfile(tmpDir)
		if err == nil {
			t.Error("ValidateDockerfile() expected error for missing Dockerfile")
		}
	})

	t.Run("path does not exist", func(t *testing.T) {
		err := ValidateDockerfile("/nonexistent/path")
		if err == nil {
			t.Error("ValidateDockerfile() expected error for nonexistent path")
		}
	})
}

func TestWalkForDockerfiles_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create service directories with Dockerfiles
	services := []string{"api", "backend", "frontend"}
	for _, svc := range services {
		svcDir := filepath.Join(tmpDir, svc)
		if err := os.MkdirAll(svcDir, 0755); err != nil {
			t.Fatalf("failed to create %s: %v", svcDir, err)
		}
		if err := os.WriteFile(filepath.Join(svcDir, "Dockerfile"), []byte("FROM alpine"), 0644); err != nil {
			t.Fatalf("failed to create Dockerfile in %s: %v", svcDir, err)
		}
	}

	// Create a non-service directory (no Dockerfile)
	nonSvcDir := filepath.Join(tmpDir, "library")
	if err := os.MkdirAll(nonSvcDir, 0755); err != nil {
		t.Fatalf("failed to create %s: %v", nonSvcDir, err)
	}

	// Create an excluded directory with a Dockerfile (should be skipped)
	excludedDir := filepath.Join(tmpDir, "node_modules")
	innerDir := filepath.Join(excludedDir, "some-module")
	if err := os.MkdirAll(innerDir, 0755); err != nil {
		t.Fatalf("failed to create %s: %v", innerDir, err)
	}
	if err := os.WriteFile(filepath.Join(innerDir, "Dockerfile"), []byte("FROM alpine"), 0644); err != nil {
		t.Fatalf("failed to create Dockerfile in excluded dir: %v", err)
	}

	svcs, errs := walkForDockerfiles(tmpDir, "latest")
	if len(errs) > 0 {
		t.Fatalf("walkForDockerfiles() returned errors: %v", errs)
	}

	if len(svcs) != 3 {
		t.Errorf("walkForDockerfiles() returned %d services, want 3", len(svcs))
	}

	// Verify service names
	found := make(map[string]bool)
	for _, svc := range svcs {
		found[svc.Name] = true
		if svc.Tag != "latest" {
			t.Errorf("service %s: Tag = %q, want %q", svc.Name, svc.Tag, "latest")
		}
	}

	for _, svc := range services {
		if !found[svc] {
			t.Errorf("service %q not found in results", svc)
		}
	}
}

func TestDiscoverExplicitServices(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a service with Dockerfile
	svcDir := filepath.Join(tmpDir, "api")
	if err := os.MkdirAll(svcDir, 0755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, "Dockerfile"), []byte("FROM alpine"), 0644); err != nil {
		t.Fatalf("failed to create Dockerfile: %v", err)
	}

	t.Run("valid service", func(t *testing.T) {
		cfg := &config.Config{
			Services: []config.Service{
				{Name: svcDir, ImageName: "my-api", Tag: "v1.0.0"},
			},
		}

		svcs, errs := discoverExplicitServices(cfg, "latest")
		if len(errs) > 0 {
			t.Fatalf("discoverExplicitServices() returned errors: %v", errs)
		}
		if len(svcs) != 1 {
			t.Fatalf("discoverExplicitServices() returned %d services, want 1", len(svcs))
		}
		if svcs[0].ImageName != "my-api" {
			t.Errorf("ImageName = %q, want %q", svcs[0].ImageName, "my-api")
		}
		if svcs[0].Tag != "v1.0.0" {
			t.Errorf("Tag = %q, want %q", svcs[0].Tag, "v1.0.0")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		cfg := &config.Config{
			Services: []config.Service{
				{Name: ""},
			},
		}

		svcs, errs := discoverExplicitServices(cfg, "latest")
		if len(svcs) != 0 {
			t.Errorf("expected 0 services, got %d", len(svcs))
		}
		if len(errs) == 0 {
			t.Error("expected errors for missing service name")
		}
	})

	t.Run("no Dockerfile", func(t *testing.T) {
		cfg := &config.Config{
			Services: []config.Service{
				{Name: "/nonexistent/path"},
			},
		}

		svcs, errs := discoverExplicitServices(cfg, "latest")
		if len(svcs) != 0 {
			t.Errorf("expected 0 services, got %d", len(svcs))
		}
		if len(errs) == 0 {
			t.Error("expected errors for missing Dockerfile")
		}
	})

	t.Run("empty services list", func(t *testing.T) {
		cfg := &config.Config{}
		svcs, errs := discoverExplicitServices(cfg, "latest")
		if len(svcs) != 0 {
			t.Errorf("expected 0 services, got %d", len(svcs))
		}
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d", len(errs))
		}
	})
}

func TestDiscoverFromInputFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a service with Dockerfile
	svcDir := filepath.Join(tmpDir, "api")
	if err := os.MkdirAll(svcDir, 0755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, "Dockerfile"), []byte("FROM alpine"), 0644); err != nil {
		t.Fatalf("failed to create Dockerfile: %v", err)
	}

	t.Run("valid input file", func(t *testing.T) {
		inputFile := filepath.Join(tmpDir, "services.txt")
		content := svcDir + "\n"
		if err := os.WriteFile(inputFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write input file: %v", err)
		}

		svcs, errs := discoverFromInputFile(inputFile, "latest")
		if len(errs) > 0 {
			t.Fatalf("discoverFromInputFile() returned errors: %v", errs)
		}
		if len(svcs) != 1 {
			t.Fatalf("discoverFromInputFile() returned %d services, want 1", len(svcs))
		}
		if svcs[0].Name != "api" {
			t.Errorf("Name = %q, want %q", svcs[0].Name, "api")
		}
	})

	t.Run("empty input file", func(t *testing.T) {
		inputFile := filepath.Join(tmpDir, "empty.txt")
		if err := os.WriteFile(inputFile, []byte(""), 0644); err != nil {
			t.Fatalf("failed to write input file: %v", err)
		}

		svcs, errs := discoverFromInputFile(inputFile, "latest")
		if len(svcs) != 0 {
			t.Errorf("expected 0 services, got %d", len(svcs))
		}
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d", len(errs))
		}
	})

	t.Run("nonexistent input file", func(t *testing.T) {
		svcs, errs := discoverFromInputFile("/nonexistent/input.txt", "latest")
		if len(svcs) != 0 {
			t.Errorf("expected 0 services, got %d", len(svcs))
		}
		if len(errs) == 0 {
			t.Error("expected errors for nonexistent input file")
		}
	})

	t.Run("service without Dockerfile", func(t *testing.T) {
		inputFile := filepath.Join(tmpDir, "missing.txt")
		content := "/nonexistent/service\n"
		if err := os.WriteFile(inputFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write input file: %v", err)
		}

		svcs, errs := discoverFromInputFile(inputFile, "latest")
		if len(svcs) != 0 {
			t.Errorf("expected 0 services, got %d", len(svcs))
		}
		if len(errs) == 0 {
			t.Error("expected errors for missing Dockerfile")
		}
	})
}

func TestDiscoverServices_AutoDiscovery(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a couple of service directories with Dockerfiles
	for _, svc := range []string{"api", "backend"} {
		svcDir := filepath.Join(tmpDir, svc)
		if err := os.MkdirAll(svcDir, 0755); err != nil {
			t.Fatalf("failed to create %s: %v", svcDir, err)
		}
		if err := os.WriteFile(filepath.Join(svcDir, "Dockerfile"), []byte("FROM alpine"), 0644); err != nil {
			t.Fatalf("failed to create Dockerfile: %v", err)
		}
	}

	// Change to temp dir to test auto-discovery (walks ".")
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current dir: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	cfg := &config.Config{}
	result, err := DiscoverServices(cfg, "test-tag")
	if err != nil {
		t.Fatalf("DiscoverServices() unexpected error: %v", err)
	}

	if len(result.Services) != 2 {
		t.Errorf("DiscoverServices() returned %d services, want 2", len(result.Services))
	}

	// Verify both services found
	found := make(map[string]bool)
	for _, svc := range result.Services {
		found[svc.Name] = true
		if svc.Tag != "test-tag" {
			t.Errorf("Tag = %q, want %q", svc.Tag, "test-tag")
		}
	}
	if !found["api"] {
		t.Error("service 'api' not found")
	}
	if !found["backend"] {
		t.Error("service 'backend' not found")
	}
}
