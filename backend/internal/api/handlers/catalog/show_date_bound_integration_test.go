package catalog

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

func (s *ShowHandlerIntegrationSuite) farFutureCreateRequest(venueID uint, override *bool) *CreateShowRequest {
	title := "Far Future Show"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 19, 0)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venueID}}
	req.Body.Artists = []Artist{{Name: testhelpers.StringPtr("Far Future Artist")}}
	req.Body.AllowFarFutureDate = override
	return req
}

func (s *ShowHandlerIntegrationSuite) requireStatus(err error, status int) huma.StatusError {
	s.Require().Error(err)
	var se huma.StatusError
	s.Require().True(errors.As(err, &se), "want a huma status error, got %v", err)
	s.Require().Equal(status, se.GetStatus(), "error: %v", err)
	return se
}

func (s *ShowHandlerIntegrationSuite) showCount() int64 {
	var n int64
	s.Require().NoError(s.deps.DB.Model(&catalogm.Show{}).Count(&n).Error)
	return n
}

// The refusal reaches the caller with its own message, naming the bound and the
// override, rather than the generic create failure.
func (s *ShowHandlerIntegrationSuite) TestCreateShow_FarFutureDateRefusedWithBoundAndOverrideNamed() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	_, err := s.handler.CreateShowHandler(testhelpers.CtxWithUser(admin), s.farFutureCreateRequest(venue.ID, nil))

	se := s.requireStatus(err, http.StatusUnprocessableEntity)
	s.Contains(se.Error(), "more than 18 months ahead")
	s.Contains(se.Error(), "allow_far_future_date")
	s.Contains(se.Error(), "SHOW_EVENT_DATE_BEYOND_BOUND")
	s.Equal(int64(0), s.showCount())
}

func (s *ShowHandlerIntegrationSuite) TestCreateShow_AdminOverrideCreatesFarFutureShow() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	yes := true

	resp, err := s.handler.CreateShowHandler(testhelpers.CtxWithUser(admin), s.farFutureCreateRequest(venue.ID, &yes))

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal(int64(1), s.showCount())
}

func (s *ShowHandlerIntegrationSuite) TestCreateShow_NonAdminOverrideIsForbidden() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	yes := true

	_, err := s.handler.CreateShowHandler(testhelpers.CtxWithUser(user), s.farFutureCreateRequest(venue.ID, &yes))

	se := s.requireStatus(err, http.StatusForbidden)
	s.Contains(se.Error(), "allow_far_future_date is admin-only")
	s.Equal(int64(0), s.showCount())
}

func (s *ShowHandlerIntegrationSuite) TestCreateShow_AnonymousOverrideIsForbidden() {
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	yes := true

	_, err := s.handler.CreateShowHandler(context.Background(), s.farFutureCreateRequest(venue.ID, &yes))

	_ = s.requireStatus(err, http.StatusForbidden)
	s.Equal(int64(0), s.showCount())
}

func (s *ShowHandlerIntegrationSuite) TestGetShows_TicketURLFilterIsExact() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	match := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Match")
	other := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Other")
	prefix := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Prefix")
	s.Require().NoError(s.deps.DB.Model(match).Update("ticket_url", " https://tix.example/e/1 ").Error)
	s.Require().NoError(s.deps.DB.Model(other).Update("ticket_url", "https://tix.example/e/2").Error)
	s.Require().NoError(s.deps.DB.Model(prefix).Update("ticket_url", "https://tix.example/e/1?ref=x").Error)

	resp, err := s.handler.GetShowsHandler(context.Background(), &GetShowsRequest{TicketURL: "https://tix.example/e/1"})

	s.Require().NoError(err)
	s.Require().Len(resp.Body.Shows, 1)
	s.Equal(match.ID, resp.Body.Shows[0].ID)
	s.Equal(int64(1), resp.Body.Total)
}
