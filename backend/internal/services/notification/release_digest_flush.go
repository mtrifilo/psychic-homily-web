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
	//
	// Fifteen minutes rather than the venue loop's sixty seconds, because the
	// work it is looking for arrives once a WEEK. A tick with nothing ready is one
	// grouped read of a partial index, so the cost either way is negligible; the
	// argument for the longer interval is log noise, and the argument against
	// making it much longer is that it is also the granularity at which a LATE
	// release joins an already-delivered week's inbox row.
	defaultReleaseDigestFlushInterval = 15 * time.Minute

	// defaultReleaseDigestWeekHold is how old a week bucket must be before it is
	// delivered, measured from the bucket's own Monday.
	//
	// 168 hours means "the week is over", which is the whole product decision
	// (PSY-1892: releases weekly) expressed as a duration. It is emphatically NOT
	// the venue loop's quiet window: that one waits for a batch to stop growing
	// and is an optimisation, whereas this one IS the schedule. Shortening it does
	// not make the roundup arrive sooner with the same contents — it makes the
	// roundup cover less of the week, permanently, because the delivery claim is
	// per (user, week) and the records that land afterwards reach the inbox row
	// and no second email.
	//
	// Shortened in tests and in manual repro so a week can be exercised in
	// minutes. That is the only reason it is an env var rather than a constant.
	defaultReleaseDigestWeekHold = 168 * time.Hour

	// defaultReleaseDigestFlushBatch caps how many weeks one tick resolves.
	//
	// Small for the same reason the venue loop's is: a tick's wall-clock is this
	// number times one week, and a week sends one SYNCHRONOUS provider request per
	// email recipient. The per-user daily cap bounds mail per RECIPIENT, not the
	// number of recipients, so a week in which a popular artist released something
	// is an unbounded number of sequential sends.
	//
	// Two rather than the venue loop's five because the steady state here is at
	// most one ready week: a backlog of more than two means something was down for
	// a fortnight, and draining that slowly is correct.
	defaultReleaseDigestFlushBatch = 2

	// defaultReleaseDigestMaxAge is the poison-pill bound: how long a week may
	// keep FAILING before it is abandoned unsent.
	//
	// It measures how long delivery has been failing, NOT how old the week is —
	// see noteReleaseDigestFailure for why bucket age would abandon every roundup
	// on its first transient error here, given that a week is always at least
	// seven days old before it is eligible at all.
	//
	// Twelve hours, twice the venue loop's, because this loop's ticks are fifteen
	// minutes apart: six hours would be a couple of dozen attempts, and a week's
	// roundup is worth more retries than a single venue-day's.
	defaultReleaseDigestMaxAge = 12 * time.Hour
)

// ReleaseDigestFlushPoller delivers the weekly artist new-release roundup
// (PSY-1897).
//
// # Why a poller and not the write path
//
// The roundup is one message per user per week across every artist they follow.
// Deciding who hears about a release cannot happen when the release arrives,
// because at that moment the week is not over and the other artists have not
// released yet. Accrual (services/catalog, inside the release create funnel)
// records each observation; this poller waits for the week to close and then
// resolves followers, preferences and the message ONCE against the whole week.
//
// # Re-runs are expected, not exceptional
//
// A release accrued after the roundup went out joins a week that has already
// been delivered, and the flush re-resolves the whole week so the inbox row can
// grow to include it. Every such pass reaches the delivery claim.
// uq_notification_log_artist_release_digest plus ON CONFLICT DO NOTHING is what
// makes it silent, and it is also what makes two concurrent ticks safe: the
// loser claims nothing and therefore sends nothing.
//
// The consequence to be honest about: the EMAIL is sent once, at first dispatch,
// and names the records that had landed by then. Later members reach the inbox
// but not a second message. The email copy promises grouping, not completeness.
//
// # No backfill
//
// Starting this on a deploy cannot alert anyone about the existing catalogue:
// artist_release_alert_batch ships empty and only accrual writes to it, so the
// poller has nothing to find until a release is created for an artist somebody
// already follows — and then only if catalogm.ReleaseAnnounceable agrees it is
// new music rather than back catalogue.
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
// poller needs. Narrowing it keeps the poller testable without standing up an
// email service, and documents that the poller owns SCHEDULING and no delivery
// logic of its own.
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

// Start begins the background poller. No boot cycle, mirroring the venue flush
// and the show-notify outbox: the first tick is one interval in.
//
// Unlike those two, an interval this long CAN be starved by a deploy cadence —
// a redeploy every ten minutes would mean no tick ever fires. That is survivable
// rather than ignored: the work is a week old already, weeks are selected
// oldest-first, and nothing is lost by a missed tick. It is called out here so
// that a future operator debugging "the roundup never went out" during a
// deploy storm has somewhere to start.
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
	// A missing flusher is a wiring bug, not work to retry. Nothing is claimed
	// here, so there is no queue state to protect — just say so and skip.
	if p.flusher == nil {
		p.logger.Error("release-digest flush: no flusher configured, skipping tick")
		return
	}

	// The kill switch is re-read EVERY tick, not just at boot. cmd/server consults
	// it once to decide whether to start this loop at all, but an operator setting
	// it during an incident is trying to stop outbound mail NOW, and a
	// boot-time-only gate would keep draining the backlog after they believed
	// sending had stopped. Accrual is already per-call, so both halves behave the
	// same way under the same flag.
	if !ArtistReleaseAlertsEnabled() {
		return
	}

	dispatched := p.flusher.FlushArtistReleaseDigests(ctx, p.batch, p.weekHold, p.maxAge)
	if dispatched > 0 {
		p.logger.Info("release digest flush tick", "dispatched", dispatched)
	}
}
