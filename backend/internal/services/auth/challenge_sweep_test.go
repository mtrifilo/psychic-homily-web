package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"

	"psychic-homily-backend/internal/services/shared"
)

type failingChallengeCleaner struct{ calls int }

func (f *failingChallengeCleaner) CleanupExpiredChallenges(context.Context) (int64, error) {
	f.calls++
	return 0, errors.New("database unavailable")
}

// A failed delete ends the cycle without panicking, so the loop keeps its
// schedule.
func TestChallengeSweepCycle_FailureDoesNotPanic(t *testing.T) {
	cleaner := &failingChallengeCleaner{}
	sweep := NewChallengeSweep(cleaner)
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
	suite.Require().NoError(suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?", expiresAt, id).Error)
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

	NewChallengeSweep(suite.service).runCycle(context.Background())

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

	sweep := NewChallengeSweep(suite.service)
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
