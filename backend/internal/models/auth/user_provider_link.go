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

// MailboxProofMayReplaceIdentity reports whether the mailbox proof of a
// pending link to the account may remove an identity the account holds from
// the link's provider before attaching the proven one. It may while the
// account's address was never verified, whether or not such an identity is
// held: nothing showed that whoever attached one reads the mailbox, and the
// proof shows that the person following the link does, so the mailbox holder
// wins. On a verified address the held identity stays, and the proof is
// refused instead.
//
// Verifying the address ends this for good, even when the person who
// verified it is the mailbox holder and the held identity and the password
// are someone else's. Confirming the verification signs no one in. The
// holder can then sign in by a magic link and disconnect the identity in
// Settings; a later Google sign-in's emailed link then clears the password
// (the mailbox proof clears it on any account). Nothing here undoes a
// verification.
//
// The sign-in that offers the pending link, the collision state's read and
// the proof all ask it, so they agree on which accounts the proof may
// replace an identity on.
func (u *User) MailboxProofMayReplaceIdentity() bool {
	return u != nil && !u.EmailVerified
}
