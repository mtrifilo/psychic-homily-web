package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func testArtistHandler() *ArtistHandler {
	return NewArtistHandler(nil, nil, nil)
}

// --- DeleteArtistHandler ---

func TestDeleteArtist_NoAuth(t *testing.T) {
	h := testArtistHandler()
	req := &DeleteArtistRequest{ArtistID: "1"}

	_, err := h.DeleteArtistHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestDeleteArtist_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &DeleteArtistRequest{ArtistID: "abc"}

	_, err := h.DeleteArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminUpdateArtist_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "abc"}

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminUpdateArtist_EmptyName(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "1"}
	empty := "   "
	req.Body.Name = &empty

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminUpdateArtist_NoFields(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "1"}

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// PSY-525: URL scheme validation rejects non-http(s) schemes on social URL
// fields. The underlying `utils.ValidateHTTPURL` is exercised exhaustively in
// `internal/utils/url_test.go`; this test asserts the handler integrates the
// validator and returns 422 (not 400) for semantic rejection.
func TestAdminUpdateArtist_RejectsJavaScriptScheme(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "1"}
	bad := "javascript:alert(1)"
	req.Body.Instagram = &bad

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminCreateArtist_RejectsJavaScriptScheme(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateArtistRequest{}
	req.Body.Name = "Test"
	bad := "javascript:alert(1)"
	req.Body.Website = &bad

	_, err := h.AdminCreateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// --- UpdateArtistBandcampHandler ---

func TestUpdateBandcamp_NoUser(t *testing.T) {
	h := testArtistHandler()
	req := &UpdateArtistBandcampRequest{ArtistID: "1"}

	_, err := h.UpdateArtistBandcampHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUpdateBandcamp_NonAdmin(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: false})
	req := &UpdateArtistBandcampRequest{ArtistID: "1"}

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUpdateBandcamp_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &UpdateArtistBandcampRequest{ArtistID: "abc"}

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateBandcamp_InvalidURL(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	url := "https://example.com/music"
	req := &UpdateArtistBandcampRequest{ArtistID: "1"}
	req.Body.BandcampEmbedURL = &url

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestUpdateBandcamp_ProfileOnlyURL(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	url := "https://artist.bandcamp.com"
	req := &UpdateArtistBandcampRequest{ArtistID: "1"}
	req.Body.BandcampEmbedURL = &url

	_, err := h.UpdateArtistBandcampHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// --- UpdateArtistSpotifyHandler ---

func TestUpdateSpotify_NoUser(t *testing.T) {
	h := testArtistHandler()
	req := &UpdateArtistSpotifyRequest{ArtistID: "1"}

	_, err := h.UpdateArtistSpotifyHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUpdateSpotify_NonAdmin(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: false})
	req := &UpdateArtistSpotifyRequest{ArtistID: "1"}

	_, err := h.UpdateArtistSpotifyHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUpdateSpotify_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &UpdateArtistSpotifyRequest{ArtistID: "abc"}

	_, err := h.UpdateArtistSpotifyHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateSpotify_InvalidURL(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	url := "https://example.com/music"
	req := &UpdateArtistSpotifyRequest{ArtistID: "1"}
	req.Body.SpotifyURL = &url

	_, err := h.UpdateArtistSpotifyHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
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

// ============================================================================
// Mock-based tests: SearchArtistsHandler
// ============================================================================

func TestSearchArtists_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		SearchArtistsFn: func(query string) ([]*contracts.ArtistDetailResponse, error) {
			if query != "radio" {
				t.Errorf("expected query='radio', got %q", query)
			}
			return []*contracts.ArtistDetailResponse{{ID: 1, Name: "Radiohead"}}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

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
	mock := &testhelpers.MockArtistService{
		SearchArtistsFn: func(_ string) ([]*contracts.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.SearchArtistsHandler(context.Background(), &SearchArtistsRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ============================================================================
// Mock-based tests: ListArtistsHandler
// ============================================================================

func TestListArtists_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(filters map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
			return []*contracts.ArtistWithShowCountResponse{
				{ArtistDetailResponse: contracts.ArtistDetailResponse{ID: 1, Name: "Artist A"}, UpcomingShowCount: 3},
				{ArtistDetailResponse: contracts.ArtistDetailResponse{ID: 2, Name: "Artist B"}, UpcomingShowCount: 1},
			}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 2 {
		t.Errorf("expected count=2, got %d", resp.Body.Count)
	}
	if resp.Body.Artists[0].UpcomingShowCount != 3 {
		t.Errorf("expected first artist show count=3, got %d", resp.Body.Artists[0].UpcomingShowCount)
	}
}

func TestListArtists_WithFilters(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(filters map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
			if filters["state"] != "AZ" {
				t.Errorf("expected state='AZ', got %v", filters["state"])
			}
			if filters["city"] != "Phoenix" {
				t.Errorf("expected city='Phoenix', got %v", filters["city"])
			}
			return []*contracts.ArtistWithShowCountResponse{
				{ArtistDetailResponse: contracts.ArtistDetailResponse{ID: 1}, UpcomingShowCount: 2},
			}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{State: "AZ", City: "Phoenix"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Body.Count)
	}
}

func TestListArtists_ServiceError(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(_ map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: GetArtistHandler
// ============================================================================

func TestGetArtist_ByID(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistFn: func(artistID uint) (*contracts.ArtistDetailResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			return &contracts.ArtistDetailResponse{ID: 42, Name: "Test Artist"}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Test Artist" {
		t.Errorf("expected name='Test Artist', got %q", resp.Body.Name)
	}
}

func TestGetArtist_BySlug(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
			if slug != "the-national" {
				t.Errorf("expected slug='the-national', got %q", slug)
			}
			return &contracts.ArtistDetailResponse{ID: 10, Slug: "the-national"}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: "the-national"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Slug != "the-national" {
		t.Errorf("expected slug='the-national', got %q", resp.Body.Slug)
	}
}

func TestGetArtist_NotFound(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistFn: func(_ uint) (*contracts.ArtistDetailResponse, error) {
			return nil, apperrors.ErrArtistNotFound(99)
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetArtist_ServiceError(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistFn: func(_ uint) (*contracts.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.GetArtistHandler(context.Background(), &GetArtistRequest{ArtistID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: GetArtistShowsHandler
// ============================================================================

func TestGetArtistShows_ByID(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetShowsForArtistFn: func(artistID uint, timezone string, limit int, timeFilter string) ([]*contracts.ArtistShowResponse, int64, error) {
			if artistID != 5 {
				t.Errorf("expected artistID=5, got %d", artistID)
			}
			return []*contracts.ArtistShowResponse{{ID: 100}}, 1, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

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
	mock := &testhelpers.MockArtistService{
		GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
			return &contracts.ArtistDetailResponse{ID: 10}, nil
		},
		GetShowsForArtistFn: func(artistID uint, _ string, _ int, _ string) ([]*contracts.ArtistShowResponse, int64, error) {
			if artistID != 10 {
				t.Errorf("expected resolved artistID=10, got %d", artistID)
			}
			return []*contracts.ArtistShowResponse{{ID: 200}}, 1, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.GetArtistShowsHandler(context.Background(), &GetArtistShowsRequest{ArtistID: "the-national", Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ArtistID != 10 {
		t.Errorf("expected artist_id=10, got %d", resp.Body.ArtistID)
	}
}

func TestGetArtistShows_ArtistNotFound(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetShowsForArtistFn: func(_ uint, _ string, _ int, _ string) ([]*contracts.ArtistShowResponse, int64, error) {
			return nil, 0, apperrors.ErrArtistNotFound(99)
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.GetArtistShowsHandler(context.Background(), &GetArtistShowsRequest{ArtistID: "99", Limit: 20})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetArtistShows_ServiceError(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetShowsForArtistFn: func(_ uint, _ string, _ int, _ string) ([]*contracts.ArtistShowResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.GetArtistShowsHandler(context.Background(), &GetArtistShowsRequest{ArtistID: "5", Limit: 20})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: DeleteArtistHandler
// ============================================================================

func TestDeleteArtist_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		DeleteArtistFn: func(artistID uint) error {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			return nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteArtist_NotFound(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		DeleteArtistFn: func(_ uint) error {
			return apperrors.ErrArtistNotFound(99)
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteArtist_HasShows(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		DeleteArtistFn: func(_ uint) error {
			return apperrors.ErrArtistHasShows(42, 3)
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "42"})
	testhelpers.AssertHumaError(t, err, 409)
}

func TestDeleteArtist_ServiceError(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		DeleteArtistFn: func(_ uint) error {
			return fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: AdminUpdateArtistHandler
// ============================================================================

func TestAdminUpdateArtist_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		UpdateArtistFn: func(artistID uint, updates map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			return &contracts.ArtistDetailResponse{ID: 42, Name: "Updated"}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
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
	mock := &testhelpers.MockArtistService{
		UpdateArtistFn: func(_ uint, _ map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
			return nil, apperrors.ErrArtistNotFound(99)
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	name := "Test"
	req := &AdminUpdateArtistRequest{ArtistID: "99"}
	req.Body.Name = &name

	_, err := h.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminUpdateArtist_AuditLogCalled(t *testing.T) {
	var auditCalled bool
	artistMock := &testhelpers.MockArtistService{
		UpdateArtistFn: func(_ uint, _ map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
			return &contracts.ArtistDetailResponse{ID: 42}, nil
		},
	}
	auditMock := &testhelpers.MockAuditLogService{
		LogActionFn: func(actorID uint, action string, entityType string, entityID uint, _ map[string]interface{}) {
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
	h := NewArtistHandler(artistMock, auditMock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
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
	mock := &testhelpers.MockArtistService{
		UpdateArtistFn: func(artistID uint, updates map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
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
			return &contracts.ArtistDetailResponse{ID: 42}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
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
	mock := &testhelpers.MockArtistService{
		UpdateArtistFn: func(_ uint, updates map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
			// When clearing, bandcamp_embed_url should be a nil *string
			if v, ok := updates["bandcamp_embed_url"]; ok {
				if sp, isStr := v.(*string); isStr && sp != nil {
					t.Errorf("expected nil *string for bandcamp_embed_url, got %q", *sp)
				}
			}
			return &contracts.ArtistDetailResponse{ID: 42}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
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
	mock := &testhelpers.MockArtistService{
		UpdateArtistFn: func(artistID uint, updates map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			if _, ok := updates["spotify"]; !ok {
				t.Error("expected spotify in updates")
			}
			return &contracts.ArtistDetailResponse{ID: 42}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
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

// ============================================================================
// Mock-based tests: GetArtistCitiesHandler
// ============================================================================

func TestGetArtistCities_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistCitiesFn: func() ([]*contracts.ArtistCityResponse, error) {
			return []*contracts.ArtistCityResponse{
				{City: "Phoenix", State: "AZ", ArtistCount: 10},
				{City: "Mesa", State: "AZ", ArtistCount: 5},
			}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.GetArtistCitiesHandler(context.Background(), &GetArtistCitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Cities) != 2 {
		t.Errorf("expected 2 cities, got %d", len(resp.Body.Cities))
	}
	if resp.Body.Cities[0].City != "Phoenix" {
		t.Errorf("expected first city='Phoenix', got %q", resp.Body.Cities[0].City)
	}
	if resp.Body.Cities[0].ArtistCount != 10 {
		t.Errorf("expected first count=10, got %d", resp.Body.Cities[0].ArtistCount)
	}
}

func TestGetArtistCities_ServiceError(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistCitiesFn: func() ([]*contracts.ArtistCityResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.GetArtistCitiesHandler(context.Background(), &GetArtistCitiesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetArtistCities_Empty(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistCitiesFn: func() ([]*contracts.ArtistCityResponse, error) {
			return []*contracts.ArtistCityResponse{}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.GetArtistCitiesHandler(context.Background(), &GetArtistCitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Cities) != 0 {
		t.Errorf("expected 0 cities, got %d", len(resp.Body.Cities))
	}
}

// ============================================================================
// Mock-based tests: ListArtistsHandler with multi-city filter
// ============================================================================

func TestListArtists_WithCitiesFilter(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(filters map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
			cities, ok := filters["cities"].([]map[string]string)
			if !ok {
				t.Error("expected cities filter to be []map[string]string")
			}
			if len(cities) != 2 {
				t.Errorf("expected 2 city filters, got %d", len(cities))
			}
			if cities[0]["city"] != "Phoenix" || cities[0]["state"] != "AZ" {
				t.Errorf("expected first city=Phoenix,AZ, got %v", cities[0])
			}
			return []*contracts.ArtistWithShowCountResponse{
				{ArtistDetailResponse: contracts.ArtistDetailResponse{ID: 1}, UpcomingShowCount: 1},
			}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{Cities: "Phoenix,AZ|Mesa,AZ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Body.Count)
	}
}

func TestListArtists_CitiesOverridesLegacy(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(filters map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
			// When Cities param is set, legacy city/state should not be in filters
			if _, ok := filters["city"]; ok {
				t.Error("legacy city filter should not be set when Cities param is provided")
			}
			if _, ok := filters["state"]; ok {
				t.Error("legacy state filter should not be set when Cities param is provided")
			}
			return []*contracts.ArtistWithShowCountResponse{}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{
		Cities: "Phoenix,AZ",
		City:   "Tempe",
		State:  "AZ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// Mock-based tests: GetArtistAliasesHandler
// ============================================================================

func TestGetArtistAliases_InvalidID(t *testing.T) {
	h := testArtistHandler()
	_, err := h.GetArtistAliasesHandler(context.Background(), &GetArtistAliasesRequest{ArtistID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetArtistAliases_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistAliasesFn: func(artistID uint) ([]*contracts.ArtistAliasResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			return []*contracts.ArtistAliasResponse{
				{ID: 1, ArtistID: 42, Alias: "Alias One"},
				{ID: 2, ArtistID: 42, Alias: "Alias Two"},
			}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	resp, err := h.GetArtistAliasesHandler(context.Background(), &GetArtistAliasesRequest{ArtistID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 2 {
		t.Errorf("expected count=2, got %d", resp.Body.Count)
	}
}

func TestGetArtistAliases_NotFound(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistAliasesFn: func(artistID uint) ([]*contracts.ArtistAliasResponse, error) {
			return nil, apperrors.ErrArtistNotFound(artistID)
		},
	}
	h := NewArtistHandler(mock, nil, nil)

	_, err := h.GetArtistAliasesHandler(context.Background(), &GetArtistAliasesRequest{ArtistID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAddArtistAlias_InvalidID(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AddArtistAliasRequest{ArtistID: "abc"}
	req.Body.Alias = "test"

	_, err := h.AddArtistAliasHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAddArtistAlias_EmptyAlias(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AddArtistAliasRequest{ArtistID: "1"}
	req.Body.Alias = "   "

	_, err := h.AddArtistAliasHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAddArtistAlias_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		AddArtistAliasFn: func(artistID uint, alias string) (*contracts.ArtistAliasResponse, error) {
			if artistID != 42 {
				t.Errorf("expected artistID=42, got %d", artistID)
			}
			return &contracts.ArtistAliasResponse{ID: 1, ArtistID: 42, Alias: alias}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AddArtistAliasRequest{ArtistID: "42"}
	req.Body.Alias = "New Alias"

	resp, err := h.AddArtistAliasHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Alias != "New Alias" {
		t.Errorf("expected alias='New Alias', got %q", resp.Body.Alias)
	}
}

func TestAddArtistAlias_Conflict(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		AddArtistAliasFn: func(artistID uint, alias string) (*contracts.ArtistAliasResponse, error) {
			return nil, fmt.Errorf("alias 'Test' already exists")
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AddArtistAliasRequest{ArtistID: "42"}
	req.Body.Alias = "Test"

	_, err := h.AddArtistAliasHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 409)
}

func TestDeleteArtistAlias_InvalidAliasID(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	_, err := h.DeleteArtistAliasHandler(ctx, &DeleteArtistAliasRequest{ArtistID: "1", AliasID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestDeleteArtistAlias_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		RemoveArtistAliasFn: func(aliasID uint) error {
			if aliasID != 5 {
				t.Errorf("expected aliasID=5, got %d", aliasID)
			}
			return nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	_, err := h.DeleteArtistAliasHandler(ctx, &DeleteArtistAliasRequest{ArtistID: "1", AliasID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteArtistAlias_NotFound(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		RemoveArtistAliasFn: func(aliasID uint) error {
			return fmt.Errorf("alias not found")
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	_, err := h.DeleteArtistAliasHandler(ctx, &DeleteArtistAliasRequest{ArtistID: "1", AliasID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestMergeArtists_MissingIDs(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &MergeArtistsRequest{}
	req.Body.CanonicalArtistID = 1
	req.Body.MergeFromArtistID = 0

	_, err := h.MergeArtistsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestMergeArtists_SelfMerge(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		MergeArtistsFn: func(canonicalID, mergeFromID uint) (*contracts.MergeArtistResult, error) {
			return nil, fmt.Errorf("cannot merge an artist with itself")
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &MergeArtistsRequest{}
	req.Body.CanonicalArtistID = 5
	req.Body.MergeFromArtistID = 5

	_, err := h.MergeArtistsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestMergeArtists_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		MergeArtistsFn: func(canonicalID, mergeFromID uint) (*contracts.MergeArtistResult, error) {
			if canonicalID != 1 {
				t.Errorf("expected canonicalID=1, got %d", canonicalID)
			}
			if mergeFromID != 2 {
				t.Errorf("expected mergeFromID=2, got %d", mergeFromID)
			}
			return &contracts.MergeArtistResult{
				CanonicalArtistID: 1,
				MergedArtistID:    2,
				MergedArtistName:  "Old Name",
				ShowsMoved:        3,
				AliasCreated:      true,
			}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &MergeArtistsRequest{}
	req.Body.CanonicalArtistID = 1
	req.Body.MergeFromArtistID = 2

	resp, err := h.MergeArtistsHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.MergedArtistName != "Old Name" {
		t.Errorf("expected merged name='Old Name', got %q", resp.Body.MergedArtistName)
	}
	if resp.Body.ShowsMoved != 3 {
		t.Errorf("expected shows_moved=3, got %d", resp.Body.ShowsMoved)
	}
}

func TestMergeArtists_NotFound(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		MergeArtistsFn: func(canonicalID, mergeFromID uint) (*contracts.MergeArtistResult, error) {
			return nil, apperrors.ErrArtistNotFound(canonicalID)
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &MergeArtistsRequest{}
	req.Body.CanonicalArtistID = 99
	req.Body.MergeFromArtistID = 2

	_, err := h.MergeArtistsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminCreateArtist_EmptyName(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateArtistRequest{}
	req.Body.Name = "   "

	_, err := h.AdminCreateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminCreateArtist_Success(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
			if req.Name != "New Artist" {
				t.Errorf("expected name='New Artist', got %q", req.Name)
			}
			return &contracts.ArtistDetailResponse{ID: 42, Name: "New Artist", Slug: "new-artist"}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateArtistRequest{}
	req.Body.Name = "New Artist"

	resp, err := h.AdminCreateArtistHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 42 {
		t.Errorf("expected ID=42, got %d", resp.Body.ID)
	}
	if resp.Body.Name != "New Artist" {
		t.Errorf("expected name='New Artist', got %q", resp.Body.Name)
	}
	if resp.Body.Slug != "new-artist" {
		t.Errorf("expected slug='new-artist', got %q", resp.Body.Slug)
	}
}

func TestAdminCreateArtist_WithSocials(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
			if req.Name != "Social Artist" {
				t.Errorf("expected name='Social Artist', got %q", req.Name)
			}
			if req.City == nil || *req.City != "Phoenix" {
				t.Errorf("expected city='Phoenix', got %v", req.City)
			}
			if req.State == nil || *req.State != "AZ" {
				t.Errorf("expected state='AZ', got %v", req.State)
			}
			// PSY-525: social URL fields must be valid http/https URLs (not handles).
			if req.Instagram == nil || *req.Instagram != "https://instagram.com/artist" {
				t.Errorf("expected instagram='https://instagram.com/artist', got %v", req.Instagram)
			}
			if req.Website == nil || *req.Website != "https://example.com" {
				t.Errorf("expected website='https://example.com', got %v", req.Website)
			}
			return &contracts.ArtistDetailResponse{ID: 43, Name: "Social Artist"}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateArtistRequest{}
	req.Body.Name = "Social Artist"
	city := "Phoenix"
	state := "AZ"
	instagram := "https://instagram.com/artist"
	website := "https://example.com"
	req.Body.City = &city
	req.Body.State = &state
	req.Body.Instagram = &instagram
	req.Body.Website = &website

	resp, err := h.AdminCreateArtistHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 43 {
		t.Errorf("expected ID=43, got %d", resp.Body.ID)
	}
}

func TestAdminCreateArtist_Conflict(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("artist with name 'Existing' already exists")
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateArtistRequest{}
	req.Body.Name = "Existing"

	_, err := h.AdminCreateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 409)
}

func TestAdminCreateArtist_ServiceError(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateArtistRequest{}
	req.Body.Name = "Test"

	_, err := h.AdminCreateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestAdminCreateArtist_AuditLogCalled(t *testing.T) {
	var auditCalled bool
	artistMock := &testhelpers.MockArtistService{
		CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
			return &contracts.ArtistDetailResponse{ID: 42, Name: req.Name}, nil
		},
	}
	auditMock := &testhelpers.MockAuditLogService{
		LogActionFn: func(actorID uint, action string, entityType string, entityID uint, metadata map[string]interface{}) {
			auditCalled = true
			if actorID != 1 {
				t.Errorf("expected actorID=1, got %d", actorID)
			}
			if action != "create_artist" {
				t.Errorf("expected action='create_artist', got %q", action)
			}
			if entityType != "artist" {
				t.Errorf("expected entityType='artist', got %q", entityType)
			}
			if entityID != 42 {
				t.Errorf("expected entityID=42, got %d", entityID)
			}
			if name, ok := metadata["name"]; !ok || name != "Audit Test Artist" {
				t.Errorf("expected metadata name='Audit Test Artist', got %v", name)
			}
		},
	}
	h := NewArtistHandler(artistMock, auditMock, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateArtistRequest{}
	req.Body.Name = "Audit Test Artist"

	_, err := h.AdminCreateArtistHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !auditCalled {
		t.Error("expected audit log to be called")
	}
}

// ============================================================================
// ID Parsing Boundary Tests
// ============================================================================

func TestDeleteArtist_ZeroID(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		DeleteArtistFn: func(id uint) error {
			return apperrors.ErrArtistNotFound(id)
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteArtist_OverflowID(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.DeleteArtistHandler(ctx, &DeleteArtistRequest{ArtistID: "99999999999"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminUpdateArtist_ZeroID(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		UpdateArtistFn: func(id uint, updates map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
			return nil, apperrors.ErrArtistNotFound(id)
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "0"}
	name := "Test Name"
	req.Body.Name = &name
	_, err := h.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminUpdateArtist_VeryLargeID(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		UpdateArtistFn: func(id uint, updates map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
			return nil, apperrors.ErrArtistNotFound(id)
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "4294967295"}
	name := "Test Name"
	req.Body.Name = &name
	_, err := h.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminUpdateArtist_OverflowID(t *testing.T) {
	h := testArtistHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminUpdateArtistRequest{ArtistID: "99999999999"}
	name := "Test"
	req.Body.Name = &name
	_, err := h.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminCreateArtist_NameTrimmed(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
			if req.Name != "Trimmed Name" {
				t.Errorf("expected trimmed name='Trimmed Name', got %q", req.Name)
			}
			return &contracts.ArtistDetailResponse{ID: 44, Name: "Trimmed Name"}, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateArtistRequest{}
	req.Body.Name = "  Trimmed Name  "

	resp, err := h.AdminCreateArtistHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Trimmed Name" {
		t.Errorf("expected name='Trimmed Name', got %q", resp.Body.Name)
	}
}
