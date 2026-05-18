package shared

import "testing"

func TestLikePattern(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain word", "indie", `%indie%`},
		{"percent literal", "foo%bar", `%foo\%bar%`},
		{"underscore literal", "a_b", `%a\_b%`},
		{"backslash literal", `path\to`, `%path\\to%`},
		{"mixed wildcards + backslash", `100%_done\`, `%100\%\_done\\%`},
		{"empty input still wraps", "", `%%`},
		{"unicode untouched", "café", `%café%`},
		{"backslash gets escaped before % so order is right", `\%`, `%\\\%%`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := LikePattern(c.in); got != c.want {
				t.Errorf("LikePattern(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
