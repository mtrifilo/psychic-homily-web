package auth

import "testing"

func TestUser_IsPrivateProfile(t *testing.T) {
	cases := []struct {
		visibility string
		want       bool
	}{
		{ProfileVisibilityPrivate, true},
		{ProfileVisibilityPublic, false},
		// Only the exact stored value is private.
		{"", false},
		{"Private", false},
	}
	for _, tc := range cases {
		if got := (&User{ProfileVisibility: tc.visibility}).IsPrivateProfile(); got != tc.want {
			t.Errorf("IsPrivateProfile() with %q = %v, want %v", tc.visibility, got, tc.want)
		}
	}
}
