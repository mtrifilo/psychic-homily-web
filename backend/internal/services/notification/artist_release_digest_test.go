package notification

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
)

// =============================================================================
// UNIT TESTS
// =============================================================================

// The digest's entity_id is a USER id. Listing it as a show alert would make
// user 42 read as "already told about show 42" and suppress unrelated show
// notifications for that reader.
func TestShowAlertEntityTypesExcludeReleaseDigests(t *testing.T) {
	assert.NotContains(t, showAlertEntityTypes, notificationm.NotificationEntityArtistReleaseDigest,
		"artist_release_digest's entity_id is a USER id, not a show id")
	assert.NotContains(t, notifiedAboutShow("nl"), notificationm.NotificationEntityArtistReleaseDigest)
}

// An email-lane row of a two-lane type must never appear as a bell entry.
func TestEmailLaneAlertTypesIncludeReleaseDigests(t *testing.T) {
	assert.Contains(t, emailLaneAlertTypes, notificationm.NotificationEntityArtistReleaseDigest)
	assert.Contains(t, inboxVisibleRows("nl"), notificationm.NotificationEntityArtistReleaseDigest,
		"the email lane of a release digest must be hidden from the inbox")
}

func TestReleaseDigestEmailLaneIsGatedOff(t *testing.T) {
	assert.False(t, releaseDigestEmailLaneEnabled,
		"the release digest email has no approved template; enabling its lane needs its own change")
}

func releaseFollow(userID, artistID uint, settings string) artistFollowerRow {
	row := artistFollowerRow{UserID: userID, ArtistID: artistID}
	if settings != "" {
		raw := json.RawMessage(settings)
		row.Settings = &raw
	}
	return row
}

func TestResolveReleaseDigestRecipients(t *testing.T) {
	releases := []releaseDigestRelease{
		{ReleaseID: 100, ArtistID: 1, ArtistName: "Oneida", Title: "Success"},
		{ReleaseID: 200, ArtistID: 2, ArtistName: "Chat Pile", Title: "Tenkiller"},
	}

	t.Run("shipped defaults are in-app on, email off", func(t *testing.T) {
		got := resolveReleaseDigestRecipients(
			[]artistFollowerRow{releaseFollow(7, 1, "")}, nil, releases)
		if assert.Len(t, got, 1) {
			assert.True(t, got[0].inApp)
			assert.False(t, got[0].email)
			assert.Len(t, got[0].releases, 1)
			assert.Equal(t, uint(100), got[0].releases[0].ReleaseID)
		}
	})

	t.Run("one reader following two releasing artists gets one roundup", func(t *testing.T) {
		got := resolveReleaseDigestRecipients([]artistFollowerRow{
			releaseFollow(7, 2, ""), releaseFollow(7, 1, ""),
		}, nil, releases)
		if assert.Len(t, got, 1) {
			assert.Len(t, got[0].releases, 2)
		}
	})

	t.Run("a reader sees only the artists they follow", func(t *testing.T) {
		got := resolveReleaseDigestRecipients([]artistFollowerRow{
			releaseFollow(7, 1, ""), releaseFollow(8, 2, ""),
		}, nil, releases)
		if assert.Len(t, got, 2) {
			assert.Equal(t, uint(7), got[0].userID)
			assert.Equal(t, uint(100), got[0].releases[0].ReleaseID)
			assert.Equal(t, uint(8), got[1].userID)
			assert.Equal(t, uint(200), got[1].releases[0].ReleaseID)
		}
	})

	t.Run("a follow with release alerts off contributes nothing", func(t *testing.T) {
		got := resolveReleaseDigestRecipients([]artistFollowerRow{
			releaseFollow(7, 1, `{"alerts":{"releases":{"enabled":false}}}`),
		}, nil, releases)
		assert.Empty(t, got)
	})

	t.Run("an account default with both channels off contributes nothing", func(t *testing.T) {
		raw := json.RawMessage(`{"releases":{"in_app":false,"email":false}}`)
		prefs := map[uint]recipientAlertPrefs{7: {AlertDefaults: &raw}}
		got := resolveReleaseDigestRecipients(
			[]artistFollowerRow{releaseFollow(7, 1, "")}, prefs, releases)
		assert.Empty(t, got)
	})

	t.Run("show-alert settings do not govern releases", func(t *testing.T) {
		got := resolveReleaseDigestRecipients([]artistFollowerRow{
			releaseFollow(7, 1, `{"alerts":{"shows":{"enabled":false}}}`),
		}, nil, releases)
		assert.Len(t, got, 1)
	})
}

func TestReleaseDigestUsesItsOwnUnsubscribeScope(t *testing.T) {
	assert.NotEqual(t, engagement.UnsubscribeScopeArtistShowAlerts, engagement.UnsubscribeScopeArtistReleaseAlerts,
		"release alerts have their own preference key, so they need their own unsubscribe scope")

	url := engagement.GenerateScopedUnsubscribeURL(
		"https://api.example.com", 42, engagement.UnsubscribeScopeArtistReleaseAlerts, "test-secret")
	html := buildArtistReleaseDigestEmailHTML([]releaseDigestRelease{
		{ReleaseID: 1, ArtistID: 1, ArtistName: "Oneida", Title: "Success", ReleaseType: "lp",
			URL: "https://example.com/releases/success"},
	}, "https://example.com", url, "https://example.com/settings/notifications")

	assert.Contains(t, html, "unsubscribe/artist-release-alerts")
	assert.NotContains(t, html, "unsubscribe/artist-show-alerts")
}

func TestBuildArtistReleaseDigestEmailHTML_EscapesScrapedText(t *testing.T) {
	html := buildArtistReleaseDigestEmailHTML([]releaseDigestRelease{
		{ReleaseID: 1, ArtistID: 1, ArtistName: "<script>x</script>", Title: `"><img src=x>`,
			URL: `https://example.com/releases/a"b`},
	}, "https://example.com", "https://api.example.com/u", "https://example.com/m")

	assert.NotContains(t, html, "<script>x</script>")
	assert.NotContains(t, html, `"><img src=x>`)
	assert.Contains(t, html, "&lt;SCRIPT&gt;")
}

func TestEmailListRowsHref(t *testing.T) {
	plain := emailListRows([]emailListRow{{Label: "LP", Title: "Success"}})
	assert.NotContains(t, plain, "<a ", "a row with no destination renders its title as plain text")

	linked := emailListRows([]emailListRow{{Label: "LP", Title: "A&B", Href: "https://example.com/r?a=1&b=2"}})
	assert.Contains(t, linked, `<a href="https://example.com/r?a=1&amp;b=2"`)
	assert.Contains(t, linked, ">A&amp;B</a>")
}

func TestReleaseDigestArtistList(t *testing.T) {
	assert.Equal(t, "", releaseDigestArtistList(nil))
	assert.Equal(t, "A, B, C", releaseDigestArtistList([]string{"A", "B", "C"}))
	assert.Equal(t, "A, B, C and 2 more", releaseDigestArtistList([]string{"A", "B", "C", "D", "E"}))
}

func TestReleaseDigestSubject(t *testing.T) {
	one := []releaseDigestRelease{{ArtistName: "Oneida\r\nBcc: x@example.com"}}
	subject := releaseDigestSubject(one)
	assert.NotContains(t, subject, "\r")
	assert.NotContains(t, subject, "\n")
	assert.True(t, strings.HasPrefix(subject, "New release from Oneida"))

	assert.Equal(t, "2 new releases from artists you follow",
		releaseDigestSubject([]releaseDigestRelease{{ArtistName: "A"}, {ArtistName: "B"}}))
}

// =============================================================================
// INTEGRATION TESTS (Testcontainer PostgreSQL)
// =============================================================================

// releaseDayAgo is a release_date inside the recency window.
func releaseDayAgo(days int) *string {
	d := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	return &d
}

// currentAlertWeek is the bucket accrual keys a release created now on.
func currentAlertWeek() string {
	return catalog.AlertWeekStart(time.Now()).Format(time.DateOnly)
}

// createRelease runs the real create funnel, where accrual is hooked.
func (s *NotificationFilterSuite) createRelease(title string, date *string, year *int, artistIDs ...uint) uint {
	entries := make([]contracts.CreateReleaseArtistEntry, 0, len(artistIDs))
	for _, id := range artistIDs {
		entries = append(entries, contracts.CreateReleaseArtistEntry{ArtistID: id, Role: "main"})
	}
	created, err := catalog.NewReleaseService(s.db).CreateRelease(&contracts.CreateReleaseRequest{
		Title:       title,
		ReleaseType: "lp",
		ReleaseDate: date,
		ReleaseYear: year,
		Artists:     entries,
	})
	s.Require().NoError(err)
	return created.ID
}

func (s *NotificationFilterSuite) releaseBatchMembers(artistID uint) int64 {
	var n int64
	s.Require().NoError(s.db.Table("artist_release_alert_batch").
		Where("artist_id = ?", artistID).Count(&n).Error)
	return n
}

func (s *NotificationFilterSuite) releaseDigestRows(userID uint, channel string) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_type = ? AND channel = ?",
			userID, notificationm.NotificationEntityArtistReleaseDigest, channel).
		Count(&n).Error)
	return n
}

// flushReleaseDigestsNow runs a flush with the week hold at zero, so the
// current week counts as closed.
func (s *NotificationFilterSuite) flushReleaseDigestsNow() int {
	return s.svc.FlushArtistReleaseDigests(context.Background(), 50, 0, 0)
}

func (s *NotificationFilterSuite) releaseDigestEntries(userID uint) []contracts.NotificationLogEntry {
	entries, err := s.svc.GetUserNotifications(contracts.ShowViewer{UserID: userID}, 50, 0)
	s.Require().NoError(err)
	var out []contracts.NotificationLogEntry
	for _, e := range entries {
		if e.EntityType == notificationm.NotificationEntityArtistReleaseDigest {
			out = append(out, e)
		}
	}
	return out
}

func (s *NotificationFilterSuite) TestReleaseDigest_NewReleaseProducesOneInAppRow() {
	capture := s.withCapturedEmail()
	userID := s.createTestUser()
	artistID := s.createTestArtist("Russian Circles")
	s.followArtistWithAlerts(userID, artistID, `{}`)

	s.createRelease("Gnosis", releaseDayAgo(3), nil, artistID)
	s.Equal(int64(1), s.releaseBatchMembers(artistID))
	s.Equal(1, s.flushReleaseDigestsNow())

	var row notificationm.NotificationLog
	s.Require().NoError(s.db.Where("user_id = ? AND entity_type = ?",
		userID, notificationm.NotificationEntityArtistReleaseDigest).First(&row).Error)
	s.Equal(notificationm.NotificationChannelInApp, row.Channel)
	s.Equal(userID, row.EntityID, "a weekly roundup's entity_id is its reader")
	s.Nil(row.SubjectEntityID)
	s.Require().NotNil(row.AlertBucket)
	s.Equal(currentAlertWeek(), row.AlertBucket.UTC().Format(time.DateOnly))

	s.Empty(capture.sent)
	s.Equal(int64(0), s.releaseDigestRows(userID, notificationm.NotificationChannelEmail))
}

func (s *NotificationFilterSuite) TestReleaseDigest_OpenWeekIsHeld() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Russian Circles")
	s.followArtistWithAlerts(userID, artistID, `{}`)
	s.createRelease("Gnosis", releaseDayAgo(1), nil, artistID)

	s.Equal(0, s.svc.FlushArtistReleaseDigests(context.Background(), 50, defaultReleaseDigestWeekHold, 0),
		"the current week has not closed")
	s.Equal(int64(0), s.releaseDigestRows(userID, notificationm.NotificationChannelInApp))
}

// The email lane is gated off: a reader who opted into release emails at both
// the account and the follow level still receives only the in-app row, and no
// email-lane row is claimed, so enabling the lane later cannot find weeks it
// already believes were mailed.
func (s *NotificationFilterSuite) TestReleaseDigest_EmailLaneSendsNothing() {
	capture := s.withCapturedEmail()
	userID := s.createTestUser()
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).
		Update("email_verified", true).Error)
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_preferences (user_id, alert_defaults)
		VALUES (?, '{"releases":{"in_app":true,"email":true}}'::jsonb)
		ON CONFLICT (user_id) DO UPDATE SET alert_defaults = EXCLUDED.alert_defaults`, userID).Error)
	artistID := s.createTestArtist("Russian Circles")
	s.followArtistWithAlerts(userID, artistID, `{"alerts":{"releases":{"email":true}}}`)

	s.createRelease("Gnosis", releaseDayAgo(3), nil, artistID)
	s.Equal(1, s.flushReleaseDigestsNow())
	s.flushReleaseDigestsNow()

	s.Empty(capture.sent, "no release-digest email may be dispatched while the lane is gated off")
	s.Equal(int64(0), s.releaseDigestRows(userID, notificationm.NotificationChannelEmail))
	s.Equal(int64(1), s.releaseDigestRows(userID, notificationm.NotificationChannelInApp))
}

// Enrichment re-runs, metadata edits, an edition or format change and a
// re-import of the same release group all leave the one alert alone.
func (s *NotificationFilterSuite) TestReleaseDigest_ReRunsAndEditsDoNotRenotify() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Russian Circles")
	s.followArtistWithAlerts(userID, artistID, `{}`)

	const rgMBID = "11111111-2222-3333-4444-555555555555"
	req := &contracts.CreateReleaseRequest{
		Title:       "Gnosis",
		ReleaseType: "lp",
		ReleaseDate: releaseDayAgo(3),
		Artists:     []contracts.CreateReleaseArtistEntry{{ArtistID: artistID, Role: "main"}},
	}
	rel, created, err := catalog.FindOrCreateReleaseByReleaseGroupMBID(s.db, rgMBID, req)
	s.Require().NoError(err)
	s.Require().True(created)
	s.Equal(1, s.flushReleaseDigestsNow())

	// A discography re-import of the same release group finds the row.
	_, created, err = catalog.FindOrCreateReleaseByReleaseGroupMBID(s.db, rgMBID, req)
	s.Require().NoError(err)
	s.False(created)

	// A metadata edit and a format change go through UpdateRelease.
	releases := catalog.NewReleaseService(s.db)
	newType := "ep"
	newTitle := "Gnosis (Deluxe)"
	_, err = releases.UpdateRelease(rel.ID, &contracts.UpdateReleaseRequest{
		ReleaseType: &newType, Title: &newTitle, ReleaseDate: releaseDayAgo(2),
	})
	s.Require().NoError(err)
	_, err = releases.AddExternalLink(rel.ID, "bandcamp", "https://russiancircles.bandcamp.com/album/gnosis")
	s.Require().NoError(err)

	s.flushReleaseDigestsNow()
	s.flushReleaseDigestsNow()

	s.Equal(int64(1), s.releaseBatchMembers(artistID))
	s.Equal(int64(1), s.releaseDigestRows(userID, notificationm.NotificationChannelInApp))
}

// Rollout: releases that already exist when the feature ships, and
// back-catalogue records imported afterwards, never produce an alert.
func (s *NotificationFilterSuite) TestReleaseDigest_NoAlertsForExistingDiscographies() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Russian Circles")

	// A release that predates the follow accrues nothing: accrual records only
	// artists who have a follower at insert time.
	s.createRelease("Station", releaseDayAgo(5), nil, artistID)
	s.followArtistWithAlerts(userID, artistID, `{}`)

	// A pre-existing catalogue row written outside the funnel (seed, restore).
	pre := catalogm.Release{Title: "Empros", ReleaseType: "lp", ReleaseDate: releaseDayAgo(10)}
	s.Require().NoError(s.db.Create(&pre).Error)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{
		ArtistID: artistID, ReleaseID: pre.ID, Role: "main"}).Error)

	// A back-catalogue import through the funnel, dated and year-only.
	year := 2011
	s.createRelease("Empros", stringPtr("2011-10-25"), &year, artistID)
	old := 2008
	s.createRelease("Station", nil, &old, artistID)
	// Undated records fail closed.
	s.createRelease("Untitled", nil, nil, artistID)

	s.Equal(int64(0), s.releaseBatchMembers(artistID))
	s.Equal(0, s.flushReleaseDigestsNow())
	s.Equal(int64(0), s.releaseDigestRows(userID, notificationm.NotificationChannelInApp))
}

func (s *NotificationFilterSuite) TestReleaseDigest_KillSwitchStopsAccrual() {
	s.T().Setenv(notificationm.ArtistReleaseAlertsDisableFlag, "1")
	userID := s.createTestUser()
	artistID := s.createTestArtist("Russian Circles")
	s.followArtistWithAlerts(userID, artistID, `{}`)

	s.createRelease("Gnosis", releaseDayAgo(3), nil, artistID)
	s.Equal(int64(0), s.releaseBatchMembers(artistID))
}

func (s *NotificationFilterSuite) TestReleaseDigest_InboxRowCarriesTheWeeksReleases() {
	userID := s.createTestUser()
	oneida := s.createTestArtist("Oneida")
	chatPile := s.createTestArtist("Chat Pile")
	stranger := s.createTestArtist("Not Followed")
	s.followArtistWithAlerts(userID, oneida, `{}`)
	s.followArtistWithAlerts(userID, chatPile, `{}`)
	// Somebody else follows the third artist, so its release accrues into the
	// same week without being this reader's to hear about.
	other := s.createTestUser()
	s.followArtistWithAlerts(other, stranger, `{}`)

	s.createRelease("Success", releaseDayAgo(3), nil, oneida)
	s.createRelease("Tenkiller", releaseDayAgo(2), nil, chatPile)
	s.createRelease("Elsewhere", releaseDayAgo(2), nil, stranger)
	s.Equal(1, s.flushReleaseDigestsNow())

	entries := s.releaseDigestEntries(userID)
	s.Require().Len(entries, 1, "one roundup per reader per week")
	e := entries[0]
	s.Equal(2, e.AlertReleaseCount)
	s.Equal(2, e.AlertReleaseArtistCount)
	s.Equal("Chat Pile, Oneida", e.AlertReleaseArtists, "name order, followed artists only")
	s.Empty(e.AlertReleaseURL, "a multi-record roundup has no single destination")
	s.Equal(currentAlertWeek(), e.AlertBucket)

	solo := s.releaseDigestEntries(other)
	s.Require().Len(solo, 1)
	s.Equal(1, solo[0].AlertReleaseCount)
	s.Contains(solo[0].AlertReleaseURL, "/releases/")
}

// A release credited to two artists the reader follows is one record, counted
// once.
func (s *NotificationFilterSuite) TestReleaseDigest_SplitReleaseCountsOnce() {
	userID := s.createTestUser()
	a := s.createTestArtist("Oneida")
	b := s.createTestArtist("Chat Pile")
	s.followArtistWithAlerts(userID, a, `{}`)
	s.followArtistWithAlerts(userID, b, `{}`)

	s.createRelease("Split", releaseDayAgo(3), nil, a, b)
	s.Equal(1, s.flushReleaseDigestsNow())

	entries := s.releaseDigestEntries(userID)
	s.Require().Len(entries, 1)
	s.Equal(1, entries[0].AlertReleaseCount)
	s.Equal(2, entries[0].AlertReleaseArtistCount, "both followed artists put it out")
	s.Equal("Chat Pile, Oneida", entries[0].AlertReleaseArtists)
	s.NotEmpty(entries[0].AlertReleaseURL, "one record has a single destination")
}

// The dispatch stamp is bounded by the member set the flush read, never by a
// clock, so a member that commits after the read keeps the week selectable.
func (s *NotificationFilterSuite) TestReleaseDigest_OnlyResolvedMembersAreRetired() {
	userID := s.createTestUser()
	a := s.createTestArtist("Oneida")
	b := s.createTestArtist("Chat Pile")
	s.followArtistWithAlerts(userID, a, `{}`)
	s.followArtistWithAlerts(userID, b, `{}`)
	s.createRelease("Success", releaseDayAgo(3), nil, a)
	week := currentAlertWeek()

	batch, err := s.svc.loadReleaseDigestWeek(week)
	s.Require().NoError(err)
	s.Require().Len(batch.resolved, 1)

	s.createRelease("Tenkiller", releaseDayAgo(2), nil, b)
	s.svc.markReleaseDigestMembersDispatched(week, batch.resolved)

	var pending int64
	s.Require().NoError(s.db.Table("artist_release_alert_batch").
		Where("alert_week = ?::date AND dispatched_at IS NULL", week).Count(&pending).Error)
	s.Equal(int64(1), pending, "the member the flush never read must stay undispatched")
}

// A record that joins a week after its roundup went out folds into the
// existing row: no second bell entry, and the row grows.
func (s *NotificationFilterSuite) TestReleaseDigest_LateMemberFoldsIntoTheDeliveredRow() {
	userID := s.createTestUser()
	a := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, a, `{}`)
	s.createRelease("Success", releaseDayAgo(3), nil, a)
	s.Equal(1, s.flushReleaseDigestsNow())

	s.createRelease("Rated O", releaseDayAgo(1), nil, a)
	s.Equal(1, s.flushReleaseDigestsNow())

	s.Equal(int64(1), s.releaseDigestRows(userID, notificationm.NotificationChannelInApp))
	entries := s.releaseDigestEntries(userID)
	s.Require().Len(entries, 1)
	s.Equal(2, entries[0].AlertReleaseCount)
}

// Pins the boundary days of the announceability window as the inbox applies it.
// Members are seeded straight into the batch, bypassing accrual's own fence, at
// each side of every boundary, and the inbox must count exactly the ones the
// predicate admits.
func (s *NotificationFilterSuite) TestReleaseDigest_InboxEnrichmentAgreesWithReleaseAnnounceable() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{}`)
	week := currentAlertWeek()

	now := time.Now()
	oldest, latest, minYear, maxYear := catalogm.ReleaseAnnounceableBounds(now)
	shiftDay := func(d string, n int) *string {
		t, err := time.Parse("2006-01-02", d)
		s.Require().NoError(err)
		out := t.AddDate(0, 0, n).Format("2006-01-02")
		return &out
	}
	yearPtr := func(y int) *int { return &y }
	candidates := []catalogm.Release{
		{Title: "oldest day", ReleaseDate: &oldest},
		{Title: "day before oldest", ReleaseDate: shiftDay(oldest, -1)},
		{Title: "latest day", ReleaseDate: &latest},
		{Title: "day after latest", ReleaseDate: shiftDay(latest, 1)},
		{Title: "current year", ReleaseYear: yearPtr(minYear)},
		{Title: "last year", ReleaseYear: yearPtr(minYear - 1)},
		{Title: "past the latest year", ReleaseYear: yearPtr(maxYear + 1)},
		{Title: "undated"},
	}

	want := 0
	for i := range candidates {
		rel := &candidates[i]
		rel.ReleaseType = "lp"
		s.Require().NoError(s.db.Create(rel).Error)
		s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{
			ArtistID: artistID, ReleaseID: rel.ID, Role: "main"}).Error)
		s.Require().NoError(s.db.Exec(`
			INSERT INTO artist_release_alert_batch (artist_id, release_id, alert_week)
			VALUES (?, ?, ?::date)`, artistID, rel.ID, week).Error)
		if ok, _ := catalogm.ReleaseAnnounceable(rel, now); ok {
			want++
		}
	}
	s.Require().NoError(s.db.Exec(`
		INSERT INTO notification_log (user_id, entity_type, entity_id, channel, sent_at, alert_bucket)
		VALUES (?, ?, ?, ?, now(), ?::date)`,
		userID, notificationm.NotificationEntityArtistReleaseDigest, userID,
		notificationm.NotificationChannelInApp, week).Error)

	entries := s.releaseDigestEntries(userID)
	s.Require().Len(entries, 1)
	s.Equal(want, entries[0].AlertReleaseCount)
	s.Equal(3, want, "oldest day, latest day and current year are the announceable candidates")
}

// seedAccruedMember writes one batch membership as accrual would have written it
// at accruedAt, bypassing the create funnel so a test can place it in the past.
func (s *NotificationFilterSuite) seedAccruedMember(artistID, releaseID uint, accruedAt time.Time) string {
	week := catalog.AlertWeekStart(accruedAt).Format(time.DateOnly)
	s.Require().NoError(s.db.Exec(`
		INSERT INTO artist_release_alert_batch (artist_id, release_id, alert_week, created_at)
		VALUES (?, ?, ?::date, ?)`, artistID, releaseID, week, accruedAt).Error)
	return week
}

func (s *NotificationFilterSuite) seedCreditedRelease(title string, artistID uint, date *string, year *int) uint {
	rel := catalogm.Release{Title: title, ReleaseType: "lp", ReleaseDate: date, ReleaseYear: year}
	s.Require().NoError(s.db.Create(&rel).Error)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{
		ArtistID: artistID, ReleaseID: rel.ID, Role: "main"}).Error)
	return rel.ID
}

// Announceability is judged as of accrual. A week delivered long ago keeps its
// row contents, and a week flushed late (a backlog, a year boundary) still
// delivers what accrued while it was new.
func (s *NotificationFilterSuite) TestReleaseDigest_JudgedAsOfAccrualNotToday() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{}`)

	// Late in December of last year: the year-only release carries last year,
	// which today's calendar rejects, and the dated one is past the recency
	// window today. Both were new when they accrued.
	accruedAt := time.Date(time.Now().UTC().Year()-1, time.December, 30, 12, 0, 0, 0, time.UTC)
	dated := accruedAt.AddDate(0, 0, -3).Format(time.DateOnly)
	yearOnly := accruedAt.Year()
	datedID := s.seedCreditedRelease("Dated", artistID, &dated, nil)
	yearID := s.seedCreditedRelease("Year Only", artistID, nil, &yearOnly)
	s.seedAccruedMember(artistID, datedID, accruedAt)
	week := s.seedAccruedMember(artistID, yearID, accruedAt)

	batch, err := s.svc.loadReleaseDigestWeek(week)
	s.Require().NoError(err)
	s.Len(batch.releases, 2, "the flush must deliver what was new when it accrued")

	s.Equal(1, s.flushReleaseDigestsNow())
	entries := s.releaseDigestEntries(userID)
	s.Require().Len(entries, 1)
	s.Equal(week, entries[0].AlertBucket)
	s.Equal(2, entries[0].AlertReleaseCount, "a delivered row must not empty out as time passes")
}

// A member whose credit was removed is read, skipped and retired. Left pending
// it would re-select its week on every tick and starve later weeks.
func (s *NotificationFilterSuite) TestReleaseDigest_UncreditedMemberIsRetired() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{}`)
	releaseID := s.createRelease("Success", releaseDayAgo(3), nil, artistID)
	s.Require().NoError(s.db.Exec(
		"DELETE FROM artist_releases WHERE artist_id = ? AND release_id = ?", artistID, releaseID).Error)

	s.Equal(1, s.flushReleaseDigestsNow())

	var pending int64
	s.Require().NoError(s.db.Table("artist_release_alert_batch").
		Where("dispatched_at IS NULL").Count(&pending).Error)
	s.Equal(int64(0), pending, "the uncredited member must be stamped dispatched")
	s.Equal(int64(0), s.releaseDigestRows(userID, notificationm.NotificationChannelInApp))
}

// The inbox row names only artists the reader has in-app release alerts on for,
// matching what the flush delivered.
func (s *NotificationFilterSuite) TestReleaseDigest_InboxHonoursPerArtistOptOut() {
	userID := s.createTestUser()
	oneida := s.createTestArtist("Oneida")
	chatPile := s.createTestArtist("Chat Pile")
	s.followArtistWithAlerts(userID, oneida, `{}`)
	s.followArtistWithAlerts(userID, chatPile, `{"alerts":{"releases":{"enabled":false}}}`)

	s.createRelease("Success", releaseDayAgo(3), nil, oneida)
	s.createRelease("Tenkiller", releaseDayAgo(2), nil, chatPile)
	s.Equal(1, s.flushReleaseDigestsNow())

	entries := s.releaseDigestEntries(userID)
	s.Require().Len(entries, 1)
	s.Equal(1, entries[0].AlertReleaseCount)
	s.Equal("Oneida", entries[0].AlertReleaseArtists)
}

// A closed account keeps its follows through its grace period and receives
// nothing.
func (s *NotificationFilterSuite) TestReleaseDigest_ClosedAccountReceivesNothing() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{}`)
	s.Require().NoError(s.db.Exec(
		"UPDATE users SET is_active = false, deleted_at = now() WHERE id = ?", userID).Error)

	s.createRelease("Success", releaseDayAgo(3), nil, artistID)
	s.flushReleaseDigestsNow()

	s.Equal(int64(0), s.releaseDigestRows(userID, notificationm.NotificationChannelInApp))
}

// The inbox applies the reader's current preferences: an account-level
// release-alert opt-out after delivery empties the row, which keeps its place
// with no counts.
func (s *NotificationFilterSuite) TestReleaseDigest_LaterOptOutNarrowsDeliveredRow() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtistWithAlerts(userID, artistID, `{}`)
	s.createRelease("Success", releaseDayAgo(3), nil, artistID)
	s.Equal(1, s.flushReleaseDigestsNow())
	s.Require().Equal(1, s.releaseDigestEntries(userID)[0].AlertReleaseCount)

	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_preferences (user_id, alert_defaults)
		VALUES (?, '{"releases":{"in_app":false,"email":false}}'::jsonb)
		ON CONFLICT (user_id) DO UPDATE SET alert_defaults = EXCLUDED.alert_defaults`, userID).Error)

	entries := s.releaseDigestEntries(userID)
	s.Require().Len(entries, 1, "the delivered row keeps its place")
	s.Equal(0, entries[0].AlertReleaseCount)
	s.Empty(entries[0].AlertReleaseArtists)
}
