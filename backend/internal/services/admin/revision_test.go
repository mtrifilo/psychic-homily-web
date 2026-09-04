package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
	"psychic-homily-backend/internal/testutil"
)

// viewerPublic and viewerAdmin name the caller the read methods redact and gate
// for (PSY-1717/1715), so a call site says which view it is asserting instead of
// trailing a bare struct literal. See the ADMIN TIER section at the foot of this
// file.
//
// viewerPublic is the ZERO viewer: anonymous, no user id, no admin bit. An
// authenticated non-admin who is not the show's submitter resolves to the same
// answers, and where the submitter tier diverges the test names its own viewer
// rather than reusing these.
var (
	viewerPublic = contracts.RevisionViewer{}
	viewerAdmin  = contracts.RevisionViewer{IsAdmin: true}
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type RevisionServiceIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *RevisionService
}

func (s *RevisionServiceIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	s.svc = NewRevisionService(s.testDB.DB)
}

func (s *RevisionServiceIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RevisionServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	// Before users and venues: shows carry FKs to both.
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

// rollbackErr runs a rollback and reports only whether it failed, for the many
// assertions whose subject is the entity state rather than the report of which
// fields were restored. Tests that assert the report call Rollback directly.
func (s *RevisionServiceIntegrationTestSuite) rollbackErr(revisionID, adminUserID uint) error {
	_, err := s.svc.Rollback(context.Background(), revisionID, adminUserID)
	return err
}

func TestRevisionServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(RevisionServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) createTestUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("rev-user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := s.db.Create(user).Error
	s.Require().NoError(err)
	return user
}

func (s *RevisionServiceIntegrationTestSuite) createTestVenue(name string) *catalogm.Venue {
	slug := fmt.Sprintf("test-venue-%d", time.Now().UnixNano())
	venue := &catalogm.Venue{
		Name:  name,
		Slug:  &slug,
		City:  "Phoenix",
		State: "AZ",
	}
	err := s.db.Create(venue).Error
	s.Require().NoError(err)
	return venue
}

// CBSA codes the offline geocoder resolves these two cities to. Named so the
// location-derivation tests read as one family.
const (
	metroPhoenix = "38060"
	metroNewYork = "35620"
)

// rollbackLatest undoes the most recent revision recorded against an entity —
// the find-the-revision-then-roll-it-back pair every rollback test repeats.
func (s *RevisionServiceIntegrationTestSuite) rollbackLatest(entityType string, entityID, adminID uint) {
	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Order("id DESC").First(&recorded).Error)
	s.Require().NoError(s.rollbackErr(recorded.ID, adminID))
}

// createTestArtist makes an artist already living in a metro, so a rollback that
// re-derives has something to be wrong about.
func (s *RevisionServiceIntegrationTestSuite) createTestArtist(name, city, state, metro string) *catalogm.Artist {
	slug := fmt.Sprintf("test-artist-%d", time.Now().UnixNano())
	artist := &catalogm.Artist{
		Name:  name,
		Slug:  &slug,
		City:  &city,
		State: &state,
		Metro: &metro,
	}
	s.Require().NoError(s.db.Create(artist).Error)
	return artist
}

func (s *RevisionServiceIntegrationTestSuite) createTestFestival(name, city, state, metro string) *catalogm.Festival {
	slug := fmt.Sprintf("test-fest-%d", time.Now().UnixNano())
	festival := &catalogm.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  slug,
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
		City:        &city,
		State:       &state,
		Metro:       &metro,
	}
	s.Require().NoError(s.db.Create(festival).Error)
	return festival
}

// =============================================================================
// RecordRevision tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_Success() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old Name", NewValue: "New Name"},
		{Field: "city", OldValue: "Phoenix", NewValue: "Tempe"},
	}

	err := s.svc.RecordRevision("venue", 42, user.ID, changes, "Updated venue info")
	s.NoError(err)

	// Verify the revision was created
	var revision adminm.Revision
	err = s.db.First(&revision).Error
	s.Require().NoError(err)
	s.Equal("venue", revision.EntityType)
	s.Equal(uint(42), revision.EntityID)
	s.Equal(user.ID, revision.UserID)
	s.Require().NotNil(revision.Summary)
	s.Equal("Updated venue info", *revision.Summary)
	s.Require().NotNil(revision.FieldChanges)

	// Verify field changes deserialization
	var parsedChanges []adminm.FieldChange
	err = json.Unmarshal(*revision.FieldChanges, &parsedChanges)
	s.NoError(err)
	s.Len(parsedChanges, 2)
	s.Equal("name", parsedChanges[0].Field)
	s.Equal("Old Name", parsedChanges[0].OldValue)
	s.Equal("New Name", parsedChanges[0].NewValue)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_EmptyChanges() {
	user := s.createTestUser()

	err := s.svc.RecordRevision("artist", 1, user.ID, []adminm.FieldChange{}, "no changes")
	s.NoError(err)

	// Verify no revision was created
	var count int64
	s.db.Model(&adminm.Revision{}).Count(&count)
	s.Equal(int64(0), count)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_NilChanges() {
	user := s.createTestUser()

	err := s.svc.RecordRevision("artist", 1, user.ID, nil, "no changes")
	s.NoError(err)

	var count int64
	s.db.Model(&adminm.Revision{}).Count(&count)
	s.Equal(int64(0), count)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_EmptySummary() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}

	err := s.svc.RecordRevision("artist", 1, user.ID, changes, "")
	s.NoError(err)

	var revision adminm.Revision
	err = s.db.First(&revision).Error
	s.Require().NoError(err)
	s.Nil(revision.Summary) // Empty summary stored as nil
}

// =============================================================================
// GetEntityHistory tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_Success() {
	user := s.createTestUser()

	// Create 3 revisions for the same entity
	for i := 0; i < 3; i++ {
		changes := []adminm.FieldChange{
			{Field: "name", OldValue: fmt.Sprintf("Name %d", i), NewValue: fmt.Sprintf("Name %d", i+1)},
		}
		err := s.svc.RecordRevision("artist", 10, user.ID, changes, fmt.Sprintf("Edit %d", i+1))
		s.Require().NoError(err)
	}

	revisions, total, err := s.svc.GetEntityHistory("artist", 10, 10, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(3), total)
	s.Len(revisions, 3)

	// Verify ordering (newest first)
	s.Require().NotNil(revisions[0].Summary)
	s.Equal("Edit 3", *revisions[0].Summary)
	s.Require().NotNil(revisions[2].Summary)
	s.Equal("Edit 1", *revisions[2].Summary)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_Pagination() {
	user := s.createTestUser()

	for i := 0; i < 5; i++ {
		changes := []adminm.FieldChange{
			{Field: "name", OldValue: "old", NewValue: "new"},
		}
		err := s.svc.RecordRevision("venue", 20, user.ID, changes, fmt.Sprintf("Edit %d", i+1))
		s.Require().NoError(err)
	}

	// Page 1
	revisions, total, err := s.svc.GetEntityHistory("venue", 20, 2, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(5), total)
	s.Len(revisions, 2)

	// Page 2
	revisions2, _, err := s.svc.GetEntityHistory("venue", 20, 2, 2, viewerPublic)
	s.NoError(err)
	s.Len(revisions2, 2)

	// No overlap
	s.NotEqual(revisions[0].ID, revisions2[0].ID)

	// Page 3
	revisions3, _, err := s.svc.GetEntityHistory("venue", 20, 2, 4, viewerPublic)
	s.NoError(err)
	s.Len(revisions3, 1)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_DefaultLimit() {
	revisions, total, err := s.svc.GetEntityHistory("artist", 999, 0, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(0), total)
	s.Empty(revisions)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_MaxLimit() {
	revisions, total, err := s.svc.GetEntityHistory("artist", 999, 200, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(0), total)
	s.Empty(revisions)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_FiltersByEntity() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	s.Require().NoError(s.svc.RecordRevision("artist", 1, user.ID, changes, "artist edit"))
	s.Require().NoError(s.svc.RecordRevision("venue", 1, user.ID, changes, "venue edit"))
	s.Require().NoError(s.svc.RecordRevision("artist", 2, user.ID, changes, "other artist edit"))

	revisions, total, err := s.svc.GetEntityHistory("artist", 1, 10, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(1), total)
	s.Len(revisions, 1)
	s.Require().NotNil(revisions[0].Summary)
	s.Equal("artist edit", *revisions[0].Summary)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_PreloadsUser() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	s.Require().NoError(s.svc.RecordRevision("artist", 1, user.ID, changes, "test"))

	revisions, _, err := s.svc.GetEntityHistory("artist", 1, 10, 0, viewerPublic)
	s.NoError(err)
	s.Require().Len(revisions, 1)
	s.Equal(user.ID, revisions[0].User.ID)
	s.Equal(*user.Email, *revisions[0].User.Email)
}

// =============================================================================
// GetRevision tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestGetRevision_Found() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}
	err := s.svc.RecordRevision("artist", 5, user.ID, changes, "test edit")
	s.Require().NoError(err)

	// Get the created revision's ID
	var created adminm.Revision
	s.db.First(&created)

	revision, err := s.svc.GetRevision(created.ID, viewerPublic)
	s.NoError(err)
	s.NotNil(revision)
	s.Equal(created.ID, revision.ID)
	s.Equal("artist", revision.EntityType)
	s.Equal(uint(5), revision.EntityID)
	s.Equal(user.ID, revision.User.ID)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetRevision_NotFound() {
	revision, err := s.svc.GetRevision(99999, viewerPublic)
	s.NoError(err)
	s.Nil(revision)
}

// =============================================================================
// GetUserRevisions tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_Success() {
	user1 := s.createTestUser()
	user2 := s.createTestUser()

	changes := []adminm.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}

	// User 1 makes 3 edits
	for i := 0; i < 3; i++ {
		s.Require().NoError(s.svc.RecordRevision("artist", uint(i+1), user1.ID, changes, fmt.Sprintf("user1 edit %d", i)))
	}

	// User 2 makes 1 edit
	s.Require().NoError(s.svc.RecordRevision("venue", 1, user2.ID, changes, "user2 edit"))

	// Get user1's revisions
	revisions, total, err := s.svc.GetUserRevisions(user1.ID, 10, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(3), total)
	s.Len(revisions, 3)

	// All revisions belong to user1
	for _, r := range revisions {
		s.Equal(user1.ID, r.UserID)
	}

	// Get user2's revisions
	revisions2, total2, err := s.svc.GetUserRevisions(user2.ID, 10, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(1), total2)
	s.Len(revisions2, 1)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_Pagination() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	for i := 0; i < 5; i++ {
		s.Require().NoError(s.svc.RecordRevision("artist", uint(i+1), user.ID, changes, ""))
	}

	revisions, total, err := s.svc.GetUserRevisions(user.ID, 2, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(5), total)
	s.Len(revisions, 2)

	revisions2, _, err := s.svc.GetUserRevisions(user.ID, 2, 2, viewerPublic)
	s.NoError(err)
	s.Len(revisions2, 2)

	// No overlap
	s.NotEqual(revisions[0].ID, revisions2[0].ID)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_Empty() {
	user := s.createTestUser()

	revisions, total, err := s.svc.GetUserRevisions(user.ID, 10, 0, viewerPublic)
	s.NoError(err)
	s.Equal(int64(0), total)
	s.Empty(revisions)
}

// =============================================================================
// Rollback tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestRollback_Success() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	venue := s.createTestVenue("Original Name")

	// Record a revision changing the venue name
	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Original Name", NewValue: "Changed Name"},
	}
	err := s.svc.RecordRevision("venue", venue.ID, user.ID, changes, "renamed venue")
	s.Require().NoError(err)

	// Apply the change to the venue (simulating what an edit handler would do)
	s.db.Table("venues").Where("id = ?", venue.ID).Updates(map[string]interface{}{
		"name": "Changed Name",
	})

	// Verify venue has changed name
	var updatedVenue catalogm.Venue
	s.db.First(&updatedVenue, venue.ID)
	s.Equal("Changed Name", updatedVenue.Name)

	// Get the revision to rollback
	var revision adminm.Revision
	s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).First(&revision)

	// Rollback
	err = s.rollbackErr(revision.ID, adminUser.ID)
	s.NoError(err)

	// Verify venue name is restored
	var restoredVenue catalogm.Venue
	s.db.First(&restoredVenue, venue.ID)
	s.Equal("Original Name", restoredVenue.Name)

	// Verify a rollback revision was created
	var allRevisions []adminm.Revision
	s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("created_at DESC").
		Find(&allRevisions)
	s.Len(allRevisions, 2) // original + rollback

	rollbackRevision := allRevisions[0]
	s.Equal(adminUser.ID, rollbackRevision.UserID)
	s.Require().NotNil(rollbackRevision.Summary)
	s.Contains(*rollbackRevision.Summary, "Rollback of revision #")

	// Verify rollback revision has inverted changes
	var rollbackChanges []adminm.FieldChange
	err = json.Unmarshal(*rollbackRevision.FieldChanges, &rollbackChanges)
	s.NoError(err)
	s.Len(rollbackChanges, 1)
	s.Equal("name", rollbackChanges[0].Field)
	s.Equal("Changed Name", rollbackChanges[0].OldValue)
	s.Equal("Original Name", rollbackChanges[0].NewValue)
}

// TestRollback_NullableTimestampBackToNull covers rolling back the first time a
// nullable timestamp column is populated, which is the only transition the show
// API can currently produce for doors_at/music_at.
//
// The regression this guards: revisiondiff used to encode "was unset" as "",
// and Rollback feeds recorded old values straight into a GORM update map, so
// the rollback died on `invalid input syntax for type timestamp with time zone:
// ""` and took every other field in the same revision down with it.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_NullableTimestampBackToNull() {
	user := s.createTestUser()
	adminUser := s.createTestUser()

	show := &catalogm.Show{
		Title:     "Rollback Times",
		EventDate: time.Now().UTC().AddDate(0, 0, 14),
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(show).Error)

	doors := show.EventDate.Add(-time.Hour)
	// Derive the changes through revisiondiff rather than hand-writing them, so
	// this test fails if the encoding of an unset timestamp regresses, not just
	// if Rollback mishandles a nil it was handed.
	changes := revisiondiff.Compare(
		&contracts.ShowResponse{Title: "Rollback Times"},
		&contracts.ShowResponse{Title: "Retitled", DoorsAt: &doors},
		revisiondiff.ShowFields,
	)
	s.Require().Len(changes, 2, "expected title + doors_at changes, got %v", changes)
	s.Require().NoError(s.svc.RecordRevision("show", show.ID, user.ID, changes, "set doors"))
	s.Require().NoError(s.db.Table("shows").Where("id = ?", show.ID).
		Updates(map[string]interface{}{"title": "Retitled", "doors_at": doors}).Error)

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "show", show.ID).
		First(&revision).Error)

	s.Require().NoError(s.rollbackErr(revision.ID, adminUser.ID),
		"rolling back a revision that first set a nullable timestamp must succeed")

	var restored catalogm.Show
	s.Require().NoError(s.db.First(&restored, show.ID).Error)
	s.Nil(restored.DoorsAt, "doors_at should be back to NULL")
	s.Equal("Rollback Times", restored.Title, "the bundled field must roll back too")
}

// TestRollback_NullablePriceBackToNull is the timestamp test's twin for a
// nullable NUMBER, and the difference between them is the whole point (PSY-1960).
//
// A "" written into a TIMESTAMPTZ blows up, so that defect announced itself. 0
// is perfectly writable to a numeric column, so this one did not: the rollback
// SUCCEEDED and restored a price nobody had entered. door_price makes it the
// common case rather than an edge one, because the column ships NULL on every
// existing row, so the first door-price edit on any show is this transition.
//
// It is a false PUBLIC claim, not just a wrong number: the ticket line renders
// 0 as "Free", so the undo publishes "DOOR Free" for a show whose door price was
// only ever unknown.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_NullablePriceBackToNull() {
	user := s.createTestUser()
	adminUser := s.createTestUser()

	show := &catalogm.Show{
		Title:     "Rollback Prices",
		EventDate: time.Now().UTC().AddDate(0, 0, 14),
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(show).Error)

	advance, door := 35.0, 40.0
	// Through revisiondiff, not hand-written changes: the defect was in what
	// Compare RECORDED, so a test that feeds Rollback a hand-written nil would
	// have passed throughout.
	changes := revisiondiff.Compare(
		&contracts.ShowResponse{Title: "Rollback Prices"},
		&contracts.ShowResponse{Title: "Rollback Prices", Price: &advance, DoorPrice: &door},
		revisiondiff.ShowFields,
	)
	s.Require().Len(changes, 2, "expected price + door_price changes, got %v", changes)
	s.Require().NoError(s.svc.RecordRevision("show", show.ID, user.ID, changes, "set prices"))
	s.Require().NoError(s.db.Table("shows").Where("id = ?", show.ID).
		Updates(map[string]interface{}{"price": advance, "door_price": door}).Error)

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "show", show.ID).
		First(&revision).Error)

	s.Require().NoError(s.rollbackErr(revision.ID, adminUser.ID))

	var restored catalogm.Show
	s.Require().NoError(s.db.First(&restored, show.ID).Error)
	s.Nil(restored.DoorPrice, "door_price must be back to NULL, not 0 (which renders as Free)")
	s.Nil(restored.Price, "price must be back to NULL, not 0")
}

// TestRollback_NullableCapacityBackToNull is the *int arm of the same fix, on a
// field that is REGISTERED in NumericEditFieldBounds. That registration is why
// it needs its own test: the nil recorded by revisiondiff passes through
// NarrowNumericUpdates, which must hand it on as a typed (*int)(nil) rather than
// narrowing it to 0 or dropping it.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_NullableCapacityBackToNull() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	venue := s.createTestVenue("Capacity Room")

	capacity := 250
	changes := revisiondiff.Compare(
		&contracts.VenueDetailResponse{Name: "Capacity Room"},
		&contracts.VenueDetailResponse{Name: "Capacity Room", Capacity: &capacity},
		revisiondiff.VenueFields,
	)
	s.Require().Len(changes, 1, "expected a capacity change, got %v", changes)
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, changes, "set capacity"))
	s.Require().NoError(s.db.Table("venues").Where("id = ?", venue.ID).
		Update("capacity", capacity).Error)

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		First(&revision).Error)

	s.Require().NoError(s.rollbackErr(revision.ID, adminUser.ID))

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Nil(restored.Capacity, "capacity must be back to NULL, not 0")
}

func (s *RevisionServiceIntegrationTestSuite) TestRollback_RevisionNotFound() {
	err := s.rollbackErr(99999, 1)
	s.Error(err)
	s.Contains(err.Error(), "revision not found")
}

func (s *RevisionServiceIntegrationTestSuite) TestRollback_EntityNotFound() {
	user := s.createTestUser()

	// Record a revision for an entity that doesn't exist
	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}
	err := s.svc.RecordRevision("venue", 99999, user.ID, changes, "test")
	s.Require().NoError(err)

	var revision adminm.Revision
	s.db.First(&revision)

	err = s.rollbackErr(revision.ID, user.ID)
	s.Error(err)
	s.Contains(err.Error(), "entity not found")
}

// Rollback replays revisions.field_changes through an untyped Updates(), so a
// stored number arrives as float64 exactly as it does on the approve path.
// Without narrowing the driver takes that for an integer column and truncates
// it, so an undo would restore a value that is not the one being undone.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_NarrowsNumericValues() {
	user := s.createTestUser()
	venue := s.createTestVenue("Rollback Capacity Venue")

	const original = 550
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("capacity", original).Error)

	// A revision that moved capacity 550 -> 3600, as the approve path records it.
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID,
		[]adminm.FieldChange{{Field: "capacity", OldValue: original, NewValue: 3600}},
		"counted the room"))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("capacity", 3600).Error)

	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&recorded).Error)

	s.Require().NoError(s.rollbackErr(recorded.ID, user.ID))

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Capacity)
	s.Equal(original, *restored.Capacity, "rollback must restore the exact prior capacity")
}

// An API-only bound still does not block an undo: a year from before that bound
// is exactly the value a rollback exists to restore.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresOutOfRangeHistoricalValue() {
	user := s.createTestUser()
	venue := s.createTestVenue("Legacy Year Venue")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID,
		[]adminm.FieldChange{{Field: "name", OldValue: "Legacy Year Venue", NewValue: "Renamed Venue"}},
		"renamed"))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("name", "Renamed Venue").Error)

	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&recorded).Error)

	s.Require().NoError(s.rollbackErr(recorded.ID, user.ID),
		"undo must not be blocked by a bound the historical value predates")

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Equal("Legacy Year Venue", restored.Name)
}

// A capacity the COLUMN refuses is skipped like any other refused field, and its
// honest siblings still restore.
//
// This is the whole reason the column bound is a per-field gate rather than a
// write that fails: venues_capacity_range would abort the UPDATE carrying every
// field, so one legacy capacity would deny the undo of a name recorded beside
// it, which is the failure the per-field rollback exists to remove.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_ColumnBoundedCapacityIsSkippedNotFatal() {
	user := s.createTestUser()
	venue := s.createTestVenue("Legacy Capacity Venue")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID,
		[]adminm.FieldChange{
			{Field: "capacity", OldValue: 0, NewValue: 550},
			{Field: "name", OldValue: "Legacy Capacity Venue", NewValue: "Counted Room"},
		},
		"counted the room and renamed it"))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Updates(map[string]interface{}{"capacity": 550, "name": "Counted Room"}).Error)

	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&recorded).Error)

	result, err := s.svc.Rollback(context.Background(), recorded.ID, user.ID)
	s.Require().NoError(err, "one refused field must not deny the undo of the others")
	s.Require().NotNil(result)

	s.Equal([]string{"name"}, result.AppliedFields)
	s.Require().Len(result.SkippedFields, 1)
	s.Equal("capacity", result.SkippedFields[0].Field)
	s.Contains(result.SkippedFields[0].Reason, "between 1 and 200000")

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Equal("Legacy Capacity Venue", restored.Name, "the honest field restored")
	s.Require().NotNil(restored.Capacity)
	s.Equal(550, *restored.Capacity, "the refused field was left as it was")
}

// A revision whose ONLY field the column refuses restores nothing, and says so
// rather than reporting an undo that did not happen.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_CapacityOnlyRevisionRestoresNothing() {
	user := s.createTestUser()
	venue := s.createTestVenue("Capacity Only Venue")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID,
		[]adminm.FieldChange{{Field: "capacity", OldValue: 0, NewValue: 550}},
		"replaced a legacy zero"))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("capacity", 550).Error)

	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&recorded).Error)

	_, err := s.svc.Rollback(context.Background(), recorded.ID, user.ID)
	s.Require().Error(err)
	s.Contains(err.Error(), "no field of this revision can be restored")
	s.Contains(err.Error(), "capacity")

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Capacity)
	s.Equal(550, *restored.Capacity, "a refused rollback leaves the entity untouched")
}

// A rollback that restores a venue's city/state must RE-DERIVE the columns the
// system derives from that location, or the venue lands back in its old city
// still carrying the timezone resolved for the city it was moved away from.
// Why that is harmful: see applyDerivedVenueLocation.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RederivesVenueTimezoneOnLocationRevert() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	venue := s.createTestVenue("Relocated Room") // Phoenix, AZ

	// The state AFTER the edit being rolled back: moved to New York with the
	// derived columns resolved for New York, as a venue write path leaves them.
	s.Require().NoError(s.db.Table("venues").Where("id = ?", venue.ID).
		Updates(map[string]interface{}{
			"city":      "New York",
			"state":     "NY",
			"timezone":  "America/New_York",
			"latitude":  40.714300,
			"longitude": -74.006000,
		}).Error)

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID,
		[]adminm.FieldChange{
			{Field: "city", OldValue: "Phoenix", NewValue: "New York"},
			{Field: "state", OldValue: "AZ", NewValue: "NY"},
		},
		"moved the venue"))

	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&recorded).Error)

	s.Require().NoError(s.rollbackErr(recorded.ID, adminUser.ID))

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Equal("Phoenix", restored.City)
	s.Equal("AZ", restored.State)

	// THE assertion: the restored city no longer carries the other city's zone.
	s.Require().NotNil(restored.Timezone, "rollback must derive a zone for the restored city")
	s.Equal("America/Phoenix", *restored.Timezone,
		"the restored city must not keep the timezone derived for the city it was moved away from")

	// Coordinates and metro come out of the same lookup and go stale the same way.
	s.Require().NotNil(restored.Latitude)
	s.InDelta(33.45, *restored.Latitude, 0.5, "latitude must be re-derived for Phoenix")
	s.Require().NotNil(restored.Longitude)
	s.InDelta(-112.07, *restored.Longitude, 0.5, "longitude must be re-derived for Phoenix")
	s.Require().NotNil(restored.Metro)
	s.Equal("38060", *restored.Metro, "metro must be re-derived to the Phoenix CBSA")
}

// The re-derivation is scoped to location reverts: a rollback that touches no
// location field must leave the derived columns exactly as they are, or every
// undo of an unrelated field would silently re-geocode the venue.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_LeavesDerivedColumnsAloneWithoutLocationChange() {
	user := s.createTestUser()
	venue := s.createTestVenue("Untouched Location Room")

	// A zone that does NOT match the venue's city, so a stray re-derivation would
	// visibly overwrite it.
	s.Require().NoError(s.db.Table("venues").Where("id = ?", venue.ID).
		Updates(map[string]interface{}{"timezone": "America/New_York"}).Error)

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID,
		[]adminm.FieldChange{{Field: "name", OldValue: "Untouched Location Room", NewValue: "Renamed"}},
		"renamed"))
	s.Require().NoError(s.db.Table("venues").Where("id = ?", venue.ID).
		Updates(map[string]interface{}{"name": "Renamed"}).Error)

	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&recorded).Error)
	s.Require().NoError(s.rollbackErr(recorded.ID, user.ID))

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Equal("Untouched Location Room", restored.Name)
	s.Require().NotNil(restored.Timezone)
	s.Equal("America/New_York", *restored.Timezone,
		"a non-location rollback must not re-derive the timezone")
}

// An artist's metro is derived from its (city, state, country) exactly the way a
// venue's timezone is, so a rollback that restores the old city must re-derive
// it. Why the stale pairing is harmful: see applyDerivedEntityMetro.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RederivesArtistMetroOnLocationRevert() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	// The state AFTER the edit being rolled back: moved to New York with the
	// metro resolved for New York, as an artist write path leaves it.
	artist := s.createTestArtist("Relocated Band", "New York", "NY", metroNewYork)

	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, user.ID,
		[]adminm.FieldChange{
			{Field: "city", OldValue: "Phoenix", NewValue: "New York"},
			{Field: "state", OldValue: "AZ", NewValue: "NY"},
		},
		"moved the artist"))

	s.rollbackLatest("artist", artist.ID, adminUser.ID)

	var restored catalogm.Artist
	s.Require().NoError(s.db.First(&restored, artist.ID).Error)
	s.Require().NotNil(restored.City)
	s.Equal("Phoenix", *restored.City)

	// THE assertion: the restored city no longer carries the other city's metro.
	s.Require().NotNil(restored.Metro, "rollback must derive a metro for the restored city")
	s.Equal(metroPhoenix, *restored.Metro,
		"the restored city must not keep the metro derived for the city it was moved away from")
}

// Festivals carry the same derived metro (PSY-1278) through the same rollback
// path, so the artist fix has to cover them too — a metro-keyed festival_count
// reads a festival into the scene its stale code points at.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RederivesFestivalMetroOnLocationRevert() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	festival := s.createTestFestival("Relocated Fest", "New York", "NY", metroNewYork)

	s.Require().NoError(s.svc.RecordRevision("festival", festival.ID, user.ID,
		[]adminm.FieldChange{
			{Field: "city", OldValue: "Phoenix", NewValue: "New York"},
			{Field: "state", OldValue: "AZ", NewValue: "NY"},
		},
		"moved the festival"))

	s.rollbackLatest("festival", festival.ID, adminUser.ID)

	var restored catalogm.Festival
	s.Require().NoError(s.db.First(&restored, festival.ID).Error)
	s.Require().NotNil(restored.City)
	s.Equal("Phoenix", *restored.City)
	s.Require().NotNil(restored.Metro, "rollback must derive a metro for the restored city")
	s.Equal(metroPhoenix, *restored.Metro,
		"the restored city must not keep the metro derived for the city it was moved away from")
}

// Artists and festivals have NULLABLE location columns, so the common rollback
// is undoing "someone added a city" — the restored value is SQL NULL. The
// re-derivation has to read that as "no location" and clear the metro; treating
// an absent value as "keep the current one" would derive from the very city the
// same write erases, which is the stale pairing in its purest form.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_ClearsArtistMetroWhenLocationRevertsToNull() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	artist := s.createTestArtist("Newly Placed Band", "Phoenix", "AZ", metroPhoenix)

	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, user.ID,
		[]adminm.FieldChange{
			{Field: "city", OldValue: nil, NewValue: "Phoenix"},
			{Field: "state", OldValue: nil, NewValue: "AZ"},
		},
		"gave the artist a hometown"))

	s.rollbackLatest("artist", artist.ID, adminUser.ID)

	var restored catalogm.Artist
	s.Require().NoError(s.db.First(&restored, artist.ID).Error)
	s.Nil(restored.City, "the rollback restores the NULL city")
	s.Nil(restored.Metro,
		"an artist with no city must not keep the metro derived for the city that was removed")
}

// The re-derivation is scoped to location reverts: undoing an unrelated field
// must leave metro exactly as it is, or every rollback would silently re-key the
// entity's scene membership off whatever the geocoder says today.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_LeavesArtistMetroAloneWithoutLocationChange() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	// A metro that does NOT match the city, so a stray re-derivation would
	// visibly overwrite it.
	artist := s.createTestArtist("Renamed Band", "Phoenix", "AZ", metroNewYork)

	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, user.ID,
		[]adminm.FieldChange{{Field: "name", OldValue: "Original Band", NewValue: "Renamed Band"}},
		"renamed"))

	s.rollbackLatest("artist", artist.ID, adminUser.ID)

	var restored catalogm.Artist
	s.Require().NoError(s.db.First(&restored, artist.ID).Error)
	s.Equal("Original Band", restored.Name)
	s.Require().NotNil(restored.Metro)
	s.Equal(metroNewYork, *restored.Metro, "a non-location rollback must not re-derive the metro")
}

// =============================================================================
// Read-time privacy redaction
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) createVerifiedTestVenue(name string) *catalogm.Venue {
	venue := s.createTestVenue(name)
	// Verified is set with an explicit UPDATE rather than on Create so both
	// helpers share one path and neither can drift onto GORM's
	// zero-value-vs-column-default behavior for bools.
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("verified", true).Error)
	venue.Verified = true
	return venue
}

// secretAddress is the value the whole gate exists to keep out of a response.
// Single-sourced so a fixture and its assertions cannot drift onto different
// strings and pass vacuously.
const secretAddress = "1234 Secret St"

// addressChanges is the diff an approved contributor address edit records.
func addressChanges() []adminm.FieldChange {
	return []adminm.FieldChange{
		{Field: "name", OldValue: "Old Room", NewValue: "The Basement"},
		{Field: "address", OldValue: "1 Old St", NewValue: secretAddress},
		{Field: "zipcode", OldValue: "85003", NewValue: "85004"},
	}
}

// nameOnlyChanges is a diff carrying nothing private: a rename, which is the
// case that proves the summary gate keys off the SUBJECT rather than the diff.
//
// Spelled out rather than sliced off addressChanges, which would silently start
// including the address if that fixture's field order ever changed.
func nameOnlyChanges() []adminm.FieldChange {
	return []adminm.FieldChange{
		{Field: "name", OldValue: "Old Room", NewValue: "The Basement"},
	}
}

// changesFor unmarshals a served revision's field_changes into a field->change
// map for assertions.
func (s *RevisionServiceIntegrationTestSuite) changesFor(r adminm.Revision) map[string]adminm.FieldChange {
	s.Require().NotNil(r.FieldChanges)
	var parsed []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*r.FieldChanges, &parsed))
	byField := make(map[string]adminm.FieldChange, len(parsed))
	for _, c := range parsed {
		byField[c.Field] = c
	}
	return byField
}

// leakySummary is the summary a contributor naturally writes for the exact edit
// addressChanges models: prose that republishes the value the diff beside it
// masks. Every fixture records it, so the whole-payload assertions have
// something to bite on in the prose slot as well as in the diff.
const leakySummary = "corrected the address to " + secretAddress

// assertSummaryNotServed is the prose half of the gate, split out so the case
// whose diff carries nothing private can assert it without also asserting a
// mask. Prose is withheld whole rather than masked, so there is no sentinel to
// look for, only absence. Checked against the whole marshalled revision so a
// future writer that copies the summary under another key still fails.
func (s *RevisionServiceIntegrationTestSuite) assertSummaryNotServed(r adminm.Revision) {
	s.Nil(r.Summary, "a gated venue's summary must not be served")
	served, err := json.Marshal(r)
	s.Require().NoError(err)
	s.NotContains(string(served), secretAddress, "no field of the response may carry the address")
}

// assertGatedVenueRevision pins the actual acceptance criterion first (neither
// the old nor the new street address appears ANYWHERE in the served revision,
// through the diff OR through the summary), and only then the shape of the mask.
// A field-by-field check alone would pass if a future writer duplicated the
// value under another key.
//
// It covers prose as well as fields because both ride the same verdict: whatever
// routes a row to redactVenueRevision, the venue lookup or the merge marker,
// masks the diff and drops the summary together. Naming it for the verdict
// rather than for "address" is what keeps that true, since a helper called
// assertAddressMasked would read as unrelated to a summary leak.
func (s *RevisionServiceIntegrationTestSuite) assertGatedVenueRevision(r adminm.Revision) {
	s.Require().NotNil(r.FieldChanges)
	s.assertSummaryNotServed(r)

	raw := string(*r.FieldChanges)
	s.NotContains(raw, "1 Old St", "the old address must not be served either")
	s.NotContains(raw, "85004")
	s.NotContains(raw, "85003")

	byField := s.changesFor(r)
	s.Equal(revisiondiff.RedactedValue, byField["address"].OldValue)
	s.Equal(revisiondiff.RedactedValue, byField["address"].NewValue)
	s.Equal(revisiondiff.RedactedValue, byField["zipcode"].OldValue)
	s.Equal(revisiondiff.RedactedValue, byField["zipcode"].NewValue)

	// The edit itself stays visible — this is redaction, not deletion.
	s.Len(byField, 3)
	s.Equal("Old Room", byField["name"].OldValue)
	s.Equal("The Basement", byField["name"].NewValue)
}

// assertPublishableVenueRevision is the positive counterpart. It exists as a
// helper for the same reason assertGatedVenueRevision does: spelled inline, the
// four sites that need it had already drifted to checking different subsets, so
// a regression masking only (say) zipcode's old value would have passed all of
// them.
//
// It asserts the summary is PRESENT for the same reason its counterpart asserts
// absence. Without this, a gate that withheld every summary unconditionally
// would pass the whole suite, and the withholding would have quietly become
// "revision history has no prose" instead of "gated venues have no prose".
//
// Presence, not a specific string: callers record whatever prose labels their
// fixture, and pinning one value here would force every positive site onto a
// shared literal for no gain.
func (s *RevisionServiceIntegrationTestSuite) assertPublishableVenueRevision(r adminm.Revision) {
	byField := s.changesFor(r)
	s.Equal("1 Old St", byField["address"].OldValue)
	s.Equal(secretAddress, byField["address"].NewValue)
	s.Equal("85003", byField["zipcode"].OldValue)
	s.Equal("85004", byField["zipcode"].NewValue)

	s.Require().NotNil(r.Summary, "a verified venue's summary is still published")
	s.NotEmpty(*r.Summary)
}

// byEntity indexes a page of revisions by the entity they point at, for the
// tests that put two venues with opposite verdicts on one page.
func (s *RevisionServiceIntegrationTestSuite) byEntity(revisions []adminm.Revision) map[uint]adminm.Revision {
	out := make(map[uint]adminm.Revision, len(revisions))
	for _, r := range revisions {
		out[r.EntityID] = r
	}
	return out
}

// markFromUnverifiedVenue stamps stored revisions the way a venue merge does,
// for the tests that exercise the read gate without running a merge.
func (s *RevisionServiceIntegrationTestSuite) markFromUnverifiedVenue(revisionID uint) {
	s.Require().NoError(s.db.Model(&adminm.Revision{}).
		Where("id = ?", revisionID).
		Update("from_unverified_venue", true).Error)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_RedactsUnverifiedVenueAddress() {
	user := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")
	s.Require().False(venue.Verified)

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))

	revisions, total, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(revisions, 1)
	s.assertGatedVenueRevision(revisions[0])

	// Redaction is a serving concern only. The stored row must still hold the
	// real values, diff AND prose: that is what rollback reads, what a later
	// verification restores, and what any future, finer policy would re-read.
	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)
	s.Contains(string(*stored.FieldChanges), secretAddress,
		"the stored row must not be rewritten by a read")
	s.Require().NotNil(stored.Summary)
	s.Equal(leakySummary, *stored.Summary, "the stored prose must not be rewritten by a read")
}

// The DB-error half of fail-closed. An aborted transaction makes the
// verified-venue lookup fail for real, which must mask rather than publish.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_LookupErrorFailsClosed() {
	user := s.createTestUser()
	venue := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))

	tx := s.db.Begin()
	defer func() { _ = tx.Rollback() }()
	// Poison the transaction: every later statement on it errors until rollback.
	s.Require().Error(tx.Exec("SELECT 1/0").Error)

	revisions, _, err := NewRevisionService(tx).GetEntityHistory("venue", venue.ID, 10, 0, viewerPublic)
	s.Require().Error(err, "the history query itself fails on a poisoned tx")
	s.Empty(revisions)

	// The lookup alone, on the same poisoned tx: a VERIFIED venue must still
	// come back unverified, because the gate cannot prove otherwise.
	verified := NewRevisionService(tx).verifiedVenueIDs([]uint{venue.ID})
	s.Empty(verified, "a lookup error must not admit a venue to the verified set")
}

// nil db is the other fail-closed input: no lookup is possible, so nothing is
// verified and every venue revision masks.
func (s *RevisionServiceIntegrationTestSuite) TestApplyPrivacyRedaction_NilDBFailsClosed() {
	raw := json.RawMessage(`[{"field":"address","old_value":"1 Old St","new_value":"1234 Secret St"}]`)
	revisions := []adminm.Revision{{ID: 1, EntityType: "venue", EntityID: 7, FieldChanges: &raw}}

	(&RevisionService{db: nil}).applyPrivacyRedaction(revisions, viewerPublic.IsAdmin)

	s.NotContains(string(*revisions[0].FieldChanges), secretAddress)
	s.NotContains(string(*revisions[0].FieldChanges), "1 Old St")
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_ServesVerifiedVenueAddress() {
	user := s.createTestUser()
	venue := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))

	revisions, _, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)
	s.assertPublishableVenueRevision(revisions[0])
}

// A revision pointing at a venue row that no longer exists must mask, not
// publish: the gate has no evidence that venue was ever verified.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_MissingVenueFailsClosed() {
	user := s.createTestUser()

	s.Require().NoError(s.svc.RecordRevision("venue", 987654, user.ID, addressChanges(), leakySummary))

	revisions, _, err := s.svc.GetEntityHistory("venue", 987654, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)
	s.assertGatedVenueRevision(revisions[0])
}

// Non-venue entities carry no private field, so their diffs must come back
// exactly as stored — the gate must not quietly reshape unrelated history.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_LeavesNonVenueEntitiesUntouched() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{
		{Field: "city", OldValue: "Phoenix", NewValue: "Tempe"},
		{Field: "description", OldValue: "", NewValue: "a band"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", 31, user.ID, changes, leakySummary))

	var stored adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ?", "artist").First(&stored).Error)

	revisions, _, err := s.svc.GetEntityHistory("artist", 31, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)
	s.JSONEq(string(*stored.FieldChanges), string(*revisions[0].FieldChanges))
	s.Require().NotNil(revisions[0].Summary)
	s.Equal(leakySummary, *revisions[0].Summary)
}

// =============================================================================
// Summary withholding: the two cases no address fixture can produce
// =============================================================================
//
// Every gated-venue test in this file records leakySummary and asserts through
// assertGatedVenueRevision, so all three read paths and the merge boundary
// already cover the prose. These two exist because no address-masking fixture
// produces their input.

// The gate keys off the SUBJECT, not the diff. A rename whose summary mentions
// the street is exactly the case a field-name rule cannot reach, and the case
// that would be silently uncovered if the withholding were ever moved inside the
// private-field loop.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_WithholdsSummaryWhenDiffTouchesNoPrivateField() {
	user := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, nameOnlyChanges(), leakySummary))

	revisions, _, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)

	s.assertSummaryNotServed(revisions[0])

	// The diff itself is untouched: nothing in it was private.
	byField := s.changesFor(revisions[0])
	s.Len(byField, 1)
	s.Equal("The Basement", byField["name"].NewValue)
}

// A revision with no readable diff still carries prose, and that is the one case
// where the summary is the ONLY thing being served, so the withholding must not
// sit behind the FieldChanges nil check.
func (s *RevisionServiceIntegrationTestSuite) TestApplyPrivacyRedaction_WithholdsSummaryWithNilFieldChanges() {
	summary := leakySummary
	revisions := []adminm.Revision{
		{ID: 1, EntityType: "venue", EntityID: 7, FieldChanges: nil, Summary: &summary},
	}

	(&RevisionService{db: nil}).applyPrivacyRedaction(revisions, viewerPublic.IsAdmin)

	s.Nil(revisions[0].Summary)
	s.Equal(leakySummary, summary, "the pointed-to string must not be overwritten in place")
}

func (s *RevisionServiceIntegrationTestSuite) TestGetRevision_RedactsUnverifiedVenueAddress() {
	user := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))

	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)

	revision, err := s.svc.GetRevision(stored.ID, viewerPublic)
	s.Require().NoError(err)
	s.Require().NotNil(revision)
	s.assertGatedVenueRevision(*revision)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_RedactsUnverifiedVenueAddress() {
	user := s.createTestUser()
	unverified := s.createTestVenue("Unverified Room")
	verified := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", unverified.ID, user.ID, addressChanges(), leakySummary))
	s.Require().NoError(s.svc.RecordRevision("venue", verified.ID, user.ID, addressChanges(), leakySummary))

	revisions, _, err := s.svc.GetUserRevisions(user.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(revisions, 2)

	// One page, two venues, opposite verdicts — proves the batched lookup keys
	// each revision to its OWN venue rather than to the page as a whole.
	byEntity := s.byEntity(revisions)
	s.assertGatedVenueRevision(byEntity[unverified.ID])
	s.assertPublishableVenueRevision(byEntity[verified.ID])
}

// The stored row is never touched, so it is still the truth an admin rollback
// restores. If Rollback read the served copy it would write "(hidden)" into
// venues.address.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresRealAddressForUnverifiedVenue() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("address", secretAddress).Error)

	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)

	s.Require().NoError(s.rollbackErr(stored.ID, adminUser.ID))

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Address)
	s.Equal("1 Old St", *restored.Address, "rollback must restore the real address, not the mask")
}

// =============================================================================
// THE VENUE-MERGE BOUNDARY
// =============================================================================
//
// The first two tests run a REAL venue merge rather than setting the column by
// hand, because the bug was never in either half on its own: the merge
// re-points revisions and deletes the venue the read gate decides from, so the
// two halves only disagree when they meet. The rest stamp the column directly,
// which is enough to pin the gate's own contract.

// TestGetEntityHistory_MergedUnverifiedVenueStaysRedacted is the leak.
// Before the merge the loser is unverified and its address history is masked.
// After it, the same row hangs off a VERIFIED venue and the loser row is gone,
// so a gate reading venues.verified alone publishes it.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_MergedUnverifiedVenueStaysRedacted() {
	user := s.createTestUser()
	canonical := s.createVerifiedTestVenue("Verified Room")
	loser := s.createTestVenue("Somebodys House")
	s.Require().False(loser.Verified)

	s.Require().NoError(s.svc.RecordRevision("venue", loser.ID, user.ID, addressChanges(), leakySummary))

	before, _, err := s.svc.GetEntityHistory("venue", loser.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(before, 1)
	s.assertGatedVenueRevision(before[0])

	_, err = catalog.NewVenueService(s.db).MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	var moved adminm.Revision
	s.Require().NoError(s.db.First(&moved).Error)
	s.Require().Equal(canonical.ID, moved.EntityID, "precondition: the revision was re-pointed")
	s.Require().True(moved.FromUnverifiedVenue, "precondition: the merge marked it")

	after, _, err := s.svc.GetEntityHistory("venue", canonical.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(after, 1)
	s.assertGatedVenueRevision(after[0])

	// GetRevision too, not because the routes are separately wired (three
	// pre-existing tests cover that) but because it redacts a COPY that still
	// shares its FieldChanges pointer with the stored row.
	single, err := s.svc.GetRevision(moved.ID, viewerPublic)
	s.Require().NoError(err)
	s.Require().NotNil(single)
	s.assertGatedVenueRevision(*single)

	// Provenance, not a scrub: the stored diff still holds the real address, so
	// rollback and the moderation surfaces are unaffected.
	s.Contains(string(*moved.FieldChanges), secretAddress,
		"the merge must not rewrite stored history")
}

// The other direction must be untouched. A verified venue's address history is
// publishable, and merging two verified rooms may not start withholding it.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_MergedVerifiedVenueStillServesAddress() {
	user := s.createTestUser()
	canonical := s.createVerifiedTestVenue("Canonical Room")
	loser := s.createVerifiedTestVenue("Duplicate Room")

	s.Require().NoError(s.svc.RecordRevision("venue", loser.ID, user.ID, addressChanges(), leakySummary))

	_, err := catalog.NewVenueService(s.db).MergeVenues(canonical.ID, loser.ID, 0)
	s.Require().NoError(err)

	var moved adminm.Revision
	s.Require().NoError(s.db.First(&moved).Error)
	s.Require().Equal(canonical.ID, moved.EntityID)
	s.False(moved.FromUnverifiedVenue)

	after, _, err := s.svc.GetEntityHistory("venue", canonical.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(after, 1)
	s.assertPublishableVenueRevision(after[0])
}

// The gate's half of the contract, stated without the merge: a marked row masks
// even though the venue it points at is verified. This is what stops a later
// reader of applyPrivacyRedaction from "simplifying" the marker away on the
// grounds that the venue lookup already answers the question.
func (s *RevisionServiceIntegrationTestSuite) TestApplyPrivacyRedaction_MarkedRowMasksEvenOnVerifiedVenue() {
	user := s.createTestUser()
	venue := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))
	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)
	s.markFromUnverifiedVenue(stored.ID)

	revisions, _, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)
	s.assertGatedVenueRevision(revisions[0])
}

// The post-merge shape, which is the one a per-VENUE implementation of the
// marker would get wrong: after a merge the canonical venue's own history and
// the loser's marked rows share an entity_id on the same page. Deciding by
// entity_id rather than per row would mask the canonical venue's own
// publishable history, trading the leak for silently unreadable history.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_MarkerIsPerRowNotPerVenue() {
	user := s.createTestUser()
	venue := s.createVerifiedTestVenue("Verified Room")

	// The carried-in row's summary is the leaky one, so the assertion that the
	// marked row publishes no address has teeth in the prose slot too, not just
	// in the diff. Its neighbour keeps a bland label to stay distinguishable.
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), "its own"))

	var stored []adminm.Revision
	s.Require().NoError(s.db.Order("id ASC").Find(&stored).Error)
	s.Require().Len(stored, 2)
	s.markFromUnverifiedVenue(stored[0].ID)

	revisions, _, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(revisions, 2)

	byID := map[uint]adminm.Revision{}
	for _, r := range revisions {
		byID[r.ID] = r
	}
	s.assertGatedVenueRevision(byID[stored[0].ID])
	s.assertPublishableVenueRevision(byID[stored[1].ID])
}

// =============================================================================
// THE ADMIN TIER (PSY-1717)
// =============================================================================
//
// PSY-1700 masked unverified-venue history for every caller, matching the
// tier-less live payload gate. That left an admin reading `address: (hidden)`
// in the History panel while the Rollback button beside it restored the real
// stored value — the moderation UI hiding what the moderation action used.
//
// Each test below is the exact counterpart of a viewerPublic test above that
// must keep failing closed, so the pair states the divergence rather than
// leaving it to a comment.
//
// They assert through assertPublishableVenueRevision, the same helper the
// verified-venue cases use, which pins the property precisely: an admin's view
// of a gated venue is the view everyone gets of a verified one. The tier reveals
// nothing beyond what verification would, and withholds nothing from an admin.
//
// The third tier of the acceptance criterion — an authenticated NON-admin is
// treated as the public — cannot be asserted here. This service sits below the
// auth boundary and sees one bool; which callers resolve to it is pinned in
// handlers/admin/revision_test.go and end-to-end over real JWTs in
// routes/revision_viewer_tier_test.go.

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_ServesUnverifiedVenueAddressToAdmin() {
	user := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")
	s.Require().False(venue.Verified)

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))

	revisions, total, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0, viewerAdmin)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(revisions, 1)
	s.assertPublishableVenueRevision(revisions[0])
	s.Equal(leakySummary, *revisions[0].Summary,
		"the prose rides the same verdict, so the admin tier restores it too")
}

func (s *RevisionServiceIntegrationTestSuite) TestGetRevision_ServesUnverifiedVenueAddressToAdmin() {
	user := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))

	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)

	revision, err := s.svc.GetRevision(stored.ID, viewerAdmin)
	s.Require().NoError(err)
	s.Require().NotNil(revision)
	s.assertPublishableVenueRevision(*revision)
}

// The tier is per CALLER, not per author: the page belongs to the contributor
// who made the edits, and it is the admin READING it who gets the unmasked view.
func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_ServesUnverifiedVenueAddressToAdmin() {
	user := s.createTestUser()
	unverified := s.createTestVenue("Unverified Room")
	verified := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", unverified.ID, user.ID, addressChanges(), leakySummary))
	s.Require().NoError(s.svc.RecordRevision("venue", verified.ID, user.ID, addressChanges(), leakySummary))

	revisions, _, err := s.svc.GetUserRevisions(user.ID, 10, 0, viewerAdmin)
	s.Require().NoError(err)
	s.Require().Len(revisions, 2)

	// Both rows publish for an admin, where viewerPublic splits this same page
	// down the middle (TestGetUserRevisions_RedactsUnverifiedVenueAddress).
	byEntity := s.byEntity(revisions)
	s.assertPublishableVenueRevision(byEntity[unverified.ID])
	s.assertPublishableVenueRevision(byEntity[verified.ID])
}

// The merge marker rides the same verdict as the venue lookup, so the admin tier
// has to clear it too. Otherwise a merged-away DIY room's history stays masked
// for the one caller who can roll it back — the original defect surviving on
// exactly the rows a merge makes hardest to reason about.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_AdminSeesMergeStampedRow() {
	user := s.createTestUser()
	venue := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), leakySummary))

	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)
	s.markFromUnverifiedVenue(stored.ID)

	// Same row, both tiers, so the marker is proved live rather than assumed.
	masked, _, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0, viewerPublic)
	s.Require().NoError(err)
	s.Require().Len(masked, 1)
	s.assertGatedVenueRevision(masked[0])

	revisions, _, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0, viewerAdmin)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)
	s.assertPublishableVenueRevision(revisions[0])
}

// The admin tier must not depend on the verified-venue lookup, which fails
// CLOSED. Applied after it, a nil db or a poisoned transaction would mask an
// admin into the view the Rollback button contradicts — the exact defect
// PSY-1717 fixes, reappearing only when the database is already unhappy. A nil
// db is the strongest available statement of "no lookup happened".
func (s *RevisionServiceIntegrationTestSuite) TestApplyPrivacyRedaction_AdminTierDoesNotNeedTheLookup() {
	raw := json.RawMessage(`[{"field":"address","old_value":"1 Old St","new_value":"1234 Secret St"}]`)
	summary := leakySummary
	revisions := []adminm.Revision{
		{ID: 1, EntityType: "venue", EntityID: 7, FieldChanges: &raw, Summary: &summary},
	}

	(&RevisionService{db: nil}).applyPrivacyRedaction(revisions, viewerAdmin.IsAdmin)

	s.Contains(string(*revisions[0].FieldChanges), secretAddress)
	s.Require().NotNil(revisions[0].Summary)
	s.Equal(leakySummary, *revisions[0].Summary)
}
