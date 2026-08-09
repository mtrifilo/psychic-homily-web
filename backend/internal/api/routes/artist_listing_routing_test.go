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

// PSY-1674. GET /artists/listing exists to be SMALL — it is the slug+name
// projection the /artists page caches instead of the sixteen-field list, which
// had grown past Vercel's 2 MB cache item cap and so was silently re-pulled
// from origin on every render. See contracts.ArtistListingEntry.
//
// Both properties this endpoint sells are invisible to a handler-level test:
// the route only resolves if the STATIC segment beats its `/artists/{artist_id}`
// sibling in the built router, and the payload only stays small if the
// serialised object keeps exactly two keys. These tests speak HTTP so both are
// observable.

// This asserts RESOLUTION, not registration, and the distinction matters.
//
// An earlier version of this test compared route-table SHAPES via matching(),
// which normalises {param} to {} — so `/artists/listing` could only ever match
// itself and `/artists/{artist_id}` was invisible to it. That assertion reduced
// to "someone registered the route" and would have passed unchanged in the world
// it claimed to guard against. It also repeated the folklore in artists.go that
// registration ORDER decides precedence; it does not — chi walks node types with
// static first, so a static segment wins regardless of order.
//
// What is actually worth pinning is the end state: a GET to /artists/listing
// reaches the listing handler rather than GetArtistHandler with
// artist_id="listing". That regression would not 404 — the frontend fails open
// on a non-OK response, so it would surface as an ItemList that is quietly
// empty, which is the same silence this ticket exists to remove. No database is
// needed, so unlike the end-to-end test below this runs in short mode.
func TestArtistListingRouteResolvesToTheListingHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/artists/listing", nil)
	w := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(w, req)

	if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
		t.Fatalf("GET /artists/listing = %d — the route is not registered", w.Code)
	}

	// This router has no database, so the listing handler answers 500 with its
	// OWN message. That message is what identifies the handler: the
	// /artists/{artist_id} sibling cannot produce it, so seeing it proves the
	// static segment won. Asserting the identity rather than a 200 keeps this
	// test free of a Postgres container, so it runs in short mode — the
	// end-to-end behaviour is covered below.
	const listingHandlerDetail = "Failed to fetch artist listing"

	var body struct {
		Detail  string             `json:"detail"`
		Artists *[]json.RawMessage `json:"artists"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v; body: %s", err, w.Body.String())
	}

	if body.Artists == nil && body.Detail != listingHandlerDetail {
		t.Errorf("GET /artists/listing did not reach the listing handler.\n"+
			"got status %d body %s\n"+
			"the parameterised /artists/{artist_id} sibling answered instead",
			w.Code, w.Body.String())
	}
}

func TestArtistListingEndToEnd(t *testing.T) {
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
		req := httptest.NewRequest(http.MethodGet, "/artists/listing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /artists/listing = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		return w
	}

	// Runs before anything is seeded, sharing this test's container rather than
	// standing up a second one. An empty catalogue must serialise as [], never
	// null: the frontend treats a missing array as a contract break and reports
	// it, so null would raise a false alarm on a legitimately empty state.
	t.Run("empty catalogue serialises as an array", func(t *testing.T) {
		var body struct {
			Artists *[]json.RawMessage `json:"artists"`
		}
		w := get(t)
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("parse response: %v; body: %s", err, w.Body.String())
		}
		if body.Artists == nil {
			t.Fatalf("artists serialised as null, want []; body: %s", w.Body.String())
		}
		if len(*body.Artists) != 0 {
			t.Fatalf("got %d artists on an empty catalogue, want 0", len(*body.Artists))
		}
	})

	// One artist per case the projection has to get right.
	seedArtist := func(name, slug string) *catalogm.Artist {
		a := &catalogm.Artist{Name: name}
		if slug != "" {
			s := slug
			a.Slug = &s
		}
		if err := td.DB.Create(a).Error; err != nil {
			t.Fatalf("seed artist %q: %v", name, err)
		}
		return a
	}
	seedShow := func(slug string, when time.Time, status catalogm.ShowStatus) *catalogm.Show {
		s := slug
		show := &catalogm.Show{Title: slug, Slug: &s, EventDate: when, Status: status}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show %q: %v", slug, err)
		}
		return show
	}
	bill := func(show *catalogm.Show, artist *catalogm.Artist) {
		if err := td.DB.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artist.ID}).Error; err != nil {
			t.Fatalf("seed show_artist: %v", err)
		}
	}

	future := time.Now().Add(48 * time.Hour)
	past := time.Now().Add(-48 * time.Hour)

	listed := seedArtist("Listed Artist", "listed-artist")
	bill(seedShow("upcoming-approved", future, catalogm.ShowStatusApproved), listed)

	// Gated out: a past show is not an upcoming one.
	pastOnly := seedArtist("Past Only Artist", "past-only-artist")
	bill(seedShow("past-approved", past, catalogm.ShowStatusApproved), pastOnly)

	// Gated out: an unapproved upcoming show does not count either.
	pendingOnly := seedArtist("Pending Only Artist", "pending-only-artist")
	bill(seedShow("upcoming-pending", future, catalogm.ShowStatusPending), pendingOnly)

	// Dropped: no slug means no URL, so the entry is unusable to any consumer.
	slugless := seedArtist("Slugless Artist", "")
	bill(seedShow("upcoming-approved-slugless", future, catalogm.ShowStatusApproved), slugless)

	w := get(t)

	var body struct {
		// Raw, so the key set of each entry can be asserted rather than a
		// decode silently discarding whatever else was serialised.
		Artists []map[string]json.RawMessage `json:"artists"`
		Count   int                          `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v; body: %s", err, w.Body.String())
	}

	if len(body.Artists) != 1 {
		t.Fatalf("got %d artists, want 1 (only the slugged artist with an upcoming approved show); body: %s",
			len(body.Artists), w.Body.String())
	}
	if body.Count != len(body.Artists) {
		t.Errorf("count = %d, want %d — count must describe the array beside it", body.Count, len(body.Artists))
	}

	// THE assertion this endpoint exists for. A widened projection is exactly
	// how the payload grew past the cache cap the first time, and it would
	// otherwise regress in total silence. It also pins that the
	// upcoming_show_count selected for the ORDER BY stays off the wire.
	entry := body.Artists[0]
	if len(entry) != 2 {
		t.Errorf("entry has %d fields %v, want exactly 2 (slug, name) — every added field is "+
			"multiplied by the whole catalogue and eats the cache-item budget", len(entry), keysOf(entry))
	}
	for _, want := range []string{"slug", "name"} {
		if _, ok := entry[want]; !ok {
			t.Errorf("entry is missing %q; got %v", want, keysOf(entry))
		}
	}

	var slug, name string
	if err := json.Unmarshal(entry["slug"], &slug); err != nil {
		t.Fatalf("decode slug: %v", err)
	}
	if err := json.Unmarshal(entry["name"], &name); err != nil {
		t.Fatalf("decode name: %v", err)
	}
	if slug != "listed-artist" || name != "Listed Artist" {
		t.Errorf("got {slug:%q name:%q}, want {slug:%q name:%q}",
			slug, name, "listed-artist", "Listed Artist")
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
