package catalog

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// This file is the artist merge's reference inventory: which tables point at an
// artist, and what MergeArtists does with each. It exists because the artist
// merge had no such inventory and no guard over it, so it fell behind the schema
// silently — PSY-1834 found nine polymorphic entity_type tables it never
// touched, including pending_entity_edits, whose rows a merge left pointing at a
// deleted artist id.
//
// An artist reference comes in exactly three shapes, and each has a list plus a
// drift guard in artist_merge_integration_test.go that fails in CI the moment a
// migration adds something the merge does not handle — the only cheap moment to
// notice:
//
//   - polymorphic (entity_type, entity_id) → polymorphicEntityRefs in
//     entity_ref_repoint.go, shared with the venue merge because a unique index
//     is a property of the table and not of the entity pointing at it.
//     Guard: TestArtistEntityRefsCoverSchema.
//   - a real foreign key to artists.id → artistFKColumns,
//     TestArtistForeignKeysAreAllHandled.
//   - a bare artist id with neither → artistUnconstrainedIDColumns,
//     TestArtistIDColumnsWithoutForeignKeysAreAccountedFor.

// artistFKColumns is every COLUMN holding a real foreign key to artists.id,
// spelled "table.column".
//
// Column-granular rather than table-granular on purpose: two of these tables
// carry two artist FKs each, so a guard keyed on table names alone would stay
// green when a migration added a second artist column to a table already in the
// list — and the merge would re-point only the first.
//
// Keeping this list is not redundant with the database's own constraints — it is
// the guard for the shape that fails most quietly. Nine of these tables cascade
// on delete and one sets NULL, so a table the merge does NOT handle makes no
// noise when the losing artist is deleted: its rows silently disappear, or
// silently lose the artist they were matched to.
//
// What each one gets, and why:
//
//   - artist_aliases, artist_labels, artist_releases, artist_reports,
//     festival_artists, show_artists — re-pointed by MergeArtists. show_artists
//     needs a collision delete first; see reassignArtistFKRefs.
//   - artist_relationships — DELETED by MergeArtists, along with the
//     artist_relationship_votes whose composite foreign key points at them.
//     Pre-existing behavior, unchanged here, and the deletion itself is
//     mandatory: these are the only foreign keys to artists with NO ON DELETE
//     clause, so leaving a row behind would make the final artist delete fail
//     outright. Auto-derived edges (auto_derived = TRUE) come back on the next
//     derivation run; a MANUALLY created relationship and its votes do not, and
//     re-pointing them instead is not a one-liner — the pair has to be
//     re-canonicalized to source < target, collapsed when the merge makes it
//     self-referential, and de-duplicated against the canonical artist's own
//     edges and votes. Worth its own ticket rather than a step smuggled into
//     this one.
//   - artist_link_suggestions, radio_plays — re-pointed by
//     reassignArtistFKRefs; both carry human/matcher work that the FK's own
//     CASCADE/SET NULL would destroy.
//   - artist_communities — re-pointed by reassignArtistFKRefs. It IS a derived
//     snapshot that community detection rebuilds wholesale, but the CASCADE
//     deletes the whole community row while every member still carries
//     artists.community_id pointing at it, so the community renders unlabelled
//     for all its members until the next run. A one-column UPDATE avoids a day
//     of that.
//   - radio_artist_affinity — deliberately left to CASCADE. Also derived and
//     rebuilt wholesale (ComputeAffinity DELETEs the table first), and unlike
//     artist_communities nothing reads it between rebuilds by artist id.
//     Re-pointing it would additionally have to re-normalize the
//     artist_a_id < artist_b_id CHECK and fold the two rows' counters, which is
//     a recomputation dressed as a merge step.
//
// TestArtistForeignKeysAreAllHandled fails if a migration adds another.
var artistFKColumns = []string{
	"artist_aliases.artist_id",
	"artist_communities.label_artist_id",
	"artist_labels.artist_id",
	"artist_link_suggestions.artist_id",
	"artist_relationships.source_artist_id",
	"artist_relationships.target_artist_id",
	"artist_releases.artist_id",
	"artist_reports.artist_id",
	"festival_artists.artist_id",
	"radio_artist_affinity.artist_a_id",
	"radio_artist_affinity.artist_b_id",
	"radio_plays.artist_id",
	"show_artists.artist_id",
	// show_dedup_keys.artist_id is DERIVED: the trigger on show_artists rebuilds
	// it, so the show_artists re-point below carries it and no step of this merge
	// touches it directly. Listed because the guard is a completeness inventory.
	"show_dedup_keys.artist_id",
}

// artistUnconstrainedIDColumns is the third class of artist reference, and the
// one neither of the other guards can see: a column that holds artist ids with
// no foreign key and no entity_type discriminator. Nothing in the database
// enforces it and nothing in the schema marks it, so the only way it stays
// handled is by being written down.
//
// Keyed "table.column", with the disposition for each:
//
//   - notification_filters.artist_ids — rewritten by MergeArtists (bigint[],
//     array_replace then de-duplicate).
//   - graph_overview_snapshots.starting_point_artist_ids — left alone, and the
//     reason is the READ path, not the rebuild: resolveStartingPoints
//     (graph_overview_read.go) joins each stored id back to the artists table
//     and drops the ones with no live row, so a merged-away id cannot be
//     offered. The nightly rebuild only cleans up afterwards. If that join is
//     ever removed — say to denormalize name and slug into the snapshot — this
//     disposition has to change with it.
//   - artists.musicbrainz_artist_id — carried onto the canonical artist by
//     reassignArtistFKRefs when the canonical has none. Not a Psychic Homily id,
//     but dropping it is not free: matchArtistByMBID (radio_matching.go)
//     resolves plays by MBID before falling back to name, so losing the merged
//     artist's MBID silently degrades radio matching from then on.
//   - artist_link_suggestions.mb_artist_id, radio_plays.musicbrainz_artist_id —
//     EXTERNAL MusicBrainz identifiers on rows this merge already re-points.
//     They travel with their row and mean the same thing afterwards.
//
// artist_relationship_votes' source/target columns are deliberately absent:
// they carry a COMPOSITE foreign key to artist_relationships, which in turn
// references artists, so they are transitively constrained and cannot hold an id
// the database would let go missing.
//
// TestArtistIDColumnsWithoutForeignKeysAreAccountedFor fails if a migration
// adds another.
var artistUnconstrainedIDColumns = []string{
	"artist_link_suggestions.mb_artist_id",
	"artists.musicbrainz_artist_id",
	"graph_overview_snapshots.starting_point_artist_ids",
	"notification_filters.artist_ids",
	"radio_plays.musicbrainz_artist_id",
}

// lockMergeArtists loads both artists under a row lock so two concurrent merges
// touching the same artist serialize instead of interleaving.
//
// Without it two admins merging the same pair in OPPOSITE directions (A→B and
// B→A) can each delete the other's canonical artist and both commit, leaving
// ZERO artists where there should be one. Nothing else in the merge takes a lock
// on the artist rows themselves, so the FK-referencing rows are the only thing
// that ever serialized these transactions — and a pair with no referencing rows
// has nothing to serialize on. lockMergeVenues exists for the same reason.
//
// The rows are locked in ascending id order regardless of which one is the
// canonical artist: the two opposite merges would otherwise grab them in
// opposite orders and deadlock.
//
// Two explicit statements rather than one `ORDER BY id FOR UPDATE`, because that
// would leave the ordering guarantee up to the query planner. Here the order is
// in the code, where it can be read and relied on.
func lockMergeArtists(tx *gorm.DB, canonicalID, mergeFromID uint) (*catalogm.Artist, *catalogm.Artist, error) {
	first, second := canonicalID, mergeFromID
	if second < first {
		first, second = second, first
	}

	loaded := make(map[uint]*catalogm.Artist, 2)
	for _, id := range []uint{first, second} {
		var a catalogm.Artist
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apperrors.ErrArtistNotFound(id)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to lock artist %d: %w", id, err)
		}
		loaded[id] = &a
	}

	return loaded[canonicalID], loaded[mergeFromID], nil
}

// reassignArtistEditHistory moves the losing artist's contributor edit history
// onto the canonical artist.
//
// Before PSY-1834 this ran for venues and shows but not artists, even though
// PUT /artists/{entity_id}/suggest-edit has always written pending_entity_edits
// rows: every artist merge left the losing artist's proposed edits — and the
// audit trail of its applied ones — pointing at an id deleted in the same
// transaction.
//
// Both tables move with editHistoryCarriesNoRedaction. The re-point cannot
// change who can see this content, because neither read path is scoped by the
// entity: pending-edit content reaches its submitter (GET /my/pending-edits,
// scoped by submitted_by) and admins, and entity_edit_audit_logs rows are read
// on the contributions feed scoped by ACTOR
// (services/user/contributor_profile.go), not by entity_id — their metadata is
// withheld there by the projection's allowlist, so only the row's existence and
// its entity id are published. See the constant for what has to change if
// either gains an entity-scoped gate.
//
// Split out rather than folded into the shared inventory so the decision is made
// once, in the open, instead of being implied by a table's presence in a list.
// Both stay named in refsRepointedElsewhere so the schema-coverage guard still
// counts them as handled.
//
// A dropped row here is a contributor's pending edit destroyed with no undo, so
// it is logged even though MergeArtistResult has no field for it.
func reassignArtistEditHistory(tx *gorm.DB, canonicalID, mergeFromID uint) error {
	for _, table := range []editHistoryTable{pendingEditsHistory, entityEditAuditHistory} {
		_, dropped, err := repointEditHistory(
			tx, table, entityTypeArtist, canonicalID, mergeFromID, editHistoryCarriesNoRedaction)
		if err != nil {
			return err
		}
		logDroppedEntityRefs(
			entityTypeArtist, canonicalID, mergeFromID, map[string]int64{table.name: dropped})
	}
	return nil
}

// dropCollidingShowArtists removes the losing artist's bill entries that could
// not be re-pointed onto the canonical artist, and MUST run before the
// show_artists UPDATE.
//
// Two distinct collisions, and only the first was handled before PSY-1834:
//
//   - the same SHOW billing both artists, which would violate the
//     (show_id, artist_id) primary key;
//   - two DIFFERENT shows at the same venue on the same date, one billing each
//     artist, which violates show_dedup_keys_artist_venue_date_uniq — UNIQUE
//     (artist_id, venue_id, event_date) over the materialized bill.
//
// The second aborted the entire merge with an opaque 500, on exactly the pairs
// an admin is most likely to be merging: duplicate ingest produces a duplicate
// artist AND a duplicate show row together. (The PSY-1581 venue merge recorded
// 119 rows colliding on this same index.)
//
// The losing bill entry is dropped rather than moved, because by definition the
// canonical artist is already billed at that venue on that date. The two SHOWS
// are the actual duplicates, and collapsing them is show dedup's job, not this
// merge's — so the losing show survives, minus a billing that is now redundant.
func dropCollidingShowArtists(tx *gorm.DB, canonicalID, mergeFromID uint) error {
	if err := tx.Exec(`
		DELETE FROM show_artists l
		WHERE l.artist_id = ?
		  AND EXISTS (
		        SELECT 1 FROM show_artists w
		        WHERE w.artist_id = ? AND w.show_id = l.show_id
		      )
	`, mergeFromID, canonicalID).Error; err != nil {
		return fmt.Errorf("failed to drop same-show show_artists conflicts: %w", err)
	}

	// Asked of show_dedup_keys rather than of show_artists' denormalized columns,
	// so every room of a multi-venue bill is compared and not just its lowest
	// venue id. A losing billing that collides only at a bill's second room is
	// invisible to the denormalized columns, and the merge would abort on it.
	if err := tx.Exec(`
		DELETE FROM show_artists l
		WHERE l.artist_id = ?
		  AND EXISTS (
		        SELECT 1
		        FROM show_dedup_keys lk
		        JOIN show_dedup_keys wk
		          ON wk.artist_id  = ?
		         AND wk.venue_id   = lk.venue_id
		         AND wk.event_date = lk.event_date
		        WHERE lk.show_id   = l.show_id
		          AND lk.artist_id = l.artist_id
		      )
	`, mergeFromID, canonicalID).Error; err != nil {
		return fmt.Errorf("failed to drop same-venue-and-date show_artists conflicts: %w", err)
	}
	return nil
}

// reassignArtistFKRefs handles the foreign-key tables the polymorphic loop
// cannot: the ones whose ON DELETE clause would destroy or blank their rows.
//
// radio_plays.artist_id is ON DELETE SET NULL, which is unrecoverable by the
// pipeline that produced it: the row keeps match_state='matched' while artist_id
// goes NULL, and scopePlaysForArtistRematch only ever revisits plays that are
// BOTH artist_id IS NULL and in an unmatched/no-match/ambiguous state. A play
// stranded that way is invisible to every future rematch sweep. Its unique index
// is on (episode_id, dedup_key) and does not mention artist_id, so the re-point
// needs no dedupe.
//
// artist_link_suggestions is ON DELETE CASCADE, so a merge silently discards the
// losing artist's pending link triage. It is unique on (artist_id, platform,
// url), so a collision has to drop one row — and the one that survives is the
// REVIEWED one. Keeping the canonical's still-pending row over a verdict a human
// already reached would put a triaged URL back in the review queue, which is the
// opposite of why these rows are being rescued at all.
//
// artist_communities.label_artist_id is ON DELETE CASCADE with no unique key, so
// it takes a bare UPDATE; see artistFKColumns for why the row is worth keeping
// even though it is derived.
//
// The MusicBrainz id is carried last, and only when the canonical artist has
// none: it is not a Psychic Homily reference, but radio matching resolves plays
// by MBID before falling back to name, so dropping the loser's would silently
// degrade matching for every future play that carries it.
func reassignArtistFKRefs(tx *gorm.DB, canonicalID, mergeFromID uint) error {
	if canonicalID == 0 || mergeFromID == 0 {
		return fmt.Errorf("reassign artist fk refs: canonical and merge-from ids are required")
	}
	// A self-merge would make every EXISTS below correlate a row against itself,
	// deleting the surviving artist's own suggestions and bill entries.
	if canonicalID == mergeFromID {
		return fmt.Errorf("reassign artist fk refs: cannot re-point artist %d onto itself", canonicalID)
	}

	// Prefer a reviewed verdict over a pending one: drop the canonical's pending
	// row when the loser carries an accepted/rejected answer for the same
	// candidate, so the human's decision is what survives the merge.
	if err := tx.Exec(`
		DELETE FROM artist_link_suggestions w
		WHERE w.artist_id = ?
		  AND w.status = ?
		  AND EXISTS (
		        SELECT 1 FROM artist_link_suggestions l
		        WHERE l.artist_id = ?
		          AND l.platform = w.platform
		          AND l.url = w.url
		          AND l.status <> ?
		      )
	`, canonicalID, catalogm.LinkSuggestionStatusPending,
		mergeFromID, catalogm.LinkSuggestionStatusPending).Error; err != nil {
		return fmt.Errorf("failed to drop superseded artist_link_suggestions rows: %w", err)
	}
	// Whatever still collides after that is a genuine duplicate; the canonical's
	// copy wins.
	if err := tx.Exec(`
		DELETE FROM artist_link_suggestions l
		WHERE l.artist_id = ?
		  AND EXISTS (
		        SELECT 1 FROM artist_link_suggestions w
		        WHERE w.artist_id = ?
		          AND w.platform = l.platform
		          AND w.url = l.url
		      )
	`, mergeFromID, canonicalID).Error; err != nil {
		return fmt.Errorf("failed to drop conflicting artist_link_suggestions rows: %w", err)
	}
	if err := tx.Exec(
		"UPDATE artist_link_suggestions SET artist_id = ? WHERE artist_id = ?",
		canonicalID, mergeFromID,
	).Error; err != nil {
		return fmt.Errorf("failed to move artist_link_suggestions rows: %w", err)
	}

	if err := tx.Exec(
		"UPDATE radio_plays SET artist_id = ? WHERE artist_id = ?",
		canonicalID, mergeFromID,
	).Error; err != nil {
		return fmt.Errorf("failed to move radio_plays rows: %w", err)
	}

	if err := tx.Exec(
		"UPDATE artist_communities SET label_artist_id = ? WHERE label_artist_id = ?",
		canonicalID, mergeFromID,
	).Error; err != nil {
		return fmt.Errorf("failed to move artist_communities rows: %w", err)
	}

	if err := tx.Exec(`
		UPDATE artists c
		SET musicbrainz_artist_id = l.musicbrainz_artist_id
		FROM artists l
		WHERE c.id = ?
		  AND l.id = ?
		  AND c.musicbrainz_artist_id IS NULL
		  AND l.musicbrainz_artist_id IS NOT NULL
	`, canonicalID, mergeFromID).Error; err != nil {
		return fmt.Errorf("failed to carry musicbrainz id onto canonical artist: %w", err)
	}

	return nil
}
