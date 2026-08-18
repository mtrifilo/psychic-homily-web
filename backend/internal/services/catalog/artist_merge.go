package catalog

import (
	"fmt"

	"gorm.io/gorm"
)

// This file is the artist merge's reference inventory: which tables point at an
// artist, and what MergeArtists does with each. It exists because the artist
// merge had no such inventory and no guard over it, so it fell behind the schema
// silently — PSY-1834 found nine polymorphic entity_type tables it never
// touched, including pending_entity_edits, whose rows a merge left pointing at a
// deleted artist id.
//
// An artist reference comes in exactly three shapes, and each list below covers
// one, with a drift guard in artist_merge_integration_test.go that fails in CI
// the moment a migration adds something the merge does not handle — the only
// cheap moment to notice:
//
//   - polymorphic (entity_type, entity_id) → artistEntityRefs,
//     TestArtistEntityRefsCoverSchema
//   - a real foreign key to artists.id → artistFKTables,
//     TestArtistForeignKeysAreAllHandled
//   - a bare artist id with neither → artistUnconstrainedIDColumns,
//     TestArtistIDColumnsWithoutForeignKeysAreAccountedFor

// artistEntityRefs is every table in the schema with a polymorphic
// (entity_type, entity_id) reference, minus the three that move through a
// dedicated step, with the unique key that constrains each re-point.
//
// These tables carry NO foreign key to artists, so a row left behind does not
// fail loudly — it silently points at an artist id that no longer exists.
//
// Tables whose CHECK constraint forbids entity_type='artist' (source_configs is
// venue/label only) are listed anyway. Their UPDATE matches zero rows, and one
// exhaustive list is cheaper to keep correct than a list plus a set of
// exceptions that has to be re-verified every time a CHECK changes. The venue
// inventory makes the same call for image_enrich_queue.
var artistEntityRefs = []entityRef{
	// Unique on (entity_type, entity_id) plus the listed columns.
	{table: "collection_items", idCol: "entity_id", dedupe: true, key: []string{"collection_id"}},
	{table: "comment_last_read", idCol: "entity_id", dedupe: true, key: []string{"user_id"}},
	{table: "comment_subscriptions", idCol: "entity_id", dedupe: true, key: []string{"user_id"}},
	{table: "entity_tags", idCol: "entity_id", dedupe: true, key: []string{"tag_id"}},
	{table: "notification_log", idCol: "entity_id", dedupe: true, key: []string{"user_id", "filter_id", "channel"}},
	{table: "tag_votes", idCol: "entity_id", dedupe: true, key: []string{"tag_id", "user_id"}},
	{table: "user_bookmarks", idCol: "entity_id", dedupe: true, key: []string{"user_id", "action"}},

	// Unique on (entity_type, entity_id) alone. image_enrich_queue's index is
	// partial (status-scoped); deleting the losing row whenever ANY winning row
	// exists is stricter than the index needs, which is the safe direction.
	{table: "image_enrich_queue", idCol: "entity_id", dedupe: true},
	{table: "source_configs", idCol: "entity_id", dedupe: true},

	// No unique key on the entity reference: re-point in place.
	{table: "audit_logs", idCol: "entity_id"},
	{table: "comments", idCol: "entity_id"},
	{table: "entity_reports", idCol: "entity_id"},
	{table: "requests", idCol: "requested_entity_id"},
	{table: "entity_requests", idCol: "created_entity_id"},
}

// artistRefsRepointedElsewhere names entity_type tables the merge DOES handle
// but not through the loop above, so the schema-coverage guard counts them as
// covered without repointEntityRefs trying to re-point them a second time.
//
// Every table here is one whose move is inseparable from a provenance decision —
// see repointRevisions and repointEditHistory, and the MergeArtists steps that
// give this merge's answers.
var artistRefsRepointedElsewhere = []string{
	"revisions",
	"pending_entity_edits",
	"entity_edit_audit_logs",
}

// artistFKTables is every table holding a real foreign key to artists.id.
//
// Keeping this list is not redundant with the database's own constraints — it is
// the guard for the shape that fails most quietly. Nine of these eleven cascade
// on delete and one sets NULL, so a table the merge does NOT handle makes no
// noise when the losing artist is deleted: its rows silently disappear, or
// silently lose the artist they were matched to.
//
// What each one gets, and why:
//
//   - artist_aliases, artist_labels, artist_releases, artist_reports,
//     festival_artists, show_artists — re-pointed by MergeArtists.
//   - artist_relationships — DELETED by MergeArtists, along with the
//     artist_relationship_votes whose composite foreign key points at them.
//     Pre-existing behavior, unchanged here, and the deletion itself is
//     mandatory: these are the only two foreign keys with NO ON DELETE clause,
//     so leaving a row behind would make the final artist delete fail outright.
//     Auto-derived edges (auto_derived = TRUE) come back on the next derivation
//     run; a MANUALLY created relationship and its votes do not, and re-pointing
//     them instead is not a one-liner — the pair has to be re-canonicalized to
//     source < target, collapsed when the merge makes it self-referential, and
//     de-duplicated against the canonical artist's own edges and votes. Worth
//     its own ticket rather than a step smuggled into this one.
//   - artist_link_suggestions, radio_plays — re-pointed by
//     reassignArtistFKRefs; both carry human/matcher work that the FK's own
//     CASCADE/SET NULL would destroy.
//   - artist_communities, radio_artist_affinity — deliberately left to CASCADE.
//     Both are derived snapshots that their producers rebuild wholesale
//     (community detection and ComputeAffinity each DELETE the whole table
//     first), so a merge that drops rows costs one stale cycle and nothing
//     more. Re-pointing radio_artist_affinity would additionally have to
//     re-normalize its artist_a_id < artist_b_id CHECK and fold the two rows'
//     counters, which is a recomputation dressed as a merge step.
//
// TestArtistForeignKeysAreAllHandled fails if a migration adds another.
var artistFKTables = []string{
	"artist_aliases",
	"artist_communities",
	"artist_labels",
	"artist_link_suggestions",
	"artist_relationships",
	"artist_releases",
	"artist_reports",
	"festival_artists",
	"radio_artist_affinity",
	"radio_plays",
	"show_artists",
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
//   - graph_overview_snapshots.starting_point_artist_ids — left alone. The
//     snapshot is an append-then-prune build artifact; the next build writes a
//     fresh row from live data and the stale one is pruned.
//   - artists.musicbrainz_artist_id, artist_link_suggestions.mb_artist_id,
//     radio_plays.musicbrainz_artist_id — EXTERNAL MusicBrainz identifiers, not
//     Psychic Homily artist ids. Nothing for a merge to re-point.
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

// artistEntityRefTables is every entity_type table this merge handles, for the
// schema-drift test: the ones re-pointed by the loop, plus the ones re-pointed
// by a dedicated step.
func artistEntityRefTables() map[string]bool {
	return entityRefTableSet(artistEntityRefs, artistRefsRepointedElsewhere)
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
// Both tables move with editHistoryCarriesNoRedaction. Nothing anonymous reads
// off either one is edit CONTENT: field_changes and metadata reach only the
// submitter (GET /my/pending-edits, scoped by submitted_by) and admins, and the
// public surfaces read aggregates. See the constant for what has to change if
// that stops being true.
//
// Split out rather than folded into artistEntityRefs so the decision is made
// once, in the open, instead of being implied by a table's presence in a list.
// Both stay named in artistRefsRepointedElsewhere so the schema-coverage guard
// still counts them as handled.
func reassignArtistEditHistory(tx *gorm.DB, canonicalID, mergeFromID uint) error {
	for _, table := range []editHistoryTable{pendingEditsHistory, entityEditAuditHistory} {
		if _, _, err := repointEditHistory(
			tx, table, mergeEntityArtist, canonicalID, mergeFromID, editHistoryCarriesNoRedaction,
		); err != nil {
			return err
		}
	}
	return nil
}

// reassignArtistFKRefs moves the two foreign-key tables whose rows the FK's own
// ON DELETE clause would otherwise destroy or blank.
//
// radio_plays.artist_id is ON DELETE SET NULL, which is the worse of the two
// failures because it is not merely lossy, it is unrecoverable by the pipeline
// that produced it: the row keeps match_state='matched' while artist_id goes
// NULL, and scopePlaysForArtistRematch only ever revisits plays that are BOTH
// artist_id IS NULL and in an unmatched/no-match/ambiguous state. A play
// stranded that way is invisible to every future rematch sweep. Its unique index
// is on (episode_id, dedup_key) and does not mention artist_id, so the re-point
// needs no dedupe.
//
// artist_link_suggestions is ON DELETE CASCADE, so a merge silently discards the
// losing artist's pending link triage. It is unique on (artist_id, platform,
// url), so a suggestion the canonical artist already carries for the same
// platform and URL is dropped before the move.
func reassignArtistFKRefs(tx *gorm.DB, canonicalID, mergeFromID uint) error {
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
	return nil
}
