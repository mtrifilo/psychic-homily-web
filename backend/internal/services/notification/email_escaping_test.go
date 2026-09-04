package notification

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/services/contracts"
)

// hostilePayload is what a contributor can put in a show title, an artist or
// venue name, a username, a comment or a moderator's reason: markup, a working
// link to somewhere else, and the two characters that end an attribute.
//
// It is one string rather than a set so that a template only passes by escaping
// all of it. The link's host is a reserved TLD, so a body that somehow shipped
// this could not reach a real destination.
const hostilePayload = `<script>alert(1)</script> <a href="https://evil.test">click</a> "quoted" & ampersand`

// assertEscaped fails if any part of hostilePayload survived as markup.
//
// The assertions name characters rather than entities because the two renderers
// spell some entities differently (html/template writes &#34; where htmlEscape
// writes &quot;), and what matters is that no interpolated value can open a tag,
// end an attribute, or become a link.
func assertEscaped(t *testing.T, template string, body string) {
	t.Helper()
	assert.NotContains(t, body, "<script>", "%s: a raw tag reached the body", template)
	assert.NotContains(t, body, "</script>", "%s: a raw closing tag reached the body", template)
	assert.NotContains(t, body, `<a href="https://evil.test"`,
		"%s: contributor text became a working link from this platform's sender", template)
	assert.NotContains(t, body, `"quoted"`, "%s: a bare quote can end an attribute", template)
	assert.NotContains(t, body, "& ampersand", "%s: a bare ampersand starts an entity", template)
	assert.Contains(t, body, "&lt;script&gt;", "%s: the payload must survive as readable text", template)
}

// captureBody sends through the resend harness and returns the one captured
// message, so each case exercises the real send path rather than a builder the
// sender might not use.
func captureBody(t *testing.T, emails chan capturedEmail, err error) capturedEmail {
	t.Helper()
	require.NoError(t, err)
	return <-emails
}

// TestEmailTemplatesEscapeContributorText drives every HTML email whose body
// carries text this platform does not author, with the same hostile value in
// every field that reaches markup.
//
// Ingest writes scraped venue-calendar text into show titles and venue names
// automatically, and these messages ship from this platform's own DKIM-aligned
// sender, so an unescaped value arrives as a link the recipient has every reason
// to trust.
func TestEmailTemplatesEscapeContributorText(t *testing.T) {
	t.Run("show reminder", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendShowReminderEmail(
			"user@test.com", hostilePayload,
			"http://localhost:3000/shows/x", "http://localhost:3000/unsubscribe?uid=1&sig=abc",
			resolvedEventTime(time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)),
			[]string{hostilePayload},
		))
		assertEscaped(t, "show_reminder", email.Html)

		// The subject is a plain-text header, not markup: it carries the title
		// as written. Escaping it would print entities in the recipient's list.
		assert.Contains(t, email.Subject, hostilePayload,
			"the plain-text subject must not be HTML-escaped")
	})

	t.Run("tier promotion", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendTierPromotionEmail(
			"user@test.com", hostilePayload, "new_user", "contributor", hostilePayload,
			"http://unsub?a=1&b=2", []string{hostilePayload},
		))
		assertEscaped(t, "tier_promotion", email.Html)
	})

	t.Run("tier demotion", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendTierDemotionEmail(
			"user@test.com", hostilePayload, "contributor", "new_user", hostilePayload,
			"http://unsub?a=1&b=2",
		))
		assertEscaped(t, "tier_demotion", email.Html)
	})

	t.Run("tier demotion warning", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendTierDemotionWarningEmail(
			"user@test.com", hostilePayload, "contributor", 0.82, 0.80, "http://unsub?a=1&b=2",
		))
		assertEscaped(t, "tier_demotion_warning", email.Html)
	})

	t.Run("edit approved", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendEditApprovedEmail(
			"user@test.com", hostilePayload, "artist", hostilePayload,
			"http://localhost:3000/artists/x", "http://unsub?a=1&b=2",
		))
		assertEscaped(t, "edit_approved", email.Html)
	})

	t.Run("edit rejected", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendEditRejectedEmail(
			"user@test.com", hostilePayload, "artist", hostilePayload, hostilePayload,
			"http://unsub?a=1&b=2",
		))
		assertEscaped(t, "edit_rejected", email.Html)
	})

	t.Run("comment notification", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendCommentNotification(
			"user@test.com", hostilePayload, "artist", hostilePayload, hostilePayload,
			"http://localhost:3000/artists/x", "http://unsub?a=1&b=2",
		))
		assertEscaped(t, "comment_notification", email.Html)
	})

	t.Run("mention notification", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendMentionNotification(
			"user@test.com", hostilePayload, "release", hostilePayload, hostilePayload,
			"http://localhost:3000/releases/x#comment-1", "http://unsub?a=1&b=2",
		))
		assertEscaped(t, "mention_notification", email.Html)
	})

	t.Run("collection digest", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendCollectionDigestEmail(
			"user@test.com",
			[]contracts.CollectionDigestGroup{{
				CollectionTitle: hostilePayload,
				CollectionURL:   "http://localhost:3000/collections/x",
				Items: []contracts.CollectionDigestEntry{{
					EntityType: hostilePayload,
					EntityName: hostilePayload,
					EntityURL:  "http://localhost:3000/artists/x",
					AddedBy:    hostilePayload,
				}},
			}},
			"http://unsub?a=1&b=2",
		))
		assertEscaped(t, "collection_digest", email.Html)
	})

	t.Run("scene digest", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendSceneDigestEmail(
			"user@test.com",
			[]contracts.SceneDigestGroup{{
				SceneName: hostilePayload,
				SceneURL:  "http://localhost:3000/scenes/x",
				Shows: []contracts.SceneDigestShow{{
					DisplayTitle: hostilePayload,
					Date:         hostilePayload,
					VenueName:    hostilePayload,
					ShowURL:      "http://localhost:3000/shows/x",
				}},
				NewArtists: []contracts.SceneDigestArtist{{
					Name:      hostilePayload,
					ArtistURL: "http://localhost:3000/artists/x",
				}},
				MoreNewArtists: 2,
			}},
			"http://unsub?a=1&b=2",
		))
		assertEscaped(t, "scene_digest", email.Html)
	})

	// The saved-filter and scene-follow alerts share one body, built in this
	// package and handed to the generic transport, so the builder's output is
	// the payload.
	t.Run("filter match", func(t *testing.T) {
		body, err := buildFilterEmailHTML(
			hostilePayload, hostilePayload, hostilePayload, hostilePayload, hostilePayload,
			hostilePayload, "http://localhost:3000/shows/x", "http://unsub?a=1&b=2",
		)
		require.NoError(t, err)
		assertEscaped(t, "filter_match", body)
	})

	// The two design-system alerts render through the email_layout.go builders
	// rather than a template. Same guarantee, different mechanism, so they are
	// exercised by the same payload.
	t.Run("artist show alert", func(t *testing.T) {
		body := buildArtistShowAlertEmailHTML(
			hostilePayload, contracts.FollowAlertScopeNearMe,
			showEmailContentParts{
				date:      hostilePayload,
				venueText: hostilePayload,
				showURL:   "http://localhost:3000/shows/x",
			},
			"http://unsub?a=1&b=2", "http://localhost:3000/settings/notifications",
		)
		assertEscaped(t, "artist_show_alert", body)
	})

	t.Run("venue show alert", func(t *testing.T) {
		body := buildVenueShowAlertEmailHTML(
			&venueAlertBatch{
				key:       venueAlertGroupKey{VenueID: 1, AlertDay: "2026-08-24"},
				venueName: hostilePayload,
				venueURL:  "http://localhost:3000/venues/x",
				loc:       time.UTC,
			},
			[]venueAlertShow{{
				ID:         1,
				Title:      hostilePayload,
				ArtistText: hostilePayload,
				EventDate:  time.Date(2026, 8, 29, 20, 0, 0, 0, time.UTC),
			}},
			"http://unsub?a=1&b=2", "http://localhost:3000/settings/notifications",
		)
		assertEscaped(t, "venue_show_alert", body)
	})
}

// htmlTagInFormat matches an opening tag in a format string: `<`, a tag name,
// then the character that ends the name. It deliberately does not match
// `<https://...>`, the angle-bracket wrapper RFC 2369 puts around a header URL,
// because a colon cannot end a tag name.
var htmlTagInFormat = regexp.MustCompile(`<[a-zA-Z][a-zA-Z0-9]*[ >/]`)

// markupRenderers are the files allowed to assemble email markup by hand.
//
// email_layout.go is the design-system builder set: it owns the frame and every
// block element, and escapes each value it is handed at the point it writes it.
// Everywhere else renders through the html/template bodies in
// email_templates.go, which escape for the context an action sits in.
var markupRenderers = map[string]bool{
	"email_layout.go": true,
}

// TestNoRawHTMLSprintf keeps the two renderers the only two.
//
// Escaping the values in a template is a fix that holds; the pattern that
// produced the defect was a new sender pasting a `fmt.Sprintf` body with `%s`
// in it, which escapes nothing and looks exactly like the senders around it.
// This fails that commit instead of the next review.
func TestNoRawHTMLSprintf(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || markupRenderers[name] {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		require.NoError(t, err)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "fmt" {
				return true
			}

			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				assert.False(t, htmlTagInFormat.MatchString(value),
					"%s: markup assembled with fmt.%s. Add the body to email_templates.go "+
						"so html/template escapes it, or a builder in email_layout.go",
					fset.Position(lit.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
}
