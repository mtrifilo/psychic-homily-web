package notification

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"psychic-homily-backend/internal/services/shared"
)

const (
	// defaultVenueAlertFlushInterval is how often the poller looks for batches
	// that have gone quiet. Frequent and cheap: a tick with nothing ready is one
	// grouped read of a partial index over rows in flight.
	defaultVenueAlertFlushInterval = 60 * time.Second

	// defaultVenueAlertQuietWindow is how long a batch must go without a new
	// member before it is considered finished.
	//
	// It MUST exceed the show-notify outbox's inter-tick gap, which is what
	// actually paces a drop: that poller drains five shows a minute, so
	// consecutive members of one calendar import land about 60 seconds apart. A
	// window shorter than that would declare the batch quiet in the ordinary gap
	// BETWEEN two ticks of the same drop, and send an email naming the first five
	// shows of fifty. Five minutes leaves four intervals of margin.
	//
	// Being wrong here is not a correctness failure — a batch flushed early
	// simply mails fewer shows, and the rest still reach the inbox row. It is a
	// quality-of-message setting, and it is a per-drop delay the reader pays for
	// a message worth reading.
	defaultVenueAlertQuietWindow = 5 * time.Minute

	// defaultVenueAlertMaxHold bounds how long a batch may wait for quiet.
	//
	// The quiet window alone can starve: a venue with a steady trickle of
	// accruals never goes quiet, and its followers would be told nothing at all.
	// This is the fail-safe, and it is safe to apply BLUNTLY because the quiet
	// window is an optimisation rather than a guard — a batch retired at the hold
	// delivers what it has, later members join the same group, the next flush
	// re-resolves it, and the delivery claim no-ops. The reader gets one alert,
	// with an inbox row that keeps growing.
	defaultVenueAlertMaxHold = 30 * time.Minute

	// defaultVenueAlertFlushBatch caps how many batches one tick resolves.
	//
	// Small for the same reason the show-notify outbox's batch is: a tick's
	// wall-clock is this number times one batch, and a batch sends one
	// SYNCHRONOUS provider request per email recipient. The per-user daily cap
	// bounds mail per RECIPIENT, not the number of recipients, so a popular venue
	// is an unbounded number of sequential sends.
	defaultVenueAlertFlushBatch = 5

	// defaultVenueAlertMaxAge is the poison-pill bound: how long a batch may keep
	// FAILING before it is abandoned unsent.
	//
	// This is not the same thing as maxHold, and the difference is what makes it
	// necessary. maxHold bounds how long a HEALTHY batch waits for quiet; this
	// bounds a batch whose delivery keeps erroring. Without it such a group is
	// retried forever, and the selection query makes that actively harmful rather
	// than merely wasteful: groups are taken oldest-first, so a permanently
	// failing one is by definition at the head and re-occupies a slot every tick.
	// Five of them and the loop stops delivering for every venue on the platform.
	//
	// Six hours is chosen so that an ordinary incident (a database restart, a
	// deploy, a provider outage) still delivers on recovery, while a genuinely
	// stuck group stops holding the queue open. Abandoning is logged per-group at
	// warning level, because it is a silent user-visible loss otherwise.
	defaultVenueAlertMaxAge = 6 * time.Hour
)

// VenueAlertFlushPoller delivers coalesced venue new-show alerts (PSY-1895).
//
// # Why a poller and not the write path
//
// Venue alerts are one per venue per venue-local day, however many shows land.
// Deciding who hears about a drop cannot happen when each show arrives, because
// at that moment the drop is not over: a `discovery-import` run over a venue's
// calendar enters a season of dates through the show-notify outbox at five a
// minute. Accrual records each show as it becomes visible; this poller waits for
// the drop to finish and then resolves followers, preferences, cross-system
// dedup and the message ONCE against the whole day's set.
//
// # Re-runs are expected, not exceptional
//
// A show announced later the same day joins a batch that has already been
// delivered, and the flush re-resolves the whole group so the inbox row can grow
// to include it. Every such pass reaches the delivery claim.
// uq_notification_log_venue_show_alert plus ON CONFLICT DO NOTHING is what makes
// it silent, and it is also what makes two concurrent ticks safe: the loser
// claims nothing and therefore sends nothing.
//
// The consequence to be honest about: the EMAIL is sent once, at first dispatch,
// and names the shows that had landed by then. Later members reach the inbox but
// not a second message. The email copy promises grouping, not completeness.
//
// # No backfill
//
// Starting this on a deploy cannot alert anyone about the existing catalogue:
// venue_show_alert_batch ships empty and only accrual writes to it, so the
// poller has nothing to find until a show becomes visible at a venue somebody
// already follows.
type VenueAlertFlushPoller struct {
	flusher venueAlertFlusher

	interval    time.Duration
	quietWindow time.Duration
	maxHold     time.Duration
	maxAge      time.Duration
	batch       int

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// venueAlertFlusher is the one method of NotificationFilterService this poller
// needs. Narrowing it keeps the poller testable without standing up an email
// service, and documents that the poller owns SCHEDULING and no delivery logic
// of its own.
type venueAlertFlusher interface {
	FlushVenueShowAlerts(ctx context.Context, limit int, quietWindow, maxHold, maxAge time.Duration) int
}

// NewVenueAlertFlushPoller constructs the poller. flusher is normally the
// container's *NotificationFilterService — the same instance that accrues, so
// both halves share one definition of what a batch is.
func NewVenueAlertFlushPoller(flusher venueAlertFlusher) *VenueAlertFlushPoller {
	return &VenueAlertFlushPoller{
		flusher:     flusher,
		interval:    shared.EnvPositiveDuration("VENUE_ALERT_FLUSH_INTERVAL_SECONDS", time.Second, defaultVenueAlertFlushInterval),
		quietWindow: shared.EnvPositiveDuration("VENUE_ALERT_QUIET_WINDOW_MINUTES", time.Minute, defaultVenueAlertQuietWindow),
		maxHold:     shared.EnvPositiveDuration("VENUE_ALERT_MAX_HOLD_MINUTES", time.Minute, defaultVenueAlertMaxHold),
		maxAge:      shared.EnvPositiveDuration("VENUE_ALERT_MAX_AGE_HOURS", time.Hour, defaultVenueAlertMaxAge),
		batch:       shared.EnvPositiveInt("VENUE_ALERT_FLUSH_BATCH", defaultVenueAlertFlushBatch),
		stopCh:      make(chan struct{}),
		logger:      slog.Default(),
	}
}

// Start begins the background poller. No boot cycle, mirroring the show-notify
// outbox: the first tick is one interval in, which no deploy cadence can starve
// at a 60s interval.
func (p *VenueAlertFlushPoller) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:     "venue_alert_flush",
			Interval: p.interval,
			StopCh:   p.stopCh,
		}, p.processTick)
	}()
	p.logger.Info("venue alert flush poller started",
		"interval", p.interval, "quiet_window", p.quietWindow,
		"max_hold", p.maxHold, "max_age", p.maxAge, "batch", p.batch)
}

// Stop gracefully stops the poller.
func (p *VenueAlertFlushPoller) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("venue alert flush poller stopped")
}

// RunNow runs one cycle immediately (tests / manual trigger).
func (p *VenueAlertFlushPoller) RunNow(ctx context.Context) { p.processTick(ctx) }

// processTick resolves one round of ready batches.
func (p *VenueAlertFlushPoller) processTick(ctx context.Context) {
	// A missing flusher is a wiring bug, not work to retry. Nothing is claimed
	// here, so there is no queue state to protect — just say so and skip.
	if p.flusher == nil {
		p.logger.Error("venue-alert flush: no flusher configured, skipping tick")
		return
	}

	// The kill switch is re-read EVERY tick, not just at boot. cmd/server
	// consults it once to decide whether to start this loop at all, but an
	// operator setting it during an incident is trying to stop outbound mail NOW,
	// and a boot-time-only gate would keep draining the backlog after they
	// believed sending had stopped. Accrual is already per-call, so both halves
	// behave the same way under the same flag.
	if !VenueShowAlertsEnabled() {
		return
	}

	dispatched := p.flusher.FlushVenueShowAlerts(ctx, p.batch, p.quietWindow, p.maxHold, p.maxAge)
	if dispatched > 0 {
		p.logger.Info("venue alert flush tick", "dispatched", dispatched)
	}
}
