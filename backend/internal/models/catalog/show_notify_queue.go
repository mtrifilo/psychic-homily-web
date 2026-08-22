package catalog

import "time"

// Show-notify outbox status values (PSY-1894).
//
//	pending → processing (claimed) → done   — MatchAndNotify ran for the show
//	                               → skipped — the show is gone or no longer visible
//	                               → pending (retry) → failed once attempts run out
//
// `skipped` is a distinct terminal state rather than a `done` with an excuse in
// last_error: "we deliberately did not notify" and "we notified" are different
// facts, and conflating them would make the one query an operator actually wants
// ("did this show's followers get told?") unanswerable.
const (
	ShowNotifyStatusPending    = "pending"
	ShowNotifyStatusProcessing = "processing"
	ShowNotifyStatusDone       = "done"
	ShowNotifyStatusSkipped    = "skipped"
	ShowNotifyStatusFailed     = "failed"
)

// ShowNotifyQueueItem is a transactional-outbox job (PSY-1894): a row enqueued in
// the SAME transaction as a show that becomes publicly visible outside the admin
// approval flow, then drained by the notification outbox poller, which runs the
// existing MatchAndNotify path.
//
// Two properties of this table are load-bearing and are enforced by the schema,
// not by caller discipline (see the migration for the full argument):
//
//   - NO BACKFILL. The table ships empty and nothing ever populates it for a
//     pre-existing show, so the rollout cannot notify anyone about the catalogue
//     that already exists. The guarantee is structural (an empty table), not a
//     watermark or a timestamp comparison — `shows` has no `approved_at` column,
//     so there is no honest clock to compare against.
//   - ONE ROW PER SHOW, EVER. uq_show_notify_queue_show is a whole-table UNIQUE
//     (not the active-states partial index image_enrich_queue uses), so a
//     terminal row permanently blocks re-enqueue. Re-ingesting or editing a show
//     is an ON CONFLICT DO NOTHING no-op. That also means this table is never
//     pruned: deleting a terminal row would re-open re-notification.
//
// It lives in models/catalog (not the notification service package) so the show
// write funnels can enqueue without importing notification, which would cycle.
// Mirrors ImageEnrichQueueItem's shape, minus the polymorphic entity_type — this
// queue is about shows only, so show_id is a real FK.
type ShowNotifyQueueItem struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	ShowID      uint       `json:"show_id" gorm:"column:show_id;not null"`
	Status      string     `json:"status" gorm:"column:status;not null;default:'pending'"`
	Attempts    int        `json:"attempts" gorm:"column:attempts;not null;default:0"`
	MaxAttempts int        `json:"max_attempts" gorm:"column:max_attempts;not null;default:3"`
	LastError   *string    `json:"last_error" gorm:"column:last_error"`
	CreatedAt   time.Time  `json:"created_at" gorm:"not null"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"not null"`
	ProcessedAt *time.Time `json:"processed_at" gorm:"column:processed_at"`
}

func (ShowNotifyQueueItem) TableName() string { return "show_notify_queue" }

// Reasons a show is not announceable. They are recorded on a `skipped` row, so
// an operator asking "why did this show's followers hear nothing?" gets an answer
// from the queue rather than from re-deriving the rule.
const (
	NotAnnounceableGone      = "show no longer exists"
	NotAnnounceableNotPublic = "show is not publicly visible"
	NotAnnounceableCancelled = "show is cancelled"
	NotAnnounceablePastEvent = "show has already happened"
)

// ShowAnnounceable reports whether a show is worth telling followers about, and
// the reason when it is not.
//
// ONE predicate, used at BOTH ends of the notification outbox (PSY-1894): the
// write funnel calls it to decide whether to enqueue, and the poller calls it
// again at delivery time. Two copies would drift, and the drift would be
// invisible — the enqueue side is what stops a notification being created and the
// delivery side is what stops one going out, so a rule present in only one of
// them silently half-works.
//
// It lives on the model rather than in either service because both callers
// already import this package and neither should have to import the other.
//
// The rules, and why each one is here rather than assumed:
//
//   - Publicly visible. Only `approved` shows are on the site; announcing a
//     `private`, `pending`, or `rejected` one would either leak a user's own list
//     or advertise something moderation has not passed.
//   - Not cancelled. Discovery scrapes cancellation straight off venue calendars
//     (createShowFromEvent sets IsCancelled from the feed, and also recovers it
//     from a "*CANCELLED*" title marker), and still writes the row `approved`.
//     Without this rule an automated import would email "New show" for an event
//     the venue has already called off. Sold-out is deliberately NOT a blocker: a
//     sold-out show is real information a follower still wants.
//   - Not already over. This is the guard against an ARCHIVAL ingest. CreateShow
//     does no past-date validation, so importing a venue's back catalogue through
//     POST /shows or the admin markdown import would otherwise fan thousands of
//     years-old listings out as new announcements — the same catastrophe as a
//     rollout backfill, arriving through a different door. It also cleans up after
//     a long poller outage: a job that waited past its own event date is dropped
//     rather than delivered stale.
//
// now is a parameter rather than time.Now() so the rule is testable and so the
// caller decides the clock. Note this is a DOMAIN comparison (event date against
// the present), not the deploy-time watermark the outbox deliberately avoids: the
// no-backfill guarantee rests on the queue being empty, and does not depend on
// this function at all.
func ShowAnnounceable(show *Show, now time.Time) (bool, string) {
	if show == nil {
		return false, NotAnnounceableGone
	}
	if show.Status != ShowStatusApproved {
		return false, NotAnnounceableNotPublic
	}
	if show.IsCancelled {
		return false, NotAnnounceableCancelled
	}
	if show.EventDate.Before(now) {
		return false, NotAnnounceablePastEvent
	}
	return true, ""
}

// QueueRowID implements shared.QueueRow so the shared job-queue mechanics
// (PSY-1572) can address this table's rows.
func (i ShowNotifyQueueItem) QueueRowID() uint { return i.ID }
