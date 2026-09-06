package auth

import "strings"

// EmailIdentityWhere is the WHERE fragment every lookup of a users row by
// address uses, bound to one argument: the address as the caller typed it.
//
// One mailbox is one account, so identity is case-insensitive while the stored
// bytes keep whatever casing the owner typed. The fold runs in Postgres, the
// same lower() the unique index users_lower_email_key is built over, so the
// query and the index cannot disagree about case.
const EmailIdentityWhere = "lower(email) = lower(?)"

// SameEmailIdentity reports whether two addresses name the same mailbox, for
// the comparisons that happen in Go rather than in SQL. It is the in-process
// spelling of EmailIdentityWhere and must keep answering the same question.
//
// strings.EqualFold is Unicode simple case-folding, which can disagree with
// Postgres lower() on exotic input. Callers here compare an address against
// one the same row supplied, so the two spellings only have to agree on what
// counts as the same mailbox, never on what the index permits to exist.
func SameEmailIdentity(a, b string) bool {
	return strings.EqualFold(a, b)
}
