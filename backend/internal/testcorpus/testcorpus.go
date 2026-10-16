// Package testcorpus reads a JSON corpus that a Go test shares with another
// suite, most often a frontend Vitest suite that reads the same file.
package testcorpus

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// Load decodes the JSON file at path into a T and returns it, failing the
// test with the file's absolute path named when the file cannot be read or
// decoded.
//
// A relative path is relative to the backend module root, the directory that
// holds go.mod: "internal/utils/testdata/<name>.json". The frontend's
// loadBackendCorpus resolves from the same directory, so the Go and TypeScript
// readers of one corpus name it with the same string. An absolute path is read
// as given. Decoding is json.Unmarshal's, so a field T does not declare is
// ignored and a test may decode only the part of a corpus it asserts on. Load
// makes no claim about the content; each caller asserts the fields it relies
// on are present.
func Load[T any](t testing.TB, path string) T {
	t.Helper()
	abs := path
	if !filepath.IsAbs(path) {
		root, err := moduleRoot()
		if err != nil {
			t.Fatalf("resolve corpus path %q: %v", path, err)
		}
		abs = filepath.Join(root, path)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read corpus: %v (a shared corpus moves only together with every suite that reads it)", err)
	}
	var corpus T
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("decode corpus %s: %v", abs, err)
	}
	return corpus
}

// moduleRoot is the directory of the go.mod that encloses this source file,
// as the compiler recorded its path. The recorded path is absolute unless the
// test binary was built with -trimpath, in which case resolution fails rather
// than guessing from the working directory.
var moduleRoot = sync.OnceValues(func() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller reported no source file for package testcorpus")
	}
	return enclosingModuleRoot(file)
})

// enclosingModuleRoot walks up from the directory of file, which must be
// absolute, to the nearest directory that holds a go.mod.
func enclosingModuleRoot(file string) (string, error) {
	if !filepath.IsAbs(file) {
		return "", fmt.Errorf("source path %q is not absolute (built with -trimpath?), so the module root cannot be found", file)
	}
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if parent := filepath.Dir(dir); parent == dir {
			return "", fmt.Errorf("no go.mod encloses %s", file)
		}
	}
}
