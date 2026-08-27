package catalog

import (
	"log/slog"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/shared"
)

// Accrual for the artist new-release weekly roundup (PSY-1897).
//
// # Why this lives here and not in services/notification
//
// The venue sibling accrues inside MatchAndNotify because shows have a
// visibility TRANSITION to hang a hook on, and MatchAndNotify already runs from
// both routes that decide a show is announceable. Releases have no such
// transition — there is no status column on `releases` and the public read path
// carries no visibility predicate, so a release is world-readable the instant
// its row is INSERTed.
//
// The hook therefore has to be the INSERT, and the insert has exactly one
// funnel: createReleaseTx. Every API, CLI and importer write reaches it —
// ReleaseService.CreateRelease and FindOrCreateReleaseByReleaseGroupMBID are its
// only callers, and they are in turn the only things in the codebase that create
// a release outside seeds and test fixtures. Hooking one function is what makes
// "no write path can be missed" a property of the call graph rather than a claim
// in a comment.
//
// enqueueImageEnrich already sits in that same funnel doing the same kind of
// thing, so the shape is precedented rather than invented.
//
// # What the seed and fixture paths do, and why that is right
//
// cmd/seed, cmd/seed/exemplars and frontend/e2e/setup-db.sh insert releases with
// raw statements that bypass createReleaseTx entirely. They therefore accrue
// nothing, which is the correct outcome: a dev seed is not an announcement. It
// also means an integration test has to go through the funnel to exercise this.
//
// # No backfill, structurally
//
// Starting this on a deploy cannot alert anyone about the existing catalogue:
// artist_release_alert_batch ships EMPTY and the only thing that writes to it is
// this function, which runs when a release is created. A merge can move rows but
// never manufacture one.

// artistReleaseFollowEntityType is the bookmark entity type these alerts
// subscribe on. Read from the model rather than spelled as a literal, matching
// the artist and venue alert loops: it is half of the WHERE clause that finds
// the followers AND the value engagement's resolver compares against.
var artistReleaseFollowEntityType = string(engagementm.BookmarkEntityArtist)

// alertWeekLayout is how a week bucket is spelled everywhere it crosses the
// Go/SQL boundary.
//
// Weeks are passed to Postgres as STRINGS with an explicit ::date cast, never as
// a time.Time. A time.Time parameter arrives as a timestamptz and is cast to
// date using the SESSION time zone, so the same instant would land on different
// days depending on a setting this package does not control. A literal
// 'YYYY-MM-DD' has no such dependency.
const alertWeekLayout = "2006-01-02"

// ArtistReleaseAlertsEnabled reports whether the release-roundup loop is on.
//
// Read by BOTH halves — this accrual and the flush poller in
// services/notification — through the one flag name in models/notification.
func ArtistReleaseAlertsEnabled() bool {
	return !shared.EnvServiceDisabled(notificationm.ArtistReleaseAlertsDisableFlag)
}

// AlertWeekStart returns the Monday, at midnight UTC, of the ISO week containing
// t.
//
// Exported because the flush, the inbox enrichment and the tests all have to
// agree with accrual about where a week begins, and three copies of a weekday
// calculation is three chances to be off by one. Monday because that is what
// date_trunc('week', ...) means in Postgres, so the Go and SQL halves of this
// feature name the same day.
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
// Best-effort, and that is enforced structurally rather than by discipline: the
// insert runs in a nested tx.Transaction, which GORM emits as a SAVEPOINT when
// the receiver is already a transaction. Postgres aborts the WHOLE transaction
// on any failed statement, so without the savepoint a failing insert here would
// poison the caller's tx and turn the release create into a phantom success —
// non-nil release, nil error, no row. With it, a failure rolls back only the
// savepoint. This is the same reasoning enqueueImageEnrich documents, and the
// same reasoning FindOrCreateReleaseByReleaseGroupMBID's step 3 relies on.
//
// Two write-side bounds, and they do different jobs:
//
//   - ONLY FOLLOWED ARTISTS ACCRUE. This bounds a table that is deliberately
//     never pruned (the inbox row reads its members live, so deleting them would
//     blank delivered history). Be precise about what it does NOT do: it gates
//     on whether the artist has ANY follower, and the flush resolves the
//     recipient list with no created_at filter. So someone who follows the band
//     later that same week receives that week's whole roundup including records
//     accrued before they subscribed — intended, since they have just declared
//     interest in exactly that artist. But an artist with NO follower at insert
//     time accrues nothing at all, so their FIRST follower gets nothing for a
//     drop that landed before they arrived. That is the price of the bound.
//
//   - ONLY ANNOUNCEABLE RELEASES ACCRUE. This is the back-catalogue fence, and
//     it is the reason a `ph submit-release` pass over a label's twenty-year
//     discography does not mail every follower of every artist on that label.
//     See catalogm.ReleaseAnnounceable for the definition and for why an undated
//     release fails closed. The flush re-checks with the same predicate, because
//     release_date is editable between accrual and delivery.
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

	// One statement for every credited artist rather than a loop of inserts. A
	// release can credit a dozen artists on a compilation, this runs inside the
	// create transaction of a bulk ingest, and the follower check is the same
	// EXISTS for all of them.
	//
	// ON CONFLICT DO NOTHING against the (artist_id, release_id) primary key is
	// the whole re-notify story: a re-import, an enrichment pass or a metadata
	// correction that reaches this funnel again finds the pair already recorded
	// and writes nothing. It also settles the concurrent-create race without a
	// separate guard, which the venue sibling needed because its natural key
	// included the day.
	err := tx.Transaction(func(itx *gorm.DB) error {
		return itx.Exec(`
			INSERT INTO artist_release_alert_batch (artist_id, release_id, alert_week, created_at)
			SELECT a.id, ?, ?::date, ?
			FROM artists a
			-- IN ? rather than = ANY(array): GORM expands this into one bind
			-- parameter per element, which is fine here and only here — the list
			-- is the artists credited on ONE release, a handful even on a
			-- compilation. The flush, whose lists are follower-sized, uses arrays.
			WHERE a.id IN ?
			  AND EXISTS (
			        SELECT 1 FROM user_bookmarks b
			        WHERE b.entity_type = ?
			          AND b.action = ?
			          AND b.entity_id = a.id
			      )
			ON CONFLICT DO NOTHING
		`,
			release.ID, week, now,
			artistIDs,
			artistReleaseFollowEntityType, string(engagementm.BookmarkActionFollow),
		).Error
	})
	if err != nil {
		slog.Default().Warn("release-alert accrual failed (release create unaffected)",
			"release_id", release.ID, "week", week, "error", err)
	}
}
