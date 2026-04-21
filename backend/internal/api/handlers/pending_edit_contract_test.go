package handlers

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// TestPendingEditContractDrift asserts every entity type in
// models.ValidPendingEditEntityTypes() has the handler-side wiring required
// for community edits. PSY-492 added 'release'; PSY-504 will add 'label'.
// Adding a new type to the allowlist without the rest of this wiring will
// fail this test, which is the intended pressure.
//
// Coverage:
//   - allowedEditFields has a non-empty entry for the type
//   - *PendingEditHandler exposes Suggest{Type}EditHandler
//   - DataGapsHandler.GetDataGapsHandler does not return 400 for the type
//
// Not covered here (needs DB integration): resolveEntityInfo name/URL lookup
// and RecordRevision wiring from admin update. Those live in service-level
// integration suites.
func TestPendingEditContractDrift(t *testing.T) {
	for _, entityType := range models.ValidPendingEditEntityTypes() {
		entityType := entityType
		t.Run(entityType, func(t *testing.T) {
			assertAllowedEditFields(t, entityType)
			assertSuggestHandlerMethod(t, entityType)
			assertDataGapsCase(t, entityType)
		})
	}
}

func assertAllowedEditFields(t *testing.T, entityType string) {
	t.Helper()
	fields, ok := allowedEditFields[entityType]
	if !ok {
		t.Fatalf("allowedEditFields missing entry for %q", entityType)
	}
	if len(fields) == 0 {
		t.Fatalf("allowedEditFields[%q] is empty", entityType)
	}
}

func assertSuggestHandlerMethod(t *testing.T, entityType string) {
	t.Helper()
	methodName := "Suggest" + capitalize(entityType) + "EditHandler"
	if _, ok := reflect.TypeOf(&PendingEditHandler{}).MethodByName(methodName); !ok {
		t.Fatalf("*PendingEditHandler missing method %s — register it alongside SuggestArtistEditHandler", methodName)
	}
}

// assertDataGapsCase verifies GetDataGapsHandler routes this entity type to a
// handler branch instead of rejecting it as "Invalid entity type".
func assertDataGapsCase(t *testing.T, entityType string) {
	t.Helper()
	h := NewDataGapsHandler(
		&mockArtistService{
			getArtistBySlugFn: func(string) (*contracts.ArtistDetailResponse, error) {
				return &contracts.ArtistDetailResponse{ID: 1, Name: "n", Slug: "s"}, nil
			},
		},
		&mockVenueService{
			getVenueBySlugFn: func(string) (*contracts.VenueDetailResponse, error) {
				return &contracts.VenueDetailResponse{ID: 1, Name: "n", Slug: "s"}, nil
			},
		},
		&mockFestivalService{
			getFestivalBySlugFn: func(string) (*contracts.FestivalDetailResponse, error) {
				return &contracts.FestivalDetailResponse{ID: 1, Name: "n", Slug: "s"}, nil
			},
		},
		&mockReleaseService{
			getReleaseBySlugFn: func(string) (*contracts.ReleaseDetailResponse, error) {
				return &contracts.ReleaseDetailResponse{ID: 1, Title: "t", Slug: "s"}, nil
			},
		},
	)

	_, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), &GetDataGapsRequest{
		EntityType: entityType,
		IDOrSlug:   "probe",
	})

	if err == nil {
		return
	}
	var huErr *huma.ErrorModel
	if errors.As(err, &huErr) && huErr.Status == 400 {
		t.Fatalf("GetDataGapsHandler returned 400 for entity_type=%q — add a case in the switch", entityType)
	}
}

// capitalize converts "artist" -> "Artist". Entity type strings are all
// lowercase ASCII, so a single-byte uppercase is sufficient.
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
