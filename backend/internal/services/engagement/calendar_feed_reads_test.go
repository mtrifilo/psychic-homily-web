package engagement

import (
	"context"
	"testing"
	"time"

	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/testutil"
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
		{"Apple iOS dataaccessd", "iOS/17.5.1 (21F90) dataaccessd/1.0", engagementm.CalendarFeedClientAppleIOS, true},
		{"Apple iOS 26 dataaccessd", "iOS/26.6 (23G71) dataaccessd/1.0", engagementm.CalendarFeedClientAppleIOS, true},
		{"Apple iOS accountsd", "iOS/17.5 (21F79) accountsd/1.0", engagementm.CalendarFeedClientAppleIOS, true},
		{"Apple iPadOS fetcher", "iPadOS/17.5.1 (21F90) dataaccessd/1.0", engagementm.CalendarFeedClientAppleIOS, true},
		{"Apple iPadOS accountsd", "iPadOS/17.5 (21F79) accountsd/1.0", engagementm.CalendarFeedClientAppleIOS, true},
		{"Apple macOS dataaccessd", "macOS/26.2 (25C56) dataaccessd/1.0", engagementm.CalendarFeedClientAppleMac, true},
		{"Apple macOS 27 dataaccessd", "macOS/27.0 (26A428) dataaccessd/1.0", engagementm.CalendarFeedClientAppleMac, true},
		{"Apple macOS 26.6.2 dataaccessd", "macOS/26.6.2 (25G83) dataaccessd/1.0", engagementm.CalendarFeedClientAppleMac, true},
		{"Apple macOS accountsd", "macOS/26.2 (25C56) accountsd/1.0", engagementm.CalendarFeedClientAppleMac, true},
		{"Mac OS X CalendarAgent", "Mac OS X/10.15.7 (19H15) CalendarAgent/954", engagementm.CalendarFeedClientAppleMac, true},
		{"Mac_OS_X CalendarAgent", "Mac_OS_X/10.9 (13A603) CalendarAgent/174", engagementm.CalendarFeedClientAppleMac, true},
		{"Mac+OS+X dataaccessd", "Mac+OS+X/10.15.7 (19H15) dataaccessd/1.0", engagementm.CalendarFeedClientAppleMac, true},
		{"Mac OS X dataaccessd", "Mac OS X/10.15.7 (19H15) dataaccessd/1.0", engagementm.CalendarFeedClientAppleMac, true},
		{"Mac_OS_X accountsd", "Mac_OS_X/10.9 (13A603) accountsd/1.0", engagementm.CalendarFeedClientAppleMac, true},
		{"CalendarAgent alone", "CalendarAgent/954 CFNetwork/1410.0.3 Darwin/22.4.0", engagementm.CalendarFeedClientAppleMac, true},
		{"iOS without an Apple fetcher is other", "iOS/17.5.1 (21F90) SomethingElse/1.0", engagementm.CalendarFeedClientOther, true},
		{"macOS without an Apple fetcher is other", "macOS/26.2 (25C56) SomethingElse/1.0", engagementm.CalendarFeedClientOther, true},
		{"accountsd without an Apple prefix is other", "Linux/6.1 accountsd/1.0", engagementm.CalendarFeedClientOther, true},
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

	status, err := suite.svc.tokenStatusAt(user.ID, "http://localhost:8080", at.Add(time.Hour))
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

// --- Forgetting a client ---

// statusReadsAt is the client families the status lists for userID at the
// instant now.
func (suite *CalendarIntegrationTestSuite) statusReadsAt(userID uint, now time.Time) []engagementm.CalendarFeedClient {
	suite.T().Helper()
	status, err := suite.svc.tokenStatusAt(userID, "http://localhost:8080", now)
	suite.Require().NoError(err)
	var clients []engagementm.CalendarFeedClient
	for _, read := range status.Reads {
		clients = append(clients, read.Client)
	}
	return clients
}

// A read exactly feedReadForgetAfter old is still listed; one a moment older
// is not, though its row is kept.
func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_ReadOlderThanTheForgetPeriodIsNotListed() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	_, err := suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at)
	suite.Require().NoError(err)
	_, err = suite.svc.recordFeedReadAt(context.Background(), token, appleUA, at.Add(time.Hour))
	suite.Require().NoError(err)

	edge := at.Add(feedReadForgetAfter)
	suite.Equal([]engagementm.CalendarFeedClient{engagementm.CalendarFeedClientAppleIOS, engagementm.CalendarFeedClientGoogle},
		suite.statusReadsAt(user.ID, edge), "a read exactly the period old is remembered")
	suite.Equal([]engagementm.CalendarFeedClient{engagementm.CalendarFeedClientAppleIOS},
		suite.statusReadsAt(user.ID, edge.Add(time.Second)), "a read older than the period is forgotten")
	suite.Nil(suite.statusReadsAt(user.ID, edge.Add(2*time.Hour)), "with every read forgotten the status lists none")
	suite.Len(suite.feedReadRows(user.ID), 2, "forgetting by age hides the rows; it does not delete them")
}

// A family read again after being forgotten by age comes back as newly seen:
// its first read is the new one.
func (suite *CalendarIntegrationTestSuite) TestRecordFeedRead_ForgottenFamilyComesBackNewlySeen() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	_, err := suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at)
	suite.Require().NoError(err)
	_, err = suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at.Add(time.Hour))
	suite.Require().NoError(err)

	back := at.Add(time.Hour + feedReadForgetAfter + time.Second)
	wrote, err := suite.svc.recordFeedReadAt(context.Background(), token, googleUA, back)
	suite.Require().NoError(err)
	suite.True(wrote)

	rows := suite.feedReadRows(user.ID)
	suite.Require().Len(rows, 1)
	suite.True(rows[0].FirstReadAt.Equal(back), "first_read_at restarts at %v, got %v", back, rows[0].FirstReadAt)
	suite.True(rows[0].LastReadAt.Equal(back), "last_read_at = %v, got %v", back, rows[0].LastReadAt)

	// A family still remembered keeps its first read.
	later := back.Add(feedReadForgetAfter)
	_, err = suite.svc.recordFeedReadAt(context.Background(), token, googleUA, later)
	suite.Require().NoError(err)
	rows = suite.feedReadRows(user.ID)
	suite.True(rows[0].FirstReadAt.Equal(back), "first_read_at stays %v, got %v", back, rows[0].FirstReadAt)
	suite.True(rows[0].LastReadAt.Equal(later))
}

// Forgetting one family removes it from the status and leaves the others,
// and that family's next real read brings it back as newly seen.
func (suite *CalendarIntegrationTestSuite) TestForgetFeedRead_RemovesOneFamilyUntilItReadsAgain() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	other := suite.createTestUser(true)
	otherToken := suite.mintFeedToken(other.ID)
	for _, tok := range []string{token, otherToken} {
		for _, ua := range []string{googleUA, appleUA} {
			suite.Require().NoError(suite.svc.RecordFeedRead(context.Background(), tok, ua))
		}
	}

	suite.Require().NoError(suite.svc.ForgetFeedRead(user.ID, engagementm.CalendarFeedClientGoogle))

	now := time.Now()
	suite.Equal([]engagementm.CalendarFeedClient{engagementm.CalendarFeedClientAppleIOS}, suite.statusReadsAt(user.ID, now))
	suite.Len(suite.statusReadsAt(other.ID, now), 2, "another account's reads are untouched")

	returned := now.Add(time.Second)
	wrote, err := suite.svc.recordFeedReadAt(context.Background(), token, googleUA, returned)
	suite.Require().NoError(err)
	suite.True(wrote, "the forgotten family's next read is recorded at once")
	suite.ElementsMatch([]engagementm.CalendarFeedClient{engagementm.CalendarFeedClientAppleIOS, engagementm.CalendarFeedClientGoogle},
		suite.statusReadsAt(user.ID, returned))
	for _, row := range suite.feedReadRows(user.ID) {
		if row.Client == engagementm.CalendarFeedClientGoogle {
			suite.True(row.FirstReadAt.Equal(returned), "first_read_at = %v, want %v", row.FirstReadAt, returned)
		}
	}
}

// Forgetting a family with no read, or with no token at all, succeeds and
// changes nothing.
func (suite *CalendarIntegrationTestSuite) TestForgetFeedRead_NothingToForgetSucceeds() {
	user := suite.createTestUser(true)
	suite.Require().NoError(suite.svc.ForgetFeedRead(user.ID, engagementm.CalendarFeedClientOther), "no token")

	token := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.RecordFeedRead(context.Background(), token, appleUA))
	suite.Require().NoError(suite.svc.ForgetFeedRead(user.ID, engagementm.CalendarFeedClientGoogle), "no read by that family")
	suite.Require().NoError(suite.svc.ForgetFeedRead(user.ID, engagementm.CalendarFeedClientGoogle), "twice")

	suite.Len(suite.feedReadRows(user.ID), 1)
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

// A marker beside a live token is not reachable through the services; it is
// planted here so the clear in the same transaction as the delete is
// observable.
func (suite *CalendarIntegrationTestSuite) TestDeleteToken_ClearsTheLinkReset() {
	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)
	suite.markLinkReset(user.ID, time.Now(), engagementm.CalendarLinkResetPasswordReset)

	suite.Require().NoError(suite.svc.DeleteToken(user.ID))

	at, reason := suite.linkReset(user.ID)
	suite.Nil(at)
	suite.Nil(reason)
}

// After a reset the user holds a marker and no token. Turning the feed off
// then succeeds and clears the marker.
func (suite *CalendarIntegrationTestSuite) TestDeleteToken_AfterAResetClearsTheMarker() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	deleted, err := DeleteCalendarTokensTx(suite.db, user.ID)
	suite.Require().NoError(err)
	suite.Require().Equal(int64(1), deleted)
	suite.markLinkReset(user.ID, time.Now(), engagementm.CalendarLinkResetPasswordReset)

	suite.Require().NoError(suite.svc.DeleteToken(user.ID))

	at, reason := suite.linkReset(user.ID)
	suite.Nil(at)
	suite.Nil(reason)
	_, err = suite.svc.ValidateCalendarToken(token)
	suite.Error(err, "the reset link stays refused")
	suite.ErrorIs(suite.svc.DeleteToken(user.ID), errNoCalendarToken, "with neither a token nor a marker there is nothing to turn off")
}

// A revoked link that a calendar app had read is marked with the reason; its
// reads go with the token.
func (suite *CalendarIntegrationTestSuite) TestRevokeCalendarLinkTx_ReadLinkIsMarked() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.RecordFeedRead(context.Background(), token, appleUA))
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	suite.Require().NoError(RevokeCalendarLinkTx(suite.db, user.ID, at, engagementm.CalendarLinkResetPasswordReset))

	_, err := suite.svc.ValidateCalendarToken(token)
	suite.Error(err, "the link is revoked")
	suite.Zero(suite.allFeedReadRows())
	markedAt, reason := suite.linkReset(user.ID)
	suite.Require().NotNil(markedAt)
	suite.True(markedAt.Equal(at))
	suite.Equal(engagementm.CalendarLinkResetPasswordReset, *reason)
}

// A revoked link no calendar app ever read leaves no marker; a browser
// opening it is not a read.
func (suite *CalendarIntegrationTestSuite) TestRevokeCalendarLinkTx_UnreadLinkIsNotMarked() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.RecordFeedRead(context.Background(), token, browserUA))

	suite.Require().NoError(RevokeCalendarLinkTx(suite.db, user.ID, time.Now(), engagementm.CalendarLinkResetSignOutEverywhere))

	_, err := suite.svc.ValidateCalendarToken(token)
	suite.Error(err, "the link is revoked")
	at, reason := suite.linkReset(user.ID)
	suite.Nil(at)
	suite.Nil(reason)
}

// A revoked link whose only reads are forgotten, by age or by the account,
// leaves no marker; a read exactly the forget period old still marks it.
func (suite *CalendarIntegrationTestSuite) TestRevokeCalendarLinkTx_ForgottenReadsAreNotCounted() {
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	revokedAt := func(readAt time.Time) *time.Time {
		user := suite.createTestUser(true)
		token := suite.mintFeedToken(user.ID)
		_, err := suite.svc.recordFeedReadAt(context.Background(), token, appleUA, readAt)
		suite.Require().NoError(err)
		suite.Require().NoError(RevokeCalendarLinkTx(suite.db, user.ID, at, engagementm.CalendarLinkResetPasswordReset))
		marked, _ := suite.linkReset(user.ID)
		return marked
	}

	suite.Nil(revokedAt(at.Add(-feedReadForgetAfter-time.Second)), "a read older than the period is not counted")
	suite.NotNil(revokedAt(at.Add(-feedReadForgetAfter)), "a read exactly the period old is counted")

	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	suite.Require().NoError(suite.svc.RecordFeedRead(context.Background(), token, appleUA))
	suite.Require().NoError(suite.svc.ForgetFeedRead(user.ID, engagementm.CalendarFeedClientAppleIOS))
	suite.Require().NoError(RevokeCalendarLinkTx(suite.db, user.ID, time.Now(), engagementm.CalendarLinkResetSignOutEverywhere))
	marked, _ := suite.linkReset(user.ID)
	suite.Nil(marked, "a read the account forgot is not counted")
}

// With no token to revoke, nothing changes, an earlier marker included.
func (suite *CalendarIntegrationTestSuite) TestRevokeCalendarLinkTx_NoTokenLeavesTheMarker() {
	user := suite.createTestUser(true)
	earlier := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	suite.markLinkReset(user.ID, earlier, engagementm.CalendarLinkResetSignOutEverywhere)

	suite.Require().NoError(RevokeCalendarLinkTx(suite.db, user.ID, earlier.Add(time.Hour), engagementm.CalendarLinkResetPasswordReset))

	at, reason := suite.linkReset(user.ID)
	suite.Require().NotNil(at)
	suite.True(at.Equal(earlier))
	suite.Equal(engagementm.CalendarLinkResetSignOutEverywhere, *reason)
}

// A read whose INSERT found the token while a revocation was deleting it
// waits on the delete, fails its foreign-key check once the delete commits,
// and reports that it wrote nothing rather than an error.
func (suite *CalendarIntegrationTestSuite) TestRecordFeedRead_TokenDeletedUnderTheInsertWritesNothing() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)

	revocation := suite.db.Begin()
	suite.Require().NoError(revocation.Error)
	defer revocation.Rollback()
	deleted, err := DeleteCalendarTokensTx(revocation, user.ID)
	suite.Require().NoError(err)
	suite.Require().Equal(int64(1), deleted)

	var wrote bool
	record := testutil.StartOnAnotherConnection(suite.T(), suite.db, func() error {
		var err error
		wrote, err = suite.svc.recordFeedReadAt(context.Background(), token, googleUA, time.Now())
		return err
	})
	suite.Require().Empty(record, "the record must be waiting on the uncommitted delete")

	suite.Require().NoError(revocation.Commit().Error)
	suite.Require().NoError(testutil.AwaitConcurrentWrite(suite.T(), record), "a vanished token is not an error")
	suite.False(wrote)
	suite.Zero(suite.allFeedReadRows())
}

// The CHECK constraints admit every family and reason the model names, so a
// constant added without widening its constraint fails here rather than on
// every poll.
func (suite *CalendarIntegrationTestSuite) TestFeedReadAndLinkResetConstraintsAdmitTheVocabularies() {
	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	var tokenID uint
	suite.Require().NoError(suite.db.Table("calendar_tokens").Where("user_id = ?", user.ID).Pluck("id", &tokenID).Error)
	for _, client := range engagementm.CalendarFeedClients {
		suite.NoError(suite.db.Exec(
			"INSERT INTO calendar_feed_reads (token_id, client, first_read_at, last_read_at) VALUES (?, ?, ?, ?)",
			tokenID, string(client), at, at).Error, "client %q", client)
	}
	for _, reason := range engagementm.CalendarLinkResetReasons {
		suite.NoError(MarkCalendarLinkResetTx(suite.db, user.ID, at, reason), "reason %q", reason)
	}
}

// Two polls inside the interval that race past the NOT EXISTS: the second
// waits on the first's row and, once it commits, the conflict branch refuses
// the update, so the first read stands.
func (suite *CalendarIntegrationTestSuite) TestRecordFeedRead_RacingPollInsideTheIntervalDoesNotWrite() {
	user := suite.createTestUser(true)
	token := suite.mintFeedToken(user.ID)
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	first := suite.db.Begin()
	suite.Require().NoError(first.Error)
	defer first.Rollback()
	wrote, err := (&CalendarService{db: first}).recordFeedReadAt(context.Background(), token, googleUA, at)
	suite.Require().NoError(err)
	suite.Require().True(wrote)

	var secondWrote bool
	second := testutil.StartOnAnotherConnection(suite.T(), suite.db, func() error {
		var err error
		secondWrote, err = suite.svc.recordFeedReadAt(context.Background(), token, googleUA, at.Add(feedReadRecordInterval/2))
		return err
	})
	suite.Require().Empty(second, "the second poll must be waiting on the first's uncommitted row")

	suite.Require().NoError(first.Commit().Error)
	suite.Require().NoError(testutil.AwaitConcurrentWrite(suite.T(), second))

	suite.False(secondWrote, "the conflict branch refuses a read inside the interval")
	rows := suite.feedReadRows(user.ID)
	suite.Require().Len(rows, 1)
	suite.True(rows[0].LastReadAt.Equal(at), "last_read_at stays %v, got %v", at, rows[0].LastReadAt)
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
