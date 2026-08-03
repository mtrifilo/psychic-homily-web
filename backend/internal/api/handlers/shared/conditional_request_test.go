package shared

import "testing"

func TestIfNoneMatchCovers(t *testing.T) {
	tests := []struct {
		name        string
		ifNoneMatch string
		etag        string
		want        bool
	}{
		{"exact", `"abc"`, `"abc"`, true},
		{"empty header", "", `"abc"`, false},
		{"empty etag", `"abc"`, "", false},
		{"wildcard", "*", `"abc"`, true},
		{"padded wildcard", "  * ", `"abc"`, true},
		{"list containing the tag", `"x", "abc" , "y"`, `"abc"`, true},
		{"list without the tag", `"x", "y"`, `"abc"`, false},
		{"different tag", `"abcd"`, `"abc"`, false},
		// RFC 9110 §13.1.2 specifies WEAK comparison for If-None-Match, so a
		// validator either side weakened still matches. The calendar feeds
		// serve W/ tags and the graph overview serves strong ones, so both
		// directions are load-bearing.
		{"client weakened our strong tag", `W/"abc"`, `"abc"`, true},
		{"client strengthened our weak tag", `"abc"`, `W/"abc"`, true},
		{"weak on both sides", `W/"abc"`, `W/"abc"`, true},
		{"weak list", `W/"other", W/"abc"`, `W/"abc"`, true},
		{"weak stale validator", `W/"stale"`, `W/"abc"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IfNoneMatchCovers(tt.ifNoneMatch, tt.etag); got != tt.want {
				t.Errorf("IfNoneMatchCovers(%q, %q) = %v, want %v", tt.ifNoneMatch, tt.etag, got, tt.want)
			}
		})
	}
}
