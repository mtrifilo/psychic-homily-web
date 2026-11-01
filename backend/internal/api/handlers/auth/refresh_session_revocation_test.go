package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// POST /auth/refresh renews only the session its validation honored: a
// revocation or closure that commits after the lenient middleware read the
// account is refused with the answer validation gives that session, and the
// renewal never carries the account's newer revocation stamp.

// refreshSeamAuthService is the real auth service with hooks at the refresh
// handler's two calls: beforeReRead runs after the middleware validated the
// session and before the handler re-reads the account, beforeMint after the
// re-read and before the mint. mints counts the mints that ran.
type refreshSeamAuthService struct {
	contracts.AuthServiceInterface
	beforeReRead func()
	beforeMint   func()
	mints        int
}

func (s *refreshSeamAuthService) GetUserProfile(userID uint) (*authm.User, error) {
	if s.beforeReRead != nil {
		s.beforeReRead()
	}
	return s.AuthServiceInterface.GetUserProfile(userID)
}

func (s *refreshSeamAuthService) RefreshUserToken(user *authm.User, authAt time.Time) (string, error) {
	if s.beforeMint != nil {
		s.beforeMint()
	}
	s.mints++
	return s.AuthServiceInterface.RefreshUserToken(user, authAt)
}

// refreshThroughTheMiddleware posts /auth/refresh carrying session as the auth
// cookie through the lenient JWT middleware the route mounts and the real
// handler, around the real services, with seam's hooks installed.
func (s *AuthHandlerIntegrationSuite) refreshThroughTheMiddleware(seam *refreshSeamAuthService, session string) *httptest.ResponseRecorder {
	s.T().Helper()
	h := s.newAuthHandler(false)
	seam.AuthServiceInterface = h.authService
	h.authService = seam

	api, _ := newWireTestAPI(s.T())
	huma.Register(api, huma.Operation{
		OperationID: "refresh",
		Method:      http.MethodPost,
		Path:        "/auth/refresh",
		Middlewares: huma.Middlewares{middleware.LenientHumaJWTMiddleware(s.jwtService(), 7*24*time.Hour)},
	}, h.RefreshTokenHandler)
	return api.Post("/auth/refresh", "Cookie: auth_token="+session)
}

func (s *AuthHandlerIntegrationSuite) jwtService() *auth.JWTService {
	return auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
}

func (s *AuthHandlerIntegrationSuite) sessionFor(user *authm.User) string {
	s.T().Helper()
	token, err := s.jwtService().CreateToken(s.reloadUser(user.ID))
	s.Require().NoError(err)
	return token
}

type refreshWireBody struct {
	Success   bool    `json:"success"`
	Token     *string `json:"token"`
	Message   string  `json:"message"`
	ErrorCode string  `json:"error_code"`
}

func (s *AuthHandlerIntegrationSuite) decodeRefresh(w *httptest.ResponseRecorder) refreshWireBody {
	s.T().Helper()
	var body refreshWireBody
	s.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body), "body: %s", w.Body.String())
	return body
}

// assertRefusedAsValidationRefuses asserts w is the refusal session
// validation gives a revoked or closed session (401 TOKEN_INVALID, "Invalid
// token"), that it carries no token, and that any auth cookie it sets clears
// the cookie rather than issuing one.
func (s *AuthHandlerIntegrationSuite) assertRefusedAsValidationRefuses(w *httptest.ResponseRecorder) {
	s.T().Helper()
	s.Equal(http.StatusUnauthorized, w.Code, "body: %s", w.Body.String())
	body := s.decodeRefresh(w)
	s.False(body.Success)
	s.Equal(autherrors.CodeTokenInvalid, body.ErrorCode)
	s.Equal("Invalid token", body.Message)
	s.Nil(body.Token, "a refused refresh carries no token")
	for _, cookie := range (&http.Response{Header: w.Header()}).Cookies() {
		s.Empty(cookie.Value, "a refused refresh issues no cookie: %s", cookie.String())
		s.Negative(cookie.MaxAge, "a refused refresh only clears the auth cookie: %s", cookie.String())
	}
}

// The mailbox proof's revocation commits after the middleware validated the
// session and before the handler re-reads the account. The refresh mints
// nothing and answers what the middleware answers that session next.
func (s *AuthHandlerIntegrationSuite) TestRefresh_RevocationBetweenValidationAndReReadIsRefused() {
	user := s.createUserWithPassword("refresh-straddles-revocation@test.com", "strong-password-123!")
	session := s.sessionFor(user)
	seam := &refreshSeamAuthService{beforeReRead: func() {
		s.Require().NoError(auth.RevokeAllSessions(s.deps.DB, s.reloadUser(user.ID), time.Now()))
	}}

	w := s.refreshThroughTheMiddleware(seam, session)

	s.assertRefusedAsValidationRefuses(w)
	s.Zero(seam.mints, "no token is minted")
	next := s.refreshThroughTheMiddleware(&refreshSeamAuthService{}, session)
	s.Equal(http.StatusUnauthorized, next.Code)
	nextBody := s.decodeRefresh(next)
	s.Equal(autherrors.CodeTokenInvalid, nextBody.ErrorCode, "validation refuses the same session with the same code")
	s.Equal("Invalid token", nextBody.Message)
}

// An account closed between validation and the re-read is refused the same
// way, whether deactivated or soft-deleted.
func (s *AuthHandlerIntegrationSuite) TestRefresh_ClosureBetweenValidationAndReReadIsRefused() {
	closures := map[string]func(userID uint){
		"deactivated":  s.deactivateUser,
		"soft-deleted": s.softDeleteUser,
	}
	for name, closeAccount := range closures {
		s.Run(name, func() {
			user := s.createUserWithPassword("refresh-straddles-"+name+"@test.com", "strong-password-123!")
			session := s.sessionFor(user)
			seam := &refreshSeamAuthService{beforeReRead: func() { closeAccount(user.ID) }}

			w := s.refreshThroughTheMiddleware(seam, session)

			s.assertRefusedAsValidationRefuses(w)
			s.Zero(seam.mints, "no token is minted")
		})
	}
}

// A refresh with nothing committed between its reads renews, and the renewal
// names the account's current revocation stamp, so validation honors it.
func (s *AuthHandlerIntegrationSuite) TestRefresh_SessionAfterARevocationRenewsWithTheCurrentStamp() {
	user := s.createUserWithPassword("refresh-after-revocation@test.com", "strong-password-123!")
	s.Require().NoError(auth.RevokeAllSessions(s.deps.DB, user, time.Now()))
	session := s.sessionFor(user)
	seam := &refreshSeamAuthService{}

	w := s.refreshThroughTheMiddleware(seam, session)

	s.Require().Equal(http.StatusOK, w.Code, "body: %s", w.Body.String())
	body := s.decodeRefresh(w)
	s.True(body.Success)
	s.Require().NotNil(body.Token)
	s.Equal(1, seam.mints)
	renewed, _, err := s.jwtService().ValidateSession(*body.Token)
	s.Require().NoError(err, "the renewal names the current stamp")
	s.Equal(user.ID, renewed.ID)
}

// A revocation that commits after the re-read, while the token is being
// minted, is not carried forward either: the renewal names the stamp the
// session was validated against, and validation refuses it on first use.
func (s *AuthHandlerIntegrationSuite) TestRefresh_RevocationDuringTheMintLeavesTheRenewalRevoked() {
	user := s.createUserWithPassword("refresh-mint-straddles-revocation@test.com", "strong-password-123!")
	session := s.sessionFor(user)
	seam := &refreshSeamAuthService{beforeMint: func() {
		s.Require().NoError(auth.RevokeAllSessions(s.deps.DB, s.reloadUser(user.ID), time.Now()))
	}}

	w := s.refreshThroughTheMiddleware(seam, session)

	s.Require().Equal(http.StatusOK, w.Code, "body: %s", w.Body.String())
	body := s.decodeRefresh(w)
	s.Require().NotNil(body.Token)
	_, _, err := s.jwtService().ValidateSession(*body.Token)
	s.True(errors.Is(err, servicesshared.ErrSessionRevoked), "the renewal is refused as revoked, got %v", err)
}
