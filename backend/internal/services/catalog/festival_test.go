package catalog

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type FestivalServiceIntegrationTestSuite struct {
	suite.Suite
	testDB          *testutil.TestDatabase
	db              *gorm.DB
	festivalService *FestivalService
	artistService   *ArtistService
	venueService    *VenueService
}

func (suite *FestivalServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.festivalService = &FestivalService{db: suite.testDB.DB}
	suite.artistService = &ArtistService{db: suite.testDB.DB}
	suite.venueService = &VenueService{db: suite.testDB.DB}
}

func (suite *FestivalServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

// TearDownTest cleans up data between tests for isolation
func (suite *FestivalServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func TestFestivalServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(FestivalServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) createTestArtistForFestival(name string) *models.Artist {
	artist := &models.Artist{
		Name: name,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *FestivalServiceIntegrationTestSuite) createTestVenue(name, city, state string) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: true,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

var festivalCounter int

func (suite *FestivalServiceIntegrationTestSuite) createBasicFestival(name string) *contracts.FestivalDetailResponse {
	festivalCounter++
	city := "Phoenix"
	state := "AZ"
	slug := utils.GenerateArtistSlug(name)
	req := &contracts.CreateFestivalRequest{
		Name:        name,
		SeriesSlug:  slug,
		EditionYear: 2026 + festivalCounter,
		City:        &city,
		State:       &state,
		StartDate:   "2026-03-06",
		EndDate:     "2026-03-08",
	}
	resp, err := suite.festivalService.CreateFestival(req)
	suite.Require().NoError(err)
	return resp
}

func strPtrFestival(s string) *string {
	return &s
}

// =============================================================================
// Group 1: CreateFestival
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestCreateFestival_Success() {
	city := "Phoenix"
	state := "AZ"
	req := &contracts.CreateFestivalRequest{
		Name:        "M3F Festival",
		SeriesSlug:  "m3f",
		EditionYear: 2026,
		City:        &city,
		State:       &state,
		StartDate:   "2026-03-06",
		EndDate:     "2026-03-08",
	}

	resp, err := suite.festivalService.CreateFestival(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("M3F Festival", resp.Name)
	suite.Equal("m3f-festival", resp.Slug)
	suite.Equal("m3f", resp.SeriesSlug)
	suite.Equal(2026, resp.EditionYear)
	suite.Equal("Phoenix", *resp.City)
	suite.Equal("AZ", *resp.State)
	suite.Equal("2026-03-06", resp.StartDate)
	suite.Equal("2026-03-08", resp.EndDate)
	suite.Equal("announced", resp.Status)
	suite.Equal(0, resp.ArtistCount)
	suite.Equal(0, resp.VenueCount)
}

func (suite *FestivalServiceIntegrationTestSuite) TestCreateFestival_DefaultStatus() {
	req := &contracts.CreateFestivalRequest{
		Name:        "Default Status Fest",
		SeriesSlug:  "default-fest",
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
	}

	resp, err := suite.festivalService.CreateFestival(req)

	suite.Require().NoError(err)
	suite.Equal("announced", resp.Status)
}

func (suite *FestivalServiceIntegrationTestSuite) TestCreateFestival_CustomStatus() {
	req := &contracts.CreateFestivalRequest{
		Name:        "Confirmed Fest",
		SeriesSlug:  "confirmed-fest",
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
		Status:      "confirmed",
	}

	resp, err := suite.festivalService.CreateFestival(req)

	suite.Require().NoError(err)
	suite.Equal("confirmed", resp.Status)
}

func (suite *FestivalServiceIntegrationTestSuite) TestCreateFestival_UniqueSlug() {
	req1 := &contracts.CreateFestivalRequest{
		Name:        "Lollapalooza",
		SeriesSlug:  "lollapalooza",
		EditionYear: 2025,
		StartDate:   "2025-08-01",
		EndDate:     "2025-08-04",
	}
	resp1, err := suite.festivalService.CreateFestival(req1)
	suite.Require().NoError(err)

	req2 := &contracts.CreateFestivalRequest{
		Name:        "Lollapalooza",
		SeriesSlug:  "lollapalooza",
		EditionYear: 2026,
		StartDate:   "2026-08-01",
		EndDate:     "2026-08-04",
	}
	resp2, err := suite.festivalService.CreateFestival(req2)
	suite.Require().NoError(err)

	suite.NotEqual(resp1.Slug, resp2.Slug)
	suite.Equal("lollapalooza", resp1.Slug)
	suite.Equal("lollapalooza-2", resp2.Slug)
}

// =============================================================================
// Group 2: GetFestival / GetFestivalBySlug
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestival_Success() {
	created := suite.createBasicFestival("Get Test Festival")

	resp, err := suite.festivalService.GetFestival(created.ID)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("Get Test Festival", resp.Name)
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestival_NotFound() {
	resp, err := suite.festivalService.GetFestival(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var festivalErr *apperrors.FestivalError
	suite.ErrorAs(err, &festivalErr)
	suite.Equal(apperrors.CodeFestivalNotFound, festivalErr.Code)
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalBySlug_Success() {
	created := suite.createBasicFestival("Slug Test Festival")

	resp, err := suite.festivalService.GetFestivalBySlug(created.Slug)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalBySlug_NotFound() {
	resp, err := suite.festivalService.GetFestivalBySlug("nonexistent-slug-xyz")

	suite.Require().Error(err)
	suite.Nil(resp)
	var festivalErr *apperrors.FestivalError
	suite.ErrorAs(err, &festivalErr)
	suite.Equal(apperrors.CodeFestivalNotFound, festivalErr.Code)
}

// =============================================================================
// Group 3: ListFestivals
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestListFestivals_All() {
	suite.createBasicFestival("Festival A")
	suite.createBasicFestival("Festival B")
	suite.createBasicFestival("Festival C")

	resp, err := suite.festivalService.ListFestivals(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 3)
}

func (suite *FestivalServiceIntegrationTestSuite) TestListFestivals_FilterByStatus() {
	// Create a confirmed festival
	city := "Phoenix"
	state := "AZ"
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "Confirmed Fest", SeriesSlug: "cf", EditionYear: 2026,
		City: &city, State: &state, StartDate: "2026-03-01", EndDate: "2026-03-03", Status: "confirmed",
	})
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "Announced Fest", SeriesSlug: "af", EditionYear: 2026,
		StartDate: "2026-04-01", EndDate: "2026-04-03",
	})

	resp, err := suite.festivalService.ListFestivals(map[string]interface{}{"status": "confirmed"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Confirmed Fest", resp[0].Name)
}

func (suite *FestivalServiceIntegrationTestSuite) TestListFestivals_FilterByYear() {
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "2025 Fest", SeriesSlug: "f25", EditionYear: 2025, StartDate: "2025-06-01", EndDate: "2025-06-03",
	})
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "2026 Fest", SeriesSlug: "f26", EditionYear: 2026, StartDate: "2026-06-01", EndDate: "2026-06-03",
	})

	resp, err := suite.festivalService.ListFestivals(map[string]interface{}{"year": 2026})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("2026 Fest", resp[0].Name)
}

func (suite *FestivalServiceIntegrationTestSuite) TestListFestivals_FilterByCity() {
	city1 := "Phoenix"
	city2 := "Chicago"
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "PHX Fest", SeriesSlug: "phx", EditionYear: 2026, City: &city1, StartDate: "2026-03-01", EndDate: "2026-03-03",
	})
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "CHI Fest", SeriesSlug: "chi", EditionYear: 2026, City: &city2, StartDate: "2026-07-01", EndDate: "2026-07-03",
	})

	resp, err := suite.festivalService.ListFestivals(map[string]interface{}{"city": "Phoenix"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("PHX Fest", resp[0].Name)
}

func (suite *FestivalServiceIntegrationTestSuite) TestListFestivals_FilterBySeriesSlug() {
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "M3F 2025", SeriesSlug: "m3f", EditionYear: 2025, StartDate: "2025-03-01", EndDate: "2025-03-03",
	})
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "M3F 2026", SeriesSlug: "m3f", EditionYear: 2026, StartDate: "2026-03-01", EndDate: "2026-03-03",
	})
	suite.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "Other Fest", SeriesSlug: "other", EditionYear: 2026, StartDate: "2026-06-01", EndDate: "2026-06-03",
	})

	resp, err := suite.festivalService.ListFestivals(map[string]interface{}{"series_slug": "m3f"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
}

// =============================================================================
// Group 4: UpdateFestival
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestUpdateFestival_BasicFields() {
	created := suite.createBasicFestival("Original Festival")

	newName := "Updated Festival"
	newStatus := "confirmed"
	resp, err := suite.festivalService.UpdateFestival(created.ID, &contracts.UpdateFestivalRequest{
		Name:   &newName,
		Status: &newStatus,
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Festival", resp.Name)
	suite.Equal("confirmed", resp.Status)
}

func (suite *FestivalServiceIntegrationTestSuite) TestUpdateFestival_NameChangeRegeneratesSlug() {
	created := suite.createBasicFestival("Old Festival Name")
	oldSlug := created.Slug

	newName := "New Festival Name"
	resp, err := suite.festivalService.UpdateFestival(created.ID, &contracts.UpdateFestivalRequest{
		Name: &newName,
	})

	suite.Require().NoError(err)
	suite.Equal("New Festival Name", resp.Name)
	suite.NotEqual(oldSlug, resp.Slug)
	suite.Equal("new-festival-name", resp.Slug)
}

func (suite *FestivalServiceIntegrationTestSuite) TestUpdateFestival_NotFound() {
	newName := "Anything"
	resp, err := suite.festivalService.UpdateFestival(99999, &contracts.UpdateFestivalRequest{Name: &newName})

	suite.Require().Error(err)
	suite.Nil(resp)
	var festivalErr *apperrors.FestivalError
	suite.ErrorAs(err, &festivalErr)
	suite.Equal(apperrors.CodeFestivalNotFound, festivalErr.Code)
}

func (suite *FestivalServiceIntegrationTestSuite) TestUpdateFestival_NoChanges() {
	created := suite.createBasicFestival("Stable Festival")

	resp, err := suite.festivalService.UpdateFestival(created.ID, &contracts.UpdateFestivalRequest{})

	suite.Require().NoError(err)
	suite.Equal("Stable Festival", resp.Name)
}

// =============================================================================
// Group 5: DeleteFestival
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestDeleteFestival_Success() {
	created := suite.createBasicFestival("Delete Me Festival")

	err := suite.festivalService.DeleteFestival(created.ID)

	suite.Require().NoError(err)

	// Verify it's gone
	_, err = suite.festivalService.GetFestival(created.ID)
	suite.Error(err)
}

func (suite *FestivalServiceIntegrationTestSuite) TestDeleteFestival_NotFound() {
	err := suite.festivalService.DeleteFestival(99999)

	suite.Require().Error(err)
	var festivalErr *apperrors.FestivalError
	suite.ErrorAs(err, &festivalErr)
	suite.Equal(apperrors.CodeFestivalNotFound, festivalErr.Code)
}

func (suite *FestivalServiceIntegrationTestSuite) TestDeleteFestival_CascadesJunctions() {
	created := suite.createBasicFestival("Cascade Festival")
	artist := suite.createTestArtistForFestival("Cascade Artist")
	venue := suite.createTestVenue("Cascade Venue", "Phoenix", "AZ")

	suite.festivalService.AddFestivalArtist(created.ID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artist.ID,
		BillingTier: "headliner",
	})
	suite.festivalService.AddFestivalVenue(created.ID, &contracts.AddFestivalVenueRequest{
		VenueID:   venue.ID,
		IsPrimary: true,
	})

	err := suite.festivalService.DeleteFestival(created.ID)
	suite.Require().NoError(err)

	// Verify junction records cleaned up
	var faCount int64
	suite.db.Model(&models.FestivalArtist{}).Where("festival_id = ?", created.ID).Count(&faCount)
	suite.Equal(int64(0), faCount)

	var fvCount int64
	suite.db.Model(&models.FestivalVenue{}).Where("festival_id = ?", created.ID).Count(&fvCount)
	suite.Equal(int64(0), fvCount)
}

// =============================================================================
// Group 6: Festival Artists (Lineup Management)
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalArtists_Empty() {
	created := suite.createBasicFestival("Empty Lineup Festival")

	resp, err := suite.festivalService.GetFestivalArtists(created.ID, nil)

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalArtists_FestivalNotFound() {
	resp, err := suite.festivalService.GetFestivalArtists(99999, nil)

	suite.Require().Error(err)
	suite.Nil(resp)
	var festivalErr *apperrors.FestivalError
	suite.ErrorAs(err, &festivalErr)
}

func (suite *FestivalServiceIntegrationTestSuite) TestAddFestivalArtist_Success() {
	festival := suite.createBasicFestival("Lineup Festival")
	artist := suite.createTestArtistForFestival("Headliner Band")

	resp, err := suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artist.ID,
		BillingTier: "headliner",
		Position:    0,
		DayDate:     strPtrFestival("2026-03-07"),
		Stage:       strPtrFestival("Main Stage"),
	})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal(artist.ID, resp.ArtistID)
	suite.Equal("Headliner Band", resp.ArtistName)
	suite.Equal("headliner", resp.BillingTier)
	suite.Equal(0, resp.Position)
	suite.Equal("2026-03-07", *resp.DayDate)
	suite.Equal("Main Stage", *resp.Stage)
}

func (suite *FestivalServiceIntegrationTestSuite) TestAddFestivalArtist_DefaultBillingTier() {
	festival := suite.createBasicFestival("Default Tier Festival")
	artist := suite.createTestArtistForFestival("Default Tier Band")

	resp, err := suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: artist.ID,
	})

	suite.Require().NoError(err)
	suite.Equal("mid_card", resp.BillingTier)
}

func (suite *FestivalServiceIntegrationTestSuite) TestAddFestivalArtist_FestivalNotFound() {
	artist := suite.createTestArtistForFestival("Orphan Band")

	resp, err := suite.festivalService.AddFestivalArtist(99999, &contracts.AddFestivalArtistRequest{ArtistID: artist.ID})

	suite.Require().Error(err)
	suite.Nil(resp)
}

func (suite *FestivalServiceIntegrationTestSuite) TestAddFestivalArtist_ArtistNotFound() {
	festival := suite.createBasicFestival("Missing Artist Festival")

	resp, err := suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{ArtistID: 99999})

	suite.Require().Error(err)
	suite.Nil(resp)
	suite.Contains(err.Error(), "artist not found")
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalArtists_OrderedByBillingTier() {
	festival := suite.createBasicFestival("Ordered Lineup Festival")
	headliner := suite.createTestArtistForFestival("Headliner")
	undercard := suite.createTestArtistForFestival("Undercard")
	local := suite.createTestArtistForFestival("Local Opener")

	// Add in reverse order to test ordering
	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: local.ID, BillingTier: "local", Position: 0,
	})
	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: headliner.ID, BillingTier: "headliner", Position: 0,
	})
	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: undercard.ID, BillingTier: "undercard", Position: 0,
	})

	resp, err := suite.festivalService.GetFestivalArtists(festival.ID, nil)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 3)
	suite.Equal("headliner", resp[0].BillingTier)
	suite.Equal("undercard", resp[1].BillingTier)
	suite.Equal("local", resp[2].BillingTier)
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalArtists_FilterByDay() {
	festival := suite.createBasicFestival("Multi Day Festival")
	day1Artist := suite.createTestArtistForFestival("Day 1 Artist")
	day2Artist := suite.createTestArtistForFestival("Day 2 Artist")

	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: day1Artist.ID, BillingTier: "headliner", DayDate: strPtrFestival("2026-03-06"),
	})
	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: day2Artist.ID, BillingTier: "headliner", DayDate: strPtrFestival("2026-03-07"),
	})

	dayFilter := "2026-03-07"
	resp, err := suite.festivalService.GetFestivalArtists(festival.ID, &dayFilter)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Day 2 Artist", resp[0].ArtistName)
}

func (suite *FestivalServiceIntegrationTestSuite) TestUpdateFestivalArtist_Success() {
	festival := suite.createBasicFestival("Update Artist Festival")
	artist := suite.createTestArtistForFestival("Promoted Band")

	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artist.ID,
		BillingTier: "undercard",
	})

	newTier := "headliner"
	newStage := "Main Stage"
	resp, err := suite.festivalService.UpdateFestivalArtist(festival.ID, artist.ID, &contracts.UpdateFestivalArtistRequest{
		BillingTier: &newTier,
		Stage:       &newStage,
	})

	suite.Require().NoError(err)
	suite.Equal("headliner", resp.BillingTier)
	suite.Equal("Main Stage", *resp.Stage)
}

func (suite *FestivalServiceIntegrationTestSuite) TestUpdateFestivalArtist_NotFound() {
	newTier := "headliner"
	resp, err := suite.festivalService.UpdateFestivalArtist(99999, 99999, &contracts.UpdateFestivalArtistRequest{
		BillingTier: &newTier,
	})

	suite.Require().Error(err)
	suite.Nil(resp)
	suite.Contains(err.Error(), "artist not found in festival lineup")
}

func (suite *FestivalServiceIntegrationTestSuite) TestRemoveFestivalArtist_Success() {
	festival := suite.createBasicFestival("Remove Artist Festival")
	artist := suite.createTestArtistForFestival("Removed Band")

	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: artist.ID,
	})

	err := suite.festivalService.RemoveFestivalArtist(festival.ID, artist.ID)

	suite.Require().NoError(err)

	// Verify removal
	lineup, err := suite.festivalService.GetFestivalArtists(festival.ID, nil)
	suite.Require().NoError(err)
	suite.Empty(lineup)
}

func (suite *FestivalServiceIntegrationTestSuite) TestRemoveFestivalArtist_NotFound() {
	err := suite.festivalService.RemoveFestivalArtist(99999, 99999)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "artist not found in festival lineup")
}

// =============================================================================
// Group 7: Festival Venues
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalVenues_Empty() {
	festival := suite.createBasicFestival("No Venue Festival")

	resp, err := suite.festivalService.GetFestivalVenues(festival.ID)

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalVenues_FestivalNotFound() {
	resp, err := suite.festivalService.GetFestivalVenues(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
}

func (suite *FestivalServiceIntegrationTestSuite) TestAddFestivalVenue_Success() {
	festival := suite.createBasicFestival("Venue Festival")
	venue := suite.createTestVenue("Main Park", "Phoenix", "AZ")

	resp, err := suite.festivalService.AddFestivalVenue(festival.ID, &contracts.AddFestivalVenueRequest{
		VenueID:   venue.ID,
		IsPrimary: true,
	})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal(venue.ID, resp.VenueID)
	suite.Equal("Main Park", resp.VenueName)
	suite.True(resp.IsPrimary)
}

func (suite *FestivalServiceIntegrationTestSuite) TestAddFestivalVenue_FestivalNotFound() {
	venue := suite.createTestVenue("Orphan Venue", "Phoenix", "AZ")

	resp, err := suite.festivalService.AddFestivalVenue(99999, &contracts.AddFestivalVenueRequest{VenueID: venue.ID})

	suite.Require().Error(err)
	suite.Nil(resp)
}

func (suite *FestivalServiceIntegrationTestSuite) TestAddFestivalVenue_VenueNotFound() {
	festival := suite.createBasicFestival("Missing Venue Festival")

	resp, err := suite.festivalService.AddFestivalVenue(festival.ID, &contracts.AddFestivalVenueRequest{VenueID: 99999})

	suite.Require().Error(err)
	suite.Nil(resp)
	suite.Contains(err.Error(), "venue not found")
}

func (suite *FestivalServiceIntegrationTestSuite) TestRemoveFestivalVenue_Success() {
	festival := suite.createBasicFestival("Remove Venue Festival")
	venue := suite.createTestVenue("Removable Venue", "Phoenix", "AZ")

	suite.festivalService.AddFestivalVenue(festival.ID, &contracts.AddFestivalVenueRequest{VenueID: venue.ID})

	err := suite.festivalService.RemoveFestivalVenue(festival.ID, venue.ID)

	suite.Require().NoError(err)

	// Verify removal
	venues, err := suite.festivalService.GetFestivalVenues(festival.ID)
	suite.Require().NoError(err)
	suite.Empty(venues)
}

func (suite *FestivalServiceIntegrationTestSuite) TestRemoveFestivalVenue_NotFound() {
	err := suite.festivalService.RemoveFestivalVenue(99999, 99999)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "venue not found in festival")
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalVenues_PrimaryFirst() {
	festival := suite.createBasicFestival("Multi Venue Festival")
	venue1 := suite.createTestVenue("Secondary Venue", "Phoenix", "AZ")
	venue2 := suite.createTestVenue("Primary Venue", "Phoenix", "AZ")

	suite.festivalService.AddFestivalVenue(festival.ID, &contracts.AddFestivalVenueRequest{
		VenueID: venue1.ID, IsPrimary: false,
	})
	suite.festivalService.AddFestivalVenue(festival.ID, &contracts.AddFestivalVenueRequest{
		VenueID: venue2.ID, IsPrimary: true,
	})

	resp, err := suite.festivalService.GetFestivalVenues(festival.ID)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	suite.True(resp[0].IsPrimary)
	suite.False(resp[1].IsPrimary)
}

// =============================================================================
// Group 8: GetFestivalsForArtist (cross-entity query)
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalsForArtist_Success() {
	artist := suite.createTestArtistForFestival("Multi Festival Artist")

	fest1 := suite.createBasicFestival("Festival One")
	fest2 := suite.createBasicFestival("Festival Two")
	suite.createBasicFestival("Festival Three") // not associated

	suite.festivalService.AddFestivalArtist(fest1.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: artist.ID, BillingTier: "headliner",
	})
	suite.festivalService.AddFestivalArtist(fest2.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: artist.ID, BillingTier: "undercard",
	})

	resp, err := suite.festivalService.GetFestivalsForArtist(artist.ID)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)

	// Verify billing tier is included
	tierMap := make(map[string]string)
	for _, f := range resp {
		tierMap[f.Name] = f.BillingTier
	}
	suite.Equal("headliner", tierMap["Festival One"])
	suite.Equal("undercard", tierMap["Festival Two"])
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalsForArtist_ArtistNotFound() {
	resp, err := suite.festivalService.GetFestivalsForArtist(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *FestivalServiceIntegrationTestSuite) TestGetFestivalsForArtist_Empty() {
	artist := suite.createTestArtistForFestival("No Festivals Artist")

	resp, err := suite.festivalService.GetFestivalsForArtist(artist.ID)

	suite.Require().NoError(err)
	suite.Empty(resp)
}

// =============================================================================
// Group 9: Artist and Venue counts on list/detail
// =============================================================================

func (suite *FestivalServiceIntegrationTestSuite) TestFestival_ArtistAndVenueCounts() {
	festival := suite.createBasicFestival("Counted Festival")
	artist1 := suite.createTestArtistForFestival("Count Artist 1")
	artist2 := suite.createTestArtistForFestival("Count Artist 2")
	venue := suite.createTestVenue("Count Venue", "Phoenix", "AZ")

	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{ArtistID: artist1.ID})
	suite.festivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{ArtistID: artist2.ID})
	suite.festivalService.AddFestivalVenue(festival.ID, &contracts.AddFestivalVenueRequest{VenueID: venue.ID})

	// Test detail response counts
	detail, err := suite.festivalService.GetFestival(festival.ID)
	suite.Require().NoError(err)
	suite.Equal(2, detail.ArtistCount)
	suite.Equal(1, detail.VenueCount)

	// Test list response counts
	list, err := suite.festivalService.ListFestivals(map[string]interface{}{})
	suite.Require().NoError(err)
	suite.Require().Len(list, 1)
	suite.Equal(2, list[0].ArtistCount)
	suite.Equal(1, list[0].VenueCount)
}
