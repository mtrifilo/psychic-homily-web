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
// 70 characters) and short enough that the encoded header stays inside the
// 998-octet line limit even at UTF-8 base64's worst-case expansion.
const maxEmailSubjectRunes = 200

// maxEmailSubjectEntityRunes bounds one scraped entity name before it is
// interpolated, so that an overlong name is what maxEmailSubjectRunes cuts
// rather than the copy around it. The artist and venue alert subjects apply it;
// the other senders rely on the whole-subject cap alone.
const maxEmailSubjectEntityRunes = 120

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
