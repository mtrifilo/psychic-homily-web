package auth

// HasTrustedTier reports whether the account's standing is trusted contributor
// or better, which is what the site treats as "this edit does not need a second
// pair of eyes". Admin always qualifies.
//
// The single definition of that standing. A caller asking it about a stored row
// gets the same answer as the pending-edit queue does.
func (u *User) HasTrustedTier() bool {
	if u == nil {
		return false
	}
	if u.IsAdmin {
		return true
	}
	switch u.UserTier {
	case "trusted_contributor", "local_ambassador":
		return true
	}
	return false
}
