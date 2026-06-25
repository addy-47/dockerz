# Dockerz v4.0.0 Roadmap

> From "works well" to "polished, reliable, extensible."
>
> Current version: **3.0.0** | Target: **4.0.0**

---

## Table of Contents

1. [Guiding Principles](#-guiding-principles)
2. [Go Version Upgrade](#-go-version-upgrade)
3. [Phase 1: Foundation & Quick Wins](#-phase-1-foundation--quick-wins)
4. [Phase 2: Architecture & DX](#-phase-2-architecture--dx)
5. [Phase 3: Advanced Features](#-phase-3-advanced-features)
6. [Phase 4: Testing & Reliability](#-phase-4-testing--reliability)
7. [Phase 5: Security & Polish](#-phase-5-security--polish)
8. [Future Considerations (v5+)](#-future-considerations-v5)
9. [Decision Log](#-decision-log)

---

## 🎯 Guiding Principles

| Principle | What it means |
|-----------|---------------|
| **Backward compatible** | No breaking config/CLI changes between 3.x → 4.0 |
| **Opt-in new behavior** | Flags default to existing behavior |
| **Phased delivery** | Each phase is independently usable and shippable |
| **No feature bloat** | Every addition must be justified by CI/CD utility |

---

## ⚡ Go Version Upgrade

| Current | Target | Status |
|---------|--------|--------|
| `go 1.23.4` | `go 1.24.x` | 🔲 Not started |

**Benefit:** Performance improvements, new stdlib features, security fixes, `go tool` improvements.

**Action:**
```bash
go get go@1.24      # Update go.mod
go mod tidy          # Clean up deps
make all             # Full build + test
```

**Risk:** Low — drop-in upgrade for this codebase. No language features used that 1.24 would break.

---

## 📦 Phase 1: Foundation & Quick Wins

> **Goal:** Highest-value, lowest-effort improvements. Can ship individually as 3.x minor releases.

### 1.1 `--dry-run` Flag

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** CLI

**What:**
```bash
dockerz build --dry-run
# Output: Would build 4 of 7 services: api, backend, frontend, shared
# API gateway: would build (git changes detected)
# User service: would skip (no changes)
```

**Implementation:**
- Add `--dry-run` flag to `buildCmd` in `main.go`
- In build execution block (`main.go:389-399`), skip `BuildImages()`, print what *would* build
- Reuse existing orchestration + discovery logic — just don't execute Docker

**Files affected:**
- `cmd/dockerz/main.go` — new flag + early-exit path
- `internal/smart/orchestrator.go` — no change (reuse as-is)

**Verification:** `dockerz build --dry-run --smart --git-track` prints planned actions without building.

---

### 1.2 Shell Completions

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** CLI

**What:**
```bash
dockerz completion bash    # Generate bash completion script
dockerz completion zsh     # Generate zsh completion script
dockerz completion fish    # Generate fish completion script
```

**Implementation:**
- cobra provides `cmd.GenBashCompletion(os.Stdout)` etc.
- Add a `completion` subcommand using cobra's built-in completions
- Or add hidden command (cobra natively supports this)

**Files affected:**
- `cmd/dockerz/main.go` — new subcommand(s)

---

### 1.3 Config Validation Command

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** CLI

**What:**
```bash
dockerz config validate                # Validate build.yaml
dockerz config validate --config path  # Validate custom path
```

**Implementation:**
- New subcommand `config` with `validate` subcommand
- Reuse `config.LoadConfig()` + `config.ValidateTxtFile()`
- Add validation for:
  - YAML structure (existing `LoadConfig` handles this)
  - GAR fields completeness when `use_gar: true`
  - Input/output file extensions
  - Service directory existence
  - `max_processes` range (1-32)

**Files affected:**
- `cmd/dockerz/main.go` — new subcommand
- `internal/config/config.go` — optionally add more validators

---

### 1.4 Configurable Push Concurrency

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** Builder

**What:**
```yaml
# build.yaml
push_concurrency: 4   # Default: 2
```

```bash
dockerz build --push-concurrency 4    # CLI override
```

**Implementation:**
- Add `PushConcurrency` field to `Config` struct (`internal/config/types.go`)
- Replace hardcoded `2` at `parallel.go:72` with configured value
- Default to `2` for backward compatibility

**Files affected:**
- `internal/config/types.go` — new field
- `internal/config/config.go` — default logic
- `internal/builder/parallel.go` — use configured value
- `cmd/dockerz/main.go` — CLI flag

---

### 1.5 BuildKit Inline Cache Support

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** Builder

**What:**
```bash
dockerz build --cache-type inline    # Use BuildKit inline cache
dockerz build --cache-type registry  # Push cache to registry (default)
```

**Implementation:**
- Add `--cache-type` flag with values: `none`, `inline`, `registry`
- Pass `--cache-from=type=inline` and `--cache-to=type=inline` to `docker build`
- Current behavior = `--cache-type registry`

**Files affected:**
- `cmd/dockerz/main.go` — new flag
- `internal/builder/builder.go` — conditional BuildKit args

---

## 🏗️ Phase 2: Architecture & DX

> **Goal:** Code health, maintainability, developer quality-of-life.

### 2.1 Discovery Logic Refactor

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** Discovery

**What:**
Extract the shared `filepath.Walk` logic from three nearly-identical functions into a single helper.

**Current state** (`internal/discovery/discovery.go`):
- `discoverFromDirectories()`  (lines 148-205) — walks services_dirs
- `autoDiscoverServices()`     (lines 208-259) — walks project root
- Both are 90% identical (only differ in root path)

**Refactor:**
```go
// walkForDockerfiles walks a root path and returns all directories with Dockerfiles
func walkForDockerfiles(root string) ([]DiscoveredService, []error)
```

- `discoverFromDirectories` calls `walkForDockerfiles` for each dir
- `autoDiscoverServices` calls `walkForDockerfiles(".")`

**Files affected:**
- `internal/discovery/discovery.go` — extract + simplify

---

### 2.2 Cache Backend Interface

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** Cache

**What:**
Define a `CacheBackend` interface so the cache isn't tied to the filesystem.

```go
type CacheBackend interface {
    Get(serviceName string) (*CacheEntry, bool)
    Set(entry *CacheEntry) error
    Clear(serviceName string) error
    Cleanup() error
}
```

**Existing implementations (refactored):**
- `FileCache` (current behavior, renamed from `DistributedCache`)
- `InMemoryCache` (used for hot cache hits)

**New (future):**
- `RedisCache` — shared cache across CI runners
- `S3Cache` — durable cache in object storage

**Files affected:**
- `internal/cache/types.go` — new interface
- `internal/cache/distributed_cache.go` — rename to `file_cache.go`, implement interface
- `internal/cache/inmemory_cache.go` — new, extracted from current mixed impl
- `internal/smart/orchestrator.go` — use interface

---

### 2.3 Configurable Cache TTLs

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** Git, Cache

**What:**
```yaml
# build.yaml
git_cache_ttl: 5m      # Default: 5 minutes
cache_ttl: 24h          # Default: 24 hours (current: hardcoded)
```

**Implementation:**
- Add `GitCacheTTL` and `CacheTTL` to config
- Pass through to git cache and smart orchestrator
- Replace hardcoded `5*time.Minute` in `tracker.go`
- CLI override via `--git-cache-ttl`, `--cache-ttl`

**Files affected:**
- `internal/config/types.go` — new fields
- `internal/git/tracker.go` — use configurable TTL
- `internal/smart/orchestrator.go` — use configurable TTL
- `cmd/dockerz/main.go` — CLI flags

---

### 2.4 Resource Monitor Defaults Per Platform

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** Config

**What:**
Sensible defaults vary by environment. Instead of hardcoded 80/85/90:

```go
func defaultThresholds() (cpu, mem, disk float64) {
    // Containers/VMs: typically resource-constrained, be more conservative
    // Bare metal: can use higher thresholds
    // Default: 80/85/90 (current behavior)
}
```

**Files affected:**
- `internal/config/config.go` — platform-aware defaults

---

### 2.5 Better Error Context

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** All

**What:**
Wrap errors with structured context instead of flat strings.

```go
// Before
return fmt.Errorf("failed to get git diff: %w", err)

// After
return &BuildError{
    Service: servicePath,
    Step:    "git_diff",
    Cause:   err,
}
```

Or at minimum, use `fmt.Errorf("service %s: git diff failed: %w", servicePath, err)` consistently.

**Files affected:**
- All internal packages — audit error messages

---

## 🔧 Phase 3: Advanced Features

> **Goal:** Differentiator features that make dockerz genuinely powerful for CI/CD.

### 3.1 Build Dependency Graph

**Status:** 🔲 Not started | **Estimate:** High effort | **Area:** Smart / Discovery

**What:**
Parse Dockerfiles to detect inter-service dependencies, then build in dependency order.

**Example:**
```dockerfile
# services/api/Dockerfile
FROM base-image:latest
COPY --from=services/shared:latest /app/lib /app/lib      # depends on "shared"
```

When a change is detected in `services/shared`:
1. Build `shared` first
2. Build `api` (and any other dependents) second

**Implementation approach:**
- Analyze `Dockerfile` `FROM` statements (base image dependencies)
- Analyze `COPY --from=...` statements (artifact dependencies)
- Build dependency graph as DAG
- Parallelize at each level of the graph
- Skip unchanged services even if they have dependencies (unless deps changed)

**CLI:**
```bash
dockerz build --smart --deps  # Enable dependency resolution
```

**Files affected:**
- `internal/discovery/types.go` — add deps field
- `internal/smart/deps.go` — new file, dependency graph logic
- `internal/smart/orchestrator.go` — use deps for ordering

---

### 3.2 Multi-Platform Builds

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** Builder

**What:**
```bash
dockerz build --platform linux/amd64,linux/arm64
```

**Implementation:**
- Add `--platform` flag (comma-separated list)
- When set, use `docker buildx build --platform ...` instead of `docker build`
- Single flag applies to all services (per-service override later)

**Prerequisite:** Docker BuildKit with `buildx` installed.

**Config:**
```yaml
platforms:
  - linux/amd64
  - linux/arm64
```

**Files affected:**
- `cmd/dockerz/main.go` — new flag
- `internal/config/types.go` — `Platforms []string`
- `internal/builder/builder.go` — conditional buildx args

---

### 3.3 SBOM Generation

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** Builder

**What:**
```bash
dockerz build --sbom                    # Generate CycloneDX/Syft SBOM
dockerz build --sbom-format spdx        # SPDX format
```

**Output:** `<service>-sbom.json` for each built service.

**Implementation:**
- Configure `docker buildx build --sbom=true` when BuildKit is available
- Or run Syft as a post-build step: `syft packages <image> -o json -f <service>-sbom.json`

**Files affected:**
- `cmd/dockerz/main.go` — new flags
- `internal/builder/builder.go` — SBOM step

---

### 3.4 Vulnerability Scanning

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** Builder

**What:**
```bash
dockerz build --scan                   # Scan images for CVEs
dockerz build --scan-threshold high    # Fail build on high/CVEs
```

**Implementation:**
- Run `trivy image <image>` as post-build step
- Parse output, compare severity against threshold
- Fail the build if CVEs exceed threshold
- Generate report file

**Config:**
```yaml
scan:
  enabled: false
  scanner: trivy          # or grype
  fail_on: high           # none, low, medium, high, critical
  format: sarif           # sarif, json, table
```

**Files affected:**
- `cmd/dockerz/main.go` — new flags
- `internal/builder/` — scan execution
- `internal/builder/scanner.go` — new file

---

### 3.5 Build Metrics Export

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** Logging / Builder

**What:**
```bash
dockerz build --metrics build-metrics.json
```

**Output:**
```json
{
  "build_id": "abc123",
  "timestamp": "2026-06-11T10:00:00Z",
  "duration_ms": 45000,
  "services": {
    "api": { "status": "success", "duration_ms": 12000, "cache_hit": false },
    "backend": { "status": "skipped", "duration_ms": 0, "cache_hit": true }
  },
  "system": { "cpu_count": 8, "memory_gb": 16 }
}
```

**Implementation:**
- Collect metrics during build flow (already tracked in `BuildResult`, `Summary`)
- Export to JSON file at the end
- Later: Prometheus push gateway, OpenTelemetry

**Files affected:**
- `internal/builder/metrics.go` — new file, export logic
- `cmd/dockerz/main.go` — new flag

---

## 🧪 Phase 4: Testing & Reliability

> **Goal:** From "integration-only" to "properly tested at every level."

### 4.1 Unit Tests for All Internal Packages

**Status:** 🔲 Not started | **Estimate:** High effort | **Area:** All

| Package | Test Focus | Priority |
|---------|-----------|----------|
| `config` | Loading, validation, backward compat, sample gen | High |
| `discovery` | Explicits, directories, auto, input file, dedup, edge cases | High |
| `git` | Change detection, depth, caching, error handling | High |
| `cache` | Get/Set/Clear, TTL expiration, in-memory + file | Medium |
| `smart` | Decision logic, priority, GAR check, git integration | High |
| `builder` | Build task creation, summary calculation | Medium |
| `logging` | Category filtering, output formatting | Low |

**Implementation approach:**
- Table-driven tests (Go idiomatic)
- No external dependencies (Docker, git repos) — use interfaces
- `discovery` tests use `tests/test-project` as fixture (already exists)

**Files created:**
- `internal/config/config_test.go`
- `internal/discovery/discovery_test.go`
- `internal/git/tracker_test.go`
- `internal/cache/cache_test.go`
- `internal/smart/orchestrator_test.go`

---

### 4.2 Interface-Based Mocking

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** All

**What:**
Define interfaces for external dependencies so unit tests don't need Docker or a git repo.

**Key interfaces:**
```go
type DockerClient interface {
    Build(task BuildTask) BuildResult
    Push(image string) error
    ManifestInspect(image string) bool
}

type GitClient interface {
    GetChangedFiles(servicePath string, depth int) ([]string, error)
    PreloadChanges(depth int) error
    IsGitRepository(path string) bool
}
```

**Implementation:**
- Start with `GitClient` interface (most needed for testing)
- Move to `DockerClient` when builder tests are added
- Use `gomock` or handwritten mocks

**Files affected:**
- `internal/git/types.go` — interface
- `internal/git/tracker.go` — struct implements interface
- `internal/smart/orchestrator.go` — accept interface

---

### 4.3 CI Test Matrix

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** CI

**What:**
Extend GitHub Actions workflow to run tests across:

| Dimension | Values |
|-----------|--------|
| Go version | 1.22, 1.23, 1.24 |
| OS | ubuntu-latest, macos-latest (if feasible) |
| Test type | Unit (`go test --short`) + Integration |

**Files affected:**
- `.github/workflows/build-dockerz.yml` — add strategy matrix

---

### 4.4 Integration Test Automation

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** CI / Testing

**What:**
Create a script that automates the 50+ manual test scenarios in `tests/scenario.md`.

```bash
tests/run-scenarios.sh    # Run all scenarios
tests/run-scenario.sh 19  # Run specific scenario
```

**Implementation:**
- Each scenario becomes a shell function
- Assertions via `docker images`, `grep` on output, file existence
- CI job runs critical path scenarios on each push

**Files created:**
- `tests/run-scenarios.sh`
- `tests/scenario-runner/` (helper functions)

---

## 🛡️ Phase 5: Security & Polish

> **Goal:** Production-hardened, documentation-complete.

### 5.1 CI Security Checks

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** CI

**Add to CI workflow:**
```yaml
- name: govulncheck
  run: go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...
- name: staticcheck
  run: go install honnef.co/go/tools/cmd/staticcheck@latest && staticcheck ./...
- name: go vet
  run: go vet ./...
```

**Files affected:**
- `.github/workflows/build-dockerz.yml`

---

### 5.2 Image Signing (Cosign)

**Status:** 🔲 Not started | **Estimate:** Medium effort | **Area:** Builder

**What:**
```bash
dockerz build --sign                    # Sign images with cosign
dockerz build --cosign-key key.pem      # Specify signing key
```

**Implementation:**
- Post-build step: `cosign sign <image>`
- Requires `COSIGN_PASSWORD` or key file
- Verify step: `cosign verify <image>`

---

### 5.3 .gitignore Audit

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** Project

**Current state:** Ignores only `.agents/*` and itself. Missing standard Go entries.

**Fix:**
```
# Go
vendor/
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out

# Build artifacts
/dockerz
/dockerz_unix
/dockerz.exe
/dockerz_darwin

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Build cache
/tmp/dockerz-*/

# Logs
*.log
```

**Files affected:**
- `.gitignore`

---

### 5.4 Documentation Updates

**Status:** 🔲 Not started | **Estimate:** Low effort | **Area:** Documentation

- Update `README.md` to reflect all new features
- Update `-h` help text in CLI (many flags added)
- Update `AGENTS.md` (already created)
- Add examples directory: `examples/basic.yaml`, `examples/gar.yaml`, `examples/cicd.yaml`

---

## 🔭 Future Considerations (v5+)

| Idea | When | Why not now |
|------|------|-------------|
| **Web UI / Dashboard** | v5+ | Large scope, core CLI first |
| **Plugin system** | v5+ | Need to stabilize interfaces first |
| **Kubernetes integration** | v5+ | Out of scope for a Docker build tool |
| **Terraform provider** | v5+ | Niche use case |
| **AI-powered build optimization** | v5+ | Experimental, needs data from metrics |

---

## 📝 Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| — | Go 1.24 upgrade first | Foundation for everything else |
| — | `--dry-run` highest priority quick win | Most practical day-to-day benefit |
| — | Unit tests before Phase 3 features | Don't add features without coverage |
| — | Backward compatibility mandatory | Version 4.0 should not break 3.x configs |
| — | Replace renderer with display package | Cleaner, terminal-agnostic, real Docker output |
| — | Fix per-push completion timing | UX improvement: services transition to done immediately |
| — | Remove log.SetOutput(io.Discard) | No longer needed, improves error visibility |
| — | Wire ProgressCallback for real Docker output | Provides transparency into Docker build steps |
| — | Keep old renderer package during transition | Ensures backward compatibility during rollout |

---

## 🏁 Release Criteria for 4.0.0

- [ ] Go upgraded to 1.24.x, all tests passing
- [ ] All Phase 1 items implemented and documented
- [ ] At least 3 of 5 Phase 2 items complete
- [ ] All internal packages have unit tests
- [ ] CI includes security checks (govulncheck, staticcheck)
- [ ] `.gitignore` updated
- [ ] Help text and `README.md` current
- [ ] No breaking changes to CLI flags or config format
- [ ] Display package replaces renderer with terminal-agnostic live status
- [ ] Per-push completion timing fixed for better UX
- [ ] Real Docker build output visible in TTY mode

---

*Last updated: 2026-06-11*
