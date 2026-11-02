package middleware

import "regexp"

// emailShapeRe matches an '@' with a run of non-delimiter characters on each
// side: the local part and the domain of anything shaped like an email address.
// The delimiters are whitespace, quotes, brackets, braces, parentheses and the
// separators ,;:/\?#, so a key name in "Email:jane@example.com", a struct field
// or a quoted JSON value stays outside the match. The pattern over-matches,
// which is the safe direction: a module path such as "pkg@v1.2.3" matches, and
// '=' is a local-part character, so "key=jane@example.com" loses its key too.
var emailShapeRe = regexp.MustCompile(emailPart + "@" + emailPart)

const emailPart = "[^\\s@\"'`<>()\\[\\]{},;:/\\\\?#]+"

// redactedEmail replaces each email-shaped match. It keeps the '@' so the URL
// userinfo rule in observability.ScrubText still sees scheme://user:pw@host and
// redacts the user name along with the password.
const redactedEmail = "[REDACTED]@[REDACTED]"

// redactEmails replaces everything in s shaped like an email address.
func redactEmails(s string) string {
	return emailShapeRe.ReplaceAllLiteralString(s, redactedEmail)
}
