package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

type failingChallengeCleaner struct{ calls int }

func (f *failingChallengeCleaner) deleteExpired(context.Context) (int64, error) {
	f.calls++
	return 0, errors.New("database unavailable")
}

// A failed delete ends the cycle without panicking, so the loop keeps its
// schedule.
func TestChallengeSweepCycle_FailureDoesNotPanic(t *testing.T) {
	cleaner := &failingChallengeCleaner{}
	sweep := NewChallengeSweep(nil)
	sweep.deleteExpired = cleaner.deleteExpired
	assert.NotPanics(t, func() { sweep.runCycle(context.Background()) })
	assert.Equal(t, 1, cleaner.calls)
}

// storeChallengeExpiringAt stores a challenge for userID and sets its expiry.
func (suite *WebAuthnServiceIntegrationTestSuite) storeChallengeExpiringAt(userID uint, expiresAt time.Time) string {
	id, err := suite.service.StoreChallenge(userID, &webauthn.SessionData{
		Challenge: "sweep-challenge",
		UserID:    []byte("sweep-user"),
	}, "authentication")
	suite.Require().NoError(err)
	suite.setChallengeExpiry(id, expiresAt)
	return id
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestChallengeSweepCycle_DeletesExpiredKeepsLive() {
	user := suite.createUser("sweep@test.com")
	now := time.Now()
	expired := []string{
		suite.storeChallengeExpiringAt(user.ID, now.Add(-time.Minute)),
		suite.storeChallengeExpiringAt(user.ID, now.Add(-30*24*time.Hour)),
	}
	live := []string{
		suite.storeChallengeExpiringAt(user.ID, now.Add(time.Minute)),
		suite.storeChallengeExpiringAt(user.ID, now.Add(5*time.Minute)),
	}

	NewChallengeSweep(suite.db).runCycle(context.Background())

	for _, id := range expired {
		suite.Zero(suite.challengeRows(id), "expired challenge %s is deleted", id)
	}
	for _, id := range live {
		suite.Equal(int64(1), suite.challengeRows(id), "live challenge %s is kept", id)
	}
}

// The sweep registers a daily run-state row on start, which is what puts it
// under the sweep health check's overdue alerting.
func (suite *WebAuthnServiceIntegrationTestSuite) TestChallengeSweep_RegistersRunState() {
	shared.SetDefaultRunStore(shared.NewGormRunStore(suite.db))
	suite.T().Cleanup(func() {
		shared.SetDefaultRunStore(nil)
		suite.Require().NoError(suite.db.Exec("DELETE FROM background_service_runs WHERE name = ?", ChallengeSweepLoopName).Error)
	})

	sweep := NewChallengeSweep(suite.db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sweep.Start(ctx)
	defer sweep.Stop()

	type runRow struct {
		IntervalSeconds  int64
		LastRegisteredAt *time.Time
	}
	var row runRow
	suite.Require().Eventually(func() bool {
		row = runRow{}
		err := suite.db.Raw(
			"SELECT interval_seconds, last_registered_at FROM background_service_runs WHERE name = ?",
			ChallengeSweepLoopName,
		).Scan(&row).Error
		return err == nil && row.LastRegisteredAt != nil
	}, 5*time.Second, 20*time.Millisecond, "the sweep registers its run-state row")
	suite.Equal(int64((24 * time.Hour).Seconds()), row.IntervalSeconds)

	var registered bool
	for _, loop := range shared.RegisteredLoops() {
		if loop.Name == ChallengeSweepLoopName {
			registered = true
			suite.Equal(24*time.Hour, loop.Interval)
		}
	}
	suite.True(registered, "the health check re-registers the sweep from the process registry")
}

// storeSignupChallengeWithDateOfBirth stores a sign-up challenge for email as
// signup/begin stores one, carrying a date of birth, and returns its id.
func (suite *WebAuthnServiceIntegrationTestSuite) storeSignupChallengeWithDateOfBirth(email string) string {
	dateOfBirth := time.Date(1990, time.March, 4, 0, 0, 0, 0, time.UTC)
	id, err := suite.service.StoreSignupChallenge(contracts.SignupChallenge{
		Email: email,
		Acceptance: contracts.LegalAcceptance{
			TermsVersion:   "terms",
			PrivacyVersion: "privacy",
			MinAgeAttested: 16,
			DateOfBirth:    &dateOfBirth,
		},
	}, &webauthn.SessionData{Challenge: "signup-sweep-" + email}, contracts.WebAuthnSignupOperation)
	suite.Require().NoError(err)
	return id
}

// setChallengeExpiry sets the expiry of the challenge stored under id.
func (suite *WebAuthnServiceIntegrationTestSuite) setChallengeExpiry(id string, expiresAt time.Time) {
	suite.Require().NoError(suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?", expiresAt, id).Error)
}

// The sign-up sweep deletes expired sign-up challenges only: unexpired sign-up
// challenges and expired challenges of every other operation stay.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignupChallengeSweepCycle_DeletesOnlyExpiredSignupChallenges() {
	user := suite.createUser("signup-sweep@test.com")
	now := time.Now()

	expiredSignup := []string{
		suite.storeSignupChallengeWithDateOfBirth("expired-a@test.com"),
		suite.storeSignupChallengeWithDateOfBirth("expired-b@test.com"),
	}
	suite.setChallengeExpiry(expiredSignup[0], now.Add(-time.Second))
	suite.setChallengeExpiry(expiredSignup[1], now.Add(-30*24*time.Hour))
	liveSignup := suite.storeSignupChallengeWithDateOfBirth("live@test.com")

	expiredLogin := suite.storeChallengeExpiringAt(user.ID, now.Add(-time.Hour))
	expiredRegistration, err := suite.service.StoreChallenge(user.ID, &webauthn.SessionData{Challenge: "registration"}, contracts.WebAuthnRegistrationOperation)
	suite.Require().NoError(err)
	suite.setChallengeExpiry(expiredRegistration, now.Add(-time.Hour))

	NewSignupChallengeSweep(suite.db).runCycle(context.Background())

	for _, id := range expiredSignup {
		suite.Zero(suite.challengeRows(id), "expired sign-up challenge %s is deleted", id)
	}
	suite.Equal(int64(1), suite.challengeRows(liveSignup), "an unexpired sign-up challenge is kept")
	suite.Equal(int64(1), suite.challengeRows(expiredLogin), "an expired login challenge is left to the daily sweep")
	suite.Equal(int64(1), suite.challengeRows(expiredRegistration), "an expired registration challenge is left to the daily sweep")
}

// A sign-up challenge outlives its own expiry only until the first cycle after
// it: a cycle at any instant up to the expiry keeps it, a cycle after the
// expiry deletes it, so with a cycle every signupChallengeSweepInterval the
// row is gone within that interval of expiring.
func (suite *WebAuthnServiceIntegrationTestSuite) TestDeleteExpiredSignupChallenges_DeletesFromTheFirstCycleAfterExpiry() {
	storedAt := time.Now()
	id := suite.storeSignupChallengeWithDateOfBirth("bound@test.com")

	var expiresAt time.Time
	suite.Require().NoError(suite.db.Raw("SELECT expires_at FROM webauthn_challenges WHERE id = ?", id).Scan(&expiresAt).Error)
	suite.WithinDuration(storedAt.Add(challengeTTL), expiresAt, 5*time.Second, "a sign-up challenge expires challengeTTL after it is stored")
	suite.Equal(15*time.Minute, challengeTTL+signupChallengeSweepInterval, "the retention bound an abandoned sign-up's date of birth is held to")

	ctx := context.Background()
	for _, clock := range []time.Time{storedAt, expiresAt.Add(-time.Second), expiresAt} {
		deleted, err := deleteExpiredSignupChallenges(ctx, suite.db, clock)
		suite.Require().NoError(err)
		suite.Zero(deleted, "a cycle at %s keeps a challenge expiring at %s", clock, expiresAt)
	}
	suite.Equal(int64(1), suite.challengeRows(id))

	deleted, err := deleteExpiredSignupChallenges(ctx, suite.db, expiresAt.Add(time.Millisecond))
	suite.Require().NoError(err)
	suite.Equal(int64(1), deleted)
	suite.Zero(suite.challengeRows(id), "the first cycle after the expiry deletes the challenge")
}

// The sign-up sweep runs a cycle when it starts, so a restart does not leave
// an expired sign-up challenge waiting a whole interval.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignupChallengeSweep_RunsACycleAtStart() {
	id := suite.storeSignupChallengeWithDateOfBirth("boot@test.com")
	suite.setChallengeExpiry(id, time.Now().Add(-time.Minute))

	sweep := NewSignupChallengeSweep(suite.db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sweep.Start(ctx)
	defer sweep.Stop()

	suite.Eventually(func() bool {
		var count int64
		err := suite.db.Raw("SELECT COUNT(*) FROM webauthn_challenges WHERE id = ?", id).Scan(&count).Error
		return err == nil && count == 0
	}, 5*time.Second, 20*time.Millisecond, "the cycle at start deletes the expired sign-up challenge")
}

// The daily sweep also deletes expired sign-up challenges, so it is the
// watched backstop when the unwatched sign-up sweep is not running.
func (suite *WebAuthnServiceIntegrationTestSuite) TestChallengeSweepCycle_DeletesExpiredSignupChallenges() {
	expired := suite.storeSignupChallengeWithDateOfBirth("daily-expired@test.com")
	suite.setChallengeExpiry(expired, time.Now().Add(-time.Minute))
	live := suite.storeSignupChallengeWithDateOfBirth("daily-live@test.com")

	NewChallengeSweep(suite.db).runCycle(context.Background())

	suite.Zero(suite.challengeRows(expired), "the daily sweep deletes an expired sign-up challenge")
	suite.Equal(int64(1), suite.challengeRows(live), "the daily sweep keeps an unexpired sign-up challenge")
}
