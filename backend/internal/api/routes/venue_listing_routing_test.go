package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1764. GET /venues/listing exists to be SMALL and COMPLETE — it is the
// slug+name projection the /venues page's JSON-LD ItemList is built from,
// replacing `GET /venues?limit=100`, which advertised 100 of 297 venues and
// reported nothing about the other 197. See contracts.VenueListingEntry.
//
// Three properties are invisible to a handler-level test: the route only
// resolves if the STATIC segment beats its `/venues/{venue_id}` sibling in the
// built router, the payload only stays small if the serialised object keeps
// exactly two keys, and the listing is only complete if it carries no limit.
// These tests speak HTTP so all three are observable.

// This asserts RESOLUTION, not registration — see the artist twin in
// artist_listing_routing_test.go for why that distinction is load-bearing. A
// `/venues/{venue_id}` sibling answering here would not 404: the frontend fails
// open on a non-OK response, so it would surface as an ItemList that is quietly
// empty, which is the same silence this ticket exists to remove. No database is
// needed, so unlike the end-to-end test below this runs in short mode.
func TestVenueListingRouteResolvesToTheListingHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/venues/listing", nil)
	w := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(w, req)

	if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
		t.Fatalf("GET /venues/listing = %d — the route is not registered", w.Code)
	}

	// This router has no database, so the listing handler answers 500 with its
	// OWN message. That message is what identifies the handler: the
	// /venues/{venue_id} sibling cannot produce it, so seeing it proves the
	// static segment won.
	const listingHandlerDetail = "Failed to fetch venue listing"

	var body struct {
		Detail string             `json:"detail"`
		Venues *[]json.RawMessage `json:"venues"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v; body: %s", err, w.Body.String())
	}

	if body.Venues == nil && body.Detail != listingHandlerDetail {
		t.Errorf("GET /venues/listing did not reach the listing handler.\n"+
			"got status %d body %s\n"+
			"the parameterised /venues/{venue_id} sibling answered instead",
			w.Code, w.Body.String())
	}
}

func TestVenueListingEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	router := chi.NewRouter()
	SetupRoutes(router, services.NewServiceContainer(td.DB, cfg), cfg)

	get := func(t *testing.T) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/venues/listing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /venues/listing = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		return w
	}

	// Runs before anything is seeded, sharing this test's container rather than
	// standing up a second one. An empty catalogue must serialise as [], never
	// null: the frontend treats a missing array as a contract break and reports
	// it, so null would raise a false alarm on a legitimately empty state.
	t.Run("empty catalogue serialises as an array", func(t *testing.T) {
		var body struct {
			Venues *[]json.RawMessage `json:"venues"`
		}
		w := get(t)
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("parse response: %v; body: %s", err, w.Body.String())
		}
		if body.Venues == nil {
			t.Fatalf("venues serialised as null, want []; body: %s", w.Body.String())
		}
		if len(*body.Venues) != 0 {
			t.Fatalf("got %d venues on an empty catalogue, want 0", len(*body.Venues))
		}
	})

	seedVenue := func(name, slug string, verified bool) *catalogm.Venue {
		v := &catalogm.Venue{Name: name, City: "Phoenix", State: "AZ", Verified: verified}
		if slug != "" {
			s := slug
			v.Slug = &s
		}
		if err := td.DB.Create(v).Error; err != nil {
			t.Fatalf("seed venue %q: %v", name, err)
		}
		return v
	}
	seedShow := func(slug string, when time.Time) *catalogm.Show {
		s := slug
		show := &catalogm.Show{Title: slug, Slug: &s, EventDate: when, Status: catalogm.ShowStatusApproved}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show %q: %v", slug, err)
		}
		return show
	}
	book := func(show *catalogm.Show, venue *catalogm.Venue) {
		if err := td.DB.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error; err != nil {
			t.Fatalf("seed show_venue: %v", err)
		}
	}

	future := time.Now().Add(48 * time.Hour)

	// The gate is `verified`, NOT activity — so this quiet venue is listed, and
	// it sorts after the busy one. That is the venue-specific half of the
	// contract and the artist twin cannot cover it.
	seedVenue("Aardvark Room", "aardvark-room", true)
	busy := seedVenue("Zebra Hall", "zebra-hall", true)
	book(seedShow("upcoming-approved", future), busy)

	// Gated out: unverified venues are not public.
	seedVenue("Unverified Room", "unverified-room", false)

	// Dropped from the array but still counted in total: no slug means no URL,
	// so the entry is unusable to any consumer — and the gap between count and
	// total is exactly the signal the caller reports on.
	seedVenue("Slugless Room", "", true)

	w := get(t)

	var body struct {
		// Raw, so the key set of each entry can be asserted rather than a
		// decode silently discarding whatever else was serialised.
		Venues []map[string]json.RawMessage `json:"venues"`
		Count  int                          `json:"count"`
		Total  int64                        `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v; body: %s", err, w.Body.String())
	}

	if len(body.Venues) != 2 {
		t.Fatalf("got %d venues, want 2 (both slugged verified venues, active or not); body: %s",
			len(body.Venues), w.Body.String())
	}
	if body.Count != len(body.Venues) {
		t.Errorf("count = %d, want %d — count must describe the array beside it", body.Count, len(body.Venues))
	}
	// Three verified venues exist; one has no slug. The shortfall must be
	// visible rather than silently absorbed, which is the whole defect.
	if body.Total != 3 {
		t.Errorf("total = %d, want 3 — total is the browse set BEFORE the slug filter, so the "+
			"caller can tell a complete listing from a short one", body.Total)
	}

	// The order is the browse page's own: most upcoming shows first, ties by
	// name. Zebra Hall has a show and Aardvark Room does not, so a listing
	// sorted by name alone would put them the other way round.
	var firstSlug string
	if err := json.Unmarshal(body.Venues[0]["slug"], &firstSlug); err != nil {
		t.Fatalf("decode slug: %v", err)
	}
	if firstSlug != "zebra-hall" {
		t.Errorf("first entry is %q, want %q — the listing must share the browse page's "+
			"upcoming-count DESC, name ASC order", firstSlug, "zebra-hall")
	}

	// THE assertion this endpoint exists for. A widened projection is exactly
	// how the artist payload grew past the cache cap, and it would otherwise
	// regress in total silence. It also pins that the upcoming_show_count
	// selected for the ORDER BY stays off the wire.
	for _, entry := range body.Venues {
		if len(entry) != 2 {
			t.Errorf("entry has %d fields %v, want exactly 2 (slug, name) — every added field is "+
				"multiplied by the whole catalogue and eats the cache-item budget", len(entry), keysOf(entry))
		}
		for _, want := range []string{"slug", "name"} {
			if _, ok := entry[want]; !ok {
				t.Errorf("entry is missing %q; got %v", want, keysOf(entry))
			}
		}
	}
}
