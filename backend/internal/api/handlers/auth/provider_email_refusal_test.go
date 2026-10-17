package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/testlog"
)

const refusalRequestID = "provider-email-refusal-request"

// The Apple callback answers a provider email claim no account can store with
// the validation refusal's own code and copy, and logs it at WARN with the
// request id.
func TestAppleCallbackHandler_UnstorableEmailClaimIsAValidationRefusal(t *testing.T) {
	svc := &mockAppleAuthService{findErr: autherrors.ErrProviderEmailUnstorable()}
	h := NewAppleAuthHandler(svc, &testhelpers.MockDiscordService{}, testConfig())
	input := &AppleCallbackRequest{}
	input.Body.IdentityToken = "any-token"

	var resp *AppleCallbackResponse
	var err error
	output := testlog.Capture(t, func() {
		ctx := testlog.Context(t, logger.SetRequestID(context.Background(), refusalRequestID))
		resp, err = h.AppleCallbackHandler(ctx, input)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.Token != "" {
		t.Fatalf("success = %v, token set = %v, want a refusal", resp.Body.Success, resp.Body.Token != "")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("error_code = %q, want %q", resp.Body.ErrorCode, autherrors.CodeValidationFailed)
	}
	if resp.Body.Message != "Validation failed" {
		t.Errorf("message = %q, want %q", resp.Body.Message, "Validation failed")
	}
	requireRefusalLoggedAtWarn(t, output, "msg=apple_auth_refused")
}

// The loopback CLI redirect carries the validation refusal's copy, with no
// code, as it does every refusal's.
func TestOAuthCallbackHandler_CLICallbackCarriesUnstorableEmailCopy(t *testing.T) {
	t.Cleanup(cleanCLICallbackStore)
	const callbackID = "unstorable-email-cli-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-cb")
	req := httptest.NewRequest("GET", "/auth/callback/google", nil)
	req.AddCookie(&http.Cookie{Name: "cli_callback_id", Value: callbackID})

	refusal := fmt.Errorf("failed to find or create user: %w", autherrors.ErrProviderEmailUnstorable())
	query := errorRedirectQuery(t, failingCallbackHandler(refusal), req, "http://localhost:8888/cli-cb")

	if got := query.Get("error"); got != "Validation failed" {
		t.Errorf("error = %q, want %q", got, "Validation failed")
	}
	if query.Has("error_code") {
		t.Errorf("error_code = %q, want the CLI redirect to carry no code", query.Get("error_code"))
	}
}

// The sign-in callback logs a refusal that carries its own copy at WARN with
// its code and the request id, and a fault at ERROR.
func TestOAuthCallbackHandler_LogsRefusalsAtWarnAndFaultsAtError(t *testing.T) {
	run := func(t *testing.T, callbackErr error) string {
		t.Helper()
		handler := failingCallbackHandler(callbackErr)
		return testlog.Capture(t, func() {
			req := httptest.NewRequest("GET", "/auth/callback/google", nil)
			req = req.WithContext(logger.SetRequestID(req.Context(), refusalRequestID))
			handler.OAuthCallbackHTTPHandler(httptest.NewRecorder(), testlog.Request(t, req))
		})
	}

	t.Run("refusals", func(t *testing.T) {
		for _, refusal := range []*autherrors.AuthError{
			autherrors.ErrProviderEmailUnstorable(),
			autherrors.ErrTermsAcceptanceRequired("no consent"),
			autherrors.ErrOAuthLinkRefused("taken@example.com"),
		} {
			t.Run(refusal.Code, func(t *testing.T) {
				const errorTextSentinel = "error-text-sentinel"
				output := run(t, fmt.Errorf("failed to find or create user (%s): %w", errorTextSentinel, refusal))
				requireRefusalLoggedAtWarn(t, output, "msg=oauth_callback_refused")
				if !strings.Contains(output, "provider=google") || !strings.Contains(output, "error_code="+refusal.Code) {
					t.Errorf("refusal log lacks the provider or the code; captured log:\n%s", output)
				}
				if strings.Contains(output, errorTextSentinel) {
					t.Errorf("refusal log carries the error's text; captured log:\n%s", output)
				}
			})
		}
	})

	t.Run("fault", func(t *testing.T) {
		output := run(t, errors.New("oauth2: cannot fetch token"))
		if !strings.Contains(output, "level=ERROR") || !strings.Contains(output, "msg=oauth_callback_failed") {
			t.Fatalf("a fault is not logged at ERROR as oauth_callback_failed; captured log:\n%s", output)
		}
		if strings.Contains(output, "oauth_callback_refused") {
			t.Errorf("a fault is logged as a refusal; captured log:\n%s", output)
		}
	})
}

// requireRefusalLoggedAtWarn asserts output holds the WARN record marker with
// the request id, and no ERROR record.
func requireRefusalLoggedAtWarn(t *testing.T, output, marker string) {
	t.Helper()
	if !strings.Contains(output, marker) {
		t.Fatalf("captured log is missing %q; captured log:\n%s", marker, output)
	}
	if !strings.Contains(output, "level=WARN") || !strings.Contains(output, "request_id="+refusalRequestID) {
		t.Errorf("the refusal is not logged at WARN with the request id; captured log:\n%s", output)
	}
	if strings.Contains(output, "level=ERROR") {
		t.Errorf("a refusal is logged at ERROR; captured log:\n%s", output)
	}
}
