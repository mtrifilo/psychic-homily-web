package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadIDs_DedupeSkipsCommentsAndBlanks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ids.txt")
	if err := os.WriteFile(path, []byte("10\n# a comment\n\n  20  \n10\n30\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ids, err := readIDs(path)
	if err != nil {
		t.Fatalf("readIDs: %v", err)
	}
	// Blanks + '#' comments skipped; duplicate 10 dropped; first-seen order kept.
	want := []uint{10, 20, 30}
	if len(ids) != len(want) {
		t.Fatalf("readIDs = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("readIDs = %v, want %v", ids, want)
		}
	}
}

func TestReadIDs_InvalidID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ids.txt")
	if err := os.WriteFile(path, []byte("10\nnope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readIDs(path); err == nil {
		t.Fatal("expected an error for a non-numeric id, got nil")
	}
}

func TestRedactDBHost_DropsCredentials(t *testing.T) {
	got := redactDBHost("postgres://user:secret@gondola.proxy.rlwy.net:34056/railway")
	if want := "gondola.proxy.rlwy.net:34056/railway"; got != want {
		t.Errorf("redactDBHost = %q, want %q", got, want)
	}
	if strings.Contains(got, "secret") {
		t.Errorf("redactDBHost leaked the password: %q", got)
	}
}
