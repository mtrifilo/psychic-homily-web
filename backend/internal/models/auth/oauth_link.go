package auth

// OAuthLinkByEmailAllowed reports whether an OAuth identity may be attached to
// an account that already exists, on the strength of a matching address alone.
//
// Both sides have to have proven the mailbox:
//
//   - providerAssertsVerified: the provider says it verified the address it
//     just returned. Absence of a signal is not verification; an address a
//     provider will not vouch for is a caller-supplied string.
//   - existing.EmailVerified: the account holds an address this system itself
//     verified. An account that never proved the mailbox is not evidence of
//     owning it, so admitting a link into one hands the mailbox's real owner
//     sign-in access to whoever created that account first.
//
// The two conditions are independent, and each closes a different half of the
// same takeover: the first stops a chosen address from reaching an account,
// the second stops a genuine address from reaching an account that was never
// entitled to it.
//
// This governs the by-address decision only. A link that resolves its account
// some other way has no address to weigh and does not ask.
func OAuthLinkByEmailAllowed(providerAssertsVerified bool, existing *User) bool {
	return providerAssertsVerified && existing != nil && existing.EmailVerified
}
