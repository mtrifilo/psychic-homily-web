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

func TestUser_ProfileHiddenFrom(t *testing.T) {
	private := &User{ID: 7, ProfileVisibility: ProfileVisibilityPrivate}
	public := &User{ID: 7, ProfileVisibility: ProfileVisibilityPublic}
	cases := []struct {
		name     string
		user     *User
		viewerID uint
		want     bool
	}{
		{"private, logged out", private, 0, true},
		{"private, another viewer", private, 8, true},
		{"private, its owner", private, 7, false},
		{"public, logged out", public, 0, false},
		{"public, another viewer", public, 8, false},
	}
	for _, tc := range cases {
		if got := tc.user.ProfileHiddenFrom(tc.viewerID); got != tc.want {
			t.Errorf("%s: ProfileHiddenFrom(%d) = %v, want %v", tc.name, tc.viewerID, got, tc.want)
		}
	}
}
