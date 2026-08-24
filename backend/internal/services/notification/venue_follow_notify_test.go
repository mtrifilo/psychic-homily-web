package notification

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/utils"
)

// Venue new-show alert delivery (PSY-1895). Integration cases run inside
// NotificationFilterSuite (real Postgres, all migrations) for the same reason
// the artist cases do, and here it matters more: the exactly-once guarantee IS
// uq_notification_log_venue_show_alert, and this feature deliberately re-runs
// delivery for batches it has already sent. A mocked DB would assert the Go and
// skip the constraint doing the work.

// =============================================================================
// UNIT TESTS (no database)
// =============================================================================

// venueSettings builds an alerts document the way a user who has touched the
// Library control has one.
func venueSettings(doc string) *json.RawMessage {
	raw := json.RawMessage(doc)
	return &raw
}

func venueAlertShows(ids ...uint) []venueAlertShow {
	out := make([]venueAlertShow, 0, len(ids))
	for _, id := range ids {
		out = append(out, venueAlertShow{ID: id, Title: fmt.Sprintf("Show %d", id)})
	}
	return out
}

// TestResolveVenueAlertRecipients covers every rule that decides who gets a
// batch and which shows their copy names.
func TestResolveVenueAlertRecipients(t *testing.T) {
	const (
		alice = uint(1)
		bob   = uint(2)
	)
	shows := venueAlertShows(10, 11, 12)

	submittedBy := func(items []venueAlertShow, user uint, ids ...uint) []venueAlertShow {
		want := map[uint]struct{}{}
		for _, id := range ids {
			want[id] = struct{}{}
		}
		out := make([]venueAlertShow, len(items))
		copy(out, items)
		for i := range out {
			if _, ok := want[out[i].ID]; ok {
				u := user
				out[i].SubmittedBy = &u
			}
		}
		return out
	}

	cases := []struct {
		name        string
		followers   []venueFollowerRow
		shows       []venueAlertShow
		alreadyTold map[uint]map[uint]struct{}
		wantUsers   []uint
		wantInApp   bool
		wantEmail   bool
		wantShows   []uint
	}{
		{
			// The shipped default: in-app ON, email OFF. Email is an intentional
			// opt-in on every alert type (PSY-1892 decision 4), and a follow that
			// stored nothing must inherit exactly that.
			name:      "a follow with no stored settings gets in-app only",
			followers: []venueFollowerRow{{UserID: alice}},
			shows:     shows,
			wantUsers: []uint{alice},
			wantInApp: true,
			wantEmail: false,
			wantShows: []uint{10, 11, 12},
		},
		{
			name: "email opt-in is honoured",
			followers: []venueFollowerRow{
				{UserID: alice, Settings: venueSettings(`{"alerts":{"shows":{"email":true}}}`)},
			},
			shows:     shows,
			wantUsers: []uint{alice},
			wantInApp: true,
			wantEmail: true,
			wantShows: []uint{10, 11, 12},
		},
		{
			name: "alerts switched off deliver nothing",
			followers: []venueFollowerRow{
				{UserID: alice, Settings: venueSettings(`{"alerts":{"shows":{"enabled":false}}}`)},
			},
			shows:     shows,
			wantUsers: nil,
		},
		{
			name: "both channels off deliver nothing",
			followers: []venueFollowerRow{
				{UserID: alice, Settings: venueSettings(`{"alerts":{"shows":{"in_app":false,"email":false}}}`)},
			},
			shows:     shows,
			wantUsers: nil,
		},
		{
			// Venue show alerts have NO scope axis: a venue sits in one place, so
			// "near me" has nothing to mean. A stored scope is stale data the
			// resolver already discards, and this pins that it cannot start
			// filtering venue followers by an area they never chose.
			name: "a stored scope on an axis-less alert type filters nothing",
			followers: []venueFollowerRow{
				{UserID: alice, Settings: venueSettings(`{"alerts":{"shows":{"scope":"near_me"}}}`)},
			},
			shows:     shows,
			wantUsers: []uint{alice},
			wantInApp: true,
			wantShows: []uint{10, 11, 12},
		},
		{
			// Cross-system dedup is per SHOW, not per batch. One overlap must not
			// cost the reader the other two announcements.
			name:      "a show already announced elsewhere drops from that user's copy",
			followers: []venueFollowerRow{{UserID: alice}},
			shows:     shows,
			alreadyTold: map[uint]map[uint]struct{}{
				alice: {11: {}},
			},
			wantUsers: []uint{alice},
			wantInApp: true,
			wantShows: []uint{10, 12},
		},
		{
			// ...but when nothing is left, the alert is not sent at all. A message
			// that names no shows is worse than no message.
			name:      "a batch entirely covered elsewhere delivers nothing",
			followers: []venueFollowerRow{{UserID: alice}},
			shows:     shows,
			alreadyTold: map[uint]map[uint]struct{}{
				alice: {10: {}, 11: {}, 12: {}},
			},
			wantUsers: nil,
		},
		{
			// Self-exclusion. The artist pass excludes the submitter of THE show;
			// a batch has many, so the honest rule is "you entered all of these".
			name:      "a user who entered every show in the batch is excluded",
			followers: []venueFollowerRow{{UserID: alice}},
			shows:     submittedBy(shows, alice, 10, 11, 12),
			wantUsers: nil,
		},
		{
			// The half that matters: entering ONE date must not silence the other
			// two, which a naive "is the submitter" check would do.
			name:      "a user who entered only some shows still hears about the rest",
			followers: []venueFollowerRow{{UserID: alice}},
			shows:     submittedBy(shows, alice, 10),
			wantUsers: []uint{alice},
			wantInApp: true,
			wantShows: []uint{10, 11, 12},
		},
		{
			name:      "output order is deterministic",
			followers: []venueFollowerRow{{UserID: bob}, {UserID: alice}},
			shows:     shows,
			wantUsers: []uint{alice, bob},
			wantInApp: true,
			wantShows: []uint{10, 11, 12},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVenueAlertRecipients(
				tc.followers, map[uint]recipientAlertPrefs{}, tc.shows, tc.alreadyTold)

			gotUsers := make([]uint, 0, len(got))
			for _, r := range got {
				gotUsers = append(gotUsers, r.userID)
			}
			assert.Equal(t, tc.wantUsers, nilIfEmptyUints(gotUsers))
			if len(tc.wantUsers) == 0 {
				return
			}

			first := got[0]
			assert.Equal(t, tc.wantInApp, first.inApp)
			assert.Equal(t, tc.wantEmail, first.email)

			gotShows := make([]uint, 0, len(first.shows))
			for _, sh := range first.shows {
				gotShows = append(gotShows, sh.ID)
			}
			assert.Equal(t, tc.wantShows, gotShows)
		})
	}
}

func nilIfEmptyUints(in []uint) []uint {
	if len(in) == 0 {
		return nil
	}
	return in
}

// TestVenueAlertShowCountPhrase pins the singular/plural split, which the
// subject line and the headline both read from so they cannot disagree.
func TestVenueAlertShowCountPhrase(t *testing.T) {
	assert.Equal(t, "New show", venueAlertShowCountPhrase(1))
	assert.Equal(t, "3 new shows", venueAlertShowCountPhrase(3))
	// Not reachable: a recipient with no shows never reaches the sender. Pinned
	// so a future caller that does reach it gets something grammatical rather
	// than "1 new show" about nothing.
	assert.Equal(t, "0 new shows", venueAlertShowCountPhrase(0))
}

// TestFormatAlertBucket covers the day/instant boundary. A DATE arrives from the
// driver as a midnight; formatting it in any zone but UTC shifts it by a day.
func TestFormatAlertBucket(t *testing.T) {
	assert.Equal(t, "", formatAlertBucket(nil))

	day := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "2026-08-24", formatAlertBucket(&day))

	// The same calendar day carried in a western zone must not read as the day
	// before.
	phx := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC).In(utils.EventLocation(nil, "AZ"))
	assert.Equal(t, "2026-08-24", formatAlertBucket(&phx))
}

// TestBuildVenueShowAlertEmailHTML covers the digest's content and its escaping.
func TestBuildVenueShowAlertEmailHTML(t *testing.T) {
	loc := utils.EventLocation(nil, "AZ")
	batch := &venueAlertBatch{
		key:       venueAlertGroupKey{VenueID: 7, AlertDay: "2026-08-24"},
		venueName: "Valley Bar",
		venueURL:  "https://example.com/venues/valley-bar",
		loc:       loc,
	}
	shows := []venueAlertShow{
		{ID: 1, Title: "Oneida", EventDate: time.Date(2026, 8, 29, 20, 0, 0, 0, time.UTC), ArtistText: "Oneida, Din of Celestial Birds"},
		{ID: 2, Title: "Chat Pile", EventDate: time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC), ArtistText: "Chat Pile"},
	}

	html := buildVenueShowAlertEmailHTML(
		batch, shows,
		"https://api.example.com/unsubscribe/artist-show-alerts?uid=1&sig=abc",
		"https://example.com/settings/notifications")

	assert.Contains(t, html, "VENUE ALERT · NEW SHOWS")
	assert.Contains(t, html, "2 new shows at Valley Bar.")
	assert.Contains(t, html, "Oneida, Din of Celestial Birds")
	assert.Contains(t, html, "Chat Pile")
	assert.Contains(t, html, "https://example.com/venues/valley-bar")
	assert.Contains(t, html, "View venue")
	// RFC 8058's in-body half: a visible, working opt-out.
	assert.Contains(t, html, "Unsubscribe from show alerts")
	assert.Contains(t, html, "unsubscribe/artist-show-alerts")
	assert.Contains(t, html, "Manage alerts in Settings")

	// The copy promises GROUPING, not completeness, because a show announced
	// later the same day joins the inbox row without a second email. Wording
	// that claimed to list everything would be a promise this cannot keep.
	assert.Contains(t, html, "grouped into one alert a day")
	assert.NotContains(t, html, "all shows announced")

	// A show with no bill still appears, so the headline's count cannot disagree
	// with the list under it.
	bare := buildVenueShowAlertEmailHTML(batch,
		[]venueAlertShow{{ID: 3, Title: "TBA", EventDate: time.Date(2026, 9, 9, 20, 0, 0, 0, time.UTC)}},
		"https://api.example.com/u", "https://example.com/m")
	assert.Contains(t, bare, "New show at Valley Bar.")
	assert.Contains(t, bare, "TBA")
}

// TestBuildVenueShowAlertEmailHTML_EscapesScrapedText is the security case. An
// ingest-created venue calendar is scraped third-party text and this message
// ships from the platform's own DKIM-aligned sender, so a name that closes a
// tag would otherwise land a working link inside trusted mail.
func TestBuildVenueShowAlertEmailHTML_EscapesScrapedText(t *testing.T) {
	batch := &venueAlertBatch{
		key:       venueAlertGroupKey{VenueID: 7, AlertDay: "2026-08-24"},
		venueName: `</td><a href="https://evil.example">Verify</a>`,
		venueURL:  "https://example.com/venues/x",
		loc:       time.UTC,
	}
	shows := []venueAlertShow{{
		ID:         1,
		Title:      `<script>alert(1)</script>`,
		EventDate:  time.Date(2026, 8, 29, 20, 0, 0, 0, time.UTC),
		ArtistText: `<img src=x onerror=alert(1)>`,
	}}

	html := buildVenueShowAlertEmailHTML(batch, shows, "https://api.example.com/u", "https://example.com/m")

	// Assert on the TAGS, not on the payload text. An escaped `&lt;img src=x
	// onerror=alert(1)&gt;` still contains the substring "onerror=" and is inert,
	// so asserting the substring is absent would be asserting something the
	// escaping does not (and need not) provide.
	assert.NotContains(t, html, `<a href="https://evil.example">`)
	assert.NotContains(t, html, "<script>")
	assert.NotContains(t, html, "<img")
	assert.Contains(t, html, "&lt;script&gt;")
	assert.Contains(t, html, "&lt;img src=x onerror=alert(1)&gt;")
}

// TestSanitizeVenueAlertSubject covers the OTHER escaping medium. htmlEscape
// does nothing for a header, where the dangerous character is a newline rather
// than an angle bracket.
func TestSanitizeVenueAlertSubject(t *testing.T) {
	got := sanitizeEmailHeaderValue("Valley Bar\r\nBcc: victim@example.com")
	assert.NotContains(t, got, "\n")
	assert.NotContains(t, got, "\r")
}

// TestEmailListRows covers the one layout element the digest could not inherit.
func TestEmailListRows(t *testing.T) {
	assert.Empty(t, emailListRows(nil), "no rows must render no block, not an empty table")

	html := emailListRows([]emailListRow{
		{Label: "SAT AUG 29", Title: "Oneida", Detail: "with Din of Celestial Birds"},
		{Label: "FRI SEP 05", Title: "Chat Pile"},
	})
	assert.Contains(t, html, "SAT AUG 29")
	assert.Contains(t, html, "Oneida")
	assert.Contains(t, html, "with Din of Celestial Birds")
	// The last row closes the block; the others must not, or adjacent rules
	// double up into a 2px line.
	assert.Equal(t, 2, strings.Count(html, "border-bottom:0 solid"))
	assert.Equal(t, 2, strings.Count(html, "border-bottom:1px solid"))
	// A row with no detail renders no detail line rather than an empty one.
	assert.Equal(t, 1, strings.Count(html, "font-size:13px; line-height:19px"))

	hostile := emailListRows([]emailListRow{{Label: "X", Title: `<b>x</b>`, Detail: `<i>y</i>`}})
	assert.NotContains(t, hostile, "<b>x</b>")
	assert.NotContains(t, hostile, "<i>y</i>")
}

// TestShowAlertEntityTypesExcludeVenueAlerts is the guard for the single most
// damaging edit this ticket makes possible.
//
// notifiedAboutShow compares notification_log.entity_id against SHOW ids. A
// venue show alert's entity_id is a VENUE id, so listing it here would make
// venue 42 read as "already told about show 42" and silence a filter match, an
// artist alert or a scene alert for an unrelated event — for every user who
// follows that venue. Nothing else in the system would raise anything.
func TestShowAlertEntityTypesExcludeVenueAlerts(t *testing.T) {
	assert.NotContains(t, showAlertEntityTypes, notificationm.NotificationEntityVenueShowAlert,
		"venue_show_alert's entity_id is a VENUE id; listing it here makes notifiedAboutShow "+
			"compare venue ids against show ids and silence unrelated notifications")

	// And the predicate itself, so the guard survives showAlertEntityTypes being
	// replaced by a different mechanism.
	assert.NotContains(t, notifiedAboutShow("nl"), notificationm.NotificationEntityVenueShowAlert)
}

// TestEmailLaneAlertTypesIncludeVenueAlerts is the mirror-image guard. Leaving a
// two-lane type OFF that list does not fail loudly: its email-lane rows simply
// start appearing in the bell and inflating the unread badge.
func TestEmailLaneAlertTypesIncludeVenueAlerts(t *testing.T) {
	assert.Contains(t, emailLaneAlertTypes, notificationm.NotificationEntityVenueShowAlert)
	assert.Contains(t, inboxVisibleRows("nl"), notificationm.NotificationEntityVenueShowAlert,
		"the email lane of a venue alert must be hidden from the inbox")
}

// TestVenueAndArtistAlertEmailsShareOneUnsubscribeScope pins the PSY-1895
// unsubscribe decision so the two templates cannot drift apart.
//
// Venue show alert emails REUSE artist-show-alerts rather than minting a scope
// of their own, because there is no narrower mutation to perform:
// alert_defaults carries ONE `shows` key covering both, and
// UserService.UnsubscribeArtistShowAlertEmails already sweeps venue follows as
// well as artist ones. A second scope would be a second name for an identical
// action, and the day one drifts is the day an unsubscribe stops
// unsubscribing.
func TestVenueAndArtistAlertEmailsShareOneUnsubscribeScope(t *testing.T) {
	const (
		backend = "https://api.example.com"
		secret  = "test-secret"
		userID  = uint(42)
	)
	shared := engagement.GenerateScopedUnsubscribeURL(
		backend, userID, engagement.UnsubscribeScopeArtistShowAlerts, secret)

	batch := &venueAlertBatch{
		key:       venueAlertGroupKey{VenueID: 7, AlertDay: "2026-08-24"},
		venueName: "Valley Bar",
		venueURL:  "https://example.com/venues/valley-bar",
		loc:       time.UTC,
	}
	venueHTML := buildVenueShowAlertEmailHTML(batch,
		venueAlertShows(1), shared, "https://example.com/m")

	artistHTML := buildArtistShowAlertEmailHTML("Oneida",
		contracts.FollowAlertScopeEverywhere,
		showEmailContentParts{date: "today"},
		shared, "https://example.com/m")

	assert.Contains(t, venueHTML, "unsubscribe/artist-show-alerts")
	assert.Contains(t, artistHTML, "unsubscribe/artist-show-alerts")

	// And the signature verifies under that scope, so the link the recipient
	// clicks actually works rather than 403ing at the door.
	assert.True(t, strings.Contains(shared, "sig="))
}

// =============================================================================
// INTEGRATION TESTS (Testcontainer PostgreSQL)
// =============================================================================

// followVenueWithAlerts creates a venue follow carrying an explicit alerts
// document. An empty document is the shipped default (in-app on, email off).
func (s *NotificationFilterSuite) followVenueWithAlerts(userID, venueID uint, alertsJSON string) {
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at, settings)
		VALUES (?, 'venue', ?, 'follow', now(), ?::jsonb)`, userID, venueID, alertsJSON).Error)
}

// venueLocalToday is the calendar day accrual will key on for a Phoenix venue,
// computed the same way accrual computes it.
func venueLocalToday() string {
	return time.Now().In(utils.EventLocation(nil, "AZ")).Format(alertDayLayout)
}

// venueAlertRows counts a user's venue-alert rows for one venue-day, per lane.
func (s *NotificationFilterSuite) venueAlertRows(userID, venueID uint, day, channel string) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND channel = ? AND alert_bucket = ?::date",
			userID, notificationm.NotificationEntityVenueShowAlert, venueID, channel, day).
		Count(&n).Error)
	return n
}

func (s *NotificationFilterSuite) venueInAppAlerts(userID, venueID uint, day string) int64 {
	return s.venueAlertRows(userID, venueID, day, notificationm.NotificationChannelInApp)
}

// batchMembers counts the shows accrued into one venue-day batch.
func (s *NotificationFilterSuite) batchMembers(venueID uint, day string) int64 {
	var n int64
	s.Require().NoError(s.db.Table("venue_show_alert_batch").
		Where("venue_id = ? AND alert_day = ?::date", venueID, day).
		Count(&n).Error)
	return n
}

// flushNow runs a flush with both bounds at zero, so every pending batch is
// considered ready. Tests that care about the quiet window pass their own.
func (s *NotificationFilterSuite) flushNow() int {
	return s.svc.FlushVenueShowAlerts(50, 0, 0)
}

// announce runs the full trigger path for a show, exactly as the admin approve
// handlers and the show-notify outbox poller both do.
func (s *NotificationFilterSuite) announce(showID uint) {
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
}

// TestVenueAlert_BulkIngestCoalescesToOneAlert is AC 1. Five shows land at a
// followed venue inside one day and the follower is told ONCE.
func (s *NotificationFilterSuite) TestVenueAlert_BulkIngestCoalescesToOneAlert() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	day := venueLocalToday()

	for i := 0; i < 5; i++ {
		showID := s.createTestShow(fmt.Sprintf("Show %d at Valley Bar", i), nil, []uint{venueID})
		s.announce(showID)
	}

	s.Equal(int64(5), s.batchMembers(venueID, day), "every show should accrue")
	// Nothing is delivered by accrual alone.
	s.Equal(int64(0), s.venueInAppAlerts(userID, venueID, day))

	s.Equal(1, s.flushNow())
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day),
		"five shows in one venue-day must produce exactly one alert")
}

// TestVenueAlert_NextDayIsASecondAlert is the other half of AC 1: the coalescing
// is per DAY, so tomorrow's drop is a new notification rather than a duplicate.
//
// Yesterday's batch is inserted directly. Accrual reads the wall clock, so the
// only alternative would be a clock seam in production code for the benefit of
// one test.
func (s *NotificationFilterSuite) TestVenueAlert_NextDayIsASecondAlert() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)

	yesterdayShow := s.createTestShow("Yesterday's drop", nil, []uint{venueID})
	yesterday := time.Now().In(utils.EventLocation(nil, "AZ")).AddDate(0, 0, -1).Format(alertDayLayout)
	s.Require().NoError(s.db.Exec(`
		INSERT INTO venue_show_alert_batch (venue_id, alert_day, show_id, created_at)
		VALUES (?, ?::date, ?, now() - interval '1 day')`, venueID, yesterday, yesterdayShow).Error)

	s.Equal(1, s.flushNow())
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, yesterday))

	todayShow := s.createTestShow("Today's drop", nil, []uint{venueID})
	s.announce(todayShow)
	today := venueLocalToday()

	s.Equal(1, s.flushNow())
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, today),
		"a new day is a new alert, not a duplicate of yesterday's")
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, yesterday),
		"yesterday's alert must not be re-sent")
}

// TestVenueAlert_LateShowFoldsIntoDispatchedBatch is the distinctive mechanic: a
// show announced after the batch went out joins the inbox row rather than
// producing a second notification, and sends no second email.
func (s *NotificationFilterSuite) TestVenueAlert_LateShowFoldsIntoDispatchedBatch() {
	capture := s.withCapturedEmail()
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{"alerts":{"shows":{"email":true}}}`)
	day := venueLocalToday()

	s.announce(s.createTestShow("First drop", nil, []uint{venueID}))
	s.Equal(1, s.flushNow())
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day))
	s.Require().Len(capture.sent, 1)

	// A late arrival, after the batch was already delivered.
	s.announce(s.createTestShow("Late addition", nil, []uint{venueID}))
	s.Equal(int64(2), s.batchMembers(venueID, day))

	s.Equal(1, s.flushNow(), "the group is re-resolved because it has a new member")
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day),
		"the late show must fold into the existing row, not mint a second one")
	s.Len(capture.sent, 1, "a late show must not send a second email")

	// The inbox row GREW: enrichment reads the batch live, which is the whole
	// reason the show list is not stamped onto the row at write time.
	entries, err := s.svc.GetUserNotifications(userID, 20, 0)
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	s.Equal(2, entries[0].AlertShowCount)
}

// TestVenueAlert_NoAlertForPreExistingShows is the rollout guard. The property
// is structural — the table ships empty and only accrual writes to it — so this
// asserts that a show which existed before the feature cannot be flushed.
func (s *NotificationFilterSuite) TestVenueAlert_NoAlertForPreExistingShows() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)

	// A show that exists and is perfectly announceable, but whose visibility
	// transition happened before this feature: no accrual row was ever written.
	s.createTestShow("A show from before the rollout", nil, []uint{venueID})

	s.Equal(int64(0), s.batchMembers(venueID, venueLocalToday()))
	s.Equal(0, s.flushNow())
	s.Equal(int64(0), s.venueInAppAlerts(userID, venueID, venueLocalToday()),
		"nothing backfills the batch table, so the existing catalogue can never alert")
}

// TestVenueAlert_EmailIsOffUntilOptedIn pins decision 4 on this alert type.
func (s *NotificationFilterSuite) TestVenueAlert_EmailIsOffUntilOptedIn() {
	capture := s.withCapturedEmail()
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	day := venueLocalToday()

	s.announce(s.createTestShow("A show", nil, []uint{venueID}))
	s.Equal(1, s.flushNow())

	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day), "in-app is on by default")
	s.Empty(capture.sent, "email must stay off until the user opts in")
	s.Equal(int64(0), s.venueAlertRows(userID, venueID, day, notificationm.NotificationChannelEmail))
}

// TestVenueAlert_EmailOptInSendsWithWorkingUnsubscribe covers the opt-in path
// and the RFC 8058 header the provider's native button posts to.
func (s *NotificationFilterSuite) TestVenueAlert_EmailOptInSendsWithWorkingUnsubscribe() {
	capture := s.withCapturedEmail()
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{"alerts":{"shows":{"email":true}}}`)

	s.announce(s.createTestShow("Oneida at Valley Bar", nil, []uint{venueID}))
	s.Equal(1, s.flushNow())

	s.Require().Len(capture.sent, 1)
	sent := capture.sent[0]
	s.Contains(sent.subject, "Valley Bar")
	s.Contains(sent.unsubscribeURL, "unsubscribe/artist-show-alerts")
	s.Contains(sent.html, "Unsubscribe from show alerts")
	s.Contains(sent.html, "VENUE ALERT · NEW SHOWS")
}

// TestVenueAlert_ReRunDoesNotRenotify is the exactly-once case, and it is the
// one that has to hold: this feature re-resolves delivered batches by design.
func (s *NotificationFilterSuite) TestVenueAlert_ReRunDoesNotRenotify() {
	capture := s.withCapturedEmail()
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{"alerts":{"shows":{"email":true}}}`)
	day := venueLocalToday()

	showID := s.createTestShow("A show", nil, []uint{venueID})
	s.announce(showID)
	s.Equal(1, s.flushNow())

	// Re-run the TRIGGER (a reclaimed outbox row, an admin approve after an
	// ingest) and then the flush, twice. Neither may produce a second anything.
	s.announce(showID)
	s.flushNow()
	s.flushNow()

	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day))
	s.Equal(int64(1), s.venueAlertRows(userID, venueID, day, notificationm.NotificationChannelEmail))
	s.Len(capture.sent, 1)
	s.Equal(int64(1), s.batchMembers(venueID, day), "re-announcing must not accrue a second membership")
}

// TestVenueAlert_TwoFlushesInARowSendOnce is the concurrency shape stated
// sequentially: the claim, not the dispatched_at stamp, is what makes the second
// pass silent. Undoing the stamp simulates a flush that crashed after delivering.
func (s *NotificationFilterSuite) TestVenueAlert_TwoFlushesInARowSendOnce() {
	capture := s.withCapturedEmail()
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{"alerts":{"shows":{"email":true}}}`)
	day := venueLocalToday()

	s.announce(s.createTestShow("A show", nil, []uint{venueID}))
	s.Equal(1, s.flushNow())
	s.Require().Len(capture.sent, 1)

	s.Require().NoError(s.db.Exec(
		`UPDATE venue_show_alert_batch SET dispatched_at = NULL WHERE venue_id = ?`, venueID).Error)

	s.Equal(1, s.flushNow(), "the batch is re-resolved")
	s.Len(capture.sent, 1, "the delivery claim, not dispatched_at, is the exactly-once guard")
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day))
}

// TestVenueAlert_QuietWindowHoldsUntilTheDropFinishes covers the scheduling
// bound: a batch that just gained a member is not ready.
func (s *NotificationFilterSuite) TestVenueAlert_QuietWindowHoldsUntilTheDropFinishes() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	day := venueLocalToday()

	s.announce(s.createTestShow("A show", nil, []uint{venueID}))

	// A generous window and hold: the drop is seconds old, so nothing is ready.
	s.Equal(0, s.svc.FlushVenueShowAlerts(50, time.Hour, time.Hour))
	s.Equal(int64(0), s.venueInAppAlerts(userID, venueID, day))

	// The max hold retires it even though it never went quiet, which is what
	// stops a trickling venue from starving its followers.
	s.Equal(1, s.svc.FlushVenueShowAlerts(50, time.Hour, 0))
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day))
}

// TestVenueAlert_EmailOnlyRowIsNotABellEntry pins the two-lane read rule. The
// email lane's row exists so the daily budget can count it, not so the bell can
// render it.
func (s *NotificationFilterSuite) TestVenueAlert_EmailOnlyRowIsNotABellEntry() {
	s.withCapturedEmail()
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID,
		`{"alerts":{"shows":{"in_app":false,"email":true}}}`)
	day := venueLocalToday()

	s.announce(s.createTestShow("A show", nil, []uint{venueID}))
	s.Equal(1, s.flushNow())

	s.Equal(int64(0), s.venueInAppAlerts(userID, venueID, day))
	s.Equal(int64(1), s.venueAlertRows(userID, venueID, day, notificationm.NotificationChannelEmail))

	entries, err := s.svc.GetUserNotifications(userID, 20, 0)
	s.Require().NoError(err)
	s.Empty(entries, "the email lane of a venue alert is not a bell entry")

	count, err := s.svc.GetUnreadCount(userID)
	s.Require().NoError(err)
	s.Equal(int64(0), count, "and it must not inflate the unread badge either")
}

// TestVenueAlert_InboxRowNamesTheVenueAndItsShows covers the read path the row
// component renders from.
func (s *NotificationFilterSuite) TestVenueAlert_InboxRowNamesTheVenueAndItsShows() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)

	for i := 0; i < 5; i++ {
		s.announce(s.createTestShow(fmt.Sprintf("Night %d", i), nil, []uint{venueID}))
	}
	s.Equal(1, s.flushNow())

	entries, err := s.svc.GetUserNotifications(userID, 20, 0)
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]

	s.Equal(notificationm.NotificationEntityVenueShowAlert, e.EntityType)
	s.Equal(venueID, e.EntityID, "entity_id is the VENUE id, not a show id")
	s.Nil(e.SubjectEntityID, "the followed entity and the subject are the same venue")
	s.Equal(venueLocalToday(), e.AlertBucket)
	s.Equal("Valley Bar", e.AlertVenueName)
	s.Contains(e.AlertVenueURL, "/venues/Valley Bar")
	s.Equal(5, e.AlertShowCount, "the count is the FULL batch size")
	// ...while the summary is capped, and says so rather than under-reporting.
	s.Contains(e.AlertShowSummary, "and 2 more")
}

// TestVenueAlert_AlreadyAnnouncedShowDropsFromTheBatch is cross-system dedup end
// to end: an artist alert about one date must not stop the venue alert about the
// others, and must not repeat that date.
func (s *NotificationFilterSuite) TestVenueAlert_AlreadyAnnouncedShowDropsFromTheBatch() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	artistID := s.createTestArtist("Oneida")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"shows":{"scope":"everywhere"}}}`)

	// One show has an artist the user also follows, so the artist pass claims it
	// first. Two others do not.
	s.announce(s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID}))
	s.announce(s.createTestShow("Someone else", nil, []uint{venueID}))
	s.announce(s.createTestShow("Someone else again", nil, []uint{venueID}))

	s.Equal(1, s.flushNow())

	entries, err := s.svc.GetUserNotifications(userID, 20, 0)
	s.Require().NoError(err)

	var venueRows, artistRows int
	for _, e := range entries {
		switch e.EntityType {
		case notificationm.NotificationEntityVenueShowAlert:
			venueRows++
		case notificationm.NotificationEntityArtistShowAlert:
			artistRows++
		}
	}
	s.Equal(1, artistRows, "the artist alert is the more specific claim and wins that show")
	s.Equal(1, venueRows, "the other two shows still earn a venue alert")
}

// TestVenueAlert_UnannounceableMembersAreNotAnnounced covers the delivery-time
// re-check. A show pulled between accrual and flush must not be announced, and
// a batch with nothing left must not send an empty message.
func (s *NotificationFilterSuite) TestVenueAlert_UnannounceableMembersAreNotAnnounced() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	day := venueLocalToday()

	showID := s.createTestShow("A show that gets pulled", nil, []uint{venueID})
	s.announce(showID)
	s.Require().NoError(s.db.Exec(
		`UPDATE shows SET is_cancelled = true WHERE id = ?`, showID).Error)

	s.Equal(1, s.flushNow(), "the batch is retired rather than examined forever")
	s.Equal(int64(0), s.venueInAppAlerts(userID, venueID, day),
		"a cancelled show must not be announced")
}

// TestVenueAlert_UnfollowedVenueAccruesNothing pins the write-side bound that
// keeps a never-pruned table from growing with the catalogue.
func (s *NotificationFilterSuite) TestVenueAlert_UnfollowedVenueAccruesNothing() {
	venueID := s.createTestVenue("Nobody Follows This")
	s.announce(s.createTestShow("A show", nil, []uint{venueID}))
	s.Equal(int64(0), s.batchMembers(venueID, venueLocalToday()))
}
