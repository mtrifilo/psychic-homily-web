package auth

import "testing"

func TestMayConnectProvider(t *testing.T) {
	address := "someone@example.com"
	empty := ""
	for _, tc := range []struct {
		name string
		user *User
		want bool
	}{
		{"verified address", &User{Email: &address, EmailVerified: true}, true},
		{"unverified address", &User{Email: &address}, false},
		{"no address", &User{}, true},
		{"empty address", &User{Email: &empty}, true},
		{"nil account", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.user.MayConnectProvider(); got != tc.want {
				t.Errorf("MayConnectProvider() = %v, want %v", got, tc.want)
			}
		})
	}
}
