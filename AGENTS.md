# dockerz — Agent Instructions

> See [ROADMAP.md](./ROADMAP.md) for planned improvements toward v4.0.0.

Compact reference for Go CLI tool that builds/pushes Docker images in parallel with smart change detection.

## Workflow Rules

- **Phase steps for code/major changes only**: The "tell → approve → proceed → report" step-wise workflow applies only to code implementation and major architectural changes. Doc updates, AGENTS.md updates, and simple config changes do NOT require step-wise approval — just do them.
- **After each phase**: Update AGENTS.md, README.md, and CLI help text (via `main.go` Long/Short descriptions) to reflect new features.
- **Phase steps**: Each phase item is one step. For each step:
  1. Tell the user what you'll change and why
  2. Wait for approval
  3. Implement the change
  4. Run validation (`make all`)
  5. Report back results
  6. Stop before the next step

## Project Facts

- **Language**: Go 1.24, module `github.com/addy-47/dockerz`
- **Entry point**: `cmd/dockerz/main.go`
- **Config**: YAML (`build.yaml`), loaded via viper, overridden by CLI flags (`cmd.Flags().Changed()` check per field)
- **CLI framework**: cobra with 5 commands: `dockerz`, `dockerz init`, `dockerz build`, `dockerz config`, `dockerz completion`
- **No env var support** for config — use CLI flags or YAML only
- **CLI flag prefix style**: kebab-case (`--git-track`, `--max-processes`)

## Architecture (8 internal modules)

| Module | Path | Purpose |
|--------|------|---------|
| builder | `internal/builder/` | Parallel Docker builds, push manager, resource monitoring |
| cache | `internal/cache/` | Layer, hash, registry, and distributed caching |
| config | `internal/config/` | YAML loading, validation, sample generation |
| discovery | `internal/discovery/` | Service auto-discovery, input file parsing |
| display | `internal/display/` | Live status display (TTY in-place or non-TTY line output) |
| git | `internal/git/` | Change detection, diff analysis, caching |
| logging | `internal/logging/` | Docker Compose-style console output + structured file logging |
| smart | `internal/smart/` | Build orchestration decisions |

## Key Commands

```bash
# Build binary
go build -o dockerz ./cmd/dockerz

# Run all tests (verbose)
go test -v ./...

# Run tests for a single package
go test -v ./internal/config/

# Tidy dependencies
go mod tidy && go mod download

# Generate sample config
dockerz init

# Validate configuration
dockerz config validate
dockerz config validate --config path/to/build.yaml

# Generate shell completions
dockerz completion bash
dockerz completion zsh
dockerz completion fish

# Dry run (preview builds without executing)
dockerz build --smart --dry-run

# Full smart build (recommended CI/CD pattern)
dockerz build --smart --git-track --cache --max-processes 6

# Advanced build with push concurrency and cache type
dockerz build --smart --git-track --cache --push-concurrency 4 --cache-type registry
```

### Makefile Targets

| Target | Command | Notes |
|--------|---------|-------|
| `build` | `go build -o dockerz ./cmd/dockerz` | Builds binary (silent) |
| `test` | `go test -v ./...` | Recursive tests |
| `all` | `test build` | Test then build (order matters) |
| `clean` | go clean + remove binaries | |
| `run` | build + execute | |
| `deps` | `go mod download && go mod tidy` | |
| `build-linux` | CGO_ENABLED=0 cross-compile | |
| `build-windows` | CGO_ENABLED=0 cross-compile | |
| `build-darwin` | CGO_ENABLED=0 cross-compile | |

## Config File (`build.yaml`)

- **services_dir**: can be `string` (comma-separated) or `[]string` in YAML — backward compatibility handled in code
- **services**: explicit list with optional `image_name` and `tag` per service; `name` field doubles as service directory **path**
- **registry_url**: OCI-compatible registry URL for any registry (GAR/ECR/ACR/generic); auto-detects type from URL pattern
- **push_to_registry**: push built images to registry after successful build
- **global_tag**: defaults to **git commit hash** if empty, not `latest`
- **max_processes**: 0 = use 4 (fallback); docs claim CPU/2 but code defaults to 4
- **Input/output changed services**: files must use `.txt` extension (validated in config)
- **BuildKit**: enabled by default (`enable_buildkit: true`)

CLI flags override YAML, but only when explicitly set (`cmd.Flags().Changed()` check). Boolean flags (`--smart`, `--git-track`, `--cache`, `--force`) will override only if explicitly passed; they don't toggle from the default.

## Smart Features (All Opt-In)

- `--smart` — enables orchestration logic
- `--git-track` — git change detection (requires git repo)
- `--depth` — git tracking depth (default 2 commits, 0 = full history)
- `--cache` — multi-level caching
- `--force` — rebuild everything, overrides all smart logic
- `--dry-run` — preview what would be built without executing Docker
- `--push-concurrency` — max concurrent image pushes (default 2)
- `--cache-type` — BuildKit cache mode: `none`, `inline`, `registry` (default `inline`)

### Smart Decision Priority

1. Force rebuild (`--force`) → always build
2. Registry check (GAR) → skip if image already exists
3. Git changes → build if files changed
4. No changes → skip build

When smart is disabled but `--git-track` is enabled, all services are built but git changes are still logged.

## Service Discovery (Unified)

Services are collected from **all sources** additively, then deduplicated:
1. Explicit `services` list in YAML
2. `services_dir` scanning
3. Auto-discovery (directories with `Dockerfile`)
4. `--input-changed-services` file (newline-separated service names)

Discovery validation: each service must have an actual `Dockerfile` at its path — services without one are skipped with warnings.

## Registry Integration (v3.2.2)

- **Generic**: `--registry-url` accepts any OCI-compatible registry URL
- **Auto-detection**: Registry type (GAR/ECR/ACR/generic) detected from URL pattern
- **Auth**: Each registry requires its own auth (gcloud for GAR, aws for ECR, az for ACR, docker login for Docker Hub)
- **GAR-specific**: Uses `docker manifest inspect` to check image existence (fast, no pull); requires all three fields: `project`, `gar`, `region`

## CI/CD

**GitHub Actions** (`.github/workflows/build-dockerz.yml`):
- Triggers on push to `master` affecting `cmd/`, `internal/`, `debian/`, `go.mod`, `go.sum`, `build.yaml`
- Builds Debian package via `dpkg-buildpackage`
- Publishes to `install` branch as an apt repository
- Go version: `1.24`

**GoReleaser** (`.goreleaser.yml`): Cross-compiles for linux/windows/darwin on amd64/arm64.

**Debian packaging** (`debian/` directory): used by CI; `dpkg-buildpackage -us -uc -b` to build.

## Testing

- Standard Go tests: `go test -v ./...`
- Integration test project at `tests/test-project/` — contains 7 discoverable services (directories with Dockerfiles) plus 3 non-Dockerfile directories for edge cases
- 50+ documented test scenarios in `tests/scenario.md` covering discovery, input files, smart features, caching, edge cases
- Test scenarios include exact commands, expected outcomes, and setup steps with git commits
- Quick sandbox at `tests/sandbox/` — 3 services (api, worker, frontend) with 1-3s sleeps. Run from sandbox dir: `../../dockerz build --force`
- To test: build binary, `cd tests/test-project`, run `dockerz build` with desired flags
- Verify built images: `docker images | grep -E "(api|backend|frontend|shared)"`

## Known Quirks & Gotchas

- **Console logging uses Docker Compose style** — no timestamps or `[INFO]` prefixes on console; just clean messages with ` ✔ ` / ` ⚠ ` / ` ✗ ` for warnings/errors. File logger (`build.log`) retains full `[HH:MM:SS] LEVEL: CATEGORY: message` format. Debug-level messages (`logger.Debug`) only go to file, not console.
- **`logging` module exists** but is undocumented in README architecture section — it's used throughout the codebase
- **`.gitignore` covers Go/std patterns** but has historically been described as minimal — verify it's up to date
- **Cloud Build YAML exists** at `tests/cloudbuild-dockerz.yaml` — uses GKE deployment with SSH-based git auth and requires many substitutions
- **`services_dir` has backward compatibility hacks** — supports both string and string array in YAML
- **Banner is printed on root command** — `dockerz --help` shows banner before help text (via custom HelpFunc), but `dockerz --version` prints just the version string (version check happens before banner)
- **`--depth 0` does not mean full history** — it is reset to `2` in all code paths (orchestrator.go `GetChangedFiles` lines 93-96 & 150-152, tracker.go `PreloadChanges` line 23, tracker.go `getCommitChanges` line 297). Full history scanning is effectively unavailable at runtime.
- **Host system needs Docker** and optionally GAR auth for full integration tests
- **No test fixtures or mocks** for internal packages — testing relies on the `tests/test-project/` integration setup
- **Phase 1 was done as a batch** (Go upgrade + 6 feature additions) instead of individual steps — retroactively added workflow rules here
- **`config validate` reuses `config.LoadConfig`** and thus only catches errors that `LoadConfig` surfaces — it may miss structural YAML issues in unused fields
- **`--cache-type` defaults to `"inline"`** in the builder (builder.go), but the config default is `""` (empty) — the CLI flag doesn't have a default string, and `cmd.Flags().Changed()` logic means an empty string from CLI gets passed through if flag is changed but empty. In practice, the builder handles the fallback.
- **Progress renderer uses `Shutdown()` not `Wait()`** — mpb v8.12.1 auto-refresh mode never calls `b.cancel()` when bars complete via `SetTotal(-1, true)`. `bar.serve()` only exits on `b.ctx.Done()`, but `b.cancel()` is only called in manual-refresh mode. `Wait()` deadlocks (`bwg.Wait()` blocks before `Shutdown()`'s context cancel). The fix: call `Progress.Shutdown()` directly, which cancels the parent context and cascades to all bar child contexts, causing `serve()` to exit and `bwg.Done()` to fire.
- **Fatal errors before progress renderer start are silently swallowed in TTY mode** — `log.SetOutput(io.Discard)` is called early in the build command to keep the terminal clean for progress bars. Any `log.Fatalf` call before the progress renderer starts (e.g., `CheckRegistryAuth` failure) writes to `io.Discard`. Fixed in v3.2.2: all early fatal errors now use `fmt.Fprintf(os.Stderr, ...); os.Exit(1)` instead of `log.Fatalf`.
