package handlers

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/models"
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
