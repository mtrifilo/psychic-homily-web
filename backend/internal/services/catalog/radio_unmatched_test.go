package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestRadioService_NilDB_UnmatchedMethods(t *testing.T) {
	svc := &RadioService{db: nil}

	assertNilDBError(t, func() error {
		_, _, err := svc.GetUnmatchedPlays(0, 10, 0)
		return err
	})
	assertNilDBError(t, func() error {
		return svc.LinkPlay(1, &contracts.LinkPlayRequest{})
	})
	assertNilDBError(t, func() error {
		_, err := svc.BulkLinkPlays(&contracts.BulkLinkRequest{ArtistName: "test", ArtistID: 1})
		return err
	})
	assertNilDBError(t, func() error {
		return svc.ComputeAffinity()
	})
	assertNilDBError(t, func() error {
		_, err := svc.ReMatchUnmatched()
		return err
	})
	assertNilDBError(t, func() error {
		_, err := svc.GetActiveStationsWithPlaylistSource()
		return err
	})
}

// =============================================================================
// INTEGRATION TESTS (Require Database)
// =============================================================================

type RadioUnmatchedSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *RadioService
}

func TestRadioUnmatchedSuite(t *testing.T) {
	suite.Run(t, new(RadioUnmatchedSuite))
}

func (s *RadioUnmatchedSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = &RadioService{db: s.db}
}

func (s *RadioUnmatchedSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RadioUnmatchedSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM radio_artist_affinity")
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM artist_aliases")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

// createTestStation creates a station for testing.
func (s *RadioUnmatchedSuite) createTestStation(name, slug, source string) *catalogm.RadioStation {
	station := &catalogm.RadioStation{
		Name:           name,
		Slug:           slug,
		BroadcastType:  "internet",
		PlaylistSource: &source,
	}
	s.Require().NoError(s.db.Create(station).Error)
	return station
}

// createTestShow creates a radio show for testing.
func (s *RadioUnmatchedSuite) createTestShow(stationID uint, name, slug string) *catalogm.RadioShow {
	show := &catalogm.RadioShow{
		StationID: stationID,
		Name:      name,
		Slug:      slug,
	}
	s.Require().NoError(s.db.Create(show).Error)
	return show
}

// createTestEpisode creates a radio episode for testing.
func (s *RadioUnmatchedSuite) createTestEpisode(showID uint, airDate string) *catalogm.RadioEpisode {
	ep := &catalogm.RadioEpisode{
		ShowID:  showID,
		AirDate: airDate,
	}
	s.Require().NoError(s.db.Create(ep).Error)
	return ep
}

// createTestPlay creates a radio play for testing.
func (s *RadioUnmatchedSuite) createTestPlay(episodeID uint, artistName string, artistID *uint) *catalogm.RadioPlay {
	play := &catalogm.RadioPlay{
		EpisodeID:  episodeID,
		ArtistName: artistName,
		ArtistID:   artistID,
		Position:   0,
	}
	s.Require().NoError(s.db.Create(play).Error)
	return play
}

// createTestArtist creates an artist for testing.
func (s *RadioUnmatchedSuite) createTestArtist(name, slug string) *catalogm.Artist {
	artist := &catalogm.Artist{
		Name: name,
		Slug: &slug,
	}
	s.Require().NoError(s.db.Create(artist).Error)
	return artist
}

// ── GetUnmatchedPlays ──

func (s *RadioUnmatchedSuite) TestGetUnmatchedPlays_GroupsByArtistName() {
	station := s.createTestStation("KEXP", "kexp-g", "kexp_api")
	show := s.createTestShow(station.ID, "Morning Show", "morning-show-g")
	ep := s.createTestEpisode(show.ID, "2026-01-01")

	// Create 3 unmatched plays for "Radiohead" and 1 for "Bjork"
	s.createTestPlay(ep.ID, "Radiohead", nil)
	s.createTestPlay(ep.ID, "Radiohead", nil)
	s.createTestPlay(ep.ID, "Radiohead", nil)
	s.createTestPlay(ep.ID, "Bjork", nil)

	groups, total, err := s.svc.GetUnmatchedPlays(0, 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(2), total)
	s.Require().Len(groups, 2)

	// Should be ordered by play_count DESC
	s.Equal("Radiohead", groups[0].ArtistName)
	s.Equal(3, groups[0].PlayCount)
	s.Equal("Bjork", groups[1].ArtistName)
	s.Equal(1, groups[1].PlayCount)
}

func (s *RadioUnmatchedSuite) TestGetUnmatchedPlays_FilterByStation() {
	station1 := s.createTestStation("KEXP", "kexp-f", "kexp_api")
	station2 := s.createTestStation("WFMU", "wfmu-f", "wfmu_scrape")

	show1 := s.createTestShow(station1.ID, "Show 1", "show-1-f")
	show2 := s.createTestShow(station2.ID, "Show 2", "show-2-f")

	ep1 := s.createTestEpisode(show1.ID, "2026-01-01")
	ep2 := s.createTestEpisode(show2.ID, "2026-01-01")

	s.createTestPlay(ep1.ID, "KEXP Artist", nil)
	s.createTestPlay(ep2.ID, "WFMU Artist", nil)

	// Filter by station 1
	groups, total, err := s.svc.GetUnmatchedPlays(station1.ID, 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(groups, 1)
	s.Equal("KEXP Artist", groups[0].ArtistName)
}

func (s *RadioUnmatchedSuite) TestGetUnmatchedPlays_IncludesStationNames() {
	station := s.createTestStation("KEXP", "kexp-sn", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-sn")
	ep := s.createTestEpisode(show.ID, "2026-01-01")
	s.createTestPlay(ep.ID, "Test Artist", nil)

	groups, _, err := s.svc.GetUnmatchedPlays(0, 50, 0)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Contains(groups[0].StationNames, "KEXP")
}

func (s *RadioUnmatchedSuite) TestGetUnmatchedPlays_SuggestedMatches() {
	station := s.createTestStation("KEXP", "kexp-sm", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-sm")
	ep := s.createTestEpisode(show.ID, "2026-01-01")

	// Create an artist in our DB
	s.createTestArtist("Radiohead", "radiohead-sm")

	// Create an unmatched play with matching name
	s.createTestPlay(ep.ID, "Radiohead", nil)

	groups, _, err := s.svc.GetUnmatchedPlays(0, 50, 0)
	s.Require().NoError(err)
	s.Require().Len(groups, 1)
	s.Require().NotEmpty(groups[0].SuggestedMatches)
	s.Equal("Radiohead", groups[0].SuggestedMatches[0].ArtistName)
}

func (s *RadioUnmatchedSuite) TestGetUnmatchedPlays_ExcludesMatched() {
	station := s.createTestStation("KEXP", "kexp-em", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-em")
	ep := s.createTestEpisode(show.ID, "2026-01-01")
	artist := s.createTestArtist("Matched Artist", "matched-artist-em")

	// One matched, one unmatched
	s.createTestPlay(ep.ID, "Matched Artist", &artist.ID)
	s.createTestPlay(ep.ID, "Unmatched Artist", nil)

	groups, total, err := s.svc.GetUnmatchedPlays(0, 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(groups, 1)
	s.Equal("Unmatched Artist", groups[0].ArtistName)
}

// ── LinkPlay ──

func (s *RadioUnmatchedSuite) TestLinkPlay_Success() {
	station := s.createTestStation("KEXP", "kexp-lp", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-lp")
	ep := s.createTestEpisode(show.ID, "2026-01-01")
	artist := s.createTestArtist("Test Artist", "test-artist-lp")
	play := s.createTestPlay(ep.ID, "Test Artist", nil)

	err := s.svc.LinkPlay(play.ID, &contracts.LinkPlayRequest{ArtistID: &artist.ID})
	s.Require().NoError(err)

	// Verify the play is now linked
	var updated catalogm.RadioPlay
	s.db.First(&updated, play.ID)
	s.Require().NotNil(updated.ArtistID)
	s.Equal(artist.ID, *updated.ArtistID)
}

func (s *RadioUnmatchedSuite) TestLinkPlay_NoFieldsToUpdate() {
	station := s.createTestStation("KEXP", "kexp-nf", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-nf")
	ep := s.createTestEpisode(show.ID, "2026-01-01")
	play := s.createTestPlay(ep.ID, "Test Artist", nil)

	err := s.svc.LinkPlay(play.ID, &contracts.LinkPlayRequest{})
	s.Require().Error(err)
	s.Contains(err.Error(), "no fields to update")
}

// ── BulkLinkPlays ──

func (s *RadioUnmatchedSuite) TestBulkLinkPlays_UpdatesCorrectPlays() {
	station := s.createTestStation("KEXP", "kexp-bl", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-bl")
	ep := s.createTestEpisode(show.ID, "2026-01-01")
	artist := s.createTestArtist("Radiohead", "radiohead-bl")

	// Create 3 unmatched plays for "Radiohead" and 1 for "Bjork"
	s.createTestPlay(ep.ID, "Radiohead", nil)
	s.createTestPlay(ep.ID, "Radiohead", nil)
	s.createTestPlay(ep.ID, "Radiohead", nil)
	s.createTestPlay(ep.ID, "Bjork", nil)

	result, err := s.svc.BulkLinkPlays(&contracts.BulkLinkRequest{
		ArtistName: "Radiohead",
		ArtistID:   artist.ID,
	})
	s.Require().NoError(err)
	s.Equal(3, result.Updated)

	// Verify only Radiohead plays were updated
	var radioheadPlays []catalogm.RadioPlay
	s.db.Where("artist_name = ? AND artist_id IS NOT NULL", "Radiohead").Find(&radioheadPlays)
	s.Len(radioheadPlays, 3)

	// Bjork play should still be unmatched
	var bjorkPlays []catalogm.RadioPlay
	s.db.Where("artist_name = ? AND artist_id IS NULL", "Bjork").Find(&bjorkPlays)
	s.Len(bjorkPlays, 1)
}

func (s *RadioUnmatchedSuite) TestBulkLinkPlays_DoesNotUpdateAlreadyMatched() {
	station := s.createTestStation("KEXP", "kexp-nm", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-nm")
	ep := s.createTestEpisode(show.ID, "2026-01-01")
	artist1 := s.createTestArtist("Radiohead", "radiohead-nm")
	artist2 := s.createTestArtist("Radiohead Tribute", "radiohead-tribute-nm")

	// One already matched, two unmatched
	s.createTestPlay(ep.ID, "Radiohead", &artist2.ID) // already matched to wrong artist
	s.createTestPlay(ep.ID, "Radiohead", nil)
	s.createTestPlay(ep.ID, "Radiohead", nil)

	result, err := s.svc.BulkLinkPlays(&contracts.BulkLinkRequest{
		ArtistName: "Radiohead",
		ArtistID:   artist1.ID,
	})
	s.Require().NoError(err)
	s.Equal(2, result.Updated) // Only the 2 unmatched ones
}

// ── ComputeAffinity ──

func (s *RadioUnmatchedSuite) TestComputeAffinity_TwoCoOccurringArtists() {
	station := s.createTestStation("KEXP", "kexp-af", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-af")

	artist1 := s.createTestArtist("Artist A", "artist-a-af")
	artist2 := s.createTestArtist("Artist B", "artist-b-af")

	// Ensure canonical ordering for verification
	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// Create 2 episodes where both artists co-occur (need >= 2 for threshold)
	ep1 := s.createTestEpisode(show.ID, "2026-01-01")
	s.createTestPlay(ep1.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep1.ID, "Artist B", &artist2.ID)

	ep2 := s.createTestEpisode(show.ID, "2026-01-02")
	s.createTestPlay(ep2.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep2.ID, "Artist B", &artist2.ID)

	err := s.svc.ComputeAffinity()
	s.Require().NoError(err)

	// Should have one affinity row with canonical ordering
	var affinity catalogm.RadioArtistAffinity
	err = s.db.Where("artist_a_id = ? AND artist_b_id = ?", lowID, highID).First(&affinity).Error
	s.Require().NoError(err)
	s.Equal(2, affinity.CoOccurrenceCount)
	s.Equal(1, affinity.StationCount)
}

func (s *RadioUnmatchedSuite) TestComputeAffinity_CanonicalOrdering() {
	station := s.createTestStation("KEXP", "kexp-co", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-co")

	// Create artists — the IDs determine canonical ordering
	artist1 := s.createTestArtist("Artist A", "artist-a-co")
	artist2 := s.createTestArtist("Artist B", "artist-b-co")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	ep1 := s.createTestEpisode(show.ID, "2026-01-01")
	s.createTestPlay(ep1.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep1.ID, "Artist B", &artist2.ID)

	ep2 := s.createTestEpisode(show.ID, "2026-01-02")
	s.createTestPlay(ep2.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep2.ID, "Artist B", &artist2.ID)

	err := s.svc.ComputeAffinity()
	s.Require().NoError(err)

	// Should be stored with canonical ordering (lower ID first)
	var affinity catalogm.RadioArtistAffinity
	err = s.db.Where("artist_a_id = ? AND artist_b_id = ?", lowID, highID).First(&affinity).Error
	s.Require().NoError(err)
	s.Equal(lowID, affinity.ArtistAID)
	s.Equal(highID, affinity.ArtistBID)
}

func (s *RadioUnmatchedSuite) TestComputeAffinity_MinimumThreshold() {
	station := s.createTestStation("KEXP", "kexp-mt", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-mt")
	artist1 := s.createTestArtist("Artist A", "artist-a-mt")
	artist2 := s.createTestArtist("Artist B", "artist-b-mt")

	// Only 1 co-occurrence (below threshold of 2)
	ep := s.createTestEpisode(show.ID, "2026-01-01")
	s.createTestPlay(ep.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep.ID, "Artist B", &artist2.ID)

	err := s.svc.ComputeAffinity()
	s.Require().NoError(err)

	// Should NOT have an affinity row (threshold is 2)
	var count int64
	s.db.Model(&catalogm.RadioArtistAffinity{}).Count(&count)
	s.Equal(int64(0), count)
}

func (s *RadioUnmatchedSuite) TestComputeAffinity_CrossStationWeighting() {
	station1 := s.createTestStation("KEXP", "kexp-cs", "kexp_api")
	station2 := s.createTestStation("WFMU", "wfmu-cs", "wfmu_scrape")

	show1 := s.createTestShow(station1.ID, "KEXP Show", "kexp-show-cs")
	show2 := s.createTestShow(station2.ID, "WFMU Show", "wfmu-show-cs")

	artist1 := s.createTestArtist("Artist A", "artist-a-cs")
	artist2 := s.createTestArtist("Artist B", "artist-b-cs")

	lowID, highID := artist1.ID, artist2.ID
	if lowID > highID {
		lowID, highID = highID, lowID
	}

	// Co-occurrence on station 1
	ep1 := s.createTestEpisode(show1.ID, "2026-01-01")
	s.createTestPlay(ep1.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep1.ID, "Artist B", &artist2.ID)

	// Co-occurrence on station 2
	ep2 := s.createTestEpisode(show2.ID, "2026-01-01")
	s.createTestPlay(ep2.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep2.ID, "Artist B", &artist2.ID)

	err := s.svc.ComputeAffinity()
	s.Require().NoError(err)

	var affinity catalogm.RadioArtistAffinity
	err = s.db.Where("artist_a_id = ? AND artist_b_id = ?", lowID, highID).First(&affinity).Error
	s.Require().NoError(err)
	s.Equal(2, affinity.CoOccurrenceCount)
	s.Equal(2, affinity.StationCount) // 2 different stations
}

func (s *RadioUnmatchedSuite) TestComputeAffinity_TruncateAndRecompute() {
	station := s.createTestStation("KEXP", "kexp-tr", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-tr")
	artist1 := s.createTestArtist("Artist A", "artist-a-tr")
	artist2 := s.createTestArtist("Artist B", "artist-b-tr")

	ep1 := s.createTestEpisode(show.ID, "2026-01-01")
	s.createTestPlay(ep1.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep1.ID, "Artist B", &artist2.ID)

	ep2 := s.createTestEpisode(show.ID, "2026-01-02")
	s.createTestPlay(ep2.ID, "Artist A", &artist1.ID)
	s.createTestPlay(ep2.ID, "Artist B", &artist2.ID)

	// First computation
	err := s.svc.ComputeAffinity()
	s.Require().NoError(err)

	var count1 int64
	s.db.Model(&catalogm.RadioArtistAffinity{}).Count(&count1)
	s.Equal(int64(1), count1)

	// Second computation should give same result (truncate + recompute)
	err = s.svc.ComputeAffinity()
	s.Require().NoError(err)

	var count2 int64
	s.db.Model(&catalogm.RadioArtistAffinity{}).Count(&count2)
	s.Equal(int64(1), count2)
}

// ── ReMatchUnmatched ──

func (s *RadioUnmatchedSuite) TestReMatchUnmatched_CatchesNewArtist() {
	station := s.createTestStation("KEXP", "kexp-rm", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-rm")
	ep := s.createTestEpisode(show.ID, "2026-01-01")

	// Create an unmatched play
	s.createTestPlay(ep.ID, "New Artist", nil)

	// No artist exists yet — re-match should find nothing
	result, err := s.svc.ReMatchUnmatched()
	s.Require().NoError(err)
	s.Equal(1, result.Total)
	s.Equal(0, result.Matched)
	s.Equal(1, result.Unmatched)

	// Now add the artist
	s.createTestArtist("New Artist", "new-artist-rm")

	// Re-match should now find it
	result2, err := s.svc.ReMatchUnmatched()
	s.Require().NoError(err)
	s.Equal(1, result2.Total)
	s.Equal(1, result2.Matched)
}

// ── GetActiveStationsWithPlaylistSource ──

func (s *RadioUnmatchedSuite) TestGetActiveStationsWithPlaylistSource() {
	kexpSrc := "kexp_api"
	s.createTestStation("KEXP", "kexp-asps", kexpSrc)

	// Inactive station — need to create active first, then update to inactive
	inactive := &catalogm.RadioStation{
		Name:           "Inactive",
		Slug:           "inactive-asps",
		BroadcastType:  "internet",
		PlaylistSource: &kexpSrc,
		IsActive:       true,
	}
	s.db.Create(inactive)
	s.db.Model(inactive).Update("is_active", false)

	// Station without playlist source
	s.db.Create(&catalogm.RadioStation{
		Name:          "No Source",
		Slug:          "no-source-asps",
		BroadcastType: "internet",
	})

	stations, err := s.svc.GetActiveStationsWithPlaylistSource()
	s.Require().NoError(err)
	s.Len(stations, 1)
	s.Equal("KEXP", stations[0].Name)
}

// ── Pagination ──

func (s *RadioUnmatchedSuite) TestGetUnmatchedPlays_Pagination() {
	station := s.createTestStation("KEXP", "kexp-pg", "kexp_api")
	show := s.createTestShow(station.ID, "Show", "show-pg")
	ep := s.createTestEpisode(show.ID, "2026-01-01")

	// Create plays for 5 different artists
	for i := 0; i < 5; i++ {
		name := "PaginationArtist" + time.Now().Format("150405.000") + string(rune('A'+i))
		s.createTestPlay(ep.ID, name, nil)
	}

	// Get first page of 2
	groups, total, err := s.svc.GetUnmatchedPlays(0, 2, 0)
	s.Require().NoError(err)
	s.Equal(int64(5), total)
	s.Len(groups, 2)

	// Get second page
	groups2, _, err := s.svc.GetUnmatchedPlays(0, 2, 2)
	s.Require().NoError(err)
	s.Len(groups2, 2)

	// Verify different results
	assert.NotEqual(s.T(), groups[0].ArtistName, groups2[0].ArtistName)
}
