package notification

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
)

// Artist new-show alert delivery (PSY-1896). Integration cases run inside
// NotificationFilterSuite (real Postgres, all migrations) so the partial UNIQUE
// that carries the exactly-once guarantee is actually exercised; a mocked DB
// would assert the Go and skip the constraint doing the work.

// Two real CBSA codes, so the near-me comparison in the tests is the same shape
// as production: a code from the offline Census dataset, not a made-up token.
const (
	metroPhoenix = "38060"
	metroChicago = "16980"
)

// =============================================================================
// UNIT TESTS (no database)
// =============================================================================

// TestShowAreaMetrosMatches pins decision 10 — the fail-open rule — as a unit
// test, because it decides whether a user hears about a show at all and it
// should be checkable without standing up Postgres.
func TestShowAreaMetrosMatches(t *testing.T) {
	area := func(unresolved bool, codes ...string) showAreaMetros {
		a := showAreaMetros{codes: map[string]struct{}{}, unresolved: unresolved}
		for _, c := range codes {
			a.codes[c] = struct{}{}
		}
		return a
	}

	cases := []struct {
		name string
		area showAreaMetros
		home string
		want bool
	}{
		{"venue in the home metro", area(false, metroPhoenix), metroPhoenix, true},
		{"venue in another metro", area(false, metroChicago), metroPhoenix, false},
		{
			"one of several venues in the home metro",
			area(false, metroChicago, metroPhoenix), metroPhoenix, true,
		},
		{
			// Decision 10. An ungeocoded venue is a gap in our own derived data,
			// and a silently dropped alert looks to the user like the feature is
			// broken.
			"unresolvable venue delivers anyway",
			area(true), metroPhoenix, true,
		},
		{
			// Fail-open is applied per VENUE: a resolved sibling does not make the
			// unresolved one safe to exclude on.
			"partly resolved show delivers anyway",
			area(true, metroChicago), metroPhoenix, true,
		},
		{
			// A near-me subscriber with no home area never reaches this function
			// (EffectiveShowScope degrades them to everywhere first), but if one
			// did, an empty home must not silently match a resolved show.
			"empty home area does not match a resolved show",
			area(false, metroPhoenix), "", false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.area.matches(tc.home))
		})
	}
}

// TestResolveArtistAlertRecipients covers the collapse of a user's several
// follows on one bill into the single alert they receive.
func TestResolveArtistAlertRecipients(t *testing.T) {
	settings := func(doc string) *json.RawMessage {
		raw := json.RawMessage(doc)
		return &raw
	}
	openArea := showAreaMetros{codes: map[string]struct{}{}, unresolved: true}
	phoenixArea := showAreaMetros{codes: map[string]struct{}{metroPhoenix: {}}}
	home := metroPhoenix

	t.Run("an inheriting follow gets in-app on and email off", func(t *testing.T) {
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{{UserID: 1, ArtistID: 10, ArtistName: "Oneida", Position: 0}},
			nil, openArea, 0,
		)
		if assert.Len(t, out, 1) {
			assert.True(t, out[0].inApp)
			assert.False(t, out[0].email, "email is an intentional opt-in on every alert type")
			assert.Equal(t, contracts.FollowAlertScopeEverywhere, out[0].scope,
				"near me degrades to everywhere without a home area")
		}
	})

	t.Run("enabled:false silences the follow on every channel", func(t *testing.T) {
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{{
				UserID: 1, ArtistID: 10, ArtistName: "Oneida",
				Settings: settings(`{"alerts":{"shows":{"enabled":false}}}`),
			}},
			nil, openArea, 0,
		)
		assert.Empty(t, out)
	})

	t.Run("both channels off produces no alert at all", func(t *testing.T) {
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{{
				UserID: 1, ArtistID: 10, ArtistName: "Oneida",
				Settings: settings(`{"alerts":{"shows":{"in_app":false,"email":false}}}`),
			}},
			nil, openArea, 0,
		)
		assert.Empty(t, out, "a row with no lane to write would be a dedup record for a notification nobody got")
	})

	t.Run("the submitter is not alerted about their own show", func(t *testing.T) {
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{{UserID: 7, ArtistID: 10, ArtistName: "Oneida"}},
			nil, openArea, 7,
		)
		assert.Empty(t, out)
	})

	t.Run("an email-enabled follow outranks a higher billing", func(t *testing.T) {
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{
				{UserID: 1, ArtistID: 10, ArtistName: "Headliner", Position: 0},
				{
					UserID: 1, ArtistID: 20, ArtistName: "Opener", Position: 1,
					Settings: settings(`{"alerts":{"shows":{"email":true}}}`),
				},
			},
			nil, openArea, 0,
		)
		if assert.Len(t, out, 1, "one user gets one alert however many of the bill they follow") {
			assert.Equal(t, "Opener", out[0].artistName,
				"dropping the email follow would discard an opt-in the user made")
			assert.True(t, out[0].email)
		}
	})

	t.Run("otherwise the follow highest on the bill wins", func(t *testing.T) {
		// Deliberately supplied opener-first: the query has no ORDER BY, so the
		// tie-break has to come from the sort inside the resolver.
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{
				{UserID: 1, ArtistID: 20, ArtistName: "Opener", Position: 1},
				{UserID: 1, ArtistID: 10, ArtistName: "Headliner", Position: 0},
			},
			nil, openArea, 0,
		)
		if assert.Len(t, out, 1) {
			assert.Equal(t, "Headliner", out[0].artistName)
		}
	})

	t.Run("a near-me follow is dropped for an out-of-area show", func(t *testing.T) {
		chicago := showAreaMetros{codes: map[string]struct{}{metroChicago: {}}}
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{{UserID: 1, ArtistID: 10, ArtistName: "Oneida"}},
			map[uint]recipientAlertPrefs{1: {UserID: 1, HomeMetro: &home}},
			chicago, 0,
		)
		assert.Empty(t, out)
	})

	t.Run("a near-me follow is kept for a home-area show", func(t *testing.T) {
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{{UserID: 1, ArtistID: 10, ArtistName: "Oneida"}},
			map[uint]recipientAlertPrefs{1: {UserID: 1, HomeMetro: &home}},
			phoenixArea, 0,
		)
		if assert.Len(t, out, 1) {
			assert.Equal(t, contracts.FollowAlertScopeNearMe, out[0].scope)
		}
	})

	t.Run("an out-of-area follow does not veto an everywhere follow on the same bill", func(t *testing.T) {
		chicago := showAreaMetros{codes: map[string]struct{}{metroChicago: {}}}
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{
				{UserID: 1, ArtistID: 10, ArtistName: "NearMeOnly", Position: 0},
				{
					UserID: 1, ArtistID: 20, ArtistName: "Everywhere", Position: 1,
					Settings: settings(`{"alerts":{"shows":{"scope":"everywhere"}}}`),
				},
			},
			map[uint]recipientAlertPrefs{1: {UserID: 1, HomeMetro: &home}},
			chicago, 0,
		)
		if assert.Len(t, out, 1) {
			assert.Equal(t, "Everywhere", out[0].artistName)
		}
	})

	t.Run("the account matrix reaches a follow that overrode nothing", func(t *testing.T) {
		defaults := json.RawMessage(`{"shows":{"email":true}}`)
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{{UserID: 1, ArtistID: 10, ArtistName: "Oneida"}},
			map[uint]recipientAlertPrefs{1: {UserID: 1, AlertDefaults: &defaults}},
			openArea, 0,
		)
		if assert.Len(t, out, 1) {
			assert.True(t, out[0].email, "an account-level opt-in must reach follows that stored nothing")
		}
	})

	t.Run("a per-follow override beats the account matrix", func(t *testing.T) {
		defaults := json.RawMessage(`{"shows":{"email":true}}`)
		out := resolveArtistAlertRecipients(
			[]artistFollowerRow{{
				UserID: 1, ArtistID: 10, ArtistName: "Oneida",
				Settings: settings(`{"alerts":{"shows":{"email":false}}}`),
			}},
			map[uint]recipientAlertPrefs{1: {UserID: 1, AlertDefaults: &defaults}},
			openArea, 0,
		)
		if assert.Len(t, out, 1) {
			assert.False(t, out[0].email)
		}
	})
}

// TestBuildArtistShowAlertEmailHTML pins the parts of the alert email that are
// contractual rather than cosmetic: the shared layout, a working opt-out, the
// scope sentence, and escaping of scraped third-party text.
func TestBuildArtistShowAlertEmailHTML(t *testing.T) {
	content := showEmailContentParts{
		date:       "Saturday, August 29, 2026",
		venueText:  "Valley Bar",
		artistText: "Oneida, Din of Celestial Birds",
		priceText:  "$18",
		showURL:    "https://psychichomily.com/shows/oneida-valley-bar",
	}
	unsubURL := "https://api.psychichomily.com/unsubscribe/artist-show-alerts?uid=1&sig=abc"
	manageURL := "https://psychichomily.com/settings/notifications"

	nearMe := buildArtistShowAlertEmailHTML("Oneida", contracts.FollowAlertScopeNearMe, content, unsubURL, manageURL)

	assert.Contains(t, nearMe, "PSYCHIC HOMILY", "must render in the shared direction-A frame")
	assert.Contains(t, nearMe, "ARTIST ALERT", "the kicker names the alert type")
	assert.Contains(t, nearMe, "Oneida announced a show.")
	assert.Contains(t, nearMe, "WHEN .... Saturday, August 29, 2026")
	assert.Contains(t, nearMe, "WHERE ... Valley Bar")
	assert.Contains(t, nearMe, "set to your home area")
	// The href is attribute-escaped, so `&` between the query parameters arrives
	// as `&amp;`. That is the correct spelling in HTML and resolves to the same
	// URL; the assertion is written against it rather than against the raw string
	// so a future change that stopped escaping would fail here.
	assert.Contains(t, nearMe, `href="`+htmlEscape(unsubURL)+`"`,
		"the opt-out must be a real anchor, not escaped text: RFC 8058 needs the body link to work")
	assert.Contains(t, nearMe, "Unsubscribe from artist show alerts")
	assert.Contains(t, nearMe, manageURL)
	assert.Contains(t, nearMe, content.showURL)

	everywhere := buildArtistShowAlertEmailHTML("Oneida", contracts.FollowAlertScopeEverywhere, content, unsubURL, manageURL)
	assert.Contains(t, everywhere, "cover every city")
	assert.NotContains(t, everywhere, "set to your home area",
		"the scope sentence has to tell the truth about which scope produced the email")

	// Ingest writes scraped venue-calendar text straight into these fields, and
	// this message ships from the platform's own DKIM-aligned sender.
	hostile := showEmailContentParts{date: "today", venueText: `</td><a href="https://evil.example">Verify</a>`}
	out := buildArtistShowAlertEmailHTML(`<script>alert(1)</script>`, contracts.FollowAlertScopeNearMe, hostile, unsubURL, manageURL)
	assert.NotContains(t, out, "<script>")
	assert.NotContains(t, out, `<a href="https://evil.example">`)
	assert.Contains(t, out, "&lt;script&gt;")
}

// TestInboxVisibleRows pins the ONE row shape the bell hides, and pins it
// negatively too: the predicate must not become the `channel = 'in_app'` read
// that would empty every existing user's inbox.
func TestInboxVisibleRows(t *testing.T) {
	pred := inboxVisibleRows("nl")
	assert.Contains(t, pred, "nl.entity_type = 'artist_show_alert'")
	assert.Contains(t, pred, "nl.channel = 'email'")
	assert.True(t, len(pred) > 0 && pred[:4] == "NOT ",
		"it must EXCLUDE one shape, never restrict the read to one channel")
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

// capturingEmailService records the alert emails a test provokes, which the
// suite-wide mockEmailService (a bare counter) cannot do.
type capturingEmailService struct {
	mockEmailService
	sent []capturedAlertEmail
}

type capturedAlertEmail struct {
	to, subject, html, unsubscribeURL string
}

func (m *capturingEmailService) SendFilterNotificationEmail(to, subject, html, unsubscribeURL string) error {
	m.sent = append(m.sent, capturedAlertEmail{to, subject, html, unsubscribeURL})
	return nil
}

// withCapturedEmail swaps in a capturing email service for one test and hands
// back the capture buffer. The suite's shared service is restored afterwards so
// tests stay order-independent.
func (s *NotificationFilterSuite) withCapturedEmail() *capturingEmailService {
	capture := &capturingEmailService{}
	previous := s.svc.emailService
	s.svc.emailService = capture
	s.T().Cleanup(func() { s.svc.emailService = previous })
	return capture
}

// followArtistWithAlerts creates an artist follow carrying an explicit alerts
// document, which is what a user who has touched the Library control has.
func (s *NotificationFilterSuite) followArtistWithAlerts(userID, artistID uint, alertsJSON string) {
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at, settings)
		VALUES (?, 'artist', ?, 'follow', now(), ?::jsonb)`, userID, artistID, alertsJSON).Error)
}

func (s *NotificationFilterSuite) setHomeMetro(userID uint, metro string) {
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_preferences (user_id, home_metro)
		VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET home_metro = EXCLUDED.home_metro`, userID, metro).Error)
}

// createTestVenueInMetro mirrors createTestVenue but stamps the derived CBSA
// code the near-me rule actually compares. City/state are set to match so the
// fixture is not internally contradictory.
func (s *NotificationFilterSuite) createTestVenueInMetro(name, city, state, metro string) uint {
	slug := name
	m := metro
	venue := catalogm.Venue{Name: name, Slug: &slug, City: city, State: state}
	if metro != "" {
		venue.Metro = &m
	}
	s.Require().NoError(s.db.Create(&venue).Error)
	return venue.ID
}

// artistAlertRows counts this user's artist-alert rows for a show, per lane.
func (s *NotificationFilterSuite) artistAlertRows(userID, showID uint, channel string) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND channel = ?",
			userID, notificationm.NotificationEntityArtistShowAlert, showID, channel).
		Count(&n).Error)
	return n
}

func (s *NotificationFilterSuite) inAppAlerts(userID, showID uint) int64 {
	return s.artistAlertRows(userID, showID, notificationm.NotificationChannelInApp)
}

// TestArtistAlert_EverywhereScopeNotifiesAnyShow is AC 1: with the scope set to
// everywhere, ANY new visible show for the artist alerts the follower.
func (s *NotificationFilterSuite) TestArtistAlert_EverywhereScopeNotifiesAnyShow() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"scope":"everywhere"}}}`)
	// A home area is set, and deliberately NOT the show's: everywhere has to
	// ignore it rather than accidentally passing because nothing was configured.
	s.setHomeMetro(userID, metroPhoenix)

	venueID := s.createTestVenueInMetro("Empty Bottle", "Chicago", "IL", metroChicago)
	showID := s.createTestShow("Oneida at Empty Bottle", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID))
}

// TestArtistAlert_NearMeScopeMatchesHomeArea is the positive half of AC 2.
func (s *NotificationFilterSuite) TestArtistAlert_NearMeScopeMatchesHomeArea() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"scope":"near_me"}}}`)
	s.setHomeMetro(userID, metroPhoenix)

	// Mesa, not Phoenix. The show is in a SUBURB of the user's metro, which is
	// exactly the case city equality drops and CBSA matching keeps.
	venueID := s.createTestVenueInMetro("Nile Theater", "Mesa", "AZ", metroPhoenix)
	showID := s.createTestShow("Oneida at Nile", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID))
}

// TestArtistAlert_NearMeScopeSuppressesOutOfArea is the negative half of AC 2.
func (s *NotificationFilterSuite) TestArtistAlert_NearMeScopeSuppressesOutOfArea() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"scope":"near_me"}}}`)
	s.setHomeMetro(userID, metroPhoenix)

	venueID := s.createTestVenueInMetro("Empty Bottle", "Chicago", "IL", metroChicago)
	showID := s.createTestShow("Oneida at Empty Bottle", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(0), s.inAppAlerts(userID, showID))
}

// TestArtistAlert_NearMeFailsOpenOnUnresolvableVenue is decision 10 end to end.
func (s *NotificationFilterSuite) TestArtistAlert_NearMeFailsOpenOnUnresolvableVenue() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"scope":"near_me"}}}`)
	s.setHomeMetro(userID, metroPhoenix)

	// No metro: a real state for a venue our geocoder could not resolve.
	venueID := s.createTestVenueInMetro("A House Show", "Somewhere", "XX", "")
	showID := s.createTestShow("Oneida somewhere", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID),
		"an unresolvable venue metro must deliver, not silently drop")
}

// TestArtistAlert_NearMeWithoutHomeAreaDelivers pins the EffectiveShowScope
// fallback: near me is the DEFAULT, so a user who never set a home area would
// otherwise be subscribed to nothing at all.
func (s *NotificationFilterSuite) TestArtistAlert_NearMeWithoutHomeAreaDelivers() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID) // no settings at all: pure inherit

	venueID := s.createTestVenueInMetro("Empty Bottle", "Chicago", "IL", metroChicago)
	showID := s.createTestShow("Oneida at Empty Bottle", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID))
}

// TestArtistAlert_ScopeChangeTakesEffectForSubsequentShows is AC 3, driven
// through the SHIPPED API rather than by writing settings JSON directly, so the
// test covers the seam a user's scope choice actually travels.
func (s *NotificationFilterSuite) TestArtistAlert_ScopeChangeTakesEffectForSubsequentShows() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)
	s.setHomeMetro(userID, metroPhoenix)

	follows := engagement.NewFollowService(s.db)
	nearMe := contracts.FollowAlertScopeNearMe
	everywhere := contracts.FollowAlertScopeEverywhere

	// Start scoped to the home area: an out-of-area show must not alert.
	_, err := follows.SetFollowAlertSettings(userID, "artist", artistID, contracts.FollowAlertUpdate{
		Shows: &contracts.FollowAlertPreferenceUpdate{Scope: &nearMe},
	})
	s.Require().NoError(err)

	chicagoVenue := s.createTestVenueInMetro("Empty Bottle", "Chicago", "IL", metroChicago)
	firstShow := s.createTestShow("Oneida in Chicago", []uint{artistID}, []uint{chicagoVenue})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(firstShow)))
	s.Equal(int64(0), s.inAppAlerts(userID, firstShow))

	// Widen the scope. The change must reach the NEXT show, and must not
	// retroactively manufacture an alert for the one already decided.
	_, err = follows.SetFollowAlertSettings(userID, "artist", artistID, contracts.FollowAlertUpdate{
		Shows: &contracts.FollowAlertPreferenceUpdate{Scope: &everywhere},
	})
	s.Require().NoError(err)

	secondShow := s.createTestShow("Oneida in Chicago again", []uint{artistID}, []uint{chicagoVenue})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(secondShow)))
	s.Equal(int64(1), s.inAppAlerts(userID, secondShow))
	s.Equal(int64(0), s.inAppAlerts(userID, firstShow),
		"widening the scope must not reopen a show that already went by")
}

// TestArtistAlert_EmailIsOffUntilOptedIn is the email half of the AC: a follow
// nobody configured gets the in-app row and no mail.
func (s *NotificationFilterSuite) TestArtistAlert_EmailIsOffUntilOptedIn() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID))
	s.Equal(int64(0), s.artistAlertRows(userID, showID, notificationm.NotificationChannelEmail))
	s.Empty(capture.sent, "email is an intentional opt-in on every alert type")
}

// TestArtistAlert_EmailOptInSendsWithWorkingUnsubscribe covers the opted-in
// path and the RFC 8058 requirement that the same URL be both the header value
// and a working link in the body.
func (s *NotificationFilterSuite) TestArtistAlert_EmailOptInSendsWithWorkingUnsubscribe() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"email":true}}}`)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.inAppAlerts(userID, showID))
	s.Equal(int64(1), s.artistAlertRows(userID, showID, notificationm.NotificationChannelEmail),
		"the email lane needs its own durable row: it is what the daily email budget counts")
	s.Require().Len(capture.sent, 1)
	sent := capture.sent[0]
	s.Contains(sent.subject, "Oneida announced a show")
	s.Contains(sent.unsubscribeURL, "/unsubscribe/"+engagement.UnsubscribeScopeArtistShowAlerts)
	// Same endpoint in the header (raw, per RFC 2369) and in the body (HTML
	// attribute-escaped). A recipient and a mailbox provider get one way out.
	s.Contains(sent.html, `href="`+htmlEscape(sent.unsubscribeURL)+`"`,
		"the header URL and the in-body link must be the same endpoint")
}

// TestArtistAlert_ReRunDoesNotRenotify is the exactly-once guarantee. The outbox
// can legitimately re-process a job (a reclaimed `processing` row), so this has
// to hold at the database rather than by caller discipline.
func (s *NotificationFilterSuite) TestArtistAlert_ReRunDoesNotRenotify() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"email":true}}}`)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.inAppAlerts(userID, showID))
	s.Equal(int64(1), s.artistAlertRows(userID, showID, notificationm.NotificationChannelEmail))
	s.Len(capture.sent, 1, "a re-processed outbox job must not send a second email")
}

// TestArtistAlert_EmailOnlyRowIsNotABellEntry: a user with in-app off gets the
// mail and nothing in the inbox, including nothing in the unread count.
func (s *NotificationFilterSuite) TestArtistAlert_EmailOnlyRowIsNotABellEntry() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"in_app":false,"email":true}}}`)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Require().Len(capture.sent, 1)
	s.Equal(int64(0), s.inAppAlerts(userID, showID))

	entries, err := s.svc.GetUserNotifications(userID, 20, 0)
	s.Require().NoError(err)
	s.Empty(entries, "the email-lane row stands for a message already in their mailbox")

	unread, err := s.svc.GetUnreadCount(userID)
	s.Require().NoError(err)
	s.Equal(int64(0), unread, "a hidden row must not inflate the badge")
}

// TestArtistAlert_InboxRowNamesTheArtistAndLinksTheShow covers the read-path
// enrichment the new discriminator needs.
func (s *NotificationFilterSuite) TestArtistAlert_InboxRowNamesTheArtistAndLinksTheShow() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	entries, err := s.svc.GetUserNotifications(userID, 20, 0)
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]
	s.Equal(notificationm.NotificationEntityArtistShowAlert, e.EntityType)
	s.Equal(showID, e.EntityID)
	s.Require().NotNil(e.SubjectEntityID)
	s.Equal(artistID, *e.SubjectEntityID)
	s.Equal("Oneida", e.AlertArtistName)
	s.Equal("Oneida at Valley Bar", e.AlertShowTitle)
	s.Contains(e.AlertShowURL, "/shows/Oneida at Valley Bar")
	s.Empty(e.FilterName,
		"an artist alert must not inherit the scene label the show branch synthesizes for filter_id NULL rows")

	unread, err := s.svc.GetUnreadCount(userID)
	s.Require().NoError(err)
	s.Equal(int64(1), unread)
}

// TestArtistAlert_SuppressesTheSceneFollowForTheSameShow pins the one
// notification per (user, show) semantic across the two follow systems, and
// pins WHICH of them wins: the specific one.
func (s *NotificationFilterSuite) TestArtistAlert_SuppressesTheSceneFollowForTheSameShow() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.inAppAlerts(userID, showID))
	s.Equal(int64(0), s.sceneLogCount(userID, showID),
		"a band you follow is a better use of the one slot than something is on in your city")

	entries, err := s.svc.GetUserNotifications(userID, 20, 0)
	s.Require().NoError(err)
	s.Len(entries, 1)
}

// TestArtistAlert_YieldsToAnExistingFilterMatch is the other half of the
// cross-system rule: an explicit filter the user wrote outranks an implicit
// follow.
func (s *NotificationFilterSuite) TestArtistAlert_YieldsToAnExistingFilterMatch() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)

	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:        "Oneida anywhere",
		ArtistIDs:   []int64{int64(artistID)},
		NotifyInApp: true,
	})
	s.Require().NoError(err)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.sceneLogCount(userID, showID), "the filter row is the one notification")
	s.Equal(int64(0), s.inAppAlerts(userID, showID))
}

// TestArtistAlert_RefusesAShowThatIsNotPubliclyVisible is the visibility fence.
// Neither caller can reach this today; the guard exists because the cost of
// being wrong is mailing strangers about a private or rejected show.
func (s *NotificationFilterSuite) TestArtistAlert_RefusesAShowThatIsNotPubliclyVisible() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Someone's private list", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("status", catalogm.ShowStatusPrivate).Error)

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(0), s.inAppAlerts(userID, showID))
}

// TestArtistAlert_SubmitterIsNotAlerted mirrors the scene pass: whoever entered
// the show does not need telling it exists.
func (s *NotificationFilterSuite) TestArtistAlert_SubmitterIsNotAlerted() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("submitted_by", userID).Error)

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(0), s.inAppAlerts(userID, showID))
}

// TestArtistAlert_AccountMatrixReachesUnconfiguredFollows pins the account
// layer end to end: the user opted into show-alert email once, in Settings, and
// never touched a single follow.
func (s *NotificationFilterSuite) TestArtistAlert_AccountMatrixReachesUnconfiguredFollows() {
	capture := s.withCapturedEmail()

	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)
	defaults := json.RawMessage(`{"shows":{"email":true}}`)
	prefs := authm.UserPreferences{UserID: userID, AlertDefaults: &defaults}
	s.Require().NoError(s.db.Create(&prefs).Error)

	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Len(capture.sent, 1)
}
