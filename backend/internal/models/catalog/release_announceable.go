package catalog

import "time"

// ReleaseAnnounceable is the ONE predicate for "this release is NEW MUSIC worth
// telling a follower about" (PSY-1897). It is consulted at accrual, again at
// flush, and again when the inbox row renders, so that three surfaces cannot
// drift into disagreeing about what the alert covers — the same role
// ShowAnnounceable plays for the show loops.
//
// # Why a predicate is needed at all, when shows get theirs for free
//
// A show has a visibility transition: it sits in a status until moderation or
// ingest approves it, and ShowAnnounceable reads that status. A RELEASE HAS NO
// SUCH TRANSITION. There is no status column, no approved flag and no soft
// delete on `releases`; the public list and detail queries carry no visibility
// predicate whatsoever, so a release is readable by the world the instant its
// row is INSERTed. Insert IS publication.
//
// That removes the obvious trigger and leaves a real hazard in its place. The
// row's created_at is the moment Psychic Homily first SAW the record, not the
// moment the world got it — the discography importer stamps today's created_at
// on a 1998 LP, and a `ph submit-release` pass over a label's back catalogue
// mints hundreds of them inside a few minutes. Keyed naively on "a row appeared",
// the weekly roundup's first real ingest run would mail a band's followers a
// digest of twenty years of records described as new.
//
// So "new" is defined from the WORLD date the release carries, not from when we
// noticed it.
//
// # The rule
//
//	release_date present  -> announceable when it is no more than
//	                         ReleaseRecencyWindow old, and not absurdly far ahead
//	release_date absent,
//	release_year present  -> announceable when the year is the CURRENT year
//	                         or later
//	neither present       -> NOT announceable
//
// Each branch, and why it fails the way it does:
//
//   - FUTURE DATES ARE ANNOUNCEABLE. A record announced today for release in
//     three months is news to somebody who follows the band, and the alert is
//     about the ANNOUNCEMENT — the same framing the venue alert uses when it
//     keys on announcement day rather than event date. (Note this deliberately
//     differs from the charts "new releases" module, which hides a future-dated
//     release until its day. That module answers "what is out now"; this one
//     answers "what is new".) Bounded by ReleaseFutureWindow so a data-entry
//     typo of the year 9999 cannot mail anyone.
//
//   - YEAR-ONLY IS THE CURRENT YEAR ONLY. A year is a twelve-month smear, so the
//     recency window cannot be applied to it without picking an arbitrary day
//     inside it. Requiring the current year is the crisp version of the same
//     intent, and it errs in the safe direction: in January it drops records
//     stamped with only last December's year, which costs a narrow slice of real
//     alerts rather than risking a back-catalogue blast.
//
//   - UNDATED IS NEVER ANNOUNCEABLE, and this is the load-bearing one. With
//     neither field set there is no way to tell a record released yesterday from
//     one released in 1998, and the ingest paths that omit both are precisely
//     the bulk label-discography runs this predicate exists to survive. Failing
//     closed loses some genuine alerts; failing open mails a band's entire
//     following about a catalogue nobody described as new. Only one of those is
//     recoverable.
//
// Returns a reason string alongside the verdict, matching ShowAnnounceable, so a
// caller can log WHY a release was skipped rather than logging that it was.
func ReleaseAnnounceable(release *Release, now time.Time) (bool, string) {
	if release == nil {
		return false, NotAnnounceableGone
	}

	if release.ReleaseDate != nil && *release.ReleaseDate != "" {
		// Stored as a DATE but modelled as a string (see Release.ReleaseDate), so
		// it is parsed here rather than trusted. An unparseable value is treated
		// as absent and falls through to the year branch, which is the same
		// fail-closed direction the rest of this predicate takes.
		if day, err := time.Parse(releaseDateLayout, *release.ReleaseDate); err == nil {
			if day.Before(now.Add(-ReleaseRecencyWindow)) {
				return false, NotAnnounceableBackCatalogue
			}
			if day.After(now.Add(ReleaseFutureWindow)) {
				return false, NotAnnounceableImplausibleDate
			}
			return true, ""
		}
	}

	if release.ReleaseYear != nil {
		if *release.ReleaseYear >= now.Year() {
			return true, ""
		}
		return false, NotAnnounceableBackCatalogue
	}

	return false, NotAnnounceableUndated
}

// releaseDateLayout is how releases.release_date is spelled. The column is a
// DATE and the model holds it as a string precisely so no zone can reinterpret
// it; parsing it with a date-only layout keeps that property.
const releaseDateLayout = "2006-01-02"

// ReleaseRecencyWindow is how old a dated release may be and still count as new.
//
// Six months, chosen to absorb ingest lag rather than to describe a release
// cycle. A record genuinely reaches the catalogue weeks or months after it came
// out — a label pass runs when someone runs it, not on release day — and a
// window shorter than that would silently drop the ordinary case this feature
// exists for. It is long enough that being wrong costs a reader a mildly stale
// line in a roundup, and short enough that no back catalogue fits through it.
const ReleaseRecencyWindow = 180 * 24 * time.Hour

// ReleaseFutureWindow bounds how far ahead a release date may sit.
//
// Not a product rule about pre-orders — those are wanted, and a year is more
// than any of them need. It is a guard against a typo'd or provider-mangled year
// turning into an alert that can never be superseded, since accrual records a
// pair once ever.
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
