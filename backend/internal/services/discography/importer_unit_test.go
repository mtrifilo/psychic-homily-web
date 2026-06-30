package discography

import (
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

func intp(i int) *int { return &i }

func TestYearFromDate(t *testing.T) {
	cases := []struct {
		in   string
		want *int
	}{
		{"2003-02-04", intp(2003)},
		{"2018", intp(2018)},
		{"1999-12", intp(1999)},
		{"2003abc", intp(2003)}, // leading 4 digits parse
		{"", nil},
		{"19", nil},     // too short to slice
		{"abcd", nil},   // non-numeric
		{"0000", nil},   // below range
		{"1899", nil},   // below range
		{"3000", nil},   // above range
	}
	for _, c := range cases {
		got := yearFromDate(c.in)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("yearFromDate(%q) = %d, want nil", c.in, *got)
		case c.want != nil && got == nil:
			t.Errorf("yearFromDate(%q) = nil, want %d", c.in, *c.want)
		case c.want != nil && got != nil && *got != *c.want:
			t.Errorf("yearFromDate(%q) = %d, want %d", c.in, *got, *c.want)
		}
	}
}

func TestReleaseTypeFor(t *testing.T) {
	cases := []struct {
		in   string
		want catalogm.ReleaseType
	}{
		{"Album", catalogm.ReleaseTypeLP},
		{"album", catalogm.ReleaseTypeLP},
		{"EP", catalogm.ReleaseTypeEP},
		{"ep", catalogm.ReleaseTypeEP},
		{"", catalogm.ReleaseTypeLP},       // default
		{"Single", catalogm.ReleaseTypeLP}, // unmapped → default (browse never sends this)
	}
	for _, c := range cases {
		if got := releaseTypeFor(c.in); got != c.want {
			t.Errorf("releaseTypeFor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
