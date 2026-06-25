package display

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	tickInterval = 250 * time.Millisecond // ticker rate
	elapsedSec   = 1 * time.Second        // min interval between elapsed-only redraws
	nameWidth    = 26                      // width for service name column
	contentWidth = 45                      // width for content/status column
)

// Display manages a live status display for a set of parallel Docker builds.
//
// In TTY mode, it uses ANSI cursor save/restore + clear-to-end-of-screen to
// update the display block in-place while preserving log lines above.
//
// In non-TTY mode, it prints plain state-change lines incrementally.
//
// Instead of fake progress bars, each building service shows the latest
// Docker --progress=plain output line, giving users real transparency into
// what Docker is doing.
type Display struct {
	mu         sync.Mutex
	services   []*ServiceStatus
	headerText string
	startTime  time.Time
	writer     io.Writer
	isTTY      bool

	ticker     *time.Ticker
	tickerDone chan struct{}
	stopped    bool
	dirty      bool      // true when a state/Docker-line change needs rendering
	lastDrawAt time.Time // last time we called draw()
	drawnOnce  bool      // true after first draw (so we know to save cursor)

	results []ServiceLine
}

// New creates a new Display that writes to the given writer.
// Set isTTY to true for live in-place updates.
func New(w io.Writer, isTTY bool) *Display {
	return &Display{
		writer: w,
		isTTY:  isTTY,
	}
}

// Start initializes the display with the list of service names.
// It prints the initial state and, in TTY mode, begins a periodic refresh loop.
func (d *Display) Start(services []string, maxParallel int, headerText string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(services) == 0 || d.stopped {
		return
	}

	d.headerText = headerText
	d.startTime = time.Now()
	d.services = make([]*ServiceStatus, len(services))
	for i, name := range services {
		d.services[i] = &ServiceStatus{
			Name:  name,
			State: StateQueued,
		}
	}

	if d.isTTY {
		d.draw()
		d.lastDrawAt = time.Now()

		d.ticker = time.NewTicker(tickInterval)
		d.tickerDone = make(chan struct{})
		go d.tickerLoop()
	} else {
		// Non-TTY: print header, then wait for events
		fmt.Fprintf(d.writer, "%s\n", headerText)
		for _, s := range d.services {
			fmt.Fprintf(d.writer, "  · %s  queued\n", truncateLeft(s.Name, 24))
		}
	}
}

// tickerLoop runs in a goroutine, refreshing the display on each tick.
func (d *Display) tickerLoop() {
	for {
		select {
		case <-d.ticker.C:
			d.refresh()
		case <-d.tickerDone:
			return
		}
	}
}

// refresh is called by the ticker goroutine (TTY mode only).
func (d *Display) refresh() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	now := time.Now()
	d.updateElapsed(now)

	if d.dirty || now.Sub(d.lastDrawAt) >= elapsedSec {
		d.draw()
		d.lastDrawAt = now
		d.dirty = false
	}
}

// Update is called when a service's Docker build emits a new --progress=plain
// line. It sets the service state to Building (if queued) and records the
// latest Docker output line.
func (d *Display) Update(name, dockerLine string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, s := range d.services {
		if s.Name != name {
			continue
		}
		if s.State == StateQueued {
			s.State = StateBuilding
			s.Elapsed = 0
		}
		if s.State == StateBuilding {
			trimmed := strings.TrimSpace(dockerLine)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "#") ||
				strings.Contains(trimmed, "=>") ||
				strings.Contains(trimmed, "exporting") {
				s.DockerLine = trimmed
			}
			if len(s.DockerLine) > contentWidth {
				s.DockerLine = s.DockerLine[:contentWidth-3] + "..."
			}
		}
		d.dirty = true
		return
	}
}

// ServiceStarted marks a service as building.
func (d *Display) ServiceStarted(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, s := range d.services {
		if s.Name == name && s.State == StateQueued {
			s.State = StateBuilding
			s.Elapsed = 0
			d.dirty = true
			return
		}
	}
}

// ServicePushStarted marks a service as pushing.
func (d *Display) ServicePushStarted(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, s := range d.services {
		if s.Name == name {
			s.State = StatePushing
			s.DockerLine = "pushing image..."
			s.Elapsed = 0
			d.dirty = true
			return
		}
	}
}

// ServiceDone marks a service as successfully completed.
func (d *Display) ServiceDone(name, image string, buildTime, pushTime time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, s := range d.services {
		if s.Name == name {
			s.State = StateDone
			s.DockerLine = ""
			s.BuildTime = buildTime
			s.PushTime = pushTime
			s.Image = image
			d.dirty = true
			break
		}
	}
	d.results = append(d.results, ServiceLine{
		Name: name, Image: image, Status: "success",
		BuildTime: buildTime, PushTime: pushTime,
	})
}

// ServiceFailed marks a service as failed.
func (d *Display) ServiceFailed(name string, errMsg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, s := range d.services {
		if s.Name == name {
			s.State = StateFailed
			s.DockerLine = ""
			s.Error = errMsg
			d.dirty = true
			break
		}
	}
	d.results = append(d.results, ServiceLine{
		Name: name, Status: "failed",
	})
}

// Stop finalizes the display and prints the summary. Safe to call multiple times.
// On Ctrl+C or other interruption, marks in-progress services as canceled.
func (d *Display) Stop() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	d.stopped = true

	if d.ticker != nil {
		d.ticker.Stop()
	}
	tickerDone := d.tickerDone
	d.mu.Unlock()

	// Allow ticker goroutine to exit
	if tickerDone != nil {
		close(tickerDone)
		time.Sleep(5 * time.Millisecond)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Mark any in-progress services as canceled (Ctrl+C or other interruption)
	for _, s := range d.services {
		if s.State == StateQueued || s.State == StateBuilding || s.State == StatePushing {
			s.State = StateFailed
			s.DockerLine = ""
			s.Error = "canceled"
			d.results = append(d.results, ServiceLine{
				Name: s.Name, Status: "canceled",
			})
		}
	}

	// Final draw overwrites the display block, then summary prints below.
	if d.isTTY && d.drawnOnce {
		d.updateElapsed(time.Now())
		d.draw()
	}

	d.printSummary()
}

// BuildSummary returns the accumulated results.
func (d *Display) BuildSummary() []ServiceLine {
	d.mu.Lock()
	defer d.mu.Unlock()
	results := make([]ServiceLine, len(d.results))
	copy(results, d.results)
	return results
}

// updateElapsed refreshes elapsed time for all in-progress services.
func (d *Display) updateElapsed(now time.Time) {
	for _, s := range d.services {
		if s.State == StateQueued || s.State == StateBuilding || s.State == StatePushing {
			s.Elapsed = now.Sub(d.startTime)
		}
	}
}

// draw renders the current state (TTY mode).
//
// On the first call, saves the cursor position so that subsequent calls can
// restore to it, clear the old display block, and redraw cleanly. This
// preserves any log lines above the display area.
//
// Uses \033[s (save cursor), \033[u (restore cursor), \033[J (clear to
// end of screen) — these are standard ANSI/DEC sequences supported by all
// modern terminals.
func (d *Display) draw() {
	if len(d.services) == 0 {
		return
	}

	// Save cursor position on first draw (this is right after the log lines).
	// On subsequent draws, restore to saved position and clear everything
	// below, which removes the old display block.
	if !d.drawnOnce {
		fmt.Fprint(d.writer, "\033[s")
		d.drawnOnce = true
	} else {
		fmt.Fprint(d.writer, "\033[u\033[J")
	}

	// Header
	fmt.Fprintf(d.writer, "%s\n", d.headerText)

	// Service lines
	for _, s := range d.services {
		line := formatServiceLine(s)
		fmt.Fprintf(d.writer, "%s\n", line)
	}

	// Blank line (visual separator)
	fmt.Fprint(d.writer, "\n")
}

// formatServiceLine builds a single display line for a service.
func formatServiceLine(s *ServiceStatus) string {
	icon := s.State.StatusIcon()
	name := truncateRight(s.Name, nameWidth)

	var content string
	switch s.State {
	case StateQueued:
		content = "queued"
	case StateBuilding:
		if s.DockerLine != "" {
			content = s.DockerLine
		} else {
			content = "building..."
		}
	case StatePushing:
		if s.DockerLine != "" {
			content = s.DockerLine
		} else {
			content = "pushing..."
		}
	case StateDone:
		content = "built"
		if s.PushTime > 0 {
			content = fmt.Sprintf("built + pushed in %.1fs", s.PushTime.Seconds())
		} else if s.BuildTime > 0 {
			content = fmt.Sprintf("built in %.1fs", s.BuildTime.Seconds())
		}
	case StateFailed:
		content = s.Error
		if content == "" {
			content = "failed"
		}
	}

	content = truncateRight(content, contentWidth)

	var elapsed string
	switch s.State {
	case StateQueued, StateBuilding, StatePushing:
		elapsed = formatElapsed(s.Elapsed)
		elapsed = fmt.Sprintf("(%5s)", elapsed)
	default:
		elapsed = "        "
	}

	return fmt.Sprintf("%s %-*s  %-*s  %s", icon, nameWidth, name, contentWidth, content, elapsed)
}

// formatElapsed formats a duration as a compact string like "12s" or "1m05s".
func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	totalSec := int(d.Seconds())
	if totalSec < 60 {
		return fmt.Sprintf("%ds", totalSec)
	}
	return fmt.Sprintf("%dm%02ds", totalSec/60, totalSec%60)
}

// truncateRight truncates a string to maxLen, keeping the leftmost part.
func truncateRight(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}

// truncateLeft truncates a string to maxLen, keeping the rightmost part.
func truncateLeft(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return "…" + s[len(s)-maxLen+1:]
}

// printSummary prints the final build summary to the writer.
func (d *Display) printSummary() {
	duration := time.Since(d.startTime)
	var built, pushed, failed, canceled int
	for _, r := range d.results {
		switch r.Status {
		case "success":
			built++
			if r.PushTime > 0 {
				pushed++
			}
		case "failed":
			failed++
		case "canceled":
			canceled++
		}
	}

	fmt.Fprintln(d.writer)
	fmt.Fprintln(d.writer, "  Build Summary")
	fmt.Fprintln(d.writer, dashes(60))

	for _, r := range d.results {
		var icon string
		timeStr := fmt.Sprintf("%.1fs", r.BuildTime.Seconds())
		if r.PushTime > 0 {
			timeStr += fmt.Sprintf(" + %.1fs", r.PushTime.Seconds())
		}
		switch r.Status {
		case "success":
			icon = "✓"
		case "canceled":
			icon = "✗"
			timeStr = "canceled"
		default:
			icon = "✗"
		}
		img := truncateRight(r.Image, 40)
		fmt.Fprintf(d.writer, "  %s %-*s  %-40s  %s\n", icon, nameWidth, r.Name, img, timeStr)
	}

	fmt.Fprintln(d.writer, dashes(60))
	fmt.Fprintf(d.writer, "  %d built, %d pushed, %d failed, %d canceled  |  %.1fs total\n",
		built, pushed, failed, canceled, duration.Seconds())

	if !d.isTTY {
		fmt.Fprintf(d.writer, "  (see build.log for full details)\n")
	}

	fmt.Fprintln(d.writer)
}

func dashes(n int) string {
	return strings.Repeat("─", n)
}
