package auth

import "testing"

// Both conditions are load-bearing and independent. A table that only varied
// one of them would pass with the other side of the rule deleted.
func TestOAuthLinkByEmailAllowed(t *testing.T) {
	verifiedAccount := &User{EmailVerified: true}
	unverifiedAccount := &User{EmailVerified: false}

	cases := []struct {
		name                    string
		providerAssertsVerified bool
		existing                *User
		want                    bool
	}{
		{"provider vouches, account proven", true, verifiedAccount, true},
		{"provider vouches, account never proven", true, unverifiedAccount, false},
		{"provider silent, account proven", false, verifiedAccount, false},
		{"neither", false, unverifiedAccount, false},
		{"no account at all", true, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OAuthLinkByEmailAllowed(tc.providerAssertsVerified, tc.existing); got != tc.want {
				t.Errorf("OAuthLinkByEmailAllowed(%v, %v) = %v, want %v",
					tc.providerAssertsVerified, tc.existing, got, tc.want)
			}
		})
	}
}
