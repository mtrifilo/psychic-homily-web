package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

const errorRedirectFrontendURL = "https://frontend.example.com"

// failingCallbackHandler is an OAuth handler whose sign-in completion always
// fails with err.
func failingCallbackHandler(err error) *OAuthHTTPHandler {
	authService := &testhelpers.MockAuthService{
		OAuthCallbackWithConsentFn: func(
			http.ResponseWriter, *http.Request, string, *contracts.OAuthSignupConsent,
		) (*authm.User, string, error) {
			return nil, "", err
		},
	}
	return NewOAuthHTTPHandler(authService, nil, &config.Config{
		Email: config.EmailConfig{FrontendURL: errorRedirectFrontendURL},
	})
}

// errorRedirectQuery runs a failed callback, checks that it redirected the
// browser to wantTarget (scheme, host and path), and returns the query.
func errorRedirectQuery(t *testing.T, handler *OAuthHTTPHandler, req *http.Request, wantTarget string) url.Values {
	t.Helper()

	w := httptest.NewRecorder()
	handler.OAuthCallbackHTTPHandler(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusTemporaryRedirect)
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("unparseable Location %q: %v", w.Header().Get("Location"), err)
	}
	if got := location.Scheme + "://" + location.Host + location.Path; got != wantTarget {
		t.Errorf("redirect target = %q, want %q", got, wantTarget)
	}
	return location.Query()
}

// The keys are spelled out rather than read from authPageErrorParam and
// authPageErrorCodeParam: they are the wire contract with the frontend auth
// page (AUTH_ERROR_PARAM and AUTH_ERROR_CODE_PARAM in frontend/lib/auth-href.ts),
// so renaming a constant has to fail here.
//
// A refused sign-in reaches the auth page with its message and its code; a
// backend fault reaches it with the generic message and no code at all.
func TestOAuthCallbackErrorRedirect_CarriesRefusalCode(t *testing.T) {
	termsRefusal := autherrors.ErrTermsAcceptanceRequired("no consent")
	existsRefusal := autherrors.ErrUserExists("taken@example.com")
	identityRefusal := autherrors.ErrOAuthIdentityInUse("google")

	cases := []struct {
		name        string
		err         error
		wantMessage string
		wantCode    string
	}{
		{
			name:        "terms acceptance required",
			err:         termsRefusal,
			wantMessage: termsRefusal.UserMessage(),
			wantCode:    autherrors.CodeTermsAcceptanceRequired,
		},
		{
			name:        "terms acceptance required, wrapped",
			err:         fmt.Errorf("oauth callback: %w", termsRefusal),
			wantMessage: termsRefusal.UserMessage(),
			wantCode:    autherrors.CodeTermsAcceptanceRequired,
		},
		{
			name:        "address already registered",
			err:         existsRefusal,
			wantMessage: existsRefusal.UserMessage(),
			wantCode:    autherrors.CodeUserExists,
		},
		{
			name:        "provider identity linked to another account",
			err:         identityRefusal,
			wantMessage: identityRefusal.UserMessage(),
			wantCode:    autherrors.CodeOAuthIdentityInUse,
		},
		{
			name:        "auth error outside the refusal list",
			err:         autherrors.ErrServiceUnavailable("database", nil),
			wantMessage: "authentication failed",
			wantCode:    "",
		},
		{
			name:        "plain provider error",
			err:         errors.New("oauth2: cannot fetch token"),
			wantMessage: "authentication failed",
			wantCode:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/auth/callback/google", nil)
			query := errorRedirectQuery(t, failingCallbackHandler(tc.err), req, errorRedirectFrontendURL+"/auth")

			if got := query.Get("error"); got != tc.wantMessage {
				t.Errorf("error = %q, want %q", got, tc.wantMessage)
			}
			if tc.wantCode == "" {
				if query.Has("error_code") {
					t.Errorf("error_code = %q, want the parameter absent", query.Get("error_code"))
				}
			} else if got := query.Get("error_code"); got != tc.wantCode {
				t.Errorf("error_code = %q, want %q", got, tc.wantCode)
			}
			for key := range query {
				if key != "error" && key != "error_code" {
					t.Errorf("unexpected query parameter %q in %s", key, query.Encode())
				}
			}
		})
	}
}

// The loopback CLI redirect keeps its message-only shape for the same refusal.
func TestOAuthCallbackErrorRedirect_CLICallbackCarriesNoCode(t *testing.T) {
	t.Cleanup(cleanCLICallbackStore)

	const callbackID = "error-redirect-cli-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-cb")

	termsRefusal := autherrors.ErrTermsAcceptanceRequired("no consent")
	req := httptest.NewRequest("GET", "/auth/callback/google", nil)
	req.AddCookie(&http.Cookie{Name: "cli_callback_id", Value: callbackID})

	query := errorRedirectQuery(t, failingCallbackHandler(termsRefusal), req, "http://localhost:8888/cli-cb")
	if got := query.Get("error"); got != termsRefusal.UserMessage() {
		t.Errorf("error = %q, want %q", got, termsRefusal.UserMessage())
	}
	if query.Has("error_code") {
		t.Errorf("error_code = %q, want the CLI redirect to carry no code", query.Get("error_code"))
	}
}
