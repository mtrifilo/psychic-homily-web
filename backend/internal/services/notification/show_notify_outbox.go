package notification

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
)

const (
	defaultShowNotifyInterval = 60 * time.Second

	// defaultShowNotifyBatch is small on purpose. A tick's wall-clock is the batch
	// size times one MatchAndNotify, and MatchAndNotify sends one SYNCHRONOUS
	// provider HTTP request per matching recipient — the per-user 10/day cap bounds
	// mail per RECIPIENT, not the number of recipients, so a popular show is an
	// unbounded number of sequential sends. Ten shows a minute drains any realistic
	// ingest burst within the hour while keeping the tick far under staleReclaim.
	defaultShowNotifyBatch = 10

	// defaultShowNotifyStaleReclaim must exceed the worst-case tick wall-clock, and
	// this value is a MARGIN, not a measurement — see reclaimStale for what is and
	// is not known about it.
	defaultShowNotifyStaleReclaim = 30 * time.Minute

	// defaultShowNotifyFinalizeBudget caps the post-work finalize writes only. It
	// must be started AFTER the notify pass returns — building it earlier makes it
	// cover the (slow, email-sending) work too, which is how the image-enrich
	// outbox stranded 1017 production rows (PSY-1569).
	defaultShowNotifyFinalizeBudget = 30 * time.Second

	// strandedInProcessing is the last_error written when reclaim gives up on a row
	// that exhausted its attempts while stuck in `processing`. A SENTINEL, not just
	// a message: it distinguishes "the machinery lost this row" from a genuine
	// matching failure, which is what makes an operator recovery query possible.
	strandedInProcessing = "stranded in processing after max attempts"
)

// ShowNotifyOutboxPoller drains show_notify_queue (PSY-1894) — the trigger that
// makes follower notifications fire for shows created by INGEST rather than by
// the admin approval flow.
//
// # Why this exists
//
// MatchAndNotify had exactly two non-test call sites, both in the admin approve
// handlers, so the fanout was coupled to the moderation queue instead of to the
// visibility transition. Ingest-created shows are born `approved` and never enter
// that queue, so the large majority of real show announcements notified nobody.
// The show write funnels now enqueue a row here when a show becomes visible, and
// this poller runs the SAME MatchAndNotify path the admin handlers run. This
// ticket deliberately adds no new alert type and no new channel logic
// (PSY-1895/1896/1897 own those) — it only makes the existing path fire.
//
// # Why an outbox rather than a direct call
//
// The ingest writers are separate, short-lived processes: cmd/discovery-import
// drives the DiscoveryService against the database and exits. A fire-and-forget
// goroutine at the write site (which is what the admin handlers do, via GoSafe)
// would be killed with the process before it delivered anything. A durable row
// drained by the long-running server is the only thing that reaches the
// notification path from there. It also gives the property the ticket demands
// outright: a notification failure can never roll back the ingest write, because
// the two are separated by a commit.
//
// # No backfill, and no clock to get wrong
//
// The rollout cannot notify anyone about the shows that already exist, because
// show_notify_queue ships EMPTY and nothing backfills it. A row exists if and only
// if a write path observed a show become visible after this shipped.
//
// The rejected alternative was a watermark — "notify shows created after T" or
// "after id N". That would have made the blast radius depend on when the
// watermark was initialised relative to the first poll, on clock skew between
// writer and poller, and on created_at being a proxy for "became visible". It is
// not one: `shows` has no approved_at column, and a show can be created `pending`
// and approved months later. An empty table needs none of those assumptions.
//
// # No re-notification
//
// Two independent layers, deliberately:
//
//  1. uq_show_notify_queue_show is a whole-table UNIQUE on show_id, so a show gets
//     at most one job row EVER. Re-ingesting a venue calendar or editing a show
//     hits ON CONFLICT DO NOTHING. (This is why the table is never pruned —
//     deleting terminal rows would re-open re-notification.)
//  2. MatchAndNotify's own per-(user, show) dedup against notification_log, which
//     already shipped for the admin path (PSY-1341/1467). This is what covers the
//     residual overlap between this poller and the admin approve handlers, so a
//     show that somehow travels both routes still yields one notification.
//
// # Concurrency and recovery
//
// Claiming uses SELECT ... FOR UPDATE SKIP LOCKED via the shared job-queue
// mechanics, so multiple server instances can poll without double-claiming. Rows
// flip to `processing` under that lock and the matching runs OUTSIDE it — the
// notify pass sends email, and holding row locks across that would be a
// long-transaction antipattern. reclaimStale recovers rows orphaned by a crash or
// a deploy mid-batch; finalize writes are guarded by `status = 'processing'`.
//
// The guard does not cover the ABA case (reclaim re-pends a row, another worker
// re-claims it, the late write lands anyway). Correctness there rests on the work
// being idempotent, and it is FOR DELIVERY: a second MatchAndNotify for the same
// show finds the notification_log rows from the first and sends nothing. It is not
// idempotent for STATISTICS — processUserMatches bumps each matching filter's
// match_count and last_matched_at before it reaches the dedup check, so a
// re-processed job inflates those counters, which users see on their filters.
// That is pre-existing MatchAndNotify behaviour (admin batch approve can do the
// same) and is left alone here rather than fixed under this ticket's scope, but it
// is the reason the reclaim window wants to stay generous.
type ShowNotifyOutboxPoller struct {
	db      *gorm.DB
	matcher showMatcher

	// queue owns claim/finalize/reclaim. PruneTerminal is deliberately never
	// called — see the type doc.
	queue *shared.JobQueue[catalogm.ShowNotifyQueueItem]

	interval     time.Duration
	batch        int
	staleReclaim time.Duration
	// finalizeBudget caps the finalize writes alone; a field so tests can shrink it.
	finalizeBudget time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// showMatcher is the one method of NotificationFilterService this poller needs.
// Narrowing it keeps the poller testable without standing up an email service and
// documents that the poller adds no matching logic of its own.
type showMatcher interface {
	MatchAndNotify(show *catalogm.Show) error
}

// NewShowNotifyOutboxPoller constructs the poller. matcher is normally the
// container's *NotificationFilterService — the same instance the admin approve
// handlers call, so both routes share one implementation of "who wants this show".
func NewShowNotifyOutboxPoller(database *gorm.DB, matcher showMatcher) *ShowNotifyOutboxPoller {
	if database == nil {
		database = db.GetDB()
	}
	return &ShowNotifyOutboxPoller{
		db:      database,
		matcher: matcher,
		queue: shared.NewJobQueue[catalogm.ShowNotifyQueueItem](database, "show-notify outbox",
			shared.QueueStatuses{
				Pending:    catalogm.ShowNotifyStatusPending,
				Processing: catalogm.ShowNotifyStatusProcessing,
				Failed:     catalogm.ShowNotifyStatusFailed,
				// Listed for completeness. Nothing calls PruneTerminal on this
				// queue and nothing should: the terminal row IS the "already
				// considered" record that blocks re-enqueue.
				Terminal: []string{
					catalogm.ShowNotifyStatusDone,
					catalogm.ShowNotifyStatusSkipped,
					catalogm.ShowNotifyStatusFailed,
				},
			}, slog.Default()),
		interval:     shared.EnvPositiveDuration("SHOW_NOTIFY_OUTBOX_INTERVAL_SECONDS", time.Second, defaultShowNotifyInterval),
		batch:        shared.EnvPositiveInt("SHOW_NOTIFY_OUTBOX_BATCH", defaultShowNotifyBatch),
		staleReclaim: shared.EnvPositiveDuration("SHOW_NOTIFY_OUTBOX_STALE_RECLAIM_MINUTES", time.Minute, defaultShowNotifyStaleReclaim),

		finalizeBudget: defaultShowNotifyFinalizeBudget,
		stopCh:         make(chan struct{}),
		logger:         slog.Default(),
	}
}

// ShowNotifyOutboxEnabled reports whether the show-notify outbox is switched on.
// It reads the SAME flag the enqueue side reads (ENABLE_SHOW_NOTIFY_OUTBOX,
// default on, "0"/"false"/"no"/"off" to disable), because gating only one side is
// a trap: gating only the drain accumulates rows that fan out in one burst when
// the flag flips back, and gating only the enqueue leaves a poller spinning on a
// table nothing writes to.
//
// It is duplicated here rather than exported from services/catalog because
// notification must not import catalog (catalog would then need notification for
// the reverse direction, and the enqueue helper's package is already the show
// write funnel's own). The flag name is the contract between them.
func ShowNotifyOutboxEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ENABLE_SHOW_NOTIFY_OUTBOX"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// Start begins the background poller. No boot cycle (mirrors the image-enrich
// outbox): the first tick is one interval in, which no deploy cadence can starve
// at a 60s interval.
func (p *ShowNotifyOutboxPoller) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:     "show_notify_outbox",
			Interval: p.interval,
			StopCh:   p.stopCh,
		}, p.processTick)
	}()
	p.logger.Info("show notify outbox poller started",
		"interval", p.interval, "batch", p.batch, "stale_reclaim", p.staleReclaim)
}

// Stop gracefully stops the poller.
func (p *ShowNotifyOutboxPoller) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("show notify outbox poller stopped")
}

// RunNow runs one cycle immediately (tests / manual trigger).
func (p *ShowNotifyOutboxPoller) RunNow(ctx context.Context) { p.processTick(ctx) }

// processTick reclaims stranded rows, claims a pending batch, and runs the
// notification match for each claimed show.
//
// Rows are processed ONE AT A TIME rather than as a batch because each one is an
// independent decision: a show that has since been deleted must be skipped, not
// fail its neighbours, and a matching error on one show must not re-queue the
// twenty that already delivered (which would be re-notification, the one thing
// this queue exists to prevent).
func (p *ShowNotifyOutboxPoller) processTick(ctx context.Context) {
	p.reclaimStale(ctx)

	items, err := p.queue.Claim(ctx, p.batch)
	if err != nil {
		p.logger.Error("show-notify outbox: claim failed", "error", err)
		return
	}
	if len(items) == 0 {
		return
	}

	var notified, skipped int
	for i := range items {
		// Stop taking on new work once the tick is cancelled, but finalize what has
		// already run. The remaining claimed rows stay `processing` and are returned
		// to `pending` by reclaimStale — they are not lost, and crucially they are
		// not marked done without having notified.
		if ctx.Err() != nil {
			p.requeueCanceled(ctx, items[i:])
			break
		}
		if p.processItem(ctx, items[i]) {
			notified++
		} else {
			skipped++
		}
	}

	p.logger.Info("show-notify outbox tick", "claimed", len(items), "notified", notified, "other", skipped)
}

// processItem runs one job. Returns true when MatchAndNotify actually ran for the
// show.
func (p *ShowNotifyOutboxPoller) processItem(ctx context.Context, item catalogm.ShowNotifyQueueItem) bool {
	show, err := p.loadShow(ctx, item.ShowID)
	if err != nil {
		p.markFailedOrRetry(ctx, item, err)
		return false
	}

	// Re-check at DELIVERY time with the SAME predicate the enqueue used. Between
	// the two a show can be rejected, made private, cancelled, deleted, or simply
	// have its date go by while the poller was down — and announcing something that
	// has been pulled, called off, or already happened is worse than announcing
	// nothing. The window is one poll interval normally, and however long an outage
	// lasted otherwise.
	if ok, reason := catalogm.ShowAnnounceable(show, time.Now()); !ok {
		p.markSkipped(ctx, item, reason)
		return false
	}

	if p.matcher == nil {
		p.markFailedOrRetry(ctx, item, errors.New("notification matcher not configured"))
		return false
	}

	if err := p.matcher.MatchAndNotify(show); err != nil {
		p.markFailedOrRetry(ctx, item, err)
		return false
	}

	p.markDone(ctx, item)
	return true
}

// loadShow reads the show back by id. A missing row is (nil, nil) — a deleted show
// is an expected outcome here, not an error to retry.
func (p *ShowNotifyOutboxPoller) loadShow(ctx context.Context, showID uint) (*catalogm.Show, error) {
	var show catalogm.Show
	err := p.db.WithContext(ctx).Where("id = ?", showID).First(&show).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &show, nil
}

// markDone records that MatchAndNotify ran for this show. This row is now the
// permanent "already considered" record that blocks any future enqueue.
func (p *ShowNotifyOutboxPoller) markDone(ctx context.Context, item catalogm.ShowNotifyQueueItem) {
	now := time.Now()
	p.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":       catalogm.ShowNotifyStatusDone,
		"processed_at": &now,
		"last_error":   nil,
	})
}

// markSkipped records a deliberate non-delivery. Terminal like `done`, so the show
// is never reconsidered — a show that was pulled before its notification went out
// should stay quiet even if it is republished later.
func (p *ShowNotifyOutboxPoller) markSkipped(ctx context.Context, item catalogm.ShowNotifyQueueItem, reason string) {
	now := time.Now()
	p.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":       catalogm.ShowNotifyStatusSkipped,
		"processed_at": &now,
		"last_error":   reason,
	})
	p.logger.Info("show-notify outbox: skipped", "show_id", item.ShowID, "reason", reason)
}

// markFailedOrRetry returns a row that still has attempts left to `pending`, and
// marks an exhausted one `failed`. items carry PRE-increment attempts (Claim
// already did the +1), so the post-increment count is Attempts+1.
func (p *ShowNotifyOutboxPoller) markFailedOrRetry(ctx context.Context, item catalogm.ShowNotifyQueueItem, cause error) {
	status := catalogm.ShowNotifyStatusPending
	if item.Attempts+1 >= item.MaxAttempts {
		status = catalogm.ShowNotifyStatusFailed
	}
	p.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":     status,
		"last_error": cause.Error(),
	})
	p.logger.Warn("show-notify outbox: match failed",
		"show_id", item.ShowID, "status", status, "error", cause)
}

// requeueCanceled returns rows abandoned mid-tick to `pending` and DECREMENTS
// attempts, undoing the claim-time increment so a deploy cannot burn a row toward
// `failed`. A show that is never notified because its row exhausted attempts
// during rolling restarts is a silent miss, which is the failure this prevents.
func (p *ShowNotifyOutboxPoller) requeueCanceled(ctx context.Context, items []catalogm.ShowNotifyQueueItem) {
	if len(items) == 0 {
		return
	}
	p.finalize(ctx, shared.QueueRowIDs(items), map[string]interface{}{
		"status":   catalogm.ShowNotifyStatusPending,
		"attempts": gorm.Expr("GREATEST(attempts - 1, 0)"),
	})
	p.logger.Info("show-notify outbox: tick canceled, requeued", "count", len(items))
}

// finalize delegates to the shared mechanics (status='processing' guard, error
// logged). The write context is detached from the tick's cancellation so a
// shutdown mid-batch still records what already happened — otherwise a row that
// DID notify would be left `processing`, get reclaimed, and notify again. It stays
// bounded so a hung database cannot wedge Stop()/wg.Wait().
func (p *ShowNotifyOutboxPoller) finalize(ctx context.Context, ids []uint, updates map[string]interface{}) {
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.finalizeBudget)
	defer cancel()
	p.queue.Finalize(fctx, ids, updates)
}

// reclaimStale delegates to the shared mechanics. staleReclaim MUST exceed the
// worst-case tick wall-clock or this reclaims rows still legitimately in flight.
//
// The honest position on the 30 minute default: it is NOT measured. A tick is
// batch (10) times one MatchAndNotify, and MatchAndNotify's cost is dominated by
// one synchronous provider request per matching recipient (filter matches and
// scene-follow fanout both send inline), which no cap in this package bounds —
// the 10/day limit is per RECIPIENT. A show matching 200 subscribers is 200
// sequential sends. Ten such shows in one claim could plausibly approach the
// window. The image-enrich outbox made exactly this mistake in the other
// direction: its doc estimated 1-2 minutes against a 15 minute default and
// production measured 9-14 (PSY-1569). Measure this against real subscriber
// counts before growing batch, and treat the number as provisional until then.
//
// What makes the residual risk tolerable rather than merely accepted: an early
// reclaim here is not a duplicate notification. The re-processed show finds its
// own notification_log rows from the first pass and delivers nothing, so the cost
// is a burned attempt and an inflated per-filter match_count, not a second email.
func (p *ShowNotifyOutboxPoller) reclaimStale(ctx context.Context) {
	p.queue.ReclaimStale(ctx, p.staleReclaim, strandedInProcessing)
}
