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
// 200 is well past what any client displays (inbox lists show roughly the first
// 70 characters), and it is a bound on what a provider is asked to accept, not
// on line length: RFC 2047 encoded-words are folded across continuation lines,
// so no single line reaches the 998-octet limit whatever this value is.
const maxEmailSubjectRunes = 200

// maxEmailSubjectEntityRunes bounds one scraped entity name, so that an
// overlong name is what gets cut rather than the copy around it. The artist and
// venue alert subjects apply it through subjectEntityName; the other senders
// rely on the whole-subject cap alone.
const maxEmailSubjectEntityRunes = 120

// subjectEntityName prepares a scraped entity name for interpolation into a
// Subject: header-safe first, bounded second, so that the bound is spent on
// runes that will still be there. Bounding first would let a name arriving with
// leading whitespace or control runes, which an HTML scrape routinely produces,
// spend its whole budget on runes headerSafeSubject then discards.
func subjectEntityName(name string) string {
	return truncateRunes(headerSafeSubject(name), maxEmailSubjectEntityRunes)
}

// headerSafeSubject makes a string safe to hand to a Subject header: no rune
// that can end a header line survives it, so an interpolated value cannot start
// a second header. Whitespace collapses because a subject is one line by
// definition, and joining with a space rather than deleting keeps the words on
// either side of a stripped run apart.
//
// NOT RFC 2047 encoded. resend-go serializes SendEmailRequest as JSON to the
// Resend HTTP API (resend-go/v2 Client.NewRequest), so the value crosses the
// wire as a JSON string and Resend composes the MIME message, including the
// encoded-word wrapping a non-ASCII subject needs. Encoding here would put a
// literal `=?utf-8?q?...?=` into that JSON for Resend to encode a second time.
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
