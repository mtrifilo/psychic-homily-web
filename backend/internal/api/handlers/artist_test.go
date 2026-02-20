package handlers

import (
	"context"
	"fmt"
	"testing"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

func testArtistHandler() *ArtistHandler {
	return NewArtistHandler(nil, nil)
}

// --- NewArtistHandler ---

func TestNewArtistHandler(t *testing.T) {
	h := testArtistHandler()
	if h == nil {
		t.Fatal("expected non-nil ArtistHandler")
	}
}

// --- DeleteArtistHandler ---

func TestDeleteArtist_NoAuth(t *testing.T) {
	h := testArtistHandler()
	req := &DeleteArtistRequest{ArtistID: "1"}

	_, err := h.DeleteArtistHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestDeleteArtist_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &DeleteArtistRequest{ArtistID: "abc"}

	_, err := h.DeleteArtistHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- AdminUpdateArtistHandler ---

func TestAdminUpdateArtist_NoUser(t *testing.T) {
	h := testArtistHandler()
	req := &AdminUpdateArtistRequest{ArtistID: "1"}

	_, err := h.AdminUpdateArtistHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminUpdateArtist_NonAdmin(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &AdminUpdateArtistRequest{ArtistID: "1"}

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestAdminUpdateArtist_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "abc"}

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestAdminUpdateArtist_EmptyName(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "1"}
	empty := "   "
	req.Body.Name = &empty

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestAdminUpdateArtist_NoFields(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "1"}

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- UpdateArtistBandcampHandler ---

func TestUpdateBandcamp_NoUser(t *testing.T) {
	h := testArtistHandler()
	req := &UpdateArtistBandcampRequest{ArtistID: "1"}

	_, err := h.UpdateArtistBandcampHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestUpdateBandcamp_NonAdmin(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &UpdateArtistBandcampRequest{ArtistID: "1"}

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestUpdateBandcamp_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &UpdateArtistBandcampRequest{ArtistID: "abc"}

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestUpdateBandcamp_InvalidURL(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	url := "https://example.com/music"
	req := &UpdateArtistBandcampRequest{ArtistID: "1"}
	req.Body.BandcampEmbedURL = &url

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestUpdateBandcamp_ProfileOnlyURL(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	url := "https://artist.bandcamp.com"
	req := &UpdateArtistBandcampRequest{ArtistID: "1"}
	req.Body.BandcampEmbedURL = &url

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- UpdateArtistSpotifyHandler ---

func TestUpdateSpotify_NoUser(t *testing.T) {
	h := testArtistHandler()
	req := &UpdateArtistSpotifyRequest{ArtistID: "1"}

	_, err := h.UpdateArtistSpotifyHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestUpdateSpotify_NonAdmin(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &UpdateArtistSpotifyRequest{ArtistID: "1"}

	_, err := h.UpdateArtistSpotifyHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestUpdateSpotify_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &UpdateArtistSpotifyRequest{ArtistID: "abc"}

	_, err := h.UpdateArtistSpotifyHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestUpdateSpotify_InvalidURL(t *testing.T) {
	h := testArtistHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	url := "https://example.com/music"
	req := &UpdateArtistSpotifyRequest{ArtistID: "1"}
	req.Body.SpotifyURL = &url

	_, err := h.UpdateArtistSpotifyHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- Helper function tests ---

func TestIsValidBandcampURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"valid album URL", "https://artist.bandcamp.com/album/cool-album", true},
		{"valid track URL", "https://artist.bandcamp.com/track/cool-track", true},
		{"profile only", "https://artist.bandcamp.com", false},
		{"wrong domain", "https://example.com/album/test", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidBandcampURL(tt.url)
			if result != tt.expected {
				t.Errorf("isValidBandcampURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestIsValidSpotifyURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"valid URL", "https://open.spotify.com/artist/abc123", true},
		{"wrong domain", "https://spotify.com/artist/abc123", false},
		{"missing /artist/", "https://open.spotify.com/track/abc123", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSpotifyURL(tt.url)
			if result != tt.expected {
				t.Errorf("isValidSpotifyURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestNilIfEmpty(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		result := nilIfEmpty("")
		if result != nil {
			t.Errorf("nilIfEmpty(\"\") = %v, want nil", result)
		}
	})

	t.Run("non-empty string returns pointer", func(t *testing.T) {
		result := nilIfEmpty("hello")
		if result == nil {
			t.Fatal("nilIfEmpty(\"hello\") = nil, want non-nil")
		}
		if *result != "hello" {
			t.Errorf("nilIfEmpty(\"hello\") = %q, want \"hello\"", *result)
		}
	})
}

// ============================================================================
// Mock-based tests: SearchArtistsHandler
// ============================================================================

func TestSearchArtists_Success(t *testing.T) {
	mock := &mockArtistService{
		searchArtistsFn: func(query string) ([]*services.ArtistDetailResponse, error) {
			if query != "radio" {
				t.Errorf("expected query='radio', got %q", query)
			}
			return []*services.ArtistDetailResponse{{ID: 1, Name: "Radiohead"}}, nil
		},
	}
	h := NewArtistHandler(mock, nil)

	resp, err := h.SearchArtistsHandler(context.Background(), &SearchArtistsRequest{Query: "radio"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Body.Count)
	}
	if resp.Body.Artists[0].Name != "Radiohead" {
		t.Errorf("expected artist name='Radiohead', got %q", resp.Body.Artists[0].Name)
	}
}

func TestSearchArtists_ServiceError(t *testing.T) {
	mock := &mockArtistService{
		searchArtistsFn: func(_ string) ([]*services.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil)

	_, err := h.SearchArtistsHandler(context.Background(), &SearchArtistsRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ============================================================================
// Mock-based tests: ListArtistsHandler
// ============================================================================

func TestListArtists_Success(t *testing.T) {
	mock := &mockArtistService{
		getArtistsFn: func(filters map[string]interface{}) ([]*services.ArtistDetailResponse, error) {
			return []*services.ArtistDetailResponse{{ID: 1}, {ID: 2}}, nil
		},
	}
	h := NewArtistHandler(mock, nil)

	resp, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 2 {
		t.Errorf("expected count=2, got %d", resp.Body.Count)
	}
}

func TestListArtists_WithFilters(t *testing.T) {
	mock := &mockArtistService{
		getArtistsFn: func(filters map[string]interface{}) ([]*services.ArtistDetailResponse, error) {
			if filters["state"] != "AZ" {
				t.Errorf("expected state='AZ', got %v", filters["state"])
			}
			if filters["city"] != "Phoenix" {
				t.Errorf("expected city='Phoenix', got %v", filters["city"])
			}
			return []*services.ArtistDetailResponse{{ID: 1}}, nil
		},
	}
	h := NewArtistHandler(mock, nil)

	resp, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{State: "AZ", City: "Phoenix"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Body.Count)
	}
}

func TestListArtists_ServiceError(t *testing.T) {
	mock := &mockArtistService{
		getArtistsFn: func(_ map[string]interface{}) ([]*services.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil)

	_, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: GetArtistHandler
// ============================================================================

func TestGetArtist_ByID(t *testing.T) {
	mock := &mockArtistService{
		getArtistFn: func(artistID uint) (*services.ArtistDetailResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			return &services.ArtistDetailResponse{ID: 42, Name: "Test Artist"}, nil
		},
	}
	h := NewArtistHandler(mock, nil)

	resp, err := h.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Test Artist" {
		t.Errorf("expected name='Test Artist', got %q", resp.Body.Name)
	}
}

func TestGetArtist_BySlug(t *testing.T) {
	mock := &mockArtistService{
		getArtistBySlugFn: func(slug string) (*services.ArtistDetailResponse, error) {
			if slug != "the-national" {
				t.Errorf("expected slug='the-national', got %q", slug)
			}
			return &services.ArtistDetailResponse{ID: 10, Slug: "the-national"}, nil
		},
	}
	h := NewArtistHandler(mock, nil)

	resp, err := h.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: "the-national"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Slug != "the-national" {
		t.Errorf("expected slug='the-national', got %q", resp.Body.Slug)
	}
}

func TestGetArtist_NotFound(t *testing.T) {
	mock := &mockArtistService{
		getArtistFn: func(_ uint) (*services.ArtistDetailResponse, error) {
			return nil, apperrors.ErrArtistNotFound(99)
		},
	}
	h := NewArtistHandler(mock, nil)

	_, err := h.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: "99"})
	assertHumaError(t, err, 404)
}

func TestGetArtist_ServiceError(t *testing.T) {
	mock := &mockArtistService{
		getArtistFn: func(_ uint) (*services.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil)

	_, err := h.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: "42"})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: GetArtistShowsHandler
// ============================================================================

func TestGetArtistShows_ByID(t *testing.T) {
	mock := &mockArtistService{
		getShowsForArtistFn: func(artistID uint, timezone string, limit int, timeFilter string) ([]*services.ArtistShowResponse, int64, error) {
			if artistID != 5 {
				t.Errorf("expected artistID=5, got %d", artistID)
			}
			return []*services.ArtistShowResponse{{ID: 100}}, 1, nil
		},
	}
	h := NewArtistHandler(mock, nil)

	resp, err := h.GetArtistShowsHandler(context.Background(), &GetArtistShowsRequest{ArtistID: "5", Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if resp.Body.ArtistID != 5 {
		t.Errorf("expected artist_id=5, got %d", resp.Body.ArtistID)
	}
}

func TestGetArtistShows_BySlug(t *testing.T) {
	mock := &mockArtistService{
		getArtistBySlugFn: func(slug string) (*services.ArtistDetailResponse, error) {
			return &services.ArtistDetailResponse{ID: 10}, nil
		},
		getShowsForArtistFn: func(artistID uint, _ string, _ int, _ string) ([]*services.ArtistShowResponse, int64, error) {
			if artistID != 10 {
				t.Errorf("expected resolved artistID=10, got %d", artistID)
			}
			return []*services.ArtistShowResponse{{ID: 200}}, 1, nil
		},
	}
	h := NewArtistHandler(mock, nil)

	resp, err := h.GetArtistShowsHandler(context.Background(), &GetArtistShowsRequest{ArtistID: "the-national", Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ArtistID != 10 {
		t.Errorf("expected artist_id=10, got %d", resp.Body.ArtistID)
	}
}

func TestGetArtistShows_ArtistNotFound(t *testing.T) {
	mock := &mockArtistService{
		getShowsForArtistFn: func(_ uint, _ string, _ int, _ string) ([]*services.ArtistShowResponse, int64, error) {
			return nil, 0, apperrors.ErrArtistNotFound(99)
		},
	}
	h := NewArtistHandler(mock, nil)

	_, err := h.GetArtistShowsHandler(context.Background(), &GetArtistShowsRequest{ArtistID: "99", Limit: 20})
	assertHumaError(t, err, 404)
}

func TestGetArtistShows_ServiceError(t *testing.T) {
	mock := &mockArtistService{
		getShowsForArtistFn: func(_ uint, _ string, _ int, _ string) ([]*services.ArtistShowResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil)

	_, err := h.GetArtistShowsHandler(context.Background(), &GetArtistShowsRequest{ArtistID: "5", Limit: 20})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: DeleteArtistHandler
// ============================================================================

func TestDeleteArtist_Success(t *testing.T) {
	mock := &mockArtistService{
		deleteArtistFn: func(artistID uint) error {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			return nil
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteArtist_NotFound(t *testing.T) {
	mock := &mockArtistService{
		deleteArtistFn: func(_ uint) error {
			return apperrors.ErrArtistNotFound(99)
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "99"})
	assertHumaError(t, err, 404)
}

func TestDeleteArtist_HasShows(t *testing.T) {
	mock := &mockArtistService{
		deleteArtistFn: func(_ uint) error {
			return apperrors.ErrArtistHasShows(42, 3)
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "42"})
	assertHumaError(t, err, 409)
}

func TestDeleteArtist_ServiceError(t *testing.T) {
	mock := &mockArtistService{
		deleteArtistFn: func(_ uint) error {
			return fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "42"})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: AdminUpdateArtistHandler
// ============================================================================

func TestAdminUpdateArtist_Success(t *testing.T) {
	mock := &mockArtistService{
		updateArtistFn: func(artistID uint, updates map[string]interface{}) (*services.ArtistDetailResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			return &services.ArtistDetailResponse{ID: 42, Name: "Updated"}, nil
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	name := "Updated"
	req := &AdminUpdateArtistRequest{ArtistID: "42"}
	req.Body.Name = &name

	resp, err := h.AdminUpdateArtistHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Updated" {
		t.Errorf("expected name='Updated', got %q", resp.Body.Name)
	}
}

func TestAdminUpdateArtist_NotFound(t *testing.T) {
	mock := &mockArtistService{
		updateArtistFn: func(_ uint, _ map[string]interface{}) (*services.ArtistDetailResponse, error) {
			return nil, apperrors.ErrArtistNotFound(99)
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	name := "Test"
	req := &AdminUpdateArtistRequest{ArtistID: "99"}
	req.Body.Name = &name

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	assertHumaError(t, err, 404)
}

func TestAdminUpdateArtist_AuditLogCalled(t *testing.T) {
	var auditCalled bool
	artistMock := &mockArtistService{
		updateArtistFn: func(_ uint, _ map[string]interface{}) (*services.ArtistDetailResponse, error) {
			return &services.ArtistDetailResponse{ID: 42}, nil
		},
	}
	auditMock := &mockAuditLogService{
		logActionFn: func(actorID uint, action string, entityType string, entityID uint, _ map[string]interface{}) {
			auditCalled = true
			if actorID != 1 {
				t.Errorf("expected actorID=1, got %d", actorID)
			}
			if action != "edit_artist" {
				t.Errorf("expected action='edit_artist', got %q", action)
			}
			if entityType != "artist" {
				t.Errorf("expected entityType='artist', got %q", entityType)
			}
			if entityID != 42 {
				t.Errorf("expected entityID=42, got %d", entityID)
			}
		},
	}
	h := NewArtistHandler(artistMock, auditMock)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	name := "New Name"
	req := &AdminUpdateArtistRequest{ArtistID: "42"}
	req.Body.Name = &name

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !auditCalled {
		t.Error("expected audit log to be called")
	}
}

// ============================================================================
// Mock-based tests: UpdateArtistBandcampHandler
// ============================================================================

func TestUpdateBandcamp_Success(t *testing.T) {
	mock := &mockArtistService{
		updateArtistFn: func(artistID uint, updates map[string]interface{}) (*services.ArtistDetailResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			// Verify bandcamp_embed_url is set
			if _, ok := updates["bandcamp_embed_url"]; !ok {
				t.Error("expected bandcamp_embed_url in updates")
			}
			// Verify social bandcamp profile URL is derived
			if _, ok := updates["bandcamp"]; !ok {
				t.Error("expected bandcamp profile URL in updates")
			}
			return &services.ArtistDetailResponse{ID: 42}, nil
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	url := "https://artist.bandcamp.com/album/cool-album"
	req := &UpdateArtistBandcampRequest{ArtistID: "42"}
	req.Body.BandcampEmbedURL = &url

	resp, err := h.UpdateArtistBandcampHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 42 {
		t.Errorf("expected ID=42, got %d", resp.Body.ID)
	}
}

func TestUpdateBandcamp_ClearURL(t *testing.T) {
	mock := &mockArtistService{
		updateArtistFn: func(_ uint, updates map[string]interface{}) (*services.ArtistDetailResponse, error) {
			// When clearing, bandcamp_embed_url should be a nil *string
			if v, ok := updates["bandcamp_embed_url"]; ok {
				if sp, isStr := v.(*string); isStr && sp != nil {
					t.Errorf("expected nil *string for bandcamp_embed_url, got %q", *sp)
				}
			}
			return &services.ArtistDetailResponse{ID: 42}, nil
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	empty := ""
	req := &UpdateArtistBandcampRequest{ArtistID: "42"}
	req.Body.BandcampEmbedURL = &empty

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// Mock-based tests: UpdateArtistSpotifyHandler
// ============================================================================

func TestUpdateSpotify_Success(t *testing.T) {
	mock := &mockArtistService{
		updateArtistFn: func(artistID uint, updates map[string]interface{}) (*services.ArtistDetailResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			if _, ok := updates["spotify"]; !ok {
				t.Error("expected spotify in updates")
			}
			return &services.ArtistDetailResponse{ID: 42}, nil
		},
	}
	h := NewArtistHandler(mock, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	url := "https://open.spotify.com/artist/abc123"
	req := &UpdateArtistSpotifyRequest{ArtistID: "42"}
	req.Body.SpotifyURL = &url

	resp, err := h.UpdateArtistSpotifyHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 42 {
		t.Errorf("expected ID=42, got %d", resp.Body.ID)
	}
}
