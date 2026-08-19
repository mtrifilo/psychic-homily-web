package catalog

import (
	"fmt"

	"psychic-homily-backend/internal/services/contracts"
)

// ──────────────────────────────────────────────
// Headline-slot classification (chart/stats reads)
// ──────────────────────────────────────────────
//
// There is no schema-level definition of "headliner": this file IS it for
// every read that CLASSIFIES bill rows into headline vs support slots.
//
// The rule, in one place on purpose. Five hand-rolled copies of a
// position-only heuristic are how PSY-1704 happened: PSY-1673 gave
// show_artists.set_type curated semantics, the copies kept reading list
// order, and a first-billed act a human had curated as `opener` still
// counted as a headliner and was therefore excluded from Openers to Watch.
//
//	CURATED BILL   -- some row on the show states a real slot (any set_type
//	                  outside "" / NULL / 'performer', the two spellings of
//	                  "slot unknown"). The headline slot is then EXACTLY the
//	                  rows curated 'headliner', and nothing is inferred from
//	                  list order. A bill whose curator named an opener but no
//	                  headliner therefore has no headline slot, which is the
//	                  honest reading: somebody described this bill and did not
//	                  say anyone topped it.
//	UNCURATED BILL -- every row is "slot unknown". Position 0 is read as the
//	                  headline slot, as before.
//
// The fallback is deliberate and is the decision recorded on PSY-1704: most
// of the corpus predates the curation surface (the PSY-1673 backfill folded
// the meaningless hardcoded 'opener' default to 'performer'), so dropping it
// would empty the role-based charts on day one rather than correcting them.
// It is per-bill, not global, so one curated bill never changes how another
// bill is read, and a bill gets the position heuristic only for as long as
// nobody has said anything better about it.
//
// Note that the write path already keeps partial curation rare: resolveArtistRole
// stamps set_type='headliner' on position 0 whenever the caller states no role
// at all, so a curated bill missing a headliner means a curator actively
// chose some other role for the top of the bill.
//
// NOT covered here, deliberately: reads that RESOLVE THE ONE headliner row of
// a single show for display or dedup (tag_service.enrichShows,
// explore.go, show_dedup.go, show.go's duplicate-headliner guard,
// pipeline/discovery.go's dedup lookup). Those order by
// `set_type = 'headliner'` first and fall back to lowest position, so they
// already prefer curation and, unlike a classification predicate, must always
// return a row. Their semantics are their own.

// headlineSlotUnknownValues is the SQL literal list of set_type values that
// mean "slot unknown". A row holding one of these states nothing, so a bill
// made only of them is uncurated. Kept next to the predicate that reads it and
// derived from the contracts vocabulary so a renamed default cannot drift.
var headlineSlotUnknownValues = fmt.Sprintf("('', '%s')", contracts.SetTypePerformer)

// headlineSlotSQL returns the SQL boolean condition for "the show_artists row
// aliased `alias` occupies the headline slot on its bill", per the rule
// documented above. The result is parenthesized, so callers can wrap it in
// NOT(...) or drop it into a CASE/HAVING without re-bracketing.
//
// `alias` must be a compile-time literal supplied by this package; it is
// interpolated into SQL and is never caller data.
//
// The curated-bill test is a correlated EXISTS rather than a join or window
// function so the predicate stays a self-contained boolean expression that any
// aggregate can consume. It sits inside a CASE, so Postgres cannot flatten it
// into a semi-join and runs it per row; that is an indexed lookup on
// idx_show_artists_show_id over an already windowed row set, on surfaces whose
// results are cached.
//
// The condition is always TRUE or FALSE, never NULL: set_type is nullable at
// the schema level (VARCHAR(50) DEFAULT 'performer', no NOT NULL) even though
// every writer supplies a value, so both reads of it are COALESCEd, and
// position is NOT NULL. That matters for the NOT(...) caller: three-valued
// logic there would silently count a NULL-set_type support slot as neither a
// headline slot nor a support slot.
func headlineSlotSQL(alias string) string {
	return `(CASE WHEN EXISTS (
			SELECT 1 FROM show_artists curated_sa
			WHERE curated_sa.show_id = ` + alias + `.show_id
				AND COALESCE(curated_sa.set_type, '') NOT IN ` + headlineSlotUnknownValues + `)
		THEN COALESCE(` + alias + `.set_type, '') = '` + contracts.SetTypeHeadliner + `'
		ELSE ` + alias + `.position = 0
	END)`
}
