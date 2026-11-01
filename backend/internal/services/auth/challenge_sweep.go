package auth

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/shared"
)

const (
	// ChallengeSweepLoopName is the loop's name in logs and its
	// background_service_runs primary key.
	ChallengeSweepLoopName = "webauthn_challenge_sweep"

	// DisableChallengeSweepEnvVar set to "1" keeps the sweep from starting.
	DisableChallengeSweepEnvVar = "DISABLE_WEBAUTHN_CHALLENGE_SWEEP"

	// defaultChallengeSweepInterval is at or above the run-state persistence
	// threshold, so the loop keeps a background_service_runs row and the sweep
	// health check watches it.
	defaultChallengeSweepInterval = 24 * time.Hour

	// SignupChallengeSweepLoopName is the sign-up sweep's name in logs.
	SignupChallengeSweepLoopName = "webauthn_signup_challenge_sweep"

	// DisableSignupChallengeSweepEnvVar set to "1" keeps the sign-up sweep from
	// starting.
	DisableSignupChallengeSweepEnvVar = "DISABLE_WEBAUTHN_SIGNUP_CHALLENGE_SWEEP"

	// signupChallengeSweepInterval bounds how long an abandoned sign-up's email
	// and date of birth outlive its challenge's expiry. It is below the run-state
	// persistence threshold, so this loop keeps no background_service_runs row
	// and the sweep health check does not watch it; the daily sweep, which is
	// watched, deletes the same rows if this loop stops.
	signupChallengeSweepInterval = 10 * time.Minute
)

// ChallengeSweep deletes expired WebAuthn challenges on a schedule. A finish
// consumes the challenge it presents, so the rows this deletes are the ones
// whose ceremony was begun and never finished.
type ChallengeSweep struct {
	name string
	// label names the sweep in its log messages.
	label string
	// deleteExpired deletes expired challenges and returns how many rows it
	// deleted.
	deleteExpired func(ctx context.Context) (int64, error)
	interval      time.Duration
	runAtBoot     bool

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewChallengeSweep constructs the daily sweep over every expired challenge in
// database. It needs only the database, so it runs whether or not the
// WebAuthn service could be constructed. It runs no cycle on the boot path;
// RunScheduledLoop schedules the first cycle from persisted run state.
func NewChallengeSweep(database *gorm.DB) *ChallengeSweep {
	return &ChallengeSweep{
		name:  ChallengeSweepLoopName,
		label: "webauthn challenge sweep",
		deleteExpired: func(ctx context.Context) (int64, error) {
			return deleteExpiredChallenges(ctx, database)
		},
		interval: defaultChallengeSweepInterval,
		stopCh:   make(chan struct{}),
		logger:   slog.Default(),
	}
}

// NewSignupChallengeSweep constructs the sweep over the expired sign-up
// challenges in database, the ones that hold a person's email and date of
// birth before any account exists. While the process runs, an abandoned
// sign-up challenge is deleted within signupChallengeSweepInterval of its
// expiry: the loop runs a cycle at boot, so a restart never pushes the next
// cycle past that bound. Like the daily sweep it needs only the database.
func NewSignupChallengeSweep(database *gorm.DB) *ChallengeSweep {
	return &ChallengeSweep{
		name:  SignupChallengeSweepLoopName,
		label: "webauthn signup challenge sweep",
		deleteExpired: func(ctx context.Context) (int64, error) {
			return deleteExpiredSignupChallenges(ctx, database, time.Now())
		},
		interval:  signupChallengeSweepInterval,
		runAtBoot: true,
		stopCh:    make(chan struct{}),
		logger:    slog.Default(),
	}
}

// Start begins the loop.
func (s *ChallengeSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:      s.name,
			Interval:  s.interval,
			StopCh:    s.stopCh,
			RunAtBoot: s.runAtBoot,
		}, s.runCycle)
	}()
	s.logger.Info(s.label+" started", "interval", s.interval)
}

// Stop stops the loop and waits for an in-flight cycle to return.
func (s *ChallengeSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info(s.label + " stopped")
}

// runCycle deletes expired challenges and reports the deleted count to the run
// state.
func (s *ChallengeSweep) runCycle(ctx context.Context) {
	deleted, err := s.deleteExpired(ctx)
	if err != nil {
		shared.LogCycleFailure(s.logger, s.label+": cycle failed", err)
		shared.ReportCycle(ctx, 0, err)
		return
	}
	shared.ReportCycle(ctx, int(deleted), nil)
	s.logger.Info(s.label+": cycle complete", "deleted", deleted)
}
