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

// GoSafe and SubmitAuditWrite read almost identically at a call site, and the
// unbounded one is the older spelling that every existing audit write used, so
// a new handler copying its neighbour is how the bound gets lost. The failure
// would be invisible: the write still lands, the tests still pass, and the
// fan-out is only visible as connection pressure under a burst.
//
// So the rule is checked rather than documented: no GoSafe call may name an
// audit write.
func TestNoAuditWriteBypassesTheBoundedWriter(t *testing.T) {
	root := backendRoot(t)

	// The second argument of GoSafe is the goroutine's label. Any label naming
	// an audit write belongs on SubmitAuditWrite instead.
	unbounded := regexp.MustCompile(`GoSafe\([^,]+,\s*"[A-Za-z0-9_]*audit[A-Za-z0-9_]*"`)

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
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(content), "\n") {
			if unbounded.MatchString(line) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel+":"+strconv.Itoa(i+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if len(offenders) > 0 {
		t.Errorf("audit writes must go through shared.SubmitAuditWrite, not GoSafe, "+
			"so a burst queues instead of opening a connection per write:\n  %s",
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
