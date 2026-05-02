package handlers

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/catalog"
)

type FestivalIntelligenceHandlerSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *FestivalIntelligenceHandler
}

func (s *FestivalIntelligenceHandlerSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	intelService := catalog.NewFestivalIntelligenceService(s.deps.db)
	s.handler = NewFestivalIntelligenceHandler(intelService, s.deps.festivalService, s.deps.artistService)
}

func (s *FestivalIntelligenceHandlerSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *FestivalIntelligenceHandlerSuite) TearDownSuite() {
	s.deps.testDB.Cleanup()
}

func TestFestivalIntelligenceHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(FestivalIntelligenceHandlerSuite))
}

// --- Helpers ---

var intelFestivalCounter int64

func (s *FestivalIntelligenceHandlerSuite) createFestival(name, seriesSlug string, year int) *contracts.FestivalDetailResponse {
	city := "Phoenix"
	state := "AZ"
	counter := atomic.AddInt64(&intelFestivalCounter, 1)
	startDate := fmt.Sprintf("%d-03-%02d", year, int(counter%28)+1)
	resp, err := s.deps.festivalService.CreateFestival(&contracts.CreateFestivalRequest{
		Name:        name,
		SeriesSlug:  seriesSlug,
		EditionYear: year,
		City:        &city,
		State:       &state,
		StartDate:   startDate,
		EndDate:     startDate,
		Status:      "confirmed",
	})
	s.Require().NoError(err)
	return resp
}

func (s *FestivalIntelligenceHandlerSuite) addArtistToFestival(festivalID uint, name, tier string) uint {
	artist := createArtist(s.deps.db, name)
	_, err := s.deps.festivalService.AddFestivalArtist(festivalID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artist.ID,
		BillingTier: tier,
	})
	s.Require().NoError(err)
	return artist.ID
}

// ============================================================================
// GetSimilarFestivals
// ============================================================================

func (s *FestivalIntelligenceHandlerSuite) TestGetSimilarFestivals_Success() {
	f1 := s.createFestival("Similar A", "sa", 2026)
	f2 := s.createFestival("Similar B", "sb", 2026)

	// Create 4 shared artists
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Shared Intel %d", i)
		artist := createArtist(s.deps.db, name)
		_, _ = s.deps.festivalService.AddFestivalArtist(f1.ID, &contracts.AddFestivalArtistRequest{ArtistID: artist.ID, BillingTier: "mid_card"})
		_, _ = s.deps.festivalService.AddFestivalArtist(f2.ID, &contracts.AddFestivalArtistRequest{ArtistID: artist.ID, BillingTier: "mid_card"})
	}

	req := &GetSimilarFestivalsRequest{FestivalID: fmt.Sprintf("%d", f1.ID), Limit: 10}
	resp, err := s.handler.GetSimilarFestivalsHandler(s.deps.ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotEmpty(resp.Body.Similar)
	s.Equal(f2.ID, resp.Body.Similar[0].Festival.ID)
}

func (s *FestivalIntelligenceHandlerSuite) TestGetSimilarFestivals_LimitParameter() {
	f1 := s.createFestival("Limit Source", "ls", 2026)

	// Create 3 festivals each sharing 3 artists with f1
	for j := 0; j < 3; j++ {
		other := s.createFestival(fmt.Sprintf("Limit Target %d", j), fmt.Sprintf("lt%d", j), 2026)
		for i := 0; i < 3; i++ {
			a := createArtist(s.deps.db, fmt.Sprintf("LimitShared %d-%d", j, i))
			_, _ = s.deps.festivalService.AddFestivalArtist(f1.ID, &contracts.AddFestivalArtistRequest{ArtistID: a.ID, BillingTier: "mid_card"})
			_, _ = s.deps.festivalService.AddFestivalArtist(other.ID, &contracts.AddFestivalArtistRequest{ArtistID: a.ID, BillingTier: "mid_card"})
		}
	}

	req := &GetSimilarFestivalsRequest{FestivalID: fmt.Sprintf("%d", f1.ID), Limit: 2}
	resp, err := s.handler.GetSimilarFestivalsHandler(s.deps.ctx, req)

	s.Require().NoError(err)
	s.LessOrEqual(len(resp.Body.Similar), 2)
}

func (s *FestivalIntelligenceHandlerSuite) TestGetSimilarFestivals_NotFound() {
	req := &GetSimilarFestivalsRequest{FestivalID: "99999"}
	_, err := s.handler.GetSimilarFestivalsHandler(s.deps.ctx, req)
	s.Require().Error(err)
}

// ============================================================================
// GetFestivalOverlap
// ============================================================================

func (s *FestivalIntelligenceHandlerSuite) TestGetFestivalOverlap_Success() {
	f1 := s.createFestival("Overlap A", "oa", 2026)
	f2 := s.createFestival("Overlap B", "ob", 2026)

	a := createArtist(s.deps.db, "Overlap Shared")
	_, _ = s.deps.festivalService.AddFestivalArtist(f1.ID, &contracts.AddFestivalArtistRequest{ArtistID: a.ID, BillingTier: "headliner"})
	_, _ = s.deps.festivalService.AddFestivalArtist(f2.ID, &contracts.AddFestivalArtistRequest{ArtistID: a.ID, BillingTier: "mid_card"})

	req := &GetFestivalOverlapRequest{
		FestivalAID: fmt.Sprintf("%d", f1.ID),
		FestivalBID: fmt.Sprintf("%d", f2.ID),
	}
	resp, err := s.handler.GetFestivalOverlapHandler(s.deps.ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.Len(resp.Body.SharedArtists, 1)
}

func (s *FestivalIntelligenceHandlerSuite) TestGetFestivalOverlap_NotFound() {
	req := &GetFestivalOverlapRequest{FestivalAID: "99999", FestivalBID: "99998"}
	_, err := s.handler.GetFestivalOverlapHandler(s.deps.ctx, req)
	s.Require().Error(err)
}

// ============================================================================
// GetFestivalBreakouts
// ============================================================================

func (s *FestivalIntelligenceHandlerSuite) TestGetFestivalBreakouts_Success() {
	f1 := s.createFestival("Breakout Early", "be", 2024)
	f2 := s.createFestival("Breakout Late", "bl", 2026)

	a := createArtist(s.deps.db, "Rising Handler Star")
	_, _ = s.deps.festivalService.AddFestivalArtist(f1.ID, &contracts.AddFestivalArtistRequest{ArtistID: a.ID, BillingTier: "undercard"})
	_, _ = s.deps.festivalService.AddFestivalArtist(f2.ID, &contracts.AddFestivalArtistRequest{ArtistID: a.ID, BillingTier: "headliner"})

	req := &GetFestivalBreakoutsRequest{FestivalID: fmt.Sprintf("%d", f2.ID)}
	resp, err := s.handler.GetFestivalBreakoutsHandler(s.deps.ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.NotEmpty(resp.Body.Breakouts)
}

func (s *FestivalIntelligenceHandlerSuite) TestGetFestivalBreakouts_NotFound() {
	req := &GetFestivalBreakoutsRequest{FestivalID: "99999"}
	_, err := s.handler.GetFestivalBreakoutsHandler(s.deps.ctx, req)
	s.Require().Error(err)
}

// ============================================================================
// GetArtistFestivalTrajectory
// ============================================================================

func (s *FestivalIntelligenceHandlerSuite) TestGetArtistFestivalTrajectory_Success() {
	f := s.createFestival("Trajectory Fest", "tf", 2026)
	artistID := s.addArtistToFestival(f.ID, "Trajectory Handler Artist", "headliner")

	req := &GetArtistFestivalTrajectoryRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	resp, err := s.handler.GetArtistFestivalTrajectoryHandler(s.deps.ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.Equal(1, resp.Body.TotalAppearances)
}

func (s *FestivalIntelligenceHandlerSuite) TestGetArtistFestivalTrajectory_NotFound() {
	req := &GetArtistFestivalTrajectoryRequest{ArtistID: "99999"}
	_, err := s.handler.GetArtistFestivalTrajectoryHandler(s.deps.ctx, req)
	s.Require().Error(err)
}

// ============================================================================
// GetSeriesComparison
// ============================================================================

func (s *FestivalIntelligenceHandlerSuite) TestGetSeriesComparison_Success() {
	s.createFestival("Comp 2024", "comp", 2024)
	f2 := s.createFestival("Comp 2025", "comp", 2025)

	s.addArtistToFestival(f2.ID, "Comp Artist", "headliner")

	req := &GetSeriesComparisonRequest{SeriesSlug: "comp", Years: "2024,2025"}
	resp, err := s.handler.GetSeriesComparisonHandler(s.deps.ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.Equal("comp", resp.Body.SeriesSlug)
	s.Len(resp.Body.Editions, 2)
}

func (s *FestivalIntelligenceHandlerSuite) TestGetSeriesComparison_InvalidYears() {
	req := &GetSeriesComparisonRequest{SeriesSlug: "test", Years: "notayear"}
	_, err := s.handler.GetSeriesComparisonHandler(s.deps.ctx, req)
	s.Require().Error(err)
}

func (s *FestivalIntelligenceHandlerSuite) TestGetSeriesComparison_TooFewYears() {
	req := &GetSeriesComparisonRequest{SeriesSlug: "test", Years: "2024"}
	_, err := s.handler.GetSeriesComparisonHandler(s.deps.ctx, req)
	s.Require().Error(err)
}

func (s *FestivalIntelligenceHandlerSuite) TestGetSeriesComparison_MissingSlug() {
	req := &GetSeriesComparisonRequest{SeriesSlug: "", Years: "2024,2025"}
	_, err := s.handler.GetSeriesComparisonHandler(s.deps.ctx, req)
	s.Require().Error(err)
}
