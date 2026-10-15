package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
)

// completingAs returns a completer that finishes every handshake as gothUser.
func completingAs(gothUser goth.User) *MockOAuthCompleter {
	completer := &MockOAuthCompleter{}
	completer.On("CompleteUserAuth", mock.Anything, mock.Anything).Return(gothUser, nil)
	return completer
}

func linkCallbackRequest() (*httptest.ResponseRecorder, *http.Request) {
	return httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/auth/callback/google", nil)
}

// CompleteOAuthLink is handed an account id, not a loaded account, so the
// account's state is read inside it. This runs against a real database because
// the property under test is whether an oauth_accounts row is written.
func (s *JWTIntegrationSuite) TestCompleteOAuthLink_AccountState() {
	states := []struct {
		name    string
		apply   func(userID uint)
		mayLink bool
	}{
		{name: "active", apply: func(uint) {}, mayLink: true},
		{name: "deactivated", apply: func(userID uint) {
			s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
		}},
		{name: "soft-deleted", apply: func(userID uint) {
			s.Require().NoError(usersvc.NewUserService(s.db).SoftDeleteAccount(userID, nil))
		}},
	}

	for _, state := range states {
		s.Run(state.name, func() {
			user := s.createActiveUser("link-" + state.name + "@test.com")
			// The account's state changes between the link start, which ran
			// under a live session, and the callback.
			state.apply(user.ID)

			subject := "google-link-" + state.name
			svc := NewAuthService(s.db, s.cfg, usersvc.NewUserService(s.db))
			svc.SetOAuthCompleter(completingAs(goth.User{Provider: "google", UserID: subject}))

			w, r := linkCallbackRequest()
			linked, err := svc.CompleteOAuthLink(w, r, "google", user.ID)

			var rows int64
			s.Require().NoError(s.db.Model(&authm.OAuthAccount{}).
				Where("provider = ? AND provider_user_id = ?", "google", subject).
				Count(&rows).Error)

			if state.mayLink {
				s.Require().NoError(err)
				s.Require().NotNil(linked)
				s.Equal(user.ID, linked.ID)
				s.Equal(int64(1), rows)
				return
			}

			s.Require().Error(err)
			s.Nil(linked)
			var authErr *apperrors.AuthError
			s.Require().ErrorAs(err, &authErr)
			s.Equal(apperrors.CodeAccountInactive, authErr.Code)
			s.Equal(int64(0), rows, "a refused link must write no identity")
		})
	}
}

// linkRecordingUserService records whether a link was attempted.
type linkRecordingUserService struct {
	stubUserServiceWithGetByID
	linkCalled bool
}

func (s *linkRecordingUserService) LinkOAuthAccountToUser(uint, goth.User, string) (*authm.User, error) {
	s.linkCalled = true
	return nil, errors.New("link must not be attempted")
}

// An account that cannot be read is not known to be allowed to sign in, so the
// link is not attempted, and the failure is not reported as the account's
// state, which was never established.
func TestCompleteOAuthLink_AccountLookupFailureLinksNothing(t *testing.T) {
	userSvc := &linkRecordingUserService{stubUserServiceWithGetByID: stubUserServiceWithGetByID{
		getByID: func(uint) (*authm.User, error) {
			return nil, fmt.Errorf("failed to get user: %w", gorm.ErrInvalidDB)
		},
	}}
	cfg := &config.Config{JWT: config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24}}
	svc := NewAuthService(nil, cfg, userSvc)
	svc.SetOAuthCompleter(completingAs(goth.User{Provider: "google", UserID: "google-lookup-failure"}))

	w, r := linkCallbackRequest()
	linked, err := svc.CompleteOAuthLink(w, r, "google", 7)

	require.Error(t, err)
	assert.Nil(t, linked)
	assert.False(t, userSvc.linkCalled, "a failed account lookup must not fall through to linking")
	assert.ErrorIs(t, err, gorm.ErrInvalidDB)
	var authErr *apperrors.AuthError
	assert.False(t, errors.As(err, &authErr), "a lookup failure is not a refusal with its own copy")
}
