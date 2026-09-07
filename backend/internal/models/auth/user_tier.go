package auth

// HasTrustedTier reports whether the account's standing is trusted contributor
// or better, which is what the site treats as "this edit does not need a second
// pair of eyes". Admin always qualifies.
//
// The pending-edit queue's bypass and the tag-membership gate both ask it, so
// the two cannot disagree. Other tier rules with their own policies (comment
// limits, entity-request auto-approval) are not this question and do not call
// it.
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
