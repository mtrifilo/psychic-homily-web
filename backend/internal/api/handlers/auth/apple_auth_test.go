package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
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
}

func (m *mockAppleAuthService) ValidateIdentityToken(string) (*contracts.AppleIdentityTokenClaims, error) {
	return &contracts.AppleIdentityTokenClaims{
		Email:            "apple@example.com",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "apple-sub-1"},
	}, nil
}

func (m *mockAppleAuthService) FindOrCreateAppleUser(_ *contracts.AppleIdentityTokenClaims, firstName, lastName string) (*authm.User, error) {
	m.called = true
	m.gotFirstName = firstName
	m.gotLastName = lastName
	return &authm.User{ID: 1, Email: strPtr("apple@example.com")}, nil
}

func (m *mockAppleAuthService) GenerateToken(*authm.User) (string, error) { return "apple-token", nil }

// TestAppleCallbackHandler_NameGuard covers the third handler that writes
// users.first_name / users.last_name. Both fields arrive in the REQUEST BODY
// rather than in the identity token, so they are caller-controlled.
//
// Unlike the other two write sites this one DROPS a refused name instead of
// erroring: FindOrCreateAppleUser ignores the names once the Apple account is
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
