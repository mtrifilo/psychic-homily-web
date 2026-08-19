package catalog

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// Venue merge is destructive — it deletes shows and a venue row — so every test
// here runs against a real Postgres with the real constraints. The two cases
// that matter most are the ones a one-off migration learned the hard way:
//
//   - Reassigning venue_id without first deleting duplicate shows violates
//     shows_artist_venue_eventdate_uniq.
//   - Deleting a duplicate show drops any support act the surviving show does
//     not list, silently, because show_artists cascades on show_id.
//
// A mocked or SQLite test cannot observe either: both are enforced by a
// Postgres partial unique index and an ON DELETE CASCADE respectively.

type VenueMergeIntegrationSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *VenueService
}

func (s *VenueMergeIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = NewVenueService(s.db)
}

func (s *VenueMergeIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *VenueMergeIntegrationSuite) SetupTest() {
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
		"DELETE FROM venue_confirmations",
		"DELETE FROM entity_tags",
		"DELETE FROM comments",
		"DELETE FROM notification_filters",
		"DELETE FROM show_artists",
		"DELETE FROM show_venues",
		"DELETE FROM shows",
		"DELETE FROM artists",
		"DELETE FROM venues",
		"DELETE FROM tags",
		"DELETE FROM users",
	} {
		_, err := sqlDB.Exec(stmt)
		s.Require().NoError(err, stmt)
	}
}

func TestVenueMergeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(VenueMergeIntegrationSuite))
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func (s *VenueMergeIntegrationSuite) createVenue(name string) *catalogm.Venue {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	v := &catalogm.Venue{Name: name, Slug: &slug, City: "Minneapolis", State: "MN"}
	s.Require().NoError(s.db.Create(v).Error)
	return v
}

// createVerifiedVenue is createVenue plus the verification flag, set with an
// explicit UPDATE so neither helper depends on GORM's zero-value-vs-column-
// default behavior for bools.
func (s *VenueMergeIntegrationSuite) createVerifiedVenue(name string) *catalogm.Venue {
	v := s.createVenue(name)
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).
		Where("id = ?", v.ID).Update("verified", true).Error)
	v.Verified = true
	return v
}

func (s *VenueMergeIntegrationSuite) createArtist(name string) *catalogm.Artist {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

func (s *VenueMergeIntegrationSuite) createUser(name string) *authm.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	u := &authm.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

// createShow builds a show at venue on date with the given bill, wiring both
// show_venues and the denormalized show_artists.venue_id/event_date that
// shows_artist_venue_eventdate_uniq actually indexes.
func (s *VenueMergeIntegrationSuite) createShow(
	title string, venue *catalogm.Venue, date time.Time, bill ...*catalogm.Artist,
) *catalogm.Show {
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	show := &catalogm.Show{Title: title, Slug: &slug, EventDate: date}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)

	for i, artist := range bill {
		d := date
		vid := venue.ID
		s.Require().NoError(s.db.Create(&catalogm.ShowArtist{
			ShowID:    show.ID,
			ArtistID:  artist.ID,
			Position:  i,
			SetType:   "performer",
			EventDate: &d,
			VenueID:   &vid,
		}).Error)
	}
	return show
}

func (s *VenueMergeIntegrationSuite) showExists(id uint) bool {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", id).Count(&n).Error)
	return n == 1
}

// mergeAuditRow is the audit_logs projection the merge tests assert on.
type mergeAuditRow struct {
	ActorID    *uint
	EntityType string
	EntityID   uint
	Metadata   string
}

// waitForMergeAuditLog polls for the merge audit entry of ONE canonical venue.
//
// Scoped by entity_id rather than action alone because the audit write is
// fire-and-forget (GoSafe): an earlier test's write can land after this test's
// SetupTest truncation, so a query matching only on action would pick up a
// stray row from a different merge.
func (s *VenueMergeIntegrationSuite) waitForMergeAuditLog(canonicalID uint) (mergeAuditRow, bool) {
	for i := 0; i < 100; i++ {
		var row mergeAuditRow
		err := s.db.Raw(
			`SELECT actor_id, entity_type, entity_id, metadata::text AS metadata
			 FROM audit_logs WHERE action = ? AND entity_id = ? LIMIT 1`,
			AuditActionMergeVenues, canonicalID).Scan(&row).Error
		s.Require().NoError(err)
		if row.EntityType != "" {
			return row, true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return mergeAuditRow{}, false
}

func (s *VenueMergeIntegrationSuite) billOf(showID uint) []uint {
	var ids []uint
	s.Require().NoError(s.db.Model(&catalogm.ShowArtist{}).
		Where("show_id = ?", showID).Order("position ASC").Pluck("artist_id", &ids).Error)
	return ids
}

// ──────────────────────────────────────────────
// The collision case
// ──────────────────────────────────────────────

// TestMergeDeduplicatesCollidingShows is the case that makes a naive merge
// fail. Both venues host the same artist on the same date, so simply setting
// show_artists.venue_id to the canonical venue would produce two rows with the
// same (artist_id, venue_id, event_date) and violate
// shows_artist_venue_eventdate_uniq.
//
// The merge must delete the duplicate show first and come back clean.
func (s *VenueMergeIntegrationSuite) TestMergeDeduplicatesCollidingShows() {
	canonical := s.createVenue("7th Street Entry")
	loser := s.createVenue("7th St Entry")
	artist := s.createArtist("Duplicated Band")
	date := time.Date(2026, 3, 14, 20, 0, 0, 0, time.UTC)

	winnerShow := s.createShow("canonical night", canonical, date, artist)
	loserShow := s.createShow("duplicate night", loser, date, artist)

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err, "merge must not violate shows_artist_venue_eventdate_uniq")

	s.Equal(int64(1), result.DuplicateShows, "the colliding show must be counted as a duplicate")
	s.True(s.showExists(winnerShow.ID), "the canonical venue's show survives")
	s.False(s.showExists(loserShow.ID), "the duplicate show is deleted")

	// The index is satisfied: exactly one bill row for this pair.
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.ShowArtist{}).
		Where("artist_id = ? AND venue_id = ? AND event_date = ?", artist.ID, canonical.ID, date).
		Count(&n).Error)
	s.Equal(int64(1), n, "exactly one (artist, venue, date) row must remain")

	// And the losing venue is gone with no dangling references.
	var venues int64
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", loser.ID).Count(&venues).Error)
	s.Zero(venues, "the merged-from venue must be deleted")

	var orphans int64
	s.Require().NoError(s.db.Model(&catalogm.ShowArtist{}).
		Where("venue_id = ?", loser.ID).Count(&orphans).Error)
	s.Zero(orphans, "no show_artists row may still point at the merged-from venue")
}

// TestMergeWithManyCollisions exercises the shape that broke the one-off
// migration at scale: every show on the losing venue collides.
func (s *VenueMergeIntegrationSuite) TestMergeWithManyCollisions() {
	canonical := s.createVenue("Metro Baltimore")
	loser := s.createVenue("Metro Gallery")

	const n = 25
	base := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		artist := s.createArtist(fmt.Sprintf("Band %d", i))
		date := base.AddDate(0, 0, i)
		s.createShow(fmt.Sprintf("canonical %d", i), canonical, date, artist)
		s.createShow(fmt.Sprintf("dupe %d", i), loser, date, artist)
	}

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)
	s.Equal(int64(n), result.DuplicateShows, "every losing show collides and must be deduped")

	var shows int64
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Count(&shows).Error)
	s.Equal(int64(n), shows, "only the canonical venue's shows survive")
}

// ──────────────────────────────────────────────
// The support-act rescue case
// ──────────────────────────────────────────────

// TestMergeRescuesSupportActs pins the silent-data-loss case. The duplicate
// show lists a support act the surviving show does not; show_artists cascades
// on show_id, so deleting the duplicate outright would drop that association
// with no error and no trace.
//
// The support act must end up on the surviving bill instead.
func (s *VenueMergeIntegrationSuite) TestMergeRescuesSupportActs() {
	canonical := s.createVenue("7th Street Entry")
	loser := s.createVenue("7th St Entry")
	headliner := s.createArtist("Headliner")
	support := s.createArtist("Scaphe")
	date := time.Date(2026, 4, 2, 20, 0, 0, 0, time.UTC)

	// The surviving show knows only the headliner.
	winnerShow := s.createShow("canonical night", canonical, date, headliner)
	// The duplicate knows the headliner AND a support act.
	loserShow := s.createShow("duplicate night", loser, date, headliner, support)

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	s.Equal(int64(1), result.DuplicateShows)
	s.Equal(int64(1), result.SupportActsRescued, "the support act must be rescued, not dropped")

	s.False(s.showExists(loserShow.ID))

	bill := s.billOf(winnerShow.ID)
	s.ElementsMatch([]uint{headliner.ID, support.ID}, bill,
		"the support act must survive on the canonical show's bill")

	// Rescued acts append after the existing bill rather than fighting it for
	// ordering, so the headliner keeps position 0.
	var headlinerRow, supportRow catalogm.ShowArtist
	s.Require().NoError(s.db.Where("show_id = ? AND artist_id = ?", winnerShow.ID, headliner.ID).
		First(&headlinerRow).Error)
	s.Require().NoError(s.db.Where("show_id = ? AND artist_id = ?", winnerShow.ID, support.ID).
		First(&supportRow).Error)
	s.Equal(0, headlinerRow.Position, "the surviving bill keeps its ordering")
	s.Greater(supportRow.Position, headlinerRow.Position, "the rescued act appends after it")

	// The rescued row must carry the canonical venue, or it would be invisible
	// to every venue-scoped query.
	s.Require().NotNil(supportRow.VenueID)
	s.Equal(canonical.ID, *supportRow.VenueID)
}

// TestMergeDoesNotDuplicateAnAlreadyPresentAct is the other half of the rescue
// rule: an artist the surviving show already lists must NOT be moved across,
// or the move violates the (show_id, artist_id) primary key.
func (s *VenueMergeIntegrationSuite) TestMergeDoesNotDuplicateAnAlreadyPresentAct() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	artist := s.createArtist("Both Bills")
	date := time.Date(2026, 5, 5, 20, 0, 0, 0, time.UTC)

	winnerShow := s.createShow("canonical", canonical, date, artist)
	s.createShow("dupe", loser, date, artist)

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)
	s.Zero(result.SupportActsRescued, "an artist already on the bill is not re-added")
	s.Len(s.billOf(winnerShow.ID), 1)
}

// ──────────────────────────────────────────────
// Non-colliding references
// ──────────────────────────────────────────────

// TestMergeReassignsNonDuplicateShows covers the ordinary path: a show that
// does not collide simply moves to the canonical venue.
func (s *VenueMergeIntegrationSuite) TestMergeReassignsNonDuplicateShows() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	artist := s.createArtist("Unique Band")

	show := s.createShow("only on the loser", loser,
		time.Date(2026, 6, 6, 20, 0, 0, 0, time.UTC), artist)

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	s.Zero(result.DuplicateShows, "nothing collides")
	s.Equal(int64(1), result.ShowVenuesMoved)
	s.True(s.showExists(show.ID), "a non-duplicate show must survive the merge")

	var sv catalogm.ShowVenue
	s.Require().NoError(s.db.Where("show_id = ?", show.ID).First(&sv).Error)
	s.Equal(canonical.ID, sv.VenueID)
}

// TestMergeMovesVenueConfirmations covers a table the one-off migration did not
// know about: venue_confirmations arrived after it. It cascades on venue
// delete, so an unhandled merge would DESTROY user vouches rather than orphan
// them — a silent loss of contributions.
func (s *VenueMergeIntegrationSuite) TestMergeMovesVenueConfirmations() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	mover := s.createUser("mover")
	both := s.createUser("both")

	for _, c := range []catalogm.VenueConfirmation{
		{UserID: mover.ID, VenueID: loser.ID},
		{UserID: both.ID, VenueID: loser.ID},
		{UserID: both.ID, VenueID: canonical.ID},
	} {
		s.Require().NoError(s.db.Create(&c).Error)
	}

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	s.Equal(int64(1), result.ConfirmationsMoved, "only the non-conflicting confirmation moves")

	var users []uint
	s.Require().NoError(s.db.Model(&catalogm.VenueConfirmation{}).
		Where("venue_id = ?", canonical.ID).Pluck("user_id", &users).Error)
	s.ElementsMatch([]uint{mover.ID, both.ID}, users,
		"both vouchers must survive on the canonical venue, deduped to one row each")
}

// TestMergeRepointsPolymorphicReferences covers the tables with no foreign key,
// where a missed row does not fail loudly — it just points at a venue id that
// no longer exists.
func (s *VenueMergeIntegrationSuite) TestMergeRepointsPolymorphicReferences() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	user := s.createUser("commenter")

	tag := &catalogm.Tag{Name: "diy-space", Slug: "diy-space", Category: catalogm.TagCategoryGenre}
	s.Require().NoError(s.db.Create(tag).Error)
	s.Require().NoError(s.db.Create(&catalogm.EntityTag{
		TagID: tag.ID, EntityType: "venue", EntityID: loser.ID, AddedByUserID: user.ID,
	}).Error)

	s.Require().NoError(s.db.Exec(
		`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html, kind, created_at, updated_at)
		 VALUES ('venue', ?, ?, 'great room', '<p>great room</p>', 'comment', NOW(), NOW())`,
		loser.ID, user.ID).Error)

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)
	s.GreaterOrEqual(result.EntityRefsMoved, int64(2))

	for _, table := range []string{"entity_tags", "comments"} {
		var stale int64
		s.Require().NoError(s.db.Table(table).
			Where("entity_type = 'venue' AND entity_id = ?", loser.ID).Count(&stale).Error)
		s.Zerof(stale, "%s left a dangling reference to the deleted venue", table)

		var moved int64
		s.Require().NoError(s.db.Table(table).
			Where("entity_type = 'venue' AND entity_id = ?", canonical.ID).Count(&moved).Error)
		s.Equalf(int64(1), moved, "%s row must be re-pointed at the canonical venue", table)
	}
}

// TestMergeRewritesNotificationFilters covers notification_filters.venue_ids —
// a bigint[] with no foreign key, so a stale id survives a venue delete
// silently and the filter simply stops matching anything.
func (s *VenueMergeIntegrationSuite) TestMergeRewritesNotificationFilters() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	tracksLoser := s.createUser("tracks-loser")
	tracksBoth := s.createUser("tracks-both")

	s.Require().NoError(s.db.Exec(
		`INSERT INTO notification_filters (user_id, name, venue_ids, created_at, updated_at)
		 VALUES (?, 'tracks loser', ARRAY[?::bigint], NOW(), NOW()),
		        (?, 'tracks both',  ARRAY[?::bigint, ?::bigint], NOW(), NOW())`,
		tracksLoser.ID, loser.ID, tracksBoth.ID, loser.ID, canonical.ID).Error)

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)
	s.Equal(int64(2), result.FiltersUpdated)

	var rows []struct {
		UserID   uint
		VenueIDs string
	}
	s.Require().NoError(s.db.Raw(
		`SELECT user_id, venue_ids::text AS venue_ids FROM notification_filters ORDER BY user_id`).
		Scan(&rows).Error)
	s.Require().Len(rows, 2)

	want := fmt.Sprintf("{%d}", canonical.ID)
	for _, r := range rows {
		s.Equal(want, r.VenueIDs,
			"the filter must track the canonical venue exactly once, with no stale id")
	}
}

// ──────────────────────────────────────────────
// Preview
// ──────────────────────────────────────────────

// TestPreviewReportsWithoutCommitting is the core guarantee of the preview:
// identical counts to the real merge, and nothing changed on disk.
func (s *VenueMergeIntegrationSuite) TestPreviewReportsWithoutCommitting() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	headliner := s.createArtist("Headliner")
	support := s.createArtist("Support")
	date := time.Date(2026, 7, 7, 20, 0, 0, 0, time.UTC)

	winnerShow := s.createShow("canonical", canonical, date, headliner)
	loserShow := s.createShow("dupe", loser, date, headliner, support)
	keeper := s.createShow("unique", loser, date.AddDate(0, 0, 7), headliner)

	preview, err := s.svc.PreviewMergeVenues(canonical.ID, loser.ID)
	s.Require().NoError(err)

	s.Equal(int64(1), preview.DuplicateShows)
	s.Equal(int64(1), preview.SupportActsRescued)
	s.Equal("Canonical Room", preview.CanonicalVenueName)
	s.Equal("Dupe Room", preview.MergedVenueName)

	// Nothing may have moved.
	s.True(s.showExists(loserShow.ID), "preview must not delete the duplicate show")
	s.True(s.showExists(keeper.ID))
	s.Len(s.billOf(winnerShow.ID), 1, "preview must not rescue anything for real")

	var venues int64
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", loser.ID).Count(&venues).Error)
	s.Equal(int64(1), venues, "preview must not delete the venue")

	// And the real merge must agree with what the preview promised.
	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)
	s.Equal(preview.DuplicateShows, result.DuplicateShows)
	s.Equal(preview.SupportActsRescued, result.SupportActsRescued)
	s.Equal(preview.ShowVenuesMoved, result.ShowVenuesMoved)
	s.Equal(preview.ConfirmationsMoved, result.ConfirmationsMoved)
	s.Equal(preview.EntityRefsMoved, result.EntityRefsMoved)
}

// TestPreviewWritesNoAuditLog — a preview is a read. Only the commit is an
// admin action worth recording.
//
// The audit write is async, so asserting absence straight after the preview
// would pass even if the preview DID schedule one. A real merge on a second
// pair is committed alongside, and its audit row is awaited first: once that
// has landed, the async path has demonstrably had time to run, and the
// preview's silence is meaningful.
func (s *VenueMergeIntegrationSuite) TestPreviewWritesNoAuditLog() {
	previewCanonical := s.createVenue("Preview Canonical")
	previewLoser := s.createVenue("Preview Dupe")
	mergeCanonical := s.createVenue("Merge Canonical")
	mergeLoser := s.createVenue("Merge Dupe")

	_, err := s.svc.PreviewMergeVenues(previewCanonical.ID, previewLoser.ID)
	s.Require().NoError(err)

	_, err = s.svc.MergeVenues(mergeCanonical.ID, mergeLoser.ID, 0)
	s.Require().NoError(err)

	_, found := s.waitForMergeAuditLog(mergeCanonical.ID)
	s.Require().True(found, "the committed merge must have written its audit row by now")

	var n int64
	s.Require().NoError(s.db.Table("audit_logs").
		Where("action = ? AND entity_id = ?", AuditActionMergeVenues, previewCanonical.ID).
		Count(&n).Error)
	s.Zero(n, "a preview commits nothing, so it records nothing")
}

// ──────────────────────────────────────────────
// Audit log
// ──────────────────────────────────────────────

// TestMergeWritesAuditLog covers the destructive-action record. The write is
// fire-and-forget (GoSafe), so this polls rather than asserting immediately.
func (s *VenueMergeIntegrationSuite) TestMergeWritesAuditLog() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	admin := s.createUser("admin")
	artist := s.createArtist("Band")
	date := time.Date(2026, 8, 8, 20, 0, 0, 0, time.UTC)
	s.createShow("canonical", canonical, date, artist)
	s.createShow("dupe", loser, date, artist)

	_, err := s.svc.MergeVenues(canonical.ID, loser.ID, admin.ID)
	s.Require().NoError(err)

	row, found := s.waitForMergeAuditLog(canonical.ID)
	s.Require().True(found, "a venue merge must leave an audit trail")

	s.Require().NotNil(row.ActorID)
	s.Equal(admin.ID, *row.ActorID)
	s.Equal("venue", row.EntityType)
	s.Equal(canonical.ID, row.EntityID)
	s.Contains(row.Metadata, "duplicate_shows", "the audit entry records what was destroyed")
	s.Contains(row.Metadata, "support_acts_rescued")
	s.Contains(row.Metadata, "Dupe Room", "the merged venue's name is gone from venues — record it here")
}

// ──────────────────────────────────────────────
// Guards and edge cases
// ──────────────────────────────────────────────

func (s *VenueMergeIntegrationSuite) TestMergeIntoSelfIsRejected() {
	v := s.createVenue("Only Room")

	_, err := s.svc.MergeVenues(v.ID, v.ID, 0)
	s.Require().Error(err)
	var venueErr *apperrors.VenueError
	s.Require().ErrorAs(err, &venueErr)
	s.Equal(apperrors.CodeVenueMergeInvalid, venueErr.Code)

	// The venue must still be there — a self-merge that deleted the row would
	// be catastrophic and is exactly what the guard exists to prevent.
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", v.ID).Count(&n).Error)
	s.Equal(int64(1), n)
}

func (s *VenueMergeIntegrationSuite) TestMergeRejectsZeroIDs() {
	v := s.createVenue("Only Room")
	for _, tc := range []struct{ canonical, from uint }{{v.ID, 0}, {0, v.ID}, {0, 0}} {
		_, err := s.svc.MergeVenues(tc.canonical, tc.from, 0)
		s.Require().Error(err)
		var venueErr *apperrors.VenueError
		s.Require().ErrorAs(err, &venueErr)
		s.Equal(apperrors.CodeVenueMergeInvalid, venueErr.Code)
	}
}

func (s *VenueMergeIntegrationSuite) TestMergeMissingVenueIsNotFound() {
	v := s.createVenue("Only Room")
	const missing = 9_999_999

	for _, tc := range []struct {
		name            string
		canonical, from uint
	}{
		{"canonical missing", missing, v.ID},
		{"merge-from missing", v.ID, missing},
	} {
		s.Run(tc.name, func() {
			_, err := s.svc.MergeVenues(tc.canonical, tc.from, 0)
			s.Require().Error(err)
			var venueErr *apperrors.VenueError
			s.Require().ErrorAs(err, &venueErr)
			s.Equal(apperrors.CodeVenueNotFound, venueErr.Code)

			var n int64
			s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", v.ID).Count(&n).Error)
			s.Equal(int64(1), n, "a failed merge must not delete the venue that does exist")
		})
	}
}

// TestMergeEmptyVenue — a venue with no shows and no references is the common
// case for a freshly ingested duplicate. It must merge cleanly, not error on
// the empty temp table.
func (s *VenueMergeIntegrationSuite) TestMergeEmptyVenue() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Empty Dupe")

	result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	s.Zero(result.DuplicateShows)
	s.Zero(result.SupportActsRescued)
	s.Zero(result.ShowVenuesMoved)

	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", loser.ID).Count(&n).Error)
	s.Zero(n)
}

// TestRepeatedMergesReuseTheConnectionCleanly guards the temp table's lifetime.
// venue_merge_dup is created per transaction; if it leaked onto the pooled
// connection the SECOND merge would fail with "relation already exists". A
// preview (which rolls back) followed by two commits covers both exits.
func (s *VenueMergeIntegrationSuite) TestRepeatedMergesReuseTheConnectionCleanly() {
	for i := 0; i < 3; i++ {
		canonical := s.createVenue(fmt.Sprintf("Canonical %d", i))
		loser := s.createVenue(fmt.Sprintf("Dupe %d", i))

		_, err := s.svc.PreviewMergeVenues(canonical.ID, loser.ID)
		s.Require().NoErrorf(err, "preview %d", i)

		_, err = s.svc.MergeVenues(canonical.ID, loser.ID, 0)
		s.Require().NoErrorf(err, "merge %d: temp table leaked across transactions", i)
	}
}

// TestFailedMergeRollsBackEverything proves the whole merge is one
// transaction. A foreign-key violation forced mid-merge must leave the
// database exactly as it was, never half-merged.
//
// The violation is forced by pointing a festival_artists row at a festival id
// that does not exist, deferred so it fires at COMMIT — after every step of the
// merge has already run.
func (s *VenueMergeIntegrationSuite) TestFailedMergeRollsBackEverything() {
	canonical := s.createVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	artist := s.createArtist("Band")
	date := time.Date(2026, 9, 9, 20, 0, 0, 0, time.UTC)
	winnerShow := s.createShow("canonical", canonical, date, artist)
	loserShow := s.createShow("dupe", loser, date, artist)

	// A trigger that raises on the venues DELETE — the LAST statement of the
	// merge, so every earlier step (dedup, rescue, reassignment) has already
	// run by the time it fires. That makes this a genuine mid-merge failure.
	s.Require().NoError(s.db.Exec(`
		CREATE OR REPLACE FUNCTION psy1597_boom() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'psy1597 forced failure'; END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER psy1597_boom_trg BEFORE DELETE ON venues
		FOR EACH ROW EXECUTE FUNCTION psy1597_boom();
	`).Error)
	defer func() {
		s.Require().NoError(s.db.Exec(`DROP TRIGGER IF EXISTS psy1597_boom_trg ON venues`).Error)
		s.Require().NoError(s.db.Exec(`DROP FUNCTION IF EXISTS psy1597_boom()`).Error)
	}()

	_, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().Error(err, "the forced failure must surface")

	// Everything the merge did before the failure must be undone.
	s.True(s.showExists(loserShow.ID), "the duplicate show must be restored by the rollback")
	s.True(s.showExists(winnerShow.ID))
	s.Len(s.billOf(loserShow.ID), 1, "the losing bill must be intact")

	var venues int64
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", loser.ID).Count(&venues).Error)
	s.Equal(int64(1), venues, "the venue must still exist after a rolled-back merge")

	var stillOnLoser int64
	s.Require().NoError(s.db.Model(&catalogm.ShowVenue{}).
		Where("venue_id = ?", loser.ID).Count(&stillOnLoser).Error)
	s.Equal(int64(1), stillOnLoser, "show_venues must not have been re-pointed")
}

// TestConcurrentOppositeMergesDoNotDeadlock fires the two merges most likely to
// deadlock — the same pair in opposite directions — at the same time.
//
// Whichever wins, the other must fail cleanly with "venue not found" (its
// target was deleted underneath it), never hang and never leave both venues
// deleted. Locking both rows in ascending id order is what makes that true; a
// merge that locked canonical-then-loser would let these two grab the rows in
// opposite orders and deadlock.
func (s *VenueMergeIntegrationSuite) TestConcurrentOppositeMergesDoNotDeadlock() {
	a := s.createVenue("Room A")
	b := s.createVenue("Room B")

	type outcome struct{ err error }
	results := make(chan outcome, 2)

	go func() {
		_, err := s.svc.MergeVenues(a.ID, b.ID, 0)
		results <- outcome{err}
	}()
	go func() {
		_, err := s.svc.MergeVenues(b.ID, a.ID, 0)
		results <- outcome{err}
	}()

	var errs []error
	for i := 0; i < 2; i++ {
		select {
		case r := <-results:
			errs = append(errs, r.err)
		case <-time.After(30 * time.Second):
			s.FailNow("concurrent opposite merges deadlocked")
		}
	}

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
		}
	}
	s.Equal(1, succeeded, "exactly one of two opposite merges may succeed; errors: %v", errs)

	// Exactly one venue survives — never zero.
	var remaining int64
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).
		Where("id IN ?", []uint{a.ID, b.ID}).Count(&remaining).Error)
	s.Equal(int64(1), remaining, "a merge race must not delete both venues")
}

// TestVenueForeignKeysAreAllHandled is the second drift guard, and the one
// that would have caught venue_confirmations.
//
// Four of the five foreign keys to venues cascade on delete, so a table this
// merge does not handle does NOT raise an error when the losing venue goes —
// its rows are silently destroyed. That is the worst failure mode available
// here: silent, permanent, and invisible in testing unless something asserts
// the list is complete.
func (s *VenueMergeIntegrationSuite) TestVenueForeignKeysAreAllHandled() {
	var tables []string
	s.Require().NoError(s.db.Raw(`
		SELECT DISTINCT tc.table_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND ccu.table_name = 'venues'
		  AND ccu.column_name = 'id'
		ORDER BY tc.table_name
	`).Scan(&tables).Error)
	s.Require().NotEmpty(tables)

	s.ElementsMatch(venueFKTables, tables,
		"the set of tables with a foreign key to venues has changed. A new one must be "+
			"handled explicitly in the merge — most of these CASCADE, so an unhandled table "+
			"loses its rows silently when the merged venue is deleted.")
}

// TestVenueEntityRefsCoverSchema is the drift guard. polymorphicEntityRefs is a
// hand-maintained list, and the failure mode of falling behind is silent:
// a new entity_type table simply keeps rows pointing at a deleted venue.
//
// The artist merge has its own copy of this assertion against the same list.
// Both are kept: the list is shared, but each merge should fail on its own terms
// so a reader of either suite learns that merge is guarded.
//
// This fails in CI the moment a migration adds one, which is the only cheap
// moment to notice.
func (s *VenueMergeIntegrationSuite) TestVenueEntityRefsCoverSchema() {
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
			"table %q has an entity_type column but is not in polymorphicEntityRefs — a venue merge "+
				"would leave its rows pointing at a deleted venue. Add it (with its unique key) "+
				"or, if it can never hold entity_type='venue', say so there.", table)
	}

	// And the reverse: nothing in the list may have been dropped by a
	// migration, which would make every merge fail at runtime.
	present := map[string]bool{}
	for _, t := range tables {
		present[t] = true
	}
	for _, ref := range polymorphicEntityRefs {
		s.Truef(present[ref.table],
			"polymorphicEntityRefs lists %q, which no longer has an entity_type column", ref.table)
	}

	// The tables handled by a dedicated step need the same reverse check. They
	// left polymorphicEntityRefs to gain a provenance decision, and taking that check
	// with them would mean a dropped entity_type column stops failing here and
	// starts failing at runtime, on every merge.
	for _, table := range refsRepointedElsewhere {
		s.Truef(present[table],
			"refsRepointedElsewhere lists %q, which no longer has an entity_type column", table)
	}
}

// ──────────────────────────────────────────────
// Revision privacy provenance
// ──────────────────────────────────────────────
//
// Revision address masking is decided at read time from the venue the revision
// points at. This merge re-points revisions and then deletes the venue the gate
// would have read, so it stamps the loser's rows on the way through. These
// tests cover the stamping; the read side of the same boundary lives in
// services/admin (TestGetEntityHistory_MergedUnverifiedVenueStaysRedacted),
// which is where the gate that honors the stamp is.

// createVenueRevision records one address edit against a venue: the diff shape
// whose values the read-time gate masks for an unverified venue.
func (s *VenueMergeIntegrationSuite) createVenueRevision(venueID, userID uint) uint {
	changes := json.RawMessage(
		`[{"field":"address","old_value":"1 Old St","new_value":"1234 Secret St"}]`)
	rev := &adminm.Revision{
		EntityType:   "venue",
		EntityID:     venueID,
		UserID:       userID,
		FieldChanges: &changes,
	}
	s.Require().NoError(s.db.Create(rev).Error)
	return rev.ID
}

// revisionByID reloads a revision through the model, so these tests read the
// mark the same way the redaction gate does rather than through a hand-written
// projection that would not notice a column-tag mistake.
func (s *VenueMergeIntegrationSuite) revisionByID(id uint) adminm.Revision {
	var rev adminm.Revision
	s.Require().NoError(s.db.First(&rev, id).Error)
	return rev
}

// The leak this closes: an unverified room's address history re-pointed onto a
// verified venue, with the row that gated it deleted in the same transaction.
func (s *VenueMergeIntegrationSuite) TestMergeMarksUnverifiedLosersRevisions() {
	canonical := s.createVerifiedVenue("Verified Room")
	loser := s.createVenue("Somebodys House")
	s.Require().False(loser.Verified)
	user := s.createUser("editor")

	loserRev := s.createVenueRevision(loser.ID, user.ID)
	canonicalRev := s.createVenueRevision(canonical.ID, user.ID)

	_, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	moved := s.revisionByID(loserRev)
	s.Equal(canonical.ID, moved.EntityID, "the revision must still be re-pointed")
	s.True(moved.FromUnverifiedVenue,
		"a revision carried off an unverified venue must be marked, or the read-time "+
			"gate publishes an address it was withholding before the merge")

	own := s.revisionByID(canonicalRev)
	s.False(own.FromUnverifiedVenue,
		"the canonical venue's OWN history must not be marked by a merge into it")
}

// A VERIFIED loser's revisions pass through in whatever state they arrived in,
// both directions of the one branch that decides it:
//
//   - unmarked stays unmarked, because nothing was being withheld and a merge
//     must not start withholding it;
//   - marked stays marked, because those rows came off an earlier unverified
//     venue and a chain of merges must not launder what one merge withholds.
func (s *VenueMergeIntegrationSuite) TestMergeLeavesVerifiedLosersRevisionsAsTheyAre() {
	canonical := s.createVerifiedVenue("Canonical Room")
	loser := s.createVerifiedVenue("Duplicate Room")
	user := s.createUser("editor")

	unmarked := s.createVenueRevision(loser.ID, user.ID)
	inherited := s.createVenueRevision(loser.ID, user.ID)
	s.Require().NoError(s.db.Model(&adminm.Revision{}).
		Where("id = ?", inherited).Update("from_unverified_venue", true).Error)

	_, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	moved := s.revisionByID(unmarked)
	s.Equal(canonical.ID, moved.EntityID)
	s.False(moved.FromUnverifiedVenue,
		"a verified venue's history is publishable; a merge must not start masking it")

	kept := s.revisionByID(inherited)
	s.Equal(canonical.ID, kept.EntityID)
	s.True(kept.FromUnverifiedVenue,
		"a mark inherited from an earlier merge must survive a later one")
}

// A preview must measure without leaving the mark behind, the same way it
// leaves no audit log and no deleted show.
func (s *VenueMergeIntegrationSuite) TestPreviewDoesNotMarkRevisions() {
	canonical := s.createVerifiedVenue("Preview Canonical")
	loser := s.createVenue("Preview Loser")
	user := s.createUser("editor")

	rev := s.createVenueRevision(loser.ID, user.ID)

	_, err := s.svc.PreviewMergeVenues(canonical.ID, loser.ID)
	s.Require().NoError(err)

	row := s.revisionByID(rev)
	s.Equal(loser.ID, row.EntityID, "a preview must roll back the re-point")
	s.False(row.FromUnverifiedVenue, "a preview must roll back the mark too")
}

// ──────────────────────────────────────────────
// Edit-history provenance
// ──────────────────────────────────────────────
//
// pending_entity_edits is the table the venue READ side treats as history:
// venueEditCounts builds the provenance stamp from it. entity_edit_audit_logs
// rides along because this merge re-points it too. Both moved inside the
// reassignEntityRefs loop until PSY-1788 pulled them out behind
// repointEditHistory, which requires the merge to state what happens to the
// redaction those rows were carrying.
//
// These tests exist to pin that the pull-out changed no behavior.

// seedPendingEdit records one proposed venue edit and returns its id.
//
// Written through the model rather than raw SQL so a column rename breaks the
// build instead of the test run.
func (s *VenueMergeIntegrationSuite) seedPendingEdit(
	venueID, userID uint, status adminm.PendingEditStatus,
) uint {
	changes := json.RawMessage(
		`[{"field":"address","old_value":"1 Old St","new_value":"1234 Secret St"}]`)
	edit := &adminm.PendingEntityEdit{
		EntityType:   adminm.PendingEditEntityVenue,
		EntityID:     venueID,
		SubmittedBy:  userID,
		FieldChanges: &changes,
		Summary:      "address fix",
		Status:       status,
	}
	s.Require().NoError(s.db.Create(edit).Error)
	return edit.ID
}

// seedEntityEditAuditLog records one applied venue edit in the audit trail.
func (s *VenueMergeIntegrationSuite) seedEntityEditAuditLog(venueID, userID uint) uint {
	metadata := json.RawMessage(`{"fields":["address"]}`)
	entry := &adminm.EntityEditAuditLog{
		ActorID:    &userID,
		EntityType: "venue",
		EntityID:   venueID,
		Metadata:   &metadata,
		CreatedAt:  time.Now().UTC(),
	}
	s.Require().NoError(s.db.Create(entry).Error)
	return entry.ID
}

func (s *VenueMergeIntegrationSuite) pendingEditByID(id uint) adminm.PendingEntityEdit {
	var edit adminm.PendingEntityEdit
	s.Require().NoError(s.db.First(&edit, id).Error)
	return edit
}

// The loser's verification state decides what happens to its REVISIONS. It
// decides nothing about its edit history, because edit content never reaches an
// anonymous reader in the first place — only venueEditCounts' aggregate does.
// Both branches run so that a future change which starts varying edit history
// by verification has to come here and say so.
func (s *VenueMergeIntegrationSuite) TestMergeMovesEditHistoryRegardlessOfLoserVerification() {
	for _, tc := range []struct {
		name          string
		loserVerified bool
	}{
		{"unverified loser", false},
		{"verified loser", true},
	} {
		s.Run(tc.name, func() {
			s.SetupTest()

			canonical := s.createVerifiedVenue("Canonical Room")
			loser := s.createVenue("Dupe Room")
			if tc.loserVerified {
				s.Require().NoError(s.db.Model(&catalogm.Venue{}).
					Where("id = ?", loser.ID).Update("verified", true).Error)
			}
			editor := s.createUser("editor")

			pendingEdit := s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusApproved)
			auditEntry := s.seedEntityEditAuditLog(loser.ID, editor.ID)

			result, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
			s.Require().NoError(err)

			moved := s.pendingEditByID(pendingEdit)
			s.Equal(canonical.ID, moved.EntityID,
				"the venue's edit history must follow it onto the canonical venue")

			var movedEntry adminm.EntityEditAuditLog
			s.Require().NoError(s.db.First(&movedEntry, auditEntry).Error)
			s.Equal(canonical.ID, movedEntry.EntityID,
				"the edit audit trail must not be left pointing at a deleted venue")

			s.GreaterOrEqual(result.EntityRefsMoved, int64(2),
				"both rows must still count toward the merge summary the admin reads")
		})
	}
}

// The dedupe is what keeps the re-point from violating
// idx_pending_entity_edits_unique. It was venue-merge behavior before PSY-1788
// and it has to survive the move into the helper: one contributor cannot end up
// with two pending edits on the canonical venue.
func (s *VenueMergeIntegrationSuite) TestMergeDropsDuplicateContributorPendingEdits() {
	canonical := s.createVerifiedVenue("Canonical Room")
	loser := s.createVenue("Dupe Room")
	bothVenues := s.createUser("prolific")
	loserOnly := s.createUser("newcomer")

	kept := s.seedPendingEdit(canonical.ID, bothVenues.ID, adminm.PendingEditStatusPending)
	dropped := s.seedPendingEdit(loser.ID, bothVenues.ID, adminm.PendingEditStatusPending)
	survives := s.seedPendingEdit(loser.ID, loserOnly.ID, adminm.PendingEditStatusPending)

	_, err := s.svc.MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	var droppedCount int64
	s.Require().NoError(s.db.Model(&adminm.PendingEntityEdit{}).
		Where("id = ?", dropped).Count(&droppedCount).Error)
	s.Zero(droppedCount,
		"the losing venue's edit from a contributor who already has one on the canonical "+
			"venue must be dropped — re-pointing it violates idx_pending_entity_edits_unique")

	s.Equal(canonical.ID, s.pendingEditByID(kept).EntityID,
		"the canonical venue's own pending edit must be untouched")
	s.Equal(canonical.ID, s.pendingEditByID(survives).EntityID,
		"a contributor with no competing edit on the canonical venue keeps theirs")

	var stale int64
	s.Require().NoError(s.db.Model(&adminm.PendingEntityEdit{}).
		Where("entity_type = ? AND entity_id = ?", adminm.PendingEditEntityVenue, loser.ID).
		Count(&stale).Error)
	s.Zero(stale, "pending_entity_edits left a dangling reference to the deleted venue")
}

// A preview must leave the edit history exactly where it found it, the same way
// it leaves no audit log and no deleted show. The dedupe makes this worth its
// own test: a preview that failed to roll back would have DELETED a row.
func (s *VenueMergeIntegrationSuite) TestPreviewDoesNotMoveOrDropEditHistory() {
	canonical := s.createVerifiedVenue("Preview Canonical")
	loser := s.createVenue("Preview Loser")
	editor := s.createUser("editor")

	kept := s.seedPendingEdit(canonical.ID, editor.ID, adminm.PendingEditStatusPending)
	wouldDrop := s.seedPendingEdit(loser.ID, editor.ID, adminm.PendingEditStatusPending)

	_, err := s.svc.PreviewMergeVenues(canonical.ID, loser.ID)
	s.Require().NoError(err)

	s.Equal(loser.ID, s.pendingEditByID(wouldDrop).EntityID,
		"a preview must roll back the re-point")
	s.Equal(canonical.ID, s.pendingEditByID(kept).EntityID)
}
