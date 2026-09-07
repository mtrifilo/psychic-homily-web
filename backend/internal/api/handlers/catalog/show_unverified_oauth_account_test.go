package catalog

import (
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

// An OAuth sign-in whose provider would not vouch for the address creates an
// account with email_verified = false. Show submission is one of the two
// capabilities keyed on that column, and it has to stay closed for such an
// account: an account that reaches this gate open is the reason creating one
// unverified is worth anything.
func (s *ShowHandlerIntegrationSuite) TestCreateShow_UnvouchedOAuthAccountBlocked() {
	user := s.createUnvouchedOAuthUser("unvouched.oauth@test.com", "goth-unvouched-subject")

	_, err := s.handler.CreateShowHandler(testhelpers.CtxWithUser(user), s.newVerifiedVenueShowRequest("Unvouched Show"))

	s.Require().Error(err)
	var se huma.StatusError
	s.Require().True(errors.As(err, &se))
	s.Equal(http.StatusForbidden, se.GetStatus())

	var shows int64
	s.Require().NoError(s.deps.DB.Table("shows").Where("submitted_by = ?", user.ID).Count(&shows).Error)
	s.Equal(int64(0), shows, "a refused submission must write no show")
}

// The gate is the column, not the way the account was made: the same account
// submits once verification flips the flag. This is what makes the refusal a
// hold rather than a permanent exclusion, and it is the outcome the
// verification email produces.
func (s *ShowHandlerIntegrationSuite) TestCreateShow_UnvouchedOAuthAccountAllowedOnceVerified() {
	user := s.createUnvouchedOAuthUser("unvouched.then.verified@test.com", "goth-then-verified-subject")

	s.Require().NoError(s.deps.DB.Model(&authm.User{}).
		Where("id = ?", user.ID).
		Update("email_verified", true).Error)
	var verified authm.User
	s.Require().NoError(s.deps.DB.First(&verified, user.ID).Error)

	resp, err := s.handler.CreateShowHandler(testhelpers.CtxWithUser(&verified), s.newVerifiedVenueShowRequest("Verified Show"))

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal("Verified Show", resp.Body.Title)
}

// createUnvouchedOAuthUser builds the row shape the OAuth create path produces
// for an address no provider vouched for: unverified, with a provider identity
// attached.
func (s *ShowHandlerIntegrationSuite) createUnvouchedOAuthUser(email, subject string) *authm.User {
	s.T().Helper()
	user := &authm.User{
		Email:         testhelpers.StringPtr(email),
		IsActive:      true,
		EmailVerified: false,
	}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: subject,
	}).Error)
	return user
}

// newVerifiedVenueShowRequest builds a submission that would otherwise succeed,
// so a refusal can only come from the verification gate.
func (s *ShowHandlerIntegrationSuite) newVerifiedVenueShowRequest(title string) *CreateShowRequest {
	s.T().Helper()
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: testhelpers.StringPtr("Test Artist")}}
	return req
}
