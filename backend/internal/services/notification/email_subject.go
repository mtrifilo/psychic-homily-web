package notification

import (
	"strings"
	"unicode"
)

// maxEmailSubjectRunes bounds a whole assembled Subject.
//
// Sanitizing stops header SPLITTING; it does nothing about length, and an
// unfolded multi-kilobyte subject is provider-dependent behaviour ranging from
// truncation to outright rejection. A rejected send on the alert paths is
// logged and swallowed, so the failure mode is a silently lost notification.
//
// 200 bounds what the provider is asked to accept. It is not derived from the
// 998-octet line limit: an ASCII subject this long is about 209 octets, and a
// non-ASCII one is encoded and folded by Resend, not here.
const maxEmailSubjectRunes = 200

// maxEmailSubjectEntityRunes bounds one interpolated name, so that an overlong
// name is what maxEmailSubjectRunes cuts rather than the copy around it. A
// subject whose name ate "is tomorrow" says nothing.
const maxEmailSubjectEntityRunes = 120

// entityNameForSubject prepares a name for interpolation into a Subject:
// header-safe first, bounded second, so that the bound is spent on runes that
// will still be there. Bounding first would let a name arriving with leading
// whitespace or control runes, which an HTML scrape routinely produces, spend
// its whole budget on runes headerSafeSubject then discards.
//
// Callers that interpolate a scraped or contributor-authored name use this. The
// fixed copy around it does not need it.
func entityNameForSubject(name string) string {
	return truncateRunes(headerSafeSubject(name), maxEmailSubjectEntityRunes)
}

// headerSafeSubject makes a string safe to hand to a Subject header: no rune
// that can end a header line survives it, so an interpolated value cannot start
// a second header. Whitespace collapses because a subject is one line by
// definition, and joining with a space rather than deleting keeps the words on
// either side of a stripped run apart.
//
// It normalizes, so it can change a legitimate subject: runs of spaces become
// one, the ends are trimmed, and a value that is nothing but whitespace comes
// back empty.
//
// It removes Cc and nothing else. The Cf format characters, which include the
// bidi overrides, the zero-width joiners and U+FEFF, pass through: none of them
// can terminate a header, and stripping the set wholesale would break emoji
// sequences and Persian text. A name carrying U+202E therefore still reorders
// the rendered subject in a mail client, which is a spoofing surface this
// function does not close.
//
// NOT RFC 2047 encoded. resend-go serializes SendEmailRequest as JSON to the
// Resend HTTP API (resend-go/v2 Client.NewRequest), so the value crosses the
// wire as a JSON string, which is also why a raw CR reaching this point could
// not have split a header: encoding/json escapes it. Resend composes the MIME
// message, so encoding here would put a literal `=?utf-8?q?...?=` into that
// JSON for Resend to encode a second time.
//
// Invalid UTF-8 leaves as U+FFFD, because ranging a string decodes an
// unconvertible byte to exactly that.
func headerSafeSubject(subject string) string {
	var b strings.Builder
	b.Grow(len(subject))

	pendingSpace := false
	for _, r := range subject {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			// Deferred: a trailing run is never written, which trims the tail.
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace {
			b.WriteRune(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
	}

	return truncateRunes(b.String(), maxEmailSubjectRunes)
}
