package shared_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
)

func userWithPassword() *authm.User {
	hash := "$2a$not-a-real-hash"
	return &authm.User{ID: 7, PasswordHash: &hash, IsActive: true}
}

// The refusal has to be readable by a caller that cannot follow a redirect, so
// the status and the code are part of the contract, not incidental.
func TestRequireRecentSessionAuth_RefusalCarriesTheCode(t *testing.T) {
	err := shared.RequireRecentSessionAuth(
		testhelpers.CtxWithSessionAuthTime(userWithPassword(), time.Now().Add(-2*time.Hour)),
		"test_mint",
	)
	if err == nil {
		t.Fatal("a stale session must be refused")
	}

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("refusal must be a huma.StatusError, got %T", err)
	}
	if statusErr.GetStatus() != http.StatusForbidden {
		t.Errorf("status = %d, want %d", statusErr.GetStatus(), http.StatusForbidden)
	}

	// Serialized, because what a client can act on is the body huma writes.
	body, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("marshalling the refusal: %v", marshalErr)
	}
	var decoded struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding the refusal body: %v", err)
	}
	if decoded.ErrorCode != autherrors.CodeReauthRequired {
		t.Errorf("error_code = %q, want %q", decoded.ErrorCode, autherrors.CodeReauthRequired)
	}
	if decoded.Success {
		t.Error("a refusal must not report success")
	}
	if decoded.Message == "" {
		t.Error("a refusal must carry copy for a client with no code map")
	}
}

func TestRequireRecentSessionAuth_SessionShapes(t *testing.T) {
	noPassword := &authm.User{ID: 9}

	cases := []struct {
		name    string
		ctx     context.Context
		allowed bool
	}{
		{
			name:    "a session authenticated a moment ago may mint",
			ctx:     testhelpers.CtxWithSessionAuthTime(userWithPassword(), time.Now().Add(-1*time.Minute)),
			allowed: true,
		},
		{
			name:    "a session authenticated two hours ago may not",
			ctx:     testhelpers.CtxWithSessionAuthTime(userWithPassword(), time.Now().Add(-2*time.Hour)),
			allowed: false,
		},
		{
			// An API-token principal, and a session minted before the auth_at
			// claim existed, both arrive this way.
			name:    "a credential establishing no authentication time may not",
			ctx:     testhelpers.CtxWithSessionAuthTime(userWithPassword(), time.Time{}),
			allowed: false,
		},
		{
			// An OAuth-only account is refused on the same rule; the factor it
			// would be challenged for differs, the answer does not.
			name:    "an account with no password is refused when stale",
			ctx:     testhelpers.CtxWithSessionAuthTime(noPassword, time.Now().Add(-2*time.Hour)),
			allowed: false,
		},
		{
			name:    "no principal at all is refused",
			ctx:     context.Background(),
			allowed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := shared.RequireRecentSessionAuth(tc.ctx, "test_mint")
			if tc.allowed && err != nil {
				t.Fatalf("expected the mint to be allowed, got %v", err)
			}
			if !tc.allowed && err == nil {
				t.Fatal("expected the mint to be refused")
			}
		})
	}
}

// The bypass this gate exists to close: POST /auth/refresh renews a session
// with no factor behind it, so a refusal the extra request could buy a way out
// of would be no refusal at all.
//
// The renewal and the middleware's read are both run for real here, which is
// what lets the per-mint tests present a stale authentication time directly:
// this test is the evidence that a refreshed session presents exactly that
// same value.
func TestRequireRecentSessionAuth_RenewalBuysNoFreshness(t *testing.T) {
	user := userWithPassword()
	stale := time.Now().Add(-2 * time.Hour).Truncate(time.Second)

	cfg := &config.Config{JWT: config.JWTConfig{
		SecretKey: "test-secret-key-at-least-32-characters-long",
		Expiry:    24,
	}}
	jwtService := authsvc.NewJWTService(nil, cfg, &testhelpers.MockUserService{
		GetUserByIDFn: func(uint) (*authm.User, error) { return user, nil },
	})

	renewed, err := jwtService.RenewSessionToken(user, stale)
	if err != nil {
		t.Fatalf("renewing the session: %v", err)
	}
	_, renewedAuthAt, err := jwtService.ValidateSession(renewed)
	if err != nil {
		t.Fatalf("reading the renewed session: %v", err)
	}
	if !renewedAuthAt.Equal(stale) {
		t.Fatalf("renewal moved the authentication time to %v, want %v", renewedAuthAt, stale)
	}
	if err := shared.RequireRecentSessionAuth(testhelpers.CtxWithSessionAuthTime(user, renewedAuthAt), "test_mint"); err == nil {
		t.Error("a refreshed stale session must still be refused")
	}
}
