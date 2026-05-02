package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/models"
)

type ShowHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *ShowHandler
}

func (s *ShowHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewShowHandler(
		s.deps.ShowService,
		s.deps.ShowService,
		s.deps.ShowService,
		s.deps.SavedShowService,
		s.deps.DiscordService,
		s.deps.MusicDiscoveryService,
		s.deps.ExtractionService,
	)
}

func (s *ShowHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *ShowHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestShowHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ShowHandlerIntegrationSuite))
}

// --- CreateShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestCreateShow_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(user)
	title := "New Show"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: testhelpers.StringPtr("Test Artist")}}

	resp, err := s.handler.CreateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("New Show", resp.Body.Title)
	// Shows with verified venues are auto-approved
	s.Equal("approved", resp.Body.Status)
}

func (s *ShowHandlerIntegrationSuite) TestCreateShow_AdminAutoApproved() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(admin)
	title := "Admin Show"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: testhelpers.StringPtr("Test Artist")}}

	resp, err := s.handler.CreateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("approved", resp.Body.Status)
}

func (s *ShowHandlerIntegrationSuite) TestCreateShow_UnverifiedEmailBlocked() {
	user := &models.User{
		Email:         testhelpers.StringPtr("unverified@test.com"),
		FirstName:     testhelpers.StringPtr("Test"),
		LastName:      testhelpers.StringPtr("User"),
		IsActive:      true,
		EmailVerified: false,
	}
	s.deps.DB.Create(user)

	ctx := testhelpers.CtxWithUser(user)
	title := "Blocked Show"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{Name: testhelpers.StringPtr("Some Venue")}}
	req.Body.Artists = []Artist{{Name: testhelpers.StringPtr("Some Artist")}}

	_, err := s.handler.CreateShowHandler(ctx, req)
	s.Error(err)
}

// --- GetShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetShow_ByID() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	req := &GetShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.GetShowHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(show.ID, resp.Body.ID)
}

func (s *ShowHandlerIntegrationSuite) TestGetShow_NotFound() {
	req := &GetShowRequest{ShowID: "99999"}
	_, err := s.handler.GetShowHandler(context.Background(), req)
	s.Error(err)
}

func (s *ShowHandlerIntegrationSuite) TestGetShow_PendingShowSubmitterCanView() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Pending Show")

	ctx := testhelpers.CtxWithUser(user)
	req := &GetShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.GetShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("pending", resp.Body.Status)
}

func (s *ShowHandlerIntegrationSuite) TestGetShow_PendingShowOtherUserDenied() {
	submitter := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreatePendingShow(s.deps.DB, submitter.ID, "Pending Show")

	ctx := testhelpers.CtxWithUser(other)
	req := &GetShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	_, err := s.handler.GetShowHandler(ctx, req)
	s.Error(err)
}

// --- GetShowsHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetShows_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Show 1")
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Show 2")

	req := &GetShowsRequest{}
	resp, err := s.handler.GetShowsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(len(resp.Body), 2)
}

func (s *ShowHandlerIntegrationSuite) TestGetShows_Empty() {
	req := &GetShowsRequest{}
	resp, err := s.handler.GetShowsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body)
}

// --- GetUpcomingShowsHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetUpcomingShows_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	testhelpers.CreateFutureApprovedShow(s.deps.DB, user.ID, "Future Show", 7)

	req := &GetUpcomingShowsRequest{Timezone: "UTC", Limit: 50}
	resp, err := s.handler.GetUpcomingShowsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(len(resp.Body.Shows), 1)
}

func (s *ShowHandlerIntegrationSuite) TestGetUpcomingShows_ExcludesPast() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	testhelpers.CreatePastApprovedShow(s.deps.DB, user.ID, "Past Show", 30)

	req := &GetUpcomingShowsRequest{Timezone: "UTC", Limit: 50}
	resp, err := s.handler.GetUpcomingShowsHandler(context.Background(), req)
	s.NoError(err)
	// Past shows should not appear
	for _, show := range resp.Body.Shows {
		s.NotEqual("Past Show", show.Title)
	}
}

func (s *ShowHandlerIntegrationSuite) TestGetUpcomingShows_Empty() {
	req := &GetUpcomingShowsRequest{Timezone: "UTC", Limit: 50}
	resp, err := s.handler.GetUpcomingShowsHandler(context.Background(), req)
	s.NoError(err)
	s.Empty(resp.Body.Shows)
}

// --- UpdateShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_OwnerSuccess() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Original Title")

	ctx := testhelpers.CtxWithUser(user)
	newTitle := "Updated Title"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Title", resp.Body.Title)
}

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_AdminSuccess() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Original Title")

	ctx := testhelpers.CtxWithUser(admin)
	newTitle := "Admin Updated"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.NoError(err)
	s.Equal("Admin Updated", resp.Body.Title)
}

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_NotOwnerForbidden() {
	submitter := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, submitter.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(other)
	newTitle := "Hacked Title"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateShowHandler(ctx, req)
	s.Error(err)
}

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_NotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	newTitle := "Updated"
	req := &UpdateShowRequest{ShowID: "99999"}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateShowHandler(ctx, req)
	s.Error(err)
}

// --- DeleteShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestDeleteShow_OwnerSuccess() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Delete Me")

	ctx := testhelpers.CtxWithUser(user)
	req := &DeleteShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	_, err := s.handler.DeleteShowHandler(ctx, req)
	s.NoError(err)
}

func (s *ShowHandlerIntegrationSuite) TestDeleteShow_NotOwnerForbidden() {
	submitter := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, submitter.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(other)
	req := &DeleteShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	_, err := s.handler.DeleteShowHandler(ctx, req)
	s.Error(err)
}

func (s *ShowHandlerIntegrationSuite) TestDeleteShow_NotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &DeleteShowRequest{ShowID: "99999"}
	_, err := s.handler.DeleteShowHandler(ctx, req)
	s.Error(err)
}

// --- CreateShowHandler with InstagramHandle ---

func (s *ShowHandlerIntegrationSuite) TestCreateShow_WithInstagramHandle() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "IG Test Venue", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(user)
	title := "IG Show"
	igHandle := "@new_ig"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: testhelpers.StringPtr("New IG Artist"), InstagramHandle: &igHandle}}

	resp, err := s.handler.CreateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Require().Len(resp.Body.Artists, 1)
	s.Require().NotNil(resp.Body.Artists[0].Socials.Instagram)
	s.Equal("@new_ig", *resp.Body.Artists[0].Socials.Instagram)

	// Verify in DB
	var artist models.Artist
	s.NoError(s.deps.DB.Where("name = ?", "New IG Artist").First(&artist).Error)
	s.Require().NotNil(artist.Social.Instagram)
	s.Equal("@new_ig", *artist.Social.Instagram)
}

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_WithInstagramOnNewArtist() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Update IG Show")

	ctx := testhelpers.CtxWithUser(user)
	igHandle := "@updated_ig"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Artists = []Artist{
		{Name: testhelpers.StringPtr("Updated IG Artist"), InstagramHandle: &igHandle},
	}

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Require().Len(resp.Body.Artists, 1)
	s.Require().NotNil(resp.Body.Artists[0].Socials.Instagram)
	s.Equal("@updated_ig", *resp.Body.Artists[0].Socials.Instagram)

	// Verify in DB
	var artist models.Artist
	s.NoError(s.deps.DB.Where("name = ?", "Updated IG Artist").First(&artist).Error)
	s.Require().NotNil(artist.Social.Instagram)
	s.Equal("@updated_ig", *artist.Social.Instagram)
}

// --- GetMySubmissionsHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetMySubmissions_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "My Show 1")
	testhelpers.CreatePendingShow(s.deps.DB, user.ID, "My Show 2")

	ctx := testhelpers.CtxWithUser(user)
	req := &GetMySubmissionsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetMySubmissionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(2, resp.Body.Total)
	s.Len(resp.Body.Shows, 2)
}

func (s *ShowHandlerIntegrationSuite) TestGetMySubmissions_Empty() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &GetMySubmissionsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetMySubmissionsHandler(ctx, req)
	s.NoError(err)
	s.Equal(0, resp.Body.Total)
}

// --- GetShowCitiesHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetShowCities_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	testhelpers.CreateFutureApprovedShow(s.deps.DB, user.ID, "Phoenix Show", 7)

	req := &GetShowCitiesRequest{Timezone: "UTC"}
	resp, err := s.handler.GetShowCitiesHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
}

// --- UnpublishShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestUnpublishShow_OwnerSuccess() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Approved Show")

	ctx := testhelpers.CtxWithUser(user)
	req := &UnpublishShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.UnpublishShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("private", resp.Body.Status)
}

func (s *ShowHandlerIntegrationSuite) TestUnpublishShow_NotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &UnpublishShowRequest{ShowID: "99999"}
	_, err := s.handler.UnpublishShowHandler(ctx, req)
	s.Error(err)
}

// --- SetShowSoldOutHandler ---

func (s *ShowHandlerIntegrationSuite) TestSetShowSoldOut_OwnerSuccess() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(user)
	req := &SetShowSoldOutRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Value = true

	resp, err := s.handler.SetShowSoldOutHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

func (s *ShowHandlerIntegrationSuite) TestSetShowSoldOut_NonOwnerForbidden() {
	submitter := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, submitter.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(other)
	req := &SetShowSoldOutRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Value = true

	_, err := s.handler.SetShowSoldOutHandler(ctx, req)
	s.Error(err)
}

// --- SetShowCancelledHandler ---

func (s *ShowHandlerIntegrationSuite) TestSetShowCancelled_OwnerSuccess() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(user)
	req := &SetShowCancelledRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Value = true

	resp, err := s.handler.SetShowCancelledHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

// --- SearchShowsHandler (PSY-520) ---

// createShowForSearch creates a show with the given title, headliner artist
// name, supporting artist names, venue name, and event_date — minimal helper
// to set up search-test fixtures with deterministic ordering. Returns the
// created show.
//
// `set_type='headliner' OR position=0` is the convention used to identify a
// show's headliner across the codebase (see checkDuplicateHeadlinerConflicts
// in catalog/show.go). There is no `is_headliner` column on show_artists.
func (s *ShowHandlerIntegrationSuite) createShowForSearch(
	title, headlinerName string,
	supportingArtistNames []string,
	venueName string,
	eventDate time.Time,
) *models.Show {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, venueName, "Phoenix", "AZ")
	headliner := testhelpers.CreateArtist(s.deps.DB, headlinerName)

	show := &models.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	// Slug — required to mirror the production format (auto-set in
	// CreateShow but raw inserts skip that).
	slug := fmt.Sprintf("show-%d", show.ID)
	s.deps.DB.Model(show).Update("slug", slug)

	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.DB.Exec(
		"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')",
		show.ID, headliner.ID,
	)

	for i, name := range supportingArtistNames {
		support := testhelpers.CreateArtist(s.deps.DB, name)
		// Position starts at 1 for openers; set_type='opener' so headliner
		// resolution doesn't pick up support artists.
		s.deps.DB.Exec(
			"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, ?, 'opener')",
			show.ID, support.ID, i+1,
		)
	}

	return show
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_TitleMatch() {
	now := time.Now().UTC()
	s.createShowForSearch("Valley Bar Showcase", "The Headliners", nil, "Valley Bar", now.AddDate(0, 0, 7))
	s.createShowForSearch("Crescent Ballroom Night", "Other Band", nil, "Crescent Ballroom", now.AddDate(0, 0, 14))

	req := &SearchShowsRequest{Query: "Valley"}
	resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Valley Bar Showcase", resp.Body.Shows[0].Title)
	s.Equal("The Headliners", resp.Body.Shows[0].HeadlinerName)
	s.Equal("Valley Bar", resp.Body.Shows[0].VenueName)
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_HeadlinerMatch() {
	now := time.Now().UTC()
	s.createShowForSearch("Generic Title", "Radiohead", nil, "Some Venue", now.AddDate(0, 0, 7))
	s.createShowForSearch("Another Show", "Different Band", nil, "Other Venue", now.AddDate(0, 0, 14))

	req := &SearchShowsRequest{Query: "radiohead"}
	resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Radiohead", resp.Body.Shows[0].HeadlinerName)
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_SupportArtistMatch() {
	now := time.Now().UTC()
	// "Sleater-Kinney" appears as a support artist (set_type='opener',
	// position=1), not as the headliner — we still want this show to come
	// up in the search.
	s.createShowForSearch(
		"Headliner Tour",
		"Big Headliner",
		[]string{"Sleater-Kinney", "Mid-Card"},
		"Crescent Ballroom",
		now.AddDate(0, 0, 7),
	)

	req := &SearchShowsRequest{Query: "Sleater"}
	resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	// Headliner field is the show's headliner — NOT the artist that
	// matched the search. This is the canonical behaviour the frontend
	// expects ("{Headliner} @ {Venue} · {Date}" format).
	s.Equal("Big Headliner", resp.Body.Shows[0].HeadlinerName)
	s.Equal("Headliner Tour", resp.Body.Shows[0].Title)
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_NoMatch() {
	now := time.Now().UTC()
	s.createShowForSearch("Some Show", "Some Band", nil, "Some Venue", now.AddDate(0, 0, 7))

	req := &SearchShowsRequest{Query: "zzznonexistent"}
	resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
	s.Empty(resp.Body.Shows)
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_EmptyQuery() {
	now := time.Now().UTC()
	// Even with shows in the DB, empty query must return [] — we never want
	// "search with no q" to return all shows (that would be a footgun if
	// the frontend ever sent an empty input).
	s.createShowForSearch("Whatever Show", "Whatever Band", nil, "Whatever Venue", now.AddDate(0, 0, 7))

	req := &SearchShowsRequest{Query: ""}
	resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_WhitespaceQuery() {
	now := time.Now().UTC()
	s.createShowForSearch("Whatever Show", "Whatever Band", nil, "Whatever Venue", now.AddDate(0, 0, 7))

	req := &SearchShowsRequest{Query: "   "}
	resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_DedupesTitleAndArtistMatch() {
	now := time.Now().UTC()
	// Show whose title AND headliner both match "Radiohead" — must appear
	// exactly once. Tests the DISTINCT ON (shows.id) clause in the query.
	s.createShowForSearch(
		"Radiohead Tour 2026",
		"Radiohead",
		[]string{"Radiohead Tribute Band"}, // even more matches on the bill
		"Some Venue",
		now.AddDate(0, 0, 7),
	)

	req := &SearchShowsRequest{Query: "radiohead"}
	resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count, "show matching both title and artist should appear exactly once")
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_OrderingByEventDateDesc() {
	now := time.Now().UTC()
	// Three matching shows on three different dates. Most-recent (= latest
	// event_date) should come first. Each show needs a unique headliner
	// because artists.name has a UNIQUE constraint — match via the title
	// instead (all three include "Festival" in the title).
	earliest := s.createShowForSearch("Early Festival Show", "Headliner Early", nil, "Venue A", now.AddDate(0, 0, 7))
	latest := s.createShowForSearch("Late Festival Show", "Headliner Late", nil, "Venue B", now.AddDate(0, 0, 60))
	middle := s.createShowForSearch("Middle Festival Show", "Headliner Middle", nil, "Venue C", now.AddDate(0, 0, 30))

	req := &SearchShowsRequest{Query: "Festival"}
	resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(3, resp.Body.Count)

	// Order: latest, middle, earliest.
	s.Equal(latest.ID, resp.Body.Shows[0].ID)
	s.Equal(middle.ID, resp.Body.Shows[1].ID)
	s.Equal(earliest.ID, resp.Body.Shows[2].ID)
}

func (s *ShowHandlerIntegrationSuite) TestSearchShows_CaseInsensitive() {
	now := time.Now().UTC()
	s.createShowForSearch("Mixed Case Show", "ALLCAPS BAND", nil, "Venue", now.AddDate(0, 0, 7))

	for _, query := range []string{"mixed", "MIXED", "MiXeD", "allcaps", "AllCaps"} {
		req := &SearchShowsRequest{Query: query}
		resp, err := s.handler.SearchShowsHandler(s.deps.Ctx, req)
		s.NoError(err, "query %q failed", query)
		s.Equal(1, resp.Body.Count, "query %q should return 1 result", query)
	}
}
