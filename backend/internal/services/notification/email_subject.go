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
//
// maxEmailSubjectEntityRunes is the tighter per-NAME bound a caller applies
// before interpolation, so that a long name is what gets cut rather than the
// rest of the sentence.
const maxEmailSubjectRunes = 200

// maxEmailSubjectEntityRunes bounds a single scraped entity name interpolated
// into a Subject, leaving room for the fixed copy around it.
const maxEmailSubjectEntityRunes = 120

// headerSafeSubject makes a string safe to hand to a Subject header, and is the
// only transform applied to a subject anywhere in this package. EmailService.send
// calls it on every outbound message; nothing else needs to.
//
// Escaping is medium-specific, and this package's other escaping is aimed at the
// BODY: html.EscapeString makes a scraped artist name safe inside markup and does
// nothing at all for a Subject, where the dangerous character is not `<` but a
// newline. Show titles, venue names and artist names reach these subjects from
// ingest without review, and the messages ship from the platform's own
// DKIM-aligned sender.
//
// Three transforms, in order:
//
//   - Control runes go away. That is the C0 range (CR and LF among them), DEL,
//     and the C1 range, exactly Unicode's Cc category.
//   - Runs of whitespace collapse to one space and the ends are trimmed, so what
//     a control rune leaves behind cannot be a run of blanks, and neither can a
//     value that arrived padded.
//   - The result is capped at maxEmailSubjectRunes.
//
// Whitespace-collapsing rather than deleting: a subject is a single line by
// definition, so nothing is lost by flattening, and joining the two sides with a
// space keeps the words apart where a deletion would run them together.
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
