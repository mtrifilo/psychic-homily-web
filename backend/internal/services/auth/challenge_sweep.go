package auth

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

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
)

// expiredChallengeCleaner is the part of WebAuthnService the sweep drives.
type expiredChallengeCleaner interface {
	CleanupExpiredChallenges(ctx context.Context) (int64, error)
}

// ChallengeSweep deletes expired WebAuthn challenges on a schedule. A finish
// consumes the challenge it presents, so the rows this deletes are the ones
// whose ceremony was begun and never finished. It runs no cycle on the boot
// path; RunScheduledLoop schedules the first cycle from persisted run state.
type ChallengeSweep struct {
	challenges expiredChallengeCleaner
	interval   time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewChallengeSweep constructs the sweep over challenges.
func NewChallengeSweep(challenges expiredChallengeCleaner) *ChallengeSweep {
	return &ChallengeSweep{
		challenges: challenges,
		interval:   defaultChallengeSweepInterval,
		stopCh:     make(chan struct{}),
		logger:     slog.Default(),
	}
}

// Start begins the loop.
func (s *ChallengeSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:     ChallengeSweepLoopName,
			Interval: s.interval,
			StopCh:   s.stopCh,
		}, s.runCycle)
	}()
	s.logger.Info("webauthn challenge sweep started", "interval", s.interval)
}

// Stop stops the loop and waits for an in-flight cycle to return.
func (s *ChallengeSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("webauthn challenge sweep stopped")
}

// runCycle deletes expired challenges and reports the deleted count to the run
// state.
func (s *ChallengeSweep) runCycle(ctx context.Context) {
	deleted, err := s.challenges.CleanupExpiredChallenges(ctx)
	if err != nil {
		logFn := s.logger.Error
		if errors.Is(err, context.Canceled) {
			logFn = s.logger.Info
		}
		logFn("webauthn challenge sweep: cycle failed", "error", err)
		shared.ReportCycle(ctx, 0, err)
		return
	}
	shared.ReportCycle(ctx, int(deleted), nil)
	s.logger.Info("webauthn challenge sweep: cycle complete", "deleted", deleted)
}
