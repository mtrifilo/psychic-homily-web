package middleware

import "regexp"

// emailShapeRe matches an '@' (or its percent-encoding %40) with a run of
// non-delimiter characters on each side: the local part and the domain of
// anything shaped like an email address. The delimiters are whitespace,
// quotes, brackets, braces, parentheses and the separators ,;:=/\?#. Because
// ':' and '=' delimit, the key in "password=P@ss#tail" or
// "Email:jane@example.com" stays outside the match, so the key=value rules in
// observability.ScrubText still find the key and redact the whole value after
// it. A module path such as "pkg@v1.2.3" also matches; over-matching the value
// side is the safe direction.
var emailShapeRe = regexp.MustCompile(emailPart + "(?:@|%40)" + emailPart)

const emailPart = "[^\\s@\"'`<>()\\[\\]{},;:=/\\\\?#]+"

// redactedEmail replaces each email-shaped match. It keeps the '@' so the URL
// userinfo rule in observability.ScrubText still sees scheme://user:pw@host and
// redacts the user name along with the password.
const redactedEmail = "[REDACTED]@[REDACTED]"

// redactEmails replaces everything in s shaped like an email address.
func redactEmails(s string) string {
	return emailShapeRe.ReplaceAllLiteralString(s, redactedEmail)
}
