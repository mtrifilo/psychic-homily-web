package notification

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"psychic-homily-backend/internal/services/shared"
)

const (
	// defaultReleaseDigestFlushInterval is how often the poller looks for weeks
	// that have closed.
	defaultReleaseDigestFlushInterval = 15 * time.Minute

	// defaultReleaseDigestWeekHold is how old a week bucket must be before it is
	// delivered, measured from the bucket's own Monday. 168 hours means "the week
	// is over": this IS the schedule, not a quiet window. Shortening it makes the
	// first delivery cover less of the week, because the claim is per
	// (user, week). Overridable only so tests and manual repro can run in
	// minutes.
	defaultReleaseDigestWeekHold = 168 * time.Hour

	// defaultReleaseDigestFlushBatch caps how many weeks one tick resolves. A
	// week sends one synchronous provider request per email recipient, and the
	// per-user daily cap does not bound the number of recipients, so a tick's
	// wall-clock grows with this number. The steady state is at most one ready
	// week.
	defaultReleaseDigestFlushBatch = 2

	// defaultReleaseDigestMaxAge is the poison-pill bound: how long a week may
	// keep FAILING before it is abandoned unsent. It measures failure duration,
	// not bucket age, since every eligible week is already at least seven days
	// old (see noteReleaseDigestFailure).
	defaultReleaseDigestMaxAge = 12 * time.Hour
)

// ReleaseDigestFlushPoller delivers the weekly artist new-release roundup: one
// message per user per week across every followed artist. Accrual
// (services/catalog) records each observation; this poller waits for the week
// to close and resolves followers, preferences and the message once against
// the whole week.
//
// Re-runs are expected: a release accrued after delivery reopens the week so the
// inbox row can grow, and every such pass reaches the delivery claim.
// uq_notification_log_artist_release_digest with ON CONFLICT DO NOTHING makes
// that silent and makes concurrent ticks safe (the loser claims nothing and
// sends nothing). Any email would go out once, at first dispatch; later
// members reach only the inbox row.
type ReleaseDigestFlushPoller struct {
	flusher releaseDigestFlusher

	interval time.Duration
	weekHold time.Duration
	maxAge   time.Duration
	batch    int

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// releaseDigestFlusher is the one method of NotificationFilterService this
// poller needs: the poller owns scheduling and no delivery logic.
type releaseDigestFlusher interface {
	FlushArtistReleaseDigests(ctx context.Context, limit int, weekHold, maxAge time.Duration) int
}

// NewReleaseDigestFlushPoller constructs the poller. flusher is normally the
// container's *NotificationFilterService.
func NewReleaseDigestFlushPoller(flusher releaseDigestFlusher) *ReleaseDigestFlushPoller {
	return &ReleaseDigestFlushPoller{
		flusher:  flusher,
		interval: shared.EnvPositiveDuration("RELEASE_DIGEST_FLUSH_INTERVAL_SECONDS", time.Second, defaultReleaseDigestFlushInterval),
		weekHold: shared.EnvPositiveDuration("RELEASE_DIGEST_WEEK_HOLD_HOURS", time.Hour, defaultReleaseDigestWeekHold),
		maxAge:   shared.EnvPositiveDuration("RELEASE_DIGEST_MAX_AGE_HOURS", time.Hour, defaultReleaseDigestMaxAge),
		batch:    shared.EnvPositiveInt("RELEASE_DIGEST_FLUSH_BATCH", defaultReleaseDigestFlushBatch),
		stopCh:   make(chan struct{}),
		logger:   slog.Default(),
	}
}

// Start begins the background poller. No boot cycle: the first tick is one
// interval in, so a deploy cadence shorter than the interval starves it. That
// is survivable (weeks are selected oldest-first and a missed tick loses
// nothing), but it is the first thing to check when a roundup never went out.
func (p *ReleaseDigestFlushPoller) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:     "release_digest_flush",
			Interval: p.interval,
			StopCh:   p.stopCh,
		}, p.processTick)
	}()
	p.logger.Info("release digest flush poller started",
		"interval", p.interval, "week_hold", p.weekHold,
		"max_age", p.maxAge, "batch", p.batch)
}

// Stop gracefully stops the poller.
func (p *ReleaseDigestFlushPoller) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("release digest flush poller stopped")
}

// RunNow runs one cycle immediately (tests / manual trigger).
func (p *ReleaseDigestFlushPoller) RunNow(ctx context.Context) { p.processTick(ctx) }

// processTick resolves one round of closed weeks.
func (p *ReleaseDigestFlushPoller) processTick(ctx context.Context) {
	// A missing flusher is a wiring bug, not work to retry.
	if p.flusher == nil {
		p.logger.Error("release-digest flush: no flusher configured, skipping tick")
		return
	}

	// The kill switch is re-read every tick, not just at boot, so setting it
	// during an incident stops delivery without a restart. Accrual also reads it
	// per call.
	if !ArtistReleaseAlertsEnabled() {
		return
	}

	dispatched := p.flusher.FlushArtistReleaseDigests(ctx, p.batch, p.weekHold, p.maxAge)
	if dispatched > 0 {
		p.logger.Info("release digest flush tick", "dispatched", dispatched)
	}
}
