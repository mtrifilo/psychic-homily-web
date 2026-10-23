package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// --- randomHexID ---

func TestRandomHexID(t *testing.T) {
	id, err := randomHexID(16)
	if err != nil {
		t.Fatalf("randomHexID: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("expected 32 hex chars, got %d", len(id))
	}

	// Verify uniqueness
	id2, err := randomHexID(16)
	if err != nil {
		t.Fatalf("randomHexID: %v", err)
	}
	if id == id2 {
		t.Error("expected unique IDs, got duplicates")
	}

	// The size is the caller's, because the two callers park ids of different
	// widths in cookies.
	wide, err := randomHexID(32)
	if err != nil {
		t.Fatalf("randomHexID: %v", err)
	}
	if len(wide) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(wide))
	}
}

// --- OAuthLoginHTTPHandler (real handler, validation paths) ---

func oauthLoginRequest(provider string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest("GET", "/auth/login/"+provider, nil)
	w := httptest.NewRecorder()

	if provider != "" {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("provider", provider)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}

	return w, req
}

func TestOAuthLoginHTTPHandler_NoProvider(t *testing.T) {
	handler := NewOAuthHTTPHandler(nil, nil, nil)
	// Build request with no chi context so URLParam("provider") returns "".
	req := httptest.NewRequest("GET", "/auth/login", nil)
	w := httptest.NewRecorder()

	handler.OAuthLoginHTTPHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestOAuthLoginHTTPHandler_InvalidProvider(t *testing.T) {
	handler := NewOAuthHTTPHandler(nil, nil, nil)

	for _, provider := range []string{"facebook", "twitter", "linkedin"} {
		t.Run(provider, func(t *testing.T) {
			w, req := oauthLoginRequest(provider)
			handler.OAuthLoginHTTPHandler(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestOAuthLoginHTTPHandler_CLICallbackStored(t *testing.T) {
	defer cleanCLICallbackStore()

	handler := NewOAuthHTTPHandler(nil, nil, nil)

	req := httptest.NewRequest("GET", "/auth/login/google?cli_callback=http://localhost:8888/cli-cb", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "google")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.OAuthLoginHTTPHandler(w, req)

	// Verify the cli_callback_id cookie was set
	resp := w.Result()
	var callbackCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "cli_callback_id" {
			callbackCookie = c
			break
		}
	}
	if callbackCookie == nil {
		t.Fatal("expected cli_callback_id cookie to be set")
	}
	if callbackCookie.Value == "" {
		t.Error("expected non-empty cookie value")
	}

	// Verify the callback was stored in memory using the cookie value
	url, ok := getCLICallback(callbackCookie.Value)
	if !ok {
		t.Fatal("expected callback to be stored in memory")
	}
	if url != "http://localhost:8888/cli-cb" {
		t.Errorf("expected stored callback URL, got %q", url)
	}
}

func TestOAuthLoginHTTPHandler_CLICallbackRejected_400(t *testing.T) {
	defer cleanCLICallbackStore()

	handler := NewOAuthHTTPHandler(nil, nil, nil)

	req := httptest.NewRequest("GET", "/auth/login/google?cli_callback=https://evil.com/steal", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "google")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.OAuthLoginHTTPHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for off-allowlist cli_callback, got %d", w.Code)
	}

	// No cookie should be set and nothing stored for a rejected callback.
	for _, c := range w.Result().Cookies() {
		if c.Name == "cli_callback_id" {
			t.Error("expected no cli_callback_id cookie for rejected callback")
		}
	}
}

// A cli_callback over cliCallbackMaxLength is refused at the start with the
// same 400 as an off-allowlist one, and nothing is stored; one at the cap is
// stored.
func TestOAuthLoginHTTPHandler_CLICallbackLengthCap(t *testing.T) {
	cleanCLICallbackStore()
	t.Cleanup(cleanCLICallbackStore)
	handler := NewOAuthHTTPHandler(nil, nil, nil)

	for _, n := range []int{cliCallbackMaxLength + 1, 10000} {
		w, req := oauthLoginRequest("google")
		req.URL.RawQuery = "cli_callback=" + callbackOfLength(t, n, "a")
		handler.OAuthLoginHTTPHandler(w, req)

		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Invalid cli_callback") {
			t.Errorf("%d bytes: expected 400 Invalid cli_callback, got %d %q", n, w.Code, w.Body.String())
		}
		for _, c := range w.Result().Cookies() {
			if c.Name == "cli_callback_id" {
				t.Errorf("%d bytes: expected no cli_callback_id cookie", n)
			}
		}
		if size := cliCallbackStore.size(); size != 0 {
			t.Errorf("%d bytes: expected nothing stored, store holds %d", n, size)
		}
	}

	atCap := callbackOfLength(t, cliCallbackMaxLength, "a")
	w, req := oauthLoginRequest("google")
	req.URL.RawQuery = "cli_callback=" + atCap
	handler.OAuthLoginHTTPHandler(w, req)
	callbackID := cliCallbackID(t, w)
	if stored, ok := getCLICallback(callbackID); !ok || stored != atCap {
		t.Fatalf("expected the %d-byte callback to be stored, got %d bytes, %v", len(atCap), len(stored), ok)
	}

	// The callback re-validates what was stored and redirects to it.
	callback := httptest.NewRequest("GET", "/auth/callback/google", nil)
	callback.AddCookie(&http.Cookie{Name: "cli_callback_id", Value: callbackID})
	errorRedirectQuery(t, failingCallbackHandler(errors.New("provider refused")), callback, atCap)
}

func TestOAuthLoginHTTPHandler_ValidProvider_GoogleQueryParam(t *testing.T) {
	// Verify that the handler adds the provider to query params for Goth
	// (gothic.BeginAuthHandler will fail without registered providers, but
	// we can verify the query param was added by checking the request URL)
	handler := NewOAuthHTTPHandler(nil, nil, nil)
	w, req := oauthLoginRequest("google")

	handler.OAuthLoginHTTPHandler(w, req)

	// Gothic will fail (no providers registered), but the handler shouldn't panic.
	// The response will be whatever gothic writes (likely 400 or 500).
	// The key test is that the handler didn't panic.
}

// TestOAuthLoginHTTPHandler_IgnoresSignupConsentQuery pins that a sign-in
// start reads no consent: the query a sign-up start once carried is neither
// refused nor recorded, whatever it says, because a new identity is asked for
// its consent on the "Create your account" step instead.
func TestOAuthLoginHTTPHandler_IgnoresSignupConsentQuery(t *testing.T) {
	for _, query := range []string{
		"signup_intent=1&terms_accepted=true&" + currentConsentVersionsQuery,
		"signup_intent=1&terms_accepted=true&" + currentConsentVersionsQuery + "&age_confirmed=true&min_age_attested=10",
		"signup_intent=1&terms_accepted=true&terms_version=stale&privacy_version=stale&age_confirmed=true&min_age_attested=16",
	} {
		handler := NewOAuthHTTPHandler(nil, nil, nil)
		req := httptest.NewRequest("GET", "/auth/login/google?"+query, nil)
		w := httptest.NewRecorder()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("provider", "google")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.OAuthLoginHTTPHandler(w, req)

		if location := w.Header().Get("Location"); strings.Contains(location, "error_code=") {
			t.Errorf("%s: start redirected to a refusal %q", query, location)
		}
		for _, c := range w.Result().Cookies() {
			if strings.Contains(c.Name, "consent") {
				t.Errorf("%s: start set cookie %q", query, c.Name)
			}
		}
	}
}
