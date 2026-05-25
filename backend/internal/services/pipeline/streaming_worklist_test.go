package pipeline

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// ──────────────────────────────────────────────
// Integration tests — exercise the LATERAL query + transition matrix
// against a real Postgres database.
// ──────────────────────────────────────────────

type StreamingWorklistIntegrationSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	service *StreamingWorklistService
}

func (s *StreamingWorklistIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.service = NewStreamingWorklistService(s.testDB.DB)
}

func (s *StreamingWorklistIntegrationSuite) TearDownTest() {
	sqlDB, _ := s.testDB.DB.DB()
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func (s *StreamingWorklistIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func TestStreamingWorklistIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(StreamingWorklistIntegrationSuite))
}

// seedArtist inserts an artist with the given status and returns the row.
func (s *StreamingWorklistIntegrationSuite) seedArtist(name string, status catalogm.StreamingDiscoveryStatus) *catalogm.Artist {
	artist := &catalogm.Artist{
		Name:                     name,
		StreamingDiscoveryStatus: status,
	}
	s.Require().NoError(s.testDB.DB.Create(artist).Error)
	return artist
}

// seedShow inserts a show + show_venues + show_artists row connecting
// the given artist to a venue on the given date.
func (s *StreamingWorklistIntegrationSuite) seedShow(title string, eventDate time.Time, artistID uint, venueName, city, state string) *catalogm.Show {
	venue := &catalogm.Venue{
		Name:  venueName,
		City:  city,
		State: state,
	}
	s.Require().NoError(s.testDB.DB.Create(venue).Error)

	show := &catalogm.Show{
		Title:     title,
		EventDate: eventDate,
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.testDB.DB.Create(show).Error)

	s.Require().NoError(s.testDB.DB.Exec(
		"INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)",
		show.ID, venue.ID,
	).Error)
	s.Require().NoError(s.testDB.DB.Exec(
		"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'performer')",
		show.ID, artistID,
	).Error)

	return show
}

// ──────────────────────────────────────────────
// ListStreamingWorklist
// ──────────────────────────────────────────────

func (s *StreamingWorklistIntegrationSuite) TestList_ReturnsNonTerminalWithUpcomingShows() {
	// Seed three artists:
	//   - unreviewed + future show → included
	//   - linked + future show → excluded (terminal)
	//   - unreviewed + only past show → excluded (no upcoming)
	artist1 := s.seedArtist("Active Band", catalogm.StreamingDiscoveryStatusUnreviewed)
	artist2 := s.seedArtist("Linked Band", catalogm.StreamingDiscoveryStatusLinked)
	artist3 := s.seedArtist("Past-Only Band", catalogm.StreamingDiscoveryStatusUnreviewed)

	now := time.Now().UTC()
	s.seedShow("Future Show A", now.Add(48*time.Hour), artist1.ID, "Valley Bar", "Phoenix", "AZ")
	s.seedShow("Future Show B", now.Add(72*time.Hour), artist2.ID, "Crescent Ballroom", "Phoenix", "AZ")
	s.seedShow("Past Show", now.Add(-7*24*time.Hour), artist3.ID, "Rebel Lounge", "Phoenix", "AZ")

	result, err := s.service.ListStreamingWorklist("", 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), result.Total, "expected only the unreviewed-with-upcoming artist")
	s.Len(result.Entries, 1)
	s.Equal("Active Band", result.Entries[0].ArtistName)
	s.Equal("unreviewed", result.Entries[0].StreamingDiscoveryStatus)
	s.NotNil(result.Entries[0].VenueName)
	s.Equal("Valley Bar", *result.Entries[0].VenueName)
}

func (s *StreamingWorklistIntegrationSuite) TestList_OrdersBySoonestThenName() {
	// Three artists, all unreviewed, with future shows at different dates.
	a := s.seedArtist("Charlie Band", catalogm.StreamingDiscoveryStatusUnreviewed)
	b := s.seedArtist("Alpha Band", catalogm.StreamingDiscoveryStatusUnreviewed)
	c := s.seedArtist("Bravo Band", catalogm.StreamingDiscoveryStatusUnreviewed)

	now := time.Now().UTC()
	// Charlie's show is the soonest; Alpha + Bravo tie on date so name decides.
	s.seedShow("Charlie's Show", now.Add(24*time.Hour), a.ID, "V1", "Phoenix", "AZ")
	tieDate := now.Add(72 * time.Hour)
	s.seedShow("Alpha's Show", tieDate, b.ID, "V2", "Phoenix", "AZ")
	s.seedShow("Bravo's Show", tieDate, c.ID, "V3", "Phoenix", "AZ")

	result, err := s.service.ListStreamingWorklist("", 50, 0)
	s.Require().NoError(err)
	s.Require().Len(result.Entries, 3)
	s.Equal("Charlie Band", result.Entries[0].ArtistName, "soonest first")
	s.Equal("Alpha Band", result.Entries[1].ArtistName, "tie broken by name")
	s.Equal("Bravo Band", result.Entries[2].ArtistName)
}

func (s *StreamingWorklistIntegrationSuite) TestList_StatusFilter() {
	a := s.seedArtist("Unreviewed Band", catalogm.StreamingDiscoveryStatusUnreviewed)
	b := s.seedArtist("Candidates Band", catalogm.StreamingDiscoveryStatusCandidatesPending)

	now := time.Now().UTC()
	s.seedShow("Show A", now.Add(48*time.Hour), a.ID, "V1", "Phoenix", "AZ")
	s.seedShow("Show B", now.Add(72*time.Hour), b.ID, "V2", "Phoenix", "AZ")

	// No filter → both
	all, err := s.service.ListStreamingWorklist("", 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(2), all.Total)

	// Filter unreviewed only
	unrev, err := s.service.ListStreamingWorklist("unreviewed", 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), unrev.Total)
	s.Len(unrev.Entries, 1)
	s.Equal("Unreviewed Band", unrev.Entries[0].ArtistName)

	// Filter candidates_pending only
	cp, err := s.service.ListStreamingWorklist("candidates_pending", 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), cp.Total)
	s.Equal("Candidates Band", cp.Entries[0].ArtistName)
}

func (s *StreamingWorklistIntegrationSuite) TestList_RejectsTerminalStatusFilter() {
	_, err := s.service.ListStreamingWorklist("linked", 50, 0)
	s.Require().Error(err)
	s.True(errors.Is(err, contracts.ErrInvalidStreamingStatusTransition))
}

func (s *StreamingWorklistIntegrationSuite) TestList_UpcomingShowCount() {
	a := s.seedArtist("Busy Band", catalogm.StreamingDiscoveryStatusUnreviewed)
	now := time.Now().UTC()
	s.seedShow("Show 1", now.Add(24*time.Hour), a.ID, "V1", "Phoenix", "AZ")
	s.seedShow("Show 2", now.Add(48*time.Hour), a.ID, "V2", "Phoenix", "AZ")
	s.seedShow("Show 3", now.Add(72*time.Hour), a.ID, "V3", "Phoenix", "AZ")
	// Past show should NOT count.
	s.seedShow("Past Show", now.Add(-48*time.Hour), a.ID, "V4", "Phoenix", "AZ")

	result, err := s.service.ListStreamingWorklist("", 50, 0)
	s.Require().NoError(err)
	s.Require().Len(result.Entries, 1)
	s.Equal(int64(3), result.Entries[0].UpcomingShowCount)
}

func (s *StreamingWorklistIntegrationSuite) TestList_LimitOffset() {
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		a := s.seedArtist(fmt.Sprintf("Band %02d", i), catalogm.StreamingDiscoveryStatusUnreviewed)
		// Stagger event dates so order is deterministic.
		s.seedShow(fmt.Sprintf("Show %02d", i), now.Add(time.Duration(i+1)*24*time.Hour), a.ID, fmt.Sprintf("V%d", i), "Phoenix", "AZ")
	}

	page1, err := s.service.ListStreamingWorklist("", 2, 0)
	s.Require().NoError(err)
	s.Equal(int64(5), page1.Total)
	s.Len(page1.Entries, 2)
	s.Equal("Band 00", page1.Entries[0].ArtistName)
	s.Equal("Band 01", page1.Entries[1].ArtistName)

	page2, err := s.service.ListStreamingWorklist("", 2, 2)
	s.Require().NoError(err)
	s.Equal(int64(5), page2.Total)
	s.Len(page2.Entries, 2)
	s.Equal("Band 02", page2.Entries[0].ArtistName)
	s.Equal("Band 03", page2.Entries[1].ArtistName)
}

func (s *StreamingWorklistIntegrationSuite) TestList_ClampsLimit() {
	a := s.seedArtist("X", catalogm.StreamingDiscoveryStatusUnreviewed)
	s.seedShow("S", time.Now().UTC().Add(24*time.Hour), a.ID, "V", "Phoenix", "AZ")

	// limit=0 falls back to default (50)
	_, err := s.service.ListStreamingWorklist("", 0, 0)
	s.Require().NoError(err)

	// limit > 200 clamps to 200; we just want no error here.
	_, err = s.service.ListStreamingWorklist("", 500, -10)
	s.Require().NoError(err)
}

// ──────────────────────────────────────────────
// UpdateStreamingDiscoveryStatus
// ──────────────────────────────────────────────

func (s *StreamingWorklistIntegrationSuite) TestUpdate_HappyPath_UnreviewedToLinked() {
	a := s.seedArtist("Band", catalogm.StreamingDiscoveryStatusUnreviewed)

	resp, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "linked",
	})
	s.Require().NoError(err)
	s.Equal(a.ID, resp.ID)
	s.Equal("linked", resp.StreamingDiscoveryStatus)
	s.Nil(resp.StreamingDiscoveryReason, "no reason persisted for linked")

	// DB row should reflect the change.
	var reloaded catalogm.Artist
	s.Require().NoError(s.testDB.DB.First(&reloaded, a.ID).Error)
	s.Equal(catalogm.StreamingDiscoveryStatusLinked, reloaded.StreamingDiscoveryStatus)
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_PersistsReason() {
	a := s.seedArtist("Band", catalogm.StreamingDiscoveryStatusCandidatesPending)
	reason := "All candidates were a different band with the same name"

	resp, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "no_links_found",
		Reason:   &reason,
	})
	s.Require().NoError(err)
	s.NotNil(resp.StreamingDiscoveryReason)
	s.Equal(reason, *resp.StreamingDiscoveryReason)
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_EmptyReasonPersistsAsNull() {
	a := s.seedArtist("Band", catalogm.StreamingDiscoveryStatusUnreviewed)
	empty := ""

	resp, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "skipped",
		Reason:   &empty,
	})
	s.Require().NoError(err)
	s.Nil(resp.StreamingDiscoveryReason)
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_ReopenClearsReason() {
	// Seed an artist already in a terminal state with a reason.
	a := s.seedArtist("Band", catalogm.StreamingDiscoveryStatusSkipped)
	priorReason := "deferred to next sweep"
	s.Require().NoError(s.testDB.DB.Model(&catalogm.Artist{}).Where("id = ?", a.ID).Update("streaming_discovery_reason", priorReason).Error)

	// Re-open with no reason in the request.
	resp, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "unreviewed",
	})
	s.Require().NoError(err)
	s.Equal("unreviewed", resp.StreamingDiscoveryStatus)
	s.Nil(resp.StreamingDiscoveryReason, "reason should be cleared on re-open")
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_RejectsTerminalToDifferentTerminal() {
	a := s.seedArtist("Band", catalogm.StreamingDiscoveryStatusLinked)

	_, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "skipped",
	})
	s.Require().Error(err)
	s.True(errors.Is(err, contracts.ErrInvalidStreamingStatusTransition))
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_RejectsTerminalToCandidatesPending() {
	a := s.seedArtist("Band", catalogm.StreamingDiscoveryStatusUnreviewed)

	_, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "candidates_pending",
	})
	s.Require().Error(err)
	s.True(errors.Is(err, contracts.ErrInvalidStreamingStatusTransition))
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_RejectsSameState() {
	a := s.seedArtist("Band", catalogm.StreamingDiscoveryStatusUnreviewed)

	_, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "unreviewed",
	})
	s.Require().Error(err)
	s.True(errors.Is(err, contracts.ErrInvalidStreamingStatusTransition))
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_RejectsUnknownStatus() {
	a := s.seedArtist("Band", catalogm.StreamingDiscoveryStatusUnreviewed)

	_, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "made-up-status",
	})
	s.Require().Error(err)
	s.True(errors.Is(err, contracts.ErrInvalidStreamingStatusTransition))
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_NotFound() {
	_, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: 99999,
		Status:   "linked",
	})
	s.Require().Error(err)
	s.True(errors.Is(err, contracts.ErrStreamingArtistNotFound))
}

func (s *StreamingWorklistIntegrationSuite) TestUpdate_ZeroArtistID() {
	_, err := s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: 0,
		Status:   "linked",
	})
	s.Require().Error(err)
}

// ──────────────────────────────────────────────
// End-to-end: terminal status excludes from worklist
// ──────────────────────────────────────────────

func (s *StreamingWorklistIntegrationSuite) TestEndToEnd_DecisionRemovesFromWorklist() {
	a := s.seedArtist("Triage Me", catalogm.StreamingDiscoveryStatusUnreviewed)
	s.seedShow("Future", time.Now().UTC().Add(48*time.Hour), a.ID, "V", "Phoenix", "AZ")

	before, err := s.service.ListStreamingWorklist("", 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), before.Total)

	_, err = s.service.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: a.ID,
		Status:   "linked",
	})
	s.Require().NoError(err)

	after, err := s.service.ListStreamingWorklist("", 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(0), after.Total)
}
