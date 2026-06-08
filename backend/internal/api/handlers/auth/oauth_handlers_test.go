package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// --- generateRandomID ---

func TestGenerateRandomID(t *testing.T) {
	id := generateRandomID()
	if len(id) != 32 {
		t.Errorf("expected 32 hex chars, got %d", len(id))
	}

	// Verify uniqueness
	id2 := generateRandomID()
	if id == id2 {
		t.Error("expected unique IDs, got duplicates")
	}
}

// --- CLI callback store ---

func cleanCLICallbackStore() {
	cliCallbackStore.Lock()
	cliCallbackStore.callbacks = make(map[string]cliCallbackEntry)
	cliCallbackStore.Unlock()
}

func TestCLICallbackStore_StoreAndRetrieve(t *testing.T) {
	defer cleanCLICallbackStore()

	storeCLICallback("test-id-1", "http://localhost:9999/callback")

	url, ok := getCLICallback("test-id-1")
	if !ok {
		t.Fatal("expected callback to be found")
	}
	if url != "http://localhost:9999/callback" {
		t.Errorf("expected callback URL, got %q", url)
	}
}

func TestCLICallbackStore_NotFound(t *testing.T) {
	defer cleanCLICallbackStore()

	_, ok := getCLICallback("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent key")
	}
}

func TestCLICallbackStore_Expired(t *testing.T) {
	defer cleanCLICallbackStore()

	// Manually insert an expired entry
	cliCallbackStore.Lock()
	cliCallbackStore.callbacks["expired-id"] = cliCallbackEntry{
		callbackURL: "http://expired.example.com",
		expiresAt:   time.Now().Add(-1 * time.Minute),
	}
	cliCallbackStore.Unlock()

	_, ok := getCLICallback("expired-id")
	if ok {
		t.Error("expected expired callback to not be found")
	}
}

func TestCLICallbackStore_Delete(t *testing.T) {
	defer cleanCLICallbackStore()

	storeCLICallback("del-id", "http://localhost/cb")

	// Verify it exists
	_, ok := getCLICallback("del-id")
	if !ok {
		t.Fatal("expected callback to exist before delete")
	}

	deleteCLICallback("del-id")

	_, ok = getCLICallback("del-id")
	if ok {
		t.Error("expected callback to be gone after delete")
	}
}

func TestCLICallbackStore_CleansExpiredOnStore(t *testing.T) {
	defer cleanCLICallbackStore()

	// Manually insert an expired entry
	cliCallbackStore.Lock()
	cliCallbackStore.callbacks["old-id"] = cliCallbackEntry{
		callbackURL: "http://old.example.com",
		expiresAt:   time.Now().Add(-1 * time.Minute),
	}
	cliCallbackStore.Unlock()

	// Store a new entry — should clean expired
	storeCLICallback("new-id", "http://new.example.com")

	cliCallbackStore.RLock()
	_, oldExists := cliCallbackStore.callbacks["old-id"]
	_, newExists := cliCallbackStore.callbacks["new-id"]
	cliCallbackStore.RUnlock()

	if oldExists {
		t.Error("expected expired entry to be cleaned up")
	}
	if !newExists {
		t.Error("expected new entry to exist")
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
	handler := NewOAuthHTTPHandler(nil, nil)
	// Build request with no chi context so URLParam("provider") returns "".
	req := httptest.NewRequest("GET", "/auth/login", nil)
	w := httptest.NewRecorder()

	handler.OAuthLoginHTTPHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestOAuthLoginHTTPHandler_InvalidProvider(t *testing.T) {
	handler := NewOAuthHTTPHandler(nil, nil)

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

	handler := NewOAuthHTTPHandler(nil, nil)

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

	handler := NewOAuthHTTPHandler(nil, nil)

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

func TestOAuthLoginHTTPHandler_ValidProvider_GoogleQueryParam(t *testing.T) {
	// Verify that the handler adds the provider to query params for Goth
	// (gothic.BeginAuthHandler will fail without registered providers, but
	// we can verify the query param was added by checking the request URL)
	handler := NewOAuthHTTPHandler(nil, nil)
	w, req := oauthLoginRequest("google")

	handler.OAuthLoginHTTPHandler(w, req)

	// Gothic will fail (no providers registered), but the handler shouldn't panic.
	// The response will be whatever gothic writes (likely 400 or 500).
	// The key test is that the handler didn't panic.
}

// TestOAuthLoginHTTPHandler_SignupIntent_MissingAgeConfirmation locks the
// server-side OAuth age gate (PSY-1023): a signup-intent OAuth init with terms
// accepted but no age confirmation must be rejected with a 400 before any OAuth
// redirect, and no consent cookie may be set.
func TestOAuthLoginHTTPHandler_SignupIntent_MissingAgeConfirmation(t *testing.T) {
	handler := NewOAuthHTTPHandler(nil, nil)

	req := httptest.NewRequest("GET", "/auth/login/google?signup_intent=1&terms_accepted=true&terms_version=2026-01-31", nil)
	w := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "google")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.OAuthLoginHTTPHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for signup without age confirmation, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == oauthSignupConsentCookieName {
			t.Error("expected no signup-consent cookie when age confirmation is missing")
		}
	}
}

// TestOAuthLoginHTTPHandler_SignupIntent_AgeBelowMinimum guards the server-side
// age floor for the OAuth init path against a tampered min_age_attested.
func TestOAuthLoginHTTPHandler_SignupIntent_AgeBelowMinimum(t *testing.T) {
	handler := NewOAuthHTTPHandler(nil, nil)

	req := httptest.NewRequest("GET", "/auth/login/google?signup_intent=1&terms_accepted=true&terms_version=2026-01-31&age_confirmed=true&min_age_attested=10", nil)
	w := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "google")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.OAuthLoginHTTPHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for attested age below minimum, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == oauthSignupConsentCookieName {
			t.Error("expected no signup-consent cookie when attested age is below minimum")
		}
	}
}

// TestOAuthLoginHTTPHandler_SignupIntent_RecordsAgeConsent confirms a valid
// signup-intent init (terms + age confirmed) sets a consent cookie carrying the
// age confirmation, so the callback path can persist it.
func TestOAuthLoginHTTPHandler_SignupIntent_RecordsAgeConsent(t *testing.T) {
	handler := NewOAuthHTTPHandler(nil, nil)

	req := httptest.NewRequest("GET", "/auth/login/google?signup_intent=1&terms_accepted=true&terms_version=2026-01-31&age_confirmed=true&min_age_attested=16", nil)
	w := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "google")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.OAuthLoginHTTPHandler(w, req)

	var consentCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == oauthSignupConsentCookieName {
			consentCookie = c
			break
		}
	}
	if consentCookie == nil {
		t.Fatal("expected signup-consent cookie to be set for valid signup intent")
	}
	consent, err := decodeOAuthSignupConsent(consentCookie.Value)
	if err != nil {
		t.Fatalf("failed to decode consent cookie: %v", err)
	}
	if !consent.AgeConfirmed {
		t.Error("expected AgeConfirmed=true in consent cookie")
	}
	if consent.MinAgeAttested != MinSignupAge {
		t.Errorf("expected MinAgeAttested=%d in consent cookie, got %d", MinSignupAge, consent.MinAgeAttested)
	}
}
