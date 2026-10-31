package indexnow

import (
	"os"
	"regexp"
	"testing"
)

// frontendSitemapShardsPath is the frontend module whose FAMILY_URL_PREFIXES
// builds the sitemap's URLs. Relative to this package's directory.
const frontendSitemapShardsPath = "../../../../frontend/app/sitemap-shards.ts"

var (
	frontendPrefixTablePattern = regexp.MustCompile(`(?s)export const FAMILY_URL_PREFIXES = \{(.*?)\}`)
	frontendPrefixEntryPattern = regexp.MustCompile(`(?m)^\s*(\w+): '([^']+)'`)
)

// The pinger submits each family's URLs under the same prefix the sitemap
// announces them under. Neither the compiler nor the API contract sees the
// pair, so a route renamed on one side only would have the pinger submit
// addresses the sitemap no longer lists, while every run still reads as a
// success.
func TestFamilyPathsMatchTheFrontendSitemap(t *testing.T) {
	source, err := os.ReadFile(frontendSitemapShardsPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v\n"+
			"This test pins the pinger's URL prefixes to the sitemap's. "+
			"If that module moved, repoint this constant; do not delete the pin.",
			frontendSitemapShardsPath, err)
	}
	table := frontendPrefixTablePattern.FindSubmatch(source)
	if table == nil {
		t.Fatalf("no FAMILY_URL_PREFIXES table found in %s", frontendSitemapShardsPath)
	}
	prefixes := map[string]string{}
	for _, entry := range frontendPrefixEntryPattern.FindAllSubmatch(table[1], -1) {
		prefixes[string(entry[1])] = string(entry[2])
	}

	for _, family := range familyPaths {
		prefix, ok := prefixes[family.family]
		if !ok {
			t.Errorf("family %q has no prefix in FAMILY_URL_PREFIXES", family.family)
			continue
		}
		if family.path != prefix+"/" {
			t.Errorf("family %q: pinger path %q, sitemap prefix %q", family.family, family.path, prefix)
		}
	}
}
