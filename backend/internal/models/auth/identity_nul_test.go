package auth

import "testing"

func TestHoldsNUL(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty", "", false},
		{"plain username", "nobody", false},
		{"plain address", "no.body@example.com", false},
		{"other control byte", "no\x01body", false},
		{"leading NUL", "\x00nobody", true},
		{"inner NUL", "no\x00body", true},
		{"trailing NUL", "no.body@example.com\x00", true},
		{"only NUL", "\x00", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HoldsNUL(tc.value); got != tc.want {
				t.Errorf("HoldsNUL(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
