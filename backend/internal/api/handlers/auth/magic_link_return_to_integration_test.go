package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
)

// magicLinkEnv is the magic-link send and verify routes over the real user
// and JWT services, the real outbox and the test database, with every link
// the outbox sends captured. The test API keeps no cookies, so each verify is
// a browser that never saw the request: only the emailed token goes along.
type magicLinkEnv struct {
	api    humatest.TestAPI
	outbox *auth.AuthEmailOutbox
	jwt    *auth.JWTService
	mu     sync.Mutex
	tokens []string
}

func (s *AuthHandlerIntegrationSuite) newMagicLinkEnv() *magicLinkEnv {
	env := &magicLinkEnv{}
	email := &testhelpers.MockEmailService{
		IsConfiguredFn: func() bool { return true },
		SendMagicLinkEmailFn: func(_ string, token string) error {
			env.mu.Lock()
			defer env.mu.Unlock()
			env.tokens = append(env.tokens, token)
			return nil
		},
	}
	env.jwt = auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
	env.outbox = auth.NewAuthEmailOutbox(s.deps.DB, s.deps.UserService, env.jwt, email)
	_, api := humatest.New(s.T(), huma.DefaultConfig("magic-link-return-to", "1.0.0"))
	h := NewAuthHandler(auth.NewAuthService(s.deps.DB, s.cfg, s.deps.UserService), env.jwt, s.deps.UserService, email, env.outbox, nil, nil, s.cfg)
	huma.Post(api, "/auth/magic-link/send", h.SendMagicLinkHandler)
	huma.Post(api, "/auth/magic-link/verify", h.VerifyMagicLinkHandler)
	env.api = api
	return env
}

func (e *magicLinkEnv) sent() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.tokens...)
}

// verifiedMagicLinkUser creates an account a magic link is sent to.
func (s *AuthHandlerIntegrationSuite) verifiedMagicLinkUser(email string) *authm.User {
	user := s.createUserWithPassword(email, "strong-password-123!")
	s.verifyEmail(user.ID)
	return user
}

// askForMagicLink posts the send with body and requires the enumeration-safe
// success answer.
func (s *AuthHandlerIntegrationSuite) askForMagicLink(env *magicLinkEnv, body map[string]any) map[string]any {
	s.T().Helper()
	resp := env.api.Post("/auth/magic-link/send", body)
	requireWireSuccess(s.T(), resp)
	return decodeWireBody(s.T(), resp.Body.Bytes())
}

// deliverMagicLink runs the outbox and returns the one link it sent.
func (s *AuthHandlerIntegrationSuite) deliverMagicLink(env *magicLinkEnv) string {
	s.T().Helper()
	before := len(env.sent())
	env.outbox.RunNow(context.Background())
	sent := env.sent()
	s.Require().Len(sent, before+1, "expected the outbox to send one link")
	return sent[len(sent)-1]
}

// openMagicLink posts the verify for token and requires a sign-in.
func (s *AuthHandlerIntegrationSuite) openMagicLink(env *magicLinkEnv, token string) map[string]any {
	s.T().Helper()
	resp := env.api.Post("/auth/magic-link/verify", map[string]any{"token": token})
	requireWireSuccess(s.T(), resp)
	s.Require().NotNil(setCookies(resp)[config.AuthCookieName], "a sign-in sets the session cookie")
	return decodeWireBody(s.T(), resp.Body.Bytes())
}

// latestOutboxRow is the newest auth_email_outbox row of userID.
func (s *AuthHandlerIntegrationSuite) latestOutboxRow(userID uint) authm.AuthEmailOutboxItem {
	s.T().Helper()
	rows := s.outboxRows(userID)
	s.Require().NotEmpty(rows)
	return rows[len(rows)-1]
}

// magicLinkClaimKeys are the claims every plain magic link token carries;
// a link with a destination adds only "dst", the outbox row's id.
var magicLinkClaimKeys = []string{"email", "exp", "iat", "iss", "sub", "user_id"}

func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_TheRequestsDestinationComesBackOnVerify() {
	const destination = "/library?tab=shows#saved"
	user := s.verifiedMagicLinkUser("ml-return@test.com")
	env := s.newMagicLinkEnv()

	asked := s.askForMagicLink(env, map[string]any{"email": "ml-return@test.com", "return_to": destination})
	unknown := s.askForMagicLink(env, map[string]any{"email": "nobody@test.com"})
	s.Equal(unknown, asked, "a destination changes nothing in the send's answer")

	row := s.latestOutboxRow(user.ID)
	s.Equal(authm.AuthEmailKindMagicLink, row.Kind)
	s.Require().NotNil(row.ReturnTo)
	s.Equal(destination, *row.ReturnTo)

	token := s.deliverMagicLink(env)
	payload := tokenPayload(s, token)
	s.ElementsMatch(append([]string{"dst"}, magicLinkClaimKeys...), keysOf(payload),
		"the token names the row and carries nothing of the destination")
	s.Equal(float64(row.ID), payload["dst"])

	body := s.openMagicLink(env, token)
	s.Equal(destination, body["return_to"])
	s.Equal(float64(user.ID), body["user"].(map[string]any)["id"])
}

func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_NoDestinationCarriesNoRef() {
	user := s.verifiedMagicLinkUser("ml-none@test.com")
	env := s.newMagicLinkEnv()

	s.askForMagicLink(env, map[string]any{"email": "ml-none@test.com"})
	s.Nil(s.latestOutboxRow(user.ID).ReturnTo)

	token := s.deliverMagicLink(env)
	s.ElementsMatch(magicLinkClaimKeys, keysOf(tokenPayload(s, token)),
		"a link without a destination carries no destination claim")

	body := s.openMagicLink(env, token)
	s.NotContains(body, "return_to", "the verify answer names no destination")
}

// The emailed URL does not grow with the destination: a destination at the
// rule's length limit, every byte of it one JSON escapes, mints a token as
// long as a short destination's.
func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_TheLinkIsTheSameLengthWhateverTheDestination() {
	long := "/?" + strings.Repeat("<", oauthReturnToMaxLength-2)
	s.Require().Equal(long, magicLinkReturnTo(long), "the rule accepts the long destination")
	env := s.newMagicLinkEnv()

	lengths := map[string]int{}
	// The two addresses are the same length, so the tokens differ only by the
	// digits of their user and row ids: at most a few base64 characters, where
	// a destination carried in the token would add thousands.
	for email, destination := range map[string]string{"ml-shrt@test.com": "/library", "ml-long@test.com": long} {
		user := s.verifiedMagicLinkUser(email)
		s.askForMagicLink(env, map[string]any{"email": email, "return_to": destination})
		s.Require().NotNil(s.latestOutboxRow(user.ID).ReturnTo)
		token := s.deliverMagicLink(env)
		lengths[destination] = len(token)
		s.Equal(destination, s.openMagicLink(env, token)["return_to"])
	}
	s.InDelta(lengths["/library"], lengths[long], 4)
}

// One case of each kind, end to end; the handler unit tests run every case of
// destinationsThatCarryNone and the shared corpus.
func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_ARefusedDestinationIsDropped() {
	user := s.verifiedMagicLinkUser("ml-refused@test.com")
	env := s.newMagicLinkEnv()
	for name, destination := range map[string]string{
		"absolute URL off the site":            "https://evil.example/library",
		"protocol-relative":                    "//evil.example",
		"scheme-carrying":                      "javascript:alert(1)",
		"backslash after the slash":            `/\evil.example`,
		"encoded spelling of the sign-in page": "/%61uth",
	} {
		s.Run(name, func() {
			s.askForMagicLink(env, map[string]any{"email": "ml-refused@test.com", "return_to": destination})
			s.Nil(s.latestOutboxRow(user.ID).ReturnTo, "a refused destination is not stored")
			body := s.openMagicLink(env, s.deliverMagicLink(env))
			s.NotContains(body, "return_to")
		})
	}
}

// sentMagicLinkRow writes a sent magic link row of userID's holding
// destination, the way a send and its delivery leave one, and returns its id.
func (s *AuthHandlerIntegrationSuite) sentMagicLinkRow(userID uint, destination string) uint {
	s.T().Helper()
	row := authm.AuthEmailOutboxItem{
		Kind:     authm.AuthEmailKindMagicLink,
		UserID:   userID,
		Status:   authm.AuthEmailStatusSent,
		ReturnTo: &destination,
	}
	s.Require().NoError(s.deps.DB.Create(&row).Error)
	return row.ID
}

// A row holding a destination the rule refuses, which no request can write,
// still returns none: the verify checks the stored destination again.
func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_VerifyChecksTheStoredDestinationAgain() {
	user := s.verifiedMagicLinkUser("ml-recheck@test.com")
	env := s.newMagicLinkEnv()
	for _, destination := range []string{"//evil.example", "https://evil.example/library", `/\evil.example`, "/auth"} {
		token, err := env.jwt.CreateMagicLinkToken(user.ID, "ml-recheck@test.com", s.sentMagicLinkRow(user.ID, destination))
		s.Require().NoError(err)
		s.NotContains(s.openMagicLink(env, token), "return_to", destination)
	}
}

// A token naming another account's row signs its own account in and returns
// no destination: a ref only ever reads its own account's request.
func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_ARefToAnotherAccountsRowReturnsNone() {
	owner := s.verifiedMagicLinkUser("ml-owner@test.com")
	other := s.verifiedMagicLinkUser("ml-other@test.com")
	env := s.newMagicLinkEnv()
	token, err := env.jwt.CreateMagicLinkToken(other.ID, "ml-other@test.com", s.sentMagicLinkRow(owner.ID, "/library"))
	s.Require().NoError(err)

	body := s.openMagicLink(env, token)
	s.Equal(float64(other.ID), body["user"].(map[string]any)["id"])
	s.NotContains(body, "return_to")
}

// The emailed token is signed whole, so editing the row it names, the one
// part of the link a person who opens it could change, spoils the link.
func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_EditingTheTokensRefSpoilsTheLink() {
	user := s.verifiedMagicLinkUser("ml-edit@test.com")
	env := s.newMagicLinkEnv()
	s.askForMagicLink(env, map[string]any{"email": "ml-edit@test.com", "return_to": "/library"})
	token := s.deliverMagicLink(env)
	otherRow := s.sentMagicLinkRow(user.ID, "/venues")

	for _, edited := range []any{otherRow, nil} {
		resp := env.api.Post("/auth/magic-link/verify", map[string]any{"token": withEditedClaim(s, token, "dst", edited)})
		body := decodeWireBody(s.T(), resp.Body.Bytes())
		s.Equal(false, body["success"], "an edited ref %v", edited)
		s.Equal("INVALID_TOKEN", body["error_code"])
		s.Nil(setCookies(resp)[config.AuthCookieName])
	}

	s.Equal("/library", s.openMagicLink(env, token)["return_to"], "the unedited link still works")
}

// The verify takes its destination from the token alone: a destination sent
// beside the token is not a field the route accepts.
func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_TheVerifyRequestCannotNameOne() {
	user := s.verifiedMagicLinkUser("ml-body@test.com")
	env := s.newMagicLinkEnv()
	token, err := env.jwt.CreateMagicLinkToken(user.ID, "ml-body@test.com", 0)
	s.Require().NoError(err)

	resp := env.api.Post("/auth/magic-link/verify", map[string]any{"token": token, "return_to": "/library"})
	s.Equal(http.StatusUnprocessableEntity, resp.Code, resp.Body.String())
	s.Nil(setCookies(resp)[config.AuthCookieName])
}

// A second request while the first is still queued sends no second email, so
// the one link goes where the first request was headed.
func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_ARepeatWhileQueuedKeepsTheFirstDestination() {
	user := s.verifiedMagicLinkUser("ml-repeat@test.com")
	env := s.newMagicLinkEnv()
	s.askForMagicLink(env, map[string]any{"email": "ml-repeat@test.com", "return_to": "/library"})
	s.askForMagicLink(env, map[string]any{"email": "ml-repeat@test.com", "return_to": "/venues"})
	s.Len(s.outboxRows(user.ID), 1)

	s.Equal("/library", s.openMagicLink(env, s.deliverMagicLink(env))["return_to"])
}

// Only a magic link may carry a destination. The row is otherwise valid, so
// the one check it can violate is auth_email_outbox_return_to_check.
func (s *AuthHandlerIntegrationSuite) TestMagicLinkReturnTo_OnlyAMagicLinkRowCarriesADestination() {
	user := s.verifiedMagicLinkUser("ml-kind@test.com")
	destination := "/library"
	err := s.deps.DB.Create(&authm.AuthEmailOutboxItem{
		Kind:     authm.AuthEmailKindAccountRecovery,
		UserID:   user.ID,
		Status:   authm.AuthEmailStatusPending,
		ReturnTo: &destination,
	}).Error
	s.ErrorIs(err, gorm.ErrCheckConstraintViolated)
}

// tokenPayload is the decoded claims segment of a JWT.
func tokenPayload(s *AuthHandlerIntegrationSuite, token string) map[string]any {
	s.T().Helper()
	parts := strings.Split(token, ".")
	s.Require().Len(parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	s.Require().NoError(err)
	var claims map[string]any
	s.Require().NoError(json.Unmarshal(raw, &claims))
	return claims
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// withEditedClaim is token with claim set to value (nil removes it) and its
// original signature kept.
func withEditedClaim(s *AuthHandlerIntegrationSuite, token, claim string, value any) string {
	s.T().Helper()
	claims := tokenPayload(s, token)
	if value == nil {
		delete(claims, claim)
	} else {
		claims[claim] = value
	}
	raw, err := json.Marshal(claims)
	s.Require().NoError(err)
	parts := strings.Split(token, ".")
	return parts[0] + "." + base64.RawURLEncoding.EncodeToString(raw) + "." + parts[2]
}
