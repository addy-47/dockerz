# Dockerz v3.2.2 - The Ultimate Docker Companion Tool

```
     _            _                    
  __| | ___   ___| | _____ _ __ ____  
 / _' |/ _ \ / __| |/ / _ \ '__|_  /  
| (_| | (_) | (__|   <  __/ |   / /   
 \__,_|\___/ \___|_|\_\___|_|  /___|  
```

**The ultimate Docker companion tool making container management effortless**

Dockerz is a powerful CLI tool for building and pushing multiple Docker images in parallel with advanced **smart features** for optimized CI/CD workflows. It combines intelligent change detection, multi-level caching, and smart build orchestration to dramatically improve build performance and reduce CI/CD pipeline times.

## 🚀 Key Features

### Core Features
- **🖥️ ASCII Art Banner**: Beautiful terminal welcome with project branding
- **Parallel Building**: Build multiple Docker images simultaneously with configurable process limits
- **Google Artifact Registry (GAR)**: Native support for GAR with automatic authentication and image naming
- **Auto-Discovery**: Automatically find and build services from directory structure or explicit configurations
- **Flexible Configuration**: YAML-based configuration with comprehensive CLI flag overrides
- **Cross-Platform**: Works on Linux, macOS, and Windows (WSL2)

### 🧠 Smart Features (v3.1.0)
- **Registry-First Skipping**: Check remote registry (GAR) before building. Skip if image exists.
- **Git Change Detection**: Optimized preloaded git diff analysis for instant results.
- **Multi-Level Caching**: Layer, local hash, and registry-based caching.
- **Parallel Execution**: simultaneous builds and background registry pushes.
- **Smart Build Orchestration**: Intelligently skip unchanged services, only rebuild what needs rebuilding.
- **Dry-Run Mode**: Preview builds without executing Docker (`--dry-run`).
- **Configurable Push Concurrency**: Control concurrent image pushes (`--push-concurrency`).
- **BuildKit Cache Modes**: `none`, `inline`, or `registry` cache types (`--cache-type`).
- **Config Validation**: Validate `build.yaml` structure and field correctness (`dockerz config validate`).
- **Shell Completions**: Generate bash/zsh/fish completions (`dockerz completion <shell>`).
- **Configurable Cache TTLs**: Per-cache expiry for git and build caches (`git_cache_ttl`, `cache_ttl`).
- **Platform-Aware Thresholds**: Resource limits adapt to container vs bare-metal environments.
- **APT Repository**: Install via GPG-signed apt repo on GitHub Pages.
- **CI/CD Integration**: Optimized for Cloud Build and GitHub Actions with minimal configuration.

### 🆕 v3.2.2 New Features
- **Live Status Display**: Real-time service status updates with per-service Docker build step output. Shows latest `--progress=plain` lines for building services. Automatically falls back to traditional logging on non-TTY.
- **Generic Multi-Registry Support**: Unified `--registry-url` and `--push-to-registry` for any OCI-compatible registry (GAR, AWS ECR, Azure ACR, Docker Hub, self-hosted). Auto-detects registry type from URL patterns.
- **Build Log Stderr Capture**: `build.log` now captures both stdout and stderr from build commands via `io.MultiWriter` + `bytes.Buffer`.

### 🏗️ Modular Architecture
Dockerz is built with a modular internal structure:
- **Builder Module**: Handles parallel Docker image builds
- **Cache Module**: Manages multi-level caching (layer, hash, registry)
- **Config Module**: Configuration management and validation
- **Discovery Module**: Intelligent service discovery and scanning
- **Display Module**: Live status display (TTY in-place or non-TTY line output)
- **Git Module**: Git-based change detection and tracking
- **Logging Module**: Comprehensive structured logging
- **Smart Module**: Advanced orchestration and decision making

## Installation

> **Note**: Dockerz is already installed on your system. Verify with: `dockerz --version`

### For Development
```bash
git clone <repository-url>
cd dockerz
go build -o dockerz ./cmd/dockerz
```

### For Users
Dockerz is distributed as a standalone binary. Download the latest release for your platform from the releases page and add it to your PATH.

## Quick Start

1. **Initialize a new project:**
   ```bash
   dockerz init
   ```

2. **Edit the generated `build.yaml`** with your service configurations

3. **Build all services:**
   ```bash
   dockerz build
   ```

4. **Smart build with change detection (recommended for CI/CD):**
   ```bash
   dockerz build --smart --git-track --cache
   ```

## Prerequisites

| Requirement | Purpose | Verification |
|-------------|---------|--------------|
| **Docker** | Required for building Docker images | `docker --version` |
| **Git** | Required for change detection and default tagging | `git --version` |
| **Go 1.19+** (for building from source) | Required to build Dockerz | `go version` |
| **Google Cloud SDK** (Optional) | Required only for GAR integration | `gcloud --version` |

## Commands

### `dockerz init`
Initialize a new project with a sample configuration file.

```bash
dockerz init
```

This creates a `build.yaml` file with sample configuration and helpful comments.

### `dockerz build`
Build Docker images based on configuration with advanced features.

```bash
dockerz build [flags]
```

**Core Flags:**
- `--config, -c`: Configuration file path (default: build.yaml)
- `--max-processes, -m`: Maximum parallel build processes
- `--version, -v`: Print version information

**Registry Integration:**
- `--registry-url`: OCI-compatible registry URL (GAR/ECR/ACR/generic)
- `--push-to-registry`: Push built images to registry after building
- `--project`: GCP project ID for GAR integration (legacy)
- `--region`: GCP region for GAR (legacy)
- `--gar`: Name of the Google Artifact Registry repository (legacy)
- `--push-to-gar`: Push to GAR (legacy, use --push-to-registry)

**Smart Features:**
- `--smart`: Enable smart build orchestration
- `--git-track`: Enable git change tracking
- `--depth`: Git tracking depth (0 for full history, default 2)
- `--cache`: Enable multi-level build caching
- `--force`: Force rebuild of all services
- `--dry-run`: Preview builds without executing Docker
- `--push-concurrency`: Max concurrent image pushes (default 2)
- `--cache-type`: BuildKit cache mode (none, inline, registry)
- `--git-cache-ttl`: Git cache TTL duration (default 5m)
- `--cache-ttl`: Build cache TTL duration (default 24h)

**CI/CD Integration:**
- `--services-dir`: Comma-separated list of directories to scan
- `--input-changed-services`: Path to input file with changed services
- `--output-changed-services`: Path to output file for detected changes

**Configuration:**
- `dockerz config validate`: Validate build.yaml configuration
- `dockerz completion <bash|zsh|fish>`: Generate shell completions

**Global Configuration:**
- `--global-tag`: Global Docker tag for all built images

## Usage Examples

### Basic Usage

```bash
# Initialize project configuration
dockerz init

# Build all discovered services
dockerz build

# Build with custom parallel processes
dockerz build --max-processes 8

# Build with custom configuration
dockerz build --project my-project --region us-west1 --gar my-registry --global-tag v3.0.0
```

### Smart Features Usage

#### Enable Smart Build Orchestration
```bash
dockerz build --smart
```

#### Git-Based Change Detection
```bash
dockerz build --smart --git-track
```

#### Multi-Level Caching
```bash
dockerz build --smart --cache
```

#### Force Rebuild Everything
```bash
dockerz build --force
```

#### Combined Smart Build (Recommended for CI/CD)
```bash
# Basic smart build with git tracking (default depth 2)
dockerz build --smart --git-track --cache --max-processes 6

# Smart build with custom git tracking depth
dockerz build --smart --git-track --depth 3 --cache --max-processes 6

# Smart build with full history tracking
dockerz build --smart --git-track --depth 0 --cache --max-processes 6
```

### Advanced Usage

#### CI/CD Integration with External Change Detection
```bash
# Use external change detection
dockerz build --input-changed-services changed_services.txt

# Generate change detection for downstream steps
dockerz build --git-track --smart --output-changed-services changed_services.txt
```

#### Build with Custom Services Directory
```bash
# Scan specific directories for services
dockerz build --services-dir ./backend,./frontend,./api
```

#### Git Track Configuration
```bash
# Enable git tracking with default depth (2)
dockerz build --smart --git-track

# Enable git tracking with custom depth
dockerz build --smart --git-track --depth 3

# Full history tracking
dockerz build --smart --git-track --depth 0
```

## Configuration

Dockerz is configured through a `build.yaml` file. Generate a sample configuration:

```bash
dockerz init
```

### Example `build.yaml`

```yaml
# Dockerz v3.2.2 Configuration
# This file configures how Dockerz builds and manages your microservices.

# ===== DIRECTORY CONFIGURATION =====
# Directories to scan for services (leave empty for auto-discovery)
services_dir: []

# ===== REGISTRY CONFIGURATION =====
# Use registry_url for any OCI-compatible registry:
#   GAR:           us-central1-docker.pkg.dev/my-project/my-repo
#   AWS ECR:       123456.dkr.ecr.us-east-1.amazonaws.com/my-repo
#   Docker Hub:    docker.io/myuser
#   Self-hosted:   registry.example.com/my-project
# Leave empty for local-only builds.
registry_url: ""
push_to_registry: false

# Legacy GAR fields (used if registry_url is empty):
project: my-gcp-project          # Your GCP project ID
gar: my-artifact-registry        # GAR repository name
region: us-central1              # GCP region

# ===== BUILD CONFIGURATION =====
global_tag: latest               # Global tag (defaults to Git commit hash)
max_processes: 4                 # Max parallel builds

# ===== SMART FEATURES =====
smart: false                     # Enable smart build orchestration
git_track: false                 # Enable git change detection
cache: false                     # Enable build caching
force: false                     # Force rebuild all services
git_track_depth: 2               # Number of commits to check
input_changed_services:          # Input file with changed services
output_changed_services:         # Output file for detected changes

# ===== SERVICE DEFINITIONS =====
# Explicitly define services (leave empty for auto-discovery)
services: []
  # - name: services/api
  #   image_name: my-api-service    # Custom image name
  #   tag: v1.0.0                   # Service-specific tag
```

### Configuration Fields

| Field | Description | Default |
|-------|-------------|---------|
| `services_dir` | Directories to scan for services | Current directory (.) |
| `registry_url` | OCI-compatible registry URL for any registry (GAR/ECR/ACR) | "" |
| `push_to_registry` | Push built images to the configured registry | false |
| `project` | GCP project ID for GAR | Required for GAR |
| `gar` | GAR repository name | Required for GAR |
| `region` | GCP region for GAR | Required for GAR |
| `global_tag` | Global tag for all images | Git commit hash |
| `max_processes` | Max parallel build processes | 4 |
| `smart` | Enable smart orchestration | false |
| `git_track` | Enable git change detection | false |
| `cache` | Enable build caching | false |
| `force` | Force rebuild all services | false |
| `git_track_depth` | Commits to check for changes | 2 |
| `input_changed_services` | Input changed services file | "" |
| `output_changed_services` | Output changed services file | "" |

## Smart Features Deep Dive

### Automatic Service Discovery
Dockerz v3.2.2 intelligently discovers services by:
- Scanning for `Dockerfile` files recursively
- Excluding build directories (`debian/`, `build/`, `dist/`)
- Excluding dependency directories (`node_modules/`, `vendor/`, `__pycache__/`)
- Excluding version control (`.git/`, `.svn/`, `.hg/`)
- Excluding IDE directories (`.vscode/`, `.idea/`, `.vs/`)
- Normalizing service names to Docker-compatible kebab-case

### Git Change Detection
When `--git-track` is enabled, Dockerz analyzes git history:

```bash
dockerz build --smart --git-track
```

**How it works:**
1. Compares current working directory with recent commits
2. Identifies modified, added, or deleted files
3. Maps changed files to service directories
4. Only rebuilds services containing changed files
5. Significantly reduces build times for large projects

### Multi-Level Caching
Dockerz implements three cache levels for optimal performance:

```bash
dockerz build --smart --cache
```

**Cache Levels:**
- **Layer Cache**: Caches Docker layer information
- **Local Hash Cache**: Stores SHA256 hashes of service contents  
- **Registry Cache**: Caches build results with TTL

### Smart Orchestration Logic
The `--smart` flag enables intelligent decisions:

1. Calculate SHA256 hash of each service
2. Check git for changes since last build
3. Compare with cached build results
4. Skip services that haven't changed
5. Build only necessary services in parallel
6. Trust Git over cache for accuracy

### CI/CD Integration

#### Using External Change Detection
```bash
# CI/CD pipeline creates changed_services.txt
echo "services/api-gateway" > changed_services.txt
echo "services/user-service" >> changed_services.txt

# Dockerz builds only changed services
dockerz build --input-changed-services changed_services.txt
```

#### Generating Change Detection for Downstream
```bash
# Dockerz detects changes and outputs to file
dockerz build --git-track --smart --output-changed-services changed_services.txt

# Other pipeline steps can use this file
for service in $(cat changed_services.txt); do
  echo "Deploying $service"
  # deployment logic here
done
```

**Changed Services File Format:**
```
services/api-gateway
services/user-service
backend/service1/frontend
```

## Multi-Registry Support (v3.2.2)

Dockerz v3.2.2 supports any OCI-compatible registry via a unified `--registry-url` flag. Auto-detects registry type from the URL pattern:

| Registry | Example URL | Auth |
|----------|-------------|------|
| **Google Artifact Registry** | `us-central1-docker.pkg.dev/my-project/my-repo` | `gcloud auth configure-docker` |
| **AWS ECR** | `123456.dkr.ecr.us-east-1.amazonaws.com/my-repo` | `aws ecr get-login-password \| docker login` |
| **Azure ACR** | `myregistry.azurecr.io/my-repo` | `az acr login --name myregistry` |
| **Docker Hub** | `docker.io/myuser` | `docker login` |
| **Self-hosted** | `registry.example.com/my-project` | `docker login` |

### Setup

```bash
# GAR
dockerz build --registry-url us-central1-docker.pkg.dev/my-project/my-repo --push-to-registry

# AWS ECR
dockerz build --registry-url 123456.dkr.ecr.us-east-1.amazonaws.com/my-repo --push-to-registry

# Local-only (no registry)
dockerz build
```

## Architecture

Dockerz follows a modular architecture with the following internal components:

### Internal Modules

```
internal/
├── builder/       # Parallel Docker image building
├── cache/         # Multi-level caching system
├── config/        # Configuration management
├── discovery/     # Service discovery and scanning
├── display/       # Live status display (TTY/non-TTY)
├── git/          # Git change detection
├── logging/      # Structured logging
└── smart/        # Smart orchestration logic
```

### Code Structure

- **cmd/dockerz/main.go**: Entry point with CLI commands and banner
- **internal/builder/**: Handles parallel build execution
- **internal/cache/**: Manages layer, hash, and registry caching
- **internal/config/**: Configuration loading and validation
- **internal/discovery/**: Service discovery and file scanning
- **internal/git/**: Git tracking and diff analysis
- **internal/logging/**: Comprehensive structured logging
- **internal/display/**: Live status display (TTY in-place or non-TTY line output)
- **internal/smart/**: Intelligent build orchestration

## Troubleshooting

| Issue | Solution |
|-------|----------|
| **Binary not found** | Use absolute path or add to PATH |
| **build.yaml not found** | Run `dockerz init` or specify with `--config` |
| **Wrong working directory** | Run from project root containing services |
| **Path errors** | Verify relative paths in configuration |
| **System overload** | Reduce `--max-processes` value |
| **Docker permission errors** | Run `sudo usermod -aG docker $USER` |
| **Missing Dockerfile** | Ensure service directories contain valid Dockerfile |
| **Git errors** | Ensure project is a Git repository |
| **GAR authentication** | Run `gcloud auth configure-docker {region}-docker.pkg.dev` |
| **Smart features not working** | Ensure Git repository with committed changes |

## Development

### Building from Source
```bash
git clone <repository-url>
cd dockerz
go build -o dockerz ./cmd/dockerz
```

### Running Tests
```bash
go test ./...
```

### Building for Multiple Platforms
```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o dockerz-linux-amd64 ./cmd/dockerz

# macOS  
GOOS=darwin GOARCH=amd64 go build -o dockerz-darwin-amd64 ./cmd/dockerz

# Windows
GOOS=windows GOARCH=amd64 go build -o dockerz-windows-amd64.exe ./cmd/dockerz
```

---

**Dockerz v3.2.2** - Making container build orchestration intelligent, fast, and developer-friendly.