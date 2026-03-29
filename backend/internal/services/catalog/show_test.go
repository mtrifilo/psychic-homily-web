package catalog

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestShowService_NilDatabase(t *testing.T) {
	svc := &ShowService{db: nil}

	t.Run("CreateShow", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.CreateShow(&contracts.CreateShowRequest{})
		})
	})

	t.Run("GetShow", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetShow(1)
		})
	})

	t.Run("GetShowBySlug", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetShowBySlug("test-slug")
		})
	})

	t.Run("GetShows", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetShows(nil)
		})
	})

	t.Run("UpdateShow", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.UpdateShow(1, map[string]interface{}{"title": "x"})
		})
	})

	t.Run("UpdateShowWithRelations", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.UpdateShowWithRelations(1, nil, nil, nil, false)
			return err
		})
	})

	t.Run("DeleteShow", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.DeleteShow(1)
		})
	})

	t.Run("GetPendingShows", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.GetPendingShows(10, 0, nil)
			return err
		})
	})

	t.Run("GetRejectedShows", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.GetRejectedShows(10, 0, "")
			return err
		})
	})

	t.Run("ApproveShow", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.ApproveShow(1, false)
		})
	})

	t.Run("RejectShow", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.RejectShow(1, "reason")
		})
	})

	t.Run("UnpublishShow", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.UnpublishShow(1, 1, false)
		})
	})

	t.Run("MakePrivateShow", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.MakePrivateShow(1, 1, false)
		})
	})

	t.Run("PublishShow", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.PublishShow(1, 1, false)
		})
	})

	t.Run("GetUserSubmissions", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.GetUserSubmissions(1, 10, 0)
			return err
		})
	})

	t.Run("GetUpcomingShows", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.GetUpcomingShows("UTC", "", 10, false, nil)
			return err
		})
	})

	t.Run("GetShowCities", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetShowCities("UTC")
		})
	})

	t.Run("SetShowSoldOut", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.SetShowSoldOut(1, true)
		})
	})

	t.Run("SetShowCancelled", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.SetShowCancelled(1, true)
		})
	})

	t.Run("ExportShowToMarkdown", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.ExportShowToMarkdown(1)
			return err
		})
	})

	t.Run("PreviewShowImport", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.PreviewShowImport([]byte("---\n---"))
		})
	})

	t.Run("ConfirmShowImport", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.ConfirmShowImport([]byte("---\n---"), false)
		})
	})

	t.Run("GetAdminShows", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.GetAdminShows(10, 0, contracts.AdminShowFilters{})
			return err
		})
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ShowServiceIntegrationTestSuite struct {
	suite.Suite
	testDB      *testutil.TestDatabase
	db          *gorm.DB
	showService *ShowService
}

func (suite *ShowServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.showService = &ShowService{db: suite.testDB.DB}
}

func (suite *ShowServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

// TearDownTest cleans up data between tests for isolation
func (suite *ShowServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestShowServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ShowServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *ShowServiceIntegrationTestSuite) createTestVenue(name, city, state string, verified bool) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: verified,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *ShowServiceIntegrationTestSuite) createTestShow(opts ...func(*contracts.CreateShowRequest)) *contracts.ShowResponse {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:     "Test Show",
		EventDate: time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "The Venue", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Test Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	for _, opt := range opts {
		opt(req)
	}
	resp, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	return resp
}

func boolPtr(b bool) *bool {
	return &b
}

func uintPtr(u uint) *uint {
	return &u
}

// =============================================================================
// Group 1: CRUD Basics
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_Success() {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:     "Rock Night",
		EventDate: time.Date(2026, 7, 10, 21, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Valley Bar", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "The Rockers", IsHeadliner: boolPtr(true)},
			{Name: "Opening Act", IsHeadliner: boolPtr(false)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Rock Night", resp.Title)
	suite.Equal("approved", resp.Status)
	suite.NotEmpty(resp.Slug)
	suite.Len(resp.Venues, 1)
	suite.Len(resp.Artists, 2)
	suite.Equal("The Rockers", resp.Artists[0].Name)
	suite.True(*resp.Artists[0].IsHeadliner)
	suite.Equal("Opening Act", resp.Artists[1].Name)
	suite.False(*resp.Artists[1].IsHeadliner)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_Private() {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:     "Private Gig",
		EventDate: time.Date(2026, 8, 1, 19, 0, 0, 0, time.UTC),
		City:      "Tempe",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Small Club", City: "Tempe", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Solo Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		IsPrivate:         true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Equal("private", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_ExistingVenue() {
	existing := suite.createTestVenue("Existing Hall", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	req := &contracts.CreateShowRequest{
		Title:     "Show at Existing",
		EventDate: time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Existing Hall", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Band One", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Require().Len(resp.Venues, 1)
	suite.Equal(existing.ID, resp.Venues[0].ID)

	// Verify no duplicate venue was created
	var venueCount int64
	suite.db.Model(&models.Venue{}).Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", "Existing Hall", "Phoenix").Count(&venueCount)
	suite.Equal(int64(1), venueCount)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NewArtistAndVenue() {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:     "Brand New Show",
		EventDate: time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC),
		City:      "Tucson",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "New Place", City: "Tucson", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Brand New Band", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Len(resp.Venues, 1)
	suite.True(*resp.Venues[0].IsNewVenue)
	suite.Len(resp.Artists, 1)
	suite.True(*resp.Artists[0].IsNewArtist)

	// Verify records exist in DB
	var artist models.Artist
	suite.NoError(suite.db.Where("name = ?", "Brand New Band").First(&artist).Error)
	suite.NotNil(artist.Slug)

	var venue models.Venue
	suite.NoError(suite.db.Where("name = ?", "New Place").First(&venue).Error)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NewArtistWithInstagram() {
	user := suite.createTestUser()
	igHandle := "@ig_artist"
	req := &contracts.CreateShowRequest{
		Title:     "IG Show",
		EventDate: time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "IG Venue", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "IG Artist", IsHeadliner: boolPtr(true), InstagramHandle: &igHandle},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Len(resp.Artists, 1)

	// Verify DB artist has instagram set
	var artist models.Artist
	suite.NoError(suite.db.Where("name = ?", "IG Artist").First(&artist).Error)
	suite.Require().NotNil(artist.Social.Instagram)
	suite.Equal("@ig_artist", *artist.Social.Instagram)

	// Verify response socials
	suite.Require().NotNil(resp.Artists[0].Socials.Instagram)
	suite.Equal("@ig_artist", *resp.Artists[0].Socials.Instagram)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NewArtistWithoutInstagram() {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:     "No IG Show",
		EventDate: time.Date(2026, 10, 3, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "No IG Venue", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "No IG Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	// Verify DB artist has no instagram
	var artist models.Artist
	suite.NoError(suite.db.Where("name = ?", "No IG Artist").First(&artist).Error)
	suite.Nil(artist.Social.Instagram)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_ExistingArtistIgnoresInstagram() {
	// Pre-create artist with no Instagram
	preExisting := models.Artist{Name: "Pre-existing Artist"}
	suite.Require().NoError(suite.db.Create(&preExisting).Error)

	user := suite.createTestUser()
	igHandle := "@should_ignore"
	req := &contracts.CreateShowRequest{
		Title:     "Existing Artist IG Show",
		EventDate: time.Date(2026, 10, 4, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Some Venue", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Pre-existing Artist", IsHeadliner: boolPtr(true), InstagramHandle: &igHandle},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Len(resp.Artists, 1)
	// Artist should be reused, not duplicated
	suite.Equal(preExisting.ID, resp.Artists[0].ID)

	// Verify DB artist still has no instagram (existing artist socials unchanged)
	var artist models.Artist
	suite.NoError(suite.db.First(&artist, preExisting.ID).Error)
	suite.Nil(artist.Social.Instagram, "Existing artist's Instagram should not be modified")
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NewArtistEmptyInstagramIgnored() {
	user := suite.createTestUser()
	emptyIG := ""
	req := &contracts.CreateShowRequest{
		Title:     "Empty IG Show",
		EventDate: time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Empty IG Venue", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Empty IG Artist", IsHeadliner: boolPtr(true), InstagramHandle: &emptyIG},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	// Verify DB artist has no instagram (empty string should be treated as nil)
	var artist models.Artist
	suite.NoError(suite.db.Where("name = ?", "Empty IG Artist").First(&artist).Error)
	suite.Nil(artist.Social.Instagram, "Empty instagram handle should not be stored")
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShow_Success() {
	created := suite.createTestShow()

	resp, err := suite.showService.GetShow(created.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Title, resp.Title)
	suite.Equal(created.Slug, resp.Slug)
	suite.Len(resp.Venues, 1)
	suite.Len(resp.Artists, 1)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShow_NotFound() {
	resp, err := suite.showService.GetShow(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowNotFound, showErr.Code)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShowBySlug_Success() {
	created := suite.createTestShow()

	resp, err := suite.showService.GetShowBySlug(created.Slug)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShowBySlug_NotFound() {
	resp, err := suite.showService.GetShowBySlug("nonexistent-slug-2026")

	suite.Require().Error(err)
	suite.Nil(resp)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowNotFound, showErr.Code)
}

func (suite *ShowServiceIntegrationTestSuite) TestDeleteShow_Success() {
	created := suite.createTestShow()

	err := suite.showService.DeleteShow(created.ID)

	suite.Require().NoError(err)

	// Verify show is gone
	_, err = suite.showService.GetShow(created.ID)
	suite.Error(err)
}

func (suite *ShowServiceIntegrationTestSuite) TestDeleteShow_AssociationsCleanedUp() {
	created := suite.createTestShow()
	showID := created.ID

	err := suite.showService.DeleteShow(showID)
	suite.Require().NoError(err)

	// Verify junction table rows are gone
	var svCount int64
	suite.db.Model(&models.ShowVenue{}).Where("show_id = ?", showID).Count(&svCount)
	suite.Zero(svCount)

	var saCount int64
	suite.db.Model(&models.ShowArtist{}).Where("show_id = ?", showID).Count(&saCount)
	suite.Zero(saCount)
}

// =============================================================================
// Group 2: Updates
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShow_BasicFields() {
	created := suite.createTestShow()

	resp, err := suite.showService.UpdateShow(created.ID, map[string]interface{}{
		"title":       "Updated Title",
		"description": "New description",
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Title", resp.Title)
	suite.Equal("New description", *resp.Description)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShow_EventDate_UTC() {
	created := suite.createTestShow()

	// Pass a non-UTC time
	eastern, _ := time.LoadLocation("America/New_York")
	newDate := time.Date(2026, 12, 25, 20, 0, 0, 0, eastern)

	resp, err := suite.showService.UpdateShow(created.ID, map[string]interface{}{
		"event_date": newDate,
	})

	suite.Require().NoError(err)
	// Verify the stored time represents the same instant (service converts to UTC before storing)
	suite.Equal(newDate.UTC().Unix(), resp.EventDate.Unix(),
		"event_date should represent the same instant after UTC conversion")
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_ReplaceArtists() {
	created := suite.createTestShow()

	newArtists := []contracts.CreateShowArtist{
		{Name: "Replacement Headliner", IsHeadliner: boolPtr(true)},
		{Name: "Replacement Opener", IsHeadliner: boolPtr(false)},
	}

	resp, _, err := suite.showService.UpdateShowWithRelations(created.ID, nil, nil, newArtists, true)

	suite.Require().NoError(err)
	suite.Require().Len(resp.Artists, 2)
	suite.Equal("Replacement Headliner", resp.Artists[0].Name)
	suite.True(*resp.Artists[0].IsHeadliner)
	suite.Equal("Replacement Opener", resp.Artists[1].Name)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_ReplaceVenues() {
	created := suite.createTestShow()

	newVenues := []contracts.CreateShowVenue{
		{Name: "Replacement Venue", City: "Tempe", State: "AZ"},
	}

	resp, _, err := suite.showService.UpdateShowWithRelations(created.ID, nil, newVenues, nil, true)

	suite.Require().NoError(err)
	suite.Require().Len(resp.Venues, 1)
	suite.Equal("Replacement Venue", resp.Venues[0].Name)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_OrphanedArtist() {
	// Create a show with a unique artist
	created := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Artists = []contracts.CreateShowArtist{
			{Name: "Soon Orphaned", IsHeadliner: boolPtr(true)},
		}
	})

	// Replace artists with a different one
	newArtists := []contracts.CreateShowArtist{
		{Name: "Brand New Star", IsHeadliner: boolPtr(true)},
	}

	_, orphans, err := suite.showService.UpdateShowWithRelations(created.ID, nil, nil, newArtists, true)

	suite.Require().NoError(err)
	suite.Require().Len(orphans, 1)
	suite.Equal("Soon Orphaned", orphans[0].Name)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_NoOrphanIfStillAssociated() {
	// Create two shows sharing the same artist
	user := suite.createTestUser()
	req1 := &contracts.CreateShowRequest{
		Title:     "Show A",
		EventDate: time.Date(2026, 6, 10, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Venue A", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "Shared Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	showA, err := suite.showService.CreateShow(req1)
	suite.Require().NoError(err)

	req2 := &contracts.CreateShowRequest{
		Title:     "Show B",
		EventDate: time.Date(2026, 6, 11, 20, 0, 0, 0, time.UTC),
		City:      "Tempe",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Venue B", City: "Tempe", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "Shared Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)
	suite.Require().NoError(err)

	// Remove the shared artist from Show A
	newArtists := []contracts.CreateShowArtist{
		{Name: "Different Artist", IsHeadliner: boolPtr(true)},
	}

	_, orphans, err := suite.showService.UpdateShowWithRelations(showA.ID, nil, nil, newArtists, true)

	suite.Require().NoError(err)
	// "Shared Artist" is still on Show B, so it should NOT be orphaned
	for _, o := range orphans {
		suite.NotEqual("Shared Artist", o.Name)
	}
}

// =============================================================================
// Group 3: Status Transitions
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestApproveShow_FromPending() {
	// Create a pending show by setting status to pending directly
	created := suite.createTestShow()
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Update("status", models.ShowStatusPending)

	resp, err := suite.showService.ApproveShow(created.ID, false)

	suite.Require().NoError(err)
	suite.Equal("approved", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestApproveShow_FromRejected() {
	created := suite.createTestShow()
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Updates(map[string]interface{}{
		"status":           models.ShowStatusRejected,
		"rejection_reason": "Bad info",
	})

	resp, err := suite.showService.ApproveShow(created.ID, false)

	suite.Require().NoError(err)
	suite.Equal("approved", resp.Status)
	// Rejection reason should be cleared
	suite.True(resp.RejectionReason == nil || *resp.RejectionReason == "")
}

func (suite *ShowServiceIntegrationTestSuite) TestApproveShow_WithVenueVerification() {
	// Create show with an unverified venue
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:     "Verify Venue Show",
		EventDate: time.Date(2026, 7, 20, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Unverified Place", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Verify Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  false, // non-admin creates unverified venue
	}
	created, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Set show to pending so we can approve it
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Update("status", models.ShowStatusPending)

	resp, err := suite.showService.ApproveShow(created.ID, true)

	suite.Require().NoError(err)
	suite.Equal("approved", resp.Status)

	// Venue should now be verified
	var venue models.Venue
	suite.db.First(&venue, resp.Venues[0].ID)
	suite.True(venue.Verified)
}

func (suite *ShowServiceIntegrationTestSuite) TestApproveShow_AlreadyApproved_Fails() {
	created := suite.createTestShow() // created as approved

	_, err := suite.showService.ApproveShow(created.ID, false)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "cannot be approved")
}

func (suite *ShowServiceIntegrationTestSuite) TestRejectShow_Success() {
	created := suite.createTestShow()
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Update("status", models.ShowStatusPending)

	resp, err := suite.showService.RejectShow(created.ID, "Duplicate listing")

	suite.Require().NoError(err)
	suite.Equal("rejected", resp.Status)
	suite.Require().NotNil(resp.RejectionReason)
	suite.Equal("Duplicate listing", *resp.RejectionReason)
}

func (suite *ShowServiceIntegrationTestSuite) TestRejectShow_NotPending_Fails() {
	created := suite.createTestShow() // approved

	_, err := suite.showService.RejectShow(created.ID, "reason")

	suite.Require().Error(err)
	suite.Contains(err.Error(), "not pending")
}

func (suite *ShowServiceIntegrationTestSuite) TestUnpublishShow_AsSubmitter() {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:             "Submitter Unpublish",
		EventDate:         time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Unpub Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Unpub Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	created, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	resp, err := suite.showService.UnpublishShow(created.ID, user.ID, false)

	suite.Require().NoError(err)
	suite.Equal("private", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestUnpublishShow_AsAdmin() {
	created := suite.createTestShow()
	adminID := uint(9999)

	resp, err := suite.showService.UnpublishShow(created.ID, adminID, true)

	suite.Require().NoError(err)
	suite.Equal("private", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestUnpublishShow_Unauthorized() {
	created := suite.createTestShow()
	differentUser := suite.createTestUser()

	_, err := suite.showService.UnpublishShow(created.ID, differentUser.ID, false)

	suite.Require().Error(err)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowUnpublishUnauthorized, showErr.Code)
}

func (suite *ShowServiceIntegrationTestSuite) TestMakePrivateShow_Success() {
	created := suite.createTestShow()
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Update("status", models.ShowStatusPending)

	// Get the submitter ID
	var show models.Show
	suite.db.First(&show, created.ID)

	resp, err := suite.showService.MakePrivateShow(created.ID, *show.SubmittedBy, false)

	suite.Require().NoError(err)
	suite.Equal("private", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestMakePrivateShow_NotPending_Fails() {
	created := suite.createTestShow() // approved
	var show models.Show
	suite.db.First(&show, created.ID)

	_, err := suite.showService.MakePrivateShow(created.ID, *show.SubmittedBy, false)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "can only make pending shows private")
}

func (suite *ShowServiceIntegrationTestSuite) TestPublishShow_Success() {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:             "To Publish",
		EventDate:         time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Pub Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Pub Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		IsPrivate:         true,
	}
	created, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)
	suite.Equal("private", created.Status)

	resp, err := suite.showService.PublishShow(created.ID, user.ID, false)

	suite.Require().NoError(err)
	suite.Equal("approved", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestPublishShow_Unauthorized() {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:             "Unauth Publish",
		EventDate:         time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Unauth Pub Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Unauth Pub Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		IsPrivate:         true,
	}
	created, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	differentUser := suite.createTestUser()
	_, err = suite.showService.PublishShow(created.ID, differentUser.ID, false)

	suite.Require().Error(err)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowPublishUnauthorized, showErr.Code)
}

// =============================================================================
// Group 4: Duplicate Detection
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_DuplicateHeadliner_SameVenueSameDay_Fails() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)

	req := &contracts.CreateShowRequest{
		Title:             "First Show",
		EventDate:         eventDate,
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Dup Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Dup Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Try creating duplicate
	req2 := &contracts.CreateShowRequest{
		Title:             "Second Show (duplicate)",
		EventDate:         eventDate,
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Dup Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Dup Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already performing")
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_DuplicateHeadliner_CaseInsensitive() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)

	req := &contracts.CreateShowRequest{
		Title:             "Original Case Show",
		EventDate:         eventDate,
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Case Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "The Band", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Try with different case
	req2 := &contracts.CreateShowRequest{
		Title:             "Case Insensitive Dup",
		EventDate:         eventDate,
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "case venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "the band", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already performing")
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_DuplicateHeadliner_DifferentDay_OK() {
	user := suite.createTestUser()

	req := &contracts.CreateShowRequest{
		Title:             "Day 1",
		EventDate:         time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Multi Day Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Day Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Same headliner, same venue, DIFFERENT day
	req2 := &contracts.CreateShowRequest{
		Title:             "Day 2",
		EventDate:         time.Date(2026, 11, 4, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Multi Day Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Day Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	resp, err := suite.showService.CreateShow(req2)

	suite.Require().NoError(err)
	suite.NotNil(resp)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_DuplicateHeadliner_DifferentVenue_OK() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC)

	req := &contracts.CreateShowRequest{
		Title:             "Venue 1",
		EventDate:         eventDate,
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Venue Alpha", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Venue Hopper", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Same headliner, same day, DIFFERENT venue
	req2 := &contracts.CreateShowRequest{
		Title:             "Venue 2",
		EventDate:         eventDate,
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Venue Beta", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Venue Hopper", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	resp, err := suite.showService.CreateShow(req2)

	suite.Require().NoError(err)
	suite.NotNil(resp)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NoHeadliner_DifferentFirstArtist_OK() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC)

	// First show with no explicit headliner — first artist used for dedup
	req := &contracts.CreateShowRequest{
		Title:     "No Headliner 1",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "NH Venue", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "Opener Only", IsHeadliner: boolPtr(false)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Different first artist, same venue and date — should succeed
	req2 := &contracts.CreateShowRequest{
		Title:     "No Headliner 2",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "NH Venue", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "Opener Only 2", IsHeadliner: boolPtr(false)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	resp, err := suite.showService.CreateShow(req2)

	suite.Require().NoError(err)
	suite.NotNil(resp)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NoHeadliner_SameFirstArtist_SameVenue_Fails() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 7, 20, 0, 0, 0, time.UTC)

	// First show — no headliner flag, first artist is "The Growlers"
	req := &contracts.CreateShowRequest{
		Title:     "Growlers Show 1",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Fallback Dedup Venue", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "The Growlers"},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Duplicate — same first artist, same venue, same day, no headliner
	req2 := &contracts.CreateShowRequest{
		Title:     "Growlers Show 2 (duplicate)",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Fallback Dedup Venue", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "The Growlers"},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already performing")
}

// =============================================================================
// Group 5: Listing & Filtering
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestGetShows_FilterByCity() {
	user := suite.createTestUser()

	// Phoenix show
	req1 := &contracts.CreateShowRequest{
		Title:             "Phoenix Show",
		EventDate:         time.Date(2026, 12, 1, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "PHX Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "PHX Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req1)
	suite.Require().NoError(err)

	// Tucson show
	req2 := &contracts.CreateShowRequest{
		Title:             "Tucson Show",
		EventDate:         time.Date(2026, 12, 2, 20, 0, 0, 0, time.UTC),
		City:              "Tucson",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "TUC Venue", City: "Tucson", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "TUC Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)
	suite.Require().NoError(err)

	resp, err := suite.showService.GetShows(map[string]interface{}{"city": "Phoenix"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Phoenix Show", resp[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShows_FilterByDateRange() {
	user := suite.createTestUser()

	dates := []time.Time{
		time.Date(2026, 12, 5, 20, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 10, 20, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 20, 20, 0, 0, 0, time.UTC),
	}

	for i, d := range dates {
		req := &contracts.CreateShowRequest{
			Title:             fmt.Sprintf("Date Show %d", i),
			EventDate:         d,
			City:              "Phoenix",
			State:             "AZ",
			Venues:            []contracts.CreateShowVenue{{Name: fmt.Sprintf("Date Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:           []contracts.CreateShowArtist{{Name: fmt.Sprintf("Date Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	resp, err := suite.showService.GetShows(map[string]interface{}{
		"from_date": time.Date(2026, 12, 6, 0, 0, 0, 0, time.UTC),
		"to_date":   time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC),
	})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Date Show 1", resp[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetUserSubmissions_Success() {
	user := suite.createTestUser()
	otherUser := suite.createTestUser()

	// Create 2 shows for user
	for i := 0; i < 2; i++ {
		req := &contracts.CreateShowRequest{
			Title:             fmt.Sprintf("User Show %d", i),
			EventDate:         time.Date(2026, 12, 1+i, 20, 0, 0, 0, time.UTC),
			City:              "Phoenix",
			State:             "AZ",
			Venues:            []contracts.CreateShowVenue{{Name: fmt.Sprintf("Sub Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:           []contracts.CreateShowArtist{{Name: fmt.Sprintf("Sub Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	// Create 1 show for other user
	req := &contracts.CreateShowRequest{
		Title:             "Other User Show",
		EventDate:         time.Date(2026, 12, 3, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Other Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Other Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &otherUser.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	shows, total, err := suite.showService.GetUserSubmissions(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(2, total)
	suite.Len(shows, 2)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetPendingShows_Success() {
	user := suite.createTestUser()

	// Create 2 shows, set one to pending
	req1 := &contracts.CreateShowRequest{
		Title:             "Pending Show",
		EventDate:         time.Date(2026, 12, 1, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Pending Venue 1", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Pending Artist 1", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	show1, err := suite.showService.CreateShow(req1)
	suite.Require().NoError(err)
	suite.db.Model(&models.Show{}).Where("id = ?", show1.ID).Update("status", models.ShowStatusPending)

	req2 := &contracts.CreateShowRequest{
		Title:             "Approved Show",
		EventDate:         time.Date(2026, 12, 2, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Pending Venue 2", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Pending Artist 2", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)
	suite.Require().NoError(err)

	shows, total, err := suite.showService.GetPendingShows(10, 0, nil)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(shows, 1)
	suite.Equal("Pending Show", shows[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetRejectedShows_Success() {
	user := suite.createTestUser()

	req := &contracts.CreateShowRequest{
		Title:             "Rejected Show",
		EventDate:         time.Date(2026, 12, 1, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Reject Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Reject Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	show, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)
	suite.db.Model(&models.Show{}).Where("id = ?", show.ID).Updates(map[string]interface{}{
		"status":           models.ShowStatusRejected,
		"rejection_reason": "Spam",
	})

	shows, total, err := suite.showService.GetRejectedShows(10, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(shows, 1)
	suite.Equal("Rejected Show", shows[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetRejectedShows_WithSearch() {
	user := suite.createTestUser()

	// Create 2 rejected shows
	for i, reason := range []string{"Duplicate entry", "Spam content"} {
		req := &contracts.CreateShowRequest{
			Title:             fmt.Sprintf("Rejected %d", i),
			EventDate:         time.Date(2026, 12, 1+i, 20, 0, 0, 0, time.UTC),
			City:              "Phoenix",
			State:             "AZ",
			Venues:            []contracts.CreateShowVenue{{Name: fmt.Sprintf("Search Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:           []contracts.CreateShowArtist{{Name: fmt.Sprintf("Search Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		show, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
		suite.db.Model(&models.Show{}).Where("id = ?", show.ID).Updates(map[string]interface{}{
			"status":           models.ShowStatusRejected,
			"rejection_reason": reason,
		})
	}

	shows, total, err := suite.showService.GetRejectedShows(10, 0, "Spam")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(shows, 1)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_Pagination() {
	user := suite.createTestUser()

	// Create 5 future shows with distinct dates (no sub-second precision issues)
	baseDate := time.Date(2027, 6, 1, 20, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		req := &contracts.CreateShowRequest{
			Title:             fmt.Sprintf("Upcoming %d", i),
			EventDate:         baseDate.AddDate(0, 0, i),
			City:              "Phoenix",
			State:             "AZ",
			Venues:            []contracts.CreateShowVenue{{Name: fmt.Sprintf("Up Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:           []contracts.CreateShowArtist{{Name: fmt.Sprintf("Up Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	// Page 1: get 3
	page1, cursor1, err := suite.showService.GetUpcomingShows("UTC", "", 3, false, nil)
	suite.Require().NoError(err)
	suite.Require().Len(page1, 3)
	suite.Require().NotNil(cursor1, "should have a next cursor when more results exist")

	// Page 2: use cursor, expect remaining 2
	page2, cursor2, err := suite.showService.GetUpcomingShows("UTC", *cursor1, 3, false, nil)
	suite.Require().NoError(err)
	suite.Require().Len(page2, 2, "page 2 should have exactly the remaining 2 shows")
	suite.Nil(cursor2, "should be no more pages after page 2")

	// Verify no overlap: page 2 IDs must not appear in page 1
	page1IDs := map[uint]bool{}
	for _, s := range page1 {
		page1IDs[s.ID] = true
	}
	for _, s := range page2 {
		suite.False(page1IDs[s.ID], "show ID %d appeared on both pages", s.ID)
	}

	// Verify chronological ordering across pages
	suite.True(page2[0].EventDate.After(page1[len(page1)-1].EventDate) ||
		(page2[0].EventDate.Equal(page1[len(page1)-1].EventDate) && page2[0].ID > page1[len(page1)-1].ID),
		"page 2 first show should come strictly after page 1 last show")
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_Success() {
	user := suite.createTestUser()

	// Create shows in different cities
	cities := []struct{ city, state string }{
		{"Phoenix", "AZ"},
		{"Phoenix", "AZ"},
		{"Tucson", "AZ"},
	}
	for i, c := range cities {
		req := &contracts.CreateShowRequest{
			Title:             fmt.Sprintf("City Show %d", i),
			EventDate:         time.Now().UTC().AddDate(0, 0, i+1),
			City:              c.city,
			State:             c.state,
			Venues:            []contracts.CreateShowVenue{{Name: fmt.Sprintf("City Venue %d", i), City: c.city, State: c.state}},
			Artists:           []contracts.CreateShowArtist{{Name: fmt.Sprintf("City Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	results, err := suite.showService.GetShowCities("UTC")

	suite.Require().NoError(err)
	suite.GreaterOrEqual(len(results), 2)

	// Phoenix should have more shows
	cityMap := make(map[string]int)
	for _, r := range results {
		cityMap[r.City] = r.ShowCount
	}
	suite.Equal(2, cityMap["Phoenix"])
	suite.Equal(1, cityMap["Tucson"])
}

// =============================================================================
// Group 6: Status Flags
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestSetShowSoldOut() {
	created := suite.createTestShow()

	// Set sold out
	resp, err := suite.showService.SetShowSoldOut(created.ID, true)
	suite.Require().NoError(err)
	suite.True(resp.IsSoldOut)

	// Clear sold out
	resp, err = suite.showService.SetShowSoldOut(created.ID, false)
	suite.Require().NoError(err)
	suite.False(resp.IsSoldOut)
}

func (suite *ShowServiceIntegrationTestSuite) TestSetShowCancelled() {
	created := suite.createTestShow()

	// Set cancelled
	resp, err := suite.showService.SetShowCancelled(created.ID, true)
	suite.Require().NoError(err)
	suite.True(resp.IsCancelled)

	// Clear cancelled
	resp, err = suite.showService.SetShowCancelled(created.ID, false)
	suite.Require().NoError(err)
	suite.False(resp.IsCancelled)
}

// =============================================================================
// Group 7: ParseShowMarkdown (Pure Logic -- no DB needed)
// =============================================================================

func TestParseShowMarkdown_Valid(t *testing.T) {
	svc := &ShowService{} // nil db is fine -- ParseShowMarkdown doesn't use it
	content := []byte(`---
version: "1.0"
exported_at: "2026-01-15T12:00:00Z"
show:
  title: "Rock Night"
  event_date: "2026-07-15T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "approved"
venues:
  - name: "Valley Bar"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "The Band"
    position: 0
    set_type: "headliner"
  - name: "Opener"
    position: 1
    set_type: "opener"
---

## Description

A great rock show with two bands.
`)

	parsed, err := svc.ParseShowMarkdown(content)

	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, "Rock Night", parsed.Frontmatter.Show.Title)
	assert.Equal(t, "2026-07-15T20:00:00Z", parsed.Frontmatter.Show.EventDate)
	assert.Equal(t, "Phoenix", parsed.Frontmatter.Show.City)
	assert.Equal(t, "AZ", parsed.Frontmatter.Show.State)
	assert.Equal(t, "approved", parsed.Frontmatter.Show.Status)
	assert.Len(t, parsed.Frontmatter.Venues, 1)
	assert.Equal(t, "Valley Bar", parsed.Frontmatter.Venues[0].Name)
	assert.Len(t, parsed.Frontmatter.Artists, 2)
	assert.Equal(t, "The Band", parsed.Frontmatter.Artists[0].Name)
	assert.Equal(t, "headliner", parsed.Frontmatter.Artists[0].SetType)
	assert.Equal(t, "Opener", parsed.Frontmatter.Artists[1].Name)
	assert.Equal(t, "opener", parsed.Frontmatter.Artists[1].SetType)
	assert.Equal(t, "A great rock show with two bands.", parsed.Description)
}

func TestParseShowMarkdown_MinimalFrontmatter(t *testing.T) {
	svc := &ShowService{}
	content := []byte(`---
show:
  title: "Minimal"
  event_date: "2026-01-01T00:00:00Z"
  status: "pending"
---
`)

	parsed, err := svc.ParseShowMarkdown(content)

	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, "Minimal", parsed.Frontmatter.Show.Title)
	assert.Empty(t, parsed.Frontmatter.Venues)
	assert.Empty(t, parsed.Frontmatter.Artists)
	assert.Empty(t, parsed.Description)
}

func TestParseShowMarkdown_NoDescription(t *testing.T) {
	svc := &ShowService{}
	content := []byte(`---
show:
  title: "No Desc"
  event_date: "2026-01-01T00:00:00Z"
  status: "approved"
---

Some other content without a Description heading.
`)

	parsed, err := svc.ParseShowMarkdown(content)

	assert.NoError(t, err)
	assert.Empty(t, parsed.Description)
}

func TestParseShowMarkdown_MultilineDescription(t *testing.T) {
	svc := &ShowService{}
	content := []byte(`---
show:
  title: "Multi Desc"
  event_date: "2026-01-01T00:00:00Z"
  status: "approved"
---

## Description

First paragraph.

Second paragraph with more details.

Third paragraph.
`)

	parsed, err := svc.ParseShowMarkdown(content)

	assert.NoError(t, err)
	assert.Contains(t, parsed.Description, "First paragraph.")
	assert.Contains(t, parsed.Description, "Second paragraph with more details.")
	assert.Contains(t, parsed.Description, "Third paragraph.")
}

func TestParseShowMarkdown_InvalidYAML(t *testing.T) {
	svc := &ShowService{}
	content := []byte(`---
show:
  title: [invalid yaml
---
`)

	_, err := svc.ParseShowMarkdown(content)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse frontmatter")
}

func TestParseShowMarkdown_NoFrontmatter(t *testing.T) {
	svc := &ShowService{}
	content := []byte(`No frontmatter at all here.`)

	_, err := svc.ParseShowMarkdown(content)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing frontmatter delimiter")
}

func TestParseShowMarkdown_EmptyContent(t *testing.T) {
	svc := &ShowService{}

	_, err := svc.ParseShowMarkdown([]byte{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing frontmatter delimiter")
}

func TestParseShowMarkdown_MultipleArtistsAndVenues(t *testing.T) {
	svc := &ShowService{}
	content := []byte(`---
show:
  title: "Big Festival"
  event_date: "2026-08-01T18:00:00Z"
  status: "approved"
venues:
  - name: "Main Stage"
    city: "Phoenix"
    state: "AZ"
  - name: "Side Stage"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Headliner A"
    position: 0
    set_type: "headliner"
  - name: "Support B"
    position: 1
    set_type: "opener"
  - name: "Support C"
    position: 2
    set_type: "opener"
---

## Description

Festival description.
`)

	parsed, err := svc.ParseShowMarkdown(content)

	assert.NoError(t, err)
	assert.Len(t, parsed.Frontmatter.Venues, 2)
	assert.Equal(t, "Main Stage", parsed.Frontmatter.Venues[0].Name)
	assert.Equal(t, "Side Stage", parsed.Frontmatter.Venues[1].Name)
	assert.Len(t, parsed.Frontmatter.Artists, 3)
	assert.Equal(t, 0, parsed.Frontmatter.Artists[0].Position)
	assert.Equal(t, 1, parsed.Frontmatter.Artists[1].Position)
	assert.Equal(t, 2, parsed.Frontmatter.Artists[2].Position)
}

func TestParseShowMarkdown_DescriptionStopsAtNextHeading(t *testing.T) {
	svc := &ShowService{}
	content := []byte(`---
show:
  title: "Heading Test"
  event_date: "2026-01-01T00:00:00Z"
  status: "approved"
---

## Description

Only this part.

## Other Section

This should not be included.
`)

	parsed, err := svc.ParseShowMarkdown(content)

	assert.NoError(t, err)
	assert.Equal(t, "Only this part.", parsed.Description)
	assert.NotContains(t, parsed.Description, "This should not be included")
}

// =============================================================================
// Group 8: ExportShowToMarkdown (DB required)
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestExportShowToMarkdown_Success() {
	created := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Export Test Show"
		req.Description = "A great test show"
		price := 25.0
		req.Price = &price
	})

	data, filename, err := suite.showService.ExportShowToMarkdown(created.ID)

	suite.Require().NoError(err)
	suite.NotEmpty(data)
	suite.NotEmpty(filename)

	// Verify markdown structure
	content := string(data)
	suite.True(strings.HasPrefix(content, "---\n"), "should start with frontmatter delimiter")
	suite.Contains(content, "Export Test Show")
	suite.Contains(content, "## Description")
	suite.Contains(content, "A great test show")
}

func (suite *ShowServiceIntegrationTestSuite) TestExportShowToMarkdown_NotFound() {
	_, _, err := suite.showService.ExportShowToMarkdown(99999)

	suite.Require().Error(err)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowNotFound, showErr.Code)
}

func (suite *ShowServiceIntegrationTestSuite) TestExportShowToMarkdown_RoundTrip() {
	created := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Round Trip Show"
		req.Description = "Round trip description"
		req.City = "Tempe"
		req.State = "AZ"
	})

	// Export
	data, _, err := suite.showService.ExportShowToMarkdown(created.ID)
	suite.Require().NoError(err)

	// Parse back
	parsed, err := suite.showService.ParseShowMarkdown(data)
	suite.Require().NoError(err)

	suite.Equal("Round Trip Show", parsed.Frontmatter.Show.Title)
	suite.Equal("Tempe", parsed.Frontmatter.Show.City)
	suite.Equal("AZ", parsed.Frontmatter.Show.State)
	suite.Equal("approved", parsed.Frontmatter.Show.Status)
	suite.Equal("Round trip description", parsed.Description)
	suite.Len(parsed.Frontmatter.Venues, 1)
	suite.Len(parsed.Frontmatter.Artists, 1)
}

func (suite *ShowServiceIntegrationTestSuite) TestExportShowToMarkdown_FilenameFormat() {
	created := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Cool Show 2026"
		req.EventDate = time.Date(2026, 3, 15, 20, 0, 0, 0, time.UTC)
	})

	_, filename, err := suite.showService.ExportShowToMarkdown(created.ID)

	suite.Require().NoError(err)
	suite.True(strings.HasPrefix(filename, "show-2026-03-15-"), "filename should start with show-date prefix")
	suite.True(strings.HasSuffix(filename, ".md"), "filename should end with .md")
	suite.Contains(filename, "cool-show-2026")
}

// =============================================================================
// Group 9: GetAdminShows (DB required)
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestGetAdminShows_NoFilters() {
	user := suite.createTestUser()
	for i := 0; i < 3; i++ {
		req := &contracts.CreateShowRequest{
			Title:             fmt.Sprintf("Admin Show %d", i),
			EventDate:         time.Date(2026, 12, 1+i, 20, 0, 0, 0, time.UTC),
			City:              "Phoenix",
			State:             "AZ",
			Venues:            []contracts.CreateShowVenue{{Name: fmt.Sprintf("Admin Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:           []contracts.CreateShowArtist{{Name: fmt.Sprintf("Admin Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	shows, total, err := suite.showService.GetAdminShows(10, 0, contracts.AdminShowFilters{})

	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(shows, 3)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetAdminShows_StatusFilter() {
	user := suite.createTestUser()

	// Create 2 approved shows
	for i := 0; i < 2; i++ {
		req := &contracts.CreateShowRequest{
			Title:             fmt.Sprintf("Approved Admin %d", i),
			EventDate:         time.Date(2026, 12, 10+i, 20, 0, 0, 0, time.UTC),
			City:              "Phoenix",
			State:             "AZ",
			Venues:            []contracts.CreateShowVenue{{Name: fmt.Sprintf("StatusF Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:           []contracts.CreateShowArtist{{Name: fmt.Sprintf("StatusF Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	// Create 1 pending show
	req := &contracts.CreateShowRequest{
		Title:             "Pending Admin Show",
		EventDate:         time.Date(2026, 12, 15, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "StatusF Venue P", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "StatusF Artist P", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	pendingShow, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)
	suite.db.Model(&models.Show{}).Where("id = ?", pendingShow.ID).Update("status", models.ShowStatusPending)

	// Filter by pending
	shows, total, err := suite.showService.GetAdminShows(10, 0, contracts.AdminShowFilters{Status: "pending"})

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(shows, 1)
	suite.Equal("Pending Admin Show", shows[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetAdminShows_Pagination() {
	user := suite.createTestUser()
	for i := 0; i < 5; i++ {
		req := &contracts.CreateShowRequest{
			Title:             fmt.Sprintf("Page Show %d", i),
			EventDate:         time.Date(2026, 12, 20+i, 20, 0, 0, 0, time.UTC),
			City:              "Phoenix",
			State:             "AZ",
			Venues:            []contracts.CreateShowVenue{{Name: fmt.Sprintf("Page Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:           []contracts.CreateShowArtist{{Name: fmt.Sprintf("Page Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	// Get page with limit=2, offset=2
	shows, total, err := suite.showService.GetAdminShows(2, 2, contracts.AdminShowFilters{})

	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(shows, 2)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetAdminShows_CityFilter() {
	user := suite.createTestUser()

	// Phoenix show
	req1 := &contracts.CreateShowRequest{
		Title:             "PHX Admin Show",
		EventDate:         time.Date(2027, 1, 1, 20, 0, 0, 0, time.UTC),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "PHX Admin Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "PHX Admin Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req1)
	suite.Require().NoError(err)

	// Tucson show
	req2 := &contracts.CreateShowRequest{
		Title:             "TUC Admin Show",
		EventDate:         time.Date(2027, 1, 2, 20, 0, 0, 0, time.UTC),
		City:              "Tucson",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "TUC Admin Venue", City: "Tucson", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "TUC Admin Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)
	suite.Require().NoError(err)

	shows, total, err := suite.showService.GetAdminShows(10, 0, contracts.AdminShowFilters{City: "Tucson"})

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(shows, 1)
	suite.Equal("TUC Admin Show", shows[0].Title)
}

// =============================================================================
// Group 10: Batch Approve / Reject
// =============================================================================

// createPendingShow creates a show and sets it to pending status, returning the show ID.
func (suite *ShowServiceIntegrationTestSuite) createPendingShow(title string, dayOffset int) uint {
	user := suite.createTestUser()
	req := &contracts.CreateShowRequest{
		Title:     title,
		EventDate: time.Date(2026, 6, 15+dayOffset, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: fmt.Sprintf("Batch Venue %s", title), City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: fmt.Sprintf("Batch Artist %s", title), IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	resp, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)
	// Force status to pending so BatchApprove/Reject can operate on it
	suite.db.Model(&models.Show{}).Where("id = ?", resp.ID).Update("status", models.ShowStatusPending)
	return resp.ID
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchApproveShows_Success() {
	id1 := suite.createPendingShow("Batch Approve 1", 0)
	id2 := suite.createPendingShow("Batch Approve 2", 1)
	id3 := suite.createPendingShow("Batch Approve 3", 2)

	result, err := suite.showService.BatchApproveShows([]uint{id1, id2, id3})

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Len(result.Succeeded, 3)
	suite.Empty(result.Errors)
	suite.ElementsMatch([]uint{id1, id2, id3}, result.Succeeded)

	// Verify all shows are now approved in the DB
	for _, id := range []uint{id1, id2, id3} {
		var show models.Show
		suite.Require().NoError(suite.db.First(&show, id).Error)
		suite.Equal(models.ShowStatusApproved, show.Status, "show %d should be approved", id)
	}
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchApproveShows_AlreadyApproved_GracefulError() {
	// Create a show that is already approved (default via admin submission)
	approvedShow := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Already Approved Batch"
		req.EventDate = time.Date(2026, 7, 1, 20, 0, 0, 0, time.UTC)
	})
	// Also create a pending show
	pendingID := suite.createPendingShow("Pending For Batch", 10)

	result, err := suite.showService.BatchApproveShows([]uint{approvedShow.ID, pendingID})

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	// The pending show should succeed
	suite.Contains(result.Succeeded, pendingID)
	// The already-approved show should produce an error entry
	suite.Require().Len(result.Errors, 1)
	suite.Equal(approvedShow.ID, result.Errors[0].ShowID)
	suite.Contains(result.Errors[0].Error, "cannot be approved")
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchApproveShows_InvalidIDs() {
	result, err := suite.showService.BatchApproveShows([]uint{999990, 999991})

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Empty(result.Succeeded)
	suite.Len(result.Errors, 2)
	for _, e := range result.Errors {
		suite.Contains(e.Error, "not found")
	}
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchApproveShows_EmptyList() {
	result, err := suite.showService.BatchApproveShows([]uint{})

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Empty(result.Succeeded)
	suite.Empty(result.Errors)
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchApproveShows_MixedValidAndInvalid() {
	pendingID := suite.createPendingShow("Mixed Valid", 20)

	result, err := suite.showService.BatchApproveShows([]uint{pendingID, 999992})

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Len(result.Succeeded, 1)
	suite.Equal(pendingID, result.Succeeded[0])
	suite.Len(result.Errors, 1)
	suite.Equal(uint(999992), result.Errors[0].ShowID)
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchRejectShows_Success() {
	id1 := suite.createPendingShow("Batch Reject 1", 0)
	id2 := suite.createPendingShow("Batch Reject 2", 1)

	result, err := suite.showService.BatchRejectShows([]uint{id1, id2}, "Duplicate listing", "duplicate")

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Len(result.Succeeded, 2)
	suite.Empty(result.Errors)
	suite.ElementsMatch([]uint{id1, id2}, result.Succeeded)

	// Verify statuses and rejection reasons in DB
	for _, id := range []uint{id1, id2} {
		var show models.Show
		suite.Require().NoError(suite.db.First(&show, id).Error)
		suite.Equal(models.ShowStatusRejected, show.Status, "show %d should be rejected", id)
		suite.Require().NotNil(show.RejectionReason)
		suite.Equal("Duplicate listing", *show.RejectionReason)
		suite.Require().NotNil(show.RejectionCategory)
		suite.Equal("duplicate", *show.RejectionCategory)
	}
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchRejectShows_WithReason_NoCategory() {
	id := suite.createPendingShow("Reject No Cat", 0)

	result, err := suite.showService.BatchRejectShows([]uint{id}, "Some reason", "")

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Len(result.Succeeded, 1)
	suite.Empty(result.Errors)

	// Verify status and reason in DB, but category should be nil/empty
	var show models.Show
	suite.Require().NoError(suite.db.First(&show, id).Error)
	suite.Equal(models.ShowStatusRejected, show.Status)
	suite.Require().NotNil(show.RejectionReason)
	suite.Equal("Some reason", *show.RejectionReason)
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchRejectShows_NotPending_Fails() {
	// Create an already-approved show
	approvedShow := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Approved For Reject Batch"
		req.EventDate = time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	})

	result, err := suite.showService.BatchRejectShows([]uint{approvedShow.ID}, "reason", "non_music")

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Empty(result.Succeeded)
	suite.Len(result.Errors, 1)
	suite.Equal(approvedShow.ID, result.Errors[0].ShowID)
	suite.Contains(result.Errors[0].Error, "not pending")
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchRejectShows_InvalidIDs() {
	result, err := suite.showService.BatchRejectShows([]uint{999993, 999994}, "reason", "bad_data")

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Empty(result.Succeeded)
	suite.Len(result.Errors, 2)
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchRejectShows_EmptyList() {
	result, err := suite.showService.BatchRejectShows([]uint{}, "reason", "non_music")

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Empty(result.Succeeded)
	suite.Empty(result.Errors)
}

func (suite *ShowServiceIntegrationTestSuite) TestBatchRejectShows_MixedValidAndInvalid() {
	pendingID := suite.createPendingShow("Mixed Reject", 25)

	result, err := suite.showService.BatchRejectShows([]uint{pendingID, 999995}, "bad data", "bad_data")

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Len(result.Succeeded, 1)
	suite.Equal(pendingID, result.Succeeded[0])
	suite.Len(result.Errors, 1)
	suite.Equal(uint(999995), result.Errors[0].ShowID)
}

// =============================================================================
// Group 11: PreviewShowImport (DB required for venue/artist matching)
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestPreviewShowImport_NewEntities() {
	content := []byte(`---
show:
  title: "Preview New Show"
  event_date: "2026-08-15T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Brand New Preview Venue"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Brand New Preview Artist"
    position: 0
    set_type: "headliner"
---

## Description

Preview test description.
`)

	resp, err := suite.showService.PreviewShowImport(content)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.True(resp.CanImport)
	suite.Equal("Preview New Show", resp.Show.Title)
	suite.Equal("2026-08-15T20:00:00Z", resp.Show.EventDate)

	// Venue should be new (will create)
	suite.Require().Len(resp.Venues, 1)
	suite.True(resp.Venues[0].WillCreate)
	suite.Nil(resp.Venues[0].ExistingID)
	suite.Equal("Brand New Preview Venue", resp.Venues[0].Name)

	// Artist should be new (will create)
	suite.Require().Len(resp.Artists, 1)
	suite.True(resp.Artists[0].WillCreate)
	suite.Nil(resp.Artists[0].ExistingID)
	suite.Equal("Brand New Preview Artist", resp.Artists[0].Name)
	suite.Equal("headliner", resp.Artists[0].SetType)

	suite.Empty(resp.Warnings)
}

func (suite *ShowServiceIntegrationTestSuite) TestPreviewShowImport_ExistingEntities() {
	// Pre-create a venue and artist
	venue := suite.createTestVenue("Existing Preview Venue", "Phoenix", "AZ", true)
	artist := &models.Artist{Name: "Existing Preview Artist"}
	suite.Require().NoError(suite.db.Create(artist).Error)

	content := []byte(`---
show:
  title: "Preview Existing Show"
  event_date: "2026-09-15T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Existing Preview Venue"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Existing Preview Artist"
    position: 0
    set_type: "headliner"
---
`)

	resp, err := suite.showService.PreviewShowImport(content)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.True(resp.CanImport)

	// Venue should match existing
	suite.Require().Len(resp.Venues, 1)
	suite.False(resp.Venues[0].WillCreate)
	suite.Require().NotNil(resp.Venues[0].ExistingID)
	suite.Equal(venue.ID, *resp.Venues[0].ExistingID)

	// Artist should match existing
	suite.Require().Len(resp.Artists, 1)
	suite.False(resp.Artists[0].WillCreate)
	suite.Require().NotNil(resp.Artists[0].ExistingID)
	suite.Equal(artist.ID, *resp.Artists[0].ExistingID)
}

func (suite *ShowServiceIntegrationTestSuite) TestPreviewShowImport_MixedNewAndExisting() {
	// Pre-create one artist but not the other
	existingArtist := &models.Artist{Name: "Known Artist"}
	suite.Require().NoError(suite.db.Create(existingArtist).Error)

	content := []byte(`---
show:
  title: "Mixed Preview Show"
  event_date: "2026-10-01T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Mixed Preview Venue"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Known Artist"
    position: 0
    set_type: "headliner"
  - name: "Unknown Artist"
    position: 1
    set_type: "opener"
---
`)

	resp, err := suite.showService.PreviewShowImport(content)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.True(resp.CanImport)

	suite.Require().Len(resp.Artists, 2)
	// First artist matches existing
	suite.False(resp.Artists[0].WillCreate)
	suite.Require().NotNil(resp.Artists[0].ExistingID)
	suite.Equal(existingArtist.ID, *resp.Artists[0].ExistingID)
	// Second artist is new
	suite.True(resp.Artists[1].WillCreate)
	suite.Nil(resp.Artists[1].ExistingID)
}

func (suite *ShowServiceIntegrationTestSuite) TestPreviewShowImport_DuplicateWarning() {
	// Create an existing show at a venue with a headliner
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	req := &contracts.CreateShowRequest{
		Title:             "Original Show",
		EventDate:         eventDate,
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Dup Preview Venue", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Dup Preview Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Preview an import with the same headliner at the same venue on the same date
	content := []byte(`---
show:
  title: "Duplicate Import"
  event_date: "2026-11-20T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Dup Preview Venue"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Dup Preview Headliner"
    position: 0
    set_type: "headliner"
---
`)

	resp, err := suite.showService.PreviewShowImport(content)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	// Should still be importable but with a warning
	suite.True(resp.CanImport)
	suite.Require().NotEmpty(resp.Warnings)
	found := false
	for _, w := range resp.Warnings {
		if strings.Contains(w, "already has a show") {
			found = true
			break
		}
	}
	suite.True(found, "expected duplicate headliner warning, got: %v", resp.Warnings)
}

func (suite *ShowServiceIntegrationTestSuite) TestPreviewShowImport_MissingEventDate() {
	content := []byte(`---
show:
  title: "No Date Show"
  status: "pending"
venues:
  - name: "Some Venue"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Some Artist"
    position: 0
    set_type: "headliner"
---
`)

	resp, err := suite.showService.PreviewShowImport(content)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.False(resp.CanImport)
	suite.Contains(resp.Warnings, "Missing event date")
}

func (suite *ShowServiceIntegrationTestSuite) TestPreviewShowImport_MissingVenues() {
	content := []byte(`---
show:
  title: "No Venues Show"
  event_date: "2026-08-01T20:00:00Z"
  status: "pending"
artists:
  - name: "Some Artist"
    position: 0
    set_type: "headliner"
---
`)

	resp, err := suite.showService.PreviewShowImport(content)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.False(resp.CanImport)
	suite.Contains(resp.Warnings, "No venues specified")
}

func (suite *ShowServiceIntegrationTestSuite) TestPreviewShowImport_MissingArtists() {
	content := []byte(`---
show:
  title: "No Artists Show"
  event_date: "2026-08-01T20:00:00Z"
  status: "pending"
venues:
  - name: "Some Venue"
    city: "Phoenix"
    state: "AZ"
---
`)

	resp, err := suite.showService.PreviewShowImport(content)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.False(resp.CanImport)
	suite.Contains(resp.Warnings, "No artists specified")
}

// =============================================================================
// Group 12: ConfirmShowImport (DB required)
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestConfirmShowImport_Success_AsAdmin() {
	content := []byte(`---
show:
  title: "Import Admin Show"
  event_date: "2026-09-20T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Import Venue Admin"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Import Artist Admin"
    position: 0
    set_type: "headliner"
  - name: "Import Opener Admin"
    position: 1
    set_type: "opener"
---

## Description

An imported show description.
`)

	resp, err := suite.showService.ConfirmShowImport(content, true)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Import Admin Show", resp.Title)
	suite.Equal("approved", resp.Status) // admin import auto-approves
	suite.NotEmpty(resp.Slug)
	suite.Require().Len(resp.Venues, 1)
	suite.Equal("Import Venue Admin", resp.Venues[0].Name)
	suite.Require().Len(resp.Artists, 2)
	suite.Equal("Import Artist Admin", resp.Artists[0].Name)
	suite.True(*resp.Artists[0].IsHeadliner)
	suite.Equal("Import Opener Admin", resp.Artists[1].Name)
	suite.False(*resp.Artists[1].IsHeadliner)

	// Verify the show exists in the database
	var show models.Show
	suite.Require().NoError(suite.db.First(&show, resp.ID).Error)
	suite.Equal("Import Admin Show", show.Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestConfirmShowImport_Success_AsNonAdmin() {
	content := []byte(`---
show:
  title: "Import User Show"
  event_date: "2026-10-20T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Import Venue User"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Import Artist User"
    position: 0
    set_type: "headliner"
---
`)

	resp, err := suite.showService.ConfirmShowImport(content, false)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Import User Show", resp.Title)
	// All non-private shows are approved (unverified venues show city-only on frontend)
	suite.Equal("approved", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestConfirmShowImport_LinksExistingVenue() {
	// Pre-create a venue
	existing := suite.createTestVenue("Pre-existing Import Venue", "Phoenix", "AZ", true)

	content := []byte(`---
show:
  title: "Import Existing Venue Show"
  event_date: "2026-11-05T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Pre-existing Import Venue"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Import Link Artist"
    position: 0
    set_type: "headliner"
---
`)

	resp, err := suite.showService.ConfirmShowImport(content, true)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Venues, 1)
	suite.Equal(existing.ID, resp.Venues[0].ID, "should link to existing venue, not create a new one")

	// Verify no duplicate venue was created
	var venueCount int64
	suite.db.Model(&models.Venue{}).Where("LOWER(name) = ? AND LOWER(city) = ?", "pre-existing import venue", "phoenix").Count(&venueCount)
	suite.Equal(int64(1), venueCount)
}

func (suite *ShowServiceIntegrationTestSuite) TestConfirmShowImport_LinksExistingArtist() {
	// Pre-create an artist
	existingArtist := &models.Artist{Name: "Pre-existing Import Artist"}
	suite.Require().NoError(suite.db.Create(existingArtist).Error)

	content := []byte(`---
show:
  title: "Import Existing Artist Show"
  event_date: "2026-11-10T20:00:00Z"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Import Artist Link Venue"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Pre-existing Import Artist"
    position: 0
    set_type: "headliner"
---
`)

	resp, err := suite.showService.ConfirmShowImport(content, true)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Artists, 1)
	suite.Equal(existingArtist.ID, resp.Artists[0].ID, "should link to existing artist, not create a new one")
}

func (suite *ShowServiceIntegrationTestSuite) TestConfirmShowImport_InvalidEventDate() {
	content := []byte(`---
show:
  title: "Bad Date Show"
  event_date: "not-a-date"
  city: "Phoenix"
  state: "AZ"
  status: "pending"
venues:
  - name: "Bad Date Venue"
    city: "Phoenix"
    state: "AZ"
artists:
  - name: "Bad Date Artist"
    position: 0
    set_type: "headliner"
---
`)

	resp, err := suite.showService.ConfirmShowImport(content, true)

	suite.Require().Error(err)
	suite.Nil(resp)
	suite.Contains(err.Error(), "invalid event date")
}

func (suite *ShowServiceIntegrationTestSuite) TestConfirmShowImport_InvalidMarkdown() {
	content := []byte(`not valid markdown frontmatter at all`)

	resp, err := suite.showService.ConfirmShowImport(content, true)

	suite.Require().Error(err)
	suite.Nil(resp)
}

func (suite *ShowServiceIntegrationTestSuite) TestConfirmShowImport_CreatesVenueAndArtist() {
	content := []byte(`---
show:
  title: "Full Create Import"
  event_date: "2026-12-01T20:00:00Z"
  city: "Tempe"
  state: "AZ"
  status: "pending"
venues:
  - name: "Freshly Created Venue"
    city: "Tempe"
    state: "AZ"
    address: "123 Main St"
artists:
  - name: "Freshly Created Headliner"
    position: 0
    set_type: "headliner"
  - name: "Freshly Created Opener"
    position: 1
    set_type: "opener"
---

## Description

A show with all new entities.
`)

	resp, err := suite.showService.ConfirmShowImport(content, true)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal("Full Create Import", resp.Title)
	suite.Equal("approved", resp.Status)

	// Verify venue was created in DB
	var venue models.Venue
	suite.Require().NoError(suite.db.Where("name = ?", "Freshly Created Venue").First(&venue).Error)
	suite.Equal("Tempe", venue.City)

	// Verify artists were created in DB
	var headliner models.Artist
	suite.Require().NoError(suite.db.Where("name = ?", "Freshly Created Headliner").First(&headliner).Error)
	suite.NotZero(headliner.ID)

	var opener models.Artist
	suite.Require().NoError(suite.db.Where("name = ?", "Freshly Created Opener").First(&opener).Error)
	suite.NotZero(opener.ID)
}
