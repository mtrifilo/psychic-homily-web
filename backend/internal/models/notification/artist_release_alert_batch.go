package notification

import "time"

// ArtistReleaseAlertBatchItem is one release's membership in one week's roundup.
// There is no batch record: a week's batch is the set of rows sharing AlertWeek.
//
// The key is (ArtistID, ReleaseID), not (week, artist, release), so a pair
// accrues once ever and an enrichment re-run cannot re-notify. Do not add an ID
// field: the
// table has no surrogate key, and GORM would emit RETURNING id against a column
// that does not exist.
type ArtistReleaseAlertBatchItem struct {
	// ArtistID is the credited, followed artist. Half of the accrual key.
	ArtistID uint `gorm:"column:artist_id" json:"artist_id"`

	// ReleaseID is the release that became visible. The other half.
	ReleaseID uint `gorm:"column:release_id" json:"release_id"`

	// AlertWeek is the Monday, in UTC, of the ISO week this observation belongs
	// to: the notification's bucket, fixed at first observation and not part of
	// the accrual key. UTC because a release has no location.
	AlertWeek time.Time `gorm:"column:alert_week" json:"alert_week"`

	// CreatedAt is when this release was accrued, not when the week opened. The
	// flush schedules on the week bucket, not on this column.
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`

	// DispatchedAt marks that a flush has resolved this row's week. Per row, so a
	// late arrival can be marked without reopening a closed week.
	//
	// It is bookkeeping, NOT the exactly-once guard: a late member makes the
	// flush re-resolve a delivered week and reach the claim again, and
	// uq_notification_log_artist_release_digest makes that silent. Do not read
	// it as "already sent, skip".
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
// Spelled once, here in models, because both halves read it from different
// packages (services/catalog accrues, services/notification delivers). Gating
// only the flush would accrue rows that fan out in one burst when the flag
// clears. Mirrors VenueShowAlertsDisableFlag.
const ArtistReleaseAlertsDisableFlag = "DISABLE_ARTIST_RELEASE_ALERTS"
