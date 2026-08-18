package catalog

import (
	"encoding/json"
	"fmt"
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
		"DELETE FROM radio_plays",
		"DELETE FROM radio_episodes",
		"DELETE FROM radio_shows",
		"DELETE FROM radio_stations",
		"DELETE FROM notification_log",
		"DELETE FROM notification_filters",
		"DELETE FROM artist_aliases",
		"DELETE FROM entity_tags",
		"DELETE FROM tag_votes",
		"DELETE FROM tags",
		"DELETE FROM artists",
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

func (s *ArtistMergeIntegrationSuite) seedLinkSuggestion(artistID uint, url string) uint {
	suggestion := &catalogm.ArtistLinkSuggestion{
		ArtistID:   artistID,
		Platform:   "bandcamp",
		URL:        url,
		Source:     "musicbrainz",
		Confidence: "high",
		Status:     catalogm.LinkSuggestionStatusPending,
		CreatedAt:  time.Now().UTC(),
	}
	s.Require().NoError(s.db.Create(suggestion).Error)
	return suggestion.ID
}

// ──────────────────────────────────────────────
// Schema drift guards
// ──────────────────────────────────────────────

// TestArtistEntityRefsCoverSchema is the guard this ticket exists to add.
// artistEntityRefs is a hand-maintained list, and the failure mode of falling
// behind is silent: a new entity_type table simply keeps rows pointing at a
// deleted artist. That is exactly what happened to pending_entity_edits, which
// gained a suggest-edit route for artists and never gained a merge step.
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

	covered := artistEntityRefTables()
	for _, table := range tables {
		s.Truef(covered[table],
			"table %q has an entity_type column but is not in artistEntityRefs — an artist merge "+
				"would leave its rows pointing at a deleted artist. Add it (with its unique key), "+
				"or handle it in a dedicated step and name it in artistRefsRepointedElsewhere.", table)
	}

	// And the reverse: nothing in either list may have been dropped by a
	// migration, which would make every merge fail at runtime.
	present := map[string]bool{}
	for _, t := range tables {
		present[t] = true
	}
	for _, ref := range artistEntityRefs {
		s.Truef(present[ref.table],
			"artistEntityRefs lists %q, which no longer has an entity_type column", ref.table)
	}
	for _, table := range artistRefsRepointedElsewhere {
		s.Truef(present[table],
			"artistRefsRepointedElsewhere lists %q, which no longer has an entity_type column", table)
	}
}

// TestArtistEntityRefColumnsExist closes the gap the coverage guard leaves: a
// ref may name the right table and still name the wrong id or dedupe column.
// A wrong idCol aborts every merge loudly, but a wrong dedupe key only surfaces
// as a unique-violation on the one merge where two rows actually collide —
// rare enough to reach production.
func (s *ArtistMergeIntegrationSuite) TestArtistEntityRefColumnsExist() {
	for _, ref := range artistEntityRefs {
		for _, col := range append([]string{ref.idCol}, ref.key...) {
			var count int64
			s.Require().NoError(s.db.Raw(`
				SELECT COUNT(*) FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = ? AND column_name = ?
			`, ref.table, col).Scan(&count).Error)
			s.Equalf(int64(1), count,
				"artistEntityRefs names column %q on table %q, which does not exist", col, ref.table)
		}
	}
}

// TestArtistForeignKeysAreAllHandled is the second drift guard, and the one that
// covers the tables the database itself would quietly empty.
//
// Nine of the eleven foreign keys to artists cascade on delete and one sets
// NULL, so a table this merge does not handle does NOT raise an error when the
// losing artist goes — its rows are silently destroyed, or silently un-matched.
// artistFKTables records the decision for each; this asserts the set is still
// the schema's.
func (s *ArtistMergeIntegrationSuite) TestArtistForeignKeysAreAllHandled() {
	var tables []string
	s.Require().NoError(s.db.Raw(`
		SELECT DISTINCT tc.table_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND ccu.table_name = 'artists'
		  AND ccu.column_name = 'id'
		ORDER BY tc.table_name
	`).Scan(&tables).Error)
	s.Require().NotEmpty(tables)

	s.ElementsMatch(artistFKTables, tables,
		"the set of tables with a foreign key to artists has changed. A new one must be "+
			"handled explicitly in the merge — most of these CASCADE, so an unhandled table "+
			"loses its rows silently when the merged artist is deleted.")
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
		  AND (c.column_name LIKE '%artist_id' OR c.column_name LIKE '%artist_ids')
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
// at the losing artist. Weaker than a per-table fixture and much cheaper to
// keep, and it grows automatically — a table added to artistEntityRefs is
// checked here without anyone remembering to.
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

	for _, ref := range artistEntityRefs {
		var remaining int64
		// #nosec G201 -- table and column come from the hardcoded inventory.
		query := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE entity_type = 'artist' AND %s = ?", ref.table, ref.idCol)
		s.Require().NoError(s.db.Raw(query, loser.ID).Scan(&remaining).Error)
		s.Zerof(remaining, "%s still points at the merged-away artist", ref.table)
	}
	for _, table := range artistRefsRepointedElsewhere {
		var remaining int64
		// #nosec G201 -- table comes from the hardcoded inventory.
		query := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE entity_type = 'artist' AND entity_id = ?", table)
		s.Require().NoError(s.db.Raw(query, loser.ID).Scan(&remaining).Error)
		s.Zerof(remaining, "%s still points at the merged-away artist", table)
	}
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

	moved := s.seedLinkSuggestion(loser.ID, "https://dupe.bandcamp.com")
	s.seedLinkSuggestion(canonical.ID, "https://shared.bandcamp.com")
	colliding := s.seedLinkSuggestion(loser.ID, "https://shared.bandcamp.com")

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
