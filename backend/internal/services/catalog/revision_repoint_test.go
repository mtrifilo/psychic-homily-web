package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// These cover the guards, which are the whole point of the helper: every one
// of them is a merge that would otherwise have written something wrong and
// committed it.
//
// No database. Each case must be rejected BEFORE any statement runs, and the
// zero-value *gorm.DB below makes that self-enforcing: a guard that stopped
// rejecting would reach Exec and panic, failing the test rather than quietly
// passing it.
func unusableTx() *gorm.DB { return &gorm.DB{} }

func TestRepointRevisions_RejectsUndecidedProvenance(t *testing.T) {
	_, err := repointRevisions(unusableTx(), entityTypeShow, 1, 2, provenanceUndecided)
	if err == nil {
		t.Fatal("the zero provenance must be rejected — it is 'nobody decided', not 'no stamp'")
	}
}

func TestRepointRevisions_RejectsUnknownProvenance(t *testing.T) {
	_, err := repointRevisions(unusableTx(), entityTypeShow, 1, 2, revisionProvenance(99))
	if err == nil {
		t.Fatal("a provenance outside the declared set must be rejected")
	}
}

// The stamp names a venue column and only the venue read gate honors it.
// Stamping an artist or a show would record a protection that does not exist.
func TestRepointRevisions_RejectsVenueStampOnOtherEntities(t *testing.T) {
	for _, entity := range []polymorphicEntityType{entityTypeArtist, entityTypeShow} {
		t.Run(string(entity), func(t *testing.T) {
			_, err := repointRevisions(unusableTx(), entity, 1, 2, stampFromUnverifiedVenue)
			if err == nil {
				t.Fatalf("stampFromUnverifiedVenue must be rejected for %q", entity)
			}
		})
	}
}

// Its twin. from_gated_show is read only by the show visibility gate, so
// stamping a venue or an artist would record a suppression nothing enforces
// while making the row look protected to the next reader.
func TestRepointRevisions_RejectsShowStampOnOtherEntities(t *testing.T) {
	for _, entity := range []polymorphicEntityType{entityTypeArtist, entityTypeVenue} {
		t.Run(string(entity), func(t *testing.T) {
			_, err := repointRevisions(unusableTx(), entity, 1, 2, stampFromGatedShow)
			if err == nil {
				t.Fatalf("stampFromGatedShow must be rejected for %q", entity)
			}
		})
	}
}

func TestRepointRevisions_RejectsUnknownEntityType(t *testing.T) {
	_, err := repointRevisions(unusableTx(), polymorphicEntityType("venues"), 1, 2, noRedactionCarryover)
	if err == nil {
		t.Fatal("a mistyped entity type must be rejected — it would match no rows and look like success")
	}
}

// A self-merge is a harmless no-op for the move and a real corruption for the
// stamp: it would mark the surviving venue's own history as carried off an
// unverified one.
func TestRepointRevisions_RejectsSelfMerge(t *testing.T) {
	_, err := repointRevisions(unusableTx(), entityTypeVenue, 7, 7, stampFromUnverifiedVenue)
	if err == nil {
		t.Fatal("re-pointing an entity onto itself must be rejected")
	}
}

func TestRepointRevisions_RejectsZeroIDs(t *testing.T) {
	if _, err := repointRevisions(unusableTx(), entityTypeVenue, 0, 2, noRedactionCarryover); err == nil {
		t.Fatal("a zero canonical id must be rejected")
	}
	if _, err := repointRevisions(unusableTx(), entityTypeVenue, 1, 0, noRedactionCarryover); err == nil {
		t.Fatal("a zero merge-from id must be rejected")
	}
}

// A required parameter only binds an author who reaches for the helper. This
// is what binds the one who does not: a merge that re-points revisions with
// its own UPDATE never has to answer the provenance question, and the leak
// this file exists to prevent comes back on a path nobody reviewed for it.
//
// The same idiom as TestVenueEntityRefsCoverSchema — a hand-maintained
// invariant whose failure mode is silent, checked at the only cheap moment.
//
// Two shapes are checked, because the raw UPDATE every merge used today is not
// the only way to write one: a GORM chain setting entity_id would compile,
// bypass the helper, and read nothing like SQL. The second check is scoped to
// files that also name the revisions table or model, so re-pointing some other
// polymorphic table through the builder does not trip it.
func TestNoRevisionRepointOutsideTheHelper(t *testing.T) {
	root := backendRoot(t)
	updateRevisions := regexp.MustCompile(`(?is)\bupdate\s+revisions\b`)
	builderSetsEntityID := regexp.MustCompile(`Updates?\(\s*"entity_id"`)
	namesRevisions := regexp.MustCompile(`adminm?\.Revision\{|Table\("revisions"\)`)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// Third-party code is not ours to route through the helper, and
			// `go mod vendor` would otherwise drop it into the scan.
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Tests seed and inspect rows directly; the rule is about production
		// merge paths.
		if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "revision_repoint.go" {
			return nil
		}

		source, readErr := os.ReadFile(path) // #nosec G304 -- path comes from walking the module's own tree
		if readErr != nil {
			return readErr
		}
		rawUpdate := updateRevisions.Match(source)
		builderUpdate := builderSetsEntityID.Match(source) && namesRevisions.Match(source)
		if rawUpdate || builderUpdate {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s writes an entity_id update against revisions. Re-pointing revisions "+
				"must go through catalog.repointRevisions, which requires a provenance "+
				"decision — without it a merge silently republishes history that was being "+
				"withheld. If this write is not a re-point, narrow this guard rather than "+
				"deleting it.", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to scan backend sources: %v", err)
	}
}

// backendRoot walks up from this file to the directory holding go.mod, so the
// scan above covers the whole module rather than one package.
func backendRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}

	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find go.mod above the catalog package")
		}
		dir = parent
	}
}
