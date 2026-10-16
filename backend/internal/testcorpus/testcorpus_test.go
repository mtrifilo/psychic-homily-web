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

// The expected path is derived from the package directory, which go test
// makes the working directory, so it does not share Load's own resolution.
func TestLoadResolvesARelativePathFromTheModuleRoot(t *testing.T) {
	want, err := filepath.Abs(filepath.Join("testdata", "absent_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}

	message := loadFailure[struct{}]("internal/testcorpus/testdata/absent_corpus.json")

	if !strings.Contains(message, want) {
		t.Errorf("failure %q does not name the resolved path %s", message, want)
	}
}

func TestEnclosingModuleRootFindsTheNearestGoMod(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "nested")
	for _, dir := range []string{
		filepath.Join(outer, "pkg", "sub"),
		filepath.Join(inner, "pkg"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{outer, inner} {
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		file, want string
	}{
		{file: filepath.Join(outer, "pkg", "sub", "f.go"), want: outer},
		{file: filepath.Join(outer, "f.go"), want: outer},
		{file: filepath.Join(inner, "pkg", "f.go"), want: inner},
	} {
		got, err := enclosingModuleRoot(tc.file)
		if err != nil || got != tc.want {
			t.Errorf("enclosingModuleRoot(%s) = %q, %v; want %q", tc.file, got, err, tc.want)
		}
	}
}

// A -trimpath build records source paths relative to the module, and the
// working directory is no substitute for the missing prefix.
func TestEnclosingModuleRootRefusesARelativeSourcePath(t *testing.T) {
	_, err := enclosingModuleRoot("psychic-homily-backend/internal/testcorpus/testcorpus.go")

	if err == nil || !strings.Contains(err.Error(), "-trimpath") {
		t.Errorf("enclosingModuleRoot accepted a relative source path: err = %v", err)
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
