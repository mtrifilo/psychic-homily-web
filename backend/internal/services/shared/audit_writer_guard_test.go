package shared

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The two ways an audit write gets back onto its own goroutine.
//
// The first is the label spelling: GoSafe and SubmitAuditWrite read almost
// identically at a call site, and GoSafe is the older one that every audit write
// used before the bound existed, so a new handler copying its neighbour is how
// the bound gets lost. The second is a bare `go func()`, which is what the
// handler scaffold emitted and what a hand-written handler reaches for first.
//
// Either failure is invisible: the write still lands, the tests still pass, and
// the fan-out shows up only as connection pressure under a burst.
//
// WHAT THIS DOES NOT CATCH, so nobody reads a pass as more than it is: a GoSafe
// label that does not contain "audit" (say "log_action"), a label built from a
// variable, and a goroutine that reaches LogAction through more than a few lines
// of intervening code. It catches the two copy-paste shapes that actually occur.
var unboundedAuditWritePatterns = []*regexp.Regexp{
	// GoSafe(<ctx>, "...audit...", - case-insensitive on the label.
	regexp.MustCompile(`GoSafe\([^,]+,\s*"(?i:[a-z0-9_]*audit[a-z0-9_]*)"`),
	// go func() { ... LogAction( / LogEntityEdit( ... } - the scaffold's shape.
	// (?s) so the body may span lines; the length bound keeps it to a goroutine
	// whose whole point is the audit write.
	regexp.MustCompile(`(?s)go func\(\)\s*\{.{0,400}?\.Log(Action|EntityEdit)\(`),
}

func TestNoAuditWriteBypassesTheBoundedWriter(t *testing.T) {
	root := backendRoot(t)

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// This file states the patterns, so it necessarily contains them.
		if filepath.Base(path) == "audit_writer_guard_test.go" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, pattern := range unboundedAuditWritePatterns {
			// One pass over the bytes before locating anything: the normal case
			// is no match in any of a thousand files.
			loc := pattern.FindIndex(content)
			if loc == nil {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			line := strings.Count(string(content[:loc[0]]), "\n") + 1
			offenders = append(offenders, rel+":"+strconv.Itoa(line))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if len(offenders) > 0 {
		t.Errorf("audit writes must go through shared.SubmitAuditWrite so a burst queues "+
			"instead of opening a connection per write:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// backendRoot walks up from this package to the module root, so the test does
// not encode how deep it happens to sit.
func backendRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test's working directory")
		}
		dir = parent
	}
}
