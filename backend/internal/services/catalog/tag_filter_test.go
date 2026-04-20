package catalog

import "testing"

func TestParseTagFilter(t *testing.T) {
	cases := []struct {
		name       string
		tags       string
		match      string
		wantSlugs  []string
		wantMatch  bool
		wantHasTag bool
	}{
		{name: "empty", tags: "", match: "", wantSlugs: nil, wantMatch: false, wantHasTag: false},
		{name: "single", tags: "post-punk", match: "", wantSlugs: []string{"post-punk"}, wantMatch: false, wantHasTag: true},
		{name: "multiple-trimmed", tags: " post-punk , shoegaze , ", match: "", wantSlugs: []string{"post-punk", "shoegaze"}, wantMatch: false, wantHasTag: true},
		{name: "dedup-and-lower", tags: "Post-Punk,post-punk,SHOEGAZE", match: "", wantSlugs: []string{"post-punk", "shoegaze"}, wantMatch: false, wantHasTag: true},
		{name: "match-any", tags: "a,b", match: "any", wantSlugs: []string{"a", "b"}, wantMatch: true, wantHasTag: true},
		{name: "match-any-caps", tags: "a,b", match: "ANY", wantSlugs: []string{"a", "b"}, wantMatch: true, wantHasTag: true},
		{name: "match-all", tags: "a,b", match: "all", wantSlugs: []string{"a", "b"}, wantMatch: false, wantHasTag: true},
		{name: "match-unknown", tags: "a", match: "wat", wantSlugs: []string{"a"}, wantMatch: false, wantHasTag: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseTagFilter(tc.tags, tc.match)
			if got.HasTags() != tc.wantHasTag {
				t.Fatalf("HasTags: got %v want %v", got.HasTags(), tc.wantHasTag)
			}
			if got.MatchAny != tc.wantMatch {
				t.Fatalf("MatchAny: got %v want %v", got.MatchAny, tc.wantMatch)
			}
			if len(got.TagSlugs) != len(tc.wantSlugs) {
				t.Fatalf("Slugs len: got %v want %v", got.TagSlugs, tc.wantSlugs)
			}
			for i := range got.TagSlugs {
				if got.TagSlugs[i] != tc.wantSlugs[i] {
					t.Fatalf("Slugs[%d]: got %q want %q", i, got.TagSlugs[i], tc.wantSlugs[i])
				}
			}
		})
	}
}
