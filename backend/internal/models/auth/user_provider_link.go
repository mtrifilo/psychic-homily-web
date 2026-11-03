package auth

// MayConnectProvider reports whether a session for the account may attach a
// provider identity to it (the Settings link). An account whose address is
// not verified may not: nothing shows that its holder reads that mailbox,
// and an identity they attach would keep signing in to an address that may
// belong to someone else. An account with no address has no mailbox to stand
// in for, and may.
//
// The link start and the link write both ask it, so they refuse the same
// accounts. The pending-link proofs, which attach an identity whose provider
// vouched for the account's own address, do not.
func (u *User) MayConnectProvider() bool {
	if u == nil {
		return false
	}
	return u.Email == nil || *u.Email == "" || u.EmailVerified
}
