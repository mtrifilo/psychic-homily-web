package engagement

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// feedRequestWithToken builds a GET request whose chi URL param "token" is
// set to the given value, mirroring how the router populates it in
// production. An empty token is added as-is so the missing-token branch can
// be exercised.
func feedRequestWithToken(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/calendar/feed", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", token)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func testCalendarConfig() *config.Config {
	return &config.Config{
		Email: config.EmailConfig{
			FrontendURL: "http://localhost:3000",
		},
	}
}

// =============================================================================
// CreateCalendarTokenHandler tests
// =============================================================================

func TestCreateCalendarTokenHandler_NoAuth(t *testing.T) {
	h := NewCalendarHandler(&testhelpers.MockCalendarService{}, testCalendarConfig())
	_, err := h.CreateCalendarTokenHandler(context.Background(), &CreateCalendarTokenRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCreateCalendarTokenHandler_Success(t *testing.T) {
	now := time.Now()
	mock := &testhelpers.MockCalendarService{
		CreateTokenFn: func(userID uint, _ *time.Time, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error) {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			return &contracts.CalendarTokenCreateResponse{
				Token:     "phcal_abc123",
				FeedURL:   apiBaseURL + "/calendar/phcal_abc123",
				CreatedAt: now,
			}, nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithSession(&authm.User{ID: 1}, time.Now())

	resp, err := h.CreateCalendarTokenHandler(ctx, &CreateCalendarTokenRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Token != "phcal_abc123" {
		t.Errorf("expected token phcal_abc123, got %s", resp.Body.Token)
	}
	if resp.Body.FeedURL == "" {
		t.Error("expected non-empty feed URL")
	}
}

// The feed URL is minted on the backend's public origin as the config
// resolves it, never on the listen address.
func TestCreateCalendarTokenHandler_FeedURLBase(t *testing.T) {
	tests := []struct {
		name   string
		server config.ServerConfig
		front  string
		want   string
	}{
		{"production www frontend on a wildcard listen address", config.ServerConfig{Addr: "0.0.0.0:8080"}, "https://www.psychichomily.com", "https://api.psychichomily.com"},
		{"BACKEND_PUBLIC_URL wins", config.ServerConfig{Addr: "0.0.0.0:8080", BackendPublicURL: "http://localhost:61001/"}, "http://localhost:61000", "http://localhost:61001"},
		{"local frontend uses the listen port", config.ServerConfig{Addr: "localhost:63516"}, "http://localhost:3000", "http://localhost:63516"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBaseURL string
			mock := &testhelpers.MockCalendarService{
				CreateTokenFn: func(_ uint, _ *time.Time, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error) {
					gotBaseURL = apiBaseURL
					return &contracts.CalendarTokenCreateResponse{Token: "phcal_abc123"}, nil
				},
			}
			cfg := &config.Config{Server: tt.server, Email: config.EmailConfig{FrontendURL: tt.front}}
			h := NewCalendarHandler(mock, cfg)
			ctx := testhelpers.CtxWithSession(&authm.User{ID: 1}, time.Now())

			if _, err := h.CreateCalendarTokenHandler(ctx, &CreateCalendarTokenRequest{}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotBaseURL != tt.want {
				t.Errorf("apiBaseURL = %q, want %q", gotBaseURL, tt.want)
			}
		})
	}
}

func TestCreateCalendarTokenHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		CreateTokenFn: func(_ uint, _ *time.Time, _ string) (*contracts.CalendarTokenCreateResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithSession(&authm.User{ID: 1}, time.Now())

	_, err := h.CreateCalendarTokenHandler(ctx, &CreateCalendarTokenRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// The mint is handed the revocation stamp of the user the JWT middleware
// loaded, which is the acting session's.
func TestCreateCalendarTokenHandler_PassesTheSessionsRevocationStamp(t *testing.T) {
	stamp := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	var got *time.Time
	mock := &testhelpers.MockCalendarService{
		CreateTokenFn: func(_ uint, sessionsValidAfter *time.Time, _ string) (*contracts.CalendarTokenCreateResponse, error) {
			got = sessionsValidAfter
			return &contracts.CalendarTokenCreateResponse{Token: "phcal_abc123"}, nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithSession(&authm.User{ID: 1, SessionsValidAfter: &stamp}, time.Now())

	if _, err := h.CreateCalendarTokenHandler(ctx, &CreateCalendarTokenRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || !got.Equal(stamp) {
		t.Errorf("sessionsValidAfter = %v, want %v", got, stamp)
	}
}

// A mint refused because the session was revoked, or the account closed,
// after the middleware validated it answers the middleware's own 401
// TOKEN_INVALID, not a server error.
func TestCreateCalendarTokenHandler_SessionNoLongerValidAnswers401(t *testing.T) {
	for name, refusal := range map[string]error{
		"session revoked": autherrors.ErrTokenInvalid(fmt.Errorf("user 1: %w", shared.ErrSessionRevoked)),
		"account closed":  &autherrors.AuthError{Code: autherrors.CodeAccountInactive, Internal: fmt.Errorf("user 1: %w", shared.ErrSessionAccountClosed)},
	} {
		t.Run(name, func(t *testing.T) {
			mock := &testhelpers.MockCalendarService{
				CreateTokenFn: func(_ uint, _ *time.Time, _ string) (*contracts.CalendarTokenCreateResponse, error) {
					return nil, refusal
				},
			}
			h := NewCalendarHandler(mock, testCalendarConfig())
			ctx := testhelpers.CtxWithSession(&authm.User{ID: 1}, time.Now())

			_, err := h.CreateCalendarTokenHandler(ctx, &CreateCalendarTokenRequest{})

			var refused *middleware.SessionRefusedError
			if !errors.As(err, &refused) {
				t.Fatalf("expected the session refusal, got %v", err)
			}
			if refused.GetStatus() != http.StatusUnauthorized || refused.ErrorCode != autherrors.CodeTokenInvalid {
				t.Errorf("got %d %s, want 401 %s", refused.GetStatus(), refused.ErrorCode, autherrors.CodeTokenInvalid)
			}
			if strings.Contains(refused.Message, "user 1") {
				t.Errorf("the refusal carries no internal text: %q", refused.Message)
			}
			var withHeaders huma.HeadersError
			if !errors.As(err, &withHeaders) ||
				!strings.Contains(strings.Join(withHeaders.GetHeaders().Values("Set-Cookie"), ";"), "auth_token=;") {
				t.Errorf("expected the auth cookie cleared, got %v", err)
			}
		})
	}
}

// =============================================================================
// GetCalendarTokenStatusHandler tests
// =============================================================================

func TestGetCalendarTokenStatusHandler_NoAuth(t *testing.T) {
	h := NewCalendarHandler(&testhelpers.MockCalendarService{}, testCalendarConfig())
	_, err := h.GetCalendarTokenStatusHandler(context.Background(), &GetCalendarTokenStatusRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestGetCalendarTokenStatusHandler_HasToken(t *testing.T) {
	now := time.Now()
	mock := &testhelpers.MockCalendarService{
		GetTokenStatusFn: func(userID uint, _ string) (*contracts.CalendarTokenStatusResponse, error) {
			return &contracts.CalendarTokenStatusResponse{
				HasToken:  true,
				CreatedAt: &now,
			}, nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.HasToken {
		t.Error("expected has_token=true")
	}
}

func TestGetCalendarTokenStatusHandler_NoToken(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		GetTokenStatusFn: func(userID uint, _ string) (*contracts.CalendarTokenStatusResponse, error) {
			return &contracts.CalendarTokenStatusResponse{HasToken: false}, nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.HasToken {
		t.Error("expected has_token=false")
	}
}

// recoverableTokenStatus answers GetTokenStatus for a stored plaintext the way
// the real service does: URLs built on the base passed, none for an empty one.
func recoverableTokenStatus(_ uint, apiBaseURL string) (*contracts.CalendarTokenStatusResponse, error) {
	now := time.Now()
	status := &contracts.CalendarTokenStatusResponse{HasToken: true, CreatedAt: &now}
	if apiBaseURL != "" {
		status.FeedURL = apiBaseURL + "/feeds/phcal_abc/saved-shows.ics"
		status.FollowsFeedURL = apiBaseURL + "/feeds/phcal_abc/follows.atom"
		status.URLRecoverable = true
	}
	return status, nil
}

// recordingStatusService is recoverableTokenStatus, recording the base the
// handler passed.
func recordingStatusService(gotBaseURL *string) *testhelpers.MockCalendarService {
	return &testhelpers.MockCalendarService{
		GetTokenStatusFn: func(userID uint, apiBaseURL string) (*contracts.CalendarTokenStatusResponse, error) {
			*gotBaseURL = apiBaseURL
			return recoverableTokenStatus(userID, apiBaseURL)
		},
	}
}

// A session caller is shown the URLs, built on the same API origin the mint
// uses, and no cache may keep the response.
func TestGetCalendarTokenStatusHandler_SessionGetsURLsUncached(t *testing.T) {
	var gotBaseURL string
	cfg := &config.Config{
		Server: config.ServerConfig{Addr: "0.0.0.0:8080"},
		Email:  config.EmailConfig{FrontendURL: "https://www.psychichomily.com"},
	}
	h := NewCalendarHandler(recordingStatusService(&gotBaseURL), cfg)
	ctx := testhelpers.CtxWithSession(&authm.User{ID: 1}, time.Now().Add(-2*time.Hour))

	resp, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBaseURL != "https://api.psychichomily.com" {
		t.Errorf("apiBaseURL = %q, want the API origin", gotBaseURL)
	}
	if !resp.Body.URLRecoverable || resp.Body.FeedURL == "" || resp.Body.FollowsFeedURL == "" {
		t.Errorf("a session caller must get both URLs, got %+v", resp.Body)
	}
	if resp.CacheControl != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", resp.CacheControl)
	}
}

func TestGetCalendarTokenStatusHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		GetTokenStatusFn: func(_ uint, _ string) (*contracts.CalendarTokenStatusResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// =============================================================================
// DeleteCalendarTokenHandler tests
// =============================================================================

func TestDeleteCalendarTokenHandler_NoAuth(t *testing.T) {
	h := NewCalendarHandler(&testhelpers.MockCalendarService{}, testCalendarConfig())
	_, err := h.DeleteCalendarTokenHandler(context.Background(), &DeleteCalendarTokenRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestDeleteCalendarTokenHandler_Success(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		DeleteTokenFn: func(userID uint) error {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			return nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.DeleteCalendarTokenHandler(ctx, &DeleteCalendarTokenRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestDeleteCalendarTokenHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		DeleteTokenFn: func(_ uint) error {
			return fmt.Errorf("no calendar token found")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteCalendarTokenHandler(ctx, &DeleteCalendarTokenRequest{})
	testhelpers.AssertHumaError(t, err, 422)
}

// =============================================================================
// GetCalendarFeedHandler tests (Chi http.HandlerFunc, token-authenticated)
// =============================================================================

func TestGetCalendarFeedHandler_MissingToken(t *testing.T) {
	h := NewCalendarHandler(&testhelpers.MockCalendarService{}, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetCalendarFeedHandler(w, feedRequestWithToken(""))

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing token status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetCalendarFeedHandler_InvalidToken(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(token string) (*authm.User, error) {
			return nil, fmt.Errorf("token not found")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetCalendarFeedHandler(w, feedRequestWithToken("phcal_bad"))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestGetCalendarFeedHandler_GenerationError(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(token string) (*authm.User, error) {
			return &authm.User{ID: 1}, nil
		},
		GenerateICSFeedFn: func(userID uint, frontendURL string) ([]byte, error) {
			return nil, fmt.Errorf("ics build failed")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetCalendarFeedHandler(w, feedRequestWithToken("phcal_ok"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("generation-error status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetCalendarFeedHandler_Success(t *testing.T) {
	ics := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(token string) (*authm.User, error) {
			if token != "phcal_ok" {
				t.Errorf("unexpected token=%q", token)
			}
			return &authm.User{ID: 7}, nil
		},
		GenerateICSFeedFn: func(userID uint, frontendURL string) ([]byte, error) {
			if userID != 7 {
				t.Errorf("unexpected userID=%d", userID)
			}
			return ics, nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetCalendarFeedHandler(w, feedRequestWithToken("phcal_ok"))

	if w.Code != http.StatusOK {
		t.Fatalf("success status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/calendar; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/calendar; charset=utf-8", ct)
	}
	if w.Body.String() != string(ics) {
		t.Errorf("body = %q, want %q", w.Body.String(), string(ics))
	}
}

func TestGetFollowsActivityFeedHandler_MissingToken(t *testing.T) {
	h := NewCalendarHandler(&testhelpers.MockCalendarService{}, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetFollowsActivityFeedHandler(w, feedRequestWithToken(""))

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing token status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetFollowsActivityFeedHandler_InvalidToken(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(token string) (*authm.User, error) {
			return nil, fmt.Errorf("token not found")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetFollowsActivityFeedHandler(w, feedRequestWithToken("phcal_bad"))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestGetFollowsActivityFeedHandler_GenerationError(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(token string) (*authm.User, error) {
			return &authm.User{ID: 1}, nil
		},
		GenerateFollowsActivityFeedFn: func(userID uint, frontendURL string) ([]byte, error) {
			return nil, fmt.Errorf("atom build failed")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetFollowsActivityFeedHandler(w, feedRequestWithToken("phcal_ok"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("generation-error status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetFollowsActivityFeedHandler_Success(t *testing.T) {
	atom := []byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<feed xmlns="http://www.w3.org/2005/Atom"></feed>`)
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(token string) (*authm.User, error) {
			if token != "phcal_ok" {
				t.Errorf("unexpected token=%q", token)
			}
			return &authm.User{ID: 7}, nil
		},
		GenerateFollowsActivityFeedFn: func(userID uint, frontendURL string) ([]byte, error) {
			if userID != 7 {
				t.Errorf("unexpected userID=%d", userID)
			}
			return atom, nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetFollowsActivityFeedHandler(w, feedRequestWithToken("phcal_ok"))

	if w.Code != http.StatusOK {
		t.Fatalf("success status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/atom+xml") {
		t.Errorf("Content-Type = %q, want application/atom+xml", ct)
	}
	if w.Body.String() != string(atom) {
		t.Errorf("body = %q, want %q", w.Body.String(), string(atom))
	}
}
