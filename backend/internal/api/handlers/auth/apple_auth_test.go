package auth

import (
	"context"
	"strings"
	"testing"

	autherrors "psychic-homily-backend/internal/errors"
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

// TestAppleCallbackHandler_RejectsMalformedNames covers the third handler that
// writes users.first_name / users.last_name. first_name and last_name arrive in
// the REQUEST BODY, not in the identity token, so they are caller-controlled on
// a public endpoint and take the same guard as registration and profile update.
//
// The refusal lands before ValidateIdentityToken, so a nil service is enough to
// reach it: a token that never gets verified proves the guard ran first.
func TestAppleCallbackHandler_RejectsMalformedNames(t *testing.T) {
	cases := []struct {
		name      string
		firstName *string
		lastName  *string
		wantMsg   string
	}{
		{"first name newline", strPtr("evil\nname"), nil, "First name contains unsupported characters"},
		{"first name too long", strPtr(strings.Repeat("x", maxProfileNameRunes+1)), nil, "First name must be 100 characters or fewer"},
		{"last name newline", nil, strPtr("evil\nname"), "Last name contains unsupported characters"},
		{"last name too long", nil, strPtr(strings.Repeat("x", maxProfileNameRunes+1)), "Last name must be 100 characters or fewer"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := testAppleAuthHandler()
			input := &AppleCallbackRequest{}
			input.Body.IdentityToken = "any-token"
			input.Body.FirstName = tc.firstName
			input.Body.LastName = tc.lastName

			resp, err := h.AppleCallbackHandler(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeValidationFailed {
				t.Errorf("expected validation failure, got success=%v code=%s", resp.Body.Success, resp.Body.ErrorCode)
			}
			if resp.Body.Message != tc.wantMsg {
				t.Errorf("expected message %q, got %q", tc.wantMsg, resp.Body.Message)
			}
		})
	}
}
