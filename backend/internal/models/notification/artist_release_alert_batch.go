package notification

import "time"

// ArtistReleaseAlertBatchItem is one release's membership in one week's roundup
// (PSY-1897).
//
// There is no batch record and no batch id. A WEEK's batch is the set of rows
// sharing AlertWeek — this type is a MEMBERSHIP, and every query against it
// works on groups rather than on single rows.
//
// The key is (ArtistID, ReleaseID) and NOT (week, artist, release). A pair
// accrues once, EVER, which is what makes "an enrichment re-run does not
// re-notify" a property of the schema rather than of a guard someone has to
// remember to write. AlertWeek is a payload column fixed at first observation.
//
// GORM has no composite-primary-key struct tag story worth relying on here, so
// this type is used for READS and for a hand-written ON CONFLICT DO NOTHING
// insert. Do not add an ID field: the table has no surrogate key, and a phantom
// one would silently produce `INSERT ... RETURNING id` against a column that
// does not exist.
type ArtistReleaseAlertBatchItem struct {
	// ArtistID is the credited, followed artist. Half of the accrual key.
	ArtistID uint `gorm:"column:artist_id" json:"artist_id"`

	// ReleaseID is the release that became visible. The other half.
	ReleaseID uint `gorm:"column:release_id" json:"release_id"`

	// AlertWeek is the Monday, in UTC, of the ISO week this observation belongs
	// to. It is the notification's bucket, not part of its accrual key.
	//
	// UTC rather than a local zone because a release has no location: the artist
	// may be in Oslo and the follower in Phoenix, and there is no third clock
	// that is more correct than the one both can name. A week is also 168 hours
	// wide, so an offset can move at most one of them across the boundary — the
	// venue alert's 24-hour day is why the same offset was load-bearing there.
	AlertWeek time.Time `gorm:"column:alert_week" json:"alert_week"`

	// CreatedAt is when this RELEASE was accrued, not when the week opened. The
	// flush needs to know how long a week has been sitting undispatched, which is
	// a question about individual rows.
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`

	// DispatchedAt marks that a flush has resolved the week this row belongs to.
	// Per row, so a late arrival can be recorded and then marked without anything
	// having to reopen a closed week.
	//
	// It is NOT the exactly-once guard. A late member makes the flush re-resolve
	// a week that already delivered, so the flush WILL reach the delivery claim
	// again; what makes that silent is
	// uq_notification_log_artist_release_digest. Reading this column as "already
	// sent, skip" would be wrong in both directions: it would drop the late
	// release from the inbox row, and it would not by itself prevent anything.
	DispatchedAt *time.Time `gorm:"column:dispatched_at" json:"dispatched_at,omitempty"`
}

// TableName specifies the table name for ArtistReleaseAlertBatchItem.
func (ArtistReleaseAlertBatchItem) TableName() string {
	return "artist_release_alert_batch"
}

// ArtistReleaseAlertsDisableFlag is the kill switch for the whole release-alert
// loop: accrual inside the release create funnel and the flush poller that
// delivers.
//
// Spelled once, here in models, because BOTH halves must read the same name and
// they live in different packages (services/catalog accrues, services/notification
// delivers). Gating only one side is a trap in either direction — gating only the
// flush accrues rows that fan out in one burst when the flag clears, and gating
// only accrual leaves a poller spinning on a table nothing writes to. This
// mirrors VenueShowAlertsDisableFlag and catalogm.ShowNotifyOutboxDisableFlag,
// which exist for exactly the same reason.
const ArtistReleaseAlertsDisableFlag = "DISABLE_ARTIST_RELEASE_ALERTS"
