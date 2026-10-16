// Package testcorpus reads a JSON corpus that a Go test shares with another
// suite, most often a frontend Vitest suite that reads the same file.
package testcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Load decodes the JSON file at path into a T and returns it, failing the
// test with the file's absolute path named when the file cannot be read or
// decoded.
//
// path is relative to the calling test's package directory, which go test
// makes the working directory: "testdata/<name>.json" for a corpus beside the
// test. Decoding is json.Unmarshal's, so a field T does not declare is
// ignored and a test may decode only the part of a corpus it asserts on.
// Load makes no claim about the content; each caller asserts the fields it
// relies on are present.
func Load[T any](t testing.TB, path string) T {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve corpus path %q: %v", path, err)
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
