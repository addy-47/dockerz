package renderer

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/term"

	"github.com/addy-47/dockerz/internal/logging"
)

// ProgressRenderer manages live terminal progress bars during a build.
// It uses mpb for multi-progress bar rendering.
type ProgressRenderer struct {
	mu          sync.Mutex
	progress    *mpb.Progress
	bars        map[string]*mpb.Bar
	states      map[string]*ProgressState
	startTime   time.Time
	serviceCnt  int
	maxParallel int
	logger      *logging.Logger
	results     []ServiceLine
	finished    bool

	// A single-line config/status header shown above the bars.
	headerText string
}

// NewProgressRenderer creates a new progress renderer.
func NewProgressRenderer(logger *logging.Logger) *ProgressRenderer {
	return &ProgressRenderer{
		bars:      make(map[string]*mpb.Bar),
		states:    make(map[string]*ProgressState),
		logger:    logger,
		startTime: time.Now(),
	}
}

// Start initializes the progress bars for all services.
// headerText is a single-line configuration summary shown above bars.
func (pr *ProgressRenderer) Start(services []string, maxParallel int, headerText string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.serviceCnt = len(services)
	pr.maxParallel = maxParallel
	pr.headerText = headerText

	if !pr.IsTerminal() || pr.serviceCnt == 0 {
		return
	}

	// Mute console logging during progress bar rendering.
	if pr.logger != nil {
		pr.logger.MuteConsole()
	}
	log.SetOutput(io.Discard)

	// Create mpb container with a sensible bar width.
	// Bar fills should be compact — 30 chars is plenty for a progress indicator.
	barWidth := 30

	pr.progress = mpb.New(
		mpb.WithWidth(barWidth),
		mpb.WithOutput(os.Stderr),
	)

	// Write the one-line header to the mpb log area.
	if headerText != "" {
		pr.progress.Write([]byte(headerText + "\n"))
	}

	// Create one bar per service — clean, no borders, compact.
	for _, name := range services {
		serviceName := name
		state := StateQueued
		pr.states[serviceName] = &state

		bar := pr.progress.New(100,
			// tqdm-style solid bar: ████████████░░░░  — no brackets, no tip
			mpb.BarStyle().Lbound("").Filler("█").Padding("░").Tip("").Rbound(""),
			mpb.PrependDecorators(
				decor.Any(func(s decor.Statistics) string {
					pr.mu.Lock()
					st := pr.states[serviceName]
					pr.mu.Unlock()
					if st == nil {
						return "· "
					}
					return st.StatusIcon() + " "
				}),
				decor.Name(truncateLeft(serviceName, 12), decor.WCSyncSpaceR),
			),
			mpb.AppendDecorators(
				decor.Any(func(s decor.Statistics) string {
					pr.mu.Lock()
					st := pr.states[serviceName]
					pr.mu.Unlock()
					if st == nil {
						return ""
					}
					return fmt.Sprintf("%-8s", st.String())
				}),
				decor.Elapsed(decor.ET_STYLE_GO, decor.WCSyncSpace),
			),
		)
		pr.bars[serviceName] = bar
	}

	// Log to file only
	pr.writeLog("BUILD", fmt.Sprintf("Building %d services (%d parallel)", pr.serviceCnt, pr.maxParallel))
}

// buildProgress animates a bar during building.
// Advances one step every 150ms until the state transitions away from StateBuilding.
// All bar operations are done under pr.mu to prevent races with ServiceDone/ServiceFailed.
func (pr *ProgressRenderer) buildProgress(serviceName string) {
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		pr.mu.Lock()
		st := pr.states[serviceName]
		bar := pr.bars[serviceName]
		if st == nil || bar == nil {
			pr.mu.Unlock()
			return
		}
		if *st != StateBuilding {
			// ServiceDone/ServiceFailed will call bar.SetTotal() under the same
			// mutex, so there is no concurrent Increment/SetTotal race.
			pr.mu.Unlock()
			return
		}
		bar.Increment() // Safe: same mutex as ServiceDone's SetTotal
		pr.mu.Unlock()
	}
}

// ServiceStarted marks a service as building.
func (pr *ProgressRenderer) ServiceStarted(serviceName string) {
	pr.mu.Lock()
	if st, ok := pr.states[serviceName]; ok {
		*st = StateBuilding
	}
	pr.mu.Unlock()

	// Log to file only (not to mpb log area, to keep display clean)
	if pr.logger != nil {
		pr.logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Building: %s", serviceName))
	}

	go pr.buildProgress(serviceName)
}

// ServicePushStarted marks a service as pushing to registry.
func (pr *ProgressRenderer) ServicePushStarted(serviceName string) {
	pr.mu.Lock()
	if st, ok := pr.states[serviceName]; ok {
		*st = StatePushing
	}
	pr.mu.Unlock()

	if pr.logger != nil {
		pr.logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Pushing: %s", serviceName))
	}
}

// ServiceDone marks a service as completed successfully.
func (pr *ProgressRenderer) ServiceDone(serviceName, image string, buildTime, pushTime time.Duration) {
	pr.mu.Lock()
	if st, ok := pr.states[serviceName]; ok {
		*st = StateDone
	}
	if bar, ok := pr.bars[serviceName]; ok {
		// SetTotal(-1, true) sets total to current value and completes the bar.
		// Using (100, true) would deadlock because current never reaches 100.
		bar.SetTotal(-1, true)
	}
	pr.results = append(pr.results, ServiceLine{
		Name:      serviceName,
		Image:     image,
		Status:    "success",
		BuildTime: buildTime,
		PushTime:  pushTime,
	})
	pr.mu.Unlock()

	if pr.logger != nil {
		pr.logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Completed: %s (%s)", serviceName, image))
	}
}

// ServiceFailed marks a service as failed.
func (pr *ProgressRenderer) ServiceFailed(serviceName string, errMsg string) {
	pr.mu.Lock()
	if st, ok := pr.states[serviceName]; ok {
		*st = StateFailed
	}
	if bar, ok := pr.bars[serviceName]; ok {
		bar.SetTotal(-1, true)
		bar.Abort(true)
	}
	pr.results = append(pr.results, ServiceLine{
		Name:   serviceName,
		Status: "failed",
	})
	pr.mu.Unlock()

	if pr.logger != nil {
		pr.logger.Info(logging.CATEGORY_BUILD, fmt.Sprintf("Failed: %s — %s", serviceName, errMsg))
	}
}

// writeLog writes a message to the structured logger (build.log only).
// It does NOT write to the mpb log area to keep the terminal display clean.
func (pr *ProgressRenderer) writeLog(category, msg string) {
	if pr.logger != nil {
		var cat logging.Category
		switch category {
		case "BUILD":
			cat = logging.CATEGORY_BUILD
		case "DISCOVERY":
			cat = logging.CATEGORY_DISCOVERY
		default:
			cat = logging.CATEGORY_BUILD
		}
		pr.logger.Info(cat, msg)
	}
}

// Stop finalizes the progress display and prints the summary.
func (pr *ProgressRenderer) Stop() {
	if pr.finished {
		return
	}
	pr.finished = true

	// Shutdown cancels all bar contexts and flushes output.
	// We use Shutdown() instead of Wait() because in auto-refresh mode,
	// bar contexts are never cancelled by SetTotal(-1, true), causing
	// Wait() to deadlock on bwg.Done().
	if pr.progress != nil {
		pr.progress.Shutdown()
	}

	// Leave completed progress bars visible — do NOT clear them.
	// This avoids blank lines between the bars and the summary.

	// Restore console logging after progress bars are cleaned up
	log.SetOutput(os.Stderr)
	if pr.logger != nil {
		pr.logger.UnmuteConsole()
	}

	// Print final summary to stdout (below the completed bars)
	pr.printSummary()
}

// printSummary outputs a clean formatted summary of the build.
func (pr *ProgressRenderer) printSummary() {
	duration := time.Since(pr.startTime)

	var built, pushed, failed int
	for _, r := range pr.results {
		switch r.Status {
		case "success":
			built++
			if r.PushTime > 0 {
				pushed++
			}
		case "failed":
			failed++
		}
	}

	fmt.Println()
	fmt.Println("  Build Summary")
	for _, r := range pr.results {
		icon := "✓"
		timeStr := fmt.Sprintf("%.1fs", r.BuildTime.Seconds())
		if r.PushTime > 0 {
			timeStr += fmt.Sprintf(" + %.1fs", r.PushTime.Seconds())
		}
		if r.Status == "failed" {
			icon = "✗"
		}
		fmt.Printf("  %s %-12s %-30s  %s\n", icon, r.Name, r.Image, timeStr)
	}
	fmt.Printf("  %s\n", dashes(60))
	fmt.Printf("  %d built, %d pushed, %d failed  |  %.1fs\n", built, pushed, failed, duration.Seconds())
	fmt.Println()
}

// IsTerminal returns true if stderr is a terminal (TTY).
func (pr *ProgressRenderer) IsTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// terminalWidth returns the terminal width, or 80 if it can't be determined.
func terminalWidth() int {
	_, w, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w < 40 {
		return 80
	}
	return w
}

// truncateLeft truncates a string to maxLen, showing the rightmost characters if truncated.
func truncateLeft(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return "…" + s[len(s)-maxLen+1:]
}

// clearLines moves the cursor up N lines and clears them.
func clearLines(n int) {
	if n <= 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\033[%dA", n)
	for i := 0; i < n; i++ {
		fmt.Fprint(os.Stderr, "\033[2K\r")
		if i < n-1 {
			fmt.Fprint(os.Stderr, "\n")
		}
	}
	fmt.Fprint(os.Stderr, "\033[G")
}

// dashes returns a string of n dashes using a Unicode box-drawing character.
func dashes(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("─", n)
}

// Ensure interface compliance
var _ fmt.Stringer = (*ProgressState)(nil)

// BuildSummary returns the collected build results
func (pr *ProgressRenderer) BuildSummary() []ServiceLine {
	return pr.results
}

// FormatBuildTime formats a duration as a human-readable string
func FormatBuildTime(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// SummaryStats returns build counts
func (pr *ProgressRenderer) SummaryStats() (built, pushed, failed, total int) {
	for _, r := range pr.results {
		total++
		switch r.Status {
		case "success":
			built++
			if r.PushTime > 0 {
				pushed++
			}
		case "failed":
			failed++
		}
	}
	if total == 0 {
		total = pr.serviceCnt
	}
	return
}
