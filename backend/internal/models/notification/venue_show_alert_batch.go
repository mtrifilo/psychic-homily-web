package notification

import "time"

// VenueShowAlertBatchItem is one show's membership in one venue-day's alert
// batch (PSY-1895).
//
// There is no batch record and no batch id. A BATCH is the set of rows sharing
// (VenueID, AlertDay) — this type is a MEMBERSHIP, and every query against it
// works on groups rather than on single rows.
//
// Modelling it as a set rather than as a parent row with children is what keeps
// the two structural operations cheap. A venue merge and a show dedup are then
// ordinary id re-points against a unique key, the same shape the merge
// inventory in services/catalog already performs on a dozen other tables; and a
// show announced after the batch was already delivered is just another member,
// with nothing to reopen and no parent row whose "closed" flag would have to be
// reasoned about.
//
// GORM has no composite-primary-key struct tag story worth relying on here, so
// this type is used for READS and for a hand-written ON CONFLICT DO NOTHING
// insert. Do not add an ID field: the table has no surrogate key, and a
// phantom one would silently produce `INSERT ... RETURNING id` against a
// column that does not exist.
type VenueShowAlertBatchItem struct {
	// VenueID is the followed venue. Half of the batch key.
	VenueID uint `gorm:"column:venue_id" json:"venue_id"`

	// AlertDay is the calendar day the show was ANNOUNCED, in the VENUE's local
	// zone. The other half of the batch key.
	//
	// Announcement day, not event date: a single ingest drop covering a season
	// of dates is ONE announcement, and keying on event date would scatter it
	// across a season of batches and defeat the coalescing entirely.
	//
	// Venue-local, not UTC: 5pm in Phoenix is already tomorrow in UTC, so a UTC
	// key would cut an ordinary evening ingest in half and send two alerts for
	// what everyone involved experienced as one drop.
	AlertDay time.Time `gorm:"column:alert_day" json:"alert_day"`

	// ShowID is the announced show.
	ShowID uint `gorm:"column:show_id" json:"show_id"`

	// CreatedAt is when this SHOW was accrued, not when the batch opened. The
	// flush poller waits for a quiet window, which is a question about the most
	// recent member, so the timestamp has to be per row.
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`

	// DispatchedAt marks that a flush has resolved the group this row belongs
	// to. Per row, so a late arrival can be recorded and then marked without
	// anything having to reopen a closed batch.
	//
	// It is NOT the exactly-once guard. A late member makes the flush re-resolve
	// a group that already delivered, so the flush WILL reach the delivery claim
	// again; what makes that silent is uq_notification_log_venue_show_alert.
	// Reading this column as "already sent, skip" would be wrong in both
	// directions: it would drop the late show from the inbox row, and it would
	// not by itself prevent anything.
	DispatchedAt *time.Time `gorm:"column:dispatched_at" json:"dispatched_at,omitempty"`
}

// TableName specifies the table name for VenueShowAlertBatchItem.
func (VenueShowAlertBatchItem) TableName() string {
	return "venue_show_alert_batch"
}

// VenueShowAlertsDisableFlag is the kill switch for the whole venue-alert loop:
// accrual inside MatchAndNotify and the flush poller that delivers.
//
// Spelled once, here in models, because BOTH halves must read the same name and
// they live in different packages. Gating only one side is a trap in either
// direction — gating only the flush accrues rows that fan out in one burst when
// the flag clears, and gating only accrual leaves a poller spinning on a table
// nothing writes to. This mirrors catalogm.ShowNotifyOutboxDisableFlag, which
// exists for exactly the same reason.
const VenueShowAlertsDisableFlag = "DISABLE_VENUE_SHOW_ALERTS"
