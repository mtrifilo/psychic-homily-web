package auth

import (
	"testing"
	"time"
)

func TestUser_CanSignIn(t *testing.T) {
	deletedAt := time.Now()

	cases := []struct {
		name string
		user *User
		want bool
	}{
		{name: "active, not deleted", user: &User{IsActive: true}, want: true},
		{name: "deactivated", user: &User{IsActive: false}, want: false},
		{name: "soft-deleted", user: &User{IsActive: false, DeletedAt: &deletedAt}, want: false},
		// Neither field is trusted to imply the other.
		{name: "deleted_at set on an active row", user: &User{IsActive: true, DeletedAt: &deletedAt}, want: false},
		{name: "nil user", user: nil, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.user.CanSignIn(); got != tc.want {
				t.Errorf("CanSignIn() = %v, want %v", got, tc.want)
			}
		})
	}
}
