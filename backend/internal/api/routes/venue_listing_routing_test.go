package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1764. GET /venues/listing exists to be SMALL and COMPLETE — it is the
// slug+name projection the /venues page's JSON-LD ItemList is built from,
// replacing a `GET /venues?limit=100` that advertised a truncated prefix of the
// catalogue and reported nothing about the rest. See contracts.VenueListingEntry
// for the measurements; they are deliberately not repeated here, because a
// catalogue size copied into a test comment is wrong within weeks.
//
// Three properties are invisible to a handler-level test: the route only
// resolves if the STATIC segment beats its `/venues/{venue_id}` sibling in the
// built router, the payload only stays small if the serialised object keeps
// exactly two keys, and the listing is only complete if no limit can be applied.
// These tests speak HTTP so the first two are observable; the third is pinned by
// TestVenueListingTakesNoParameters below, because "returns everything seeded"
// cannot distinguish an absent limit from one larger than the fixture.

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

// The completeness guarantee, pinned where it can actually fail.
//
// "The ItemList covers every venue" survives only while no limit can be applied,
// and a seeded end-to-end assertion cannot see the difference: adding
// `Limit int \`query:"limit" default:"100"\`` to the request would still return
// every venue in any fixture small enough to write down, and the frontend test
// only inspects the URL WE send, so a server-side default is invisible there
// too. Every signal would stay green while the defect came back.
//
// The request type having no fields is the property that forecloses it, so that
// is what this asserts. Any query parameter added here — limit, offset, a filter
// — has to come with a consumer and a deliberate edit to this test.
func TestVenueListingTakesNoParameters(t *testing.T) {
	if n := reflect.TypeOf(catalogh.ListVenueListingRequest{}).NumField(); n != 0 {
		t.Errorf("ListVenueListingRequest has %d field(s), want 0 — this endpoint answers "+
			"with the whole browse set, and a query parameter is how the truncation "+
			"PSY-1764 removed would come back", n)
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

	// The gate is `verified`, NOT activity — so this quiet venue is listed at
	// all, which is the venue-specific half of the contract the artist twin
	// cannot cover. It is also named to sort FIRST, ahead of the venue that has
	// a show, so the alphabetical order is distinguishable from the browse
	// page's activity order rather than agreeing with it by accident.
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

	// An uncached hit scans every verified venue and the body is byte-identical
	// for every caller, so any intermediary willing to hold it is worth having.
	// Same reasoning and same window as GET /sitemap/entries.
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Errorf("Cache-Control = %q, want %q", cc, "public, max-age=300")
	}

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
	// visible rather than silently absorbed, which is the whole defect. Total
	// comes from a window count in the SAME statement as the rows, so this also
	// pins that it describes the gated set rather than the returned array.
	if body.Total != 3 {
		t.Errorf("total = %d, want 3 — total is the browse set BEFORE the slug filter, so the "+
			"caller can tell a complete listing from a short one", body.Total)
	}

	// Ordered by name, deliberately NOT by the browse page's activity sort: the
	// consumer stamps ItemList `position` from this order, and an activity sort
	// renumbers every entry whenever a show is booked. Zebra Hall has the only
	// upcoming show, so under the browse order it would come first.
	var firstSlug string
	if err := json.Unmarshal(body.Venues[0]["slug"], &firstSlug); err != nil {
		t.Fatalf("decode slug: %v", err)
	}
	if firstSlug != "aardvark-room" {
		t.Errorf("first entry is %q, want %q — the listing is ordered by name so that "+
			"ItemList positions are stable against bookings", firstSlug, "aardvark-room")
	}

	// THE assertion this endpoint exists for. A widened projection is exactly
	// how the artist payload grew past the cache cap, and it would otherwise
	// regress in total silence. It also pins that the window count selected
	// alongside the projection stays off the wire.
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
