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

// MailboxProofReplacesIdentity reports whether the mailbox proof of a pending
// link to the account removes the account's identity from the link's provider
// before attaching the proven one. It does while the account's address was
// never verified: nothing showed that whoever attached that identity reads
// the mailbox, and the proof shows that the person following the link does,
// so the mailbox holder wins. A verified address keeps its identity, and the
// proof is refused instead.
//
// Verifying the address ends this for good, even when the person who
// verified it is the mailbox holder and the identity and password are
// someone else's. That holder signs in by the verification and removes the
// identity and resets the password from Settings; nothing here undoes a
// verification.
//
// The sign-in that offers the pending link, the collision state's read and
// the proof all ask it, so they agree on which accounts the proof replaces
// an identity on.
func (u *User) MailboxProofReplacesIdentity() bool {
	return u != nil && !u.EmailVerified
}
