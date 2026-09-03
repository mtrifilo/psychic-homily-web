package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestIsValidPendingEditEntityType(t *testing.T) {
	assert.True(t, adminm.IsValidPendingEditEntityType("artist"))
	assert.True(t, adminm.IsValidPendingEditEntityType("venue"))
	assert.True(t, adminm.IsValidPendingEditEntityType("festival"))
	assert.True(t, adminm.IsValidPendingEditEntityType("release"))
	assert.True(t, adminm.IsValidPendingEditEntityType("label"))
	assert.False(t, adminm.IsValidPendingEditEntityType("show"))
	assert.False(t, adminm.IsValidPendingEditEntityType(""))
	assert.False(t, adminm.IsValidPendingEditEntityType("comment"))
}

// NarrowNumericUpdates is the last gate before an untyped Updates() writes a
// stored JSON number into an integer column, on BOTH the approve path and the
// rollback path, so it is unit-tested independently of the database-backed
// flows below.
func TestNarrowNumericUpdates(t *testing.T) {
	// Round-tripping through JSON is the real shape: field_changes is JSONB, so
	// every number comes back out as float64, never as the int a client sent.
	updates := map[string]interface{}{"capacity": float64(550), "name": "Crescent"}
	assert.NoError(t, NarrowNumericUpdates(updates))
	narrowed, ok := updates["capacity"].(*int)
	assert.True(t, ok, "capacity must be narrowed to *int, got %T", updates["capacity"])
	assert.Equal(t, 550, *narrowed)
	// Unregistered fields are left exactly as they were.
	assert.Equal(t, "Crescent", updates["name"])

	// nil is the clear gesture and must reach the column as SQL NULL, which
	// means a TYPED nil pointer, not a bare interface nil.
	updates = map[string]interface{}{"capacity": nil}
	assert.NoError(t, NarrowNumericUpdates(updates))
	typedNil, ok := updates["capacity"].(*int)
	assert.True(t, ok, "a cleared capacity must stay a typed *int")
	assert.Nil(t, typedNil)

	// An absent field is not invented.
	updates = map[string]interface{}{"name": "Crescent"}
	assert.NoError(t, NarrowNumericUpdates(updates))
	_, present := updates["capacity"]
	assert.False(t, present)

	// Narrowing is a TYPE fix, so it does not apply the domain range: rollback
	// restores historical values that may predate a bound.
	for _, outOfRange := range []float64{0, -1, contracts.MaxVenueCapacity + 1} {
		updates = map[string]interface{}{"capacity": outOfRange}
		assert.NoErrorf(t, NarrowNumericUpdates(updates), "%v is out of range but narrowable", outOfRange)
	}

	// A value with no faithful narrowing is an error rather than a silent write.
	// "550" is in this list on purpose: capacity carries no LegacyTextEncoding
	// flag, because its registry entry and its numeric drawer control shipped
	// together, so a capacity string can only be a corrupt row.
	for _, bad := range []any{550.7, "550", true, map[string]any{"x": 1}} {
		updates = map[string]interface{}{"capacity": bad}
		assert.Errorf(t, NarrowNumericUpdates(updates), "capacity %#v must be rejected", bad)
	}
}

// PSY-1703: the year fields mirror capacity above, with one addition that is the
// whole reason this test exists separately: they carry LegacyTextEncoding.
//
// Registering them made this function start inspecting values that were written
// YEARS ago, when the drawer submitted a year as text. Without the flag, every
// one of those stored rows would turn into an error: a pending edit nobody can
// approve, and a revision nobody can roll back. Undo breaking on old history is
// a worse failure than the corruption the registry was added to prevent, so the
// assertions below are the ones that keep both working.
func TestNarrowNumericUpdates_CatalogYears(t *testing.T) {
	for _, field := range []string{"founded_year", "release_year"} {
		t.Run(field, func(t *testing.T) {
			// The current shape: JSONB hands a number back as float64.
			updates := map[string]interface{}{field: float64(1985)}
			assert.NoError(t, NarrowNumericUpdates(updates))
			narrowed, ok := updates[field].(*int)
			assert.True(t, ok, "%s must be narrowed to *int, got %T", field, updates[field])
			assert.Equal(t, 1985, *narrowed)

			// The LEGACY shape: what the old text drawer could only have
			// written for a year edit, and what a rollback of one therefore has
			// to be able to read.
			for raw, want := range map[string]int{"1985": 1985, " 1985 ": 1985, "0999": 999} {
				updates = map[string]interface{}{field: raw}
				assert.NoErrorf(t, NarrowNumericUpdates(updates), "legacy %q must narrow", raw)
				narrowed, ok = updates[field].(*int)
				assert.Truef(t, ok, "legacy %q must become *int, got %T", raw, updates[field])
				assert.Equalf(t, want, *narrowed, "legacy %q narrowed to the wrong year", raw)
			}

			// nil is the clear gesture and must reach the column as SQL NULL.
			updates = map[string]interface{}{field: nil}
			assert.NoError(t, NarrowNumericUpdates(updates))
			typedNil, ok := updates[field].(*int)
			assert.True(t, ok, "a cleared %s must stay a typed *int", field)
			assert.Nil(t, typedNil)

			// Type fix only, no range: rollback restores history, and history can
			// hold a year that predates this bound.
			for _, outOfRange := range []any{float64(0), float64(-1), float64(9999), "9999"} {
				updates = map[string]interface{}{field: outOfRange}
				assert.NoErrorf(t, NarrowNumericUpdates(updates), "%#v is out of range but narrowable", outOfRange)
			}

			// Leniency stops at values with one unambiguous integer meaning.
			// A string that is not a plain integer has no faithful narrowing, so
			// the flag must not turn into "parse whatever you find".
			for _, bad := range []any{1985.7, "1985.0", "1e3", "1985 approx", "", "banana", true, map[string]any{"x": 1}} {
				updates = map[string]interface{}{field: bad}
				assert.Errorf(t, NarrowNumericUpdates(updates), "%s %#v must be rejected", field, bad)
			}
		})
	}
}

// checkNumericUpdateBounds is the policy half, applied only where NEW
// contributor input is accepted.
func TestCheckNumericUpdateBounds(t *testing.T) {
	inRange := func(v int) map[string]interface{} { return map[string]interface{}{"capacity": &v} }

	for _, ok := range []int{contracts.MinVenueCapacity, contracts.MaxVenueCapacity, 550} {
		assert.NoErrorf(t, checkNumericUpdateBounds(inRange(ok)), "capacity %d is legal", ok)
	}
	for _, bad := range []int{0, -1, contracts.MaxVenueCapacity + 1} {
		assert.Errorf(t, checkNumericUpdateBounds(inRange(bad)), "capacity %d must be rejected", bad)
	}

	// The clear gesture is not a range violation.
	assert.NoError(t, checkNumericUpdateBounds(map[string]interface{}{"capacity": (*int)(nil)}))
	// Neither is an absent field.
	assert.NoError(t, checkNumericUpdateBounds(map[string]interface{}{"name": "Crescent"}))
}

// The year ceiling has to be resolved when a value is CHECKED, not when the
// process starts: a package var initialised from time.Now() would leave a
// long-running server refusing a legitimate next-year value through early
// January. That is why the registry is a function, and this pins it. Converting
// it back to a var would freeze the ceiling and fail here only after a year
// boundary, which is precisely the bug that would otherwise ship unnoticed.
func TestNumericEditFieldBoundsResolveTheYearCeiling(t *testing.T) {
	registry := contracts.NumericEditFieldBounds()
	for _, field := range []string{"founded_year", "release_year"} {
		bounds, ok := registry[field]
		assert.Truef(t, ok, "%s must be registered", field)
		assert.Equalf(t, contracts.MaxCatalogYear(), bounds.Max,
			"%s ceiling must track MaxCatalogYear, not a frozen copy", field)
		assert.Equalf(t, contracts.MinCatalogYear, bounds.Min, "%s floor", field)
		assert.Truef(t, bounds.LegacyTextEncoding,
			"%s predates its registration and must keep reading stored strings", field)
	}
	// capacity is fixed and has no text history; it must not drift into either.
	capacity := registry["capacity"]
	assert.Equal(t, contracts.MaxVenueCapacity, capacity.Max)
	assert.False(t, capacity.LegacyTextEncoding,
		"capacity was registered alongside its numeric control, so a string is corrupt")
}

// PSY-1703: the approve-path range check for the year fields. Its counterpart at
// submit (shared.ValidateFieldChangeValue) already rejected these, so this is
// the defence-in-depth half: the last point before an untyped Updates() that
// can still tell a year from a typo.
func TestCheckNumericUpdateBounds_CatalogYears(t *testing.T) {
	// Resolved the same way the registry resolves it, so this test does not
	// start failing on 1 January.
	maxYear := contracts.MaxCatalogYear()

	for _, field := range []string{"founded_year", "release_year"} {
		t.Run(field, func(t *testing.T) {
			at := func(v int) map[string]interface{} { return map[string]interface{}{field: &v} }

			for _, ok := range []int{contracts.MinCatalogYear, maxYear, maxYear - 1, 1985} {
				assert.NoErrorf(t, checkNumericUpdateBounds(at(ok)), "%s %d is legal", field, ok)
			}
			for _, bad := range []int{0, -1, contracts.MinCatalogYear - 1, maxYear + 1, 19850} {
				assert.Errorf(t, checkNumericUpdateBounds(at(bad)), "%s %d must be rejected", field, bad)
			}

			// The clear gesture is not a range violation.
			assert.NoError(t, checkNumericUpdateBounds(map[string]interface{}{field: (*int)(nil)}))
		})
	}
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

// mockEmailServiceForPendingEdit implements contracts.EmailServiceInterface for testing.
type mockEmailServiceForPendingEdit struct {
	configured        bool
	editApprovedCalls []editApprovedCall
	editRejectedCalls []editRejectedCall
	editApprovedErr   error
	editRejectedErr   error
}

type editApprovedCall struct {
	ToEmail        string
	Username       string
	EntityType     string
	EntityName     string
	EntityURL      string
	UnsubscribeURL string
}

type editRejectedCall struct {
	ToEmail         string
	Username        string
	EntityType      string
	EntityName      string
	RejectionReason string
	UnsubscribeURL  string
}

func (m *mockEmailServiceForPendingEdit) IsConfigured() bool                      { return m.configured }
func (m *mockEmailServiceForPendingEdit) SendVerificationEmail(_, _ string) error { return nil }
func (m *mockEmailServiceForPendingEdit) SendMagicLinkEmail(_, _ string) error    { return nil }
func (m *mockEmailServiceForPendingEdit) SendAccountRecoveryEmail(_, _ string, _ int) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendShowReminderEmail(_, _, _, _ string, _ contracts.LocalizedEventTime, _ []string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendFilterNotificationEmail(_, _, _, _ string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendTierPromotionEmail(_, _, _, _, _, _ string, _ []string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendTierDemotionEmail(_, _, _, _, _, _ string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendTierDemotionWarningEmail(_, _, _ string, _, _ float64, _ string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendEditApprovedEmail(toEmail, username, entityType, entityName, entityURL, unsubscribeURL string) error {
	m.editApprovedCalls = append(m.editApprovedCalls, editApprovedCall{
		ToEmail: toEmail, Username: username, EntityType: entityType,
		EntityName: entityName, EntityURL: entityURL, UnsubscribeURL: unsubscribeURL,
	})
	return m.editApprovedErr
}
func (m *mockEmailServiceForPendingEdit) SendEditRejectedEmail(toEmail, username, entityType, entityName, rejectionReason, unsubscribeURL string) error {
	m.editRejectedCalls = append(m.editRejectedCalls, editRejectedCall{
		ToEmail: toEmail, Username: username, EntityType: entityType,
		EntityName: entityName, RejectionReason: rejectionReason, UnsubscribeURL: unsubscribeURL,
	})
	return m.editRejectedErr
}
func (m *mockEmailServiceForPendingEdit) SendCommentNotification(_, _, _, _, _, _, _ string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendMentionNotification(_, _, _, _, _, _, _ string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendSceneDigestEmail(_ string, _ []contracts.SceneDigestGroup, _ string) error {
	return nil
}

func (m *mockEmailServiceForPendingEdit) SendCollectionDigestEmail(_ string, _ []contracts.CollectionDigestGroup, _ string) error {
	return nil
}

type PendingEditServiceIntegrationTestSuite struct {
	suite.Suite
	testDB      *testutil.TestDatabase
	db          *gorm.DB
	svc         *PendingEditService
	revisionSvc *RevisionService
	mockEmail   *mockEmailServiceForPendingEdit
}

func (s *PendingEditServiceIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.revisionSvc = NewRevisionService(s.db)
	s.mockEmail = &mockEmailServiceForPendingEdit{configured: true}
	s.svc = NewPendingEditService(s.db, s.revisionSvc, s.mockEmail, "http://localhost:3000", "http://localhost:8080", "test-jwt-secret")
}

func (s *PendingEditServiceIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *PendingEditServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM pending_entity_edits")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM users")
	// Reset mock email state between tests
	s.mockEmail.editApprovedCalls = nil
	s.mockEmail.editRejectedCalls = nil
	s.mockEmail.editApprovedErr = nil
	s.mockEmail.editRejectedErr = nil
}

func TestPendingEditServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(PendingEditServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) createTestUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("pe-user-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("pe-user-%d", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := s.db.Create(user).Error
	s.Require().NoError(err)
	return user
}

func (s *PendingEditServiceIntegrationTestSuite) createTestArtist(name string) *catalogm.Artist {
	slug := fmt.Sprintf("test-artist-%d", time.Now().UnixNano())
	artist := &catalogm.Artist{
		Name: name,
		Slug: &slug,
	}
	err := s.db.Create(artist).Error
	s.Require().NoError(err)
	return artist
}

func (s *PendingEditServiceIntegrationTestSuite) createTestVenue(name string) *catalogm.Venue {
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

func (s *PendingEditServiceIntegrationTestSuite) createTestFestival(name string) *catalogm.Festival {
	slug := fmt.Sprintf("test-festival-%d", time.Now().UnixNano())
	festival := &catalogm.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  slug,
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
	}
	err := s.db.Create(festival).Error
	s.Require().NoError(err)
	return festival
}

func makeChanges(field, oldVal, newVal string) []adminm.FieldChange {
	return []adminm.FieldChange{{Field: field, OldValue: oldVal, NewValue: newVal}}
}

// =============================================================================
// CreatePendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_ArtistSuccess() {
	user := s.createTestUser()
	artist := s.createTestArtist("Old Name")

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Old Name", "New Name"),
		Summary:    "Fix artist name",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("artist", resp.EntityType)
	s.Equal(artist.ID, resp.EntityID)
	s.Equal(user.ID, resp.SubmittedBy)
	s.Equal(adminm.PendingEditStatusPending, resp.Status)
	s.Equal("Fix artist name", resp.Summary)
	s.Len(resp.FieldChanges, 1)
	s.Equal("name", resp.FieldChanges[0].Field)
	s.NotEmpty(resp.SubmitterName)
	// PSY-619: SubmitterUsername populated for users who set a username so the
	// frontend renders the moderation-queue byline as a /users/:username link.
	s.Require().NotNil(resp.SubmitterUsername)
	s.Equal(*user.Username, *resp.SubmitterUsername)
}

// TestCreatePendingEdit_NoUsername covers the unlinked-byline path (PSY-619):
// a submitter without a username on file should ship `submitter_username: null`
// so the frontend renders plain text rather than a broken /users/null link.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_NoUsername() {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("pe-user-no-uname-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	artist := s.createTestArtist("Old Name")

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Old Name", "New Name"),
		Summary:    "Fix artist name",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.NotEmpty(resp.SubmitterName, "name should fall back through the resolution chain")
	s.Nil(resp.SubmitterUsername, "username should be nil when not set on the user")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_VenueSuccess() {
	user := s.createTestUser()
	venue := s.createTestVenue("Test Venue")

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "name", OldValue: "Test Venue", NewValue: "The Test Venue"},
			{Field: "city", OldValue: "Phoenix", NewValue: "Tempe"},
		},
		Summary: "Correct venue name and city",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("venue", resp.EntityType)
	s.Len(resp.FieldChanges, 2)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_FestivalSuccess() {
	user := s.createTestUser()
	festival := s.createTestFestival("Fest 2026")

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "festival",
		EntityID:   festival.ID,
		UserID:     user.ID,
		Changes:    makeChanges("description", "", "A great music festival"),
		Summary:    "Add festival description",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("festival", resp.EntityType)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_InvalidEntityType() {
	user := s.createTestUser()

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "show",
		EntityID:   1,
		UserID:     user.ID,
		Changes:    makeChanges("name", "a", "b"),
		Summary:    "test",
	})

	s.Error(err)
	s.Contains(err.Error(), "invalid entity type")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_EntityNotFound() {
	user := s.createTestUser()

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   99999,
		UserID:     user.ID,
		Changes:    makeChanges("name", "a", "b"),
		Summary:    "test",
	})

	s.Error(err)
	s.Contains(err.Error(), "entity not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_EmptyChanges() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    []adminm.FieldChange{},
		Summary:    "test",
	})

	s.Error(err)
	s.Contains(err.Error(), "no changes provided")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_EmptySummary() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "a", "b"),
		Summary:    "",
	})

	s.Error(err)
	s.Contains(err.Error(), "summary is required")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_DuplicatePending() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	// First edit succeeds
	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Test", "Test2"),
		Summary:    "First edit",
	})
	s.NoError(err)

	// Second pending edit for same entity by same user fails
	_, err = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Test", "Test3"),
		Summary:    "Second edit",
	})
	s.Error(err)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_DifferentUsersSameEntity() {
	user1 := s.createTestUser()
	user2 := s.createTestUser()
	artist := s.createTestArtist("Test")

	// User 1 creates edit
	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user1.ID,
		Changes:    makeChanges("name", "Test", "Test2"),
		Summary:    "User 1 edit",
	})
	s.NoError(err)

	// User 2 can also create edit for same entity
	_, err = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user2.ID,
		Changes:    makeChanges("name", "Test", "Test3"),
		Summary:    "User 2 edit",
	})
	s.NoError(err)
}

// =============================================================================
// GetPendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestGetPendingEdit_Found() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Test", "New"),
		Summary:    "test",
	})
	s.Require().NoError(err)

	resp, err := s.svc.GetPendingEdit(created.ID)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal(created.ID, resp.ID)
	s.Equal("artist", resp.EntityType)
	s.NotEmpty(resp.SubmitterName)
}

func (s *PendingEditServiceIntegrationTestSuite) TestGetPendingEdit_NotFound() {
	resp, err := s.svc.GetPendingEdit(99999)
	s.NoError(err)
	s.Nil(resp)
}

// =============================================================================
// GetPendingEditsForEntity tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestGetPendingEditsForEntity_Success() {
	user1 := s.createTestUser()
	user2 := s.createTestUser()
	artist := s.createTestArtist("Test")

	// Two users submit edits for the same artist
	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user1.ID,
		Changes: makeChanges("name", "Test", "A"), Summary: "edit 1",
	})
	s.Require().NoError(err)

	_, err = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user2.ID,
		Changes: makeChanges("name", "Test", "B"), Summary: "edit 2",
	})
	s.Require().NoError(err)

	edits, err := s.svc.GetPendingEditsForEntity("artist", artist.ID)
	s.NoError(err)
	s.Len(edits, 2)
}

func (s *PendingEditServiceIntegrationTestSuite) TestGetPendingEditsForEntity_ExcludesNonPending() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Approved"), Summary: "will be approved",
	})
	s.Require().NoError(err)

	// Approve the edit
	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	// Only pending edits returned
	edits, err := s.svc.GetPendingEditsForEntity("artist", artist.ID)
	s.NoError(err)
	s.Len(edits, 0)
}

// The contribution flow bypasses VenueService, so the empty-to-NULL and
// trimming normalization that path applies to age_policy has to be repeated in
// ApprovePendingEdit. This is the flow the edit drawer uses, so it produces most
// values: without the repeat, "  " lands a present-but-blank policy and " 21+ "
// becomes a second bucket beside "21+".
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_VenueAgePolicyTrimsAndBlanksToNull() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Age Policy Venue")

	padded, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{{Field: "age_policy", NewValue: "  21+  "}},
		Summary: "record the door rule",
	})
	s.Require().NoError(err)
	_, err = s.svc.ApprovePendingEdit(context.Background(), padded.ID, reviewer.ID)
	s.Require().NoError(err)

	var updated catalogm.Venue
	s.Require().NoError(s.db.First(&updated, venue.ID).Error)
	s.Require().NotNil(updated.AgePolicy)
	s.Equal("21+", *updated.AgePolicy, "contributor value must be trimmed")

	blank, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{{Field: "age_policy", NewValue: "   "}},
		Summary: "venue dropped the rule",
	})
	s.Require().NoError(err)
	_, err = s.svc.ApprovePendingEdit(context.Background(), blank.ID, reviewer.ID)
	s.Require().NoError(err)

	s.Require().NoError(s.db.First(&updated, venue.ID).Error)
	s.Nil(updated.AgePolicy, "whitespace-only contributor value must clear to NULL")
}

// Capacity is the only field the drawer submits as a JSON number, so approving
// one is the only place the pipeline has to narrow a JSONB number to an integer
// column. Pins the full round trip: submit → JSONB → approve → *int in the
// column, and the clear gesture landing as NULL rather than as 0.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_VenueCapacitySetsAndClears() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Capacity Venue")

	set, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{{Field: "capacity", NewValue: 550}},
		Summary: "counted the room",
	})
	s.Require().NoError(err)
	_, err = s.svc.ApprovePendingEdit(context.Background(), set.ID, reviewer.ID)
	s.Require().NoError(err)

	var updated catalogm.Venue
	s.Require().NoError(s.db.First(&updated, venue.ID).Error)
	s.Require().NotNil(updated.Capacity)
	s.Equal(550, *updated.Capacity)

	// The revision is what the History UI diffs, so the approval has to leave
	// one carrying the capacity change, not just mutate the column.
	var revisionJSON string
	s.Require().NoError(s.db.Raw(
		"SELECT field_changes::text FROM revisions WHERE entity_type = 'venue' AND entity_id = ? ORDER BY id DESC LIMIT 1",
		venue.ID).Scan(&revisionJSON).Error)
	s.Contains(revisionJSON, `"capacity"`, "the approval must record a capacity revision")
	s.Contains(revisionJSON, "550")

	cleared, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{{Field: "capacity", NewValue: nil}},
		Summary: "the number was wrong, we do not know it",
	})
	s.Require().NoError(err)
	_, err = s.svc.ApprovePendingEdit(context.Background(), cleared.ID, reviewer.ID)
	s.Require().NoError(err)

	s.Require().NoError(s.db.First(&updated, venue.ID).Error)
	s.Nil(updated.Capacity, "clearing capacity must store NULL, not 0")
}

// The fraction case is the one that fails if normalizeCapacityUpdate is
// deleted. Postgres does not reject a float written to an integer column: it
// stores a truncated value and reports success, so without the narrowing a
// contributor's 550.7 would land as 550 with nothing anywhere saying so.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RejectsFractionalCapacity() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Fractional Capacity Venue")

	edit, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{{Field: "capacity", NewValue: 550.7}},
		Summary: "a capacity cannot have a fraction",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), edit.ID, reviewer.ID)
	s.Require().Error(err)

	var untouched catalogm.Venue
	s.Require().NoError(s.db.First(&untouched, venue.ID).Error)
	s.Nil(untouched.Capacity, "a fraction must be rejected, not silently truncated into the column")
}

// A capacity outside the accepted range cannot be applied. The suggest-edit
// handler rejects it at submit, so reaching approval means the row arrived from
// somewhere else; the entity must be left untouched rather than take the value.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RejectsOutOfRangeCapacity() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Out Of Range Venue")

	edit, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{{Field: "capacity", NewValue: 0}},
		Summary: "zero is not a capacity",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), edit.ID, reviewer.ID)
	s.Require().Error(err)

	var untouched catalogm.Venue
	s.Require().NoError(s.db.First(&untouched, venue.ID).Error)
	s.Nil(untouched.Capacity, "a rejected capacity must not reach the column")

	// The edit stays pending so an admin can reject it with a real reason
	// instead of it vanishing behind a failed approval.
	var row adminm.PendingEntityEdit
	s.Require().NoError(s.db.First(&row, edit.ID).Error)
	s.Equal(adminm.PendingEditStatusPending, row.Status)
}

// TestApprovePendingEdit_VenueLocationReGeocodes verifies PSY-985: approving a
// venue location edit through the contribution flow re-geocodes the venue (this
// path bypasses VenueService), populating timezone/coordinates for the new city.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_VenueLocationReGeocodes() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Geo Venue") // Phoenix, AZ — created directly, no timezone yet

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{
			{Field: "city", NewValue: "Denver"},
			{Field: "state", NewValue: "CO"},
		},
		Summary: "relocate to Denver",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	var updated catalogm.Venue
	s.Require().NoError(s.db.First(&updated, venue.ID).Error)
	s.Require().NotNil(updated.Timezone, "venue location edit should re-geocode and set timezone")
	s.Equal("America/Denver", *updated.Timezone)
	s.Require().NotNil(updated.Latitude)
	s.Require().NotNil(updated.Longitude)
}

// Artists and festivals derive a metro from their location the way a venue
// derives a timezone, and the contribution flow bypasses the services that
// would maintain it. Both types go through the same helper, so both are
// asserted here: this is the approve half of applyDerivedLocation, and it had
// no coverage before PSY-1744 moved it.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_ArtistAndFestivalLocationRederiveMetro() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Metro Band") // no location yet
	fest := s.createTestFestival("Metro Fest") // no location yet

	artistEdit, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{
			{Field: "city", NewValue: "Phoenix"},
			{Field: "state", NewValue: "AZ"},
		},
		Summary: "give the band a hometown",
	})
	s.Require().NoError(err)
	_, err = s.svc.ApprovePendingEdit(context.Background(), artistEdit.ID, reviewer.ID)
	s.Require().NoError(err)

	var updatedArtist catalogm.Artist
	s.Require().NoError(s.db.First(&updatedArtist, artist.ID).Error)
	s.Require().NotNil(updatedArtist.Metro, "an approved location edit must derive the artist's metro")
	s.Equal(metroPhoenix, *updatedArtist.Metro)

	festEdit, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "festival", EntityID: fest.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{
			{Field: "city", NewValue: "Phoenix"},
			{Field: "state", NewValue: "AZ"},
		},
		Summary: "give the festival a home",
	})
	s.Require().NoError(err)
	_, err = s.svc.ApprovePendingEdit(context.Background(), festEdit.ID, reviewer.ID)
	s.Require().NoError(err)

	var updatedFest catalogm.Festival
	s.Require().NoError(s.db.First(&updatedFest, fest.ID).Error)
	s.Require().NotNil(updatedFest.Metro, "an approved location edit must derive the festival's metro")
	s.Equal(metroPhoenix, *updatedFest.Metro)
}

// TestApprovePendingEdit_VenueAddressChangeClearsStreetGeocode verifies
// PSY-1536: the contribution path doesn't call Nominatim inline, so approving
// an edit that changes any address-key component must CLEAR the street-level
// geocode fields — coordinates for the OLD address must never survive under
// the new one (the backfill CLI re-resolves them later).
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_VenueAddressChangeClearsStreetGeocode() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Street Geo Venue")

	// Stamp an existing street geocode for the current address.
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).Updates(map[string]interface{}{
		"address":           "130 N Central Ave",
		"street_latitude":   33.448227,
		"street_longitude":  -112.073069,
		"geocode_precision": "rooftop",
		"geocoded_address":  "130 N Central Ave, Phoenix, AZ",
	}).Error)

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{
			{Field: "address", NewValue: "308 N 2nd Ave"},
		},
		Summary: "fix street address",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	var updated catalogm.Venue
	s.Require().NoError(s.db.First(&updated, venue.ID).Error)
	s.Require().NotNil(updated.Address)
	s.Equal("308 N 2nd Ave", *updated.Address)
	s.Nil(updated.StreetLatitude, "old address's street latitude must be cleared")
	s.Nil(updated.StreetLongitude, "old address's street longitude must be cleared")
	s.Nil(updated.GeocodePrecision)
	s.Nil(updated.GeocodedAddress)
}

// mockBandcampFiller records FillProfileResolvedEmbedFromBandcamp calls so a test
// can assert the pending-edit approval flow invokes the PSY-1190 resolver on a
// `bandcamp` edit (and only then). It implements
// contracts.BandcampProfileFillerInterface.
type mockBandcampFiller struct {
	calls []struct {
		artistID   uint
		profileURL string
	}
}

func (m *mockBandcampFiller) FillProfileResolvedEmbedFromBandcamp(artistID uint, profileURL string) {
	m.calls = append(m.calls, struct {
		artistID   uint
		profileURL string
	}{artistID, profileURL})
}

// PSY-1190: a trusted/community pending edit that sets an artist's social.bandcamp
// to a profile root applies via a direct UPDATE here, bypassing
// ArtistService.UpdateArtist's resolver. The approval flow must invoke the filler
// so the embed still gets resolved (fill-when-empty).
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_BandcampProfileTriggersFill() {
	filler := &mockBandcampFiller{}
	s.svc.SetBandcampFiller(filler)
	defer s.svc.SetBandcampFiller(nil) // don't leak into other tests in the suite

	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Bandcamp Artist")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("bandcamp", "", "https://bandcampartist.bandcamp.com"),
		Summary: "add bandcamp profile",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	s.Require().Len(filler.calls, 1, "approving a bandcamp edit must invoke the resolver")
	s.Equal(artist.ID, filler.calls[0].artistID)
	s.Equal("https://bandcampartist.bandcamp.com", filler.calls[0].profileURL)
}

// A non-bandcamp artist edit (or any non-artist edit) must NOT invoke the filler.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_NonBandcampEditNoFill() {
	filler := &mockBandcampFiller{}
	s.svc.SetBandcampFiller(filler)
	defer s.svc.SetBandcampFiller(nil)

	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Name Edit Artist")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Name Edit Artist", "Renamed Artist"),
		Summary: "rename",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	s.Empty(filler.calls, "a non-bandcamp edit must not invoke the resolver")
}

// =============================================================================
// GetUserPendingEdits tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestGetUserPendingEdits_Success() {
	user := s.createTestUser()
	artist1 := s.createTestArtist("Artist 1")
	artist2 := s.createTestArtist("Artist 2")

	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist1.ID, UserID: user.ID,
		Changes: makeChanges("name", "Artist 1", "A1"), Summary: "edit 1",
	})
	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist2.ID, UserID: user.ID,
		Changes: makeChanges("name", "Artist 2", "A2"), Summary: "edit 2",
	})

	edits, total, err := s.svc.GetUserPendingEdits(user.ID, 10, 0)
	s.NoError(err)
	s.Equal(int64(2), total)
	s.Len(edits, 2)
}

func (s *PendingEditServiceIntegrationTestSuite) TestGetUserPendingEdits_Pagination() {
	user := s.createTestUser()
	for i := 0; i < 5; i++ {
		artist := s.createTestArtist(fmt.Sprintf("Artist %d", i))
		_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
			EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
			Changes: makeChanges("name", artist.Name, "new"), Summary: fmt.Sprintf("edit %d", i),
		})
	}

	edits, total, err := s.svc.GetUserPendingEdits(user.ID, 2, 0)
	s.NoError(err)
	s.Equal(int64(5), total)
	s.Len(edits, 2)

	edits2, _, err := s.svc.GetUserPendingEdits(user.ID, 2, 2)
	s.NoError(err)
	s.Len(edits2, 2)
	s.NotEqual(edits[0].ID, edits2[0].ID)
}

// PSY-600: contributor /submissions surface must NEVER leak edits submitted
// by other users. Two users with overlapping pending edits — each sees only
// their own.
func (s *PendingEditServiceIntegrationTestSuite) TestGetUserPendingEdits_OnlyOwnEdits() {
	alice := s.createTestUser()
	bob := s.createTestUser()
	artist := s.createTestArtist("Shared Artist")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: alice.ID,
		Changes: makeChanges("description", "old", "alice's note"), Summary: "alice edit",
	})
	s.Require().NoError(err)
	_, err = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: bob.ID,
		Changes: makeChanges("city", "Old City", "Phoenix"), Summary: "bob edit",
	})
	s.Require().NoError(err)

	aliceEdits, aliceTotal, err := s.svc.GetUserPendingEdits(alice.ID, 20, 0)
	s.NoError(err)
	s.Equal(int64(1), aliceTotal)
	s.Len(aliceEdits, 1)
	s.Equal(alice.ID, aliceEdits[0].SubmittedBy, "alice must only see her own pending edit")
	s.Equal("alice edit", aliceEdits[0].Summary)

	bobEdits, bobTotal, err := s.svc.GetUserPendingEdits(bob.ID, 20, 0)
	s.NoError(err)
	s.Equal(int64(1), bobTotal)
	s.Len(bobEdits, 1)
	s.Equal(bob.ID, bobEdits[0].SubmittedBy, "bob must only see his own pending edit")
	s.Equal("bob edit", bobEdits[0].Summary)
}

// PSY-600: pending-edit responses must include EntitySlug for slug-addressed
// types so the contributor list can render functional /artists/:slug-style
// links instead of broken /artists/:id-style ones.
func (s *PendingEditServiceIntegrationTestSuite) TestGetUserPendingEdits_PopulatesEntitySlug() {
	user := s.createTestUser()
	artist := s.createTestArtist("With Slug")
	s.Require().NotNil(artist.Slug, "test fixture should set artist slug")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", artist.Name, "Renamed"), Summary: "rename",
	})
	s.Require().NoError(err)

	edits, _, err := s.svc.GetUserPendingEdits(user.ID, 20, 0)
	s.NoError(err)
	s.Require().Len(edits, 1)
	s.Require().NotNil(edits[0].EntitySlug, "EntitySlug must be set for artist edits")
	s.Equal(*artist.Slug, *edits[0].EntitySlug)
	s.Equal("With Slug", edits[0].EntityName)
}

// =============================================================================
// ListPendingEdits tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestListPendingEdits_DefaultFilters() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")
	venue := s.createTestVenue("Test Venue")

	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "A"), Summary: "artist edit",
	})
	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test Venue", "V"), Summary: "venue edit",
	})

	edits, total, err := s.svc.ListPendingEdits(nil)
	s.NoError(err)
	s.Equal(int64(2), total)
	s.Len(edits, 2)
}

func (s *PendingEditServiceIntegrationTestSuite) TestListPendingEdits_FilterByStatus() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Approved"), Summary: "will approve",
	})
	_, _ = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)

	// Only pending
	edits, total, err := s.svc.ListPendingEdits(&contracts.PendingEditFilters{Status: "pending"})
	s.NoError(err)
	s.Equal(int64(0), total)
	s.Len(edits, 0)

	// Only approved
	edits, total, err = s.svc.ListPendingEdits(&contracts.PendingEditFilters{Status: "approved"})
	s.NoError(err)
	s.Equal(int64(1), total)
	s.Len(edits, 1)
}

func (s *PendingEditServiceIntegrationTestSuite) TestListPendingEdits_FilterByEntityType() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")
	venue := s.createTestVenue("Test Venue")

	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "A"), Summary: "artist",
	})
	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test Venue", "V"), Summary: "venue",
	})

	edits, total, err := s.svc.ListPendingEdits(&contracts.PendingEditFilters{EntityType: "artist"})
	s.NoError(err)
	s.Equal(int64(1), total)
	s.Len(edits, 1)
	s.Equal("artist", edits[0].EntityType)
}

// =============================================================================
// ApprovePendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AppliesChangesToArtist() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Old Name")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: []adminm.FieldChange{
			{Field: "name", OldValue: "Old Name", NewValue: "New Name"},
			{Field: "city", OldValue: nil, NewValue: "Phoenix"},
		},
		Summary: "Update artist info",
	})
	s.Require().NoError(err)

	resp, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal(adminm.PendingEditStatusApproved, resp.Status)
	s.Require().NotNil(resp.ReviewedBy)
	s.Equal(reviewer.ID, *resp.ReviewedBy)
	s.NotNil(resp.ReviewedAt)

	// Verify entity was updated
	var updated catalogm.Artist
	s.db.First(&updated, artist.ID)
	s.Equal("New Name", updated.Name)
	s.Require().NotNil(updated.City)
	s.Equal("Phoenix", *updated.City)
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AppliesChangesToVenue() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Old Venue")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("name", "Old Venue", "New Venue"),
		Summary: "Fix venue name",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.NoError(err)

	var updated catalogm.Venue
	s.db.First(&updated, venue.ID)
	s.Equal("New Venue", updated.Name)
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RecordsRevision() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Updated"), Summary: "Update name",
	})

	_, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.NoError(err)

	// Verify revision was created
	var revision adminm.Revision
	err = s.db.Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).First(&revision).Error
	s.NoError(err)
	s.Equal(user.ID, revision.UserID) // Attributed to submitter, not reviewer
	s.Require().NotNil(revision.Summary)
	s.Equal("Update name", *revision.Summary)

	var changes []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*revision.FieldChanges, &changes))
	s.Len(changes, 1)
	s.Equal("name", changes[0].Field)
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_NotFound() {
	_, err := s.svc.ApprovePendingEdit(context.Background(), 99999, 1)
	s.Error(err)
	s.Contains(err.Error(), "pending edit not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AlreadyApproved() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "New"), Summary: "test",
	})

	_, _ = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)

	// Try to approve again
	_, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Error(err)
	s.Contains(err.Error(), "not pending")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_EntityDeleted() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Will Delete")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Will Delete", "New"), Summary: "test",
	})

	// Delete the entity
	s.db.Delete(&artist)

	// Approve should fail because entity is gone
	_, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Error(err)
	s.Contains(err.Error(), "entity not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AllowsNewPendingAfter() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	// Create and approve first edit
	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "V2"), Summary: "first edit",
	})
	_, _ = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)

	// User can now submit another pending edit for same entity
	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "V2", "V3"), Summary: "second edit",
	})
	s.NoError(err)
	s.NotNil(resp)
}

// =============================================================================
// PSY-572: per-entity field allowlist gate inside ApprovePendingEdit
// =============================================================================
//
// These tests assert the defence-in-depth gate that filters non-allowlisted
// columns out of pending_edits.field_changes BEFORE the untyped Updates()
// call. Even if the suggest-edit handler validator was bypassed (legacy
// rows, direct DB writes, future code paths), ApprovePendingEdit must
// not apply privileged columns like is_admin, password_hash, etc.

// TestApprovePendingEdit_RejectsDisallowedField simulates a malformed pending
// edit row carrying both an in-allowlist column ("name") and an out-of-
// allowlist column ("is_admin"). The whole approval must fail (no partial
// apply), the entity must be unchanged, and the pending_edit must move to
// 'rejected' with a rejection_reason naming the offending field.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RejectsDisallowedField() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Genuine")

	// CreatePendingEdit doesn't validate field names server-side (the
	// handler-level validator is what blocks contributors), so we can
	// land a malformed row directly through the service.
	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "name", OldValue: "Genuine", NewValue: "Compromised"},
			{Field: "is_admin", OldValue: false, NewValue: true},
		},
		Summary: "malicious edit smuggling is_admin",
	})
	s.Require().NoError(err)
	s.Require().NotNil(created)

	// Approve must fail with the disallowed-fields sentinel.
	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().Error(err)
	s.True(errors.Is(err, adminm.ErrPendingEditDisallowedFields),
		"expected ErrPendingEditDisallowedFields, got %v", err)
	s.Contains(err.Error(), "is_admin")

	// Entity must NOT have been mutated — neither the in-allowlist field
	// (name) nor the out-of-allowlist field (which doesn't exist on artists
	// anyway, but we want the test to prove "no Updates() ran").
	var artistAfter catalogm.Artist
	s.Require().NoError(s.db.First(&artistAfter, artist.ID).Error)
	s.Equal("Genuine", artistAfter.Name, "name should not have changed despite being on allowlist — the whole approval was rejected")

	// Pending edit must be marked 'rejected' with a reason that names the
	// offending field, so the admin UI surfaces a clear explanation.
	resp, err := s.svc.GetPendingEdit(created.ID)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal(adminm.PendingEditStatusRejected, resp.Status)
	s.Require().NotNil(resp.ReviewedBy)
	s.Equal(reviewer.ID, *resp.ReviewedBy)
	s.NotNil(resp.ReviewedAt)
	s.Require().NotNil(resp.RejectionReason)
	s.Contains(*resp.RejectionReason, "is_admin")
	s.Contains(*resp.RejectionReason, "artist")
}

// TestApprovePendingEdit_AppliesAllAllowedFields covers the happy path
// alongside the rejection path: a pending_edit with only allowlisted
// fields still applies cleanly after PSY-572 (no regression).
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AppliesAllAllowedFields() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Old")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "name", OldValue: "Old", NewValue: "New"},
			{Field: "city", OldValue: nil, NewValue: "Phoenix"},
			{Field: "instagram", OldValue: nil, NewValue: "https://instagram.com/x"},
		},
		Summary: "fill in info",
	})
	s.Require().NoError(err)

	resp, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)
	s.Equal(adminm.PendingEditStatusApproved, resp.Status)

	var artistAfter catalogm.Artist
	s.Require().NoError(s.db.First(&artistAfter, artist.ID).Error)
	s.Equal("New", artistAfter.Name)
	s.Require().NotNil(artistAfter.City)
	s.Equal("Phoenix", *artistAfter.City)
}

// TestApprovePendingEdit_RejectsMultipleDisallowedFields: every offending
// field must show up in the rejection reason, not just the first one.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RejectsMultipleDisallowedFields() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Genuine Venue")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "name", NewValue: "Pwned"},
			{Field: "is_admin", NewValue: true},
			{Field: "password_hash", NewValue: "x"},
			{Field: "trust_tier", NewValue: "admin"},
		},
		Summary: "many bad fields",
	})

	_, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().Error(err)
	s.True(errors.Is(err, adminm.ErrPendingEditDisallowedFields))
	for _, bad := range []string{"is_admin", "password_hash", "trust_tier"} {
		s.Contains(err.Error(), bad)
	}

	var venueAfter catalogm.Venue
	s.Require().NoError(s.db.First(&venueAfter, venue.ID).Error)
	s.Equal("Genuine Venue", venueAfter.Name)
}

// =============================================================================
// RejectPendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_Success() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Bad Name"), Summary: "bad edit",
	})

	resp, err := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "Name doesn't follow our naming conventions")
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal(adminm.PendingEditStatusRejected, resp.Status)
	s.Require().NotNil(resp.RejectionReason)
	s.Equal("Name doesn't follow our naming conventions", *resp.RejectionReason)
	s.Require().NotNil(resp.ReviewedBy)
	s.Equal(reviewer.ID, *resp.ReviewedBy)

	// Verify entity was NOT changed
	var artist2 catalogm.Artist
	s.db.First(&artist2, artist.ID)
	s.Equal("Test", artist2.Name)
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_EmptyReason() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "X"), Summary: "test",
	})

	_, err := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "")
	s.Error(err)
	s.Contains(err.Error(), "rejection reason is required")
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_NotFound() {
	_, err := s.svc.RejectPendingEdit(99999, 1, "reason")
	s.Error(err)
	s.Contains(err.Error(), "pending edit not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_AllowsNewPendingAfter() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Bad"), Summary: "bad edit",
	})
	_, _ = s.svc.RejectPendingEdit(created.ID, reviewer.ID, "bad name")

	// User can submit a new edit after rejection
	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Better"), Summary: "improved edit",
	})
	s.NoError(err)
	s.NotNil(resp)
}

// =============================================================================
// CancelPendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_Success() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "New"), Summary: "will cancel",
	})

	err := s.svc.CancelPendingEdit(created.ID, user.ID)
	s.NoError(err)

	// Verify it's deleted
	resp, err := s.svc.GetPendingEdit(created.ID)
	s.NoError(err)
	s.Nil(resp)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_WrongUser() {
	user := s.createTestUser()
	otherUser := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "New"), Summary: "test",
	})

	err := s.svc.CancelPendingEdit(created.ID, otherUser.ID)
	s.Error(err)
	s.Contains(err.Error(), "only the submitter")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_NotFound() {
	err := s.svc.CancelPendingEdit(99999, 1)
	s.Error(err)
	s.Contains(err.Error(), "pending edit not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_AlreadyApproved() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "New"), Summary: "test",
	})
	_, _ = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)

	err := s.svc.CancelPendingEdit(created.ID, user.ID)
	s.Error(err)
	s.Contains(err.Error(), "not pending")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_AllowsNewPendingAfter() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "V1"), Summary: "will cancel",
	})
	_ = s.svc.CancelPendingEdit(created.ID, user.ID)

	// User can create a new pending edit
	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "V2"), Summary: "new edit",
	})
	s.NoError(err)
	s.NotNil(resp)
}

// =============================================================================
// Email notification tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_SendsApprovalEmail() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Cool Band")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Cool Band", "The Cool Band"), Summary: "Add 'The'",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.NoError(err)

	// Verify approval email was sent
	s.Require().Len(s.mockEmail.editApprovedCalls, 1)
	call := s.mockEmail.editApprovedCalls[0]
	s.Equal(*user.Email, call.ToEmail)
	s.Equal("artist", call.EntityType)
	s.Equal("The Cool Band", call.EntityName) // Entity was updated, so name should reflect the update
	s.Contains(call.EntityURL, "/artists/")
	s.Contains(call.EntityURL, "http://localhost:3000")
	// PSY-756: an edit-notifications unsubscribe URL is minted and passed.
	s.Contains(call.UnsubscribeURL, "/unsubscribe/edit-notifications", "expected an HMAC-signed edit-notifications unsubscribe URL")
	s.Contains(call.UnsubscribeURL, "uid=")
	s.Contains(call.UnsubscribeURL, "sig=")
}

// TestApprovePendingEdit_SuppressedWhenOptedOut verifies the edit-notifications
// opt-out flag suppresses the email without blocking the approval (PSY-756).
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_SuppressedWhenOptedOut() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Optout Band")

	// Opt the submitter out of edit-review emails.
	s.Require().NoError(s.db.Create(&authm.UserPreferences{
		UserID:                    user.ID,
		NotifyOnEditNotifications: false,
	}).Error)
	s.Require().NoError(s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", user.ID).
		Update("notify_on_edit_notifications", false).Error)

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Optout Band", "The Optout Band"), Summary: "add 'The'",
	})
	s.Require().NoError(err)

	resp, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.NoError(err)
	s.Equal(adminm.PendingEditStatusApproved, resp.Status, "approval must still succeed when emails are opted out")

	// No email should have been sent.
	s.Empty(s.mockEmail.editApprovedCalls, "approval email must be suppressed when opted out")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_EmailErrorDoesNotFail() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test Band")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test Band", "Better Name"), Summary: "improve name",
	})
	s.Require().NoError(err)

	// Make email service return an error
	s.mockEmail.editApprovedErr = fmt.Errorf("email API is down")

	// Approval should still succeed despite email error
	resp, err := s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(adminm.PendingEditStatusApproved, resp.Status)
	s.Len(s.mockEmail.editApprovedCalls, 1) // Email was attempted
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_SendsRejectionEmail() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Great Venue")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("name", "Great Venue", "Bad Name"), Summary: "rename",
	})
	s.Require().NoError(err)

	_, err = s.svc.RejectPendingEdit(created.ID, reviewer.ID, "Name does not match official venue name")
	s.NoError(err)

	// Verify rejection email was sent
	s.Require().Len(s.mockEmail.editRejectedCalls, 1)
	call := s.mockEmail.editRejectedCalls[0]
	s.Equal(*user.Email, call.ToEmail)
	s.Equal("venue", call.EntityType)
	s.Equal("Great Venue", call.EntityName) // Entity was NOT updated on rejection
	s.Equal("Name does not match official venue name", call.RejectionReason)
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_EmailErrorDoesNotFail() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test Artist")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test Artist", "Wrong"), Summary: "bad edit",
	})
	s.Require().NoError(err)

	// Make email service return an error
	s.mockEmail.editRejectedErr = fmt.Errorf("email API is down")

	// Rejection should still succeed despite email error
	resp, err := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "incorrect info")
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(adminm.PendingEditStatusRejected, resp.Status)
	s.Len(s.mockEmail.editRejectedCalls, 1) // Email was attempted
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_VenueEmailHasCorrectURL() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("The Rebel Lounge")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("city", "Phoenix", "Tempe"), Summary: "fix city",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.NoError(err)

	s.Require().Len(s.mockEmail.editApprovedCalls, 1)
	call := s.mockEmail.editApprovedCalls[0]
	s.Contains(call.EntityURL, "/venues/")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_FestivalEmailHasCorrectURL() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	festival := s.createTestFestival("Fest 2026")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "festival", EntityID: festival.ID, UserID: user.ID,
		Changes: makeChanges("description", "", "Great festival"), Summary: "add desc",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.NoError(err)

	s.Require().Len(s.mockEmail.editApprovedCalls, 1)
	call := s.mockEmail.editApprovedCalls[0]
	s.Contains(call.EntityURL, "/festivals/")
}

// =============================================================================
// Nil email service tests (unit tests, no DB)
// =============================================================================

func TestPendingEditService_NilEmailServiceDoesNotPanic(t *testing.T) {
	// Constructor with nil email service should work fine
	svc := NewPendingEditService(nil, nil, nil, "", "", "")
	assert.NotNil(t, svc)

	// sendApprovalEmail and sendRejectionEmail should not panic with nil email service
	svc.sendApprovalEmail(&adminm.PendingEntityEdit{SubmittedBy: 1, EntityType: "artist", EntityID: 1})
	svc.sendRejectionEmail(&adminm.PendingEntityEdit{SubmittedBy: 1, EntityType: "artist", EntityID: 1}, "reason")
}

func TestPendingEditService_UnconfiguredEmailServiceDoesNotPanic(t *testing.T) {
	mockEmail := &mockEmailServiceForPendingEdit{configured: false}
	svc := NewPendingEditService(nil, nil, mockEmail, "http://localhost:3000", "http://localhost:8080", "test-jwt-secret")

	// Should return early without attempting to send
	svc.sendApprovalEmail(&adminm.PendingEntityEdit{SubmittedBy: 1, EntityType: "artist", EntityID: 1})
	svc.sendRejectionEmail(&adminm.PendingEntityEdit{SubmittedBy: 1, EntityType: "artist", EntityID: 1}, "reason")

	assert.Len(t, mockEmail.editApprovedCalls, 0)
	assert.Len(t, mockEmail.editRejectedCalls, 0)
}
