package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/addy-47/dockerz/internal/builder"
	"github.com/addy-47/dockerz/internal/cache"
	"github.com/addy-47/dockerz/internal/config"
	"github.com/addy-47/dockerz/internal/discovery"
	"github.com/addy-47/dockerz/internal/display"
	"github.com/addy-47/dockerz/internal/git"
	"github.com/addy-47/dockerz/internal/logging"
	"github.com/addy-47/dockerz/internal/smart"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	configPath            string
	maxProcesses          int
	gitTrack              bool
	depth                 int
	cacheEnabled          bool
	forceRebuild          bool
	smartEnabled          bool
	project               string
	region                string
	gar                   string
	registryURL           string
	globalTag             string
	inputChangedServices  string
	outputChangedServices string
	useGAR                bool
	pushToGAR             bool
	pushToRegistry        bool
	servicesDir           string
	version               bool
	dryRun                bool
	pushConcurrency       int
	cacheType             string
	gitCacheTTL           string
	cacheTTL              string
)

// PrintDockerzBanner prints the ASCII art banner with vibrant colors
func PrintDockerzBanner() {
	// Define premium color palette
	magenta := color.New(color.FgHiMagenta).Add(color.Bold)
	cyan := color.New(color.FgHiCyan).Add(color.Bold)
	green := color.New(color.FgHiGreen).Add(color.Bold)
	white := color.New(color.FgHiWhite)
	yellow := color.New(color.FgHiYellow)

	// ASCII art for "dockerz" with a color gradient feel
	fmt.Println()
	magenta.Println(`     _            _                    `)
	magenta.Println(`  __| | ___   ___| | _____ _ __ ____  `)
	cyan.Println(` / _' |/ _ \ / __| |/ / _ \ '__|_  /  `)
	cyan.Println(`| (_| | (_) | (__|   <  __/ |   / /   `)
	green.Println(` \__,_|\___/ \___|_|\_\___|_|  /___|  `)
	fmt.Println()

	white.Printf(" %s ", color.New(color.BgHiMagenta, color.FgHiWhite).Sprint(" v3.2.2 "))
	yellow.Println(" The ultimate Docker companion for smart, parallel builds")
	fmt.Println()
}

var rootCmd = &cobra.Command{
	Use:   "dockerz",
	Short: "🚀 Dockerz - Supercharge your Docker builds with smart orchestration",
	Long: `Dockerz (v3.2.2) is a high-performance Docker orchestration tool designed for monorepos and complex CI/CD pipelines.

It intelligently analyzes your repository using Git tracking, skips unchanged services, 
and leverages multi-level caching to reduce build times by up to 90%.

Quick Start:
  1. Initialize:  dockerz init
  2. Configure:   Edit build.yaml
  3. Build:       dockerz build --smart`,
	Run: func(cmd *cobra.Command, args []string) {
		if version {
			fmt.Println("dockerz version 3.2.2")
			return
		}
		PrintDockerzBanner()
		fmt.Println("Ready to accelerate. Use 'dockerz --help' to see all commands.")
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new project with sample configuration",
	Long:  `Create a sample build.yaml configuration file in the current directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := config.SaveSampleConfig("build.yaml"); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				fmt.Printf("ℹ %v. Skipping initialization.\n", err)
				return
			}
			fmt.Fprintf(os.Stderr, "Failed to create sample config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Created sample build.yaml")
		fmt.Println("\nNext steps:")
		fmt.Println("1. Edit build.yaml to configure your services")
		fmt.Println("2. Run 'dockerz build' to build your images")
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Validate and inspect configuration",
	Long:  `Manage and validate dockerz configuration.`,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the build.yaml configuration file",
	Long: `Validate the build.yaml configuration file for correctness.
Checks YAML structure, required fields, file paths, and service directories.

Examples:
  dockerz config validate
  dockerz config validate --config path/to/build.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		cfgPath := configPath
		if cfgPath == "" {
			cfgPath = "build.yaml"
		}

		fmt.Printf("Validating configuration: %s\n", cfgPath)

		// Load and validate the config
		cfg, err := config.LoadConfig(cfgPath)
		if err != nil {
			fmt.Printf("❌ Configuration validation failed:\n   %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ YAML structure valid")

		// Validate service directories
		if len(cfg.Services) > 0 {
			fmt.Printf("✅ %d explicit services defined\n", len(cfg.Services))
			for _, svc := range cfg.Services {
				fmt.Printf("   - %s", svc.Name)
				if svc.ImageName != "" {
					fmt.Printf(" (image: %s)", svc.ImageName)
				}
				if svc.Tag != "" {
					fmt.Printf(" (tag: %s)", svc.Tag)
				}
				fmt.Println()
			}
		}

		if len(cfg.ServicesDir) > 0 {
			fmt.Printf("✅ %d service directories configured\n", len(cfg.ServicesDir))
			for _, dir := range cfg.ServicesDir {
				fmt.Printf("   - %s\n", dir)
			}
		}

		if cfg.RegistryURL != "" {
			fmt.Printf("✅ Registry configured: %s\n", cfg.RegistryURL)
			if cfg.PushToRegistry {
				fmt.Printf("✅ Push to registry enabled\n")
			}
		}

		if cfg.InputChangedServices != "" {
			fmt.Printf("✅ Input changed services file: %s\n", cfg.InputChangedServices)
		}

		if cfg.OutputChangedServices != "" {
			fmt.Printf("✅ Output changed services file: %s\n", cfg.OutputChangedServices)
		}

		fmt.Printf("\n📋 Configuration summary:\n")
		fmt.Printf("   Max processes:     %d\n", cfg.MaxProcesses)
		fmt.Printf("   Global tag:        %s\n", cfg.GlobalTag)
		fmt.Printf("   BuildKit:          %v\n", cfg.EnableBuildKit)
		fmt.Printf("   Push concurrency:  %d\n", cfg.PushConcurrency)
		fmt.Printf("   Cache type:        %s\n", cfg.CacheType)
		fmt.Printf("   Git cache TTL:     %s\n", cfg.GitCacheTTL)
		fmt.Printf("   Cache TTL:         %s\n", cfg.CacheTTL)
		fmt.Printf("   Smart:             %v\n", cfg.Smart)
		fmt.Printf("   Git track:         %v\n", cfg.GitTrack)
		fmt.Printf("   Cache:             %v\n", cfg.Cache)
		fmt.Printf("   Force rebuild:     %v\n", cfg.Force)

		fmt.Println("\n✅ Configuration is valid")
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for dockerz.

To use, source the output in your shell:

  Bash:
    source <(dockerz completion bash)
    # or save to file:
    dockerz completion bash > /etc/bash_completion.d/dockerz

  Zsh:
    source <(dockerz completion zsh)
    # or save to file:
    dockerz completion zsh > /usr/local/share/zsh/site-functions/_dockerz

  Fish:
    dockerz completion fish > ~/.config/fish/completions/dockerz.fish`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		shell := args[0]
		switch shell {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		default:
			fmt.Fprintf(os.Stderr, "Unsupported shell: %s. Use: bash, zsh, or fish\n", shell)
			os.Exit(1)
		}
	},
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "🔨 Execute smart builds and pushes",
	Long: `Build and push Docker images with intelligent orchestration.

Dockerz Build intelligently manages:
  - 🧠 Smart Change Detection: Only rebuild what changed since the last build/commit.
  - 🚄 Parallel Execution: Utilize all CPU cores for simultaneous builds and pushes.
  - 💾 Multi-Level Caching: Layer caching + Local hash caching + Registry existence checks.
  - ☁️ Multi-Registry Support: GAR, AWS ECR, Docker Hub, self-hosted — any OCI registry.

Examples:
  dockerz build --smart --git-track --cache
  dockerz build --registry-url my-registry.io/project --push-to-registry --global-tag v1.0.1
  dockerz build --services-dir=api,worker --max-processes=4`,
	Run: func(cmd *cobra.Command, args []string) {

		// Initialize comprehensive logging
		logger, err := logging.NewLogger("build.log")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
			os.Exit(1)
		}
		defer logger.Close()

		// Check if stderr is a TTY — determines whether we use live
		// in-place status updates (cursor-up + carriage-return) or
		// simple line-by-line output.
		isTTY := term.IsTerminal(int(os.Stderr.Fd()))
		liveDisplay := isTTY && !dryRun

		// Load configuration
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			logger.Error(logging.CATEGORY_CONFIG, fmt.Sprintf("Failed to load config: %v", err))
			fmt.Fprintf(os.Stderr, "Error: Failed to load config: %v\n", err)
			os.Exit(1)
		}
		logger.Info(logging.CATEGORY_CONFIG, fmt.Sprintf("Loaded config from %s", configPath))

		// Validate registry authentication if a registry is configured
		if cfg.RegistryURL != "" {
			if err := builder.CheckRegistryAuth(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error: Registry authentication failed: %v\n", err)
				os.Exit(1)
			}
		}

		// Get default tag (short Git commit ID) if global_tag is not specified
		// Check if global tag was explicitly provided via CLI flag
		var defaultTag string
		if cmd.Flags().Changed("global-tag") {
			defaultTag = cfg.GlobalTag
		} else if cfg.GlobalTag == "" {
			defaultTag = builder.GetGitCommitID()
		} else {
			defaultTag = cfg.GlobalTag
		}

		// Override config with CLI flags if provided
		if cmd.Flags().Changed("git-track") {
			cfg.GitTrack = gitTrack
			cfg.GitTrackDepth = depth
		}
		if cmd.Flags().Changed("cache") {
			cfg.Cache = cacheEnabled
		}
		if cmd.Flags().Changed("force") {
			cfg.Force = forceRebuild
		}
		if cmd.Flags().Changed("smart") {
			cfg.Smart = smartEnabled
		}
		if cmd.Flags().Changed("registry-url") {
			cfg.RegistryURL = registryURL
		}
		// Deprecated GAR flags — map to RegistryURL if set
		if cmd.Flags().Changed("project") {
			cfg.Project = project
		}
		if cmd.Flags().Changed("region") {
			cfg.Region = region
		}
		if cmd.Flags().Changed("gar") {
			cfg.GAR = gar
		}
		if cmd.Flags().Changed("use-gar") {
			cfg.UseGAR = useGAR
			// Auto-construct registry_url from deprecated GAR fields if not already set
			if cfg.RegistryURL == "" && cfg.Project != "" && cfg.Region != "" && cfg.GAR != "" {
				cfg.RegistryURL = fmt.Sprintf("%s-docker.pkg.dev/%s/%s", cfg.Region, cfg.Project, cfg.GAR)
			}
		}
		if cmd.Flags().Changed("push-to-gar") || cmd.Flags().Changed("push-to-registry") {
			cfg.PushToGAR = pushToGAR
			cfg.PushToRegistry = pushToGAR || pushToRegistry
		}
		if cmd.Flags().Changed("push-concurrency") {
			cfg.PushConcurrency = pushConcurrency
		}
		if cmd.Flags().Changed("cache-type") {
			cfg.CacheType = cacheType
		}
		if servicesDir != "" {
			// Parse comma-separated services directories
			dirs := strings.Split(servicesDir, ",")
			for i, dir := range dirs {
				dirs[i] = strings.TrimSpace(dir)
			}
			cfg.ServicesDir = dirs
		}

		// Handle input/output changed services files with proper priority:
		// CLI flag takes precedence over YAML config, YAML config used when no CLI flag
		var effectiveInputFile string
		if cmd.Flags().Changed("input-changed-services") {
			effectiveInputFile = inputChangedServices
		} else if cfg.InputChangedServices != "" {
			effectiveInputFile = cfg.InputChangedServices
		}

		var effectiveOutputFile string
		if cmd.Flags().Changed("output-changed-services") {
			effectiveOutputFile = outputChangedServices
		} else if cfg.OutputChangedServices != "" {
			effectiveOutputFile = cfg.OutputChangedServices
		}

		// Validate input file extension if provided (either from CLI flag or YAML config)
		if effectiveInputFile != "" {
			if err := config.ValidateTxtFile(effectiveInputFile); err != nil {
				logger.Error(logging.CATEGORY_CONFIG, fmt.Sprintf("Invalid input changed services file: %v", err))
				fmt.Fprintf(os.Stderr, "Invalid input changed services file: %v\n", err)
				os.Exit(1)
			}
		}

		// Set package-level loggers for discovery and builder packages
		discovery.SetLogger(logger)
		builder.SetLogger(logger)

		// Discover services (unified discovery including input file)
		discoveryResult, err := discovery.DiscoverServices(cfg, defaultTag, effectiveInputFile)
		if err != nil {
			logger.Error(logging.CATEGORY_DISCOVERY, fmt.Sprintf("Failed to discover services: %v", err))
			fmt.Fprintf(os.Stderr, "Error: Failed to discover services: %v\n", err)
			os.Exit(1)
		}

		// Validate output file extension if provided (either from CLI flag or YAML config)
		if effectiveOutputFile != "" {
			if err := config.ValidateTxtFile(effectiveOutputFile); err != nil {
				logger.Error(logging.CATEGORY_CONFIG, fmt.Sprintf("Invalid output changed services file: %v", err))
				fmt.Fprintf(os.Stderr, "Invalid output changed services file: %v\n", err)
				os.Exit(1)
			}
		}

		// Log any discovery errors
		for _, discoveryErr := range discoveryResult.Errors {
			logger.Error(logging.CATEGORY_DISCOVERY, fmt.Sprintf("Discovery error: %v", discoveryErr))
		}

		// Smart orchestration if enabled (disabled by default for basic builds)
		var servicesToBuild []discovery.DiscoveredService
		var changedFiles map[string][]string // Track changed files for output

		if cfg.Smart {
			logger.Debug(logging.CATEGORY_SMART, "Smart build orchestration enabled")

			smartConfig := &smart.SmartConfig{
				Enabled:       cfg.Smart,
				GitTracking:   cfg.GitTrack,
				GitTrackDepth: cfg.GitTrackDepth,
				CacheEnabled:  cfg.Cache,
				CacheLevel:    cache.RegistryCacheLevel, // Default to registry cache
				CacheTTL:      24 * time.Hour,           // 24 hours TTL
				ForceRebuild:  cfg.Force,
			}

			orchestrator := smart.NewOrchestrator(smartConfig)
			orchestrator.SetLogger(logger)
			result, err := orchestrator.OrchestrateBuilds(cfg, discoveryResult.Services)
			if err != nil {
				logger.Error(logging.CATEGORY_SMART, fmt.Sprintf("Failed to orchestrate builds: %v", err))
				fmt.Fprintf(os.Stderr, "Error: Failed to orchestrate builds: %v\n", err)
				os.Exit(1)
			}

			logger.Debug(logging.CATEGORY_SMART, orchestrator.GetStats(result))

			// Log detailed decisions for each service
			for serviceName, decision := range result.Decisions {
				switch decision {
				case smart.ForceBuild:
					logger.Debug(logging.CATEGORY_SMART, fmt.Sprintf("%s: FORCE_BUILD (configured)", serviceName))
				case smart.ConditionalBuild:
					logger.Debug(logging.CATEGORY_SMART, fmt.Sprintf("%s: BUILD (changes detected)", serviceName))
				case smart.SkipBuild:
					logger.Debug(logging.CATEGORY_SMART, fmt.Sprintf("%s: SKIP (no changes)", serviceName))
				}
			}

			// Filter services that need building
			for i, service := range discoveryResult.Services {
				if decision, exists := result.Decisions[service.Name]; exists && decision != smart.SkipBuild {
					servicesToBuild = append(servicesToBuild, service)
					// Update service with smart info
					if i < len(result.ServiceStates) {
						state := result.ServiceStates[i]
						servicesToBuild[len(servicesToBuild)-1].CurrentHash = state.CurrentHash
						servicesToBuild[len(servicesToBuild)-1].ChangedFiles = state.ChangedFiles
						servicesToBuild[len(servicesToBuild)-1].NeedsBuild = true
					}
				}
			}
		} else {
			// For non-smart builds, build all services but check for git changes if requested
			servicesToBuild = discoveryResult.Services

			// If git tracking is enabled but smart is disabled, check for changes
			if cfg.GitTrack {
				logger.Debug(logging.CATEGORY_GIT, fmt.Sprintf("Git tracking enabled (depth: %d)", cfg.GitTrackDepth))
				changedFiles = make(map[string][]string)
				gitTracker := git.NewTracker()
				changesFound := false
				for _, service := range servicesToBuild {
					depth := cfg.GitTrackDepth
					if depth == 0 {
						depth = 2
					}
					if files, err := gitTracker.GetChangedFiles(service.Path, depth); err == nil && len(files) > 0 {
						changedFiles[service.Path] = files
						changesFound = true
						logger.Debug(logging.CATEGORY_GIT, fmt.Sprintf("Changes found in %s: %d files", service.Name, len(files)))
					}
				}
				if !changesFound {
					logger.Debug(logging.CATEGORY_GIT, "No git changes detected in any service")
				}
			}

			// Mark all as needing build when smart features disabled
			for i := range servicesToBuild {
				servicesToBuild[i].NeedsBuild = true
			}
		}

		// Root feature: Write changed services to file if requested (works with any command)
		if effectiveOutputFile != "" {
			var servicesForOutput []discovery.DiscoveredService

			if cfg.Smart && len(servicesToBuild) < len(discoveryResult.Services) {
				servicesForOutput = servicesToBuild
			} else if cfg.GitTrack && len(changedFiles) > 0 {
				for _, service := range servicesToBuild {
					if _, hasChanges := changedFiles[service.Path]; hasChanges {
						servicesForOutput = append(servicesForOutput, service)
					}
				}
			} else {
				servicesForOutput = servicesToBuild
			}

			if err := discovery.WriteChangedServicesFile(servicesForOutput, effectiveOutputFile); err != nil {
				logger.Warn(logging.CATEGORY_CONFIG, fmt.Sprintf("Failed to write changed services file: %v", err))
			} else {
				logger.Info(logging.CATEGORY_CONFIG, fmt.Sprintf("Changed services written: %d services", len(servicesForOutput)))
			}
		}

		// Dry-run: print what would be built and exit
		if dryRun {
			logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Dry-run: %d of %d services would be built", len(servicesToBuild), len(discoveryResult.Services)))
			for _, service := range servicesToBuild {
				logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("  Would build: %s (%s:%s)", service.Name, service.ImageName, service.Tag))
			}
			for _, service := range discoveryResult.Services {
				isBuilding := false
				for _, toBuild := range servicesToBuild {
					if toBuild.Name == service.Name {
						isBuilding = true
						break
					}
				}
				if !isBuilding {
					logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("  Would skip:  %s (no changes detected)", service.Name))
				}
			}
			return
		}

		// Resolve max processes (CLI flag overrides config)
		maxProcs := maxProcesses
		if maxProcs == 0 {
			maxProcs = cfg.MaxProcesses
		}

		// Create new discovery result with filtered services
		filteredResult := &discovery.DiscoveryResult{
			Services: servicesToBuild,
			Errors:   discoveryResult.Errors,
		}

		// Build images in parallel

		// Create live status display for terminal output (skipped for non-TTY and dry-run).
		// Shows real Docker --progress=plain step lines per service.
		var statusDisplay *display.Display
		if liveDisplay {
			// Build a clean status header with key config details
			line1 := []string{fmt.Sprintf("Discovered %d services", len(servicesToBuild))}
			if defaultTag != "" {
				shortTag := defaultTag
				if len(shortTag) > 8 {
					shortTag = shortTag[:8]
				}
				line1 = append(line1, fmt.Sprintf("tag: %s", shortTag))
			}
			if cfg.RegistryURL != "" && cfg.PushToRegistry {
				line1 = append(line1, "push")
			}
			line1 = append(line1, fmt.Sprintf("max %d", maxProcs))

			line2 := []string{}
			if cfg.Smart {
				line2 = append(line2, "smart")
			}
			if cfg.GitTrack {
				line2 = append(line2, "git-track")
			}
			if cfg.Force {
				line2 = append(line2, "force")
			}
			if cfg.Cache {
				line2 = append(line2, "cache")
			}
			if cfg.CacheType != "" && cfg.Cache {
				line2 = append(line2, fmt.Sprintf("cache-type: %s", cfg.CacheType))
			}

			headerText := strings.Join(line1, "  |  ")
			if len(line2) > 0 {
				headerText += "\n  " + strings.Join(line2, "  |  ")
			}

			statusDisplay = display.New(os.Stderr, true)
			serviceNames := make([]string, len(servicesToBuild))
			for i, svc := range servicesToBuild {
				serviceNames[i] = svc.Name
			}
			statusDisplay.Start(serviceNames, maxProcs, headerText)

			// Set up SIGINT handler for clean Ctrl+C exit.
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT)
			defer signal.Stop(sigChan)
			go func() {
				<-sigChan
				statusDisplay.Stop()
				os.Exit(130)
			}()
		}

		startBuildTime := time.Now()
		_, summary := builder.BuildImages(cfg, filteredResult, maxProcs, statusDisplay)
		buildDuration := time.Since(startBuildTime)

		// Stop the live display and print the final summary to stderr.
		if statusDisplay != nil {
			statusDisplay.Stop()
		}

		// Log build summary to build.log (and stdout in non-TTY mode).
		logger.PrintSummary(map[string]interface{}{
			"services_discovered": len(discoveryResult.Services),
			"services_built":      summary.SuccessfulBuilds,
			"services_failed":     summary.FailedBuilds,
			"build_duration":      buildDuration,
		})

		// Exit with error code if there were build failures
		if summary.FailedBuilds > 0 {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	// Set custom help function for root command
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		PrintDockerzBanner()
		// Get the default help template and execute it
		cmd.Println(cmd.UsageString())
	})

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(completionCmd)

	configCmd.AddCommand(configValidateCmd)

	rootCmd.Flags().BoolVarP(&version, "version", "v", false, "Print version information")

	buildCmd.Flags().StringVarP(&configPath, "config", "c", "build.yaml", "Path to the build.yaml configuration file (default: build.yaml)")
	buildCmd.Flags().IntVarP(&maxProcesses, "max-processes", "m", 0, "Maximum number of parallel build processes (0 = use system default; overrides config file)")
	buildCmd.Flags().StringVar(&registryURL, "registry-url", "", "OCI-compatible registry URL (e.g., us-central1-docker.pkg.dev/my-project/my-repo, 123456.dkr.ecr.us-east-1.amazonaws.com/my-repo). Leave empty for local-only builds.")
	buildCmd.Flags().StringVar(&project, "project", "", "Google Cloud Platform project ID (used with --gar and --region for GAR; alternative to --registry-url)")
	buildCmd.Flags().StringVar(&region, "region", "", "GCP region for GAR (e.g., us-central1, europe-west1; alternative to --registry-url)")
	buildCmd.Flags().StringVar(&gar, "gar", "", "Name of the Google Artifact Registry repository (alternative to --registry-url)")
	buildCmd.Flags().StringVar(&globalTag, "global-tag", "", "Global Docker tag to apply to all built images (overrides config file and git commit ID)")
	buildCmd.Flags().StringVar(&servicesDir, "services-dir", "", "Comma-separated list of directories to scan for service definitions (overrides config file)")
	buildCmd.Flags().StringVar(&inputChangedServices, "input-changed-services", "", "Path to a file containing a newline-separated list of service names to build selectively")
	buildCmd.Flags().StringVar(&outputChangedServices, "output-changed-services", "", "Path to output file where the list of changed services will be written for CI/CD integration")

	buildCmd.Flags().BoolVar(&gitTrack, "git-track", false, "Enable git change tracking")
	buildCmd.Flags().IntVar(&depth, "depth", 2, "Git tracking depth (0 for full history, default 2)")

	buildCmd.Flags().BoolVar(&cacheEnabled, "cache", false, "Enable multi-level build caching (layer, local hash, and registry cache)")
	buildCmd.Flags().BoolVar(&forceRebuild, "force", false, "Force rebuild of all services, ignoring cache and change detection")
	buildCmd.Flags().BoolVar(&smartEnabled, "smart", false, "Enable smart build orchestration with automatic dependency analysis and optimization")
	buildCmd.Flags().BoolVar(&useGAR, "use-gar", false, "Use Google Artifact Registry naming convention for image tags (alternative to --registry-url)")
	buildCmd.Flags().BoolVar(&pushToGAR, "push-to-gar", false, "Push built images to Google Artifact Registry (alternative to --push-to-registry)")
	buildCmd.Flags().BoolVar(&pushToRegistry, "push-to-registry", false, "Automatically push built images to the configured registry after successful builds")

	// Phase 1 new flags
	buildCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be built without executing Docker builds")
	buildCmd.Flags().IntVar(&pushConcurrency, "push-concurrency", 0, "Maximum concurrent image pushes to registry (default: 2)")
	buildCmd.Flags().StringVar(&cacheType, "cache-type", "", "BuildKit cache mode: none, inline, registry (default: inline)")

	// Cache TTL flags
	buildCmd.Flags().StringVar(&gitCacheTTL, "git-cache-ttl", "", "Git operation cache TTL (Go duration format, e.g. \"5m\", \"1h\")")
	buildCmd.Flags().StringVar(&cacheTTL, "cache-ttl", "", "Build cache TTL (Go duration format, e.g. \"24h\", \"72h\")")

	// Reuse config path flag for config validate command
	configValidateCmd.Flags().StringVarP(&configPath, "config", "c", "build.yaml", "Path to the build.yaml configuration file")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
