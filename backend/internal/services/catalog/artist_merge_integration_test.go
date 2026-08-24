package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/testutil"
)

// Artist merge is destructive — it deletes an artist row, and nine of the eleven
// foreign keys to artists cascade — so every test here runs against a real
// Postgres with the real constraints. Two of the cases cannot be observed any
// other way:
//
//   - notification_log's unique index on
//     (user_id, filter_id, entity_type, entity_id, channel), which the merge's
//     old bare UPDATE would have violated for a user notified about both
//     artists;
//   - radio_plays' ON DELETE SET NULL, which silently strands a matched play
//     rather than raising anything.

type ArtistMergeIntegrationSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *ArtistService
}

func (s *ArtistMergeIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = &ArtistService{db: s.db}
}

func (s *ArtistMergeIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *ArtistMergeIntegrationSuite) SetupTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, stmt := range []string{
		"DELETE FROM audit_logs",
		// pending_entity_edits.submitted_by has no ON DELETE clause, so a stale
		// row here makes the users delete below fail rather than the test that
		// left it behind.
		"DELETE FROM pending_entity_edits",
		"DELETE FROM entity_edit_audit_logs",
		"DELETE FROM revisions",
		"DELETE FROM artist_link_suggestions",
		"DELETE FROM image_enrich_queue",
		"DELETE FROM radio_plays",
		"DELETE FROM radio_episodes",
		"DELETE FROM radio_shows",
		"DELETE FROM radio_stations",
		"DELETE FROM notification_log",
		"DELETE FROM notification_filters",
		"DELETE FROM artist_aliases",
		"DELETE FROM artist_communities",
		"DELETE FROM entity_tags",
		"DELETE FROM tag_votes",
		"DELETE FROM tags",
		"DELETE FROM show_artists",
		"DELETE FROM show_venues",
		"DELETE FROM shows",
		"DELETE FROM artists",
		"DELETE FROM venues",
		"DELETE FROM users",
	} {
		_, err := sqlDB.Exec(stmt)
		s.Require().NoError(err, stmt)
	}
}

func TestArtistMergeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ArtistMergeIntegrationSuite))
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func (s *ArtistMergeIntegrationSuite) createArtist(name string) *catalogm.Artist {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

func (s *ArtistMergeIntegrationSuite) createUser(name string) *authm.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	u := &authm.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

// seedPendingEdit records one proposed artist edit and returns its id.
//
// Written through the model rather than raw SQL so a column rename breaks the
// build instead of the test run.
func (s *ArtistMergeIntegrationSuite) seedPendingEdit(
	artistID, userID uint, status adminm.PendingEditStatus,
) uint {
	changes := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	edit := &adminm.PendingEntityEdit{
		EntityType:   adminm.PendingEditEntityArtist,
		EntityID:     artistID,
		SubmittedBy:  userID,
		FieldChanges: &changes,
		Summary:      "name fix",
		Status:       status,
	}
	s.Require().NoError(s.db.Create(edit).Error)
	return edit.ID
}

func (s *ArtistMergeIntegrationSuite) pendingEditByID(id uint) adminm.PendingEntityEdit {
	var edit adminm.PendingEntityEdit
	s.Require().NoError(s.db.First(&edit, id).Error)
	return edit
}

func (s *ArtistMergeIntegrationSuite) seedEntityEditAuditLog(artistID, userID uint) uint {
	metadata := json.RawMessage(`{"fields":["name"]}`)
	entry := &adminm.EntityEditAuditLog{
		ActorID:    &userID,
		EntityType: "artist",
		EntityID:   artistID,
		Metadata:   &metadata,
		CreatedAt:  time.Now().UTC(),
	}
	s.Require().NoError(s.db.Create(entry).Error)
	return entry.ID
}

// seedNotificationLog writes one notification about an artist.
//
// filterID is a pointer because it decides whether the unique index can fire at
// all: the index is NULLS DISTINCT, so two rows with a NULL filter_id never
// collide no matter what the other four columns say. Both shapes are exercised
// below.
func (s *ArtistMergeIntegrationSuite) seedNotificationLog(artistID, userID uint, filterID *uint) uint {
	entry := &notificationm.NotificationLog{
		UserID:     userID,
		FilterID:   filterID,
		EntityType: "artist",
		EntityID:   artistID,
		Channel:    notificationm.NotificationChannelInApp,
		SentAt:     time.Now().UTC(),
	}
	s.Require().NoError(s.db.Create(entry).Error)
	return entry.ID
}

func (s *ArtistMergeIntegrationSuite) seedNotificationFilter(userID uint) *notificationm.NotificationFilter {
	filter := &notificationm.NotificationFilter{UserID: userID, Name: "watchlist"}
	s.Require().NoError(s.db.Create(filter).Error)
	return filter
}

// seedMatchedPlay records one radio play already matched to an artist — the
// state radio_plays' ON DELETE SET NULL destroys.
func (s *ArtistMergeIntegrationSuite) seedMatchedPlay(artistID uint, trackTitle string) uint {
	station := &catalogm.RadioStation{
		Name:          fmt.Sprintf("Station %d", time.Now().UnixNano()),
		Slug:          fmt.Sprintf("station-%d", time.Now().UnixNano()),
		BroadcastType: catalogm.BroadcastTypeInternet,
	}
	s.Require().NoError(s.db.Create(station).Error)

	show := &catalogm.RadioShow{
		StationID: station.ID,
		Name:      "Test Show",
		Slug:      fmt.Sprintf("test-show-%d", time.Now().UnixNano()),
		IsActive:  true,
	}
	s.Require().NoError(s.db.Create(show).Error)

	episode := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: "2026-01-01"}
	s.Require().NoError(s.db.Create(episode).Error)

	title := trackTitle
	play := &catalogm.RadioPlay{
		EpisodeID:  episode.ID,
		Position:   1,
		ArtistName: "Whoever",
		TrackTitle: &title,
		ArtistID:   &artistID,
		MatchState: catalogm.RadioPlayMatchStateMatched,
	}
	s.Require().NoError(s.db.Create(play).Error)
	return play.ID
}

func (s *ArtistMergeIntegrationSuite) seedLinkSuggestion(artistID uint, url, status string) uint {
	suggestion := &catalogm.ArtistLinkSuggestion{
		ArtistID:   artistID,
		Platform:   "bandcamp",
		URL:        url,
		Source:     "musicbrainz",
		Confidence: "high",
		Status:     status,
		CreatedAt:  time.Now().UTC(),
	}
	s.Require().NoError(s.db.Create(suggestion).Error)
	return suggestion.ID
}

// seedShowAt books artist at venue on date, wiring both show_venues and the
// denormalized show_artists.venue_id/event_date that
// shows_artist_venue_eventdate_uniq actually indexes.
func (s *ArtistMergeIntegrationSuite) seedShowAt(
	artistID, venueID uint, date time.Time,
) *catalogm.Show {
	slug := fmt.Sprintf("show-%d", time.Now().UnixNano())
	show := &catalogm.Show{Title: "Gig", Slug: &slug, EventDate: date}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)

	d := date
	v := venueID
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{
		ShowID:    show.ID,
		ArtistID:  artistID,
		Position:  0,
		SetType:   "performer",
		EventDate: &d,
		VenueID:   &v,
	}).Error)
	return show
}

func (s *ArtistMergeIntegrationSuite) createVenue(name string) *catalogm.Venue {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	v := &catalogm.Venue{Name: name, Slug: &slug, City: "Phoenix", State: "AZ"}
	s.Require().NoError(s.db.Create(v).Error)
	return v
}

// ──────────────────────────────────────────────
// Schema drift guards
// ──────────────────────────────────────────────

// TestArtistEntityRefsCoverSchema is the guard this ticket exists to add.
// polymorphicEntityRefs is a hand-maintained list, and the failure mode of
// falling behind is silent: a new entity_type table simply keeps rows pointing
// at a deleted artist. That is exactly what happened to pending_entity_edits,
// which gained a suggest-edit route for artists and never gained a merge step.
//
// This fails in CI the moment a migration adds one, which is the only cheap
// moment to notice.
func (s *ArtistMergeIntegrationSuite) TestArtistEntityRefsCoverSchema() {
	var tables []string
	s.Require().NoError(s.db.Raw(`
		SELECT table_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'entity_type'
		ORDER BY table_name
	`).Scan(&tables).Error)
	s.Require().NotEmpty(tables)

	covered := entityRefTables()
	for _, table := range tables {
		s.Truef(covered[table],
			"table %q has an entity_type column but is not in polymorphicEntityRefs — an artist merge "+
				"would leave its rows pointing at a deleted artist. Add it (with its unique key), "+
				"or handle it in a dedicated step and name it in refsRepointedElsewhere.", table)
	}

	// And the reverse: nothing in either list may have been dropped by a
	// migration, which would make every merge fail at runtime.
	present := map[string]bool{}
	for _, t := range tables {
		present[t] = true
	}
	for _, ref := range polymorphicEntityRefs {
		s.Truef(present[ref.table],
			"polymorphicEntityRefs lists %q, which no longer has an entity_type column", ref.table)
	}
	for _, table := range refsRepointedElsewhere {
		s.Truef(present[table],
			"refsRepointedElsewhere lists %q, which no longer has an entity_type column", table)
	}
}

// TestArtistEntityRefDedupeKeysMatchTheSchema closes the gap the coverage guard
// leaves: a ref may name the right table and still describe the wrong index.
//
// A wrong idCol aborts every merge loudly. A wrong dedupe key does not — it
// surfaces only on the one merge where two rows actually collide, which is rare
// enough to reach production. So this reads the REAL unique indexes out of
// pg_index and asserts that each ref's declared key matches one of them, rather
// than merely asserting the named columns exist.
//
// A ref with dedupe=false must have no unique index over its entity reference at
// all; that direction matters just as much, since it is the one that produces
// the constraint violation.
func (s *ArtistMergeIntegrationSuite) TestArtistEntityRefDedupeKeysMatchTheSchema() {
	for _, ref := range polymorphicEntityRefs {
		indexes := s.uniqueIndexColumnSets(ref.table)

		declared := map[string]bool{"entity_type": true, ref.idCol: true}
		for _, col := range ref.key {
			declared[col] = true
		}

		var covering []map[string]bool
		for _, cols := range indexes {
			if cols["entity_type"] && cols[ref.idCol] {
				covering = append(covering, cols)
			}
		}

		if !ref.dedupe {
			s.Emptyf(covering,
				"%s is declared dedupe=false but carries a unique index over its entity "+
					"reference; the re-point will violate it", ref.table)
			continue
		}

		s.Require().NotEmptyf(covering,
			"%s is declared dedupe=true but has no unique index over (entity_type, %s); the "+
				"dedupe is deleting rows for no reason", ref.table, ref.idCol)

		matched := false
		for _, cols := range covering {
			if len(cols) == len(declared) && subsetOf(cols, declared) {
				matched = true
				break
			}
		}
		s.Truef(matched,
			"%s declares key %v, which matches none of its real unique indexes %v — a dedupe "+
				"narrower than the index aborts the merge on a collision, a wider one deletes "+
				"rows the database would have kept", ref.table, ref.key, indexes)
	}
}

// uniqueIndexColumnSets returns the column set of every unique index on table
// that is defined over plain columns (expression indexes are skipped: none of
// the inventory's dedupe keys is an expression, and one would not be
// representable as an entityRef.key anyway).
func (s *ArtistMergeIntegrationSuite) uniqueIndexColumnSets(table string) []map[string]bool {
	type row struct {
		IndexName string
		Columns   string
	}
	var rows []row
	s.Require().NoError(s.db.Raw(`
		SELECT i.relname AS index_name,
		       string_agg(a.attname, ',' ORDER BY a.attname) AS columns
		FROM pg_index x
		JOIN pg_class i ON i.oid = x.indexrelid
		JOIN pg_class t ON t.oid = x.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(x.indkey)
		WHERE n.nspname = 'public' AND t.relname = ? AND x.indisunique
		GROUP BY i.relname, x.indnatts
		HAVING COUNT(*) = x.indnatts
	`, table).Scan(&rows).Error)

	out := make([]map[string]bool, 0, len(rows))
	for _, r := range rows {
		cols := map[string]bool{}
		for _, c := range strings.Split(r.Columns, ",") {
			cols[c] = true
		}
		out = append(out, cols)
	}
	return out
}

func subsetOf(got, want map[string]bool) bool {
	for col := range got {
		if !want[col] {
			return false
		}
	}
	return true
}

// TestArtistForeignKeysAreAllHandled is the second drift guard, and the one that
// covers the columns the database itself would quietly empty.
//
// Nine of these tables cascade on delete and one sets NULL, so a column this
// merge does not handle does NOT raise an error when the losing artist goes —
// its rows are silently destroyed, or silently un-matched.
//
// Matched per COLUMN, not per table: artist_relationships and
// radio_artist_affinity each already carry two artist foreign keys, so a
// table-level assertion would stay green when a migration added a second artist
// column to a table already in the list, and the merge would re-point only the
// first.
func (s *ArtistMergeIntegrationSuite) TestArtistForeignKeysAreAllHandled() {
	var columns []string
	s.Require().NoError(s.db.Raw(`
		SELECT DISTINCT kcu.table_name || '.' || kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND ccu.table_name = 'artists'
		  AND ccu.column_name = 'id'
		ORDER BY 1
	`).Scan(&columns).Error)
	s.Require().NotEmpty(columns)

	s.ElementsMatch(artistFKColumns, columns,
		"the set of columns with a foreign key to artists has changed. A new one must be "+
			"given a disposition in artistFKColumns — most of these CASCADE, so an unhandled "+
			"column loses its rows silently when the merged artist is deleted.")
}

// TestArtistIDColumnsWithoutForeignKeysAreAccountedFor is the third guard, and
// the only one that can see notification_filters.artist_ids: a bigint[] with no
// foreign key and no entity_type column, invisible to both lists above. Nothing
// in the database enforces these columns and nothing in the schema marks them,
// so an unhandled one is a stale artist id that raises nothing, forever.
//
// The match is on the COLUMN NAME, which deliberately catches the MusicBrainz
// identifiers too. They are not our ids and need no re-point, but a guard that
// filtered them out by a name heuristic would filter out a real column named
// the same way. Listing them costs one line each and makes "checked, and it is
// not ours" a recorded decision rather than an omission.
//
// The pattern is "contains artist, ends in id or ids" rather than "ends in
// artist_id", because this schema does not consistently use that suffix:
// radio_artist_affinity names its pair artist_a_id / artist_b_id. Those two are
// FK-constrained today and so excluded anyway, but they prove the narrower
// pattern would have a hole exactly where the naming deviates.
//
// The exclusion is "participates in ANY foreign key", not "references artists".
// That is what drops artist_relationship_votes' source/target columns, whose
// composite key points at artist_relationships rather than at artists directly:
// they are still constrained, still cannot go dangling, and the merge already
// deletes them.
func (s *ArtistMergeIntegrationSuite) TestArtistIDColumnsWithoutForeignKeysAreAccountedFor() {
	var columns []string
	s.Require().NoError(s.db.Raw(`
		SELECT c.table_name || '.' || c.column_name
		FROM information_schema.columns c
		WHERE c.table_schema = 'public'
		  AND c.column_name LIKE '%artist%'
		  AND (c.column_name LIKE '%id' OR c.column_name LIKE '%ids')
		  AND NOT EXISTS (
		        SELECT 1
		        FROM information_schema.key_column_usage kcu
		        JOIN information_schema.table_constraints tc
		          ON tc.constraint_name = kcu.constraint_name
		         AND tc.constraint_type = 'FOREIGN KEY'
		        WHERE kcu.table_schema = c.table_schema
		          AND kcu.table_name = c.table_name
		          AND kcu.column_name = c.column_name
		      )
		ORDER BY 1
	`).Scan(&columns).Error)
	s.Require().NotEmpty(columns)

	s.ElementsMatch(artistUnconstrainedIDColumns, columns,
		"a column holding artist ids with no foreign key has been added or removed. Neither "+
			"the entity_type guard nor the foreign-key guard can see one, so it must be given "+
			"a disposition in artistUnconstrainedIDColumns — even if that disposition is "+
			"'external identifier, nothing to re-point'.")
}

// ──────────────────────────────────────────────
// Edit history
// ──────────────────────────────────────────────

// The orphan this ticket closes: PUT /artists/{entity_id}/suggest-edit has
// always written pending_entity_edits rows, and the artist merge never moved
// them. Every merge left a contributor's proposed edit pointing at an artist id
// deleted in the same transaction.
func (s *ArtistMergeIntegrationSuite) TestMergeRepointsPendingEdits() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	editor := s.createUser("editor")

	pending := s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusPending)
	approved := s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusApproved)
	auditEntry := s.seedEntityEditAuditLog(loser.ID, editor.ID)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	s.Equal(canonical.ID, s.pendingEditByID(pending).EntityID,
		"a pending edit must follow its artist onto the canonical artist")
	s.Equal(canonical.ID, s.pendingEditByID(approved).EntityID,
		"an already-reviewed edit is history and must move too")

	var movedEntry adminm.EntityEditAuditLog
	s.Require().NoError(s.db.First(&movedEntry, auditEntry).Error)
	s.Equal(canonical.ID, movedEntry.EntityID,
		"the edit audit trail must not be left pointing at a deleted artist")
}

// The dedupe is what keeps the re-point from violating
// idx_pending_entity_edits_unique, which is UNIQUE
// (entity_type, entity_id, submitted_by) WHERE status = 'pending'. Without it a
// merge of two artists the same contributor had proposed edits on would abort
// the whole transaction.
func (s *ArtistMergeIntegrationSuite) TestMergeDropsDuplicateContributorPendingEdits() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	editor := s.createUser("editor")

	kept := s.seedPendingEdit(canonical.ID, editor.ID, adminm.PendingEditStatusPending)
	dropped := s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusPending)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	s.Equal(canonical.ID, s.pendingEditByID(kept).EntityID,
		"the canonical artist's own pending edit must survive untouched")

	var still adminm.PendingEntityEdit
	err = s.db.First(&still, dropped).Error
	s.ErrorIs(err, gorm.ErrRecordNotFound,
		"the colliding pending edit must be dropped, not carried into a unique violation")
}

// idx_pending_entity_edits_unique is PARTIAL — it constrains only rows with
// status='pending'. An approved or rejected edit therefore cannot collide with
// anything, and the dedupe must leave it alone.
//
// This is not bookkeeping: approved pending_entity_edits rows are what the
// trusted-tier auto-promotion counts, and what its rolling demotion check counts
// too, so destroying them can silently cost a contributor their tier. The
// unscoped dedupe this replaces deleted every one of the contributor's rows on
// the losing artist.
func (s *ArtistMergeIntegrationSuite) TestMergeKeepsReviewedEditsThatCannotCollide() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	editor := s.createUser("editor")

	// The contributor has a pending edit on the canonical artist, which is what
	// makes the losing artist's rows candidates for the dedupe at all.
	s.seedPendingEdit(canonical.ID, editor.ID, adminm.PendingEditStatusPending)
	approved := s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusApproved)
	rejected := s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusRejected)
	collides := s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusPending)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	for _, id := range []uint{approved, rejected} {
		moved := s.pendingEditByID(id)
		s.Equal(canonical.ID, moved.EntityID,
			"a reviewed edit cannot violate a partial index over status='pending' and must survive")
	}

	var gone adminm.PendingEntityEdit
	s.ErrorIs(s.db.First(&gone, collides).Error, gorm.ErrRecordNotFound,
		"only the row that would actually violate the index may be dropped")
}

// uq_image_enrich_queue_active is PARTIAL over status IN ('pending','processing').
// A canonical artist whose enrichment already finished holds a 'done' row that
// constrains nothing, so the losing artist's still-queued job must move rather
// than be thrown away — nothing re-enqueues it.
func (s *ArtistMergeIntegrationSuite) TestMergeKeepsQueuedEnrichmentBehindAFinishedOne() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")

	s.Require().NoError(s.db.Exec(
		"INSERT INTO image_enrich_queue (entity_type, entity_id, status) VALUES ('artist', ?, 'done')",
		canonical.ID).Error)
	s.Require().NoError(s.db.Exec(
		"INSERT INTO image_enrich_queue (entity_type, entity_id, status) VALUES ('artist', ?, 'pending')",
		loser.ID).Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var queued int64
	s.Require().NoError(s.db.Raw(`
		SELECT COUNT(*) FROM image_enrich_queue
		WHERE entity_type = 'artist' AND entity_id = ? AND status = 'pending'
	`, canonical.ID).Scan(&queued).Error)
	s.Equal(int64(1), queued,
		"a queued enrichment must not be deleted by a finished row it could never collide with")
}

// The other direction: two ACTIVE rows really do collide, so one must go.
func (s *ArtistMergeIntegrationSuite) TestMergeDropsCollidingActiveEnrichmentJobs() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")

	s.Require().NoError(s.db.Exec(
		"INSERT INTO image_enrich_queue (entity_type, entity_id, status) VALUES ('artist', ?, 'pending')",
		canonical.ID).Error)
	s.Require().NoError(s.db.Exec(
		"INSERT INTO image_enrich_queue (entity_type, entity_id, status) VALUES ('artist', ?, 'processing')",
		loser.ID).Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err, "two active jobs must not abort the merge on the partial index")

	var active int64
	s.Require().NoError(s.db.Raw(`
		SELECT COUNT(*) FROM image_enrich_queue
		WHERE entity_type = 'artist' AND entity_id = ? AND status IN ('pending', 'processing')
	`, canonical.ID).Scan(&active).Error)
	s.Equal(int64(1), active, "the partial index permits exactly one active job per entity")
}

// ──────────────────────────────────────────────
// The rest of the polymorphic inventory
// ──────────────────────────────────────────────

// notification_log is unique on
// (user_id, filter_id, entity_type, entity_id, channel). The merge used to move
// it with a bare UPDATE and no dedupe, so a user notified about BOTH artists
// through the same filter and channel would have aborted the entire merge on a
// unique violation — the whole admin action failing on a row nobody would think
// to look at. Routing it through the shared inventory is what fixes that, and
// this is the case that proves it.
func (s *ArtistMergeIntegrationSuite) TestMergeDeduplicatesNotificationLog() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	reader := s.createUser("reader")
	filter := s.seedNotificationFilter(reader.ID)

	kept := s.seedNotificationLog(canonical.ID, reader.ID, &filter.ID)
	colliding := s.seedNotificationLog(loser.ID, reader.ID, &filter.ID)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err, "a duplicate notification must not abort the merge")

	var survivor notificationm.NotificationLog
	s.Require().NoError(s.db.First(&survivor, kept).Error)
	s.Equal(canonical.ID, survivor.EntityID)

	var gone notificationm.NotificationLog
	s.ErrorIs(s.db.First(&gone, colliding).Error, gorm.ErrRecordNotFound,
		"the colliding notification row must be dropped before the re-point")
}

// The other half of the same index: a NULL filter_id makes two rows distinct no
// matter what, so nothing collides and nothing may be dropped. The dedupe's
// `w.filter_id = l.filter_id` is NULL-unsafe on purpose — it has to match the
// index's NULLS DISTINCT semantics exactly, and a well-meaning rewrite to
// IS NOT DISTINCT FROM would start deleting notifications the database was
// perfectly happy to keep.
func (s *ArtistMergeIntegrationSuite) TestMergeKeepsUnfilteredNotificationsThatCannotCollide() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	reader := s.createUser("reader")

	kept := s.seedNotificationLog(canonical.ID, reader.ID, nil)
	alsoKept := s.seedNotificationLog(loser.ID, reader.ID, nil)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	for _, id := range []uint{kept, alsoKept} {
		var row notificationm.NotificationLog
		s.Require().NoError(s.db.First(&row, id).Error,
			"a notification the unique index treats as distinct must not be deleted by the dedupe")
		s.Equal(canonical.ID, row.EntityID)
	}
}

// A plain re-point through the same loop, on a table with no unique key. Seeded
// with raw SQL because audit_logs is written by fire-and-forget helpers rather
// than a model this package owns.
func (s *ArtistMergeIntegrationSuite) TestMergeRepointsAuditLogs() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")

	s.Require().NoError(s.db.Exec(
		"INSERT INTO audit_logs (action, entity_type, entity_id) VALUES ('update', 'artist', ?)",
		loser.ID).Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var orphaned int64
	s.Require().NoError(s.db.Raw(
		"SELECT COUNT(*) FROM audit_logs WHERE entity_type = 'artist' AND entity_id = ?",
		loser.ID).Scan(&orphaned).Error)
	s.Zero(orphaned, "the audit trail must not be left pointing at a deleted artist")
}

// The sweep: after a merge, NOTHING in the polymorphic inventory may still point
// at the losing artist.
//
// Honest about its reach: it only proves anything for the four tables seeded
// below. The other thirteen assertions run against empty tables and pass
// vacuously, so this is NOT a substitute for the per-table cases above — it is a
// cheap backstop that catches a whole table dropping out of the loop. What
// actually keeps the inventory correct is TestArtistEntityRefsCoverSchema (the
// list covers the schema) and TestArtistEntityRefDedupeKeysMatchTheSchema (each
// entry describes its real index).
func (s *ArtistMergeIntegrationSuite) TestMergeLeavesNoDanglingArtistReferences() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	editor := s.createUser("editor")

	s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusPending)
	s.seedEntityEditAuditLog(loser.ID, editor.ID)
	s.seedNotificationLog(loser.ID, editor.ID, nil)
	s.Require().NoError(s.db.Exec(
		"INSERT INTO audit_logs (action, entity_type, entity_id) VALUES ('update', 'artist', ?)",
		loser.ID).Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	for _, ref := range polymorphicEntityRefs {
		var remaining int64
		// #nosec G201 -- table and column come from the hardcoded inventory.
		query := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE entity_type = 'artist' AND %s = ?", ref.table, ref.idCol)
		s.Require().NoError(s.db.Raw(query, loser.ID).Scan(&remaining).Error)
		s.Zerof(remaining, "%s still points at the merged-away artist", ref.table)
	}
	for _, table := range refsRepointedElsewhere {
		var remaining int64
		// #nosec G201 -- table comes from the hardcoded inventory.
		query := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE entity_type = 'artist' AND entity_id = ?", table)
		s.Require().NoError(s.db.Raw(query, loser.ID).Scan(&remaining).Error)
		s.Zerof(remaining, "%s still points at the merged-away artist", table)
	}
}

// ──────────────────────────────────────────────
// The unique index that used to abort the whole merge
// ──────────────────────────────────────────────

// shows_artist_venue_eventdate_uniq is UNIQUE (artist_id, venue_id, event_date)
// WHERE both are NOT NULL. The merge's original conflict delete keyed on
// show_id alone, so two DIFFERENT shows at one venue on one date — one billing
// each artist — survived it and then collided on the UPDATE, aborting the entire
// merge with an opaque 500.
//
// That is not an exotic pairing: duplicate ingest produces a duplicate artist
// and a duplicate show row together, which is exactly the pair an admin reaches
// for the merge button on.
func (s *ArtistMergeIntegrationSuite) TestMergeSurvivesSameVenueAndDateOnDifferentShows() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	venue := s.createVenue("The Rebel Lounge")
	date := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)

	canonicalShow := s.seedShowAt(canonical.ID, venue.ID, date)
	loserShow := s.seedShowAt(loser.ID, venue.ID, date)
	s.Require().NotEqual(canonicalShow.ID, loserShow.ID)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err,
		"two different shows at one venue on one date must not abort the merge")

	var billings int64
	s.Require().NoError(s.db.Raw(`
		SELECT COUNT(*) FROM show_artists
		WHERE artist_id = ? AND venue_id = ? AND event_date = ?
	`, canonical.ID, venue.ID, date).Scan(&billings).Error)
	s.Equal(int64(1), billings,
		"exactly one billing may survive — the index permits no more")

	var stranded int64
	s.Require().NoError(s.db.Raw(
		"SELECT COUNT(*) FROM show_artists WHERE artist_id = ?", loser.ID).Scan(&stranded).Error)
	s.Zero(stranded, "no bill entry may be left on the merged-away artist")
}

// The pre-existing same-show conflict still has to be dropped: one show billing
// both artists would violate the (show_id, artist_id) primary key.
func (s *ArtistMergeIntegrationSuite) TestMergeDropsSameShowBillingConflicts() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	venue := s.createVenue("Valley Bar")
	date := time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC)

	show := s.seedShowAt(canonical.ID, venue.ID, date)
	d := date
	v := venue.ID
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{
		ShowID: show.ID, ArtistID: loser.ID, Position: 1, SetType: "performer",
		EventDate: &d, VenueID: &v,
	}).Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var billings int64
	s.Require().NoError(s.db.Raw(
		"SELECT COUNT(*) FROM show_artists WHERE show_id = ? AND artist_id = ?",
		show.ID, canonical.ID).Scan(&billings).Error)
	s.Equal(int64(1), billings, "one show may bill the canonical artist exactly once")
}

// ──────────────────────────────────────────────
// Concurrency
// ──────────────────────────────────────────────

// Two admins merging the same pair in OPPOSITE directions must resolve to
// exactly one surviving artist. Before the row lock, each transaction deleted
// the other's canonical artist and both could commit — leaving ZERO artists
// where there should be one, silently and irreversibly.
//
// The pair here deliberately has no FK-referencing rows: those were the only
// thing that ever serialized these transactions, so a bare pair is precisely the
// case the lock has to cover.
func (s *ArtistMergeIntegrationSuite) TestConcurrentOppositeMergesLeaveExactlyOneArtist() {
	a := s.createArtist("Band A")
	b := s.createArtist("Band B")

	results := make(chan error, 2)
	go func() {
		_, err := s.svc.MergeArtists(a.ID, b.ID)
		results <- err
	}()
	go func() {
		_, err := s.svc.MergeArtists(b.ID, a.ID)
		results <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-results:
		case <-time.After(30 * time.Second):
			s.FailNow("concurrent opposite merges deadlocked")
		}
	}

	var remaining int64
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("id IN ?", []uint{a.ID, b.ID}).Count(&remaining).Error)
	s.Equal(int64(1), remaining, "a merge race must not delete both artists")
}

// ──────────────────────────────────────────────
// Foreign-key tables the ON DELETE clause would have eaten
// ──────────────────────────────────────────────

// radio_plays.artist_id is ON DELETE SET NULL, and the row keeps
// match_state='matched'. scopePlaysForArtistRematch only revisits plays that are
// BOTH artist_id IS NULL and in an unmatched/no-match/ambiguous state, so a play
// blanked by a merge is invisible to every future rematch sweep — permanently
// lost match work that raises nothing at the time.
func (s *ArtistMergeIntegrationSuite) TestMergeRepointsMatchedRadioPlays() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	playID := s.seedMatchedPlay(loser.ID, "Some Track")

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var play catalogm.RadioPlay
	s.Require().NoError(s.db.First(&play, playID).Error)
	s.Require().NotNil(play.ArtistID,
		"a merge must not blank a matched play: the row stays match_state='matched' and no "+
			"rematch sweep will ever look at it again")
	s.Equal(canonical.ID, *play.ArtistID)
	s.Equal(catalogm.RadioPlayMatchStateMatched, play.MatchState)
}

// artist_link_suggestions is ON DELETE CASCADE, so a merge used to silently
// discard the losing artist's pending link triage. It is unique on
// (artist_id, platform, url), so a duplicate candidate is dropped rather than
// carried into a constraint violation.
func (s *ArtistMergeIntegrationSuite) TestMergeRepointsLinkSuggestions() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")

	moved := s.seedLinkSuggestion(loser.ID, "https://dupe.bandcamp.com", catalogm.LinkSuggestionStatusPending)
	s.seedLinkSuggestion(canonical.ID, "https://shared.bandcamp.com", catalogm.LinkSuggestionStatusPending)
	colliding := s.seedLinkSuggestion(loser.ID, "https://shared.bandcamp.com", catalogm.LinkSuggestionStatusPending)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err, "a duplicate suggestion must not abort the merge")

	var survivor catalogm.ArtistLinkSuggestion
	s.Require().NoError(s.db.First(&survivor, moved).Error)
	s.Equal(canonical.ID, survivor.ArtistID,
		"pending link triage is human work and must survive the merge")

	var gone catalogm.ArtistLinkSuggestion
	s.ErrorIs(s.db.First(&gone, colliding).Error, gorm.ErrRecordNotFound,
		"the colliding suggestion must be dropped before the re-point")
}

// When the two rows disagree, the HUMAN's answer wins. Keeping the canonical
// artist's still-pending row over a verdict someone already reached would put a
// triaged URL back in the admin review queue — the opposite of why these rows
// are rescued from the CASCADE at all.
func (s *ArtistMergeIntegrationSuite) TestMergeKeepsTheReviewedLinkSuggestion() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")

	stillPending := s.seedLinkSuggestion(
		canonical.ID, "https://contested.bandcamp.com", catalogm.LinkSuggestionStatusPending)
	reviewed := s.seedLinkSuggestion(
		loser.ID, "https://contested.bandcamp.com", catalogm.LinkSuggestionStatusRejected)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var kept catalogm.ArtistLinkSuggestion
	s.Require().NoError(s.db.First(&kept, reviewed).Error,
		"a reviewed verdict must not be discarded in favour of a pending duplicate")
	s.Equal(canonical.ID, kept.ArtistID)
	s.Equal(catalogm.LinkSuggestionStatusRejected, kept.Status)

	var superseded catalogm.ArtistLinkSuggestion
	s.ErrorIs(s.db.First(&superseded, stillPending).Error, gorm.ErrRecordNotFound,
		"the pending duplicate must give way to the reviewed row")
}

// artist_communities.label_artist_id is ON DELETE CASCADE, so merging away a
// community's label artist deletes the whole community row while every member
// still carries artists.community_id pointing at it — the community renders
// unlabelled for all of them until the next detection run.
func (s *ArtistMergeIntegrationSuite) TestMergeRepointsCommunityLabel() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")

	s.Require().NoError(s.db.Exec(
		"INSERT INTO artist_communities (id, label_artist_id, member_count) VALUES (1, ?, 6)",
		loser.ID).Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var labelID uint
	s.Require().NoError(s.db.Raw(
		"SELECT label_artist_id FROM artist_communities WHERE id = 1").Scan(&labelID).Error)
	s.Equal(canonical.ID, labelID,
		"the community must keep a label rather than cascade away with the merged artist")
}

// musicbrainz_artist_id is not a Psychic Homily reference, but radio matching
// resolves plays by MBID before falling back to name, so dropping the merged
// artist's MBID silently degrades matching from then on.
func (s *ArtistMergeIntegrationSuite) TestMergeCarriesMusicBrainzIDWhenCanonicalHasNone() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).Where("id = ?", loser.ID).
		Update("musicbrainz_artist_id", "mbid-from-loser").Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var survivor catalogm.Artist
	s.Require().NoError(s.db.First(&survivor, canonical.ID).Error)
	s.Require().NotNil(survivor.MusicBrainzArtistID)
	s.Equal("mbid-from-loser", *survivor.MusicBrainzArtistID)
}

// ...but never over one the canonical artist already has. Two MBIDs is a data
// question for a human, and overwriting is the destructive answer.
func (s *ArtistMergeIntegrationSuite) TestMergeKeepsTheCanonicalMusicBrainzID() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).Where("id = ?", canonical.ID).
		Update("musicbrainz_artist_id", "mbid-canonical").Error)
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).Where("id = ?", loser.ID).
		Update("musicbrainz_artist_id", "mbid-from-loser").Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var survivor catalogm.Artist
	s.Require().NoError(s.db.First(&survivor, canonical.ID).Error)
	s.Require().NotNil(survivor.MusicBrainzArtistID)
	s.Equal("mbid-canonical", *survivor.MusicBrainzArtistID)
}

// PSY-1896: notification_log carries a SECOND entity reference the polymorphic
// inventory has no concept of. subject_entity_id names the followed artist an
// alert is about; its column is not entity_id and its type is implied by the
// row's own discriminator, so the inventory loop cannot see it. It has no
// foreign key either, so a merge that ignores it raises nothing — the alert just
// loses the band's name in the user's inbox forever after.
func (s *ArtistMergeIntegrationSuite) TestMergeRepointsArtistShowAlertSubject() {
	canonical := s.createArtist("Canonical Band")
	loser := s.createArtist("Dupe Band")
	reader := s.createUser("alert-reader")

	// An alert the user already received, about a show, attributed to the artist
	// that is about to be merged away. entity_id is a show id and is deliberately
	// left alone by this merge.
	s.Require().NoError(s.db.Exec(`
		INSERT INTO notification_log (user_id, entity_type, entity_id, subject_entity_id, channel, sent_at)
		VALUES (?, 'artist_show_alert', 987654, ?, 'in_app', now())`, reader.ID, loser.ID).Error)

	_, err := s.svc.MergeArtists(canonical.ID, loser.ID)
	s.Require().NoError(err)

	var subjects []uint
	s.Require().NoError(s.db.Table("notification_log").
		Where("user_id = ? AND entity_type = 'artist_show_alert'", reader.ID).
		Pluck("subject_entity_id", &subjects).Error)
	s.Require().Len(subjects, 1)
	s.Equal(canonical.ID, subjects[0],
		"the alert must name the surviving artist, not an id that no longer exists")
}
