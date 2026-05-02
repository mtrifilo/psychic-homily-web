package admin

import (
	"context"
	"fmt"
	"testing"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
)

// ============================================================================
// Test helpers
// ============================================================================

func testDataGapsHandler() *DataGapsHandler {
	return NewDataGapsHandler(&testhelpers.MockArtistService{}, &testhelpers.MockVenueService{}, &testhelpers.MockFestivalService{}, &testhelpers.MockReleaseService{}, &testhelpers.MockLabelService{})
}

func dataGapsCtxWithUser() context.Context {
	username := "testuser"
	return context.WithValue(context.Background(), middleware.UserContextKey, &models.User{ID: 1, Username: &username})
}

// ============================================================================
// Tests: Artist data gaps
// ============================================================================

func TestDataGapsHandler_Artist_WithMissingFields(t *testing.T) {
	h := NewDataGapsHandler(
		&testhelpers.MockArtistService{
			GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
				return &contracts.ArtistDetailResponse{
					ID:   1,
					Slug: "test-artist",
					Name: "Test Artist",
					// All optional fields nil => all gaps returned
					Social: contracts.SocialResponse{},
				}, nil
			},
		},
		&testhelpers.MockVenueService{},
		&testhelpers.MockFestivalService{},
		&testhelpers.MockReleaseService{},
		&testhelpers.MockLabelService{},
	)

	resp, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: "artist",
		IDOrSlug:   "test-artist",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Body.Gaps) != 7 {
		t.Fatalf("expected 7 gaps for artist with all fields missing, got %d", len(resp.Body.Gaps))
	}

	// Verify priority ordering
	for i := 1; i < len(resp.Body.Gaps); i++ {
		if resp.Body.Gaps[i].Priority < resp.Body.Gaps[i-1].Priority {
			t.Errorf("gaps not sorted by priority: gap[%d].Priority=%d < gap[%d].Priority=%d",
				i, resp.Body.Gaps[i].Priority, i-1, resp.Body.Gaps[i-1].Priority)
		}
	}

	// First gap should be bandcamp (highest priority)
	if resp.Body.Gaps[0].Field != "bandcamp" {
		t.Errorf("expected first gap field=bandcamp, got %s", resp.Body.Gaps[0].Field)
	}
}

func TestDataGapsHandler_Artist_Complete(t *testing.T) {
	city := "Phoenix"
	state := "AZ"
	desc := "A great band"
	h := NewDataGapsHandler(
		&testhelpers.MockArtistService{
			GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
				return &contracts.ArtistDetailResponse{
					ID:          1,
					Slug:        "complete-artist",
					Name:        "Complete Artist",
					City:        &city,
					State:       &state,
					Description: &desc,
					Social: contracts.SocialResponse{
						Bandcamp:  shared.PtrString("https://band.bandcamp.com"),
						Spotify:   shared.PtrString("https://open.spotify.com/artist/123"),
						Website:   shared.PtrString("https://band.com"),
						Instagram: shared.PtrString("https://instagram.com/band"),
					},
				}, nil
			},
		},
		&testhelpers.MockVenueService{},
		&testhelpers.MockFestivalService{},
		&testhelpers.MockReleaseService{},
		&testhelpers.MockLabelService{},
	)

	resp, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: "artist",
		IDOrSlug:   "complete-artist",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Body.Gaps) != 0 {
		t.Errorf("expected 0 gaps for complete artist, got %d: %+v", len(resp.Body.Gaps), resp.Body.Gaps)
	}
}

// ============================================================================
// Tests: Venue data gaps
// ============================================================================

func TestDataGapsHandler_Venue_WithMissingFields(t *testing.T) {
	h := NewDataGapsHandler(
		&testhelpers.MockArtistService{},
		&testhelpers.MockVenueService{
			GetVenueBySlugFn: func(slug string) (*contracts.VenueDetailResponse, error) {
				return &contracts.VenueDetailResponse{
					ID:   1,
					Slug: "test-venue",
					Name: "Test Venue",
					City: "Phoenix",
					// Description nil, social fields nil
					Social: contracts.SocialResponse{},
				}, nil
			},
		},
		&testhelpers.MockFestivalService{},
		&testhelpers.MockReleaseService{},
		&testhelpers.MockLabelService{},
	)

	resp, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: "venue",
		IDOrSlug:   "test-venue",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Body.Gaps) != 3 {
		t.Fatalf("expected 3 gaps for venue with all optional fields missing, got %d", len(resp.Body.Gaps))
	}

	// First should be website (priority 1)
	if resp.Body.Gaps[0].Field != "website" {
		t.Errorf("expected first gap field=website, got %s", resp.Body.Gaps[0].Field)
	}
}

// ============================================================================
// Tests: Festival data gaps
// ============================================================================

func TestDataGapsHandler_Festival_WithMissingFields(t *testing.T) {
	h := NewDataGapsHandler(
		&testhelpers.MockArtistService{},
		&testhelpers.MockVenueService{},
		&testhelpers.MockFestivalService{
			GetFestivalBySlugFn: func(slug string) (*contracts.FestivalDetailResponse, error) {
				return &contracts.FestivalDetailResponse{
					ID:   1,
					Slug: "test-fest",
					Name: "Test Fest",
					// Website, FlyerURL, Description all nil
				}, nil
			},
		},
		&testhelpers.MockReleaseService{},
		&testhelpers.MockLabelService{},
	)

	resp, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: "festival",
		IDOrSlug:   "test-fest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Body.Gaps) != 3 {
		t.Fatalf("expected 3 gaps for festival with all optional fields missing, got %d", len(resp.Body.Gaps))
	}

	if resp.Body.Gaps[0].Field != "website" {
		t.Errorf("expected first gap field=website, got %s", resp.Body.Gaps[0].Field)
	}
	if resp.Body.Gaps[1].Field != "flyer_url" {
		t.Errorf("expected second gap field=flyer_url, got %s", resp.Body.Gaps[1].Field)
	}
}

// ============================================================================
// Tests: Error handling
// ============================================================================

func TestDataGapsHandler_NotFound(t *testing.T) {
	h := NewDataGapsHandler(
		&testhelpers.MockArtistService{
			GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
				return nil, apperrors.ErrArtistNotFound(0)
			},
		},
		&testhelpers.MockVenueService{
			GetVenueBySlugFn: func(slug string) (*contracts.VenueDetailResponse, error) {
				return nil, apperrors.ErrVenueNotFound(0)
			},
		},
		&testhelpers.MockFestivalService{
			GetFestivalBySlugFn: func(slug string) (*contracts.FestivalDetailResponse, error) {
				return nil, apperrors.ErrFestivalNotFound(0)
			},
		},
		&testhelpers.MockReleaseService{},
		&testhelpers.MockLabelService{},
	)

	tests := []struct {
		name       string
		entityType string
	}{
		{"artist not found", "artist"},
		{"venue not found", "venue"},
		{"festival not found", "festival"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
				EntityType: tt.entityType,
				IDOrSlug:   "nonexistent",
			})
			testhelpers.AssertHumaError(t, err, 404)
		})
	}
}

func TestDataGapsHandler_InvalidEntityType(t *testing.T) {
	h := testDataGapsHandler()

	_, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: "invalid",
		IDOrSlug:   "something",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestDataGapsHandler_Unauthenticated(t *testing.T) {
	h := testDataGapsHandler()

	// No user in context
	_, err := h.GetDataGapsHandler(context.Background(), &GetDataGapsRequest{
		EntityType: "artist",
		IDOrSlug:   "test",
	})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestDataGapsHandler_ServiceError(t *testing.T) {
	h := NewDataGapsHandler(
		&testhelpers.MockArtistService{
			GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
				return nil, fmt.Errorf("database error")
			},
		},
		&testhelpers.MockVenueService{},
		&testhelpers.MockFestivalService{},
		&testhelpers.MockReleaseService{},
		&testhelpers.MockLabelService{},
	)

	_, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: "artist",
		IDOrSlug:   "test",
	})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestDataGapsHandler_NumericID(t *testing.T) {
	h := NewDataGapsHandler(
		&testhelpers.MockArtistService{
			GetArtistFn: func(id uint) (*contracts.ArtistDetailResponse, error) {
				if id != 42 {
					t.Errorf("expected artist ID 42, got %d", id)
				}
				return &contracts.ArtistDetailResponse{
					ID:     42,
					Slug:   "some-artist",
					Name:   "Some Artist",
					Social: contracts.SocialResponse{},
				}, nil
			},
		},
		&testhelpers.MockVenueService{},
		&testhelpers.MockFestivalService{},
		&testhelpers.MockReleaseService{},
		&testhelpers.MockLabelService{},
	)

	resp, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: "artist",
		IDOrSlug:   "42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestDataGapsHandler_EmptyStringNotAGap(t *testing.T) {
	// An empty string should count as a gap (same as nil)
	emptyStr := ""
	h := NewDataGapsHandler(
		&testhelpers.MockArtistService{
			GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
				return &contracts.ArtistDetailResponse{
					ID:   1,
					Slug: "test",
					Name: "Test",
					Social: contracts.SocialResponse{
						Bandcamp: &emptyStr, // empty string = still a gap
					},
				}, nil
			},
		},
		&testhelpers.MockVenueService{},
		&testhelpers.MockFestivalService{},
		&testhelpers.MockReleaseService{},
		&testhelpers.MockLabelService{},
	)

	resp, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: "artist",
		IDOrSlug:   "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Bandcamp should still be a gap (empty string)
	hasBandcamp := false
	for _, gap := range resp.Body.Gaps {
		if gap.Field == "bandcamp" {
			hasBandcamp = true
			break
		}
	}
	if !hasBandcamp {
		t.Error("expected bandcamp to be a gap when field is empty string")
	}
}
