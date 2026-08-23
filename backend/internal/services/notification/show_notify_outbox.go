package notification

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
)

const (
	defaultShowNotifyInterval = 60 * time.Second

	// defaultShowNotifyBatch is small on purpose, for two reasons. A tick's
	// wall-clock is the batch size times one MatchAndNotify, and MatchAndNotify
	// sends one SYNCHRONOUS provider request per matching recipient — the per-user
	// 10/day cap bounds mail per RECIPIENT, not the number of recipients, so a
	// popular show is an unbounded number of sequential sends. And a bulk ingest
	// (a `discovery-import` glob across a season of venue calendars) can enqueue
	// hundreds of shows at once, so the drain rate is what decides whether a scene
	// follower meets their whole daily allowance in five minutes or over an hour.
	// Five a minute is still 7,200/day of headroom.
	defaultShowNotifyBatch = 5

	// defaultShowNotifyMaxJobAge is how long a job may wait before it is dropped
	// unsent. It is the fail-safe that makes a backlog structurally unable to burst,
	// whatever produced it — a poller outage, a long deploy freeze, or the kill
	// switch being set on the server while a separately-configured ingest process
	// kept writing rows. Without it, clearing the switch after a week would fan out
	// a week of accumulated announcements at once.
	//
	// A day is chosen so that a normal incident (hours) still delivers, while
	// anything that has gone stale enough for the announcement to read as noise does
	// not. Note this is NOT the no-backfill guard — that is the empty table, and it
	// needs no clock. This bounds only rows that legitimately entered the queue.
	defaultShowNotifyMaxJobAge = 24 * time.Hour

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

	// skipJobTooOld is the reason recorded when maxJobAge drops an undelivered job.
	// Also a sentinel: these rows are the ones an operator may legitimately want to
	// investigate as "announcements we chose not to send", distinct from shows that
	// were pulled or cancelled.
	skipJobTooOld = "job exceeded max age before delivery"
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
// re-claims it, the late write lands anyway). Correctness there rests on
// MatchAndNotify being idempotent, and that holds only SEQUENTIALLY — a later
// MatchAndNotify for the same show finds the notification_log rows the earlier one
// wrote and sends nothing. Two claims are strictly sequential today: railway.toml
// configures no replicas, and RunScheduledLoop runs one tick at a time.
//
// Two caveats, both of which matter the moment a second replica is configured (and
// this poller's own doc advertises SKIP LOCKED as making that safe):
//
//   - CONCURRENT calls are NOT idempotent for scene-follow delivery.
//     notifySceneFollowers dedups with a Count-then-Create, and its log rows carry
//     filter_id = NULL — NULLs compare distinct in the notification_log UNIQUE, so
//     the constraint does not catch the race and both runs send. (The filter path
//     is accidentally safe: pickDeliveryMatch is deterministic, so the second
//     Create violates the UNIQUE and no second email goes out.) Closing this needs
//     a partial UNIQUE ... WHERE filter_id IS NULL plus ON CONFLICT DO NOTHING in
//     scene_follow_notify.go — out of scope here, but it is the prerequisite for
//     running more than one replica.
//   - Not idempotent for STATISTICS either: processUserMatches bumps each matching
//     filter's match_count and last_matched_at before reaching the dedup check, so
//     a re-processed job inflates counters users can see. Pre-existing behaviour
//     (admin batch approve does the same), left alone here.
//
// Both are reasons the reclaim window wants to stay generous.
type ShowNotifyOutboxPoller struct {
	db      *gorm.DB
	matcher showMatcher

	// queue owns claim/finalize/reclaim. PruneTerminal is deliberately never
	// called — see the type doc.
	queue *shared.JobQueue[catalogm.ShowNotifyQueueItem]

	interval     time.Duration
	batch        int
	staleReclaim time.Duration
	maxJobAge    time.Duration
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
				// Terminal is DELIBERATELY EMPTY, and that emptiness is the
				// enforcement — not the comment.
				//
				// Pruning terminal rows here would re-open ON CONFLICT DO NOTHING
				// against uq_show_notify_queue_show, so the next re-ingest of any
				// pruned show would enqueue again and re-notify real subscribers.
				// Naming the statuses and then writing "but never call
				// PruneTerminal" would leave that prohibition resting on a comment,
				// in a file whose sibling (imageenrich/outbox.go) calls
				// PruneTerminal as routine maintenance — the obvious next edit for
				// someone watching this table grow. JobQueue.PruneTerminal returns
				// 0 immediately on an empty Terminal, so with the list empty that
				// edit is a no-op rather than a re-notification incident.
				Terminal: nil,
			}, slog.Default()),
		interval:     shared.EnvPositiveDuration("SHOW_NOTIFY_OUTBOX_INTERVAL_SECONDS", time.Second, defaultShowNotifyInterval),
		batch:        shared.EnvPositiveInt("SHOW_NOTIFY_OUTBOX_BATCH", defaultShowNotifyBatch),
		staleReclaim: shared.EnvPositiveDuration("SHOW_NOTIFY_OUTBOX_STALE_RECLAIM_MINUTES", time.Minute, defaultShowNotifyStaleReclaim),
		maxJobAge:    shared.EnvPositiveDuration("SHOW_NOTIFY_OUTBOX_MAX_JOB_AGE_HOURS", time.Hour, defaultShowNotifyMaxJobAge),

		finalizeBudget: defaultShowNotifyFinalizeBudget,
		stopCh:         make(chan struct{}),
		logger:         slog.Default(),
	}
}

// ShowNotifyOutboxEnabled reports whether the show-notify outbox is switched on,
// so cmd/server can decide whether to start this poller at all.
//
// It reads the SAME env var through the SAME parser the enqueue side uses
// (catalogm.ShowNotifyOutboxDisableFlag, via shared.EnvServiceDisabled). Gating
// only one side is a trap in both directions: gating only the drain accumulates
// rows that fan out in one burst when the flag flips back, and gating only the
// enqueue leaves a poller spinning on a table nothing writes to.
//
// The flag NAME is deliberately not re-spelled here — it comes from
// models/catalog, which both halves already import for the queue's statuses and
// its announceability predicate. A literal repeated in two packages is exactly how
// a rename silently un-gates one half. (The name could not live in the enqueue's
// own package without making notification import services/catalog, which would add
// a production dependency edge this design does not otherwise need.)
func ShowNotifyOutboxEnabled() bool {
	return !shared.EnvServiceDisabled(catalogm.ShowNotifyOutboxDisableFlag)
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
// Rows are processed and finalized ONE AT A TIME rather than as a batch, because
// each is an independent decision: a show that has since been deleted must be
// skipped without failing its neighbours, and a matching error on one show must
// not re-queue the rest of the batch that already delivered — re-queueing a
// delivered row is re-notification, the one thing this queue exists to prevent.
func (p *ShowNotifyOutboxPoller) processTick(ctx context.Context) {
	// A missing matcher is a wiring bug, not a job failure. Bail BEFORE claiming:
	// claiming and then failing each row would burn every job's attempts against a
	// condition no retry can fix, walking the whole queue to `failed` — and a
	// `failed` row is terminal, so those shows could never be notified even after
	// the misconfiguration was corrected. Leaving the rows `pending` means a fixed
	// deploy simply picks them up.
	if p.matcher == nil {
		p.logger.Error("show-notify outbox: no notification matcher configured, skipping tick")
		return
	}

	// The kill switch is re-read EVERY tick, not just at boot. cmd/server consults
	// it once to decide whether to start this loop at all, but an operator setting
	// it during an incident is trying to stop outbound mail NOW, and a boot-time-only
	// gate would keep draining the backlog after they believed sending had stopped.
	// The enqueue side is already per-call, so this makes both halves behave the
	// same way under the same flag.
	if !ShowNotifyOutboxEnabled() {
		return
	}

	p.reclaimStale(ctx)
	p.reapExpired(ctx)

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
		// Stop taking on new work once the tick is cancelled (a deploy landing
		// mid-batch), and hand every row this loop has not reached straight back to
		// `pending` with its claim-time attempt undone. Doing it here rather than
		// leaving them for reclaimStale matters twice over: they are re-tried on the
		// next tick instead of after the 30-minute stale window, and they are never
		// marked done without having notified. Rows already processed above keep
		// their finalized state, because finalize detaches from this cancellation.
		if ctx.Err() != nil {
			p.requeueWithoutAttempt(ctx, items[i:], "")
			p.logger.Info("show-notify outbox: tick canceled, requeued", "count", len(items[i:]))
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
	// Age is checked FIRST, before any other outcome can be decided, because
	// maxJobAge is the ONE bound guaranteed to terminate a row. The attempt counter
	// is not: several paths below deliberately return a row to `pending` with the
	// claim-time attempt refunded (a deploy cancelling mid-item), so `attempts` can
	// be pinned below max_attempts indefinitely and neither the claim filter nor
	// ReclaimStale's fail branch will ever retire the row. Checking age anywhere
	// further down leaves whichever branches sit above it outside that guarantee,
	// which is how a row cycles forever holding a batch slot.
	//
	// (reapExpired normally retires these before the claim; this catches a row that
	// crossed the line during the tick, and keeps the guarantee local to the one
	// function that decides a job's fate.)
	if p.jobExpired(item) {
		p.markSkipped(ctx, item, skipJobTooOld)
		return false
	}

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
		// "Not publicly visible" is the one reason that is routinely TEMPORARY, and
		// treating it as terminal costs a real user a real notification: unpublish,
		// fix a typo, republish is an ordinary correction flow, and the window is a
		// whole poll interval. A row skipped here can never be re-enqueued (the
		// whole-table UNIQUE) and PublishShow does not enqueue, so a terminal skip
		// would silently spend that show's only chance.
		//
		// So it is RETRIED, but it BURNS AN ATTEMPT doing so, which is the part that
		// matters and the part an earlier version of this code got wrong. Claim
		// orders by created_at ASC and never rewrites it, so a row that is re-pended
		// without burning an attempt returns to the head of the queue on every tick
		// forever. Enough simultaneously-unpublished shows to fill one batch would
		// then monopolise every tick, and newer announcements would sit unclaimed
		// until maxJobAge silently discarded them: a handful of admin unpublishes
		// taking out a day of everybody's notifications. Burning the attempt bounds
		// the whole failure at max_attempts ticks.
		//
		// Exhausting those attempts lands `skipped`, not `failed`: nothing
		// malfunctioned, the show simply never became visible again in time.
		//
		// The other reasons stay terminal immediately because they do not un-happen:
		// a deleted show stays deleted, a cancelled one needs a fresh announcement
		// decision rather than a resumed one, and a past date only gets further past.
		if reason == catalogm.NotAnnounceableNotPublic && item.Attempts+1 < item.MaxAttempts {
			p.retryWithAttempt(ctx, item, reason)
			return false
		}
		p.markSkipped(ctx, item, reason)
		return false
	}

	if err := p.matcher.MatchAndNotify(show); err != nil {
		p.markFailedOrRetry(ctx, item, err)
		return false
	}

	p.markDone(ctx, item)
	return true
}

// jobExpired reports whether a job has waited longer than maxJobAge. Measured from
// the row's own created_at — the moment the show became announceable — so it is a
// property of the job rather than of how many times the poller has looked at it.
func (p *ShowNotifyOutboxPoller) jobExpired(item catalogm.ShowNotifyQueueItem) bool {
	return time.Since(item.CreatedAt) > p.maxJobAge
}

// reapExpired retires aged-out pending rows AS A SET, before the claim, and this
// ordering is the whole point of the function.
//
// Retiring them one at a time inside processItem (which is also done, as a
// backstop for rows that cross the line mid-tick) is not enough on its own,
// because Claim takes the `batch` OLDEST pending rows and expired rows are by
// definition the oldest. A backlog would therefore drain at batch-per-interval
// even though none of those rows was going to be delivered: 5 a minute here, so a
// 2,000-row backlog is nearly seven hours during which no NEW show is ever
// claimed. Those new shows then cross maxJobAge while waiting and are discarded
// too, permanently, since the whole-table UNIQUE forbids re-enqueue. That inverts
// the intent exactly: the fail-safe meant to protect against a stale burst would
// instead be dropping precisely the announcements still worth sending.
//
// Clearing them in one statement makes recovery from any backlog take a single
// tick, and leaves the batch for rows that can actually be delivered.
func (p *ShowNotifyOutboxPoller) reapExpired(ctx context.Context) {
	cutoff := time.Now().Add(-p.maxJobAge)
	now := time.Now()
	res := p.db.WithContext(ctx).Model(&catalogm.ShowNotifyQueueItem{}).
		Where("status = ? AND created_at < ?", catalogm.ShowNotifyStatusPending, cutoff).
		Updates(map[string]interface{}{
			"status":       catalogm.ShowNotifyStatusSkipped,
			"last_error":   skipJobTooOld,
			"processed_at": &now,
		})
	if res.Error != nil {
		p.logger.Error("show-notify outbox: reap of expired jobs failed", "error", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		p.logger.Warn("show-notify outbox: dropped expired jobs unsent",
			"count", res.RowsAffected, "max_job_age", p.maxJobAge)
	}
}

// retryWithAttempt returns a row to `pending` and KEEPS the claim-time attempt
// increment, for a condition that may clear but is the row's own situation rather
// than the machinery's fault.
//
// The kept attempt is what makes the retry self-limiting. Claim orders by
// created_at ASC and never rewrites it, so a row re-pended for free sits at the
// head of the queue indefinitely and starves every newer job behind it. Counting
// the attempt caps that at max_attempts ticks.
func (p *ShowNotifyOutboxPoller) retryWithAttempt(ctx context.Context, item catalogm.ShowNotifyQueueItem, reason string) {
	p.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":     catalogm.ShowNotifyStatusPending,
		"last_error": reason,
	})
}

// requeueWithoutAttempt returns rows to `pending` and UNDOES the claim-time
// attempt increment.
//
// It is the single shape used for every "this is not the row's fault" outcome: a
// deploy cancelling a tick, a context error mid-item, and a show that is
// temporarily not publicly visible. All three are expected to clear on their own,
// so none may walk the row toward terminal `failed` — a row that exhausts its
// attempts during rolling restarts is a show whose followers are silently never
// told. maxJobAge, not the attempt counter, is what eventually stops the retrying.
//
// reason may be empty, in which case last_error is left as it was.
func (p *ShowNotifyOutboxPoller) requeueWithoutAttempt(ctx context.Context, items []catalogm.ShowNotifyQueueItem, reason string) {
	if len(items) == 0 {
		return
	}
	updates := map[string]interface{}{
		"status":   catalogm.ShowNotifyStatusPending,
		"attempts": gorm.Expr("GREATEST(attempts - 1, 0)"),
	}
	if reason != "" {
		updates["last_error"] = reason
	}
	p.finalize(ctx, shared.QueueRowIDs(items), updates)
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
//
// # `failed` is permanent, and the ONLY legal recovery is an UPDATE
//
// Nothing re-opens a `failed` row on its own: Claim filters on `pending`,
// ReclaimStale only touches `processing`, EnqueueShowNotify hits ON CONFLICT DO
// NOTHING against the whole-table UNIQUE, and this queue is never pruned. So a
// show whose three attempts all landed during, say, a database restart is one
// whose followers are never told — silently, unless somebody queries the table.
//
// That is the deliberate trade (failing closed on notifications beats
// re-notifying), but an operator who wants to retry such a show MUST reuse the
// existing row:
//
//	UPDATE show_notify_queue
//	   SET status = 'pending', attempts = 0, last_error = NULL
//	 WHERE id = <id>;   -- or: WHERE status = 'failed' AND updated_at > ...
//
// DELETEing the row instead is the instinctive move and the wrong one: it drops
// the "already considered" record, so any later re-ingest of that show enqueues
// afresh and can re-notify anyone the first pass had already reached. The UPDATE
// keeps one row per show, which is the invariant the whole design rests on.
func (p *ShowNotifyOutboxPoller) markFailedOrRetry(ctx context.Context, item catalogm.ShowNotifyQueueItem, cause error) {
	// A cancellation is a deploy, not a failure, and it must not count as an
	// attempt. processTick's ctx check only runs BETWEEN items, so a SIGTERM landing
	// during loadShow or MatchAndNotify surfaces here as context.Canceled — a
	// one-item-wide hole through which rolling restarts could otherwise walk a row
	// to terminal `failed` without the match ever having been tried once. That is
	// exactly the silent miss the requeue path exists to prevent.
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		p.requeueWithoutAttempt(ctx, []catalogm.ShowNotifyQueueItem{item}, cause.Error())
		p.logger.Info("show-notify outbox: canceled mid-item, requeued", "show_id", item.ShowID)
		return
	}

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
// batch (5) times one MatchAndNotify, and MatchAndNotify's cost is dominated by
// one synchronous provider request per matching recipient (filter matches and
// scene-follow fanout both send inline), which no cap in this package bounds —
// the 10/day limit is per RECIPIENT. A show matching 200 subscribers is 200
// sequential sends. Five such shows in one claim could plausibly approach the
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
