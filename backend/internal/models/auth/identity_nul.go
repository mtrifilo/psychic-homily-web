package auth

import "strings"

// HoldsNUL reports whether value holds a NUL byte. Postgres text cannot store
// one, so no users row has such an email or username, and a query binding one
// fails with an encoding error rather than matching nothing. A lookup of a
// users row by a caller's value answers "not found" for such a value without
// querying.
func HoldsNUL(value string) bool {
	return strings.ContainsRune(value, 0)
}
