package auth

import (
	"strings"
	"unicode/utf8"
)

// PostgresRejectsText reports whether Postgres refuses value as a UTF8 text
// parameter: value holds a NUL byte or is not valid UTF-8. No users row can
// hold such an email or username, and a query binding one fails with an
// encoding error rather than matching nothing, so a lookup of a users row by a
// caller's value answers "not found" for it without querying.
func PostgresRejectsText(value string) bool {
	return strings.IndexByte(value, 0) >= 0 || !utf8.ValidString(value)
}
