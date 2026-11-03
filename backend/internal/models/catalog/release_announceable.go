package catalog

import "time"

// ReleaseAnnounceable is the one predicate for "this release is new music worth
// telling a follower about". Accrual, the flush and the inbox enrichment SQL all
// apply it (the SQL through ReleaseAnnounceableBounds), so those surfaces cannot
// disagree about what the alert covers; ShowAnnounceable plays the same role for
// the show loops.
//
// A release has no visibility transition (insert is publication), and its
// created_at is when the catalogue first saw it, not when the world did: a
// discography import stamps today on a 1998 LP. So "new" is defined from the
// release's own date:
//
//	release_date present  -> announceable when no more than ReleaseRecencyWindow
//	                         old and no more than ReleaseFutureWindow ahead
//	release_date absent,
//	release_year present  -> announceable when the year is the current UTC year
//	                         or later
//	neither present       -> NOT announceable
//
// Future dates are announceable because the alert is about the announcement
// (unlike the charts "new releases" module, which answers "what is out now").
// Year-only cannot take the day window, so it requires the current year and
// errs toward dropping. Undated fails closed: bulk label-discography runs are
// exactly the rows that omit both fields.
//
// The reason string lets a caller log why a release was skipped.
func ReleaseAnnounceable(release *Release, now time.Time) (bool, string) {
	if release == nil {
		return false, NotAnnounceableGone
	}

	oldest, latest, minYear := ReleaseAnnounceableBounds(now)

	if release.ReleaseDate != nil && *release.ReleaseDate != "" {
		// Stored as a DATE but modelled as a string (see Release.ReleaseDate), so
		// it is parsed here rather than trusted. An unparseable value is treated
		// as absent and falls through to the year branch, which is the same
		// fail-closed direction the rest of this predicate takes.
		if parsed, err := time.Parse(releaseDateLayout, *release.ReleaseDate); err == nil {
			// Canonical YYYY-MM-DD strings order the same as the days they name.
			day := parsed.Format(releaseDateLayout)
			if day < oldest {
				return false, NotAnnounceableBackCatalogue
			}
			if day > latest {
				return false, NotAnnounceableImplausibleDate
			}
			return true, ""
		}
	}

	if release.ReleaseYear != nil {
		if *release.ReleaseYear >= minYear {
			return true, ""
		}
		return false, NotAnnounceableBackCatalogue
	}

	return false, NotAnnounceableUndated
}

// ReleaseAnnounceableBounds returns the inclusive window ReleaseAnnounceable
// applies at now, as calendar days in UTC: the oldest and latest announceable
// release_date (YYYY-MM-DD) and the lowest announceable year-only release_year.
//
// Day-granular because release_date is a DATE: comparing it against an instant
// would make the window's edge days depend on the hour and on the zone of the
// caller. SQL restatements of the predicate bind these values so they cannot
// disagree with it.
func ReleaseAnnounceableBounds(now time.Time) (oldest, latest string, minYear int) {
	utc := now.UTC()
	return utc.Add(-ReleaseRecencyWindow).Format(releaseDateLayout),
		utc.Add(ReleaseFutureWindow).Format(releaseDateLayout),
		utc.Year()
}

// releaseDateLayout is how releases.release_date is spelled. The column is a
// DATE and the model holds it as a string precisely so no zone can reinterpret
// it; parsing it with a date-only layout keeps that property.
const releaseDateLayout = "2006-01-02"

// ReleaseRecencyWindow is how old a dated release may be and still count as new.
// Sized to absorb ingest lag (records reach the catalogue weeks or months after
// release) while keeping back catalogue out.
const ReleaseRecencyWindow = 180 * 24 * time.Hour

// ReleaseFutureWindow bounds how far ahead a release date may sit. Pre-orders are
// wanted; this guards against a mangled year becoming an alert that can never be
// superseded, since accrual records a pair once ever.
const ReleaseFutureWindow = 365 * 24 * time.Hour

// Reasons a release is not announceable. NotAnnounceableGone is shared with the
// show predicate and lives beside it; these are the release-specific ones.
const (
	// NotAnnounceableBackCatalogue: the release is dated, and dated too long ago.
	NotAnnounceableBackCatalogue = "back_catalogue"
	// NotAnnounceableUndated: no release_date and no release_year, so there is no
	// evidence it is new.
	NotAnnounceableUndated = "undated"
	// NotAnnounceableImplausibleDate: dated so far ahead that it is almost
	// certainly wrong.
	NotAnnounceableImplausibleDate = "implausible_date"
)
