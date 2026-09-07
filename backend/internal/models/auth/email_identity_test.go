package auth

import "testing"

// TestSameEmailIdentity pins the in-process spelling of EmailIdentityWhere.
// It must answer the same question the SQL clause does: do these two strings
// name the same mailbox?
func TestSameEmailIdentity(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"identical", "user@example.com", "user@example.com", true},
		{"local part case", "User@example.com", "user@example.com", true},
		{"domain case", "user@Example.com", "user@example.com", true},
		{"both parts", "Sym.Case@Example.com", "SYM.CASE@EXAMPLE.COM", true},
		{"different local part", "user@example.com", "other@example.com", false},
		{"different domain", "user@example.com", "user@example.org", false},
		{"padding is not folded away", " user@example.com", "user@example.com", false},
		{"empty pair", "", "", true},
		{"one empty", "", "user@example.com", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SameEmailIdentity(tc.a, tc.b); got != tc.want {
				t.Errorf("SameEmailIdentity(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
