package auth

// TrustedTiers are the user_tier values HasTrustedTier accepts (an admin
// qualifies regardless of tier). A query that must apply the same rule in SQL
// builds its tier list from this slice so the two cannot drift apart.
var TrustedTiers = []string{"trusted_contributor", "local_ambassador"}

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
	for _, t := range TrustedTiers {
		if u.UserTier == t {
			return true
		}
	}
	return false
}
