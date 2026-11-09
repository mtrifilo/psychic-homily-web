package engagement

import (
	"context"
	"testing"
	"time"

	engagementm "psychic-homily-backend/internal/models/engagement"
)

func TestClassifyCalendarFeedClient(t *testing.T) {
	cases := []struct {
		name       string
		userAgent  string
		wantClient engagementm.CalendarFeedClient
		wantRead   bool
	}{
		{"Google Calendar importer", "Google-Calendar-Importer", engagementm.CalendarFeedClientGoogle, true},
		{"Google importer inside a longer string", "Mozilla/5.0 (compatible; Google-Calendar-Importer)", engagementm.CalendarFeedClientGoogle, true},
		{"Apple iOS fetcher", "iOS/17.5.1 (21F90) dataaccessd/1.0", engagementm.CalendarFeedClientAppleIOS, true},
		{"Apple iPadOS fetcher", "iPadOS/17.5.1 (21F90) dataaccessd/1.0", engagementm.CalendarFeedClientAppleIOS, true},
		{"Apple macOS fetcher", "macOS/26.2 (25C56) dataaccessd/1.0", engagementm.CalendarFeedClientAppleMac, true},
		{"macOS CalendarAgent", "CalendarAgent/954 CFNetwork/1410.0.3 Darwin/22.4.0", engagementm.CalendarFeedClientAppleMac, true},
		{"iOS without dataaccessd is not Apple iOS", "iOS/17.5.1 (21F90) SomethingElse/1.0", engagementm.CalendarFeedClientOther, true},
		{"desktop browser is not a read", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36", "", false},
		{"empty User-Agent", "", engagementm.CalendarFeedClientOther, true},
		{"unknown client", "Thunderbird-Calendar/1.0", engagementm.CalendarFeedClientOther, true},
		{"curl", "curl/8.7.1", engagementm.CalendarFeedClientOther, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client, read := classifyCalendarFeedClient(c.userAgent)
			if read != c.wantRead || client != c.wantClient {
				t.Errorf("classifyCalendarFeedClient(%q) = (%q, %v), want (%q, %v)",
					c.userAgent, client, read, c.wantClient, c.wantRead)
			}
		})
	}
}

// --- Over a real database ---

const (
	googleUA  = "Google-Calendar-Importer"
	appleUA   = "iOS/17.5.1 (21F90) dataaccessd/1.0"
	browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15"
)

// feedReadRows returns the recorded reads of userID's current token.
func (suite *CalendarIntegrationTestSuite) feedReadRows(userID uint) []engagementm.CalendarFeedRead {
	suite.T().Helper()
	var rows []engagementm.CalendarFeedRead
	suite.Require().NoError(suite.db.
		Joins("JOIN calendar_tokens ON calendar_tokens.id = calendar_feed_reads.token_id").
		Where("calendar_tokens.user_id = ?", userID).
		Order("client").
		Find(&rows).Error)
	return rows
}

// allFeedReadRows counts every recorded read, whichever token it belongs to.
func (suite *CalendarIntegrationTestSuite) allFeedReadRows() int64 {
	suite.T().Helper()
	var n int64
	suite.Require().NoError(suite.db.Model(&engagementm.CalendarFeedRead{}).Count(&n).Error)
	return n
}

// mintFeedToken mints userID a feed token and returns its plaintext.
func (suite *CalendarIntegrationTestSuite) mintFeedToken(userID uint) string {
	suite.T().Helper()
	minted, err := suite.svc.CreateToken(userID, nil, "http://localhost:8080")
	suite.Require().NoError(err)
	return minted.Token
}

// linkReset is userID's link-reset marker as the status read sees it.
func (suite *CalendarIntegrationTestSuite) linkReset(userID uint) (*time.Time, *engagementm.CalendarLinkResetReason) {
	suite.T().Helper()
	at, reason, err := suite.svc.linkResetMarker(userID)
	suite.Require().NoError(err)
	return at, reason
}

func (suite *CalendarIntegrationTestSuite) markLinkReset(userID uint, at time.Time, reason engagementm.CalendarLinkResetReason) {
	suite.T().Helper()
	suite.Require().NoError(MarkCalendarLinkResetTx(suite.db, userID, at, reason))
}

func (suite *CalendarIntegrationTestSuite) TestRecordFeedRead_CalendarClientUpsertsOneRow() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	wrote, err := suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at)
	suite.Require().NoError(err)
	suite.True(wrote)

	rows := suite.feedReadRows(user.ID)
	suite.Require().Len(rows, 1)
	suite.Equal(engagementm.CalendarFeedClientGoogle, rows[0].Client)
	suite.True(rows[0].FirstReadAt.Equal(at), "first_read_at = %v", rows[0].FirstReadAt)
	suite.True(rows[0].LastReadAt.Equal(at), "last_read_at = %v", rows[0].LastReadAt)
}

func (suite *CalendarIntegrationTestSuite) TestRecordFeedRead_SecondReadInsideTheIntervalDoesNotWrite() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	_, err := suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at)
	suite.Require().NoError(err)

	wrote, err := suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at.Add(feedReadRecordInterval-time.Second))
	suite.Require().NoError(err)
	suite.False(wrote, "a read inside the interval writes nothing")
	rows := suite.feedReadRows(user.ID)
	suite.Require().Len(rows, 1)
	suite.True(rows[0].LastReadAt.Equal(at), "last_read_at stays %v, got %v", at, rows[0].LastReadAt)

	later := at.Add(feedReadRecordInterval)
	wrote, err = suite.svc.recordFeedReadAt(context.Background(), token, googleUA, later)
	suite.Require().NoError(err)
	suite.True(wrote, "a read once the interval has passed is recorded")
	rows = suite.feedReadRows(user.ID)
	suite.Require().Len(rows, 1)
	suite.True(rows[0].FirstReadAt.Equal(at), "first_read_at stays %v, got %v", at, rows[0].FirstReadAt)
	suite.True(rows[0].LastReadAt.Equal(later), "last_read_at moves to %v, got %v", later, rows[0].LastReadAt)

	wrote, err = suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at)
	suite.Require().NoError(err)
	suite.False(wrote, "an earlier instant never moves last_read_at back")
}

func (suite *CalendarIntegrationTestSuite) TestRecordFeedRead_EachClientFamilyHasItsOwnRow() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	for _, ua := range []string{googleUA, appleUA, ""} {
		wrote, err := suite.svc.recordFeedReadAt(context.Background(), token, ua, at)
		suite.Require().NoError(err)
		suite.True(wrote, "first read by %q", ua)
	}

	rows := suite.feedReadRows(user.ID)
	suite.Require().Len(rows, 3)
	suite.Equal(engagementm.CalendarFeedClientAppleIOS, rows[0].Client)
	suite.Equal(engagementm.CalendarFeedClientGoogle, rows[1].Client)
	suite.Equal(engagementm.CalendarFeedClientOther, rows[2].Client)
}

func (suite *CalendarIntegrationTestSuite) TestRecordFeedRead_BrowserWritesNothing() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)

	suite.Require().NoError(suite.svc.RecordFeedRead(context.Background(), token, browserUA))

	suite.Empty(suite.feedReadRows(user.ID))
}

func (suite *CalendarIntegrationTestSuite) TestRecordFeedRead_UnknownTokenWritesNothing() {
	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)

	wrote, err := suite.svc.recordFeedReadAt(context.Background(), "phcal_not_a_token", googleUA, time.Now())
	suite.Require().NoError(err)
	suite.False(wrote)
	suite.Zero(suite.allFeedReadRows())
}

func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_ListsReadsMostRecentFirst() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	_, err := suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at)
	suite.Require().NoError(err)
	_, err = suite.svc.recordFeedReadAt(context.Background(), token, appleUA, at.Add(time.Minute))
	suite.Require().NoError(err)

	status, err := suite.svc.GetTokenStatus(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	suite.Require().Len(status.Reads, 2)
	suite.Equal(engagementm.CalendarFeedClientAppleIOS, status.Reads[0].Client)
	suite.True(status.Reads[0].LastReadAt.Equal(at.Add(time.Minute)))
	suite.Equal(engagementm.CalendarFeedClientGoogle, status.Reads[1].Client)
	suite.True(status.Reads[1].FirstReadAt.Equal(at))
	suite.Nil(status.LinkResetAt)
	suite.Nil(status.LinkResetReason)
}

func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_NoReadsYet() {
	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)

	status, err := suite.svc.GetTokenStatus(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.True(status.HasToken)
	suite.Nil(status.Reads)
}

// Replacing the token deletes the old token's reads with it.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_ReplacingTheTokenRemovesItsReads() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.RecordFeedRead(context.Background(), token, googleUA))
	suite.Require().Len(suite.feedReadRows(user.ID), 1)

	suite.mintFeedToken(user.ID)

	suite.Zero(suite.allFeedReadRows())
	status, err := suite.svc.GetTokenStatus(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.Empty(status.Reads)
}

func (suite *CalendarIntegrationTestSuite) TestDeleteCalendarTokensTx_RemovesItsReads() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.RecordFeedRead(context.Background(), token, appleUA))

	deleted, err := DeleteCalendarTokensTx(suite.db, user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), deleted)

	suite.Zero(suite.allFeedReadRows())
}

// --- The link-reset marker ---

func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_NoTokenCarriesTheLinkReset() {
	user := suite.createTestUser(true)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	suite.markLinkReset(user.ID, at, engagementm.CalendarLinkResetPasswordReset)

	status, err := suite.svc.GetTokenStatus(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.False(status.HasToken)
	suite.Require().NotNil(status.LinkResetAt)
	suite.True(status.LinkResetAt.Equal(at))
	suite.Require().NotNil(status.LinkResetReason)
	suite.Equal(engagementm.CalendarLinkResetPasswordReset, *status.LinkResetReason)
}

// A user with no preferences row reads as having no marker.
func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_NoPreferencesRowNoLinkReset() {
	user := suite.createTestUser(true)
	var prefs int64
	suite.Require().NoError(suite.db.Table("user_preferences").Where("user_id = ?", user.ID).Count(&prefs).Error)
	suite.Require().Zero(prefs, "fixture: the user has no preferences row")

	status, err := suite.svc.GetTokenStatus(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.Nil(status.LinkResetAt)
	suite.Nil(status.LinkResetReason)
}

// The marker is written for a user who has no preferences row yet, and
// overwrites an earlier one.
func (suite *CalendarIntegrationTestSuite) TestMarkCalendarLinkResetTx_CreatesOrOverwrites() {
	user := suite.createTestUser(true)
	first := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	suite.markLinkReset(user.ID, first, engagementm.CalendarLinkResetSignOutEverywhere)

	at, reason := suite.linkReset(user.ID)
	suite.Require().NotNil(at)
	suite.True(at.Equal(first))
	suite.Equal(engagementm.CalendarLinkResetSignOutEverywhere, *reason)

	second := first.Add(time.Hour)
	suite.markLinkReset(user.ID, second, engagementm.CalendarLinkResetPasswordReset)
	at, reason = suite.linkReset(user.ID)
	suite.True(at.Equal(second))
	suite.Equal(engagementm.CalendarLinkResetPasswordReset, *reason)
}

func (suite *CalendarIntegrationTestSuite) TestCreateToken_ClearsTheLinkReset() {
	user := suite.createTestUser(true)
	suite.markLinkReset(user.ID, time.Now(), engagementm.CalendarLinkResetSignOutEverywhere)

	suite.mintFeedToken(user.ID)

	at, reason := suite.linkReset(user.ID)
	suite.Nil(at)
	suite.Nil(reason)
}

func (suite *CalendarIntegrationTestSuite) TestDeleteToken_ClearsTheLinkReset() {
	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)
	// A marker beside a live token is not reachable through the services; it
	// is planted here so the clear is observable.
	suite.markLinkReset(user.ID, time.Now(), engagementm.CalendarLinkResetPasswordReset)

	suite.Require().NoError(suite.svc.DeleteToken(user.ID))

	at, reason := suite.linkReset(user.ID)
	suite.Nil(at)
	suite.Nil(reason)
}

// With no token, turning the feed off fails and changes nothing, the marker
// included.
func (suite *CalendarIntegrationTestSuite) TestDeleteToken_NoTokenLeavesTheLinkReset() {
	user := suite.createTestUser(true)
	suite.markLinkReset(user.ID, time.Now(), engagementm.CalendarLinkResetPasswordReset)

	suite.Require().ErrorIs(suite.svc.DeleteToken(user.ID), errNoCalendarToken)

	at, _ := suite.linkReset(user.ID)
	suite.NotNil(at)
}

// The CHECK constraints admit only the known families and reasons, and only a
// complete marker.
func (suite *CalendarIntegrationTestSuite) TestFeedReadAndLinkResetConstraints() {
	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)
	var tokenID uint
	suite.Require().NoError(suite.db.Table("calendar_tokens").Where("user_id = ?", user.ID).Pluck("id", &tokenID).Error)

	suite.Error(suite.db.Exec(
		"INSERT INTO calendar_feed_reads (token_id, client, first_read_at, last_read_at) VALUES (?, 'outlook', NOW(), NOW())",
		tokenID).Error, "an unknown client family is refused")
	suite.Error(MarkCalendarLinkResetTx(suite.db, user.ID, time.Now(), "logged_out"), "an unknown reason is refused")
	suite.Error(suite.db.Exec(
		"INSERT INTO user_preferences (user_id, calendar_link_reset_at) VALUES (?, NOW())",
		user.ID).Error, "a marker without a reason is refused")
}
