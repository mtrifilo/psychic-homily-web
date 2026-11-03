package catalog

import (
	"log/slog"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/shared"
)

// Accrual for the artist new-release weekly roundup.
//
// Releases have no visibility transition (insert is publication), so the hook is
// the insert itself, inside createReleaseTx. Its callers, CreateRelease and
// FindOrCreateReleaseByReleaseGroupMBID, are the only service-layer release
// creators; the seed commands and e2e fixtures bypass it and correctly accrue
// nothing, so an integration test must go through the funnel.
//
// No backfill: artist_release_alert_batch ships empty and only this function
// writes to it. A merge can move rows but never manufacture one.

// artistReleaseFollowEntityType is the bookmark entity type these alerts
// subscribe on, read from the model so the follower WHERE clause and
// engagement's resolver compare the same value.
var artistReleaseFollowEntityType = string(engagementm.BookmarkEntityArtist)

// alertWeekLayout is how a week bucket is spelled where it crosses the Go/SQL
// boundary. Weeks are passed as strings with an explicit ::date cast, never as a
// time.Time, which Postgres would cast to date in the SESSION time zone.
const alertWeekLayout = "2006-01-02"

// ArtistReleaseAlertsEnabled reports whether the release-roundup loop is on.
// Accrual and the flush poller read the same flag name from models/notification.
func ArtistReleaseAlertsEnabled() bool {
	return !shared.EnvServiceDisabled(notificationm.ArtistReleaseAlertsDisableFlag)
}

// AlertWeekStart returns the Monday, at midnight UTC, of the ISO week containing
// t.
//
// Monday because that is what date_trunc('week', ...) means in Postgres, so the
// Go and SQL halves name the same day.
func AlertWeekStart(t time.Time) time.Time {
	utc := t.UTC()
	// time.Weekday is Sunday=0; shift so Monday=0 and Sunday=6.
	offset := (int(utc.Weekday()) + 6) % 7
	day := utc.AddDate(0, 0, -offset)
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
}

// enqueueArtistReleaseAlerts records this release's membership in each credited,
// FOLLOWED artist's roundup for the current week.
//
// Best-effort by construction: the insert runs in a nested tx.Transaction, which
// GORM emits as a SAVEPOINT. Postgres aborts the whole transaction on any failed
// statement, so without the savepoint a failing insert would poison the
// caller's tx and make the release create a phantom success. Same reasoning as
// enqueueImageEnrich.
//
// Two write-side bounds:
//
//   - Only artists with at least one follower accrue, which bounds a table that
//     is never pruned. The flush resolves recipients without a follow-date
//     filter, so a reader who follows mid-week gets that week's whole roundup;
//     an artist with no follower at insert time accrues nothing at all.
//   - Only announceable releases accrue (catalogm.ReleaseAnnounceable): the
//     back-catalogue fence. The flush re-checks, because release_date is
//     editable between accrual and delivery.
func enqueueArtistReleaseAlerts(tx *gorm.DB, release *catalogm.Release, artistIDs []uint) {
	if release == nil || len(artistIDs) == 0 {
		return
	}
	if !ArtistReleaseAlertsEnabled() {
		return
	}

	now := time.Now()
	if ok, reason := catalogm.ReleaseAnnounceable(release, now); !ok {
		slog.Default().Debug("release-alert accrual skipped",
			"release_id", release.ID, "reason", reason)
		return
	}

	week := AlertWeekStart(now).Format(alertWeekLayout)

	// One statement for every credited artist. ON CONFLICT DO NOTHING against
	// the (artist_id, release_id) primary key means a re-import or correction
	// that reaches this funnel again writes nothing, and settles the
	// concurrent-create race.
	//
	// The SQL carries no comments: GORM binds every literal question mark in
	// the text, a comment's included.
	err := tx.Transaction(func(itx *gorm.DB) error {
		return itx.Exec(`
			INSERT INTO artist_release_alert_batch (artist_id, release_id, alert_week, created_at)
			SELECT a.id, ?, ?::date, ?
			FROM artists a
			WHERE a.id = ANY(?)
			  AND EXISTS (
			        SELECT 1 FROM user_bookmarks b
			        WHERE b.entity_type = ?
			          AND b.action = ?
			          AND b.entity_id = a.id
			      )
			ON CONFLICT DO NOTHING
		`,
			release.ID, week, now,
			pq.Array(artistIDs),
			artistReleaseFollowEntityType, string(engagementm.BookmarkActionFollow),
		).Error
	})
	if err != nil {
		slog.Default().Warn("release-alert accrual failed (release create unaffected)",
			"release_id", release.ID, "week", week, "error", err)
	}
}
