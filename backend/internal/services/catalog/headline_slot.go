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
// KNOWN CONSEQUENCE, disclosed on PSY-1704 rather than papered over: a
// PARTIALLY curated bill has no headline slot at all. The show form always
// states a role for every act (artist 1 seeds as Headliner), but an API client
// can send `set_type` on one act and nothing on another, and
// handlers/catalog.initializeArtist then defaults the silent act's
// is_headliner to a non-nil FALSE, which means resolveArtistRole's
// position-0 fallback never fires on POST /shows, and the top act is stored
// 'performer'. On such a bill the genuine headliner is counted as a support
// slot and becomes eligible for Openers to Watch.
//
// That is a write-path defect (initializeArtist destroys the "caller stated
// nothing" signal that resolveArtistRole is built to detect); reading it as a
// headline slot here would only hide it. Narrowing the fallback to "no row
// states 'headliner'" would also mask it, and would re-introduce the position
// heuristic on bills whose curator described an opener and no headliner.
//
// NOT covered here, deliberately, in three groups:
//
//  1. SQL reads over show_artists that RESOLVE THE ONE headliner row of a
//     show. They prefer a `set_type = 'headliner'` row and fall back to lowest
//     position, so they already prefer curation and, unlike a classification
//     predicate, must always return a row. Four sites, in two shapes:
//
//     tag_service.enrichShows, explore.headlinerNameByShow, and
//     show_dedup.RecanonicaliseShowSlug RANK the bill. The required shape is
//     rank, then position, then a stable id:
//
//     CASE WHEN set_type = 'headliner' THEN 0 ELSE 1 END, position ASC, <id> ASC
//
//     Every part is load-bearing. The shorter bare-boolean `DESC` form is
//     NULLS FIRST in Postgres, so an unslotted row would outrank the curated
//     headliner. The id tiebreak matters because position is NOT NULL
//     DEFAULT 0 and ingest paths leave whole bills at 0, so without it the
//     winner is planner order and can change after an unrelated UPDATE.
//
//     show.SearchShows instead COALESCEs a filtered subquery over an
//     unfiltered one, which is NULL-safe for a different reason
//     (`NULL = 'headliner'` is NULL, so the row fails the filter rather than
//     sorting ahead of the winner).
//
//     show_dedup.RecanonicaliseShowSlug resolves this to GENERATE A PERSISTED
//     SLUG rather than to display a name, so a mis-ranked row there is written
//     down, not merely rendered.
//
//  2. IN-MEMORY resolutions that pick a headliner from a request or export
//     payload rather than from show_artists, and feed utils.GenerateShowSlug:
//     catalog.CreateShow, pipeline/discovery.resolveHeadlinerName, and two in
//     admin/data_sync.go (importShows and backfillShowSlugs). They are not
//     reachable by the SQL rule above and need their own audit. NOTE that the
//     data_sync pair is position-only and ignores set_type entirely even
//     though the export payload carries it, so a curated bill can persist a
//     slug naming the wrong act. That is a live defect, not a documented
//     exclusion, and needs its own ticket.
//
//  3. The duplicate-headliner GUARDS at show.go's checkDuplicateHeadlinerConflicts
//     and pipeline/discovery.go's checkHeadlinerDuplicate. These do still use
//     the retired `(set_type = 'headliner' OR position = 0)` disjunction, and
//     it is NOT equivalent to this rule. They are deliberately left: they are
//     write-time collision checks where the two error directions are not
//     symmetric with a chart's, and PSY-1673 added their position arm on
//     purpose so a position-inferred headliner is still duplicate-checked.
//     They inherit the same misread this ticket fixes for charts (a curated
//     first-billed opener still matches as "the headliner" there), which needs
//     its own ticket and its own test surface.

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
