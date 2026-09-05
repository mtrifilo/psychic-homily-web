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

// hostile builds the value a contributor could put in one named field: markup, a
// working link to somewhere else, the two characters that end an attribute, and
// the field's own name as a tail marker.
//
// The marker is what makes a body's assertions per-field. Without it, a body
// whose fields all carry the same payload passes while one of them silently
// stops rendering, because the payload still appears somewhere else in the body.
//
// The link's host is a reserved TLD, so a body that somehow shipped this could
// not reach a real destination.
func hostile(field string) string {
	return `<script>alert(1)</script> <a href="https://evil.test">click</a> "quoted" & ` + field
}

// assertEscaped fails if any part of a hostile value survived as markup, or if
// any named field is missing from the body.
//
// The assertions name characters rather than entities: what has to hold is that
// no interpolated value can open a tag, end an attribute, or become a link, and
// which entity a renderer spells that with is its own business.
func assertEscaped(t *testing.T, name string, body string, fields ...string) {
	t.Helper()
	require.NotEmpty(t, fields, "%s: name the fields carrying a hostile value", name)

	assert.NotContains(t, body, "<script>", "%s: a raw tag reached the body", name)
	assert.NotContains(t, body, "</script>", "%s: a raw closing tag reached the body", name)
	assert.NotContains(t, body, `<a href="https://evil.test"`,
		"%s: contributor text became a working link from this platform's sender", name)
	assert.NotContains(t, body, `"quoted"`, "%s: a bare quote can end an attribute", name)
	assert.Contains(t, body, "&lt;script&gt;", "%s: the payload must survive as readable text", name)

	for _, field := range fields {
		assert.Contains(t, body, "&amp; "+field,
			"%s: %s is missing from the body, or its ampersand is not escaped", name, field)
		assert.NotContains(t, body, "& "+field,
			"%s: %s reached the body with a bare ampersand", name, field)
	}

	// html/template writes this in place of a URL whose scheme it will not
	// vouch for, and returns no error while doing it, so a dead link renders
	// green unless something looks for the marker.
	assert.NotContains(t, body, "ZgotmplZ", "%s: an href was neutered by the URL filter", name)

	// The frame is a shared {{define}} block. These counts catch a body that
	// renders it twice or omits it; a misspelled block name is an execute error
	// the caller surfaces instead.
	assert.Equal(t, 1, strings.Count(body, "<html"), "%s: one opening html", name)
	assert.Equal(t, 1, strings.Count(body, "</html>"), "%s: one closing html", name)
	assert.Equal(t, 1, strings.Count(body, "PSYCHIC HOMILY")+strings.Count(body, ">Psychic Homily</h1>"),
		"%s: one masthead", name)
}

// captureBody sends through the resend harness and returns the one captured
// message, so each case exercises the real send path rather than a builder the
// sender might not use.
func captureBody(t *testing.T, emails chan capturedEmail, err error) capturedEmail {
	t.Helper()
	require.NoError(t, err)
	return <-emails
}

// TestEveryTemplateRenders executes each body once against its data type.
//
// html/template defers its contextual-escaping analysis to the first Execute, so
// this is where a template that cannot be escaped safely, or that disagrees with
// its data struct, is caught. Without it those failures reach a send path, where
// they are reported and the message is dropped.
func TestEveryTemplateRenders(t *testing.T) {
	for _, entry := range allEmailTemplates {
		t.Run(entry.tmpl.Name(), func(t *testing.T) {
			body, err := renderEmailTemplate(entry.tmpl, entry.data)
			require.NoError(t, err)
			assert.Contains(t, body, "</html>", "a body must render its frame")
		})
	}
}

// TestEmailTemplatesEscapeContributorText drives every HTML email in this
// package with a distinct hostile value in every field that reaches markup.
//
// Every send path is represented: the thirteen html/template bodies and the
// three built from the email_layout.go builders.
func TestEmailTemplatesEscapeContributorText(t *testing.T) {
	t.Run("verification", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendVerificationEmail("user@test.com", hostile("token")))
		assertEscaped(t, "verification", email.Html, "token")
	})

	t.Run("magic link", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendMagicLinkEmail("user@test.com", hostile("token")))
		assertEscaped(t, "magic_link", email.Html, "token")
	})

	t.Run("account recovery", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendAccountRecoveryEmail("user@test.com", hostile("token"), 14))
		assertEscaped(t, "account_recovery", email.Html, "token")
	})

	t.Run("show reminder", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		title := hostile("showTitle")
		email := captureBody(t, emails, svc.SendShowReminderEmail(
			"user@test.com", title,
			"http://localhost:3000/shows/x", "http://localhost:3000/unsubscribe?uid=1&sig=abc",
			resolvedEventTime(time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)),
			[]string{hostile("venue")},
		))
		assertEscaped(t, "show_reminder", email.Html, "showTitle", "venue")

		// The subject is a plain-text header, not markup: it carries the title
		// as written. Escaping it would print entities in the recipient's list.
		assert.Contains(t, email.Subject, title, "the plain-text subject must not be HTML-escaped")
	})

	t.Run("tier promotion", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendTierPromotionEmail(
			"user@test.com", hostile("username"), "new_user", "contributor", hostile("reason"),
			"http://unsub?a=1&b=2", []string{hostile("permission")},
		))
		assertEscaped(t, "tier_promotion", email.Html, "username", "reason", "permission")
	})

	t.Run("tier demotion", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendTierDemotionEmail(
			"user@test.com", hostile("username"), "contributor", "new_user", hostile("reason"),
			"http://unsub?a=1&b=2",
		))
		assertEscaped(t, "tier_demotion", email.Html, "username", "reason")
	})

	t.Run("tier demotion warning", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendTierDemotionWarningEmail(
			"user@test.com", hostile("username"), "contributor", 0.82, 0.80, "http://unsub?a=1&b=2",
		))
		assertEscaped(t, "tier_demotion_warning", email.Html, "username")
	})

	t.Run("edit approved", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendEditApprovedEmail(
			"user@test.com", hostile("username"), "artist", hostile("entityName"),
			"http://localhost:3000/artists/x", "http://unsub?a=1&b=2",
		))
		assertEscaped(t, "edit_approved", email.Html, "username", "entityName")
	})

	t.Run("edit rejected", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendEditRejectedEmail(
			"user@test.com", hostile("username"), "artist", hostile("entityName"), hostile("reason"),
			"http://unsub?a=1&b=2",
		))
		assertEscaped(t, "edit_rejected", email.Html, "username", "entityName", "reason")
	})

	t.Run("comment notification", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		name := hostile("entityName")
		email := captureBody(t, emails, svc.SendCommentNotification(
			"user@test.com", hostile("commenter"), "artist", name, hostile("excerpt"),
			"http://localhost:3000/artists/x", "http://unsub?a=1&b=2",
		))
		assertEscaped(t, "comment_notification", email.Html, "commenter", "entityName", "excerpt")
		assert.Contains(t, email.Subject, name, "the plain-text subject must not be HTML-escaped")
	})

	t.Run("mention notification", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		name := hostile("entityName")
		email := captureBody(t, emails, svc.SendMentionNotification(
			"user@test.com", hostile("mentioner"), "release", name, hostile("excerpt"),
			"http://localhost:3000/releases/x#comment-1", "http://unsub?a=1&b=2",
		))
		assertEscaped(t, "mention_notification", email.Html, "mentioner", "entityName", "excerpt")
		assert.Contains(t, email.Subject, name, "the plain-text subject must not be HTML-escaped")
	})

	t.Run("collection digest", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendCollectionDigestEmail(
			"user@test.com",
			[]contracts.CollectionDigestGroup{{
				CollectionTitle: hostile("collectionTitle"),
				CollectionURL:   "http://localhost:3000/collections/x",
				Items: []contracts.CollectionDigestEntry{{
					EntityType: hostile("entityType"),
					EntityName: hostile("entityName"),
					EntityURL:  "http://localhost:3000/artists/x",
					AddedBy:    hostile("addedBy"),
				}},
			}},
			"http://unsub?a=1&b=2",
		))
		assertEscaped(t, "collection_digest", email.Html,
			"collectionTitle", "entityType", "entityName", "addedBy")
	})

	t.Run("scene digest", func(t *testing.T) {
		svc, emails, _ := setupEmailTest(t)
		email := captureBody(t, emails, svc.SendSceneDigestEmail(
			"user@test.com",
			[]contracts.SceneDigestGroup{{
				SceneName: hostile("sceneName"),
				SceneURL:  "http://localhost:3000/scenes/x",
				Shows: []contracts.SceneDigestShow{{
					DisplayTitle: hostile("displayTitle"),
					Date:         hostile("date"),
					VenueName:    hostile("venueName"),
					ShowURL:      "http://localhost:3000/shows/x",
				}},
				NewArtists: []contracts.SceneDigestArtist{{
					Name:      hostile("artistName"),
					ArtistURL: "http://localhost:3000/artists/x",
				}},
				MoreNewArtists: 2,
			}},
			"http://unsub?a=1&b=2",
		))
		assertEscaped(t, "scene_digest", email.Html,
			"sceneName", "displayTitle", "date", "venueName", "artistName")
	})

	// The saved-filter and scene-follow alerts share one body, built in this
	// package and handed to the generic transport, so the builder's output is
	// the payload.
	t.Run("filter match", func(t *testing.T) {
		body, err := buildFilterEmailHTML(
			hostile("filterName"), hostile("showTitle"), hostile("showDate"), hostile("venueText"),
			hostile("artistText"), hostile("priceText"),
			"http://localhost:3000/shows/x", "http://unsub?a=1&b=2",
		)
		require.NoError(t, err)
		assertEscaped(t, "filter_match", body,
			"filterName", "showTitle", "showDate", "venueText", "artistText", "priceText")
	})

	// The two show alerts render through the email_layout.go builders rather
	// than a template. Those builders escape every value they interpolate, which
	// is the property this payload tests. Unlike html/template they do not also
	// filter an href's URL scheme, which costs nothing while every href they are
	// given is built from the configured frontend URL.
	t.Run("artist show alert", func(t *testing.T) {
		body := buildArtistShowAlertEmailHTML(
			hostile("artistName"), contracts.FollowAlertScopeNearMe,
			showEmailContentParts{
				date:       hostile("date"),
				venueText:  hostile("venueText"),
				artistText: hostile("artistText"),
				priceText:  hostile("priceText"),
				showURL:    "http://localhost:3000/shows/x",
			},
			"http://unsub?a=1&b=2", "http://localhost:3000/settings/notifications",
		)
		assertEscaped(t, "artist_show_alert", body,
			"artistName", "date", "venueText", "artistText", "priceText")
	})

	t.Run("venue show alert", func(t *testing.T) {
		body := buildVenueShowAlertEmailHTML(
			&venueAlertBatch{
				key:       venueAlertGroupKey{VenueID: 1, AlertDay: "2026-08-24"},
				venueName: hostile("venueName"),
				venueURL:  "http://localhost:3000/venues/x",
				loc:       time.UTC,
			},
			[]venueAlertShow{{
				ID:         1,
				Title:      hostile("showTitle"),
				ArtistText: hostile("artistText"),
				EventDate:  time.Date(2026, 8, 29, 20, 0, 0, 0, time.UTC),
			}},
			"http://unsub?a=1&b=2", "http://localhost:3000/settings/notifications",
		)
		assertEscaped(t, "venue_show_alert", body, "venueName", "showTitle", "artistText")
	})
}

// TestTierPromotionNextTierBranches pins the three next-step paragraphs, which
// are an {{if}}/{{else if}} chain on a tier constant and are otherwise the one
// part of these bodies no assertion reaches.
func TestTierPromotionNextTierBranches(t *testing.T) {
	cases := map[string]string{
		"contributor":         "reach <strong>Trusted Contributor</strong> status",
		"trusted_contributor": "reach <strong>Local Ambassador</strong> status",
		"local_ambassador":    "highest contributor tier",
	}
	for tier, sentence := range cases {
		t.Run(tier, func(t *testing.T) {
			svc, emails, _ := setupEmailTest(t)
			email := captureBody(t, emails, svc.SendTierPromotionEmail(
				"user@test.com", "alice", "new_user", tier, "reason", "http://unsub", nil))
			assert.Contains(t, email.Html, sentence)
			for otherTier, other := range cases {
				if otherTier != tier {
					assert.NotContains(t, email.Html, other, "only the reached tier's next step")
				}
			}
		})
	}
}

// htmlTagLiteral matches an opening or closing tag. The class after a tag name
// includes whitespace because this codebase's markup wraps lines right after
// one. It deliberately does not match `<https://...>`, the angle-bracket wrapper
// RFC 2369 puts around a header URL, because a colon cannot end a tag name.
var htmlTagLiteral = regexp.MustCompile(`<[a-zA-Z][a-zA-Z0-9]*[\s>/]|</[a-zA-Z]`)

// safeMarkupTypes are the html/template types that declare a value already safe
// and skip escaping. A conversion to one of them is the single in-package way to
// defeat contextual escaping, so the guard rejects them outright rather than
// leaving them to review.
var safeMarkupTypes = map[string]bool{
	"HTML": true, "HTMLAttr": true, "URL": true, "JS": true, "JSStr": true,
	"CSS": true, "Srcset": true,
}

// markupRenderers are the files allowed to hold email markup.
//
// email_layout.go is the design-system builder set. Its leaf builders
// (emailHeadline, emailParagraph, emailButton, emailMonoNote, emailMonoDetails,
// emailListRows, emailFineprint, emailFineprintWithLinks) escape the values they
// are handed; emailShell, emailFineprintLine and emailFineprintRow take
// already-built markup and escape nothing, so they take builder output and never
// a value. A builder added there inherits that obligation by hand, which is why
// the file is a listed exception rather than a place to put markup that has no
// escaping at all.
//
// email_templates.go is html/template, where escaping belongs to the renderer.
var markupRenderers = map[string]bool{
	"email_layout.go":    true,
	"email_templates.go": true,
}

// TestNoEmailMarkupOutsideRenderers keeps those two files the only two that hold
// markup, and keeps the escaping opt-out types out of the package entirely.
//
// It checks string literals rather than fmt.Sprintf calls specifically, because
// the shapes that reintroduce the defect are not all fmt calls: a package-level
// format const, a `"<p>" + name + "</p>"` concatenation, and a
// strings.Builder.WriteString all escape nothing and all read like the code
// around them.
//
// Scope is this package. A sender written in a sibling package is not covered,
// and would need its own guard or a move in here.
func TestNoEmailMarkupOutsideRenderers(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		// Mode 0 leaves comments out of the AST, so prose about markup in a doc
		// comment is not a finding.
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		require.NoError(t, err)

		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				pkg, isIdent := sel.X.(*ast.Ident)
				if isIdent && pkg.Name == "template" && safeMarkupTypes[sel.Sel.Name] {
					assert.Fail(t, "escaping opt-out",
						"%s: template.%s declares a value pre-escaped and skips the renderer. "+
							"Pass it as a plain string and let the template escape it",
						fset.Position(sel.Pos()), sel.Sel.Name)
				}
				return true
			}
			if markupRenderers[name] {
				return true
			}
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			assert.False(t, htmlTagLiteral.MatchString(value),
				"%s: email markup outside the two renderers. Put the body in "+
					"email_templates.go so html/template escapes it, or add a builder "+
					"to email_layout.go that escapes every value it interpolates",
				fset.Position(lit.Pos()))
			return true
		})
	}
}
