package middleware

import (
	"regexp"

	"psychic-homily-backend/internal/observability"
)

// redactPanicText redacts a panic value's text: the secret shapes
// observability.RedactSecrets knows, then email addresses. Secrets go first,
// so an email match, which can run into the text around it, never takes a
// secret's key and leaves its value for the key=value rules to miss. The
// result carries no length cap; a caller caps it afterwards, so the cap cannot
// cut an address into a fragment the email pattern no longer matches.
func redactPanicText(s string) string {
	return redactEmails(observability.RedactSecrets(s))
}

// emailShapeRe matches an '@', or its percent-encoding (%40, or %2540 encoded
// twice), with a run of non-delimiter characters on each side: the local part
// and the domain of anything shaped like an email address. Repeated and
// chained separators ("jane@@example.com", "a@jane@example.com") match whole.
// The delimiters are whitespace, double quotes, backticks, brackets, braces,
// parentheses and the separators ,;:/\?#, so the key in
// "Email:jane@example.com", a struct field or a quoted JSON value stays outside
// the match. The pattern over-matches, which
// is safe because secrets are already redacted when it runs: a module path
// such as "pkg@v1.2.3" matches, and "key=jane@example.com" loses its key.
var emailShapeRe = regexp.MustCompile(emailPart + "(?:(?:@|%40|%2540)+" + emailPart + ")+")

const emailPart = "[^\\s@\"`<>()\\[\\]{},;:/\\\\?#]+"

// redactedEmail replaces each email-shaped match, marking where an address
// was.
const redactedEmail = "[REDACTED]@[REDACTED]"

// redactEmails replaces everything in s shaped like an email address.
func redactEmails(s string) string {
	return emailShapeRe.ReplaceAllLiteralString(s, redactedEmail)
}
