package utils

import "testing"

func TestStateNameToAbbrev(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"Minnesota", "MN", true},
		{"minnesota", "MN", true},
		{"  New York  ", "NY", true},
		{"District of Columbia", "DC", true},
		{"MN", "MN", true},       // already an abbreviation
		{"ca", "CA", true},       // lowercase abbreviation
		{"Australia", "", false}, // non-US
		{"United States", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := StateNameToAbbrev(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("StateNameToAbbrev(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}
