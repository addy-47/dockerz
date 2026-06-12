package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTxtFile(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"empty path", "", false},
		{"valid .txt", "changes.txt", false},
		{"valid .TXT uppercase", "changes.TXT", false},
		{"no extension", "changes", true},
		{"wrong extension .csv", "changes.csv", true},
		{"wrong extension .yaml", "changes.yaml", true},
		{"wrong extension .json", "changes.json", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTxtFile(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTxtFile(%q) error = %v, wantErr = %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestSaveSampleConfig(t *testing.T) {
	// Test saving to a temp location
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	err := SaveSampleConfig(configPath)
	if err != nil {
		t.Fatalf("SaveSampleConfig() unexpected error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("SaveSampleConfig() did not create the file")
	}

	// Verify content is non-empty
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("SaveSampleConfig() created empty file")
	}

	// Test saving again (should fail - file exists)
	err = SaveSampleConfig(configPath)
	if err == nil {
		t.Fatal("SaveSampleConfig() should error when file already exists")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/build.yaml")
	if err == nil {
		t.Fatal("LoadConfig() expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("invalid: [yaml: broken"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() expected error for invalid YAML")
	}
}

func TestLoadConfig_ValidMinimal(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	yamlContent := `
project: my-project
gar: my-repo
region: us-central1
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() unexpected error: %v", err)
	}

	// Verify defaults are applied
	if cfg.Project != "my-project" {
		t.Errorf("Project = %q, want %q", cfg.Project, "my-project")
	}
	if cfg.MaxProcesses != 4 {
		t.Errorf("MaxProcesses = %d, want 4", cfg.MaxProcesses)
	}
	if cfg.PushConcurrency != 2 {
		t.Errorf("PushConcurrency = %d, want 2", cfg.PushConcurrency)
	}
	if cfg.CacheType != "inline" {
		t.Errorf("CacheType = %q, want %q", cfg.CacheType, "inline")
	}
	if cfg.GitCacheTTL != "5m" {
		t.Errorf("GitCacheTTL = %q, want %q", cfg.GitCacheTTL, "5m")
	}
	if cfg.CacheTTL != "24h" {
		t.Errorf("CacheTTL = %q, want %q", cfg.CacheTTL, "24h")
	}
	if !cfg.EnableBuildKit {
		t.Error("EnableBuildKit should default to true")
	}
}

func TestLoadConfig_GARValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	// use_gar=true but missing project/gar/region
	yamlContent := `
use_gar: true
project: ""
gar: ""
region: ""
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() expected error for missing GAR fields")
	}
}

func TestLoadConfig_InvalidCacheType(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	yamlContent := `
cache_type: invalid-cache
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() expected error for invalid cache_type")
	}
}

func TestLoadConfig_InvalidGitCacheTTL(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	yamlContent := `
git_cache_ttl: not-a-duration
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() expected error for invalid git_cache_ttl")
	}
}

func TestLoadConfig_InvalidCacheTTL(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	yamlContent := `
cache_ttl: bad-value
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() expected error for invalid cache_ttl")
	}
}

func TestLoadConfig_BackwardCompatServicesDirString(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	// services_dir as a single string (old format)
	yamlContent := `
services_dir: services,backend
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() unexpected error: %v", err)
	}

	if len(cfg.ServicesDir) != 2 {
		t.Fatalf("ServicesDir length = %d, want 2", len(cfg.ServicesDir))
	}
	if cfg.ServicesDir[0] != "services" {
		t.Errorf("ServicesDir[0] = %q, want %q", cfg.ServicesDir[0], "services")
	}
	if cfg.ServicesDir[1] != "backend" {
		t.Errorf("ServicesDir[1] = %q, want %q", cfg.ServicesDir[1], "backend")
	}
}

func TestLoadConfig_BackwardCompatServicesDirList(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	// services_dir as a list (new format)
	yamlContent := `
services_dir:
  - services
  - backend
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() unexpected error: %v", err)
	}

	if len(cfg.ServicesDir) != 2 {
		t.Fatalf("ServicesDir length = %d, want 2", len(cfg.ServicesDir))
	}
	if cfg.ServicesDir[0] != "services" {
		t.Errorf("ServicesDir[0] = %q, want %q", cfg.ServicesDir[0], "services")
	}
	if cfg.ServicesDir[1] != "backend" {
		t.Errorf("ServicesDir[1] = %q, want %q", cfg.ServicesDir[1], "backend")
	}
}

func TestLoadConfig_InputChangedServicesValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	// input_changed_services without .txt extension should fail
	yamlContent := `
input_changed_services: changes.csv
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() expected error for non-.txt input file")
	}
}

func TestLoadConfig_OutputChangedServicesValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	yamlContent := `
output_changed_services: results.csv
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() expected error for non-.txt output file")
	}
}

func TestLoadConfig_WithExplicitValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "build.yaml")

	yamlContent := `
project: prod-project
gar: prod-repo
region: europe-west1
global_tag: v2.0.0
max_processes: 8
push_concurrency: 4
cache_type: registry
enable_buildkit: true
git_cache_ttl: 10m
cache_ttl: 48h
services:
  - name: services/api
    image_name: my-api
    tag: v1.0.0
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() unexpected error: %v", err)
	}

	if cfg.Project != "prod-project" {
		t.Errorf("Project = %q, want %q", cfg.Project, "prod-project")
	}
	if cfg.GlobalTag != "v2.0.0" {
		t.Errorf("GlobalTag = %q, want %q", cfg.GlobalTag, "v2.0.0")
	}
	if cfg.MaxProcesses != 8 {
		t.Errorf("MaxProcesses = %d, want 8", cfg.MaxProcesses)
	}
	if cfg.PushConcurrency != 4 {
		t.Errorf("PushConcurrency = %d, want 4", cfg.PushConcurrency)
	}
	if cfg.CacheType != "registry" {
		t.Errorf("CacheType = %q, want %q", cfg.CacheType, "registry")
	}
	if cfg.GitCacheTTL != "10m" {
		t.Errorf("GitCacheTTL = %q, want %q", cfg.GitCacheTTL, "10m")
	}
	if cfg.CacheTTL != "48h" {
		t.Errorf("CacheTTL = %q, want %q", cfg.CacheTTL, "48h")
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("Services length = %d, want 1", len(cfg.Services))
	}
	if cfg.Services[0].Name != "services/api" {
		t.Errorf("Services[0].Name = %q, want %q", cfg.Services[0].Name, "services/api")
	}
	if cfg.Services[0].ImageName != "my-api" {
		t.Errorf("Services[0].ImageName = %q, want %q", cfg.Services[0].ImageName, "my-api")
	}
}
