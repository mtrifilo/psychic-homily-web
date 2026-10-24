package auth

import "testing"

func TestHasPassword(t *testing.T) {
	hash := "$2a$10$hash"
	empty := ""
	cases := []struct {
		name string
		user *User
		want bool
	}{
		{"nil user", nil, false},
		{"no hash", &User{}, false},
		{"empty hash", &User{PasswordHash: &empty}, false},
		{"hash", &User{PasswordHash: &hash}, true},
	}
	for _, tc := range cases {
		if got := tc.user.HasPassword(); got != tc.want {
			t.Errorf("%s: HasPassword() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
