package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// unguardedTestPackages lists the internal/ packages, relative to internal/,
// whose tests run without RefuseNonLoopbackNetwork, each with the reason. A
// package belongs here only when its test binary links no outbound HTTP client
// and its tests send no HTTP request.
var unguardedTestPackages = map[string]string{
	"config":         "no outbound HTTP client linked; tests read http constants only",
	"errors":         "no outbound HTTP client linked; tests read http status constants only",
	"legal":          "no outbound HTTP client linked",
	"models/admin":   "no outbound HTTP client linked",
	"models/auth":    "no outbound HTTP client linked",
	"models/catalog": "no outbound HTTP client linked",
	"respond":        "no outbound HTTP client linked; tests write to an httptest recorder",
	"scoring":        "no outbound HTTP client linked",
	"seeddata":       "no outbound HTTP client linked",
	"testcorpus":     "no outbound HTTP client linked",
	"testenv":        "no outbound HTTP client linked",
	"testlog":        "no outbound HTTP client linked",
	"testutil":       "the guard's own tests install and remove it per test",
	"utils":          "no outbound HTTP client linked",
	"utils/urlguard": "no outbound HTTP client linked; its tests pin DNS through a map resolver",
}

// guardCall is split so this file's own source does not contain it and count
// testutil as guarded.
const guardCall = "testutil." + "RefuseNonLoopbackNetwork(m)"

// TestNetworkGuard_EveryInternalTestPackageInstallsItOrIsListed keeps the
// guard from going missing in a new package: every package under internal/
// with test files calls RefuseNonLoopbackNetwork from its TestMain or is in
// unguardedTestPackages, and no listed package installs it.
func TestNetworkGuard_EveryInternalTestPackageInstallsItOrIsListed(t *testing.T) {
	internalDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	guarded, unguarded := map[string]bool{}, map[string]bool{}
	err = filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(internalDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(rel)
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(src), guardCall) {
			guarded[pkg] = true
		} else {
			unguarded[pkg] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(guarded) == 0 {
		t.Fatalf("no package under %s calls %s; the walk is looking in the wrong place", internalDir, guardCall)
	}

	var missing, stale []string
	for pkg := range unguarded {
		if guarded[pkg] {
			continue
		}
		if _, listed := unguardedTestPackages[pkg]; !listed {
			missing = append(missing, pkg)
		}
	}
	for pkg := range unguardedTestPackages {
		if guarded[pkg] || !unguarded[pkg] {
			stale = append(stale, pkg)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	for _, pkg := range missing {
		t.Errorf("internal/%s has tests but no TestMain calling %s: add a network_guard_test.go with\n\tfunc TestMain(m *testing.M) { os.Exit(%s) }\nor, if its tests can send no HTTP request, list it in unguardedTestPackages with the reason", pkg, guardCall, guardCall)
	}
	for _, pkg := range stale {
		t.Errorf("internal/%s is listed in unguardedTestPackages but installs the guard or has no tests: remove the entry", pkg)
	}
}
