package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
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
		s.deps.ExtractionService,
		nil, // revisionService — not exercised in integration tests
		testhelpers.AllShowsVisible(),
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
	user := &authm.User{
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

// TestUpdateShow_RejectsTooManyArtists: PSY-1267 — the update path (no Resolve) caps
// the array in the handler, before any DB work, since an update can also create new
// artists (same outbound-enrichment amplification as create). Assert the specific
// 422 from the cap, NOT just any error: ShowID "1" doesn't exist, so without the cap
// the handler would fall through to a 404 and a bare s.Error would still pass
// (false-green) — the 422 status is what proves the cap fired first.
func (s *ShowHandlerIntegrationSuite) TestUpdateShow_RejectsTooManyArtists() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &UpdateShowRequest{ShowID: "1"}
	artists := make([]Artist, maxShowArtists+1)
	for i := range artists {
		name := fmt.Sprintf("Artist %d", i)
		artists[i] = Artist{Name: &name}
	}
	req.Body.Artists = artists

	_, err := s.handler.UpdateShowHandler(ctx, req)
	s.Require().Error(err, "an update with more than %d artists must be rejected", maxShowArtists)
	var se huma.StatusError
	s.Require().ErrorAs(err, &se)
	s.Equal(http.StatusUnprocessableEntity, se.GetStatus(),
		"must be the cap 422, not a 404 fall-through (guards against the cap being removed)")
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
	s.GreaterOrEqual(len(resp.Body.Shows), 2)
	s.GreaterOrEqual(resp.Body.Total, int64(2))
}

func (s *ShowHandlerIntegrationSuite) TestGetShows_Empty() {
	req := &GetShowsRequest{}
	resp, err := s.handler.GetShowsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Shows)
	s.Equal(int64(0), resp.Body.Total)
}

// The point of PSY-1748: a caller that sends no limit gets a bounded page, not
// the whole approved table. Seeded past the default so "everything" and "one
// page" are distinguishable.
func (s *ShowHandlerIntegrationSuite) TestGetShows_OmittedLimitIsBounded() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	const seeded = defaultShowListLimit + 5
	for i := 0; i < seeded; i++ {
		testhelpers.CreateApprovedShow(s.deps.DB, user.ID, fmt.Sprintf("Bounded Show %02d", i))
	}

	resp, err := s.handler.GetShowsHandler(context.Background(), &GetShowsRequest{})
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Len(resp.Body.Shows, defaultShowListLimit, "an omitted limit must not mean 'all rows'")
	s.Equal(defaultShowListLimit, resp.Body.Limit)
	// The total still describes the whole matching set, so the caller can tell
	// there is more without having asked for it.
	s.GreaterOrEqual(resp.Body.Total, int64(seeded))
}

// Offset walks the ordered list without repeating or dropping a row.
//
// Every seeded show is forced onto the SAME event_date, because that is the
// case offset paging gets wrong without a tiebreak: ordering by event_date
// alone leaves the order among tied rows implementation-defined, and Postgres
// is free to answer two OFFSET windows inconsistently, showing one row twice
// and another never. `shows.id ASC` is what closes it. Left to
// CreateApprovedShow's own `time.Now()` the dates would differ by microseconds
// and the test would pass with or without the fix.
func (s *ShowHandlerIntegrationSuite) TestGetShows_OffsetPagesWithoutOverlap() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	const seeded = 7
	for i := 0; i < seeded; i++ {
		testhelpers.CreateApprovedShow(s.deps.DB, user.ID, fmt.Sprintf("Paged Show %02d", i))
	}
	tied := time.Now().UTC().AddDate(0, 0, 7).Truncate(time.Hour)
	s.Require().NoError(
		s.deps.DB.Model(&catalogm.Show{}).
			Where("title LIKE ?", "Paged Show%").
			Update("event_date", tied).Error,
	)

	seen := make(map[uint]bool)
	for offset := 0; offset < seeded; offset += 3 {
		resp, err := s.handler.GetShowsHandler(context.Background(), &GetShowsRequest{
			Limit:  3,
			Offset: offset,
		})
		s.Require().NoError(err)
		s.Equal(int64(seeded), resp.Body.Total)
		for _, show := range resp.Body.Shows {
			s.False(seen[show.ID], "show %d returned on two pages", show.ID)
			seen[show.ID] = true
		}
	}
	s.Len(seen, seeded, "offset paging must reach every row exactly once")
}

// An offset past the end is an ordinary request under offset paging, not an
// error: empty page, real total.
func (s *ShowHandlerIntegrationSuite) TestGetShows_OffsetPastEndIsEmptyNotError() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Only Show")

	resp, err := s.handler.GetShowsHandler(context.Background(), &GetShowsRequest{
		Limit:  10,
		Offset: 5000,
	})
	s.NoError(err)
	s.Require().NotNil(resp)
	s.NotNil(resp.Body.Shows, "an empty page must serialize as [] rather than null")
	s.Empty(resp.Body.Shows)
	s.Equal(int64(1), resp.Body.Total)
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

// TestCreateShow_CarriesShowTimes covers the handler-level wiring of
// doors_at/music_at from the request body into the service request. Without
// this, both lines can be deleted and the whole suite still passes: the API
// would accept the fields, return 200, and silently discard them.
func (s *ShowHandlerIntegrationSuite) TestCreateShow_CarriesShowTimes() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	eventDate := time.Now().UTC().AddDate(0, 0, 14)
	doors := eventDate.Add(-time.Hour)
	music := eventDate

	ctx := testhelpers.CtxWithUser(user)
	title := "Show With Times"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = eventDate
	req.Body.DoorsAt = &doors
	req.Body.MusicAt = &music
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: testhelpers.StringPtr("Test Artist")}}

	resp, err := s.handler.CreateShowHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.DoorsAt, "doors_at must reach the service, not be dropped")
	s.Require().NotNil(resp.Body.MusicAt, "music_at must reach the service, not be dropped")
	s.Equal(doors.Unix(), resp.Body.DoorsAt.Unix())
	s.Equal(music.Unix(), resp.Body.MusicAt.Unix())
}

// TestCreateShow_RejectsMusicBeforeDoors pins the one ordering rule that is
// true by definition. Any window against event_date is deliberately not
// enforced.
func (s *ShowHandlerIntegrationSuite) TestCreateShow_RejectsMusicBeforeDoors() {
	eventDate := time.Now().UTC().AddDate(0, 0, 14)
	doors := eventDate
	music := eventDate.Add(-time.Hour)

	body := &CreateShowRequestBody{
		EventDate: eventDate,
		DoorsAt:   &doors,
		MusicAt:   &music,
	}
	errs := body.Resolve(nil)

	s.Require().NotEmpty(errs, "music before doors must be rejected")
	found := false
	for _, e := range errs {
		var detail *huma.ErrorDetail
		if errors.As(e, &detail) && detail.Location == "body.music_at" {
			found = true
		}
	}
	s.True(found, "expected a body.music_at validation error, got %v", errs)
}

// TestUpdateShow_CarriesShowTimes is the update-path analog, and additionally
// pins that an omitted time is left alone rather than cleared.
func (s *ShowHandlerIntegrationSuite) TestUpdateShow_CarriesShowTimes() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Timed Show")

	doors := show.EventDate.Add(-time.Hour)
	music := show.EventDate

	ctx := testhelpers.CtxWithUser(user)
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.DoorsAt = &doors
	req.Body.MusicAt = &music

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.DoorsAt, "doors_at must reach the service, not be dropped")
	s.Require().NotNil(resp.Body.MusicAt, "music_at must reach the service, not be dropped")
	s.Equal(doors.Unix(), resp.Body.DoorsAt.Unix())
	s.Equal(music.Unix(), resp.Body.MusicAt.Unix())

	newTitle := "Retitled Only"
	titleOnly := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	titleOnly.Body.Title = &newTitle
	resp, err = s.handler.UpdateShowHandler(ctx, titleOnly)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.DoorsAt)
	s.Equal(doors.Unix(), resp.Body.DoorsAt.Unix(), "omitted doors_at must survive an unrelated edit")
}

// TestUpdateShow_RejectsMusicBeforeStoredDoors covers the partial-update case:
// the body carries only music_at, and it must be judged against the doors_at
// already on the row.
func (s *ShowHandlerIntegrationSuite) TestUpdateShow_RejectsMusicBeforeStoredDoors() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Timed Show")

	doors := show.EventDate
	ctx := testhelpers.CtxWithUser(user)
	setDoors := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	setDoors.Body.DoorsAt = &doors
	_, err := s.handler.UpdateShowHandler(ctx, setDoors)
	s.Require().NoError(err)

	earlierMusic := doors.Add(-time.Hour)
	bad := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	bad.Body.MusicAt = &earlierMusic
	_, err = s.handler.UpdateShowHandler(ctx, bad)
	s.Require().Error(err, "music before the stored doors_at must be rejected")
}

// TestUpdateShow_StoredDisorderDoesNotBlockUnrelatedEdits pins that the
// ordering rule only applies to requests that touch a show time.
//
// A row can reach an out-of-order state without passing through validation: an
// admin revision rollback, a data import, or two concurrent PUTs each checking
// against its own pre-write snapshot. If the check ran on every update, such a
// row would reject every later edit, including title-only ones, citing a field
// the caller never sent. No UI writes these times yet, so there would be no
// in-product way to repair it.
func (s *ShowHandlerIntegrationSuite) TestUpdateShow_StoredDisorderDoesNotBlockUnrelatedEdits() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Disordered Show")

	// Write an out-of-order pair straight to the row, bypassing the handler.
	doors := show.EventDate
	music := show.EventDate.Add(-2 * time.Hour)
	s.Require().NoError(s.deps.DB.Model(&catalogm.Show{}).Where("id = ?", show.ID).
		Updates(map[string]interface{}{"doors_at": doors, "music_at": music}).Error)

	ctx := testhelpers.CtxWithUser(user)
	newTitle := "Retitled Despite Disorder"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.Require().NoError(err, "a title-only edit must not be blocked by pre-existing stored time disorder")
	s.Equal("Retitled Despite Disorder", resp.Body.Title)

	// Touching a time on the same row is still validated.
	worse := doors.Add(-3 * time.Hour)
	bad := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	bad.Body.MusicAt = &worse
	_, err = s.handler.UpdateShowHandler(ctx, bad)
	s.Require().Error(err, "a request that does touch a show time is still checked")
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
	// PSY-1118: the bare "@new_ig" handle is normalized to the canonical
	// instagram.com URL form (same as the artist/venue/label edit paths).
	s.Equal("https://instagram.com/new_ig", *resp.Body.Artists[0].Socials.Instagram)

	// Verify in DB
	var artist catalogm.Artist
	s.NoError(s.deps.DB.Where("name = ?", "New IG Artist").First(&artist).Error)
	s.Require().NotNil(artist.Social.Instagram)
	s.Equal("https://instagram.com/new_ig", *artist.Social.Instagram)
}

// PSY-1118: a URL-shaped instagram_handle on show-create must be rejected (it
// previously bypassed the social-host anchor and rendered as an off-platform
// href). The create path returns a precise field-located 422 via Resolve.
func (s *ShowHandlerIntegrationSuite) TestCreateShow_RejectsURLShapedInstagramHandle() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "IG Reject Venue", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(user)
	title := "IG Reject Show"
	badHandle := "https://evil.test"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: testhelpers.StringPtr("Evil IG Artist"), InstagramHandle: &badHandle}}

	resp, err := s.handler.CreateShowHandler(ctx, req)
	s.Error(err)
	s.Nil(resp)

	// No artist should have been created with the attacker host.
	var count int64
	s.deps.DB.Model(&catalogm.Artist{}).Where("name = ?", "Evil IG Artist").Count(&count)
	s.Equal(int64(0), count)
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
	// PSY-1118: normalized to the canonical instagram.com URL form.
	s.Equal("https://instagram.com/updated_ig", *resp.Body.Artists[0].Socials.Instagram)

	// Verify in DB
	var artist catalogm.Artist
	s.NoError(s.deps.DB.Where("name = ?", "Updated IG Artist").First(&artist).Error)
	s.Require().NotNil(artist.Social.Instagram)
	// PSY-1118: normalized to the canonical instagram.com URL form.
	s.Equal("https://instagram.com/updated_ig", *artist.Social.Instagram)
}

// PSY-1118: the update path shares associateArtists with create, so a
// URL-shaped handle must be rejected there too. UpdateShow has no Resolve, so
// the rejection comes from the service chokepoint (surfaced as an error).
func (s *ShowHandlerIntegrationSuite) TestUpdateShow_RejectsURLShapedInstagramHandle() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Update IG Reject Show")

	ctx := testhelpers.CtxWithUser(user)
	badHandle := "https://evil.test"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Artists = []Artist{
		{Name: testhelpers.StringPtr("Evil Update IG Artist"), InstagramHandle: &badHandle},
	}

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.Error(err)
	s.Nil(resp)

	var count int64
	s.deps.DB.Model(&catalogm.Artist{}).Where("name = ?", "Evil Update IG Artist").Count(&count)
	s.Equal(int64(0), count)
}

// PSY-1860 at the endpoint the ticket names. UpdateShow has no Resolve, so
// initializeArtist never runs and a nil is_headliner reaches resolveArtistRole,
// whose position-0 fallback used to promote the act the caller said nothing
// about -- giving the show TWO set_type='headliner' rows, with the undesignated
// one winning every `ORDER BY position ASC LIMIT 1` headliner read.
//
// Asserted through the HTTP handler rather than the service alone to cover the
// endpoint the ticket actually names, end to end: the handler's artist mapping,
// the service rule, and the stored rows.
//
// It does NOT by itself guard the handler mapping. Its pair below
// (TestUpdateShow_UndescribedBillStillInfersPositionZero) is what catches the
// tempting "default the flag like create does" edit here: such an edit leaves
// THIS test green (every act would then state a role, so the service skips
// suppression and still writes one headliner) while silently killing position
// inference on undescribed bills. The two are only a guard together.
func (s *ShowHandlerIntegrationSuite) TestUpdateShow_StatedBillDoesNotInferASecondHeadliner() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Stated Bill Show")

	ctx := testhelpers.CtxWithUser(user)
	headliner := "headliner"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Artists = []Artist{
		// States nothing at all.
		{Name: testhelpers.StringPtr("Earth")},
		{Name: testhelpers.StringPtr("Boris"), SetType: &headliner},
	}

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Artists, 2)
	s.False(*resp.Body.Artists[0].IsHeadliner, "the act nobody designated must not be promoted")
	s.True(*resp.Body.Artists[1].IsHeadliner)

	var headlinerRows int64
	s.Require().NoError(s.deps.DB.Model(&catalogm.ShowArtist{}).
		Where("show_id = ? AND set_type = ?", show.ID, "headliner").
		Count(&headlinerRows).Error)
	s.EqualValues(1, headlinerRows, "an update must write exactly the headliner the caller stated")
}

// The guard against relocating the PSY-1860 fix into this handler. Defaulting a
// nil is_headliner here the way create's initializeArtist does would make every
// act "stated", so the service's suppression never arms, its helper goes dead on
// its only call site, and a bill nobody described would be written with NO
// headliner row at all instead of reading position 0.
func (s *ShowHandlerIntegrationSuite) TestUpdateShow_UndescribedBillStillInfersPositionZero() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Undescribed Bill Show")

	ctx := testhelpers.CtxWithUser(user)
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Artists = []Artist{
		// Neither act states set_type or is_headliner.
		{Name: testhelpers.StringPtr("Undescribed Top")},
		{Name: testhelpers.StringPtr("Undescribed Support")},
	}

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Artists, 2)
	s.True(*resp.Body.Artists[0].IsHeadliner, "an undescribed bill must still read position 0 as the headliner")
	s.False(*resp.Body.Artists[1].IsHeadliner)

	var headlinerRows int64
	s.Require().NoError(s.deps.DB.Model(&catalogm.ShowArtist{}).
		Where("show_id = ? AND set_type = ?", show.ID, "headliner").
		Count(&headlinerRows).Error)
	s.EqualValues(1, headlinerRows, "an undescribed bill must not lose its headline slot")
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
// Seeds the headliner as `set_type='headliner'` at position 0 so the winner is
// unambiguous for the display resolvers under test, which rank a curated
// 'headliner' first and fall back to lowest position. That pairing is a fixture
// convenience, not a codebase-wide definition of "headliner":
// internal/services/catalog/headline_slot.go owns the rule and groups the sites
// that diverge. These rows are raw INSERTs with the denormalised dedup columns
// left NULL, so no duplicate guard and no unique index sees them.
// There is no `is_headliner` column on show_artists.
func (s *ShowHandlerIntegrationSuite) createShowForSearch(
	title, headlinerName string,
	supportingArtistNames []string,
	venueName string,
	eventDate time.Time,
) *catalogm.Show {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, venueName, "Phoenix", "AZ")
	headliner := testhelpers.CreateArtist(s.deps.DB, headlinerName)

	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
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
