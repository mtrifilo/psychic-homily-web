package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
)

// VenueBillNetworkIntegrationSuite covers the PSY-365 venue co-bill graph
// service entry point. Mirrors SceneGraphIntegrationSuite in shape — the two
// endpoints share the same response contract on the frontend, so they share
// the same kind of integration coverage on the backend.
//
// What we pin here:
//   - Edge weight is AT-VENUE shared shows, not global (`TestCrossVenueLeakagePrevented`).
//   - The min-shared-shows threshold gates an edge from surfacing
//     (`TestThreshold`).
//   - Time-window filter scopes the artist + edge sets (`TestWindowFilter`).
//   - Empty / sparse cases produce stable, non-nil array fields so the JSON
//     contract holds for the frontend.
//   - Isolate derivation matches the post-edge-filter graph
//     (`TestIsolatesAndUpcomingCount`).
type VenueBillNetworkIntegrationSuite struct {
	suite.Suite
	deps *handlerIntegrationDeps
}

func (s *VenueBillNetworkIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
}

func (s *VenueBillNetworkIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *VenueBillNetworkIntegrationSuite) TearDownSuite() {
	s.deps.testDB.Cleanup()
}

func TestVenueBillNetworkIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(VenueBillNetworkIntegrationSuite))
}

// --- Helpers ---

// seedArtistForVenueBills mirrors seedSceneArtist — bare row with a slug so
// the response payload looks complete.
func (s *VenueBillNetworkIntegrationSuite) seedArtist(name string) *models.Artist {
	a := &models.Artist{Name: name}
	s.deps.db.Create(a)
	slug := name + "-slug"
	s.deps.db.Model(a).Update("slug", slug)
	return a
}

// seedShowAtVenue mirrors the scene_graph_test helper — one approved show on
// `eventDate` at `venue` with the given artist IDs in lineup order.
func (s *VenueBillNetworkIntegrationSuite) seedShowAtVenue(eventDate time.Time, venue *models.Venue, artistIDs ...uint) uint {
	user := createTestUser(s.deps.db)
	show := &models.Show{
		Title:       "Show",
		EventDate:   eventDate,
		City:        stringPtr(venue.City),
		State:       stringPtr(venue.State),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(show)
	s.deps.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	for i, aid := range artistIDs {
		setType := "opener"
		if i == 0 {
			setType = "headliner"
		}
		s.deps.db.Exec(
			"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, ?, ?)",
			show.ID, aid, i, setType,
		)
	}
	return show.ID
}

// --- Tests ---

// TestVenueNotFound: an unknown venue id surfaces the existing 404 path so
// the handler can map it to huma.Error404NotFound consistently with the
// other venue endpoints.
func (s *VenueBillNetworkIntegrationSuite) TestVenueNotFound() {
	_, err := s.deps.venueService.GetVenueBillNetwork(999_999, "all", nil)
	s.Require().Error(err)
}

// TestEmptyVenue: a venue with zero approved shows still resolves; the
// payload arrays are non-nil empty so JSON is contract-stable.
func (s *VenueBillNetworkIntegrationSuite) TestEmptyVenue() {
	venue := createVerifiedVenue(s.deps.db, "Empty Bar", "Phoenix", "AZ")

	graph, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(venue.ID, graph.Venue.ID)
	s.Equal("Phoenix", graph.Venue.City)
	s.Equal(0, graph.Venue.ArtistCount)
	s.Equal(0, graph.Venue.EdgeCount)
	s.Equal(0, graph.Venue.ShowCount)
	s.Equal("all_time", graph.Venue.Window)
	s.Empty(graph.Nodes)
	s.Empty(graph.Links)
	s.Empty(graph.Clusters)
	s.NotNil(graph.Nodes, "nodes should be empty array, not nil")
	s.NotNil(graph.Links, "links should be empty array, not nil")
	s.NotNil(graph.Clusters, "clusters should be empty array, not nil")
}

// TestSingleShowNoCoBills: a venue with one show + one artist surfaces the
// artist as an isolate (no co-bills). No edges, AtVenueShowCount=1.
func (s *VenueBillNetworkIntegrationSuite) TestSingleShowNoCoBills() {
	venue := createVerifiedVenue(s.deps.db, "Solo Bar", "Phoenix", "AZ")
	a := s.seedArtist("Solo")
	s.seedShowAtVenue(time.Now().UTC().AddDate(0, -1, 0), venue, a.ID)

	graph, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(1, graph.Venue.ArtistCount)
	s.Equal(1, graph.Venue.ShowCount)
	s.Equal(0, graph.Venue.EdgeCount)
	s.Require().Len(graph.Nodes, 1)
	s.True(graph.Nodes[0].IsIsolate)
	s.Equal(1, graph.Nodes[0].AtVenueShowCount)
}

// TestThreshold: two artists must share at least venueBillMinSharedShows
// shows at the venue (= 2) for an edge to surface. One shared show → no edge,
// both artists are isolates. Two shared shows → edge surfaces.
func (s *VenueBillNetworkIntegrationSuite) TestThreshold() {
	venue := createVerifiedVenue(s.deps.db, "Threshold Bar", "Phoenix", "AZ")
	a := s.seedArtist("Threshold-A")
	b := s.seedArtist("Threshold-B")

	now := time.Now().UTC()
	// One shared show — below threshold.
	s.seedShowAtVenue(now.AddDate(0, -1, 0), venue, a.ID, b.ID)

	graph, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(0, graph.Venue.EdgeCount, "single shared show should not surface an edge (min=2)")
	s.Empty(graph.Links)
	for _, n := range graph.Nodes {
		s.True(n.IsIsolate, "%s should be isolate below threshold", n.Name)
	}

	// Add a second shared show — now above threshold.
	s.seedShowAtVenue(now.AddDate(0, -2, 0), venue, a.ID, b.ID)
	graph2, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(1, graph2.Venue.EdgeCount)
	s.Require().Len(graph2.Links, 1)
	s.Equal(models.RelationshipTypeSharedBills, graph2.Links[0].Type)

	// Detail blob carries shared_count + last_shared per PSY-362 grammar.
	detail, ok := graph2.Links[0].Detail.(map[string]any)
	s.Require().True(ok, "Detail should be a map[string]any")
	s.Equal(2, detail["shared_count"])
	s.Contains(detail, "last_shared")

	for _, n := range graph2.Nodes {
		s.False(n.IsIsolate, "%s should not be isolate after edge surfaces", n.Name)
	}
}

// TestCrossVenueLeakagePrevented: the same two artists share shows at TWO
// different venues. The bill-network for venue A must only count the shows
// AT venue A in the edge weight — not the global pair count.
func (s *VenueBillNetworkIntegrationSuite) TestCrossVenueLeakagePrevented() {
	venueA := createVerifiedVenue(s.deps.db, "Venue A", "Phoenix", "AZ")
	venueB := createVerifiedVenue(s.deps.db, "Venue B", "Phoenix", "AZ")

	a := s.seedArtist("Cross-A")
	b := s.seedArtist("Cross-B")

	now := time.Now().UTC()
	// Two shared shows at A — surface the edge.
	s.seedShowAtVenue(now.AddDate(0, -1, 0), venueA, a.ID, b.ID)
	s.seedShowAtVenue(now.AddDate(0, -2, 0), venueA, a.ID, b.ID)
	// Three additional shared shows at B — must NOT count toward A's edge.
	s.seedShowAtVenue(now.AddDate(0, -3, 0), venueB, a.ID, b.ID)
	s.seedShowAtVenue(now.AddDate(0, -4, 0), venueB, a.ID, b.ID)
	s.seedShowAtVenue(now.AddDate(0, -5, 0), venueB, a.ID, b.ID)

	graphA, err := s.deps.venueService.GetVenueBillNetwork(venueA.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(2, graphA.Venue.ShowCount)
	s.Require().Len(graphA.Links, 1)
	detailA := graphA.Links[0].Detail.(map[string]any)
	s.Equal(2, detailA["shared_count"], "edge weight at venue A is AT-A count, not global")

	graphB, err := s.deps.venueService.GetVenueBillNetwork(venueB.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(3, graphB.Venue.ShowCount)
	s.Require().Len(graphB.Links, 1)
	detailB := graphB.Links[0].Detail.(map[string]any)
	s.Equal(3, detailB["shared_count"])
}

// TestWindowFilter: window=12m and window=year both scope the artist set +
// edge counts. A pair with two shared shows OUTSIDE the window must not
// surface as an edge in the windowed view.
func (s *VenueBillNetworkIntegrationSuite) TestWindowFilter() {
	venue := createVerifiedVenue(s.deps.db, "Window Bar", "Phoenix", "AZ")
	a := s.seedArtist("Win-A")
	b := s.seedArtist("Win-B")

	now := time.Now().UTC()
	// Two shows two years ago — out of 12m and out of "current year".
	s.seedShowAtVenue(now.AddDate(-2, 0, 0), venue, a.ID, b.ID)
	s.seedShowAtVenue(now.AddDate(-2, -1, 0), venue, a.ID, b.ID)

	all, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(1, all.Venue.EdgeCount, "all-time should surface the edge")

	last12m, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "12m", nil)
	s.Require().NoError(err)
	s.Equal("last_12m", last12m.Venue.Window)
	s.Equal(0, last12m.Venue.EdgeCount, "12m window should exclude shows from 2 years ago")
	s.Equal(0, last12m.Venue.ArtistCount)

	// Year filter: pick a year that's earlier than the seeded shows and
	// later than them — both scopes should be zero.
	currentYear := now.Year()
	yr := currentYear
	currentYrGraph, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "year", &yr)
	s.Require().NoError(err)
	s.Equal("year", currentYrGraph.Venue.Window)
	s.Require().NotNil(currentYrGraph.Venue.Year)
	s.Equal(currentYear, *currentYrGraph.Venue.Year)
	s.Equal(0, currentYrGraph.Venue.EdgeCount)

	// The actual seeded year (2 years ago) should surface the edge.
	twoYearsAgo := now.AddDate(-2, 0, 0).Year()
	twoAgoGraph, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "year", &twoYearsAgo)
	s.Require().NoError(err)
	s.Equal(1, twoAgoGraph.Venue.EdgeCount, "year=%d should surface the seeded edge", twoYearsAgo)
}

// TestUnknownWindowFallsBackToAll: a malformed window string degrades
// gracefully to "all" rather than 500ing or returning empty — same posture
// as the scene-graph types allowlist.
func (s *VenueBillNetworkIntegrationSuite) TestUnknownWindowFallsBackToAll() {
	venue := createVerifiedVenue(s.deps.db, "Unknown Bar", "Phoenix", "AZ")
	a := s.seedArtist("Unk-A")
	b := s.seedArtist("Unk-B")

	now := time.Now().UTC()
	s.seedShowAtVenue(now.AddDate(0, -1, 0), venue, a.ID, b.ID)
	s.seedShowAtVenue(now.AddDate(0, -2, 0), venue, a.ID, b.ID)

	graph, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "garbage", nil)
	s.Require().NoError(err)
	s.Equal("all_time", graph.Venue.Window, "unknown window should normalize to all-time")
	s.Equal(1, graph.Venue.EdgeCount)
}

// TestMultiArtistShow: a single show with three artists produces three
// distinct co-bill pairs (k(k-1)/2). With one show, none of those pairs hit
// the threshold; with two shows they all do.
func (s *VenueBillNetworkIntegrationSuite) TestMultiArtistShow() {
	venue := createVerifiedVenue(s.deps.db, "Trio Bar", "Phoenix", "AZ")
	a := s.seedArtist("Trio-A")
	b := s.seedArtist("Trio-B")
	c := s.seedArtist("Trio-C")

	now := time.Now().UTC()
	s.seedShowAtVenue(now.AddDate(0, -1, 0), venue, a.ID, b.ID, c.ID)
	graph, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(3, graph.Venue.ArtistCount)
	s.Equal(0, graph.Venue.EdgeCount, "one show below threshold (=2)")

	s.seedShowAtVenue(now.AddDate(0, -2, 0), venue, a.ID, b.ID, c.ID)
	graph2, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "all", nil)
	s.Require().NoError(err)
	s.Equal(3, graph2.Venue.EdgeCount, "k(k-1)/2 = 3 pairs all above threshold after 2 shared shows")
	for _, n := range graph2.Nodes {
		s.False(n.IsIsolate, "%s should not be isolate", n.Name)
	}
}

// TestIsolatesAndUpcomingCount: an artist with a venue show but no co-bills
// (single-headliner shows) is an isolate; UpcomingShowCount tracks future
// approved shows globally (not just at this venue).
func (s *VenueBillNetworkIntegrationSuite) TestIsolatesAndUpcomingCount() {
	venue := createVerifiedVenue(s.deps.db, "Iso Bar", "Phoenix", "AZ")
	otherVenue := createVerifiedVenue(s.deps.db, "Other Bar", "Phoenix", "AZ")
	a := s.seedArtist("Iso")

	now := time.Now().UTC()
	// Past solo show at the venue — makes Iso part of the artist set.
	s.seedShowAtVenue(now.AddDate(0, -1, 0), venue, a.ID)
	// Future show at a DIFFERENT venue — should bump UpcomingShowCount but
	// not the at-venue count.
	s.seedShowAtVenue(now.AddDate(0, 1, 0), otherVenue, a.ID)

	graph, err := s.deps.venueService.GetVenueBillNetwork(venue.ID, "all", nil)
	s.Require().NoError(err)
	s.Require().Len(graph.Nodes, 1)
	s.True(graph.Nodes[0].IsIsolate)
	s.Equal(1, graph.Nodes[0].AtVenueShowCount, "only past show at this venue counts")
	s.Equal(1, graph.Nodes[0].UpcomingShowCount, "UpcomingShowCount counts future shows globally")
}
