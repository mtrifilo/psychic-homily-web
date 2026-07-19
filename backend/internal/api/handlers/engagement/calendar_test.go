package engagement

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
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
		CreateTokenFn: func(userID uint, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error) {
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
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

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

func TestCreateCalendarTokenHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		CreateTokenFn: func(_ uint, _ string) (*contracts.CalendarTokenCreateResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.CreateCalendarTokenHandler(ctx, &CreateCalendarTokenRequest{})
	testhelpers.AssertHumaError(t, err, 500)
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
		GetTokenStatusFn: func(userID uint) (*contracts.CalendarTokenStatusResponse, error) {
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
		GetTokenStatusFn: func(userID uint) (*contracts.CalendarTokenStatusResponse, error) {
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

func TestGetCalendarTokenStatusHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		GetTokenStatusFn: func(_ uint) (*contracts.CalendarTokenStatusResponse, error) {
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

// =============================================================================
// getAPIBaseURL tests
// =============================================================================

func TestGetAPIBaseURL(t *testing.T) {
	t.Setenv("API_ADDR", "")

	tests := []struct {
		name        string
		frontendURL string
		expected    string
	}{
		{"production", "https://psychichomily.com", "https://api.psychichomily.com"},
		{"stage", "https://stage.psychichomily.com", "https://api-stage.psychichomily.com"},
		{"development", "http://localhost:3000", "http://localhost:8080"},
		{"empty", "", "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Email: config.EmailConfig{FrontendURL: tt.frontendURL},
			}
			result := getAPIBaseURL(cfg)
			if result != tt.expected {
				t.Errorf("getAPIBaseURL(%q) = %q, want %q", tt.frontendURL, result, tt.expected)
			}
		})
	}

	t.Run("uses API_ADDR in local/dev", func(t *testing.T) {
		t.Setenv("API_ADDR", "localhost:63516")
		cfg := &config.Config{Email: config.EmailConfig{FrontendURL: "http://localhost:3000"}}
		if got := getAPIBaseURL(cfg); got != "http://localhost:63516" {
			t.Errorf("getAPIBaseURL with API_ADDR = %q, want http://localhost:63516", got)
		}
	})
}
