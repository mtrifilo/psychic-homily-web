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
// PARTIALLY curated bill has no headline slot at all. This rule is not the place
// to repair one. Narrowing the fallback to "no row states 'headliner'" would
// re-introduce the position heuristic on bills whose curator described an opener
// and no headliner, which is exactly what the curated arm exists to stop.
//
// Which bills arrive in that shape is a write-path question. On the show
// service's create and update paths, an act that states neither set_type nor
// is_headliner keeps its "caller stated nothing" signal, so resolveArtistRole's
// position-0 fallback names a headliner on any bill where no other act claims
// the slot (suppressPositionInferenceWhenHeadlinerNamed). A bill reaches this
// shape when its top act is pinned off the headline slot instead: by its own
// caller (set_type 'performer', or is_headliner false), by ConfirmShowImport,
// which pins every frontmatter entry unconditionally, or by the community
// fulfiller, which pins the silent acts of an admin-typed bill that states any
// set_type, and of every bill adopted from a contributor's request payload.
//
// NOT covered here, deliberately, in three groups:
//
//  1. SQL reads over show_artists that RESOLVE THE ONE headliner row of a
//     show. They prefer a `set_type = 'headliner'` row and fall back to lowest
//     position, so they already prefer curation and, unlike a classification
//     predicate, name an act whenever the bill has one at all (on an
//     artist-less show they degrade to '' or omit the show rather than
//     reporting "no headline slot"). Four sites, in two shapes:
//
//     tag_service.enrichShows, explore.headlinerNameByShow, and
//     show_dedup.RecanonicaliseShowSlug RANK the bill. The required shape is
//     rank, then position, then a stable id:
//
//     CASE WHEN set_type = 'headliner' THEN 0 ELSE 1 END, position ASC, <id> ASC
//
//     The shorter bare-boolean `DESC` form is NULLS FIRST in Postgres, so an
//     unslotted row would outrank the curated headliner. No current writer
//     produces such a row -- the set_type backfill normalized the NULLs that
//     existed, and the model's non-pointer SetType field cannot write one --
//     so this is defense-in-depth against a nullable column, not a repair of
//     a surface that was observably naming the wrong act. The id tiebreak is
//     there because the PK is (show_id, artist_id) and idx_show_artists_position
//     is NOT unique, so two rows on one show may share a position; an untied
//     `LIMIT 1` then returns planner order, which can change after an unrelated
//     UPDATE rewrites the tuples. Tied bills are real -- admin
//     data_quality.getShowsNoBillingOrder reports shows whose every row sits at
//     position 0 -- though current write paths do assign incrementing
//     positions, so the tiebreak guards the corpus, not the writers.
//
//     Test coverage is asymmetric, deliberately, and uneven across the four:
//
//       - enrichShows (tag) and headlinerNameByShow (explore): rank arm pinned
//         by mutation-checked tests at both.
//       - The id tiebreak is pinned ONLY at explore. enrichShows reaches
//         show_artists by the (show_id, artist_id) primary key, so its plan
//         already yields ascending artist_id and a tied-bill test there passes
//         with the tiebreak deleted; it is kept as a guard against a plan
//         change and documented rather than falsely pinned.
//       - SearchShows and RecanonicaliseShowSlug are UNPINNED for both arms.
//         Their existing fixtures seed the headliner at position 0, so
//         deleting either arm leaves those suites green. RecanonicaliseShowSlug
//         is the one that writes its answer into a slug, so it is both the
//         highest-consequence site and the least defended. Adding a
//         curated-bill case there is the obvious next increment.
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
//     payload rather than from show_artists, and feed utils.GenerateShowSlug.
//     In internal/ there are four: catalog.CreateShow,
//     pipeline/discovery.createShowFromEvent, and two in
//     internal/services/admin/data_sync.go -- importShow and
//     backfillShowSlugs. (Note that path: there is also an
//     internal/api/handlers/admin, which is NOT where this lives.) cmd/seed has
//     two more; it is dev tooling and is not audited here.
//
//     ResolveHeadlinerName below is that group's shared rule and all four call
//     it. It has to be a separate function rather than headlineSlotSQL because
//     the rows it ranks may not exist to query yet, and because a slug needs a
//     name even from a bill that names no headliner. A slug is written down and
//     outlives any read-path fix, which is why this group ranks on the curated
//     role rather than on list position.
//
//  3. The duplicate-headliner GUARDS at show.go's checkDuplicateHeadlinerConflicts
//     and pipeline/discovery.go's checkHeadlinerDuplicate. The act each one
//     PROBES is resolved separately from the slug: catalog's by
//     probedHeadlinerNames off the request, reading an act addressed by id under
//     the name that artist row stores; discovery's by its own
//     resolveHeadlinerName off the scraped payload, which takes the FIRST act in
//     list order that states 'headliner' and otherwise the lowest 1-based
//     billing_order, where 0 means absent. ResolveHeadlinerName takes the
//     lowest-POSITION curated headliner on a 0-based position, so the two
//     disagree on a tied or multi-headliner bill and are not interchangeable as
//     written. They keep the
//     `(set_type = 'headliner' OR position = 0)` disjunction, which is NOT
//     equivalent to this rule and is deliberately broader: they ask whether a
//     write would collide, not which row tops a bill. The error directions stay
//     asymmetric with a chart's (a false positive blocks a legitimate save; a
//     false negative admits a duplicate). The rationale, and the reason
//     aligning them to this predicate is a defect rather than a cleanup, lives
//     on checkDuplicateHeadlinerConflicts.

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

// HeadlineCandidate is one act of a bill as an in-memory writer holds it. Name
// and Position are what that writer will store. SetType is normalized here, so a
// caller may pass either the label its payload carried or the value it has
// already resolved.
type HeadlineCandidate struct {
	Name     string
	SetType  string
	Position int
}

// ResolveHeadlinerName names the act that tops `bill`, for writers that must
// pick a headliner from a request or export payload rather than from
// show_artists. It is the in-memory counterpart of the group-1 RESOLVE reads
// documented above and ranks on the same two keys, curated 'headliner' first and
// then lowest position. The third key differs: those reads break a tie on a
// stable row id, which does not exist yet here, so this breaks it on bill order.
//
// It always names an act when the bill has one, because its callers write the
// answer into a slug and a slug needs a name. That is what separates it from
// headlineSlotSQL, which reports that a curated bill naming no headliner has no
// headline slot at all. Returns "" only for an empty bill.
//
// Position is read from the value the caller will store, not from slice index,
// because discovery derives position from billing_order and the data-sync import
// carries an exported Position that need not match list order.
func ResolveHeadlinerName(bill []HeadlineCandidate) string {
	if len(bill) == 0 {
		return ""
	}

	// Comparisons are strict, so an act that ties the incumbent leaves it in
	// place and bill order is the final tiebreak.
	best := bill[0]
	bestCurated := claimsHeadlinerRole(best)
	for _, act := range bill[1:] {
		curated := claimsHeadlinerRole(act)
		if curated != bestCurated {
			if curated {
				best, bestCurated = act, curated
			}
			continue
		}
		if act.Position < best.Position {
			best = act
		}
	}
	return best.Name
}

// claimsHeadlinerRole reports whether this act's stated role is the headline
// slot. Normalized rather than compared raw, so a caller may pass the label its
// payload carried.
func claimsHeadlinerRole(act HeadlineCandidate) bool {
	return contracts.NormalizeSetType(act.SetType) == contracts.SetTypeHeadliner
}

// storedBillCandidates maps the rows a create just wrote onto the shape
// ResolveHeadlinerName ranks, so the slug reads the stored set_type and position
// rather than response order.
func storedBillCandidates(artists []contracts.ArtistResponse) []HeadlineCandidate {
	bill := make([]HeadlineCandidate, 0, len(artists))
	for _, artist := range artists {
		bill = append(bill, HeadlineCandidate{
			Name:     artist.Name,
			SetType:  artist.SetType,
			Position: artist.Position,
		})
	}
	return bill
}
