package auth

// EmailIdentityWhere is the WHERE fragment every lookup of a users row by
// address uses. It binds one argument: the address exactly as the caller
// typed it.
//
// One mailbox is one account: address identity is case-insensitive, while the
// stored bytes keep whatever casing the owner typed. The fold therefore lives
// in the comparison and nowhere else. A guard that lowercased on write would
// store one string and authenticate against another.
//
// Both sides run through Postgres lower(), which is also what the unique index
// on users (lower(email)) is built over. Folding the argument in Go instead
// would compare Go's case rules against the index's Postgres rules, so a row
// the index treats as a duplicate could still be missed by the lookup that is
// supposed to find it.
const EmailIdentityWhere = "lower(email) = lower(?)"
