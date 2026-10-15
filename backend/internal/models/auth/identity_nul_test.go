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
		{"other control byte", "no\x01body", false},
		{"NUL", "no\x00body", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HoldsNUL(tc.value); got != tc.want {
				t.Errorf("HoldsNUL(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
