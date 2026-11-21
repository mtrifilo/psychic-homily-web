package engagement

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
)

func TestCalendarLinkMintHour_IsTheClockHour(t *testing.T) {
	phoenix := time.FixedZone("MST", -7*60*60)
	for name, c := range map[string]struct{ at, want time.Time }{
		"mid hour":       {time.Date(2026, 11, 21, 12, 34, 56, 789, time.UTC), time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)},
		"last instant":   {time.Date(2026, 11, 21, 12, 59, 59, 999999000, time.UTC), time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)},
		"on the hour":    {time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC), time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC)},
		"another zone":   {time.Date(2026, 11, 21, 5, 30, 0, 0, phoenix), time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)},
		"half-hour zone": {time.Date(2026, 11, 21, 18, 15, 0, 0, time.FixedZone("IST", 5*60*60+30*60)), time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)},
	} {
		t.Run(name, func(t *testing.T) {
			if got := calendarLinkMintHour(c.at); !got.Equal(c.want) {
				t.Errorf("calendarLinkMintHour(%s) = %s, want %s", c.at, got, c.want)
			}
		})
	}
}

// --- Over a real database ---

// drivenClock is a clock a test sets, read by the service's transactions.
type drivenClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *drivenClock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func (c *drivenClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// serviceOn is a CalendarService whose transactions read clock.
func (suite *CalendarIntegrationTestSuite) serviceOn(clock *drivenClock) *CalendarService {
	return &CalendarService{
		db:           suite.db.Session(&gorm.Session{NowFunc: clock.read}),
		savedShowSvc: suite.svc.savedShowSvc,
	}
}

// mintCount is the account's stored hour and count, ok false when it has
// none.
func (suite *CalendarIntegrationTestSuite) mintCount(userID uint) (hour time.Time, count int, ok bool) {
	suite.T().Helper()
	var rows []struct {
		MintHour  time.Time
		MintCount int
	}
	suite.Require().NoError(suite.db.Raw(
		`SELECT mint_hour, mint_count FROM calendar_link_mint_counts WHERE user_id = ?`, userID,
	).Scan(&rows).Error)
	if len(rows) == 0 {
		return time.Time{}, 0, false
	}
	return rows[0].MintHour, rows[0].MintCount, true
}

// midHourClock is a driven clock set mid-hour, so a test's mints share one
// clock hour however long the test takes.
func midHourClock() *drivenClock {
	return &drivenClock{now: time.Date(2026, 11, 21, 12, 30, 0, 0, time.UTC)}
}

// mintOn mints userID's link through svc, failing the test on an error.
func (suite *CalendarIntegrationTestSuite) mintOn(svc *CalendarService, userID uint) string {
	suite.T().Helper()
	minted, err := svc.CreateToken(userID, nil, "http://localhost:8080")
	suite.Require().NoError(err)
	return minted.Token
}

// mintAt mints userID's link at the instant at through svc on clock.
func mintAt(svc *CalendarService, clock *drivenClock, userID uint, at time.Time) (string, error) {
	clock.set(at)
	minted, err := svc.CreateToken(userID, nil, "http://localhost:8080")
	if err != nil {
		return "", err
	}
	return minted.Token, nil
}

// requireRefused asserts err is the hourly cap's refusal with retryAfter.
func (suite *CalendarIntegrationTestSuite) requireRefused(err error, retryAfter time.Duration) {
	suite.T().Helper()
	var limited *apperrors.CalendarLinkRateLimitedError
	suite.Require().True(errors.As(err, &limited), "want the hourly cap's refusal, got %v", err)
	suite.Equal(calendarLinkMintsPerHour, limited.Limit)
	suite.Equal(retryAfter, limited.RetryAfter)
}

// Ten mints in one clock hour succeed, the first link among them. The
// eleventh is refused with the time to the next clock hour, and leaves the
// account's link, its retired rows and its count as they were.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_EleventhInAClockHourIsRefusedAndChangesNothing() {
	user := suite.createTestUser(true)
	clock := &drivenClock{}
	svc := suite.serviceOn(clock)
	hour := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

	var current string
	for i := range calendarLinkMintsPerHour {
		token, err := mintAt(svc, clock, user.ID, hour.Add(time.Duration(i)*time.Minute+123456*time.Microsecond))
		suite.Require().NoError(err, "mint %d", i+1)
		current = token
	}
	retiredBefore := suite.retiredRowCount()
	suite.Equal(int64(calendarLinkMintsPerHour-1), retiredBefore, "every mint but the first replaced a link")
	var tokenBefore engagementm.CalendarToken
	suite.Require().NoError(suite.db.Where("user_id = ?", user.ID).Take(&tokenBefore).Error)

	at := hour.Add(40*time.Minute + 250*time.Microsecond)
	_, err := mintAt(svc, clock, user.ID, at)
	suite.requireRefused(err, 20*time.Minute-250*time.Microsecond)

	var tokens []engagementm.CalendarToken
	suite.Require().NoError(suite.db.Where("user_id = ?", user.ID).Find(&tokens).Error)
	suite.Require().Len(tokens, 1)
	suite.Equal(tokenBefore.ID, tokens[0].ID)
	suite.Equal(tokenBefore.TokenHash, tokens[0].TokenHash)
	validated, err := suite.svc.ValidateCalendarToken(current)
	suite.Require().NoError(err, "the link from before the refusal still works")
	suite.Equal(user.ID, validated.ID)
	suite.Equal(retiredBefore, suite.retiredRowCount(), "the refusal wrote no retired row")

	storedHour, count, ok := suite.mintCount(user.ID)
	suite.Require().True(ok)
	suite.True(storedHour.Equal(hour), "stored hour %s, want %s", storedHour, hour)
	suite.Equal(calendarLinkMintsPerHour, count)
}

// The count belongs to the clock hour: ten at the end of one hour leave the
// next hour's first instant free, which then allows ten of its own.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_NextClockHourStartsAtZero() {
	user := suite.createTestUser(true)
	clock := &drivenClock{}
	svc := suite.serviceOn(clock)
	lastSecond := time.Date(2026, 11, 21, 12, 59, 59, 0, time.UTC)
	nextHour := time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC)

	for i := range calendarLinkMintsPerHour {
		_, err := mintAt(svc, clock, user.ID, lastSecond.Add(time.Duration(i)*time.Microsecond))
		suite.Require().NoError(err, "mint %d at hh:59:59", i+1)
	}
	lastInstant := nextHour.Add(-time.Microsecond)
	_, err := mintAt(svc, clock, user.ID, lastInstant)
	suite.requireRefused(err, time.Microsecond)

	for i := range calendarLinkMintsPerHour {
		_, err := mintAt(svc, clock, user.ID, nextHour.Add(time.Duration(i)*time.Second))
		suite.Require().NoError(err, "mint %d in the next hour", i+1)
	}
	at := nextHour.Add(30 * time.Minute)
	_, err = mintAt(svc, clock, user.ID, at)
	suite.requireRefused(err, 30*time.Minute)

	storedHour, count, ok := suite.mintCount(user.ID)
	suite.Require().True(ok)
	suite.True(storedHour.Equal(nextHour), "stored hour %s, want %s", storedHour, nextHour)
	suite.Equal(calendarLinkMintsPerHour, count)
}

// A server clock behind the one that last counted cannot restart the count:
// its mint counts against the later hour already stored.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_AClockBehindCountsAgainstTheLaterHour() {
	user := suite.createTestUser(true)
	clock := &drivenClock{}
	svc := suite.serviceOn(clock)
	nextHour := time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC)

	for i := range calendarLinkMintsPerHour - 1 {
		_, err := mintAt(svc, clock, user.ID, nextHour.Add(time.Duration(i)*time.Millisecond))
		suite.Require().NoError(err)
	}
	behind := nextHour.Add(-time.Second)
	_, err := mintAt(svc, clock, user.ID, behind)
	suite.Require().NoError(err, "the tenth, read on a clock a second behind")
	_, err = mintAt(svc, clock, user.ID, behind)
	suite.requireRefused(err, time.Hour+time.Second)

	storedHour, count, _ := suite.mintCount(user.ID)
	suite.True(storedHour.Equal(nextHour), "stored hour %s, want %s", storedHour, nextHour)
	suite.Equal(calendarLinkMintsPerHour, count)
}

// Two mints at once at nine: exactly one is made and the other is refused,
// whichever order the database takes them in.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_TwoConcurrentMintsAtNineMakeOne() {
	user := suite.createTestUser(true)
	svc := suite.serviceOn(midHourClock())
	for range calendarLinkMintsPerHour - 1 {
		suite.mintOn(svc, user.ID)
	}

	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = svc.CreateToken(user.ID, nil, "http://localhost:8080")
		}()
	}
	close(start)
	wg.Wait()

	var made, refused int
	for _, err := range errs {
		var limited *apperrors.CalendarLinkRateLimitedError
		switch {
		case err == nil:
			made++
		case errors.As(err, &limited):
			refused++
		default:
			suite.Failf("unexpected error", "%v", err)
		}
	}
	suite.Equal(1, made)
	suite.Equal(1, refused)
	_, count, _ := suite.mintCount(user.ID)
	suite.Equal(calendarLinkMintsPerHour, count)
	suite.Equal(int64(calendarLinkMintsPerHour-1), suite.retiredRowCount())
}

// While one mint's count is uncommitted, another mint of the same account
// waits for it, then is refused if that mint commits and made if it rolls
// back: a mint that rolls back uses no slot.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_WaitsForAnUncommittedCount() {
	for name, commit := range map[string]bool{"commits": true, "rolls back": false} {
		suite.Run(name, func() {
			user := suite.createTestUser(true)
			clock := midHourClock()
			svc := suite.serviceOn(clock)
			for range calendarLinkMintsPerHour - 1 {
				suite.mintOn(svc, user.ID)
			}

			tx := suite.db.Begin()
			suite.Require().NoError(tx.Error)
			// Releases the lock if an assertion stops the test first; a no-op
			// once the transaction has ended.
			defer tx.Rollback()
			suite.Require().NoError(countCalendarLinkMintTx(tx, user.ID, clock.read()))

			done := make(chan error, 1)
			go func() {
				_, err := svc.CreateToken(user.ID, nil, "http://localhost:8080")
				done <- err
			}()
			select {
			case err := <-done:
				suite.FailNow("the second mint did not wait for the first's count", "returned %v", err)
			case <-time.After(300 * time.Millisecond):
			}

			if commit {
				suite.Require().NoError(tx.Commit().Error)
				var limited *apperrors.CalendarLinkRateLimitedError
				err := <-done
				suite.True(errors.As(err, &limited), "want the hourly cap's refusal, got %v", err)
			} else {
				suite.Require().NoError(tx.Rollback().Error)
				suite.NoError(<-done)
			}
			_, count, _ := suite.mintCount(user.ID)
			suite.Equal(calendarLinkMintsPerHour, count)
		})
	}
}

// Only making a link counts: reading it, turning it off, and the resets of
// signing out everywhere and a password reset leave the count where it was.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_OnlyMakingALinkCounts() {
	user := suite.createTestUser(true)
	svc := suite.serviceOn(midHourClock())
	token := suite.mintOn(svc, user.ID)

	_, err := suite.svc.GetTokenStatus(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)
	_, err = suite.svc.ValidateCalendarToken(token)
	suite.Require().NoError(err)
	suite.Require().NoError(suite.svc.DeleteToken(user.ID))
	for _, reason := range []engagementm.CalendarLinkResetReason{
		engagementm.CalendarLinkResetSignOutEverywhere,
		engagementm.CalendarLinkResetPasswordReset,
	} {
		suite.mintOn(svc, user.ID)
		suite.Require().NoError(suite.db.Transaction(func(tx *gorm.DB) error {
			_, err := RevokeCalendarLinkTx(tx, user.ID, time.Now(), reason)
			return err
		}))
	}

	_, count, ok := suite.mintCount(user.ID)
	suite.Require().True(ok)
	suite.Equal(3, count, "the three mints, and nothing else")
}

// The count is deleted with its account.
func (suite *CalendarIntegrationTestSuite) TestCalendarLinkMintCount_IsDeletedWithTheAccount() {
	user := suite.createTestUser(true)
	suite.mintFeedToken(user.ID)
	_, _, ok := suite.mintCount(user.ID)
	suite.Require().True(ok)

	suite.Require().NoError(suite.db.Unscoped().Delete(&authm.User{}, user.ID).Error)

	_, _, ok = suite.mintCount(user.ID)
	suite.False(ok, "the count outlived its account")
}

// The count stores the account, an hour and a number: nothing finer than the
// hour and nothing about any link.
func (suite *CalendarIntegrationTestSuite) TestCalendarLinkMintCounts_HoldOnlyTheHourAndTheCount() {
	var columns []string
	suite.Require().NoError(suite.db.Raw(`
		SELECT column_name FROM information_schema.columns
		WHERE table_name = 'calendar_link_mint_counts' ORDER BY ordinal_position`).Scan(&columns).Error)
	suite.Equal([]string{"user_id", "mint_hour", "mint_count"}, columns)
}

// The refusal says what every surface already says when a link could not be
// made, so the cap adds no wording of its own.
func TestCalendarLinkRateLimitedError_IsTheCapsRefusal(t *testing.T) {
	err := error(&apperrors.CalendarLinkRateLimitedError{Limit: calendarLinkMintsPerHour, RetryAfter: time.Minute})
	var limited *apperrors.CalendarLinkRateLimitedError
	if !errors.As(err, &limited) || limited.UserMessage() != "Couldn't create your calendar link. Try again." {
		t.Fatalf("refusal = %v", err)
	}
}
