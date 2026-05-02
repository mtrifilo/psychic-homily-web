package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// ArtistBillCompositionIntegrationSuite covers the PSY-364 bill-composition endpoint.
//
// Bill composition is a derived view over `show_artists.position` + `set_type`. These tests
// seed shows with explicit role markers and assert the handler buckets co-bill rows correctly
// into opens-with / closes-with / mini-graph nodes, respects the <3-shows threshold, and
// applies the months time filter consistently across stats and aggregation.
type ArtistBillCompositionIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *ArtistRelationshipHandler
}

func (s *ArtistBillCompositionIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewArtistRelationshipHandler(s.deps.ArtistRelationshipService, s.deps.AuditLogService)
}

func (s *ArtistBillCompositionIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *ArtistBillCompositionIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestArtistBillCompositionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ArtistBillCompositionIntegrationSuite))
}

// seedShowWithBill creates an approved show on `eventDate` where each (artistID, position, setType)
// triple becomes a row in show_artists. Returns the show ID.
func (s *ArtistBillCompositionIntegrationSuite) seedShowWithBill(title string, eventDate time.Time, lineup []billLineupEntry) uint {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, fmt.Sprintf("Venue for %s", title), "Phoenix", "AZ")

	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)

	for _, entry := range lineup {
		s.deps.DB.Exec(
			"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, ?, ?)",
			show.ID, entry.artistID, entry.position, entry.setType,
		)
	}
	return show.ID
}

type billLineupEntry struct {
	artistID uint
	position int
	setType  string
}

// --- Below threshold ---

func (s *ArtistBillCompositionIntegrationSuite) TestBillComposition_BelowThreshold_HidesContent() {
	a := s.deps.ArtistRelationshipService
	headliner := s.createArtist("Headliner")
	opener := s.createArtist("Opener")

	// Only 2 shows — below the 3-show threshold.
	now := time.Now().UTC()
	s.seedShowWithBill("Show 1", now.AddDate(0, -1, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener, 1, "opener"},
	})
	s.seedShowWithBill("Show 2", now.AddDate(0, -2, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener, 1, "opener"},
	})

	bc, err := a.GetArtistBillComposition(headliner, 0)
	s.Require().NoError(err)
	s.True(bc.BelowThreshold)
	s.Equal(2, bc.Stats.TotalShows)
	s.Empty(bc.OpensWith)
	s.Empty(bc.ClosesWith)
	s.Empty(bc.Graph.Nodes)
	s.Empty(bc.Graph.Links)
}

// --- Pure headliner: opens-with populated, closes-with empty ---

func (s *ArtistBillCompositionIntegrationSuite) TestBillComposition_PureHeadliner_OpensWithOnly() {
	a := s.deps.ArtistRelationshipService
	headliner := s.createArtist("HL")
	opener1 := s.createArtist("Opener1")
	opener2 := s.createArtist("Opener2")

	now := time.Now().UTC()
	// 4 shows where headliner anchors and Opener1 supports 3x, Opener2 supports 1x.
	s.seedShowWithBill("Show A", now.AddDate(0, -1, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener1, 1, "opener"},
	})
	s.seedShowWithBill("Show B", now.AddDate(0, -2, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener1, 1, "opener"},
	})
	s.seedShowWithBill("Show C", now.AddDate(0, -3, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener1, 1, "opener"},
	})
	s.seedShowWithBill("Show D", now.AddDate(0, -4, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener2, 1, "opener"},
	})

	bc, err := a.GetArtistBillComposition(headliner, 0)
	s.Require().NoError(err)
	s.False(bc.BelowThreshold)
	s.Equal(4, bc.Stats.TotalShows)
	s.Equal(4, bc.Stats.HeadlinerCount)
	s.Equal(0, bc.Stats.OpenerCount)

	// OpensWith is sorted by shared_count desc.
	s.Require().Len(bc.OpensWith, 2)
	s.Equal(opener1, bc.OpensWith[0].Artist.ID)
	s.Equal(3, bc.OpensWith[0].SharedCount)
	s.Equal(opener2, bc.OpensWith[1].Artist.ID)
	s.Equal(1, bc.OpensWith[1].SharedCount)

	// ClosesWith stays empty — this artist never opened.
	s.Empty(bc.ClosesWith)

	// Graph: 2 co-bill nodes, ≥2 links from center.
	s.Len(bc.Graph.Nodes, 2)
	s.GreaterOrEqual(len(bc.Graph.Links), 2)
}

// --- Mixed roles: both opens-with and closes-with populated, plus cross-connection ---

func (s *ArtistBillCompositionIntegrationSuite) TestBillComposition_MixedRoles_BothTablesAndCrossConnection() {
	a := s.deps.ArtistRelationshipService
	center := s.createArtist("Center")
	opener := s.createArtist("CenterOpener") // opens for Center
	bigger := s.createArtist("BiggerAct")    // headlines above Center

	now := time.Now().UTC()
	// 2 shows: center headlines, opener supports
	s.seedShowWithBill("Center HL 1", now.AddDate(0, -1, 0), []billLineupEntry{
		{center, 0, "headliner"}, {opener, 1, "opener"},
	})
	s.seedShowWithBill("Center HL 2", now.AddDate(0, -2, 0), []billLineupEntry{
		{center, 0, "headliner"}, {opener, 1, "opener"},
	})
	// 2 shows: bigger act headlines, center opens
	s.seedShowWithBill("Bigger HL 1", now.AddDate(0, -3, 0), []billLineupEntry{
		{bigger, 0, "headliner"}, {center, 1, "opener"},
	})
	s.seedShowWithBill("Bigger HL 2", now.AddDate(0, -4, 0), []billLineupEntry{
		{bigger, 0, "headliner"}, {center, 1, "opener"},
	})
	// 1 show that does NOT include center: opener and bigger share a bill.
	// This becomes a graph cross-connection between the two co-bill nodes.
	s.seedShowWithBill("BiggerOpener share", now.AddDate(0, -5, 0), []billLineupEntry{
		{bigger, 0, "headliner"}, {opener, 1, "opener"},
	})

	bc, err := a.GetArtistBillComposition(center, 0)
	s.Require().NoError(err)
	s.False(bc.BelowThreshold)
	s.Equal(4, bc.Stats.TotalShows)
	s.Equal(2, bc.Stats.HeadlinerCount)
	s.Equal(2, bc.Stats.OpenerCount)

	// OpensWith
	s.Require().Len(bc.OpensWith, 1)
	s.Equal(opener, bc.OpensWith[0].Artist.ID)
	s.Equal(2, bc.OpensWith[0].SharedCount)

	// ClosesWith
	s.Require().Len(bc.ClosesWith, 1)
	s.Equal(bigger, bc.ClosesWith[0].Artist.ID)
	s.Equal(2, bc.ClosesWith[0].SharedCount)

	// Graph: 2 co-bill nodes; cross-connection edge between opener and bigger.
	s.Len(bc.Graph.Nodes, 2)
	hasCrossConnection := false
	for _, link := range bc.Graph.Links {
		if (link.SourceID == opener && link.TargetID == bigger) ||
			(link.SourceID == bigger && link.TargetID == opener) {
			hasCrossConnection = true
		}
	}
	s.True(hasCrossConnection, "expected a cross-connection edge between opener and bigger via the share show")
}

// --- Time filter excludes old shows ---

func (s *ArtistBillCompositionIntegrationSuite) TestBillComposition_TimeFilter_ExcludesOldShows() {
	a := s.deps.ArtistRelationshipService
	headliner := s.createArtist("HL2")
	opener := s.createArtist("Opener3")

	now := time.Now().UTC()
	// 2 recent shows (within 12 months) + 2 old shows (>13 months)
	s.seedShowWithBill("Recent 1", now.AddDate(0, -2, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener, 1, "opener"},
	})
	s.seedShowWithBill("Recent 2", now.AddDate(0, -6, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener, 1, "opener"},
	})
	s.seedShowWithBill("Recent 3", now.AddDate(0, -10, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener, 1, "opener"},
	})
	s.seedShowWithBill("Old 1", now.AddDate(-2, 0, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener, 1, "opener"},
	})
	s.seedShowWithBill("Old 2", now.AddDate(-3, 0, 0), []billLineupEntry{
		{headliner, 0, "headliner"}, {opener, 1, "opener"},
	})

	// All-time: 5 shows
	all, err := a.GetArtistBillComposition(headliner, 0)
	s.Require().NoError(err)
	s.Equal(5, all.Stats.TotalShows)
	s.Require().Len(all.OpensWith, 1)
	s.Equal(5, all.OpensWith[0].SharedCount)

	// 12-month window: 3 shows
	recent, err := a.GetArtistBillComposition(headliner, 12)
	s.Require().NoError(err)
	s.Equal(3, recent.Stats.TotalShows)
	s.Require().Len(recent.OpensWith, 1)
	s.Equal(3, recent.OpensWith[0].SharedCount)
	s.Equal(12, recent.TimeFilterMonths)
}

// --- Artist not found ---

func (s *ArtistBillCompositionIntegrationSuite) TestBillComposition_ArtistNotFound() {
	a := s.deps.ArtistRelationshipService
	_, err := a.GetArtistBillComposition(99999, 0)
	s.Error(err)
}

// --- Helper: minimal artist insert (no upcoming-show requirement) ---

func (s *ArtistBillCompositionIntegrationSuite) createArtist(name string) uint {
	artist := &catalogm.Artist{Name: name}
	s.deps.DB.Create(artist)
	slug := fmt.Sprintf("%s-slug-%d", name, artist.ID)
	s.deps.DB.Model(artist).Update("slug", slug)
	return artist.ID
}
