package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
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

// The account's state, and the session's, change between the caller's
// session check and the link, and the link write refuses. This runs against a
// real database because the property under test is whether an oauth_accounts
// row is written.
func (s *JWTIntegrationSuite) TestCompleteOAuthLink_AccountState() {
	states := []struct {
		name  string
		apply func(userID uint)
		// wantCode is the refusal, or "" when the link is written.
		wantCode string
	}{
		{name: "active", apply: func(uint) {}},
		{name: "deactivated", apply: func(userID uint) {
			s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
		}, wantCode: apperrors.CodeAccountInactive},
		{name: "soft-deleted", apply: func(userID uint) {
			s.Require().NoError(usersvc.NewUserService(s.db).SoftDeleteAccount(userID, nil))
		}, wantCode: apperrors.CodeAccountInactive},
		{name: "sessions revoked", apply: func(userID uint) {
			s.Require().NoError(RevokeAllSessions(s.db, &authm.User{ID: userID}, time.Now()))
		}, wantCode: apperrors.CodeTokenInvalid},
	}

	for _, state := range states {
		s.Run(state.name, func() {
			user := s.createActiveUser("link-" + state.name + "@test.com")
			// user is the account as the caller's session check loaded it.
			state.apply(user.ID)

			subject := "google-link-" + state.name
			svc := NewAuthService(s.db, s.cfg, usersvc.NewUserService(s.db))
			svc.SetOAuthCompleter(completingAs(goth.User{Provider: "google", UserID: subject}))

			w, r := linkCallbackRequest()
			linked, err := svc.CompleteOAuthLink(w, r, "google", user.ID, user.SessionsValidAfter)

			var rows int64
			s.Require().NoError(s.db.Model(&authm.OAuthAccount{}).
				Where("provider = ? AND provider_user_id = ?", "google", subject).
				Count(&rows).Error)

			if state.wantCode == "" {
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
			s.Equal(state.wantCode, authErr.Code)
			s.Equal(state.wantCode == apperrors.CodeTokenInvalid, errors.Is(err, shared.ErrSessionRevoked))
			s.Equal(int64(0), rows, "a refused link must write no identity")
		})
	}
}

// linkRecordingUserService records the link it is asked for, and fails any
// account read: CompleteOAuthLink leaves the account to the link write.
type linkRecordingUserService struct {
	nilDBUserService
	t            *testing.T
	linkUserID   uint
	linkStamp    *time.Time
	linkProvider string
}

func (s *linkRecordingUserService) GetUserByID(uint) (*authm.User, error) {
	s.t.Error("CompleteOAuthLink must not read the account itself")
	return nil, nil
}

func (s *linkRecordingUserService) LinkOAuthAccountToUser(userID uint, sessionsValidAfter *time.Time, _ goth.User, provider string) (*authm.User, error) {
	s.linkUserID, s.linkStamp, s.linkProvider = userID, sessionsValidAfter, provider
	return &authm.User{ID: userID}, nil
}

// After the handshake, the link write is the only database work: the account
// and the caller's revocation stamp go to it unread.
func TestCompleteOAuthLink_HandsTheAccountAndStampToTheLinkWrite(t *testing.T) {
	stamp := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	userSvc := &linkRecordingUserService{t: t}
	cfg := &config.Config{JWT: config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24}}
	svc := NewAuthService(nil, cfg, userSvc)
	svc.SetOAuthCompleter(completingAs(goth.User{Provider: "github", UserID: "github-unread"}))

	w, r := linkCallbackRequest()
	linked, err := svc.CompleteOAuthLink(w, r, "github", 7, &stamp)

	require.NoError(t, err)
	assert.Equal(t, uint(7), linked.ID)
	assert.Equal(t, uint(7), userSvc.linkUserID)
	require.NotNil(t, userSvc.linkStamp)
	assert.True(t, stamp.Equal(*userSvc.linkStamp), "the stamp reaches the link write unchanged")
	assert.Equal(t, "github", userSvc.linkProvider)
}

// linkRefusingUserService refuses every link as an account that may not sign
// in.
type linkRefusingUserService struct {
	nilDBUserService
}

func (s *linkRefusingUserService) LinkOAuthAccountToUser(uint, *time.Time, goth.User, string) (*authm.User, error) {
	return nil, apperrors.ErrAccountInactive()
}

// The link write's refusal reaches the caller with its own code.
func TestCompleteOAuthLink_LinkWritersRefusalKeepsItsCode(t *testing.T) {
	cfg := &config.Config{JWT: config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24}}
	svc := NewAuthService(nil, cfg, &linkRefusingUserService{})
	svc.SetOAuthCompleter(completingAs(goth.User{Provider: "google", UserID: "google-closed-in-window"}))

	w, r := linkCallbackRequest()
	linked, err := svc.CompleteOAuthLink(w, r, "google", 7, nil)

	require.Error(t, err)
	assert.Nil(t, linked)
	var authErr *apperrors.AuthError
	require.ErrorAs(t, err, &authErr)
	assert.Equal(t, apperrors.CodeAccountInactive, authErr.Code)
}
