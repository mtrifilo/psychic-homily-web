package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// Mock: CalendarServiceInterface
// =============================================================================

type mockCalendarService struct {
	createTokenFn    func(userID uint, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error)
	getTokenStatusFn func(userID uint) (*contracts.CalendarTokenStatusResponse, error)
	deleteTokenFn    func(userID uint) error
	validateTokenFn  func(plainToken string) (*models.User, error)
	generateFeedFn   func(userID uint, frontendURL string) ([]byte, error)
}

func (m *mockCalendarService) CreateToken(userID uint, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error) {
	if m.createTokenFn != nil {
		return m.createTokenFn(userID, apiBaseURL)
	}
	return nil, nil
}

func (m *mockCalendarService) GetTokenStatus(userID uint) (*contracts.CalendarTokenStatusResponse, error) {
	if m.getTokenStatusFn != nil {
		return m.getTokenStatusFn(userID)
	}
	return nil, nil
}

func (m *mockCalendarService) DeleteToken(userID uint) error {
	if m.deleteTokenFn != nil {
		return m.deleteTokenFn(userID)
	}
	return nil
}

func (m *mockCalendarService) ValidateCalendarToken(plainToken string) (*models.User, error) {
	if m.validateTokenFn != nil {
		return m.validateTokenFn(plainToken)
	}
	return nil, nil
}

func (m *mockCalendarService) GenerateICSFeed(userID uint, frontendURL string) ([]byte, error) {
	if m.generateFeedFn != nil {
		return m.generateFeedFn(userID, frontendURL)
	}
	return nil, nil
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
	h := NewCalendarHandler(&mockCalendarService{}, testCalendarConfig())
	_, err := h.CreateCalendarTokenHandler(context.Background(), &CreateCalendarTokenRequest{})
	assertHumaError(t, err, 401)
}

func TestCreateCalendarTokenHandler_Success(t *testing.T) {
	now := time.Now()
	mock := &mockCalendarService{
		createTokenFn: func(userID uint, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error) {
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
	ctx := ctxWithUser(&models.User{ID: 1})

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
	mock := &mockCalendarService{
		createTokenFn: func(_ uint, _ string) (*contracts.CalendarTokenCreateResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.CreateCalendarTokenHandler(ctx, &CreateCalendarTokenRequest{})
	assertHumaError(t, err, 500)
}

// =============================================================================
// GetCalendarTokenStatusHandler tests
// =============================================================================

func TestGetCalendarTokenStatusHandler_NoAuth(t *testing.T) {
	h := NewCalendarHandler(&mockCalendarService{}, testCalendarConfig())
	_, err := h.GetCalendarTokenStatusHandler(context.Background(), &GetCalendarTokenStatusRequest{})
	assertHumaError(t, err, 401)
}

func TestGetCalendarTokenStatusHandler_HasToken(t *testing.T) {
	now := time.Now()
	mock := &mockCalendarService{
		getTokenStatusFn: func(userID uint) (*contracts.CalendarTokenStatusResponse, error) {
			return &contracts.CalendarTokenStatusResponse{
				HasToken:  true,
				CreatedAt: &now,
			}, nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.HasToken {
		t.Error("expected has_token=true")
	}
}

func TestGetCalendarTokenStatusHandler_NoToken(t *testing.T) {
	mock := &mockCalendarService{
		getTokenStatusFn: func(userID uint) (*contracts.CalendarTokenStatusResponse, error) {
			return &contracts.CalendarTokenStatusResponse{HasToken: false}, nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.HasToken {
		t.Error("expected has_token=false")
	}
}

func TestGetCalendarTokenStatusHandler_ServiceError(t *testing.T) {
	mock := &mockCalendarService{
		getTokenStatusFn: func(_ uint) (*contracts.CalendarTokenStatusResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	assertHumaError(t, err, 500)
}

// =============================================================================
// DeleteCalendarTokenHandler tests
// =============================================================================

func TestDeleteCalendarTokenHandler_NoAuth(t *testing.T) {
	h := NewCalendarHandler(&mockCalendarService{}, testCalendarConfig())
	_, err := h.DeleteCalendarTokenHandler(context.Background(), &DeleteCalendarTokenRequest{})
	assertHumaError(t, err, 401)
}

func TestDeleteCalendarTokenHandler_Success(t *testing.T) {
	mock := &mockCalendarService{
		deleteTokenFn: func(userID uint) error {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			return nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.DeleteCalendarTokenHandler(ctx, &DeleteCalendarTokenRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestDeleteCalendarTokenHandler_ServiceError(t *testing.T) {
	mock := &mockCalendarService{
		deleteTokenFn: func(_ uint) error {
			return fmt.Errorf("no calendar token found")
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.DeleteCalendarTokenHandler(ctx, &DeleteCalendarTokenRequest{})
	assertHumaError(t, err, 422)
}

// =============================================================================
// getAPIBaseURL tests
// =============================================================================

func TestGetAPIBaseURL(t *testing.T) {
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
}
