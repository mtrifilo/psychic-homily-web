package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testlog"
)

func testAppleAuthHandler() *AppleAuthHandler {
	return NewAppleAuthHandler(nil, nil, testConfig())
}

// --- AppleCallbackHandler ---

func TestAppleCallbackHandler_EmptyToken(t *testing.T) {
	h := testAppleAuthHandler()
	input := &AppleCallbackRequest{}
	// IdentityToken is empty string (zero value)

	resp, err := h.AppleCallbackHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Identity token is required" {
		t.Errorf("expected message 'Identity token is required', got %q", resp.Body.Message)
	}
}

// mockAppleAuthService records the names the handler passes through, so a test
// can assert what would reach the users row.
type mockAppleAuthService struct {
	gotFirstName string
	gotLastName  string
	called       bool
	// findErr, when set, is what FindOrCreateAppleUser returns instead of a user.
	findErr error
}

func (m *mockAppleAuthService) ValidateIdentityToken(context.Context, string) (*contracts.AppleIdentityTokenClaims, error) {
	return &contracts.AppleIdentityTokenClaims{
		Email:            "apple@example.com",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "apple-sub-1"},
	}, nil
}

func (m *mockAppleAuthService) FindOrCreateAppleUser(_ *contracts.AppleIdentityTokenClaims, firstName, lastName string) (*authm.User, error) {
	m.called = true
	m.gotFirstName = firstName
	m.gotLastName = lastName
	if m.findErr != nil {
		return nil, m.findErr
	}
	return &authm.User{ID: 1, Email: strPtr("apple@example.com")}, nil
}

func (m *mockAppleAuthService) GenerateToken(*authm.User) (string, error) { return "apple-token", nil }

// TestAppleCallbackHandler_NameGuard covers the third of the three REQUEST-BODY
// handlers that write users.first_name / users.last_name, the others being
// RegisterHandler and UpdateProfileHandler. Both fields arrive in the request
// body rather than in the identity token, so they are caller-controlled.
//
// Those three are not the whole inventory: the goth OAuth callback
// (services/user/user.go) writes the same columns from provider-asserted values
// and is guarded by nothing. That gap is tracked, not covered here.
//
// Unlike the other two request-body sites this one DROPS a refused name instead
// of erroring: FindOrCreateAppleUser ignores the names once the Apple account is
// known, so refusing would fail an authentication over a discarded field.
func TestAppleCallbackHandler_NameGuard(t *testing.T) {
	overLong := strings.Repeat("x", maxProfileFieldRunes+1)

	cases := []struct {
		name      string
		firstName *string
		lastName  *string
		wantFirst string
		wantLast  string
	}{
		{"clean names pass through", strPtr("Ada"), strPtr("Lovelace"), "Ada", "Lovelace"},
		{"names are trimmed", strPtr("  Ada  "), strPtr("  Lovelace  "), "Ada", "Lovelace"},
		{"control character is dropped", strPtr("evil\nname"), strPtr("Lovelace"), "", "Lovelace"},
		{"over-long first name is dropped", strPtr(overLong), nil, "", ""},
		{"over-long last name is dropped", strPtr("Ada"), strPtr(overLong), "Ada", ""},
		{"absent names stay empty", nil, nil, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockAppleAuthService{}
			h := NewAppleAuthHandler(svc, &testhelpers.MockDiscordService{}, testConfig())
			input := &AppleCallbackRequest{}
			input.Body.IdentityToken = "any-token"
			input.Body.FirstName = tc.firstName
			input.Body.LastName = tc.lastName

			resp, err := h.AppleCallbackHandler(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// A refused name must never abort the sign-in.
			if !resp.Body.Success {
				t.Fatalf("expected success=true, got code=%s message=%q", resp.Body.ErrorCode, resp.Body.Message)
			}
			if !svc.called {
				t.Fatal("expected the sign-in to reach FindOrCreateAppleUser")
			}
			if svc.gotFirstName != tc.wantFirst {
				t.Errorf("first_name reaching the service = %q, want %q", svc.gotFirstName, tc.wantFirst)
			}
			if svc.gotLastName != tc.wantLast {
				t.Errorf("last_name reaching the service = %q, want %q", svc.gotLastName, tc.wantLast)
			}
		})
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubAppleKeyEndpoint answers every request to Apple's key host with body for
// the rest of the test. The Apple service's default client has no transport of
// its own, so its fetches go through http.DefaultTransport.
func stubAppleKeyEndpoint(t *testing.T, body []byte) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "appleid.apple.com" {
			return orig.RoundTrip(r)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    r,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// The key id in an identity token header is caller-supplied text. A token
// whose key id Apple's set lacks is refused, and no warn-or-higher record of
// the attempt carries more than the id's first 8 characters.
func TestAppleCallbackHandler_TokenInvalidLogBoundsKeyID(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	stubAppleKeyEndpoint(t, []byte(`{"keys":[{"kty":"RSA","kid":"known-kid","use":"sig","alg":"RS256","n":"`+n+`","e":"`+e+`"}]}`))

	cfg := testConfig()
	cfg.Apple.BundleID = "com.psychichomily.ios"
	h := NewAppleAuthHandler(authsvc.NewAppleAuthService(nil, cfg, nil), &testhelpers.MockDiscordService{}, cfg)

	callerKid := "kid-from-the-caller-" + strings.Repeat("z", 64)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://appleid.apple.com",
		"aud": cfg.Apple.BundleID,
		"sub": "apple-sub-1",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	token.Header["kid"] = callerKid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	input := &AppleCallbackRequest{}
	input.Body.IdentityToken = signed
	var resp *AppleCallbackResponse
	logs := testlog.Capture(t, func() {
		resp, err = h.AppleCallbackHandler(testlog.Context(t, context.Background()), input)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ErrorCode != autherrors.CodeTokenInvalid {
		t.Fatalf("error_code = %q, want %q", resp.Body.ErrorCode, autherrors.CodeTokenInvalid)
	}

	sawRefusal := false
	for _, line := range strings.Split(logs, "\n") {
		if !strings.Contains(line, "level=WARN") && !strings.Contains(line, "level=ERROR") {
			continue
		}
		if strings.Contains(line, "msg=apple_auth_token_invalid") {
			sawRefusal = true
			if !strings.Contains(line, callerKid[:8]) {
				t.Errorf("the refusal should carry the key id's first 8 characters: %s", line)
			}
		}
		if strings.Contains(line, callerKid[:9]) {
			t.Errorf("a warn-or-higher record carries more than 8 characters of the caller's key id: %s", line)
		}
	}
	if !sawRefusal {
		t.Fatalf("no apple_auth_token_invalid warning was logged:\n%s", logs)
	}
}
