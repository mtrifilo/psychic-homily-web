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
	assert.Equal(t, "Valley Bar", entityNameForSubject(padded),
		"leading line breaks must not consume the entity budget")

	spacePadded := strings.Repeat(" ", 40) + strings.Repeat("V", 100)
	assert.Equal(t, strings.Repeat("V", 100), entityNameForSubject(spacePadded),
		"leading spaces must not consume the entity budget either")

	// The bound itself still applies, to the sanitized value.
	assert.Equal(t, maxEmailSubjectEntityRunes+1,
		len([]rune(entityNameForSubject(strings.Repeat("V", maxEmailSubjectEntityRunes+50)))))
}

// =============================================================================
// The guard
// =============================================================================

// chokepointMethod is the one method allowed to name resend.SendEmailRequest or
// reach the Resend client, identified by receiver as well as by name so that a
// method called send on some other type is not silently exempt.
const (
	chokepointMethod   = "send"
	chokepointReceiver = "EmailService"
)

// rogueSendSites walks one parsed file and returns the positions where
// resend.SendEmailRequest is named, or the Resend client is reached, outside
// the chokepoint. One walk, so the guard below and the meta-test that proves
// the guard can fail assert on the same predicate.
//
// The walk covers the whole file rather than its function declarations, so a
// package-level var holding a request or a func literal that sends is caught.
// It looks for the TYPE NAME rather than for a composite literal, so
// `var p resend.SendEmailRequest` and `new(resend.SendEmailRequest)` are caught
// too, and for the client's service FIELDS rather than for a Send call, so
// hoisting one into a local (`c := s.client.Emails; c.Send(p)`) is caught as
// well. Emails and Batch are both fields on resend.Client that can send. The
// resend package's local name comes from the file's own imports; a dot-import
// makes every identifier ambiguous, so the walk reports the import itself.
func rogueSendSites(fset *token.FileSet, file *ast.File) (requestSites, clientSites []token.Position) {
	pkg, dot := resendLocalName(file)
	if dot {
		requestSites = append(requestSites, fset.Position(file.Pos()))
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && isChokepoint(fn) {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch {
		case pkg != "" && sel.Sel.Name == "SendEmailRequest" && isIdent(sel.X, pkg):
			requestSites = append(requestSites, fset.Position(sel.Pos()))
		case isResendService(sel):
			clientSites = append(clientSites, fset.Position(sel.Pos()))
		}
		return true
	})
	return requestSites, clientSites
}

// isResendService reports whether sel selects a sending service off a field
// named client, e.g. s.client.Emails or b.client.Batch. Anchoring on the field
// name keeps an unrelated struct field called Emails from failing the guard.
func isResendService(sel *ast.SelectorExpr) bool {
	if sel.Sel.Name != "Emails" && sel.Sel.Name != "Batch" {
		return false
	}
	inner, ok := sel.X.(*ast.SelectorExpr)
	return ok && inner.Sel.Name == "client"
}

// isChokepoint reports whether fn is EmailService.send.
func isChokepoint(fn *ast.FuncDecl) bool {
	if fn.Name.Name != chokepointMethod || fn.Recv == nil || len(fn.Recv.List) != 1 {
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

// resendLocalName returns the name the resend SDK is bound to in this file and
// whether it is dot-imported. The name is "" if the file does not import it.
func resendLocalName(file *ast.File) (name string, dot bool) {
	for _, imp := range file.Imports {
		if imp.Path == nil || !strings.Contains(imp.Path.Value, "resend-go") {
			continue
		}
		if imp.Name == nil {
			return "resend", false
		}
		if imp.Name.Name == "." {
			return "", true
		}
		return imp.Name.Name, false
	}
	return "", false
}

// TestOnlySendReachesResend is the structural half of this defence.
// headerSafeSubject cannot be forgotten by a new sender as long as the request
// type is named in exactly one method and the client is reached from exactly
// one method, so this test fails a second construction site or a second call
// rather than trusting a convention.
//
// The check is syntactic on purpose. It runs against the package's own source,
// so it catches a sender added tomorrow, which a test that enumerates today's
// senders cannot. Its reach is this directory: a new package that imported the
// SDK would be outside it.
func TestOnlySendReachesResend(t *testing.T) {
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

		requestSites, clientSites := rogueSendSites(fset, file)
		for _, pos := range requestSites {
			t.Errorf("%s: names resend.SendEmailRequest outside %s.%s, "+
				"which is how headerSafeSubject gets bypassed",
				pos, chokepointReceiver, chokepointMethod)
		}
		for _, pos := range clientSites {
			t.Errorf("%s: reaches the Resend client outside %s.%s",
				pos, chokepointReceiver, chokepointMethod)
		}
	}

	require.Greater(t, checked, 5, "the guard scanned almost nothing; the walk is broken")
}

// TestGuardRejectsASecondConstructionSite proves the guard is not vacuous, and
// proves it against the evasions a guard like this usually misses rather than
// only against the copy-paste case. Every row here was a live hole at some
// point in this ticket's review.
func TestGuardRejectsASecondConstructionSite(t *testing.T) {
	tests := []struct {
		name                      string
		src                       string
		requestSites, clientSites int
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
			requestSites: 1, clientSites: 1,
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
			requestSites: 1, clientSites: 1,
		},
		{
			name: "a package-level var and a func literal",
			src: `package notification

import resend "github.com/resend/resend-go/v2"

var leaked = &resend.SendEmailRequest{Subject: "raw"}

var doSend = func(s *EmailService) {
	_, _ = s.client.Emails.Send(leaked)
}
`,
			requestSites: 1, clientSites: 1,
		},
		{
			name: "the batch service instead of Emails",
			src: `package notification

import resend "github.com/resend/resend-go/v2"

type digestBatcher struct {
	client  *resend.Client
	pending []*resend.SendEmailRequest
}

func (b *digestBatcher) flush() error {
	_, err := b.client.Batch.Send(b.pending)
	return err
}
`,
			requestSites: 1, clientSites: 1,
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
			requestSites: 1, clientSites: 1,
		},
		{
			name: "a dot import",
			src: `package notification

import . "github.com/resend/resend-go/v2"

func sendRogue(s *EmailService) error {
	_, err := s.client.Emails.Send(&SendEmailRequest{Subject: "raw"})
	return err
}
`,
			requestSites: 1, clientSites: 1,
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
			requestSites: 0, clientSites: 1,
		},
		{
			name: "a method named send on another type",
			src: `package notification

import resend "github.com/resend/resend-go/v2"

func (s *NotificationFilterService) send(p *resend.SendEmailRequest) error {
	return nil
}
`,
			requestSites: 1, clientSites: 0,
		},
		{
			name: "an unrelated field called Emails is not a client",
			src: `package notification

type digest struct{ Emails []string }

func count(d digest) int { return len(d.Emails) }
`,
			requestSites: 0, clientSites: 0,
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
			requestSites: 0, clientSites: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "rogue.go", tt.src, 0)
			require.NoError(t, err)

			requestSites, clientSites := rogueSendSites(fset, file)
			assert.Len(t, requestSites, tt.requestSites, "request-type sites")
			assert.Len(t, clientSites, tt.clientSites, "client sites")
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
		// list is false for a message with no list to leave, which is the
		// only reason send may omit the RFC 8058 headers.
		list bool
	}{
		{"verification", func(svc *EmailService) error {
			return svc.SendVerificationEmail("u@test.com", injectionProbe)
		}, false},
		{"magic_link", func(svc *EmailService) error {
			return svc.SendMagicLinkEmail("u@test.com", injectionProbe)
		}, false},
		{"account_recovery", func(svc *EmailService) error {
			return svc.SendAccountRecoveryEmail("u@test.com", injectionProbe, 7)
		}, false},
		{"show_reminder", func(svc *EmailService) error {
			return svc.SendShowReminderEmail("u@test.com", injectionProbe, "http://x/s", unsubURL,
				contracts.LocalizedEventTime{}, []string{injectionProbe})
		}, true},
		{"filter_notification", func(svc *EmailService) error {
			return svc.SendFilterNotificationEmail("u@test.com", injectionProbe, "<p>body</p>", unsubURL)
		}, true},
		{"tier_promotion", func(svc *EmailService) error {
			return svc.SendTierPromotionEmail("u@test.com", injectionProbe, "new_user", injectionProbe,
				injectionProbe, unsubURL, []string{injectionProbe})
		}, true},
		{"tier_demotion", func(svc *EmailService) error {
			return svc.SendTierDemotionEmail("u@test.com", injectionProbe, "contributor", "new_user",
				injectionProbe, unsubURL)
		}, true},
		{"tier_demotion_warning", func(svc *EmailService) error {
			return svc.SendTierDemotionWarningEmail("u@test.com", injectionProbe, injectionProbe,
				0.2, 0.5, unsubURL)
		}, true},
		{"edit_approved", func(svc *EmailService) error {
			return svc.SendEditApprovedEmail("u@test.com", injectionProbe, "artist", injectionProbe,
				"http://x/a", unsubURL)
		}, true},
		{"edit_rejected", func(svc *EmailService) error {
			return svc.SendEditRejectedEmail("u@test.com", injectionProbe, "artist", injectionProbe,
				injectionProbe, unsubURL)
		}, true},
		{"comment_notification", func(svc *EmailService) error {
			return svc.SendCommentNotification("u@test.com", injectionProbe, "artist", injectionProbe,
				injectionProbe, "http://x/a", unsubURL)
		}, true},
		{"mention_notification", func(svc *EmailService) error {
			return svc.SendMentionNotification("u@test.com", injectionProbe, "artist", injectionProbe,
				injectionProbe, "http://x/a", unsubURL)
		}, true},
		{"collection_digest", func(svc *EmailService) error {
			return svc.SendCollectionDigestEmail("u@test.com", []contracts.CollectionDigestGroup{{
				CollectionTitle: injectionProbe,
				CollectionURL:   "http://x/c",
				Items: []contracts.CollectionDigestEntry{{
					EntityName: injectionProbe, EntityType: "artist", EntityURL: "http://x/a",
					AddedBy: injectionProbe,
				}},
			}}, unsubURL)
		}, true},
		{"scene_digest", func(svc *EmailService) error {
			return svc.SendSceneDigestEmail("u@test.com", []contracts.SceneDigestGroup{{
				SceneName: injectionProbe,
				SceneURL:  "http://x/s",
				Shows: []contracts.SceneDigestShow{{
					DisplayTitle: injectionProbe, Date: "Sat Aug 29", VenueName: injectionProbe,
					ShowURL: "http://x/show",
				}},
			}}, unsubURL)
		}, true},
	}

	for _, sender := range senders {
		t.Run(sender.name, func(t *testing.T) {
			svc, emails, _ := setupEmailTest(t)
			require.NoError(t, sender.call(svc))

			sent := <-emails
			assertHeaderSafe(t, sent.Subject)

			// send omits the unsubscribe headers when unsubscribeURL is
			// empty, and an unset struct field is empty, so forgetting the
			// field on a new list sender is a silent RFC 8058 regression.
			// Assert it here, where every sender is already enumerated.
			if sender.list {
				assert.Equal(t, "<"+unsubURL+">", sent.Headers["List-Unsubscribe"],
					"a list message must carry List-Unsubscribe")
				assert.Equal(t, "List-Unsubscribe=One-Click", sent.Headers["List-Unsubscribe-Post"],
					"a list message must advertise RFC 8058 one-click")
			} else {
				assert.NotContains(t, sent.Headers, "List-Unsubscribe",
					"a message with no list to leave must not offer one")
			}
		})
	}
}

// TestSubjectCopySurvivesAnOverlongName pins what the entity bound is for. The
// whole-subject cap alone would cut from the end, so a scraped name long enough
// to fill it takes the sentence with it and the reader learns nothing.
func TestSubjectCopySurvivesAnOverlongName(t *testing.T) {
	huge := strings.Repeat("\u03a9", maxEmailSubjectRunes+100)

	t.Run("show reminder", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		require.NoError(t, svc.SendShowReminderEmail("u@test.com", huge, "http://x/s",
			"http://api.test.local/u", contracts.LocalizedEventTime{}, nil))
		assert.Contains(t, (<-emails).Subject, "is tomorrow")
	})

	t.Run("collection digest", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		require.NoError(t, svc.SendCollectionDigestEmail("u@test.com",
			[]contracts.CollectionDigestGroup{{
				CollectionTitle: huge,
				CollectionURL:   "http://x/c",
				Items: []contracts.CollectionDigestEntry{{
					EntityName: "A", EntityType: "artist", EntityURL: "http://x/a", AddedBy: "B",
				}},
			}}, "http://api.test.local/u"))
		assert.Contains(t, (<-emails).Subject, "1 item")
	})

	t.Run("scene digest", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		require.NoError(t, svc.SendSceneDigestEmail("u@test.com",
			[]contracts.SceneDigestGroup{{
				SceneName: huge,
				SceneURL:  "http://x/s",
				Shows: []contracts.SceneDigestShow{{
					DisplayTitle: "A", Date: "Sat Aug 29", VenueName: "V", ShowURL: "http://x/show",
				}},
			}}, "http://api.test.local/u"))
		assert.Contains(t, (<-emails).Subject, "The next 7 days in")
	})
}

// TestFilterSenderSubjectsAreHeaderSafe records the SHAPE of the four subjects
// composed outside EmailService and handed to SendFilterNotificationEmail, and
// asserts that shape survives the shared sender.
//
// It does not execute filter_service.go, artist_follow_notify.go or
// venue_follow_notify.go: the strings below are copies, so a change to one of
// those format strings does not fail here. What it does establish is the claim
// that matters for this ticket, which the composers themselves cannot: whatever
// an upstream composer hands the shared sender comes out header-safe.
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
