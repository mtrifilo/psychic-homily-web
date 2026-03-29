package catalog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestFestivalIntelligenceService_NilDatabase(t *testing.T) {
	svc := &FestivalIntelligenceService{db: nil}

	t.Run("GetSimilarFestivals", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetSimilarFestivals(1, 10)
		})
	})

	t.Run("GetFestivalOverlap", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetFestivalOverlap(1, 2)
		})
	})

	t.Run("GetFestivalBreakouts", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetFestivalBreakouts(1)
		})
	})

	t.Run("GetArtistFestivalTrajectory", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetArtistFestivalTrajectory(1)
		})
	})

	t.Run("GetSeriesComparison", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetSeriesComparison("m3f", []int{2024, 2025})
		})
	})
}

func TestTierWeight(t *testing.T) {
	assert.Equal(t, 5.0, tierWeight("headliner"))
	assert.Equal(t, 3.0, tierWeight("sub_headliner"))
	assert.Equal(t, 2.0, tierWeight("mid_card"))
	assert.Equal(t, 1.0, tierWeight("undercard"))
	assert.Equal(t, 0.5, tierWeight("local"))
	assert.Equal(t, 0.5, tierWeight("dj"))
	assert.Equal(t, 0.25, tierWeight("host"))
	assert.Equal(t, 1.0, tierWeight("unknown_tier"))
}

func TestTierRank(t *testing.T) {
	assert.Equal(t, 1, tierRank("headliner"))
	assert.Equal(t, 2, tierRank("sub_headliner"))
	assert.Equal(t, 3, tierRank("mid_card"))
	assert.Equal(t, 7, tierRank("host"))
	assert.Equal(t, 8, tierRank("unknown"))
}

func TestRankToTier(t *testing.T) {
	assert.Equal(t, "headliner", rankToTier(1))
	assert.Equal(t, "sub_headliner", rankToTier(2))
	assert.Equal(t, "host", rankToTier(7))
	assert.Equal(t, "unknown", rankToTier(99))
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type FestivalIntelligenceTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	svc     *FestivalIntelligenceService
	festSvc *FestivalService
}

func (suite *FestivalIntelligenceTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.svc = &FestivalIntelligenceService{db: suite.testDB.DB}
	suite.festSvc = &FestivalService{db: suite.testDB.DB}
}

func (suite *FestivalIntelligenceTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *FestivalIntelligenceTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func TestFestivalIntelligenceTestSuite(t *testing.T) {
	suite.Run(t, new(FestivalIntelligenceTestSuite))
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func (suite *FestivalIntelligenceTestSuite) createArtist(name string) *models.Artist {
	a := &models.Artist{Name: name}
	suite.Require().NoError(suite.db.Create(a).Error)
	return a
}

func (suite *FestivalIntelligenceTestSuite) createFestival(name, seriesSlug string, year int, startDate string) *contracts.FestivalDetailResponse {
	city := "Phoenix"
	state := "AZ"
	req := &contracts.CreateFestivalRequest{
		Name:        name,
		SeriesSlug:  seriesSlug,
		EditionYear: year,
		City:        &city,
		State:       &state,
		StartDate:   startDate,
		EndDate:     startDate,
		Status:      "confirmed",
	}
	resp, err := suite.festSvc.CreateFestival(req)
	suite.Require().NoError(err)
	return resp
}

func (suite *FestivalIntelligenceTestSuite) addArtistToFestival(festivalID, artistID uint, tier string) {
	_, err := suite.festSvc.AddFestivalArtist(festivalID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artistID,
		BillingTier: tier,
	})
	suite.Require().NoError(err)
}

// ─────────────────────────────────────────────
// GetSimilarFestivals tests
// ─────────────────────────────────────────────

func (suite *FestivalIntelligenceTestSuite) TestGetSimilarFestivals_WithOverlap() {
	// Create two festivals sharing 4 artists + each has unique ones
	f1 := suite.createFestival("Fest A", "a", 2026, "2026-03-01")
	f2 := suite.createFestival("Fest B", "b", 2026, "2026-06-01")

	shared := make([]*models.Artist, 4)
	for i := 0; i < 4; i++ {
		shared[i] = suite.createArtist(fmt.Sprintf("Shared Artist %d", i))
	}

	// Add shared artists to both: headliner at A, mid_card at B
	for i, a := range shared {
		tier := "mid_card"
		if i == 0 {
			tier = "headliner"
		}
		suite.addArtistToFestival(f1.ID, a.ID, tier)
		suite.addArtistToFestival(f2.ID, a.ID, "mid_card")
	}

	// Add unique artists to each
	for i := 0; i < 3; i++ {
		uniqA := suite.createArtist(fmt.Sprintf("Unique A %d", i))
		suite.addArtistToFestival(f1.ID, uniqA.ID, "undercard")
		uniqB := suite.createArtist(fmt.Sprintf("Unique B %d", i))
		suite.addArtistToFestival(f2.ID, uniqB.ID, "undercard")
	}

	similar, err := suite.svc.GetSimilarFestivals(f1.ID, 10)

	suite.Require().NoError(err)
	suite.Require().Len(similar, 1)
	suite.Equal(f2.ID, similar[0].Festival.ID)
	suite.Equal(4, similar[0].SharedArtistCount)
	suite.Greater(similar[0].Jaccard, 0.0)
	suite.Greater(similar[0].WeightedScore, 0.0)
	suite.LessOrEqual(len(similar[0].TopShared), 5)
}

func (suite *FestivalIntelligenceTestSuite) TestGetSimilarFestivals_BelowThreshold() {
	// Only 2 shared artists — below minimum of 3
	f1 := suite.createFestival("Fest X", "x", 2026, "2026-03-01")
	f2 := suite.createFestival("Fest Y", "y", 2026, "2026-06-01")

	a1 := suite.createArtist("Shared 1")
	a2 := suite.createArtist("Shared 2")
	suite.addArtistToFestival(f1.ID, a1.ID, "headliner")
	suite.addArtistToFestival(f1.ID, a2.ID, "mid_card")
	suite.addArtistToFestival(f2.ID, a1.ID, "headliner")
	suite.addArtistToFestival(f2.ID, a2.ID, "mid_card")

	similar, err := suite.svc.GetSimilarFestivals(f1.ID, 10)

	suite.Require().NoError(err)
	suite.Empty(similar) // Below 3-artist threshold
}

func (suite *FestivalIntelligenceTestSuite) TestGetSimilarFestivals_EmptyFestival() {
	f := suite.createFestival("Empty Fest", "empty", 2026, "2026-03-01")

	similar, err := suite.svc.GetSimilarFestivals(f.ID, 10)

	suite.Require().NoError(err)
	suite.Empty(similar)
}

func (suite *FestivalIntelligenceTestSuite) TestGetSimilarFestivals_NotFound() {
	_, err := suite.svc.GetSimilarFestivals(99999, 10)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "not found")
}

func (suite *FestivalIntelligenceTestSuite) TestGetSimilarFestivals_TierWeighting() {
	// Two festivals with same overlap count but different tier weighting
	f1 := suite.createFestival("Source Fest", "src", 2026, "2026-03-01")
	f2 := suite.createFestival("Headliner Overlap", "ho", 2026, "2026-06-01")
	f3 := suite.createFestival("Local Overlap", "lo", 2026, "2026-09-01")

	// Create 3 shared artists
	shared := make([]*models.Artist, 3)
	for i := 0; i < 3; i++ {
		shared[i] = suite.createArtist(fmt.Sprintf("Weight Artist %d", i))
	}

	// Add to source as headliners
	for _, a := range shared {
		suite.addArtistToFestival(f1.ID, a.ID, "headliner")
	}

	// f2 shares them as sub_headliners (decent weight, but source headliner = 5.0 wins via max)
	for _, a := range shared {
		suite.addArtistToFestival(f2.ID, a.ID, "sub_headliner")
	}

	// f3 shares them as locals (low weight 0.5, but source headliner = 5.0 wins via max)
	// So both f2 and f3 have the same weighted score (3 * 5.0 = 15).
	// To differentiate, give f2 more unique artists to affect Jaccard.
	for _, a := range shared {
		suite.addArtistToFestival(f3.ID, a.ID, "local")
	}
	// f3 has extra local artists that dilute its Jaccard
	for i := 0; i < 10; i++ {
		extra := suite.createArtist(fmt.Sprintf("Extra F3 %d", i))
		suite.addArtistToFestival(f3.ID, extra.ID, "local")
	}

	similar, err := suite.svc.GetSimilarFestivals(f1.ID, 10)

	suite.Require().NoError(err)
	suite.Require().Len(similar, 2)
	// f2 should rank higher due to higher Jaccard (fewer total artists = higher intersection/union ratio)
	suite.Equal(f2.ID, similar[0].Festival.ID)
	suite.GreaterOrEqual(similar[0].Jaccard, similar[1].Jaccard)
}

// ─────────────────────────────────────────────
// GetFestivalOverlap tests
// ─────────────────────────────────────────────

func (suite *FestivalIntelligenceTestSuite) TestGetFestivalOverlap_Success() {
	f1 := suite.createFestival("Overlap A", "oa", 2026, "2026-03-01")
	f2 := suite.createFestival("Overlap B", "ob", 2026, "2026-06-01")

	shared1 := suite.createArtist("Overlap Shared 1")
	shared2 := suite.createArtist("Overlap Shared 2")
	uniqueA := suite.createArtist("Only A")
	uniqueB := suite.createArtist("Only B")

	suite.addArtistToFestival(f1.ID, shared1.ID, "headliner")
	suite.addArtistToFestival(f1.ID, shared2.ID, "mid_card")
	suite.addArtistToFestival(f1.ID, uniqueA.ID, "undercard")

	suite.addArtistToFestival(f2.ID, shared1.ID, "sub_headliner")
	suite.addArtistToFestival(f2.ID, shared2.ID, "headliner")
	suite.addArtistToFestival(f2.ID, uniqueB.ID, "undercard")

	overlap, err := suite.svc.GetFestivalOverlap(f1.ID, f2.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(overlap)
	suite.Equal(f1.ID, overlap.FestivalA.ID)
	suite.Equal(f2.ID, overlap.FestivalB.ID)
	suite.Len(overlap.SharedArtists, 2)
	suite.Equal(1, overlap.AOnlyCount)
	suite.Equal(1, overlap.BOnlyCount)
	suite.Greater(overlap.Jaccard, 0.0)
	suite.Greater(overlap.WeightedScore, 0.0)

	// Check shared artists have correct tier info
	for _, sa := range overlap.SharedArtists {
		suite.NotEmpty(sa.TierAtSource)
		suite.NotEmpty(sa.TierAtTarget)
	}
}

func (suite *FestivalIntelligenceTestSuite) TestGetFestivalOverlap_NoOverlap() {
	f1 := suite.createFestival("Disjoint A", "da", 2026, "2026-03-01")
	f2 := suite.createFestival("Disjoint B", "db", 2026, "2026-06-01")

	a1 := suite.createArtist("Only In A")
	a2 := suite.createArtist("Only In B")
	suite.addArtistToFestival(f1.ID, a1.ID, "headliner")
	suite.addArtistToFestival(f2.ID, a2.ID, "headliner")

	overlap, err := suite.svc.GetFestivalOverlap(f1.ID, f2.ID)

	suite.Require().NoError(err)
	suite.Empty(overlap.SharedArtists)
	suite.Equal(0.0, overlap.Jaccard)
}

func (suite *FestivalIntelligenceTestSuite) TestGetFestivalOverlap_NotFound() {
	_, err := suite.svc.GetFestivalOverlap(99999, 99998)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "not found")
}

// ─────────────────────────────────────────────
// GetFestivalBreakouts tests
// ─────────────────────────────────────────────

func (suite *FestivalIntelligenceTestSuite) TestGetFestivalBreakouts_WithBreakout() {
	// Artist goes from undercard to sub_headliner across two festivals
	f1 := suite.createFestival("Early Fest", "ef", 2024, "2024-03-01")
	f2 := suite.createFestival("Later Fest", "lf", 2026, "2026-03-01")

	artist := suite.createArtist("Rising Star")
	suite.addArtistToFestival(f1.ID, artist.ID, "undercard")
	suite.addArtistToFestival(f2.ID, artist.ID, "sub_headliner")

	breakouts, err := suite.svc.GetFestivalBreakouts(f2.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(breakouts)
	suite.Require().NotEmpty(breakouts.Breakouts)

	b := breakouts.Breakouts[0]
	suite.Equal(artist.ID, b.Artist.ID)
	suite.Equal("sub_headliner", b.CurrentTier)
	suite.Greater(b.TierImprovement, 0)
	suite.Greater(b.BreakoutScore, 0.0)
	suite.Len(b.Trajectory, 2)
}

func (suite *FestivalIntelligenceTestSuite) TestGetFestivalBreakouts_FirstAppearanceMilestone() {
	f := suite.createFestival("Debut Fest", "df", 2026, "2026-03-01")
	artist := suite.createArtist("Newcomer")
	suite.addArtistToFestival(f.ID, artist.ID, "local")

	breakouts, err := suite.svc.GetFestivalBreakouts(f.ID)

	suite.Require().NoError(err)
	suite.Require().NotEmpty(breakouts.Milestones)

	m := breakouts.Milestones[0]
	suite.Equal(artist.ID, m.Artist.ID)
	suite.Equal("first_festival_appearance", m.Milestone)
}

func (suite *FestivalIntelligenceTestSuite) TestGetFestivalBreakouts_FirstHeadlinerMilestone() {
	f1 := suite.createFestival("Before Fest", "bf", 2024, "2024-03-01")
	f2 := suite.createFestival("Headliner Fest", "hf", 2026, "2026-03-01")

	artist := suite.createArtist("New Headliner")
	suite.addArtistToFestival(f1.ID, artist.ID, "mid_card")
	suite.addArtistToFestival(f2.ID, artist.ID, "headliner")

	breakouts, err := suite.svc.GetFestivalBreakouts(f2.ID)

	suite.Require().NoError(err)

	// Should have both a breakout and a milestone
	suite.NotEmpty(breakouts.Breakouts)

	foundHeadlinerMilestone := false
	for _, m := range breakouts.Milestones {
		if m.Milestone == "first_headliner" {
			foundHeadlinerMilestone = true
			suite.Equal(artist.ID, m.Artist.ID)
		}
	}
	suite.True(foundHeadlinerMilestone)
}

func (suite *FestivalIntelligenceTestSuite) TestGetFestivalBreakouts_EmptyFestival() {
	f := suite.createFestival("Empty Breakout Fest", "ebf", 2026, "2026-03-01")

	breakouts, err := suite.svc.GetFestivalBreakouts(f.ID)

	suite.Require().NoError(err)
	suite.Empty(breakouts.Breakouts)
	suite.Empty(breakouts.Milestones)
}

func (suite *FestivalIntelligenceTestSuite) TestGetFestivalBreakouts_NotFound() {
	_, err := suite.svc.GetFestivalBreakouts(99999)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "not found")
}

// ─────────────────────────────────────────────
// GetArtistFestivalTrajectory tests
// ─────────────────────────────────────────────

func (suite *FestivalIntelligenceTestSuite) TestGetArtistFestivalTrajectory_MultipleAppearances() {
	f1 := suite.createFestival("Traj Fest 2024", "traj", 2024, "2024-03-01")
	f2 := suite.createFestival("Traj Fest 2025", "traj", 2025, "2025-03-01")
	f3 := suite.createFestival("Traj Fest 2026", "traj", 2026, "2026-03-01")

	artist := suite.createArtist("Trajectory Artist")
	suite.addArtistToFestival(f1.ID, artist.ID, "undercard")
	suite.addArtistToFestival(f2.ID, artist.ID, "mid_card")
	suite.addArtistToFestival(f3.ID, artist.ID, "sub_headliner")

	traj, err := suite.svc.GetArtistFestivalTrajectory(artist.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(traj)
	suite.Equal(artist.ID, traj.Artist.ID)
	suite.Len(traj.Appearances, 3)
	suite.Equal("sub_headliner", traj.BestTier)
	suite.Equal(3, traj.TotalAppearances)
	suite.Greater(traj.BreakoutScore, 0.0)

	// Verify chronological order
	suite.Equal(2024, traj.Appearances[0].Year)
	suite.Equal(2025, traj.Appearances[1].Year)
	suite.Equal(2026, traj.Appearances[2].Year)
}

func (suite *FestivalIntelligenceTestSuite) TestGetArtistFestivalTrajectory_NoAppearances() {
	artist := suite.createArtist("No Fest Artist")

	traj, err := suite.svc.GetArtistFestivalTrajectory(artist.ID)

	suite.Require().NoError(err)
	suite.Empty(traj.Appearances)
	suite.Equal(0, traj.TotalAppearances)
	suite.Equal(0.0, traj.BreakoutScore)
}

func (suite *FestivalIntelligenceTestSuite) TestGetArtistFestivalTrajectory_NotFound() {
	_, err := suite.svc.GetArtistFestivalTrajectory(99999)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "not found")
}

// ─────────────────────────────────────────────
// GetSeriesComparison tests
// ─────────────────────────────────────────────

func (suite *FestivalIntelligenceTestSuite) TestGetSeriesComparison_Success() {
	f1 := suite.createFestival("M3F 2024", "m3f", 2024, "2024-03-01")
	f2 := suite.createFestival("M3F 2025", "m3f", 2025, "2025-03-01")

	// Returning artist
	returning := suite.createArtist("Returning Act")
	suite.addArtistToFestival(f1.ID, returning.ID, "mid_card")
	suite.addArtistToFestival(f2.ID, returning.ID, "sub_headliner")

	// Unique to 2024
	old := suite.createArtist("Old Act")
	suite.addArtistToFestival(f1.ID, old.ID, "undercard")

	// Newcomer in 2025
	newcomer := suite.createArtist("New Act")
	suite.addArtistToFestival(f2.ID, newcomer.ID, "local")

	comparison, err := suite.svc.GetSeriesComparison("m3f", []int{2024, 2025})

	suite.Require().NoError(err)
	suite.Require().NotNil(comparison)
	suite.Equal("m3f", comparison.SeriesSlug)
	suite.Len(comparison.Editions, 2)

	// One returning artist
	suite.Len(comparison.ReturningArtists, 1)
	suite.Equal(returning.ID, comparison.ReturningArtists[0].Artist.ID)
	suite.Contains(comparison.ReturningArtists[0].Tiers, "2024")
	suite.Contains(comparison.ReturningArtists[0].Tiers, "2025")

	// One newcomer
	suite.Len(comparison.Newcomers, 1)
	suite.Equal(newcomer.ID, comparison.Newcomers[0].Artist.ID)

	// Retention: 1 out of 2 returned = 0.5
	suite.Equal(0.5, comparison.RetentionRate)
}

func (suite *FestivalIntelligenceTestSuite) TestGetSeriesComparison_NotEnoughYears() {
	_, err := suite.svc.GetSeriesComparison("m3f", []int{2024})
	suite.Require().Error(err)
	suite.Contains(err.Error(), "at least 2 years")
}

func (suite *FestivalIntelligenceTestSuite) TestGetSeriesComparison_NoFestivals() {
	_, err := suite.svc.GetSeriesComparison("nonexistent", []int{2024, 2025})
	suite.Require().Error(err)
	suite.Contains(err.Error(), "no festivals found")
}

func (suite *FestivalIntelligenceTestSuite) TestGetSeriesComparison_ThreeEditions() {
	f1 := suite.createFestival("Series 2023", "series", 2023, "2023-03-01")
	f2 := suite.createFestival("Series 2024", "series", 2024, "2024-03-01")
	f3 := suite.createFestival("Series 2025", "series", 2025, "2025-03-01")

	// Veteran appears in all 3
	veteran := suite.createArtist("Veteran")
	suite.addArtistToFestival(f1.ID, veteran.ID, "headliner")
	suite.addArtistToFestival(f2.ID, veteran.ID, "headliner")
	suite.addArtistToFestival(f3.ID, veteran.ID, "headliner")

	// Another appears in 2023 and 2025
	sporadic := suite.createArtist("Sporadic")
	suite.addArtistToFestival(f1.ID, sporadic.ID, "mid_card")
	suite.addArtistToFestival(f3.ID, sporadic.ID, "sub_headliner")

	comparison, err := suite.svc.GetSeriesComparison("series", []int{2023, 2024, 2025})

	suite.Require().NoError(err)
	suite.Len(comparison.Editions, 3)

	// Both artists return in at least 2 editions
	suite.Len(comparison.ReturningArtists, 2)

	// Veteran should rank first (3 appearances > 2)
	suite.Equal(veteran.ID, comparison.ReturningArtists[0].Artist.ID)
	suite.Len(comparison.ReturningArtists[0].Years, 3)
}

func (suite *FestivalIntelligenceTestSuite) TestGetSeriesComparison_LineupGrowth() {
	f1 := suite.createFestival("Growth 2024", "growth", 2024, "2024-03-01")
	f2 := suite.createFestival("Growth 2025", "growth", 2025, "2025-03-01")

	// 2 artists in 2024
	a1 := suite.createArtist("Growth A1")
	a2 := suite.createArtist("Growth A2")
	suite.addArtistToFestival(f1.ID, a1.ID, "headliner")
	suite.addArtistToFestival(f1.ID, a2.ID, "mid_card")

	// 4 artists in 2025 (100% growth)
	a3 := suite.createArtist("Growth A3")
	a4 := suite.createArtist("Growth A4")
	suite.addArtistToFestival(f2.ID, a1.ID, "headliner")
	suite.addArtistToFestival(f2.ID, a2.ID, "mid_card")
	suite.addArtistToFestival(f2.ID, a3.ID, "undercard")
	suite.addArtistToFestival(f2.ID, a4.ID, "local")

	comparison, err := suite.svc.GetSeriesComparison("growth", []int{2024, 2025})

	suite.Require().NoError(err)
	suite.Equal(1.0, comparison.LineupGrowth)  // (4-2)/2 = 1.0
	suite.Equal(1.0, comparison.RetentionRate) // Both returned
}
