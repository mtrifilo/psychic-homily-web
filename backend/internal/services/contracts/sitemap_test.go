package contracts

import (
	"reflect"
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
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		if _, ok := counts[jsonTag]; !ok {
			t.Errorf(
				"SitemapEntries.%s (json:%q) has no entry in Counts() — add it, or the family is silently unlogged",
				field.Name, jsonTag,
			)
		}
	}

	if len(counts) != structType.NumField() {
		t.Errorf("Counts() has %d keys but SitemapEntries has %d fields — one of them is stale",
			len(counts), structType.NumField())
	}
}
