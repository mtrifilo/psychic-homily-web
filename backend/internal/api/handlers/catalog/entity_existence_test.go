package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
)

// mockEntityExistenceService answers every probe with one verdict and records
// the arguments of the last call, so a test can see which identifiers a handler
// passed through.
type mockEntityExistenceService struct {
	exists    bool
	err       error
	args      *[]string
	canonical string
}

func (m mockEntityExistenceService) answer(args ...string) (bool, error) {
	if m.args != nil {
		*m.args = args
	}
	return m.exists, m.err
}

func (m mockEntityExistenceService) Exists(entityType, idOrSlug string) (bool, error) {
	return m.answer(entityType, idOrSlug)
}

func (m mockEntityExistenceService) ServedSceneSlug(slug string) (string, bool, error) {
	exists, err := m.answer("scenes", slug)
	if err != nil || !exists {
		return "", exists, err
	}
	return m.canonical, true, nil
}

func (m mockEntityExistenceService) RadioStationExists(stationIDOrSlug string) (bool, error) {
	return m.answer(stationIDOrSlug)
}

func (m mockEntityExistenceService) RadioShowExists(stationIDOrSlug, showIDOrSlug string) (bool, error) {
	return m.answer(stationIDOrSlug, showIDOrSlug)
}

func (m mockEntityExistenceService) RadioEpisodeExists(stationIDOrSlug, showIDOrSlug, airDate string) (bool, error) {
	return m.answer(stationIDOrSlug, showIDOrSlug, airDate)
}

func (m mockEntityExistenceService) CollectionExists(collectionIDOrSlug string) (bool, error) {
	return m.answer(collectionIDOrSlug)
}

func TestEntityExistsHandler_Success(t *testing.T) {
	h := NewEntityExistenceHandler(mockEntityExistenceService{exists: true})

	resp, err := h.EntityExistsHandler(context.Background(), &EntityExistsRequest{
		EntityType: "shows",
		EntityID:   "approved-show",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
}

func TestEntityExistsHandler_NotFound(t *testing.T) {
	h := NewEntityExistenceHandler(mockEntityExistenceService{exists: false})

	_, err := h.EntityExistsHandler(context.Background(), &EntityExistsRequest{
		EntityType: "tags",
		EntityID:   "missing-tag",
	})

	testhelpers.AssertHumaError(t, err, 404)
}

func TestEntityExistsHandler_ServiceError(t *testing.T) {
	h := NewEntityExistenceHandler(mockEntityExistenceService{err: fmt.Errorf("database down")})

	_, err := h.EntityExistsHandler(context.Background(), &EntityExistsRequest{
		EntityType: "scenes",
		EntityID:   "phoenix-az",
	})

	testhelpers.AssertHumaError(t, err, 500)
}

// A scene probe names the slug the scene is served at in the Canonical-Slug
// header, and every other entity type sends no such header.
func TestEntityExistsHandler_ServedSceneSlug(t *testing.T) {
	var args []string
	h := NewEntityExistenceHandler(mockEntityExistenceService{exists: true, args: &args, canonical: "phoenix-az"})

	resp, err := h.EntityExistsHandler(context.Background(), &EntityExistsRequest{EntityType: "scenes", EntityID: "tempe-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CanonicalSlug != "phoenix-az" {
		t.Errorf("CanonicalSlug = %q, want phoenix-az", resp.CanonicalSlug)
	}
	if fmt.Sprint(args) != fmt.Sprint([]string{"scenes", "tempe-az"}) {
		t.Errorf("service got %v, want [scenes tempe-az]", args)
	}

	resp, err = h.EntityExistsHandler(context.Background(), &EntityExistsRequest{EntityType: "artists", EntityID: "tempe-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CanonicalSlug != "" {
		t.Errorf("artist probe CanonicalSlug = %q, want none", resp.CanonicalSlug)
	}
}

func TestEntityExistsHandler_SceneMissing(t *testing.T) {
	h := NewEntityExistenceHandler(mockEntityExistenceService{exists: false, canonical: "phoenix-az"})

	_, err := h.EntityExistsHandler(context.Background(), &EntityExistsRequest{EntityType: "scenes", EntityID: "nowhere-zz"})

	testhelpers.AssertHumaError(t, err, 404)
}

// Each family probe passes its path parameters through in order and maps the
// verdict the way the generic probe does: 200 empty, 404, or 500.
func TestFamilyExistsHandlers(t *testing.T) {
	probes := []struct {
		name     string
		call     func(h *EntityExistenceHandler) (*FamilyExistsResponse, error)
		wantArgs []string
	}{
		{
			name: "radio station",
			call: func(h *EntityExistenceHandler) (*FamilyExistsResponse, error) {
				return h.RadioStationExistsHandler(context.Background(), &RadioStationExistsRequest{Slug: "kexp"})
			},
			wantArgs: []string{"kexp"},
		},
		{
			name: "radio show",
			call: func(h *EntityExistenceHandler) (*FamilyExistsResponse, error) {
				return h.RadioShowExistsHandler(context.Background(), &RadioShowExistsRequest{
					Slug: "kexp", ShowSlug: "the-morning-show",
				})
			},
			wantArgs: []string{"kexp", "the-morning-show"},
		},
		{
			name: "radio episode",
			call: func(h *EntityExistenceHandler) (*FamilyExistsResponse, error) {
				return h.RadioEpisodeExistsHandler(context.Background(), &RadioEpisodeExistsRequest{
					Slug: "kexp", ShowSlug: "the-morning-show", Date: "2025-01-15",
				})
			},
			wantArgs: []string{"kexp", "the-morning-show", "2025-01-15"},
		},
		{
			name: "collection",
			call: func(h *EntityExistenceHandler) (*FamilyExistsResponse, error) {
				return h.CollectionExistsHandler(context.Background(), &CollectionExistsRequest{Slug: "my-list"})
			},
			wantArgs: []string{"my-list"},
		},
	}

	for _, probe := range probes {
		t.Run(probe.name+" exists", func(t *testing.T) {
			var args []string
			resp, err := probe.call(NewEntityExistenceHandler(mockEntityExistenceService{exists: true, args: &args}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil {
				t.Fatal("expected response, got nil")
			}
			if fmt.Sprint(args) != fmt.Sprint(probe.wantArgs) {
				t.Errorf("service got %v, want %v", args, probe.wantArgs)
			}
		})
		t.Run(probe.name+" missing", func(t *testing.T) {
			_, err := probe.call(NewEntityExistenceHandler(mockEntityExistenceService{exists: false}))
			testhelpers.AssertHumaError(t, err, 404)
		})
		t.Run(probe.name+" service error", func(t *testing.T) {
			_, err := probe.call(NewEntityExistenceHandler(mockEntityExistenceService{err: fmt.Errorf("database down")}))
			testhelpers.AssertHumaError(t, err, 500)
		})
	}
}
