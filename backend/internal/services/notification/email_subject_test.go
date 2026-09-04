package notification

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/services/contracts"
)

// injectionProbe is the hostile value every sender is fed below: a header
// terminator followed by a header a sender must never be able to add.
const injectionProbe = "Foo\r\nBcc: victim@example.test"

// assertHeaderSafe checks what a Subject must hold, whatever went into it: no
// rune that can end a header line, and so no way to start a second one.
//
// It does NOT assert that "Bcc:" is absent. The probe's text is expected to
// survive as ordinary subject copy on the same line; what defeats the injection
// is that the line break in front of it is gone. Asserting the substring away
// would be asserting something the transform does not, and need not, provide.
func assertHeaderSafe(t *testing.T, subject string) {
	t.Helper()
	assert.NotContains(t, subject, "\r", "a CR ends a header line: %q", subject)
	assert.NotContains(t, subject, "\n", "an LF ends a header line: %q", subject)
	assert.NotContains(t, subject, "\r\nBcc:", "the probe's header line survived: %q", subject)
	for _, r := range subject {
		assert.False(t, unicode.IsControl(r),
			"control rune %U survived into the subject: %q", r, subject)
	}
}

// =============================================================================
// The builder
// =============================================================================

func TestHeaderSafeSubject(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is untouched", "Verify your email", "Verify your email"},
		{"CRLF injection collapses to one line", injectionProbe, "Foo Bcc: victim@example.test"},
		{"a bare LF collapses too", "Foo\nBcc: x@y", "Foo Bcc: x@y"},
		{"a bare CR collapses too", "Foo\rBcc: x@y", "Foo Bcc: x@y"},
		{"C0 controls go away", "Fo\x00o\x07 Bar", "Fo o Bar"},
		{"DEL goes away", "Foo\x7fBar", "Foo Bar"},
		{"C1 controls go away", "Foo\u0085\u0090Bar", "Foo Bar"},
		{"a tab is whitespace", "Foo\tBar", "Foo Bar"},
		{"line and paragraph separators are whitespace", "Foo\u2028Bar\u2029Baz", "Foo Bar Baz"},
		{"a non-breaking space is whitespace", "Foo\u00a0Bar", "Foo Bar"},
		{"runs of whitespace collapse", "Foo   \t  Bar", "Foo Bar"},
		{"the ends are trimmed", "  \r\n Foo Bar \n\n ", "Foo Bar"},
		{"a subject that is only whitespace empties", " \r\n\t ", ""},
		{"non-ASCII passes through unencoded", "Björk announced a show", "Björk announced a show"},
		{"emoji pass through", "🎸 Show tonight", "🎸 Show tonight"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, headerSafeSubject(tt.in))
		})
	}
}

// TestHeaderSafeSubjectCaps pins the length bound, which is what keeps a
// multi-kilobyte scraped name from producing a provider-dependent rejection
// that the alert paths log and swallow.
func TestHeaderSafeSubjectCaps(t *testing.T) {
	got := headerSafeSubject(strings.Repeat("a", maxEmailSubjectRunes+50))
	assert.Equal(t, maxEmailSubjectRunes+1, len([]rune(got)),
		"the cap counts runes and the ellipsis is the one rune added")
	assert.True(t, strings.HasSuffix(got, "…"), "a cut subject says it was cut")

	// Runes, not bytes: a multi-byte name must not be sliced into invalid UTF-8.
	multiByte := headerSafeSubject(strings.Repeat("é", maxEmailSubjectRunes+50))
	assert.Equal(t, maxEmailSubjectRunes+1, len([]rune(multiByte)))
	assert.True(t, strings.ContainsRune(multiByte, 'é'))
}

// =============================================================================
// The guard
// =============================================================================

// TestResendRequestsAreBuiltOnlyBySend is the structural half of this defence.
// headerSafeSubject cannot be forgotten by a new sender as long as the request
// is built in exactly one function and the client is called from exactly one
// function, so this test fails a second construction site or a second call
// rather than trusting a convention.
//
// The check is syntactic on purpose. It runs against the package's own source,
// so it catches a sender added tomorrow, which a test that enumerates today's
// senders cannot.
func TestResendRequestsAreBuiltOnlyBySend(t *testing.T) {
	const chokepoint = "send"

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	checked := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++

		file, parseErr := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		require.NoError(t, parseErr, "parsing %s", name)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CompositeLit:
					if isSelector(node.Type, "resend", "SendEmailRequest") && fn.Name.Name != chokepoint {
						t.Errorf("%s: %s builds a resend.SendEmailRequest; only %s may, "+
							"so that headerSafeSubject cannot be bypassed",
							fset.Position(node.Pos()), fn.Name.Name, chokepoint)
					}
				case *ast.CallExpr:
					if isEmailsSend(node.Fun) && fn.Name.Name != chokepoint {
						t.Errorf("%s: %s calls the Resend client directly; only %s may",
							fset.Position(node.Pos()), fn.Name.Name, chokepoint)
					}
				}
				return true
			})
		}
	}

	require.Greater(t, checked, 5, "the guard scanned almost nothing; the walk is broken")
}

// isSelector reports whether expr is the qualified identifier pkg.name, with or
// without a leading &.
func isSelector(expr ast.Expr, pkg, name string) bool {
	if unary, ok := expr.(*ast.UnaryExpr); ok {
		expr = unary.X
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

// isEmailsSend reports whether expr selects Send off an Emails field, which is
// the resend-go client's only send entry point (Emails.Send / Emails.SendWithContext).
func isEmailsSend(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || !strings.HasPrefix(sel.Sel.Name, "Send") {
		return false
	}
	inner, ok := sel.X.(*ast.SelectorExpr)
	return ok && inner.Sel.Name == "Emails"
}

// TestGuardRejectsASecondConstructionSite proves the guard is not vacuous:
// the same walk over a synthetic file that builds a request outside send must
// report both an extra construction site and an extra client call.
func TestGuardRejectsASecondConstructionSite(t *testing.T) {
	const rogue = `package notification

func sendRogue(s *EmailService) error {
	params := &resend.SendEmailRequest{Subject: "raw\r\nBcc: x@y"}
	_, err := s.client.Emails.Send(params)
	return err
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "rogue.go", rogue, 0)
	require.NoError(t, err)

	var builds, calls int
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				if isSelector(node.Type, "resend", "SendEmailRequest") && fn.Name.Name != "send" {
					builds++
				}
			case *ast.CallExpr:
				if isEmailsSend(node.Fun) && fn.Name.Name != "send" {
					calls++
				}
			}
			return true
		})
	}

	assert.Equal(t, 1, builds, "the guard must see a request built outside send")
	assert.Equal(t, 1, calls, "the guard must see the client called outside send")
}

// =============================================================================
// Per-sender injection
// =============================================================================

// TestEverySenderSubjectIsHeaderSafe drives every EmailService sender with the
// injection probe in each of its contributor-writable inputs and asserts on the
// Subject the transport actually received.
//
// The senders with fixed copy are here too. They have no interpolation point
// today, and the assertion is what says so: it fails the day one grows an
// interpolated value that does not go through send.
func TestEverySenderSubjectIsHeaderSafe(t *testing.T) {
	const unsubURL = "http://api.test.local/unsubscribe?uid=1&sig=abc"

	senders := []struct {
		name string
		call func(svc *EmailService) error
	}{
		{"verification", func(svc *EmailService) error {
			return svc.SendVerificationEmail("u@test.com", injectionProbe)
		}},
		{"magic_link", func(svc *EmailService) error {
			return svc.SendMagicLinkEmail("u@test.com", injectionProbe)
		}},
		{"account_recovery", func(svc *EmailService) error {
			return svc.SendAccountRecoveryEmail("u@test.com", injectionProbe, 7)
		}},
		{"show_reminder", func(svc *EmailService) error {
			return svc.SendShowReminderEmail("u@test.com", injectionProbe, "http://x/s", unsubURL,
				contracts.LocalizedEventTime{}, []string{injectionProbe})
		}},
		{"filter_notification", func(svc *EmailService) error {
			return svc.SendFilterNotificationEmail("u@test.com", injectionProbe, "<p>body</p>", unsubURL)
		}},
		{"tier_promotion", func(svc *EmailService) error {
			return svc.SendTierPromotionEmail("u@test.com", injectionProbe, "new_user", injectionProbe,
				injectionProbe, unsubURL, []string{injectionProbe})
		}},
		{"tier_demotion", func(svc *EmailService) error {
			return svc.SendTierDemotionEmail("u@test.com", injectionProbe, "contributor", "new_user",
				injectionProbe, unsubURL)
		}},
		{"tier_demotion_warning", func(svc *EmailService) error {
			return svc.SendTierDemotionWarningEmail("u@test.com", injectionProbe, injectionProbe,
				0.2, 0.5, unsubURL)
		}},
		{"edit_approved", func(svc *EmailService) error {
			return svc.SendEditApprovedEmail("u@test.com", injectionProbe, "artist", injectionProbe,
				"http://x/a", unsubURL)
		}},
		{"edit_rejected", func(svc *EmailService) error {
			return svc.SendEditRejectedEmail("u@test.com", injectionProbe, "artist", injectionProbe,
				injectionProbe, unsubURL)
		}},
		{"comment_notification", func(svc *EmailService) error {
			return svc.SendCommentNotification("u@test.com", injectionProbe, "artist", injectionProbe,
				injectionProbe, "http://x/a", unsubURL)
		}},
		{"mention_notification", func(svc *EmailService) error {
			return svc.SendMentionNotification("u@test.com", injectionProbe, "artist", injectionProbe,
				injectionProbe, "http://x/a", unsubURL)
		}},
		{"collection_digest", func(svc *EmailService) error {
			return svc.SendCollectionDigestEmail("u@test.com", []contracts.CollectionDigestGroup{{
				CollectionTitle: injectionProbe,
				CollectionURL:   "http://x/c",
				Items: []contracts.CollectionDigestEntry{{
					EntityName: injectionProbe, EntityType: "artist", EntityURL: "http://x/a",
					AddedBy: injectionProbe,
				}},
			}}, unsubURL)
		}},
		{"scene_digest", func(svc *EmailService) error {
			return svc.SendSceneDigestEmail("u@test.com", []contracts.SceneDigestGroup{{
				SceneName: injectionProbe,
				SceneURL:  "http://x/s",
				Shows: []contracts.SceneDigestShow{{
					DisplayTitle: injectionProbe, Date: "Sat Aug 29", VenueName: injectionProbe,
					ShowURL: "http://x/show",
				}},
			}}, unsubURL)
		}},
	}

	for _, sender := range senders {
		t.Run(sender.name, func(t *testing.T) {
			svc, emails, _ := setupEmailTest(t)
			require.NoError(t, sender.call(svc))
			assertHeaderSafe(t, (<-emails).Subject)
		})
	}
}

// TestFilterSenderSubjectsAreHeaderSafe covers the four subjects composed
// outside EmailService and handed to SendFilterNotificationEmail: the filter
// match and scene-follow subjects in filter_service.go, and the artist and
// venue alert subjects in artist_follow_notify.go / venue_follow_notify.go.
//
// They are exercised through the sender they all share rather than through
// their own services, which need a database. That is the whole coverage claim:
// a subject composed anywhere upstream becomes header-safe by passing through
// this one method.
func TestFilterSenderSubjectsAreHeaderSafe(t *testing.T) {
	upstream := map[string]string{
		"filter match": `New show matching "` + injectionProbe + `"`,
		"scene follow": "New show in " + injectionProbe,
		"artist alert": injectionProbe + " announced a show",
		"venue alert":  "2 shows at " + injectionProbe,
	}

	for name, subject := range upstream {
		t.Run(name, func(t *testing.T) {
			svc, emails, _ := setupEmailTest(t)
			require.NoError(t, svc.SendFilterNotificationEmail(
				"u@test.com", subject, "<p>body</p>", "http://api.test.local/u"))
			assertHeaderSafe(t, (<-emails).Subject)
		})
	}
}
