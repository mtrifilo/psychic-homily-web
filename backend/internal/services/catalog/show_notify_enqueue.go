package catalog

import (
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
)

// showNotifyEnabled gates BOTH the enqueue here and the drain in the notification
// outbox poller on one flag, deliberately: gating only the drain would let rows
// pile up while the feature is "off" and then fan them all out at once when it is
// switched back on — the exact blast this ticket exists to prevent (the same
// lesson the image-enrich outbox learned, from the other direction).
//
// Two caveats an operator reaching for the switch mid-incident needs, because the
// symmetry above is not as total as it sounds:
//
//   - It is read PER PROCESS. The server's poller is the only thing that SENDS, so
//     setting the flag there does stop outbound mail immediately. But
//     cmd/discovery-import is a separate process loading its own .env, and it will
//     keep WRITING rows unless the flag is set in that environment too.
//   - Rows already `pending` when the flag flipped survive it and drain when it is
//     cleared. The poller's maxJobAge is the backstop that keeps that from being a
//     burst: anything that waited past the window is dropped unsent, whatever its
//     event date. An operator who wants a genuinely clean restart can still clear
//     the backlog first (`DELETE FROM show_notify_queue WHERE status = 'pending'`),
//     which is safe precisely because those rows have not notified anyone.
func showNotifyEnabled() bool {
	return !shared.EnvServiceDisabled(catalogm.ShowNotifyOutboxDisableFlag)
}

// maxEnqueueShowAge is how old a show may be and still be enqueued. See the
// freshness guard in EnqueueShowNotify for why this exists and why it is not the
// deploy watermark the design rejected.
//
// One hour is chosen for the margin on both sides rather than for precision: no
// show-create transaction takes minutes, and no show in the existing catalogue is
// younger than the deploy. Anything in between is a caller doing something the
// design has not reasoned about, which is exactly what should be refused loudly.
const maxEnqueueShowAge = time.Hour

// ShowNotifyTrust answers the question this feature must not get wrong: WHO is
// allowed to cause an email to every subscriber who matches a show?
//
// It is an explicit parameter rather than something EnqueueShowNotify infers,
// because the answer is not a property of the show — two identical rows differ
// only in who wrote them — and because a silent default here is a spam vector.
// Every call site has to state its answer.
//
// # Why this gate exists
//
// PSY-1894 is about SHOWS THAT ARRIVE BY INGEST. But the funnel those arrive
// through, ShowService.CreateShow, is also the self-serve `POST /shows` endpoint,
// and determineShowStatus returns `approved` for EVERY non-private submission
// regardless of who sent it. Enqueuing unconditionally would therefore hand any
// email-verified account a button that emails every matching subscriber and scene
// follower, with an unmoderated title and up to 50 attacker-chosen artist names,
// from the platform's own DKIM-aligned sender. Before this ticket a human admin
// gated every outbound show email; enqueuing unconditionally would remove that
// gate for the entire self-serve path, which PSY-1894 never asked for.
//
// So untrusted submissions keep EXACTLY their pre-ticket behaviour: the show is
// created and publicly visible, and it notifies nobody until a human approves it
// through the admin flow (which still calls MatchAndNotify directly).
type ShowNotifyTrust int

const (
	// ShowNotifyUntrusted is a self-serve or anonymous submission. No enqueue.
	ShowNotifyUntrusted ShowNotifyTrust = iota

	// ShowNotifyIngest is an operator-driven or automated import: an admin-token
	// `ph` CLI submission, the admin markdown import, the community
	// entity-request fulfiller, or the discovery pipeline. These are the paths
	// PSY-1894 exists to unblock.
	ShowNotifyIngest
)

// EnqueueShowNotify writes a best-effort notification outbox job for a show that
// has just BECOME PUBLICLY VISIBLE, on the caller's transaction (PSY-1894).
//
// # Where this is called from, and where it deliberately is not
//
// The repo has three independent show-insert implementations, and they are not
// interchangeable in intent. This is called from the two that represent a real
// show ANNOUNCEMENT:
//
//   - ShowService.CreateShow. Be precise about what this funnel is, because the
//     answer decides who can cause mail to be sent: it backs the PUBLIC
//     `POST /shows` endpoint, which any email-verified user can call (JWT, not
//     admin-only; 10/hour/IP), AND the `ph` ingest CLI, which is the same endpoint
//     with an admin API token, AND the admin markdown import, AND the community
//     entity-request fulfiller. determineShowStatus returns `approved` for every
//     non-private submission regardless of submitter, so ALL of those are born
//     publicly visible. Only the admin-authenticated ones enqueue — see the trust
//     section below.
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
// It is NOT called from DiscoveryService.updateShowFromEvent either, and that
// omission is a decision rather than an oversight. A re-scrape that flips a show
// from cancelled back on IS arguably a new announcement, but enqueuing on any
// UPDATE path is a backfill vector: an ordinary re-scrape touches shows that have
// existed for months, including every show that predates this feature. Announcing
// a reinstatement is closer to a new alert type (PSY-1895/1896/1897) than to the
// visibility transition this ticket hooks, and it needs a signal narrow enough to
// distinguish "reinstated" from "re-scraped" before it can be safe. Left explicit
// rather than quietly missing.
//
// The admin approval transitions (ApproveShow / BatchApproveShows) keep their
// existing direct MatchAndNotify call in the handler and are not enqueued, so
// each show is considered for notification by exactly one mechanism.
//
// Those two mechanisms cannot both fire for one show TODAY, and the reason is
// worth stating precisely because it is a property of code elsewhere: ApproveShow
// accepts only `pending`/`rejected` shows, and NOTHING in the repo moves an
// `approved` show back to `pending` (UnpublishShow goes to `private`, and
// PublishShow returns it to `approved`). So a born-approved, enqueued show can
// never reach the admin approve handler at all. That is a fact about today's state
// machine, not a guarantee — which is why the per-(user, show) dedup in
// notification_log remains the real backstop, and why the outbox tests assert one
// notification when both routes run against the same show.
//
// # Only INGEST-trusted writes are enqueued
//
// The trust parameter is the gate that decides WHO may cause mail to be sent at
// all, and it is the reason the CreateShow bullet above is split rather than
// listed as one funnel. See ShowNotifyTrust: a self-serve submission through that
// same funnel gets ShowNotifyUntrusted and keeps its pre-ticket behaviour.
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
// re-enqueue a silent no-op. Be exact about what that buys, because it is narrower
// than "re-ingesting the same show does not re-notify": the guarantee is ONE
// NOTIFICATION PER SHOW ROW, at the database rather than by caller convention. A
// re-ingest that lands on the SAME row cannot re-notify, full stop.
//
// A re-ingest that mints a DUPLICATE row is a different matter — it is a second
// show as far as this table, and notification_log's per-(user, show_id) dedup, can
// tell, so it is a second notification. That is bounded upstream by the show dedup
// itself (checkDuplicateHeadlinerConflicts on create, the
// (source_venue, source_event_id) match in discovery), and cmd/dedup-shows exists
// because that dedup is not perfect: a scrape that changes a headliner's decoration
// or states a time the previous scrape left implicit can produce a second row. Such
// a duplicate was already a duplicate LISTING; it is now also a duplicate email.
// Reducing it is a show-dedup problem, not one this queue can solve.
func EnqueueShowNotify(tx *gorm.DB, show *catalogm.Show, trust ShowNotifyTrust) {
	if !showNotifyEnabled() {
		return
	}
	if trust != ShowNotifyIngest {
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

	// FRESHNESS GUARD — the structural half of the no-backfill guarantee.
	//
	// The guarantee itself is that show_notify_queue ships empty and only
	// newly-announceable shows are ever added. Until this check, "only newly
	// announceable" rested on nothing but the doc above: this function is exported,
	// takes any *Show, and one line added to PublishShow, to an UpdateShow status
	// transition, or to a backfill CLI would mass-enqueue the entire existing
	// catalogue. That is the single most damaging outcome this ticket exists to
	// prevent, and every invariant of comparable weight in this codebase has a guard
	// rather than a comment (TestShowForeignKeysAreAllHandled, the mocks-drift gate).
	//
	// This is NOT the rejected deploy-time watermark. It compares the show's own age
	// against a fixed window, so it neither knows nor cares when the feature shipped;
	// the empty table is still what makes the rollout silent. All this does is bound
	// WHO may add to the table: a caller holding a show that was created hours ago is
	// not observing a creation, and should not be able to announce one by accident.
	//
	// The window is generous relative to any real create transaction and tiny
	// relative to the catalogue's age, so it separates the two cases with enormous
	// margin. A legitimate future "announce on publish" feature (PSY-1894 explicitly
	// declines to make that call) needs its own enqueue path with its own reasoning,
	// which is the intended outcome: this refuses rather than silently allows.
	if age := time.Since(show.CreatedAt); age > maxEnqueueShowAge {
		slog.Default().Warn("show-notify enqueue refused: show is not newly created",
			"show_id", show.ID, "age", age, "max_age", maxEnqueueShowAge)
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
