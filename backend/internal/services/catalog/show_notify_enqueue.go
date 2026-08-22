package catalog

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// showNotifyEnabled gates BOTH the enqueue here and the drain in the notification
// outbox poller on one flag, deliberately: gating only the drain would let rows
// pile up while the feature is "off" and then fan them all out at once when it is
// switched back on — the exact blast this ticket exists to prevent (the same
// lesson the image-enrich outbox learned, from the other direction).
//
// Unlike ENABLE_IMAGE_ENRICH_SWEEP this defaults ON. The image-enrich outbox was
// opt-in because its consumer was not shipped yet; here the whole point of
// PSY-1894 is that ingest-created shows notify, so an opt-in default would ship a
// dormant feature and leave the acceptance criteria unmet in production.
// ENABLE_SHOW_NOTIFY_OUTBOX=0 (or "false") is the operator kill switch.
//
// Turning the switch off and back on is safe in the direction that matters: shows
// that became visible while it was off simply have no row and are never notified
// about. There is no catch-up. Rows that were already `pending` when the switch
// flipped DO survive and drain on re-enable — that backlog is bounded by what one
// poll interval had not yet reached, and the past-event rule in ShowAnnounceable
// drops any of it that went stale while the switch was off.
func showNotifyEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ENABLE_SHOW_NOTIFY_OUTBOX"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// EnqueueShowNotify writes a best-effort notification outbox job for a show that
// has just BECOME PUBLICLY VISIBLE, on the caller's transaction (PSY-1894).
//
// # Where this is called from, and where it deliberately is not
//
// The repo has three independent show-insert implementations, and they are not
// interchangeable in intent. This is called from the two that represent a real
// show ANNOUNCEMENT:
//
//   - ShowService.CreateShow — POST /shows (the `ph` ingest CLI's only write
//     path), the admin markdown show import, and the community entity-request
//     fulfiller.
//   - DiscoveryService.createShowFromEvent — cmd/discovery-import and
//     POST /admin/discovery/import.
//
// It is NOT called from DataSyncService.importShow (POST /admin/data/import).
// That path is bulk database replication — it re-imports an export of the whole
// catalogue, so hooking it would fan thousands of already-known shows out to
// subscribers on the next stage-to-prod sync. Nor from cmd/seed, which is
// development fixtures. Those two omissions are the point, not an oversight.
//
// It is also NOT called from ShowService.PublishShow (private → approved). That
// is a genuine visibility transition, but "a user un-privating their own show
// emails the whole scene" is a product decision PSY-1894 does not make; it is
// left for an explicit follow-up rather than guessed at here.
//
// The admin approval transitions (ApproveShow / BatchApproveShows) keep their
// existing direct MatchAndNotify call in the handler and are not enqueued, so
// each show is considered for notification by exactly one mechanism. Where the
// two could still overlap — a born-approved show that later re-enters the
// moderation queue — the per-(user, show) dedup in notification_log is what
// makes the second pass produce nothing user-visible.
//
// # Only announceable shows are enqueued
//
// show is the row as it was actually written, and catalogm.ShowAnnounceable is
// the single owner of the rule — the poller re-runs the SAME predicate at
// delivery time, so the two ends cannot drift. It covers three things an ingest
// path can produce that must not become an announcement: a show that is not
// publicly visible, a show the venue has already cancelled, and a show whose date
// has passed (the archival-import guard). See that function for the full argument.
//
// # Atomicity, and why a failed enqueue cannot poison the show write
//
// The insert runs in a nested transaction, which GORM emits as SAVEPOINT /
// ROLLBACK TO SAVEPOINT when the receiver is already a tx. That matters because
// in Postgres ANY failed statement aborts the WHOLE transaction: a naive failing
// insert inside the show-create tx would poison it, and the later COMMIT would
// silently become a ROLLBACK — the caller would see a phantom success (a show
// response, a nil error, and no row in the database). With the savepoint a failed
// enqueue rolls back only itself and the show still commits, at the cost of that
// show never notifying. Failing closed on notifications is the right trade
// against losing an ingest write.
//
// ON CONFLICT DO NOTHING against the whole-table UNIQUE (show_id) makes a
// re-enqueue a silent no-op, which is what gives "re-ingesting the same show does
// not re-notify" a database-level guarantee instead of a caller convention.
func EnqueueShowNotify(tx *gorm.DB, show *catalogm.Show) {
	if !showNotifyEnabled() {
		return
	}
	if tx == nil || show == nil || show.ID == 0 {
		return
	}
	if ok, reason := catalogm.ShowAnnounceable(show, time.Now()); !ok {
		slog.Default().Debug("show-notify enqueue skipped",
			"show_id", show.ID, "reason", reason)
		return
	}

	item := &catalogm.ShowNotifyQueueItem{
		ShowID: show.ID,
		Status: catalogm.ShowNotifyStatusPending,
	}
	err := tx.Transaction(func(itx *gorm.DB) error {
		return itx.Clauses(clause.OnConflict{DoNothing: true}).Create(item).Error
	})
	if err != nil {
		slog.Default().Warn("show-notify enqueue failed (show write unaffected)",
			"show_id", show.ID, "error", err)
	}
}
