package catalog

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

type FestivalHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *FestivalHandler
}

func (s *FestivalHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewFestivalHandler(s.deps.FestivalService, s.deps.ArtistService, s.deps.AuditLogService, nil)
}

func (s *FestivalHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *FestivalHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestFestivalHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(FestivalHandlerIntegrationSuite))
}

// --- Helpers ---

var handlerFestivalCounter int64

func (s *FestivalHandlerIntegrationSuite) createFestivalViaService(name string) *contracts.FestivalDetailResponse {
	city := "Phoenix"
	state := "AZ"
	counter := atomic.AddInt64(&handlerFestivalCounter, 1)
	resp, err := s.deps.FestivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name:        name,
		SeriesSlug:  utils.GenerateArtistSlug(name),
		EditionYear: 2026 + int(counter),
		City:        &city,
		State:       &state,
		StartDate:   "2026-03-06",
		EndDate:     "2026-03-08",
	})
	s.Require().NoError(err)
	return resp
}

func (s *FestivalHandlerIntegrationSuite) createArtistViaArtistService(name string) uint {
	resp, err := s.deps.ArtistService.CreateArtist(&contracts.CreateArtistRequest{Name: name})
	s.Require().NoError(err)
	return resp.ID
}

func (s *FestivalHandlerIntegrationSuite) createVenueViaDB(name, city, state string) uint {
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, name, city, state)
	return venue.ID
}

// --- ListFestivalsHandler ---

func (s *FestivalHandlerIntegrationSuite) TestListFestivals_Success() {
	s.createFestivalViaService("Festival A")
	s.createFestivalViaService("Festival B")

	req := &ListFestivalsRequest{}
	resp, err := s.handler.ListFestivalsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 2)
}

func (s *FestivalHandlerIntegrationSuite) TestListFestivals_Empty() {
	req := &ListFestivalsRequest{}
	resp, err := s.handler.ListFestivalsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *FestivalHandlerIntegrationSuite) TestListFestivals_FilterByStatus() {
	s.deps.FestivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "Confirmed Fest", SeriesSlug: "cf", EditionYear: 2026,
		StartDate: "2026-03-01", EndDate: "2026-03-03", Status: "confirmed",
	})
	s.deps.FestivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name: "Announced Fest", SeriesSlug: "af", EditionYear: 2026,
		StartDate: "2026-04-01", EndDate: "2026-04-03",
	})

	req := &ListFestivalsRequest{Status: "confirmed"}
	resp, err := s.handler.ListFestivalsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.Equal(1, resp.Body.Count)
	s.Equal("Confirmed Fest", resp.Body.Festivals[0].Name)
}

// --- GetFestivalHandler ---

func (s *FestivalHandlerIntegrationSuite) TestGetFestival_ByID() {
	festival := s.createFestivalViaService("Test Festival")

	req := &GetFestivalRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	resp, err := s.handler.GetFestivalHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Test Festival", resp.Body.Name)
}

func (s *FestivalHandlerIntegrationSuite) TestGetFestival_BySlug() {
	s.createFestivalViaService("Slug Festival")

	req := &GetFestivalRequest{FestivalID: "slug-festival"}
	resp, err := s.handler.GetFestivalHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Slug Festival", resp.Body.Name)
}

func (s *FestivalHandlerIntegrationSuite) TestGetFestival_NotFound() {
	req := &GetFestivalRequest{FestivalID: "99999"}
	_, err := s.handler.GetFestivalHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- CreateFestivalHandler ---

func (s *FestivalHandlerIntegrationSuite) TestCreateFestival_AdminSuccess() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateFestivalRequest{}
	req.Body.Name = "New Festival"
	req.Body.SeriesSlug = "new-fest"
	req.Body.EditionYear = 2026
	req.Body.StartDate = "2026-06-01"
	req.Body.EndDate = "2026-06-03"

	resp, err := s.handler.CreateFestivalHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("New Festival", resp.Body.Name)
	s.Equal("new-fest", resp.Body.SeriesSlug)
	s.Equal(2026, resp.Body.EditionYear)
}

func (s *FestivalHandlerIntegrationSuite) TestCreateFestival_EmptyName() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateFestivalRequest{}
	req.Body.Name = ""
	req.Body.SeriesSlug = "empty"
	req.Body.EditionYear = 2026
	req.Body.StartDate = "2026-06-01"
	req.Body.EndDate = "2026-06-03"

	_, err := s.handler.CreateFestivalHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *FestivalHandlerIntegrationSuite) TestCreateFestival_MissingSeriesSlug() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateFestivalRequest{}
	req.Body.Name = "Missing Series"
	req.Body.SeriesSlug = ""
	req.Body.EditionYear = 2026
	req.Body.StartDate = "2026-06-01"
	req.Body.EndDate = "2026-06-03"

	_, err := s.handler.CreateFestivalHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// --- UpdateFestivalHandler ---

func (s *FestivalHandlerIntegrationSuite) TestUpdateFestival_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Original Festival")

	ctx := testhelpers.CtxWithUser(admin)
	newName := "Updated Festival"
	req := &UpdateFestivalRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	req.Body.Name = &newName

	resp, err := s.handler.UpdateFestivalHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Festival", resp.Body.Name)
}

func (s *FestivalHandlerIntegrationSuite) TestUpdateFestival_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	newName := "New Name"
	req := &UpdateFestivalRequest{FestivalID: "99999"}
	req.Body.Name = &newName

	_, err := s.handler.UpdateFestivalHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- DeleteFestivalHandler ---

func (s *FestivalHandlerIntegrationSuite) TestDeleteFestival_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Deletable Festival")

	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteFestivalRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	_, err := s.handler.DeleteFestivalHandler(ctx, req)
	s.NoError(err)

	// Verify festival is gone
	getReq := &GetFestivalRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	_, err = s.handler.GetFestivalHandler(s.deps.Ctx, getReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *FestivalHandlerIntegrationSuite) TestDeleteFestival_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteFestivalRequest{FestivalID: "99999"}

	_, err := s.handler.DeleteFestivalHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- GetFestivalArtistsHandler ---

func (s *FestivalHandlerIntegrationSuite) TestGetFestivalArtists_Success() {
	festival := s.createFestivalViaService("Lineup Festival")
	artistID := s.createArtistViaArtistService("Lineup Artist")

	s.deps.FestivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artistID,
		BillingTier: "headliner",
	})

	req := &GetFestivalArtistsRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	resp, err := s.handler.GetFestivalArtistsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Lineup Artist", resp.Body.Artists[0].ArtistName)
	s.Equal("headliner", resp.Body.Artists[0].BillingTier)
}

func (s *FestivalHandlerIntegrationSuite) TestGetFestivalArtists_BySlug() {
	s.createFestivalViaService("Slug Lineup Festival")

	req := &GetFestivalArtistsRequest{FestivalID: "slug-lineup-festival"}
	resp, err := s.handler.GetFestivalArtistsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *FestivalHandlerIntegrationSuite) TestGetFestivalArtists_NotFound() {
	req := &GetFestivalArtistsRequest{FestivalID: "99999"}
	_, err := s.handler.GetFestivalArtistsHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- AddFestivalArtistHandler ---

func (s *FestivalHandlerIntegrationSuite) TestAddFestivalArtist_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Add Artist Festival")
	artistID := s.createArtistViaArtistService("Added Artist")

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddFestivalArtistHandlerRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	req.Body.ArtistID = artistID
	req.Body.BillingTier = "headliner"

	resp, err := s.handler.AddFestivalArtistHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Added Artist", resp.Body.ArtistName)
	s.Equal("headliner", resp.Body.BillingTier)
}

func (s *FestivalHandlerIntegrationSuite) TestAddFestivalArtist_MissingArtistID() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Missing Artist ID Festival")

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddFestivalArtistHandlerRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	req.Body.ArtistID = 0

	_, err := s.handler.AddFestivalArtistHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// --- UpdateFestivalArtistHandler ---

func (s *FestivalHandlerIntegrationSuite) TestUpdateFestivalArtist_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Update Artist Festival")
	artistID := s.createArtistViaArtistService("Promoted Artist")

	s.deps.FestivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artistID,
		BillingTier: "undercard",
	})

	ctx := testhelpers.CtxWithUser(admin)
	newTier := "headliner"
	req := &UpdateFestivalArtistHandlerRequest{
		FestivalID: fmt.Sprintf("%d", festival.ID),
		ArtistID:   fmt.Sprintf("%d", artistID),
	}
	req.Body.BillingTier = &newTier

	resp, err := s.handler.UpdateFestivalArtistHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("headliner", resp.Body.BillingTier)
}

func (s *FestivalHandlerIntegrationSuite) TestUpdateFestivalArtist_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	newTier := "headliner"
	req := &UpdateFestivalArtistHandlerRequest{
		FestivalID: "99999",
		ArtistID:   "99999",
	}
	req.Body.BillingTier = &newTier

	_, err := s.handler.UpdateFestivalArtistHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- RemoveFestivalArtistHandler ---

func (s *FestivalHandlerIntegrationSuite) TestRemoveFestivalArtist_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Remove Artist Festival")
	artistID := s.createArtistViaArtistService("Removed Artist")

	s.deps.FestivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: artistID,
	})

	ctx := testhelpers.CtxWithUser(admin)
	req := &RemoveFestivalArtistRequest{
		FestivalID: fmt.Sprintf("%d", festival.ID),
		ArtistID:   fmt.Sprintf("%d", artistID),
	}

	_, err := s.handler.RemoveFestivalArtistHandler(ctx, req)
	s.NoError(err)
}

// --- GetFestivalVenuesHandler ---

func (s *FestivalHandlerIntegrationSuite) TestGetFestivalVenues_Success() {
	festival := s.createFestivalViaService("Venue Festival")
	venueID := s.createVenueViaDB("Test Venue", "Phoenix", "AZ")

	s.deps.FestivalService.AddFestivalVenue(festival.ID, &contracts.AddFestivalVenueRequest{
		VenueID:   venueID,
		IsPrimary: true,
	})

	req := &GetFestivalVenuesRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	resp, err := s.handler.GetFestivalVenuesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Test Venue", resp.Body.Venues[0].VenueName)
	s.True(resp.Body.Venues[0].IsPrimary)
}

func (s *FestivalHandlerIntegrationSuite) TestGetFestivalVenues_NotFound() {
	req := &GetFestivalVenuesRequest{FestivalID: "99999"}
	_, err := s.handler.GetFestivalVenuesHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- AddFestivalVenueHandler ---

func (s *FestivalHandlerIntegrationSuite) TestAddFestivalVenue_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Add Venue Festival")
	venueID := s.createVenueViaDB("Added Venue", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddFestivalVenueHandlerRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	req.Body.VenueID = venueID
	req.Body.IsPrimary = true

	resp, err := s.handler.AddFestivalVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Added Venue", resp.Body.VenueName)
	s.True(resp.Body.IsPrimary)
}

func (s *FestivalHandlerIntegrationSuite) TestAddFestivalVenue_MissingVenueID() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Missing Venue ID Festival")

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddFestivalVenueHandlerRequest{FestivalID: fmt.Sprintf("%d", festival.ID)}
	req.Body.VenueID = 0

	_, err := s.handler.AddFestivalVenueHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// --- RemoveFestivalVenueHandler ---

func (s *FestivalHandlerIntegrationSuite) TestRemoveFestivalVenue_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	festival := s.createFestivalViaService("Remove Venue Festival")
	venueID := s.createVenueViaDB("Removable Venue", "Phoenix", "AZ")

	s.deps.FestivalService.AddFestivalVenue(festival.ID, &contracts.AddFestivalVenueRequest{
		VenueID: venueID,
	})

	ctx := testhelpers.CtxWithUser(admin)
	req := &RemoveFestivalVenueRequest{
		FestivalID: fmt.Sprintf("%d", festival.ID),
		VenueID:    fmt.Sprintf("%d", venueID),
	}

	_, err := s.handler.RemoveFestivalVenueHandler(ctx, req)
	s.NoError(err)
}

// --- GetArtistFestivalsHandler ---

func (s *FestivalHandlerIntegrationSuite) TestGetArtistFestivals_Success() {
	festival := s.createFestivalViaService("Artist Fest History")
	artistID := s.createArtistViaArtistService("Festival Artist")

	s.deps.FestivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artistID,
		BillingTier: "headliner",
	})

	req := &GetArtistFestivalsRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	resp, err := s.handler.GetArtistFestivalsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Artist Fest History", resp.Body.Festivals[0].Name)
	s.Equal("headliner", resp.Body.Festivals[0].BillingTier)
}

func (s *FestivalHandlerIntegrationSuite) TestGetArtistFestivals_BySlug() {
	festival := s.createFestivalViaService("Slug Artist Festival")
	artistID := s.createArtistViaArtistService("Slug Festival Artist")

	s.deps.FestivalService.AddFestivalArtist(festival.ID, &contracts.AddFestivalArtistRequest{
		ArtistID: artistID,
	})

	req := &GetArtistFestivalsRequest{ArtistID: "slug-festival-artist"}
	resp, err := s.handler.GetArtistFestivalsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
}

func (s *FestivalHandlerIntegrationSuite) TestGetArtistFestivals_ArtistNotFound() {
	req := &GetArtistFestivalsRequest{ArtistID: "99999"}
	_, err := s.handler.GetArtistFestivalsHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *FestivalHandlerIntegrationSuite) TestGetArtistFestivals_Empty() {
	artistID := s.createArtistViaArtistService("No Festivals Artist")

	req := &GetArtistFestivalsRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	resp, err := s.handler.GetArtistFestivalsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}
