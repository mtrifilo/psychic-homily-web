package testcorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fatalRecorder stands in for a test so a failing Load can be observed. Its
// Fatalf records the message and ends the goroutine, as testing.T's does;
// every other testing.TB method is the nil embedded one and panics if Load
// ever calls it.
type fatalRecorder struct {
	testing.TB
	message string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// loadFailure runs Load against a recorder and returns the failure message,
// or "" when Load returned normally.
func loadFailure[T any](path string) string {
	r := &fatalRecorder{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Load[T](r, path)
	}()
	<-done
	return r.message
}

func TestLoadDecodesIntoTheCallersType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corpus.json")
	if err := os.WriteFile(path, []byte(`{"cases":["a","b"],"undeclared":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got := Load[struct {
		Cases []string `json:"cases"`
	}](t, path)

	if len(got.Cases) != 2 || got.Cases[0] != "a" || got.Cases[1] != "b" {
		t.Errorf("Load decoded %+v, want cases [a b]", got)
	}
}

func TestLoadResolvesARelativePathFromThePackageDirectory(t *testing.T) {
	missing := filepath.Join("testdata", "absent_corpus.json")
	abs, err := filepath.Abs(missing)
	if err != nil {
		t.Fatal(err)
	}

	message := loadFailure[struct{}](missing)

	if !strings.Contains(message, abs) {
		t.Errorf("failure %q does not name the resolved path %s", message, abs)
	}
}

func TestLoadNamesAFileItCannotDecode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "truncated.json")
	if err := os.WriteFile(path, []byte(`{"cases":[`), 0o600); err != nil {
		t.Fatal(err)
	}

	message := loadFailure[struct{}](path)

	if !strings.HasPrefix(message, "decode corpus ") || !strings.Contains(message, path) {
		t.Errorf("failure %q does not name the undecodable file %s", message, path)
	}
}
