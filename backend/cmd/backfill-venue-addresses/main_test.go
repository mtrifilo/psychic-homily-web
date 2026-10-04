package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"psychic-homily-backend/internal/services/catalog"
)

func TestWriteReport_OwnerOnlyFilesForEitherSpelling(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "addr")
	// An existing world-readable file is replaced and made owner-only.
	if err := os.WriteFile(base+".json", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &catalog.VenueAddressReport{Phases: map[string]*catalog.VenueAddressPhaseTotals{}}
	if err := writeReport(r, base+".json"); err != nil {
		t.Fatalf("writeReport: %v", err)
	}
	for _, p := range []string{base + ".md", base + ".json"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("%s mode = %o, want 600", p, mode)
		}
	}
	if b, _ := os.ReadFile(base + ".json"); strings.Contains(string(b), "old") {
		t.Fatal("the existing file was not replaced")
	}
}

func TestValidateFlags(t *testing.T) {
	tests := []struct {
		name     string
		confirm  bool
		approved string
		report   string
		set      []string
		wantErr  string
	}{
		{name: "lookup with a report", report: "/tmp/addr", set: []string{"report", "limit"}},
		{name: "lookup without a report", wantErr: "needs --report"},
		{name: "apply", confirm: true, approved: "/tmp/addr.json", set: []string{"confirm", "approved"}},
		{name: "confirm without approved", confirm: true, wantErr: "reviewed report"},
		{name: "approved without confirm", approved: "/tmp/addr.json", report: "/tmp/x", wantErr: "only with --confirm"},
		{name: "apply with a lookup flag", confirm: true, approved: "/tmp/addr.json", set: []string{"city"}, wantErr: "--city"},
		{name: "apply with a report path", confirm: true, approved: "/tmp/addr.json", report: "/tmp/r", set: []string{"report"}, wantErr: "--report"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := map[string]bool{}
			for _, n := range tt.set {
				set[n] = true
			}
			err := validateFlags(tt.confirm, tt.approved, tt.report, set)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}
