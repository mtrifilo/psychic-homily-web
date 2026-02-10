package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ── History ──────────────────────────────────────────────────────────

const historyFile = ".test-history.json"
const maxHistory = 20
const estimateWindow = 5

type suiteResult struct {
	Ms     int64 `json:"ms"`
	Passed bool  `json:"passed"`
}

type historyRun struct {
	Time   string                  `json:"time"`
	Suites map[string]*suiteResult `json:"suites"`
}

type historyData struct {
	Runs []historyRun `json:"runs"`
}

func historyPath(projectRoot string) string {
	return filepath.Join(projectRoot, historyFile)
}

func loadHistory(projectRoot string) historyData {
	data, err := os.ReadFile(historyPath(projectRoot))
	if err != nil {
		return historyData{}
	}
	var h historyData
	if err := json.Unmarshal(data, &h); err != nil {
		return historyData{}
	}
	return h
}

func saveHistory(projectRoot string, h historyData) {
	if len(h.Runs) > maxHistory {
		h.Runs = h.Runs[len(h.Runs)-maxHistory:]
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(historyPath(projectRoot), data, 0644)
}

func estimateMs(h historyData, suite string) int64 {
	var times []int64
	for i := len(h.Runs) - 1; i >= 0 && len(times) < estimateWindow; i-- {
		if s, ok := h.Runs[i].Suites[suite]; ok && s.Passed {
			times = append(times, s.Ms)
		}
	}
	if len(times) == 0 {
		return 0
	}
	var sum int64
	for _, t := range times {
		sum += t
	}
	return sum / int64(len(times))
}

// ── Suite State ──────────────────────────────────────────────────────

type status int

const (
	statusPending status = iota
	statusRunning
	statusPassed
	statusFailed
)

type suiteState struct {
	mu         sync.Mutex
	name       string
	status     status
	startTime  time.Time
	elapsed    time.Duration
	estimateMs int64
	lines      int
	lastLine   string
	output     []string
}

func (s *suiteState) setStatus(st status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = st
	if st == statusRunning {
		s.startTime = time.Now()
	}
	if st == statusPassed || st == statusFailed {
		s.elapsed = time.Since(s.startTime)
	}
}

func (s *suiteState) addLine(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines++
	s.lastLine = line
	s.output = append(s.output, line)
}

func (s *suiteState) snapshot() (status, time.Duration, int64, int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := s.elapsed
	if s.status == statusRunning {
		elapsed = time.Since(s.startTime)
	}
	return s.status, elapsed, s.estimateMs, s.lines, s.lastLine
}

func (s *suiteState) getOutput() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.output, "\n")
}

// ── Formatting helpers ───────────────────────────────────────────────

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[91m"
	green  = "\033[92m"
	yellow = "\033[93m"
	cyan   = "\033[96m"
	white  = "\033[97m"
)

func fmtDuration(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func fmtDurationMs(ms int64) string {
	s := int(ms / 1000)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func fmtDelta(actualMs, avgMs int64) string {
	delta := actualMs - avgMs
	sign := "+"
	if delta < 0 {
		sign = "-"
		delta = -delta
	}
	return fmt.Sprintf("%ss", sign+fmtDurationMs(delta))
}

// ── Progress bar ─────────────────────────────────────────────────────

const barWidth = 24

func progressBar(elapsed time.Duration, estimateMs int64, st status, tick int) string {
	if st == statusPassed {
		return green + strings.Repeat("█", barWidth) + reset
	}
	if st == statusFailed {
		return red + strings.Repeat("█", barWidth) + reset
	}
	if st == statusPending {
		return dim + strings.Repeat("░", barWidth) + reset
	}

	// Running
	if estimateMs <= 0 {
		// Bouncing indicator — no history
		pos := tick % (barWidth * 2)
		if pos >= barWidth {
			pos = barWidth*2 - pos - 1
		}
		bar := []rune(strings.Repeat("░", barWidth))
		if pos >= 0 && pos < barWidth {
			bar[pos] = '▓'
		}
		return cyan + string(bar) + reset
	}

	fraction := float64(elapsed.Milliseconds()) / float64(estimateMs)
	if fraction > 1 {
		fraction = 1
	}
	filled := int(math.Round(fraction * float64(barWidth)))
	if filled > barWidth {
		filled = barWidth
	}
	return cyan + strings.Repeat("█", filled) + dim + strings.Repeat("░", barWidth-filled) + reset
}

func statusIcon(st status) string {
	switch st {
	case statusPending:
		return dim + "○" + reset
	case statusRunning:
		return yellow + "●" + reset
	case statusPassed:
		return green + "✓" + reset
	case statusFailed:
		return red + "✗" + reset
	}
	return " "
}

func statusLabel(st status) string {
	switch st {
	case statusPending:
		return dim + "pending" + reset
	case statusRunning:
		return yellow + "running" + reset
	case statusPassed:
		return green + "passed" + reset
	case statusFailed:
		return red + "failed" + reset
	}
	return ""
}

// ── Rendering ────────────────────────────────────────────────────────

const dashboardLines = 7

func renderDashboard(suites []*suiteState, startTime time.Time, tick int) string {
	totalElapsed := time.Since(startTime)

	// Estimate remaining = max suite remaining (parallel execution)
	var maxRemaining time.Duration
	allDone := true
	for _, s := range suites {
		st, elapsed, estMs, _, _ := s.snapshot()
		if st == statusRunning || st == statusPending {
			allDone = false
			if estMs > 0 {
				estDur := time.Duration(estMs) * time.Millisecond
				var rem time.Duration
				if st == statusPending {
					rem = estDur
				} else {
					rem = estDur - elapsed
					if rem < 0 {
						rem = 0
					}
				}
				if rem > maxRemaining {
					maxRemaining = rem
				}
			}
		}
	}

	var b strings.Builder

	// Header
	remaining := ""
	if maxRemaining > 0 && !allDone {
		remaining = fmt.Sprintf("  ·  %s~%s remaining%s", dim, fmtDuration(maxRemaining), reset)
	}
	b.WriteString(fmt.Sprintf("  %s%sTest Runner%s  ·  %s elapsed%s\n", bold, white, reset, fmtDuration(totalElapsed), remaining))
	b.WriteString("\n")

	// Suite rows
	for _, s := range suites {
		st, elapsed, estMs, lines, _ := s.snapshot()
		bar := progressBar(elapsed, estMs, st, tick)

		timeStr := fmtDuration(elapsed)
		if estMs > 0 {
			timeStr += " / ~" + fmtDurationMs(estMs)
		}

		lineInfo := ""
		if lines > 0 {
			lineInfo = fmt.Sprintf(" (%d lines)", lines)
		}

		b.WriteString(fmt.Sprintf("  %-9s %s  %s   %s %s%s%s\n",
			s.name, bar, timeStr, statusIcon(st), statusLabel(st), dim+lineInfo+reset, ""))
	}

	// Latest output line (from the first running suite that has output)
	b.WriteString("\n")
	latestLine := ""
	for _, s := range suites {
		st, _, _, _, last := s.snapshot()
		if st == statusRunning && last != "" {
			latestLine = last
			break
		}
	}
	if latestLine != "" {
		// Truncate to 72 chars
		if len(latestLine) > 72 {
			latestLine = latestLine[:72]
		}
		b.WriteString(fmt.Sprintf("  %s› %s%s\n", dim, latestLine, reset))
	} else {
		b.WriteString("\n")
	}

	return b.String()
}

func clearDashboard() {
	for i := 0; i < dashboardLines; i++ {
		fmt.Print("\033[A\033[2K")
	}
}

// ── Suite execution ──────────────────────────────────────────────────

type suiteConfig struct {
	name    string
	dir     string
	command string
	args    []string
}

// Track running processes for cleanup on interrupt
var (
	procMu    sync.Mutex
	procPids  []int
)

func trackPid(pid int) {
	procMu.Lock()
	defer procMu.Unlock()
	procPids = append(procPids, pid)
}

func killAllProcessGroups() {
	procMu.Lock()
	defer procMu.Unlock()
	for _, pid := range procPids {
		// Kill the entire process group
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

func runSuite(sc suiteConfig, state *suiteState, projectRoot string) {
	state.setStatus(statusRunning)

	cmd := exec.Command(sc.command, sc.args...)
	cmd.Dir = filepath.Join(projectRoot, sc.dir)
	cmd.Env = append(os.Environ(), "FORCE_COLOR=0", "NO_COLOR=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		state.addLine(fmt.Sprintf("error creating pipe: %v", err))
		state.setStatus(statusFailed)
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		state.addLine(fmt.Sprintf("error starting: %v", err))
		state.setStatus(statusFailed)
		return
	}

	trackPid(cmd.Process.Pid)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		state.addLine(scanner.Text())
	}

	if err := cmd.Wait(); err != nil {
		state.setStatus(statusFailed)
	} else {
		state.setStatus(statusPassed)
	}
}

// ── Claude Code fix prompt ──────────────────────────────────────────

const maxPromptBytes = 200 * 1024 // macOS arg limit safety margin
const maxTailLines = 500

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func buildClaudePrompt(suites []*suiteState) string {
	var b strings.Builder
	b.WriteString("The following test suite(s) failed during `bun run test-all`. Read the errors below, investigate the root cause in the codebase, and fix the failing tests.\n")

	for _, s := range suites {
		st, elapsed, _, _, _ := s.snapshot()
		if st != statusFailed {
			continue
		}
		b.WriteString(fmt.Sprintf("\n## Failed: %s (exit after %s)\n\n", s.name, fmtDuration(elapsed)))
		b.WriteString(s.getOutput())
		b.WriteString("\n")
	}

	prompt := b.String()

	// Truncate if too long for execve args
	if len(prompt) > maxPromptBytes {
		var tb strings.Builder
		tb.WriteString("The following test suite(s) failed during `bun run test-all`. Read the errors below, investigate the root cause in the codebase, and fix the failing tests.\n")
		tb.WriteString("\n(Note: output was truncated to the last 500 lines per suite)\n")

		for _, s := range suites {
			st, elapsed, _, _, _ := s.snapshot()
			if st != statusFailed {
				continue
			}
			tb.WriteString(fmt.Sprintf("\n## Failed: %s (exit after %s)\n\n", s.name, fmtDuration(elapsed)))
			lines := s.output
			if len(lines) > maxTailLines {
				lines = lines[len(lines)-maxTailLines:]
			}
			tb.WriteString(strings.Join(lines, "\n"))
			tb.WriteString("\n")
		}
		prompt = tb.String()
	}

	return prompt
}

func offerClaudeFix(suites []*suiteState) {
	if !isTerminal() {
		return
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return // claude not installed, skip silently
	}

	fmt.Printf("  Fix with Claude Code? [Y/n] ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "" && answer != "y" {
		return
	}

	prompt := buildClaudePrompt(suites)
	if err := syscall.Exec(claudePath, []string{"claude", prompt}, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "  Failed to launch claude: %v\n", err)
	}
}

// ── Main ─────────────────────────────────────────────────────────────

func main() {
	// Find project root (two levels up from scripts/test-runner/)
	exe, _ := os.Getwd()
	projectRoot := filepath.Join(exe, "../..")
	// Resolve to absolute for clean paths
	projectRoot, _ = filepath.Abs(projectRoot)

	// Also support running from project root directly
	if _, err := os.Stat(filepath.Join(projectRoot, "backend")); err != nil {
		// Try current directory as project root
		if _, err := os.Stat(filepath.Join(exe, "backend")); err == nil {
			projectRoot = exe
		}
	}

	history := loadHistory(projectRoot)

	suiteConfigs := []suiteConfig{
		{name: "backend", dir: "backend", command: "go", args: []string{"test", "-count=1", "./..."}},
		{name: "frontend", dir: "frontend", command: "bun", args: []string{"run", "test:run"}},
		{name: "e2e", dir: "frontend", command: "bun", args: []string{"run", "test:e2e"}},
	}

	suites := make([]*suiteState, len(suiteConfigs))
	for i, sc := range suiteConfigs {
		suites[i] = &suiteState{
			name:       sc.name,
			status:     statusPending,
			estimateMs: estimateMs(history, sc.name),
		}
	}

	startTime := time.Now()

	// Hide cursor
	fmt.Print("\033[?25l")

	// Signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Print initial dashboard (blank lines to establish space)
	for i := 0; i < dashboardLines; i++ {
		fmt.Println()
	}

	// Render loop
	stopRender := make(chan struct{})
	renderDone := make(chan struct{})
	tick := 0
	go func() {
		defer close(renderDone)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopRender:
				// Final render
				clearDashboard()
				fmt.Print(renderDashboard(suites, startTime, tick))
				return
			case <-ticker.C:
				tick++
				clearDashboard()
				fmt.Print(renderDashboard(suites, startTime, tick))
			}
		}
	}()

	// Run all suites in parallel
	var wg sync.WaitGroup
	for i, sc := range suiteConfigs {
		wg.Add(1)
		go func(idx int, cfg suiteConfig) {
			defer wg.Done()
			runSuite(cfg, suites[idx], projectRoot)
		}(i, sc)
	}

	// Wait for either completion or signal
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	interrupted := false
	select {
	case <-doneCh:
		// All suites finished
	case <-sigCh:
		interrupted = true
		killAllProcessGroups()
		for _, s := range suites {
			s.mu.Lock()
			if s.status == statusRunning {
				s.status = statusFailed
				s.elapsed = time.Since(s.startTime)
			}
			s.mu.Unlock()
		}
	}

	// Stop render loop and wait for final render
	close(stopRender)
	<-renderDone

	// Show cursor
	fmt.Print("\033[?25h")

	if interrupted {
		fmt.Println()
		fmt.Printf("  %sInterrupted%s\n", red, reset)
		os.Exit(130)
	}

	// Clear dashboard for summary
	clearDashboard()

	totalElapsed := time.Since(startTime)

	// ── Summary ──────────────────────────────────────────────────────
	fmt.Println()
	fmt.Printf("  %s══════════════════════════════════════════════%s\n", dim, reset)
	fmt.Printf("  %s%sTEST SUMMARY%s%s%s\n",
		bold, white, reset,
		strings.Repeat(" ", 28),
		fmtDuration(totalElapsed))
	fmt.Printf("  %s══════════════════════════════════════════════%s\n", dim, reset)

	anyFailed := false
	for _, s := range suites {
		st, elapsed, estMs, _, _ := s.snapshot()
		icon := statusIcon(st)
		label := statusLabel(st)
		timeStr := fmtDuration(elapsed)

		comparison := ""
		if estMs > 0 {
			comparison = fmt.Sprintf("  %s(avg %s, %s)%s",
				dim, fmtDurationMs(estMs), fmtDelta(elapsed.Milliseconds(), estMs), reset)
		}

		fmt.Printf("  %s %-10s %s   %s%s\n", icon, s.name, label, timeStr, comparison)

		if st == statusFailed {
			anyFailed = true
		}
	}

	fmt.Printf("  %s──────────────────────────────────────────────%s\n", dim, reset)

	if anyFailed {
		fmt.Printf("  %s%sSome suites failed!%s\n", bold, red, reset)
	} else {
		fmt.Printf("  %s%sAll suites passed!%s\n", bold, green, reset)
	}
	fmt.Println()

	// Print failed suite output
	for _, s := range suites {
		st, _, _, _, _ := s.snapshot()
		if st == statusFailed {
			output := s.getOutput()
			if output != "" {
				fmt.Printf("  %s══ %s output ══════════════════════════════════%s\n", red, s.name, reset)
				fmt.Println(output)
				fmt.Println()
			}
		}
	}

	// ── Save history (before prompt so timing data isn't lost) ───────
	run := historyRun{
		Time:   time.Now().UTC().Format(time.RFC3339),
		Suites: make(map[string]*suiteResult),
	}
	for _, s := range suites {
		st, elapsed, _, _, _ := s.snapshot()
		run.Suites[s.name] = &suiteResult{
			Ms:     elapsed.Milliseconds(),
			Passed: st == statusPassed,
		}
	}
	history.Runs = append(history.Runs, run)
	saveHistory(projectRoot, history)

	if anyFailed {
		offerClaudeFix(suites)
		os.Exit(1)
	}
}
