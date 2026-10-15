package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// fixedOAuthCompleter completes every handshake as the same provider identity.
type fixedOAuthCompleter struct {
	user goth.User
}

func (c fixedOAuthCompleter) CompleteUserAuth(http.ResponseWriter, *http.Request) (goth.User, error) {
	return c.user, nil
}

// CompleteOAuthLink is handed an account id, not a loaded account, so the
// account's state is read inside it. These run against a real database because
// the property under test is whether an oauth_accounts row is written.
type OAuthLinkAccountStateSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	cfg    *config.Config
}

func TestOAuthLinkAccountStateSuite(t *testing.T) {
	suite.Run(t, new(OAuthLinkAccountStateSuite))
}

func (s *OAuthLinkAccountStateSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.cfg = &config.Config{
		JWT: config.JWTConfig{SecretKey: "test-secret-oauth-link-account-state", Expiry: 24},
	}
}

func (s *OAuthLinkAccountStateSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *OAuthLinkAccountStateSuite) TearDownTest() {
	s.db.Exec("DELETE FROM user_preferences")
	s.db.Exec("DELETE FROM oauth_accounts")
	s.db.Exec("DELETE FROM users")
}

func (s *OAuthLinkAccountStateSuite) TestCompleteOAuthLink_AccountState() {
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
			email := "link-" + state.name + "@test.com"
			user := &authm.User{Email: &email, IsActive: true}
			s.Require().NoError(s.db.Create(user).Error)
			// The account's state changes between the link start, which ran
			// under a live session, and the callback.
			state.apply(user.ID)

			subject := "google-link-" + state.name
			svc := NewAuthService(s.db, s.cfg, usersvc.NewUserService(s.db))
			svc.SetOAuthCompleter(fixedOAuthCompleter{user: goth.User{Provider: "google", UserID: subject}})

			linked, err := svc.CompleteOAuthLink(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/auth/callback/google", nil), "google", user.ID)

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
			s.Equal(apperrors.ErrAccountInactive().UserMessage(), authErr.UserMessage())
			s.Equal(int64(0), rows, "a refused link must write no identity")
		})
	}
}

// linkRecordingUserService answers GetUserByID from getByID and records
// whether a link was attempted.
type linkRecordingUserService struct {
	nilDBUserService
	getByID    func(userID uint) (*authm.User, error)
	linkCalled bool
}

func (s *linkRecordingUserService) GetUserByID(userID uint) (*authm.User, error) {
	return s.getByID(userID)
}

func (s *linkRecordingUserService) LinkOAuthAccountToUser(uint, goth.User, string) (*authm.User, error) {
	s.linkCalled = true
	return nil, errors.New("link must not be attempted")
}

// An account that cannot be read is not known to be allowed to sign in, so the
// link is not attempted, and the failure is not reported as the account's
// state, which was never established.
func TestCompleteOAuthLink_AccountLookupFailureLinksNothing(t *testing.T) {
	userSvc := &linkRecordingUserService{getByID: func(uint) (*authm.User, error) {
		return nil, fmt.Errorf("failed to get user: %w", gorm.ErrInvalidDB)
	}}
	cfg := &config.Config{JWT: config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24}}
	svc := NewAuthService(nil, cfg, userSvc)
	svc.SetOAuthCompleter(fixedOAuthCompleter{user: goth.User{Provider: "google", UserID: "google-lookup-failure"}})

	linked, err := svc.CompleteOAuthLink(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/auth/callback/google", nil), "google", 7)

	require.Error(t, err)
	assert.Nil(t, linked)
	assert.False(t, userSvc.linkCalled, "a failed account lookup must not fall through to linking")
	assert.ErrorIs(t, err, gorm.ErrInvalidDB)
	var authErr *apperrors.AuthError
	assert.False(t, errors.As(err, &authErr), "a lookup failure is not a refusal with its own copy")
}
