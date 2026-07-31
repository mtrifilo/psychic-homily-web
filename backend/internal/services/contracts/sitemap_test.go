package contracts

import (
	"reflect"
	"strings"
	"testing"
)

// TestSitemapEntriesCountsCoversEveryFamily is the enforcement behind the
// "hand-maintained" note on Counts(): adding a field to SitemapEntries compiles
// fine and silently leaves the new family uncounted, so a reflection check is
// the only thing that actually keeps the map in sync with the struct.
//
// This matters because PSY-1622 adds several families at once.
func TestSitemapEntriesCountsCoversEveryFamily(t *testing.T) {
	counts := SitemapEntries{}.Counts()

	structType := reflect.TypeOf(SitemapEntries{})
	families := 0
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		// Take the name only. Roughly a fifth of this backend's json tags carry
		// `,omitempty`, and keying off the whole tag would fail against a
		// correct Counts() while telling the maintainer to add a key they
		// already added.
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		families++
		if _, ok := counts[name]; !ok {
			t.Errorf(
				"SitemapEntries.%s (json:%q) has no entry in Counts() — add it, or the family is silently unlogged",
				field.Name, name,
			)
		}
	}

	// Compare against the fields actually considered, not NumField(), so an
	// untagged or json:"-" field does not read as a stale Counts() key.
	if len(counts) != families {
		t.Errorf("Counts() has %d keys but SitemapEntries has %d JSON families — one of them is stale",
			len(counts), families)
	}
}
