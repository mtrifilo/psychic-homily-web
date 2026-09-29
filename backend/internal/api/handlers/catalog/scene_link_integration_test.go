package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
)

// SceneLinkHandlerIntegrationSuite pins the scene link on the three detail
// routes a landing page reads: present on the wire when the scene page serves,
// and absent from the JSON entirely when it does not.
type SceneLinkHandlerIntegrationSuite struct {
	suite.Suite
	deps          *testhelpers.IntegrationDeps
	artistHandler *ArtistHandler
	venueHandler  *VenueHandler
	showHandler   *ShowHandler
}

func (s *SceneLinkHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.artistHandler = NewArtistHandler(s.deps.ArtistService, s.deps.AuditLogService, nil, nil)
	s.venueHandler = NewVenueHandler(s.deps.VenueService, s.deps.DiscordService, s.deps.AuditLogService, nil)
	s.showHandler = NewShowHandler(
		s.deps.ShowService,
		s.deps.ShowService,
		s.deps.ShowService,
		s.deps.SavedShowService,
		s.deps.DiscordService,
		s.deps.ExtractionService,
		nil,
		testhelpers.AllShowsVisible(),
	)
}

func (s *SceneLinkHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *SceneLinkHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestSceneLinkHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(SceneLinkHandlerIntegrationSuite))
}

// seedShow creates an approved show at a verified Phoenix venue whose billed
// artist has no location of its own, and returns the show, venue and artist
// ids.
func (s *SceneLinkHandlerIntegrationSuite) seedShow() (showID, venueID, artistID uint) {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Scene Link Show")
	var ids struct {
		VenueID  uint
		ArtistID uint
	}
	s.Require().NoError(s.deps.DB.Raw(`
		SELECT sv.venue_id, sa.artist_id
		FROM show_venues sv JOIN show_artists sa ON sa.show_id = sv.show_id
		WHERE sv.show_id = ?`, show.ID).Scan(&ids).Error)
	s.Require().NotZero(ids.VenueID)
	s.Require().NotZero(ids.ArtistID)
	return show.ID, ids.VenueID, ids.ArtistID
}

// wireScene marshals a response body and returns its "scene" member, and
// whether the key is present at all.
func (s *SceneLinkHandlerIntegrationSuite) wireScene(body any) (map[string]any, bool) {
	raw, err := json.Marshal(body)
	s.Require().NoError(err)
	var decoded map[string]any
	s.Require().NoError(json.Unmarshal(raw, &decoded))
	scene, present := decoded["scene"]
	if !present {
		return nil, false
	}
	asMap, ok := scene.(map[string]any)
	s.Require().True(ok, "scene must serialize as an object")
	return asMap, true
}

func (s *SceneLinkHandlerIntegrationSuite) TestDetailRoutesCarryTheSceneWhenItServes() {
	showID, venueID, artistID := s.seedShow()
	// The second verified room is what lifts Phoenix over the scene venue floor.
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Second Room", "Phoenix", "AZ")
	want := map[string]any{"slug": "phoenix-az", "city": "Phoenix", "state": "AZ"}

	showResp, err := s.showHandler.GetShowHandler(context.Background(), &GetShowRequest{ShowID: fmt.Sprint(showID)})
	s.Require().NoError(err)
	scene, present := s.wireScene(showResp.Body)
	s.Require().True(present, "show detail")
	s.Equal(want, scene)

	venueResp, err := s.venueHandler.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: fmt.Sprint(venueID)})
	s.Require().NoError(err)
	scene, present = s.wireScene(venueResp.Body)
	s.Require().True(present, "venue detail")
	s.Equal(want, scene)

	artistResp, err := s.artistHandler.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: fmt.Sprint(artistID)})
	s.Require().NoError(err)
	scene, present = s.wireScene(artistResp.Body)
	s.Require().True(present, "artist detail, derived from the latest show's venue")
	s.Equal(want, scene)
}

func (s *SceneLinkHandlerIntegrationSuite) TestDetailRoutesOmitTheSceneWhenItDoesNotServe() {
	// One verified room: /scenes/phoenix-az would 404, so nothing links there.
	showID, venueID, artistID := s.seedShow()

	showResp, err := s.showHandler.GetShowHandler(context.Background(), &GetShowRequest{ShowID: fmt.Sprint(showID)})
	s.Require().NoError(err)
	_, present := s.wireScene(showResp.Body)
	s.False(present, "show detail")

	venueResp, err := s.venueHandler.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: fmt.Sprint(venueID)})
	s.Require().NoError(err)
	_, present = s.wireScene(venueResp.Body)
	s.False(present, "venue detail")

	artistResp, err := s.artistHandler.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: fmt.Sprint(artistID)})
	s.Require().NoError(err)
	_, present = s.wireScene(artistResp.Body)
	s.False(present, "artist detail")
}
