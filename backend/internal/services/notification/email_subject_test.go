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

// TestSubjectEntityNameSanitizesBeforeBounding pins the ORDER of the two
// transforms. A scraped name routinely arrives with leading whitespace or line
// breaks, and bounding first spends the whole budget on runes that are then
// discarded, so the name vanishes from a subject that still has room for it.
func TestSubjectEntityNameSanitizesBeforeBounding(t *testing.T) {
	padded := strings.Repeat("\r\n", 60) + "Valley Bar"
	assert.Equal(t, "Valley Bar", subjectEntityName(padded),
		"leading line breaks must not consume the entity budget")

	spacePadded := strings.Repeat(" ", 40) + strings.Repeat("V", 100)
	assert.Equal(t, strings.Repeat("V", 100), subjectEntityName(spacePadded),
		"leading spaces must not consume the entity budget either")

	// The bound itself still applies, to the sanitized value.
	assert.Equal(t, maxEmailSubjectEntityRunes+1,
		len([]rune(subjectEntityName(strings.Repeat("V", maxEmailSubjectEntityRunes+50)))))
}

// =============================================================================
// The guard
// =============================================================================

// chokepoint is the one method allowed to name resend.SendEmailRequest or reach
// the Resend client, identified by receiver as well as by name so that a method
// called send on some other type is not silently exempt.
const (
	chokepoint         = "send"
	chokepointReceiver = "EmailService"
)

// rogueSendSites walks one parsed file and returns the positions where
// resend.SendEmailRequest is named, or the Resend client's Emails field is
// touched, outside the chokepoint. One walk, so the guard below and the
// meta-test that proves the guard can fail assert on the same predicate.
//
// It looks for the TYPE NAME rather than for a composite literal, so
// `var p resend.SendEmailRequest` and `new(resend.SendEmailRequest)` are caught
// too, and for the Emails FIELD rather than for a Send call, so hoisting the
// service into a local (`c := s.client.Emails; c.Send(p)`) is caught as well.
// The resend package's local name is read from the file's own imports, so an
// aliased import does not slip past.
func rogueSendSites(fset *token.FileSet, file *ast.File) (requests, clients []token.Position) {
	pkg := resendLocalName(file)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || isChokepoint(fn) {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch {
			case pkg != "" && sel.Sel.Name == "SendEmailRequest" && isIdent(sel.X, pkg):
				requests = append(requests, fset.Position(sel.Pos()))
			case sel.Sel.Name == "Emails":
				clients = append(clients, fset.Position(sel.Pos()))
			}
			return true
		})
	}
	return requests, clients
}

// isChokepoint reports whether fn is EmailService.send.
func isChokepoint(fn *ast.FuncDecl) bool {
	if fn.Name.Name != chokepoint || fn.Recv == nil || len(fn.Recv.List) != 1 {
		return false
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	return isIdent(recv, chokepointReceiver)
}

func isIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

// resendLocalName returns the name the resend SDK is bound to in this file, or
// "" if the file does not import it.
func resendLocalName(file *ast.File) string {
	for _, imp := range file.Imports {
		if imp.Path == nil || !strings.Contains(imp.Path.Value, "resend-go") {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "resend"
	}
	return ""
}

// TestResendRequestsAreBuiltOnlyBySend is the structural half of this defence.
// headerSafeSubject cannot be forgotten by a new sender as long as the request
// type is named in exactly one method and the client is reached from exactly
// one method, so this test fails a second construction site or a second call
// rather than trusting a convention.
//
// The check is syntactic on purpose. It runs against the package's own source,
// so it catches a sender added tomorrow, which a test that enumerates today's
// senders cannot.
func TestResendRequestsAreBuiltOnlyBySend(t *testing.T) {
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

		requests, clients := rogueSendSites(fset, file)
		for _, pos := range requests {
			t.Errorf("%s: names resend.SendEmailRequest outside %s.%s, "+
				"which is how headerSafeSubject gets bypassed", pos, chokepointReceiver, chokepoint)
		}
		for _, pos := range clients {
			t.Errorf("%s: reaches the Resend client outside %s.%s",
				pos, chokepointReceiver, chokepoint)
		}
	}

	require.Greater(t, checked, 5, "the guard scanned almost nothing; the walk is broken")
}

// TestGuardRejectsASecondConstructionSite proves the guard is not vacuous, and
// proves it against the evasions a guard like this usually misses rather than
// only against the copy-paste case.
func TestGuardRejectsASecondConstructionSite(t *testing.T) {
	tests := []struct {
		name              string
		src               string
		requests, clients int
	}{
		{
			name: "the copy-paste case",
			src: `package notification

import resend "github.com/resend/resend-go/v2"

func sendRogue(s *EmailService) error {
	params := &resend.SendEmailRequest{Subject: "raw"}
	_, err := s.client.Emails.Send(params)
	return err
}
`,
			requests: 1, clients: 1,
		},
		{
			name: "a var declaration instead of a composite literal",
			src: `package notification

import resend "github.com/resend/resend-go/v2"

func sendRogue(s *EmailService) error {
	var p resend.SendEmailRequest
	p.Subject = "raw"
	_, err := s.client.Emails.Send(&p)
	return err
}
`,
			requests: 1, clients: 1,
		},
		{
			name: "an aliased import",
			src: `package notification

import r "github.com/resend/resend-go/v2"

func sendRogue(s *EmailService) error {
	_, err := s.client.Emails.Send(&r.SendEmailRequest{Subject: "raw"})
	return err
}
`,
			requests: 1, clients: 1,
		},
		{
			name: "the client hoisted into a local",
			src: `package notification

func sendRogue(s *EmailService, p any) error {
	c := s.client.Emails
	_, err := c.Send(p)
	return err
}
`,
			requests: 0, clients: 1,
		},
		{
			name: "a method named send on another type",
			src: `package notification

import resend "github.com/resend/resend-go/v2"

func (s *NotificationFilterService) send(p *resend.SendEmailRequest) error {
	return nil
}
`,
			requests: 1, clients: 0,
		},
		{
			name: "the chokepoint itself is exempt",
			src: `package notification

import resend "github.com/resend/resend-go/v2"

func (s *EmailService) send(msg outboundEmail) error {
	params := &resend.SendEmailRequest{Subject: headerSafeSubject(msg.subject)}
	_, err := s.client.Emails.Send(params)
	return err
}
`,
			requests: 0, clients: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "rogue.go", tt.src, 0)
			require.NoError(t, err)

			requests, clients := rogueSendSites(fset, file)
			assert.Len(t, requests, tt.requests, "request-type sites")
			assert.Len(t, clients, tt.clients, "client sites")
		})
	}
}

// =============================================================================
// Per-sender injection
// =============================================================================

// TestEverySenderSubjectIsHeaderSafe drives every EmailService sender with the
// injection probe in each of its contributor-writable inputs and asserts on the
// Subject the transport actually received.
//
// The three senders with constant subjects are here too. They take the probe as
// their token, which reaches only a URL in the body, so what their subtests
// assert is narrow: the constant arrives at the transport as written, and a
// subject that grew an interpolation of that token would fail here. A new field
// on one of them is not covered until this table is extended.
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
