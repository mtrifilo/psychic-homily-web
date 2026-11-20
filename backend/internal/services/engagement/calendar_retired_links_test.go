package engagement

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	engagementm "psychic-homily-backend/internal/models/engagement"
)

// The owner-approved wording, spelled out here rather than read from
// retiredLinkWordings, so a change to the text fails these tests.
var wantRetiredWording = map[engagementm.CalendarLinkRetiredReason]retiredLinkWording{
	engagementm.CalendarLinkRetiredReset: {
		"Psychic Homily: this calendar link was reset. Add it again from your Library.",
		"This feed link was reset",
		"Add it again from Settings on Psychic Homily.",
	},
	engagementm.CalendarLinkRetiredReplaced: {
		"Psychic Homily: this calendar link was replaced. Add the new one from your Library.",
		"This feed link was replaced",
		"Copy the new one from Settings on Psychic Homily.",
	},
	engagementm.CalendarLinkRetiredTurnedOff: {
		"Psychic Homily: this calendar link was turned off. Turn it on again from your Library.",
		"This feed link was turned off",
		"Turn it on again from Settings on Psychic Homily.",
	},
}

const day = 24 * time.Hour

// Every reason has its wording, verbatim, and there is no wording for a
// reason the model does not name.
func TestRetiredLinkWordings_CoverEveryReasonVerbatim(t *testing.T) {
	assert.Equal(t, wantRetiredWording, retiredLinkWordings)
	assert.Len(t, retiredLinkWordings, len(engagementm.CalendarLinkRetiredReasons))
}

// phoenixDate is t's date where the notice reads it, as an RFC 5545 DATE.
func phoenixDate(t *testing.T, at time.Time) string {
	t.Helper()
	loc, err := time.LoadLocation("America/Phoenix")
	require.NoError(t, err)
	return at.In(loc).Format("20060102")
}

// noticeEventLines is the content lines of the notice's single VEVENT.
func noticeEventLines(t *testing.T, feed []byte) map[string]string {
	t.Helper()
	props := map[string]string{}
	for _, line := range icsEventLines(t, string(feed)) {
		name, value, _ := strings.Cut(line, ":")
		props[name] = value
	}
	return props
}

// The note moves to each fetch's date and keeps one UID; within the 30 days
// its bytes change only when the date does, and each day's copy outranks the
// day before.
func TestBuildRetiredLinkCalendar_OneNotePerDateWithARisingSequence(t *testing.T) {
	retiredAt := time.Date(2026, 11, 20, 18, 30, 15, 0, time.UTC) // 11:30 in Phoenix
	link := &engagementm.RetiredCalendarLink{ID: 42, TokenHash: strings.Repeat("ab", 32), Reason: engagementm.CalendarLinkRetiredReplaced, RetiredAt: retiredAt}

	sameDayLater := buildRetiredLinkCalendar(link, retiredAt.Add(10*time.Hour)) // 21:30 Phoenix, Nov 20
	assert.Equal(t, buildRetiredLinkCalendar(link, retiredAt), sameDayLater, "same date, same bytes")

	first := noticeEventLines(t, buildRetiredLinkCalendar(link, retiredAt))
	assert.Regexp(t, `^retired-link-[0-9a-f]{32}@psychichomily\.com$`, first["UID"], "derived from the hash, not the row id")
	assert.NotContains(t, first["UID"], link.TokenHash)
	assert.Equal(t, "0", first["SEQUENCE"])
	assert.Equal(t, "20261120T183015Z", first["DTSTAMP"], "day 0 is stamped at the retirement")
	assert.Equal(t, "20261120", first["DTSTART;VALUE=DATE"])
	assert.Equal(t, "20261121", first["DTEND;VALUE=DATE"])
	assert.Equal(t, "TRANSPARENT", first["TRANSP"])

	// 06:59 UTC on Nov 21 is still Nov 20 in Phoenix.
	assert.Equal(t, first, noticeEventLines(t, buildRetiredLinkCalendar(link, time.Date(2026, 11, 21, 6, 59, 0, 0, time.UTC))))

	next := noticeEventLines(t, buildRetiredLinkCalendar(link, time.Date(2026, 11, 21, 7, 0, 0, 0, time.UTC)))
	assert.Equal(t, first["UID"], next["UID"])
	assert.Equal(t, "1", next["SEQUENCE"])
	assert.Equal(t, "20261121T070000Z", next["DTSTAMP"], "later days are stamped at the start of the date")
	assert.Equal(t, "20261121", next["DTSTART;VALUE=DATE"])
	assert.Equal(t, "20261122", next["DTEND;VALUE=DATE"])
}

func TestBuildRetiredLinkCalendar_NoteEndsAtThirtyDays(t *testing.T) {
	retiredAt := time.Date(2026, 11, 20, 18, 30, 15, 0, time.UTC)
	link := &engagementm.RetiredCalendarLink{ID: 1, Reason: engagementm.CalendarLinkRetiredReset, RetiredAt: retiredAt}

	assertRFC5545Wire(t, buildRetiredLinkCalendar(link, retiredAt.Add(retiredLinkNoteFor)), 1)
	assertRFC5545Wire(t, buildRetiredLinkCalendar(link, retiredAt.Add(retiredLinkNoteFor+time.Microsecond)), 0)
}

// --- Over a real database ---

// retirement is one way a feed link stops working, through the path the site
// takes for it.
type retirement struct {
	name   string
	reason engagementm.CalendarLinkRetiredReason
	retire func(userID uint)
}

func (suite *CalendarIntegrationTestSuite) retirements() []retirement {
	revoke := func(reason engagementm.CalendarLinkResetReason) func(uint) {
		return func(userID uint) {
			suite.Require().NoError(suite.db.Transaction(func(tx *gorm.DB) error {
				_, err := RevokeCalendarLinkTx(tx, userID, time.Now(), reason)
				return err
			}))
		}
	}
	return []retirement{
		{"New link", engagementm.CalendarLinkRetiredReplaced, func(userID uint) { suite.mintFeedToken(userID) }},
		{"Turn off", engagementm.CalendarLinkRetiredTurnedOff, func(userID uint) { suite.Require().NoError(suite.svc.DeleteToken(userID)) }},
		{"sign out everywhere", engagementm.CalendarLinkRetiredReset, revoke(engagementm.CalendarLinkResetSignOutEverywhere)},
		{"password reset", engagementm.CalendarLinkRetiredReset, revoke(engagementm.CalendarLinkResetPasswordReset)},
	}
}

// retiredRow is the remembered row for plainToken, failing when there is none.
func (suite *CalendarIntegrationTestSuite) retiredRow(plainToken string) engagementm.RetiredCalendarLink {
	suite.T().Helper()
	var row engagementm.RetiredCalendarLink
	suite.Require().NoError(suite.db.Where("token_hash = ?", hashCalendarToken(plainToken)).Take(&row).Error)
	return row
}

func (suite *CalendarIntegrationTestSuite) retiredRowCount() int64 {
	suite.T().Helper()
	var n int64
	suite.Require().NoError(suite.db.Model(&engagementm.RetiredCalendarLink{}).Count(&n).Error)
	return n
}

// Every way a link stops while its owner keeps the account remembers the old
// address with its reason and refuses it as a credential.
func (suite *CalendarIntegrationTestSuite) TestRetirement_RemembersTheAddressWithItsReason() {
	for _, r := range suite.retirements() {
		suite.Run(r.name, func() {
			user := suite.createTestUser(true)
			old := suite.mintFeedToken(user.ID)

			r.retire(user.ID)

			suite.Equal(r.reason, suite.retiredRow(old).Reason)
			_, err := suite.svc.ValidateCalendarToken(old)
			suite.Error(err, "a retired link is refused")
		})
	}
}

// The row is the token's hash, the reason and the instant: no account, no
// show, no plaintext.
func (suite *CalendarIntegrationTestSuite) TestRetiredCalendarLinks_HoldNothingAboutTheAccount() {
	var columns []string
	suite.Require().NoError(suite.db.Raw(`
		SELECT column_name FROM information_schema.columns
		WHERE table_name = 'retired_calendar_links' ORDER BY ordinal_position`).Scan(&columns).Error)
	suite.Equal([]string{"id", "token_hash", "reason", "retired_at"}, columns)

	user := suite.createTestUser(true)
	old := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.DeleteToken(user.ID))

	row := suite.retiredRow(old)
	suite.Equal(hashCalendarToken(old), row.TokenHash)
	suite.NotContains(row.TokenHash, strings.TrimPrefix(old, CalendarTokenPrefix))
}

func (suite *CalendarIntegrationTestSuite) TestRetireCalendarTokensTx_StoresTheInstantToTheMicrosecond() {
	user := suite.createTestUser(true)
	old := suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 20, 18, 30, 15, 123456789, time.UTC)

	deleted, err := RetireCalendarTokensTx(suite.db, user.ID, engagementm.CalendarLinkRetiredTurnedOff, at)
	suite.Require().NoError(err)
	suite.Equal(int64(1), deleted)

	suite.True(suite.retiredRow(old).RetiredAt.Equal(at.Truncate(time.Microsecond)),
		"retired_at = %v, want %v", suite.retiredRow(old).RetiredAt, at.Truncate(time.Microsecond))
}

// The calendar answer for each reason, driven from the stored retirement
// instant: the note on the fetch's date on days 0 and 29, a valid empty
// calendar on day 31, and nothing remembered on day 366.
func (suite *CalendarIntegrationTestSuite) TestRetiredSavedShowsNotice_ThirtyDaysOfNoteThenEmptyThenUnknown() {
	for _, r := range suite.retirements() {
		suite.Run(r.name, func() {
			t := suite.T()
			user := suite.createTestUser(true)
			old := suite.mintFeedToken(user.ID)
			r.retire(user.ID)
			row := suite.retiredRow(old)

			for _, n := range []int{0, 29} {
				now := row.RetiredAt.Add(time.Duration(n) * day)
				notice, err := suite.svc.retiredNoticeAt(old, now, renderRetiredSavedShows)
				suite.Require().NoError(err)
				suite.Require().NotNil(notice, "day %d", n)
				assertRFC5545Wire(t, notice.Body, 1)

				props := noticeEventLines(t, notice.Body)
				suite.Equal(wantRetiredWording[r.reason].calendarNote, props["SUMMARY"], "day %d", n)
				suite.Equal(phoenixDate(t, now), props["DTSTART;VALUE=DATE"], "day %d", n)
				suite.Equal(retiredLinkNoticeUID(&row), props["UID"])
				suite.NotEmpty(props["DTSTAMP"])
				suite.Contains(string(notice.Body), "\r\nX-WR-CALNAME:Psychic Homily saved shows\r\n")
				suite.Contains(string(notice.Body), "\r\nNAME:Psychic Homily saved shows\r\n")
				suite.NotEmpty(notice.ETag)
			}

			empty, err := suite.svc.retiredNoticeAt(old, row.RetiredAt.Add(31*day), renderRetiredSavedShows)
			suite.Require().NoError(err)
			suite.Require().NotNil(empty, "day 31 is still remembered")
			assertRFC5545Wire(t, empty.Body, 0)
			suite.Contains(string(empty.Body), "\r\nX-WR-CALNAME:Psychic Homily saved shows\r\n")

			gone, err := suite.svc.retiredNoticeAt(old, row.RetiredAt.Add(366*day), renderRetiredSavedShows)
			suite.Require().NoError(err)
			suite.Nil(gone, "day 366 is not remembered")
		})
	}
}

// parsedAtom is the part of an Atom document these tests read.
type parsedAtom struct {
	XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Updated string   `xml:"updated"`
	Author  struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Entries []struct {
		Title   string `xml:"title"`
		ID      string `xml:"id"`
		Updated string `xml:"updated"`
		Content struct {
			Type string `xml:"type,attr"`
			Text string `xml:",chardata"`
		} `xml:"content"`
	} `xml:"entry"`
}

// The follows answer for each reason: one entry with the exact title and
// body, one id and one date (the retirement) for as long as the link is
// remembered, then nothing.
func (suite *CalendarIntegrationTestSuite) TestRetiredFollowsNotice_OneStableEntryWhileRemembered() {
	for _, r := range suite.retirements() {
		suite.Run(r.name, func() {
			user := suite.createTestUser(true)
			old := suite.mintFeedToken(user.ID)
			r.retire(user.ID)
			row := suite.retiredRow(old)

			first, err := suite.svc.retiredNoticeAt(old, row.RetiredAt, renderRetiredFollows)
			suite.Require().NoError(err)
			suite.Require().NotNil(first)
			suite.True(bytes.HasPrefix(first.Body, []byte(xml.Header)))

			var feed parsedAtom
			suite.Require().NoError(xml.Unmarshal(first.Body, &feed))
			suite.Equal(followsActivityFeedTitle, feed.Title)
			suite.Equal("Psychic Homily", feed.Author.Name)
			suite.NotEmpty(feed.ID)
			suite.Require().Len(feed.Entries, 1)
			entry := feed.Entries[0]
			suite.Equal(wantRetiredWording[r.reason].feedTitle, entry.Title)
			suite.Equal(wantRetiredWording[r.reason].feedBody, entry.Content.Text)
			suite.Equal("text", entry.Content.Type)
			suite.NotEmpty(entry.ID)
			suite.Equal(row.RetiredAt.UTC().Format(time.RFC3339), entry.Updated)
			suite.Equal(entry.Updated, feed.Updated)

			late, err := suite.svc.retiredNoticeAt(old, row.RetiredAt.Add(364*day), renderRetiredFollows)
			suite.Require().NoError(err)
			suite.Require().NotNil(late)
			suite.Equal(first.Body, late.Body, "the entry keeps its id and date")
			suite.Equal(first.ETag, late.ETag)

			gone, err := suite.svc.retiredNoticeAt(old, row.RetiredAt.Add(366*day), renderRetiredFollows)
			suite.Require().NoError(err)
			suite.Nil(gone)
		})
	}
}

// A retired address is accepted nowhere a live token is: the validator
// refuses it (the public-read limiter's exemption asks the same validator),
// a read of it is not recorded, and the account's status holds no token.
func (suite *CalendarIntegrationTestSuite) TestRetiredLink_IsNeverACredential() {
	user := suite.createTestUser(true)
	old := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.DeleteToken(user.ID))
	suite.Require().NotZero(suite.retiredRowCount())

	_, err := suite.svc.ValidateCalendarToken(old)
	suite.Error(err)

	wrote, err := suite.svc.recordFeedReadAt(context.Background(), old, googleUA, time.Now())
	suite.Require().NoError(err)
	suite.False(wrote)
	suite.Zero(suite.allFeedReadRows())

	status, err := suite.svc.GetTokenStatus(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.False(status.HasToken)
}

func (suite *CalendarIntegrationTestSuite) TestRetiredNotices_LiveAndUnknownTokensAreNotRetired() {
	user := suite.createTestUser(true)
	live := suite.mintFeedToken(user.ID)
	for name, token := range map[string]string{"live": live, "unknown": CalendarTokenPrefix + "0000"} {
		calendar, err := suite.svc.RetiredSavedShowsNotice(token)
		suite.Require().NoError(err, name)
		suite.Nil(calendar, name)
		atom, err := suite.svc.RetiredFollowsNotice(token)
		suite.Require().NoError(err, name)
		suite.Nil(atom, name)
	}
}

// The sweep keeps every link retired within a year and a day and deletes the
// older ones, across statements when there are more rows than one batch, and
// retiring a link deletes no other row.
func (suite *CalendarIntegrationTestSuite) TestForgetExpiredRetiredLinks_DeletesOnlyLinksPastTheYear() {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	cutoff := retiredLinkSweepCutoff(now)
	suite.Equal(time.Date(2025, 11, 19, 12, 0, 0, 0, time.UTC), cutoff)
	insert := func(hash string, at time.Time) {
		suite.Require().NoError(suite.db.Exec(
			"INSERT INTO retired_calendar_links (token_hash, reason, retired_at) VALUES (?, 'turned_off', ?)", hash, at).Error)
	}
	insert(strings.Repeat("a", 64), cutoff)
	for i := 0; i < retiredLinkSweepBatch+5; i++ {
		insert(fmt.Sprintf("%064x", i), cutoff.Add(-time.Second))
	}

	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)
	_, err := RetireCalendarTokensTx(suite.db, user.ID, engagementm.CalendarLinkRetiredTurnedOff, now)
	suite.Require().NoError(err)
	suite.Equal(int64(retiredLinkSweepBatch+7), suite.retiredRowCount(), "retiring a link deletes nothing else")

	deleted, err := ForgetExpiredRetiredLinks(context.Background(), suite.db, now)
	suite.Require().NoError(err)
	suite.Equal(int64(retiredLinkSweepBatch+5), deleted)
	suite.Equal(int64(2), suite.retiredRowCount(), "the link at the cutoff and today's are kept")
}

// The sweep never deletes a link a read would still answer for, including
// across a February 29.
func TestRetiredLinkSweepCutoff_NeverPrecedesTheRememberedYear(t *testing.T) {
	start := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	for d := 0; d < 3*366; d++ {
		retiredAt := start.AddDate(0, 0, d)
		link := &engagementm.RetiredCalendarLink{RetiredAt: retiredAt}
		lastRemembered := retiredAt.AddDate(1, 0, 0)
		require.True(t, retiredLinkRemembered(link, lastRemembered), "retired %v", retiredAt)
		require.False(t, retiredAt.Before(retiredLinkSweepCutoff(lastRemembered)),
			"retired %v is swept at %v, while still remembered", retiredAt, lastRemembered)
	}
}

// "A year" is a calendar year, so a link retired the day after a February 29
// is still remembered on the same date a year later.
func (suite *CalendarIntegrationTestSuite) TestRetiredLink_RememberedForACalendarYearAcrossALeapDay() {
	user := suite.createTestUser(true)
	old := suite.mintFeedToken(user.ID)
	retiredAt := time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC)
	_, err := RetireCalendarTokensTx(suite.db, user.ID, engagementm.CalendarLinkRetiredTurnedOff, retiredAt)
	suite.Require().NoError(err)

	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{time.Date(2028, 2, 29, 13, 0, 0, 0, time.UTC), true},
		{time.Date(2028, 3, 1, 12, 0, 0, 0, time.UTC), true},
		{time.Date(2028, 3, 1, 12, 0, 1, 0, time.UTC), false},
	} {
		notice, err := suite.svc.retiredNoticeAt(old, c.at, renderRetiredFollows)
		suite.Require().NoError(err)
		suite.Equal(c.want, notice != nil, "at %v", c.at)
	}
}

// A row with a reason this build has no wording for answers as unknown rather
// than as a note with no text.
func (suite *CalendarIntegrationTestSuite) TestRetiredLink_ReasonWithoutWordingAnswersAsUnknown() {
	user := suite.createTestUser(true)
	old := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.DeleteToken(user.ID))

	saved := retiredLinkWordings
	retiredLinkWordings = map[engagementm.CalendarLinkRetiredReason]retiredLinkWording{}
	defer func() { retiredLinkWordings = saved }()

	for _, render := range []retiredNoticeRenderer{renderRetiredSavedShows, renderRetiredFollows} {
		notice, err := suite.svc.retiredNoticeAt(old, time.Now(), render)
		suite.Require().NoError(err)
		suite.Nil(notice)
	}
}

// Deleting the account deletes the token by cascade and remembers nothing, so
// the address answers as unknown. A closed account keeps its token, which is
// refused as a credential and is not a retired link either.
func (suite *CalendarIntegrationTestSuite) TestAccountDeletion_LeavesNoRetiredLink() {
	deleted := suite.createTestUser(true)
	deletedToken := suite.mintFeedToken(deleted.ID)
	suite.Require().NoError(suite.db.Exec("DELETE FROM users WHERE id = ?", deleted.ID).Error)

	closed := suite.createTestUser(true)
	closedToken := suite.mintFeedToken(closed.ID)
	suite.Require().NoError(suite.db.Exec("UPDATE users SET is_active = false, deleted_at = NOW() WHERE id = ?", closed.ID).Error)

	suite.Zero(suite.retiredRowCount())
	for _, token := range []string{deletedToken, closedToken} {
		_, err := suite.svc.ValidateCalendarToken(token)
		suite.Error(err)
		notice, err := suite.svc.RetiredSavedShowsNotice(token)
		suite.Require().NoError(err)
		suite.Nil(notice)
	}
}

// The CHECK constraint admits exactly the model's reasons: the set it spells
// is read back from the catalog and compared whole.
func (suite *CalendarIntegrationTestSuite) TestRetiredCalendarLinks_ReasonCheckMatchesTheModel() {
	var def string
	suite.Require().NoError(suite.db.Raw(`
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conname = 'retired_calendar_links_reason_check'`).Scan(&def).Error)
	admitted := regexp.MustCompile(`'([a-z_]+)'::text`).FindAllStringSubmatch(def, -1)
	suite.Require().NotEmpty(admitted, "constraint definition: %s", def)

	var got []string
	for _, m := range admitted {
		got = append(got, m[1])
	}
	var want []string
	for _, reason := range engagementm.CalendarLinkRetiredReasons {
		want = append(want, string(reason))
	}
	suite.ElementsMatch(want, got, "constraint definition: %s", def)
}

// runRetiredLinksMigration runs one direction of the retired-links migration.
func (suite *CalendarIntegrationTestSuite) runRetiredLinksMigration(suffix string) {
	suite.T().Helper()
	_, here, _, _ := runtime.Caller(0)
	sqlText, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "db", "migrations",
		"20261120194533_retired_calendar_links"+suffix))
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(string(sqlText)).Error
	}))
}

// The migration goes down cleanly, with tokens and remembered links in
// place, and comes back up.
func (suite *CalendarIntegrationTestSuite) TestRetiredCalendarLinksMigration_DownThenUp() {
	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)
	suite.mintFeedToken(user.ID)
	suite.Require().NotZero(suite.retiredRowCount())

	suite.runRetiredLinksMigration(".down.sql")
	var exists bool
	suite.Require().NoError(suite.db.Raw("SELECT to_regclass('retired_calendar_links') IS NOT NULL").Scan(&exists).Error)
	suite.False(exists)
	suite.runRetiredLinksMigration(".up.sql")

	old := suite.mintFeedToken(user.ID)
	suite.mintFeedToken(user.ID)
	suite.Equal(engagementm.CalendarLinkRetiredReplaced, suite.retiredRow(old).Reason)
}

// golang-ical parses the notice the same way it parses the live feed.
func TestRetiredLinkCalendar_Parses(t *testing.T) {
	link := &engagementm.RetiredCalendarLink{ID: 3, Reason: engagementm.CalendarLinkRetiredTurnedOff, RetiredAt: time.Now()}
	parsed, err := ics.ParseCalendar(bytes.NewReader(buildRetiredLinkCalendar(link, time.Now())))
	require.NoError(t, err)
	require.Len(t, parsed.Events(), 1)
	assert.Equal(t, wantRetiredWording[engagementm.CalendarLinkRetiredTurnedOff].calendarNote,
		parsed.Events()[0].GetProperty(ics.ComponentPropertySummary).Value)
}
