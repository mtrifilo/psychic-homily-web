package admin

import (
	"errors"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// POST /admin/tokens mints a phk_ bearer that outlives the session it was asked
// from by up to a year, so the session has to have proven the account recently.

// IsAdmin is set because that is the principal HumaAdminMiddleware puts in
// front of this handler, not because the handler reads it: it does not. The
// password hash is what the gate reads, and only to label the log.
func adminUser() *authm.User {
	hash := "$2a$not-a-real-hash"
	return &authm.User{ID: 3, IsAdmin: true, IsActive: true, PasswordHash: &hash}
}

// tokenHandlerRecordingMints reports whether the service was reached, which is
// what separates a refusal from a mint that merely returned an error.
func tokenHandlerRecordingMints(minted *bool) *AdminTokenHandler {
	return NewAdminTokenHandler(&testhelpers.MockAPITokenService{
		CreateTokenFn: func(userID uint, description *string, expirationDays int) (*contracts.APITokenCreateResponse, error) {
			*minted = true
			return &contracts.APITokenCreateResponse{ID: 1, Token: "phk_test"}, nil
		},
	})
}

func createTokenRequest() *CreateAPITokenRequest {
	req := &CreateAPITokenRequest{}
	req.Body.ExpirationDays = 90
	return req
}

func TestCreateAPITokenHandler_FreshSessionMints(t *testing.T) {
	var minted bool
	h := tokenHandlerRecordingMints(&minted)

	resp, err := h.CreateAPITokenHandler(
		testhelpers.CtxWithSessionAuthTime(adminUser(), time.Now().Add(-1*time.Minute)),
		createTokenRequest(),
	)
	if err != nil {
		t.Fatalf("a fresh session must mint: %v", err)
	}
	if !minted {
		t.Error("expected the token service to be reached")
	}
	if resp.Body.Token != "phk_test" {
		t.Errorf("token = %q, want phk_test", resp.Body.Token)
	}
}

// A refreshed session arrives carrying the same authentication time it had, so
// it reaches this handler as the stale value below; that the renewal carries
// rather than moves it is established by
// TestRequireRecentSessionAuth_RenewalBuysNoFreshness in handlers/shared. A
// phk_ bearer presents none at all, so one cannot mint its own successor.
func TestCreateAPITokenHandler_SessionsWithoutRecentAuthRefused(t *testing.T) {
	for name, authAt := range map[string]time.Time{
		"stale session":       time.Now().Add(-2 * time.Hour),
		"api-token principal": {},
	} {
		t.Run(name, func(t *testing.T) {
			var minted bool
			h := tokenHandlerRecordingMints(&minted)

			_, err := h.CreateAPITokenHandler(
				testhelpers.CtxWithSessionAuthTime(adminUser(), authAt),
				createTokenRequest(),
			)
			var refusal *shared.ReauthRequiredError
			if !errors.As(err, &refusal) {
				t.Fatalf("expected a re-authentication refusal, got %v", err)
			}
			if minted {
				t.Error("a refused request must not reach the token service")
			}
		})
	}
}
