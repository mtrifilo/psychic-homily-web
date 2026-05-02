//go:build openapi_snapshot

package catalog

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
)

// TestFestivalIntelligenceOpenAPISnapshot writes the OpenAPI spec for the
// festival intelligence handlers to the file given by env var
// FESTIVAL_INTELLIGENCE_OPENAPI_OUT. Used by PSY-426 to diff before/after the
// shared.BodyResponse[T] refactor.
//
// Run with:
//
//	FESTIVAL_INTELLIGENCE_OPENAPI_OUT=/tmp/before.json \
//	  go test -tags openapi_snapshot \
//	    -run TestFestivalIntelligenceOpenAPISnapshot \
//	    ./internal/api/handlers/catalog/
func TestFestivalIntelligenceOpenAPISnapshot(t *testing.T) {
	out := os.Getenv("FESTIVAL_INTELLIGENCE_OPENAPI_OUT")
	if out == "" {
		t.Skip("set FESTIVAL_INTELLIGENCE_OPENAPI_OUT to write snapshot")
	}

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Psychic Homily", "1.0.0"))

	h := NewFestivalIntelligenceHandler(
		&testhelpers.MockFestivalIntelligenceService{},
		&testhelpers.MockFestivalService{},
		&testhelpers.MockArtistService{},
	)
	huma.Get(api, "/festivals/{festival_id}/similar", h.GetSimilarFestivalsHandler)
	huma.Get(api, "/festivals/{festival_a_id}/overlap/{festival_b_id}", h.GetFestivalOverlapHandler)
	huma.Get(api, "/festivals/{festival_id}/breakouts", h.GetFestivalBreakoutsHandler)
	huma.Get(api, "/artists/{artist_id}/festival-trajectory", h.GetArtistFestivalTrajectoryHandler)
	huma.Get(api, "/festivals/series/{series_slug}/compare", h.GetSeriesComparisonHandler)

	spec := api.OpenAPI()
	b, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("wrote %d bytes to %s", len(b), out)
}
