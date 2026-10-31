package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testlog"
)

const errorRedirectFrontendURL = "https://frontend.example.com"

// failingCallbackHandler is an OAuth handler whose sign-in completion always
// fails with err.
func failingCallbackHandler(err error) *OAuthHTTPHandler {
	return callbackHandlerFor(&testhelpers.MockAuthService{
		OAuthCallbackFn: func(
			http.ResponseWriter, *http.Request, string,
		) (*authm.User, string, error) {
			return nil, "", err
		},
	})
}

func callbackHandlerFor(authService *testhelpers.MockAuthService) *OAuthHTTPHandler {
	return NewOAuthHTTPHandler(authService, nil, &config.Config{
		Email: config.EmailConfig{FrontendURL: errorRedirectFrontendURL},
	})
}

// callbackRequest is a callback for provider with rawQuery, carrying chi's
// provider parameter as the router sets it.
func callbackRequest(provider, rawQuery string) *http.Request {
	target := "/auth/callback/" + provider
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", provider)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
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

// assertOnlyCode requires an auth-page query of exactly error_code=want: no
// message text for the page to render, and nothing else.
//
// The key is spelled out rather than read from authPageErrorCodeParam: it is
// the wire contract with the frontend auth page (AUTH_ERROR_CODE_PARAM in
// frontend/lib/auth-href.ts), so renaming the constant has to fail here.
func assertOnlyCode(t *testing.T, query url.Values, want string) {
	t.Helper()
	if got, wantQuery := query.Encode(), (url.Values{"error_code": {want}}).Encode(); got != wantQuery {
		t.Errorf("query = %q, want exactly %q", got, wantQuery)
	}
}

// Every refusal the sign-in callback can hand the auth page reaches it as a
// code and nothing else: a refusal with its own code, a backend fault as
// UNKNOWN, and never text the page could render.
func TestOAuthCallbackErrorRedirect_CarriesOnlyACode(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		err      error
		wantCode string
	}{
		{
			name:     "terms acceptance required",
			provider: "google",
			err:      autherrors.ErrTermsAcceptanceRequired("no consent"),
			wantCode: autherrors.CodeTermsAcceptanceRequired,
		},
		{
			name:     "terms acceptance required, wrapped",
			provider: "google",
			err:      fmt.Errorf("oauth callback: %w", autherrors.ErrTermsAcceptanceRequired("no consent")),
			wantCode: autherrors.CodeTermsAcceptanceRequired,
		},
		{
			// A new identity on a provider the sign-up step does not serve.
			name:     "new identity on a provider without the sign-up step",
			provider: "github",
			err: &contracts.OAuthSignupRequiredError{
				Refusal: autherrors.ErrTermsAcceptanceRequired("new oauth identity arrived without consent"),
			},
			wantCode: autherrors.CodeTermsAcceptanceRequired,
		},
		{
			name:     "age confirmation required",
			provider: "google",
			err:      autherrors.ErrAgeConfirmationRequired("under age"),
			wantCode: autherrors.CodeAgeConfirmationRequired,
		},
		{
			name:     "address already registered",
			provider: "google",
			err:      autherrors.ErrUserExists("taken@example.com"),
			wantCode: autherrors.CodeUserExists,
		},
		{
			name:     "address belongs to an account",
			provider: "google",
			err:      autherrors.ErrOAuthLinkRefused("taken@example.com"),
			wantCode: autherrors.CodeOAuthLinkRefused,
		},
		{
			// A collision on a provider the collision state does not serve.
			name:     "address belongs to an account, provider without the collision state",
			provider: "github",
			err: &contracts.OAuthLinkRequiredError{
				UserID:  3,
				Refusal: autherrors.ErrOAuthLinkRefused("taken@example.com"),
			},
			wantCode: autherrors.CodeOAuthLinkRefused,
		},
		{
			name:     "provider identity linked to another account",
			provider: "google",
			err:      autherrors.ErrOAuthIdentityInUse("google"),
			wantCode: autherrors.CodeOAuthIdentityInUse,
		},
		{
			name:     "account may not sign in, wrapped",
			provider: "google",
			err:      fmt.Errorf("failed to find or create user: %w", autherrors.ErrAccountInactive()),
			wantCode: autherrors.CodeAccountInactive,
		},
		{
			name:     "deleted account inside its recovery window",
			provider: "google",
			err:      autherrors.ErrAccountRecoverable(),
			wantCode: autherrors.CodeAccountRecoverable,
		},
		{
			name:     "provider email claim no account can store, wrapped",
			provider: "google",
			err:      fmt.Errorf("failed to find or create user: %w", autherrors.ErrProviderEmailUnstorable()),
			wantCode: autherrors.CodeValidationFailed,
		},
		{
			name:     "auth error outside the refusal list",
			provider: "google",
			err:      autherrors.ErrServiceUnavailable("database", nil),
			wantCode: autherrors.CodeUnknown,
		},
		{
			name:     "plain provider error",
			provider: "google",
			err:      errors.New("oauth2: cannot fetch token"),
			wantCode: autherrors.CodeUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := errorRedirectQuery(t, failingCallbackHandler(tc.err),
				callbackRequest(tc.provider, "state=s&code=c"), errorRedirectFrontendURL+"/auth")
			assertOnlyCode(t, query, tc.wantCode)
		})
	}
}

// The sign-up step's writer: a new identity whose pending sign-up cannot be
// stored reaches the auth page as a code, not as text.
func TestOAuthCallbackErrorRedirect_SignupStepStoreFailureCarriesOnlyACode(t *testing.T) {
	handler := callbackHandlerFor(&testhelpers.MockAuthService{
		OAuthCallbackFn: func(http.ResponseWriter, *http.Request, string) (*authm.User, string, error) {
			return nil, "", &contracts.OAuthSignupRequiredError{
				Identity: contracts.OAuthIdentity{Provider: "google", Subject: "sub"},
				Refusal:  autherrors.ErrTermsAcceptanceRequired("new oauth identity arrived without consent"),
			}
		},
		StartOAuthSignupFn: func(contracts.OAuthIdentity, string) (string, error) {
			return "", errors.New("pending sign-up insert failed")
		},
	})

	query := errorRedirectQuery(t, handler, callbackRequest("google", "state=s&code=c"), errorRedirectFrontendURL+"/auth")
	assertOnlyCode(t, query, autherrors.CodeUnknown)
}

// The collision state's writer: a collision whose pending link cannot be
// stored reaches the auth page with the refusal's code, not its text.
func TestOAuthCallbackErrorRedirect_LinkStepStoreFailureCarriesOnlyACode(t *testing.T) {
	handler := callbackHandlerFor(&testhelpers.MockAuthService{
		OAuthCallbackFn: func(http.ResponseWriter, *http.Request, string) (*authm.User, string, error) {
			return nil, "", &contracts.OAuthLinkRequiredError{
				Identity: contracts.OAuthIdentity{Provider: "google", Subject: "sub"},
				UserID:   3,
				Refusal:  autherrors.ErrOAuthLinkRefused("taken@example.com"),
			}
		},
		StartOAuthLinkFn: func(contracts.OAuthIdentity, uint, string) (string, error) {
			return "", errors.New("pending link insert failed")
		},
	})

	query := errorRedirectQuery(t, handler, callbackRequest("google", "state=s&code=c"), errorRedirectFrontendURL+"/auth")
	assertOnlyCode(t, query, autherrors.CodeOAuthLinkRefused)
}

// A handler whose sign-in completion fails the test if it runs: a decline has
// no authorization code, so there is nothing to complete.
func declineHandler(t *testing.T) *OAuthHTTPHandler {
	t.Helper()
	return failingCallbackHandlerFn(func() (*authm.User, string, error) {
		t.Error("a decline at the provider ran the handshake")
		return nil, "", errors.New("unexpected handshake")
	})
}

func failingCallbackHandlerFn(fn func() (*authm.User, string, error)) *OAuthHTTPHandler {
	return callbackHandlerFor(&testhelpers.MockAuthService{
		OAuthCallbackFn: func(http.ResponseWriter, *http.Request, string) (*authm.User, string, error) {
			return fn()
		},
	})
}

// Google's decline reaches the auth page as OAUTH_CANCELLED, without running
// the handshake, logged at INFO rather than as a failure.
func TestOAuthCallbackDeclinedAtGoogle(t *testing.T) {
	handler := declineHandler(t)
	req := callbackRequest("google", "error=access_denied&state=s")

	var query url.Values
	output := testlog.Capture(t, func() {
		query = errorRedirectQuery(t, handler, testlog.Request(t, req), errorRedirectFrontendURL+"/auth")
	})

	assertOnlyCode(t, query, "OAUTH_CANCELLED")
	if !strings.Contains(output, "level=INFO") || !strings.Contains(output, "msg=oauth_callback_cancelled") {
		t.Errorf("decline not logged at INFO; captured log:\n%s", output)
	}
	if strings.Contains(output, "level=ERROR") || strings.Contains(output, "level=WARN") {
		t.Errorf("decline logged as a failure; captured log:\n%s", output)
	}
}

// The decline carries the sign-in's destination back, as every refusal does.
func TestOAuthCallbackDeclinedAtGoogle_KeepsTheDestination(t *testing.T) {
	req := callbackRequest("google", "error=access_denied&state=s")
	req.AddCookie(returnToCookie("/shows"))

	query := errorRedirectQuery(t, declineHandler(t), req, errorRedirectFrontendURL+"/auth")

	want := url.Values{"error_code": {"OAUTH_CANCELLED"}, "returnTo": {"/shows"}}
	if query.Encode() != want.Encode() {
		t.Errorf("query = %q, want exactly %q", query.Encode(), want.Encode())
	}
}

// Only Google's decline is named, because the auth page's line for the code
// names Google; any other provider error is not a decline. Both keep the
// generic failure the handshake produces.
func TestOAuthCallbackProviderErrorsThatAreNotGoogleDeclines(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		rawQuery string
	}{
		{"github decline", "github", "error=access_denied&state=s"},
		{"google server error", "google", "error=server_error&state=s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ran := false
			handler := failingCallbackHandlerFn(func() (*authm.User, string, error) {
				ran = true
				return nil, "", errors.New("oauth2: cannot fetch token")
			})

			query := errorRedirectQuery(t, handler, callbackRequest(tc.provider, tc.rawQuery), errorRedirectFrontendURL+"/auth")

			if !ran {
				t.Error("the handshake did not run")
			}
			assertOnlyCode(t, query, autherrors.CodeUnknown)
		})
	}
}

// cliCallbackRequest is a callback carrying the cookie of a CLI sign-in whose
// loopback destination is stored.
func cliCallbackRequest(t *testing.T, rawQuery string) *http.Request {
	t.Helper()
	t.Cleanup(cleanCLICallbackStore)

	const callbackID = "error-redirect-cli-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-cb")
	req := callbackRequest("google", rawQuery)
	req.AddCookie(&http.Cookie{Name: "cli_callback_id", Value: callbackID})
	return req
}

// The loopback CLI redirect keeps the message it has always carried, so an
// older CLI still reads it, and carries the code beside it.
func TestOAuthCallbackErrorRedirect_CLICarriesMessageAndCode(t *testing.T) {
	termsRefusal := autherrors.ErrTermsAcceptanceRequired("no consent")

	for _, tc := range []struct {
		name        string
		handler     func(t *testing.T) *OAuthHTTPHandler
		rawQuery    string
		wantMessage string
		wantCode    string
	}{
		{
			name:        "refusal with its own copy",
			handler:     func(*testing.T) *OAuthHTTPHandler { return failingCallbackHandler(termsRefusal) },
			rawQuery:    "state=s&code=c",
			wantMessage: termsRefusal.UserMessage(),
			wantCode:    autherrors.CodeTermsAcceptanceRequired,
		},
		{
			name: "backend fault",
			handler: func(*testing.T) *OAuthHTTPHandler {
				return failingCallbackHandler(errors.New("oauth2: cannot fetch token"))
			},
			rawQuery:    "state=s&code=c",
			wantMessage: "authentication failed",
			wantCode:    autherrors.CodeUnknown,
		},
		{
			name:        "declined at Google",
			handler:     declineHandler,
			rawQuery:    "error=access_denied&state=s",
			wantMessage: "authentication failed",
			wantCode:    "OAUTH_CANCELLED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := errorRedirectQuery(t, tc.handler(t), cliCallbackRequest(t, tc.rawQuery), "http://localhost:8888/cli-cb")

			want := url.Values{"error": {tc.wantMessage}, "error_code": {tc.wantCode}}
			if query.Encode() != want.Encode() {
				t.Errorf("query = %q, want exactly %q", query.Encode(), want.Encode())
			}
		})
	}
}
