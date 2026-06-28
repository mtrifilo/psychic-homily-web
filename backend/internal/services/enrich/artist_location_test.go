package enrich

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/pipeline"
)

func strptr(s string) *string { return &s }

func TestParseBandcampLocation(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		want   ResolvedLocation
		wantOK bool
	}{
		{"us city + full state", "Phoenix, Arizona", ResolvedLocation{City: "Phoenix", State: "AZ"}, true},
		{"us city + abbrev state", "Austin, TX", ResolvedLocation{City: "Austin", State: "TX"}, true},
		{"two-word state", "Brooklyn, New York", ResolvedLocation{City: "Brooklyn", State: "NY"}, true},
		{"intl city + country", "Tokyo, Japan", ResolvedLocation{City: "Tokyo", Country: "Japan"}, true},
		{"intl city + country 2", "Berlin, Germany", ResolvedLocation{City: "Berlin", Country: "Germany"}, true},
		{"city state country (country canonicalized)", "Brooklyn, New York, USA", ResolvedLocation{City: "Brooklyn", State: "NY", Country: "United States"}, true},
		{"city county state (trailing token is the state, not country)", "Brooklyn, Kings County, New York", ResolvedLocation{City: "Brooklyn", State: "NY"}, true},
		// "Georgia" homograph: the COUNTRY when the city positively resolves there...
		{"georgia the country", "Tbilisi, Georgia", ResolvedLocation{City: "Tbilisi", Country: "Georgia"}, true},
		// ...but the US STATE when the city actually sits in (US) Georgia.
		{"georgia the us state", "Atlanta, Georgia", ResolvedLocation{City: "Atlanta", State: "GA"}, true},
		// A 2-letter state abbrev that collides with an ISO code (GA=Gabon) must
		// NEVER be read as the country — it's the state.
		{"abbrev GA is the state not Gabon", "Adel, GA", ResolvedLocation{City: "Adel", State: "GA"}, true},
		{"abbrev CA is the state not Canada", "Lodi, CA", ResolvedLocation{City: "Lodi", State: "CA"}, true},
		// A small US-GA town absent from the offline cities dataset stays the US
		// state (no positive evidence it's in the country Georgia).
		{"small georgia town stays the us state", "Dahlonega, Georgia", ResolvedLocation{City: "Dahlonega", State: "GA"}, true},
		{"extra whitespace", "  Seattle ,  Washington  ", ResolvedLocation{City: "Seattle", State: "WA"}, true},
		{"single token city", "Portland", ResolvedLocation{}, false},
		{"single token country", "France", ResolvedLocation{}, false},
		{"empty", "", ResolvedLocation{}, false},
		{"only separators", " , , ", ResolvedLocation{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseBandcampLocation(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLocationFromMBResult(t *testing.T) {
	cases := []struct {
		name   string
		result pipeline.MBArtistResult
		want   ResolvedLocation
		wantOK bool
	}{
		{
			name: "begin-area city + area subdivision + iso country",
			result: pipeline.MBArtistResult{
				Country:   "US",
				BeginArea: &pipeline.MBArea{Name: "Minneapolis", Type: "City"},
				Area:      &pipeline.MBArea{Name: "Minnesota", Type: "Subdivision"},
			},
			want:   ResolvedLocation{City: "Minneapolis", State: "MN", Country: "United States"},
			wantOK: true,
		},
		{
			name:   "iso country only (canonicalized to name)",
			result: pipeline.MBArtistResult{Country: "GB"},
			want:   ResolvedLocation{Country: "United Kingdom"},
			wantOK: true,
		},
		{
			name: "begin-area city only",
			result: pipeline.MBArtistResult{
				BeginArea: &pipeline.MBArea{Name: "Perth", Type: "City"},
			},
			want:   ResolvedLocation{City: "Perth"},
			wantOK: true,
		},
		{
			name: "area typed as country preferred over iso when present",
			result: pipeline.MBArtistResult{
				Area: &pipeline.MBArea{Name: "Japan", Type: "Country"},
			},
			want:   ResolvedLocation{Country: "Japan"},
			wantOK: true,
		},
		{
			name:   "nothing usable",
			result: pipeline.MBArtistResult{},
			want:   ResolvedLocation{},
			wantOK: false,
		},
		{
			name: "non-us subdivision does not map to a state",
			result: pipeline.MBArtistResult{
				BeginArea: &pipeline.MBArea{Name: "Ontario", Type: "City"},
				Area:      &pipeline.MBArea{Name: "British Columbia", Type: "Subdivision"},
				Country:   "CA",
			},
			// "British Columbia" isn't a US state → State stays empty; city+country fill.
			want:   ResolvedLocation{City: "Ontario", Country: "Canada"},
			wantOK: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := locationFromMBResult(tc.result)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBuildArtistLocationUpdate(t *testing.T) {
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	full := ResolvedLocation{City: "Phoenix", State: "AZ", Country: "US"}

	t.Run("empty artist fills all + provenance", func(t *testing.T) {
		a := &catalogm.Artist{ID: 1}
		updates, filled := buildArtistLocationUpdate(a, full, DataSourceBandcamp, confidenceBandcamp, now)
		if len(filled) != 3 {
			t.Fatalf("filled = %v, want 3 fields", filled)
		}
		if updates["city"] != "Phoenix" || updates["state"] != "AZ" || updates["country"] != "US" {
			t.Fatalf("location fields wrong: %+v", updates)
		}
		if updates["data_source"] != DataSourceBandcamp || updates["source_confidence"] != confidenceBandcamp {
			t.Fatalf("provenance wrong: %+v", updates)
		}
		if updates["last_verified_at"] != now {
			t.Fatalf("last_verified_at = %v, want %v", updates["last_verified_at"], now)
		}
	})

	t.Run("fills only empty fields", func(t *testing.T) {
		a := &catalogm.Artist{ID: 2, City: strptr("Mesa")} // city already set
		updates, filled := buildArtistLocationUpdate(a, full, DataSourceBandcamp, confidenceBandcamp, now)
		if _, ok := updates["city"]; ok {
			t.Fatalf("city should NOT be overwritten, got %v", updates["city"])
		}
		if updates["state"] != "AZ" || updates["country"] != "US" {
			t.Fatalf("expected state+country filled, got %+v", updates)
		}
		if len(filled) != 2 {
			t.Fatalf("filled = %v, want [state country]", filled)
		}
	})

	t.Run("fully located artist yields no update", func(t *testing.T) {
		a := &catalogm.Artist{ID: 3, City: strptr("Phoenix"), State: strptr("AZ"), Country: strptr("US")}
		updates, filled := buildArtistLocationUpdate(a, full, DataSourceBandcamp, confidenceBandcamp, now)
		if updates != nil || filled != nil {
			t.Fatalf("expected no update, got updates=%+v filled=%v", updates, filled)
		}
	})

	t.Run("existing data_source: location fills but provenance triple left intact", func(t *testing.T) {
		a := &catalogm.Artist{ID: 4, DataSource: strptr("spotify")}
		updates, filled := buildArtistLocationUpdate(a, full, DataSourceBandcamp, confidenceBandcamp, now)
		// Location still fills.
		if updates["city"] != "Phoenix" || len(filled) != 3 {
			t.Fatalf("location should still fill, got %+v filled=%v", updates, filled)
		}
		// The provenance triple is written together-or-not-at-all: a row already
		// attributed to another enrichment (spotify) is left fully intact, so we
		// don't bump last_verified_at to point at a source it no longer describes.
		for _, k := range []string{"data_source", "source_confidence", "last_verified_at"} {
			if _, ok := updates[k]; ok {
				t.Fatalf("%s must NOT be written when data_source already set, got %v", k, updates[k])
			}
		}
	})

	t.Run("blank-string fields count as empty", func(t *testing.T) {
		a := &catalogm.Artist{ID: 5, State: strptr("  ")}
		updates, filled := buildArtistLocationUpdate(a, full, DataSourceMusicBrainz, confidenceMusicBrainz, now)
		if updates["state"] != "AZ" {
			t.Fatalf("blank state should be filled, got %+v", updates)
		}
		if len(filled) != 3 {
			t.Fatalf("filled = %v, want 3", filled)
		}
	})

	// --- PSY-1249: MBID stamping ---
	mbLoc := ResolvedLocation{City: "Baltimore", State: "MD", Country: "US", MBID: "65f4f0c5-ef9e-490c-aee3-909e7ae6b2ab"}

	t.Run("MB match stamps the MBID alongside a location fill", func(t *testing.T) {
		a := &catalogm.Artist{ID: 6}
		updates, filled := buildArtistLocationUpdate(a, mbLoc, DataSourceMusicBrainz, confidenceMusicBrainz, now)
		if updates["musicbrainz_artist_id"] != mbLoc.MBID {
			t.Fatalf("expected MBID stamped, got %+v", updates)
		}
		if len(filled) != 3 {
			t.Fatalf("filled = %v, want 3 location fields", filled)
		}
	})

	t.Run("a set MBID is never overwritten", func(t *testing.T) {
		a := &catalogm.Artist{ID: 7, MusicBrainzArtistID: strptr("11111111-2222-3333-4444-555555555555")}
		updates, _ := buildArtistLocationUpdate(a, mbLoc, DataSourceMusicBrainz, confidenceMusicBrainz, now)
		if _, ok := updates["musicbrainz_artist_id"]; ok {
			t.Fatalf("MBID must NOT be overwritten, got %v", updates["musicbrainz_artist_id"])
		}
	})

	t.Run("a Bandcamp location (no MBID) stamps nothing", func(t *testing.T) {
		a := &catalogm.Artist{ID: 8}
		bcLoc := ResolvedLocation{City: "Tokyo", Country: "Japan"} // MBID empty
		updates, _ := buildArtistLocationUpdate(a, bcLoc, DataSourceBandcamp, confidenceBandcamp, now)
		if _, ok := updates["musicbrainz_artist_id"]; ok {
			t.Fatalf("no MBID should be stamped for a Bandcamp location, got %v", updates["musicbrainz_artist_id"])
		}
	})

	t.Run("MBID-only write when the location is already complete (no provenance)", func(t *testing.T) {
		a := &catalogm.Artist{ID: 9, City: strptr("Baltimore"), State: strptr("MD"), Country: strptr("US")}
		updates, filled := buildArtistLocationUpdate(a, mbLoc, DataSourceMusicBrainz, confidenceMusicBrainz, now)
		if filled != nil {
			t.Fatalf("expected no location fill, got filled=%v", filled)
		}
		if updates == nil || updates["musicbrainz_artist_id"] != mbLoc.MBID {
			t.Fatalf("expected an MBID-only update, got %+v", updates)
		}
		// An MBID-only stamp claims no location provenance and writes no location field.
		for _, k := range []string{"data_source", "source_confidence", "last_verified_at", "city", "state", "country"} {
			if _, ok := updates[k]; ok {
				t.Fatalf("%s must NOT be written on an MBID-only stamp, got %v", k, updates[k])
			}
		}
	})
}

func TestMatchMBLocation(t *testing.T) {
	candidates := []pipeline.MBArtistResult{
		{ID: "44444444-4444-4444-4444-444444444444", Name: "Famous Namesake", Country: "GB", Score: 100},
		{ID: "11111111-1111-1111-1111-111111111111", Name: "Snail Mail", BeginArea: &pipeline.MBArea{Name: "Baltimore", Type: "City"}, Country: "US"},
	}
	t.Run("exact name match (case-insensitive) wins over higher-scored namesake", func(t *testing.T) {
		loc, ok := matchMBLocation(candidates, "snail mail")
		if !ok {
			t.Fatal("expected a match")
		}
		if loc.City != "Baltimore" || loc.Country != "United States" {
			t.Fatalf("got %+v", loc)
		}
		// PSY-1249: the matched candidate's MBID rides along (NOT the namesake's).
		if loc.MBID != "11111111-1111-1111-1111-111111111111" {
			t.Fatalf("MBID = %q, want the exact-name match's id (not the namesake's)", loc.MBID)
		}
	})
	t.Run("a malformed (non-UUID) candidate id is not carried", func(t *testing.T) {
		c := []pipeline.MBArtistResult{
			{ID: "not-a-uuid", Name: "Garbage Id", BeginArea: &pipeline.MBArea{Name: "Austin", Type: "City"}, Country: "US"},
		}
		loc, ok := matchMBLocation(c, "Garbage Id")
		if !ok {
			t.Fatal("expected a location match")
		}
		if loc.City != "Austin" {
			t.Fatalf("location should still resolve, got %+v", loc)
		}
		if loc.MBID != "" {
			t.Fatalf("a non-UUID id must NOT be carried, got %q", loc.MBID)
		}
	})
	t.Run("no name match", func(t *testing.T) {
		if _, ok := matchMBLocation(candidates, "Nonexistent"); ok {
			t.Fatal("expected no match")
		}
	})
	t.Run("name matches but no usable location", func(t *testing.T) {
		c := []pipeline.MBArtistResult{{Name: "Locationless"}}
		if _, ok := matchMBLocation(c, "Locationless"); ok {
			t.Fatal("expected no location")
		}
	})
}

// TestBackfillArtistLocations_Memo covers the PSY-1250 sweep behavior the manual cmd
// doesn't exercise: the no-result memo (stamp-before-resolve + cutoff filtering) is on
// only when ReattemptWindow > 0, and never writes in a dry run.
func TestBackfillArtistLocations_Memo(t *testing.T) {
	const window = 720 * time.Hour // 30 days
	mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
		"Resolvable": {{ID: "11111111-1111-1111-1111-111111111111", Name: "Resolvable", BeginArea: &pipeline.MBArea{Name: "Austin", Type: "City"}, Country: "US"}},
	}}

	t.Run("sweep mode stamps the WHOLE batch before resolving (incl. a miss) + passes a cutoff", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{
			{ID: 1, Name: "Resolvable"},
			{ID: 2, Name: "Unresolvable"}, // no MB/Bandcamp → a miss, but still stamped
		}}
		_, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{ReattemptWindow: window})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := store.stamped[1]; !ok {
			t.Fatalf("artist 1 should be stamped attempted")
		}
		if _, ok := store.stamped[2]; !ok {
			t.Fatalf("artist 2 (a miss) must still be stamped (poison-row safety)")
		}
		if store.lastCutoff == nil {
			t.Fatalf("sweep mode must pass a reattempt cutoff to the store")
		}
		if store.updates[1]["city"] != "Austin" {
			t.Fatalf("the resolvable artist should still fill, got %+v", store.updates[1])
		}
	})

	t.Run("manual cmd mode (no ReattemptWindow) never stamps + passes a nil cutoff", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{{ID: 1, Name: "Resolvable"}}}
		_, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(store.stamped) != 0 {
			t.Fatalf("cmd mode must not stamp the memo, got %v", store.stamped)
		}
		if store.lastCutoff != nil {
			t.Fatalf("cmd mode must pass a nil cutoff, got %v", *store.lastCutoff)
		}
	})

	t.Run("dry-run sweep filters by cutoff but writes nothing", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{{ID: 1, Name: "Resolvable"}}}
		_, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{ReattemptWindow: window, DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(store.stamped) != 0 {
			t.Fatalf("dry-run must not stamp the memo, got %v", store.stamped)
		}
		if len(store.updates) != 0 {
			t.Fatalf("dry-run must not write locations, got %v", store.updates)
		}
		if store.lastCutoff == nil {
			t.Fatalf("dry-run sweep should still filter selection by the cutoff")
		}
	})

	t.Run("a recently-attempted artist is excluded by the cutoff", func(t *testing.T) {
		recent := time.Now() // attempted now → within any window → excluded
		store := &fakeStore{artists: []catalogm.Artist{
			{ID: 1, Name: "Resolvable"},                                     // never attempted → in
			{ID: 2, Name: "Resolvable", LocationEnrichAttemptedAt: &recent}, // recent → out
		}}
		_, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{ReattemptWindow: window})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := store.stamped[1]; !ok {
			t.Fatalf("never-attempted artist 1 should be processed")
		}
		if _, ok := store.stamped[2]; ok {
			t.Fatalf("recently-attempted artist 2 must be excluded from the batch")
		}
	})
}

// TestBackfillArtistLocations_CtxCancel pins the sweep's mid-batch shutdown behavior
// (PSY-1250): with an already-cancelled ctx the whole batch is still stamped attempted
// up front (poison-row safety), but the per-artist loop breaks before resolving any, so
// nothing is written. The fake MB would resolve a city if the loop ran — proving the
// break, not a missing match, is what prevents the write.
func TestBackfillArtistLocations_CtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	store := &fakeStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Resolvable"},
		{ID: 2, Name: "Resolvable"},
	}}
	mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
		"Resolvable": {{ID: "11111111-1111-1111-1111-111111111111", Name: "Resolvable", BeginArea: &pipeline.MBArea{Name: "Austin", Type: "City"}, Country: "US"}},
	}}
	_, err := backfillArtistLocations(ctx, store, fakeBandcamp{}, mb, Options{ReattemptWindow: 720 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.stamped) != 2 {
		t.Fatalf("the whole batch should be stamped up front, got %v", store.stamped)
	}
	if len(store.updates) != 0 {
		t.Fatalf("a cancelled ctx must process (write) no artists, got %v", store.updates)
	}
}

// TestEnrichArtistLocationByID covers the PSY-1251 on-create single-artist path: it
// fills + stamps a city-less artist, no-ops on an already-located or missing one, and
// stamps (but doesn't fill) a miss so the sweep doesn't immediately retry. The conflict
// path shares resolveLocation with the sweep (covered by TestBackfillArtistLocations).
func TestEnrichArtistLocationByID(t *testing.T) {
	const mbid = "11111111-1111-1111-1111-111111111111"
	mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
		"Snail Mail": {{ID: mbid, Name: "Snail Mail", BeginArea: &pipeline.MBArea{Name: "Baltimore", Type: "City"}, Country: "US"}},
	}}

	t.Run("fills location + MBID and stamps the memo", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{{ID: 1, Name: "Snail Mail"}}}
		if err := enrichArtistLocationByID(context.Background(), store, fakeBandcamp{}, mb, 1); err != nil {
			t.Fatal(err)
		}
		if store.updates[1]["city"] != "Baltimore" {
			t.Fatalf("expected city filled, got %+v", store.updates[1])
		}
		if store.updates[1]["musicbrainz_artist_id"] != mbid {
			t.Fatalf("expected MBID stamped, got %+v", store.updates[1])
		}
		if _, ok := store.stamped[1]; !ok {
			t.Fatalf("expected the sweep memo stamped so the sweep skips it")
		}
	})

	t.Run("no-op when already located (fill-when-empty)", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{{ID: 1, Name: "Snail Mail", City: strptr("Phoenix")}}}
		if err := enrichArtistLocationByID(context.Background(), store, fakeBandcamp{}, mb, 1); err != nil {
			t.Fatal(err)
		}
		if len(store.updates) != 0 || len(store.stamped) != 0 {
			t.Fatalf("a located artist must not be written or stamped, got updates=%v stamped=%v", store.updates, store.stamped)
		}
	})

	t.Run("no-op when the artist is gone", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{}}
		if err := enrichArtistLocationByID(context.Background(), store, fakeBandcamp{}, mb, 999); err != nil {
			t.Fatal(err)
		}
		if len(store.updates) != 0 || len(store.stamped) != 0 {
			t.Fatalf("a missing artist must do nothing")
		}
	})

	t.Run("a miss stamps but does not fill", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{{ID: 1, Name: "Unknown Band"}}}
		if err := enrichArtistLocationByID(context.Background(), store, fakeBandcamp{}, mb, 1); err != nil {
			t.Fatal(err)
		}
		if len(store.updates) != 0 {
			t.Fatalf("a miss must not fill, got %v", store.updates)
		}
		if _, ok := store.stamped[1]; !ok {
			t.Fatalf("a miss must still be stamped so the sweep doesn't immediately retry")
		}
	})
}

// --- orchestrator fakes ---

type fakeStore struct {
	artists    []catalogm.Artist
	updates    map[uint]map[string]interface{}
	loadErr    error
	lastCutoff *time.Time         // records the reattemptCutoff the orchestrator passed
	stamped    map[uint]time.Time // ids marked via StampLocationAttempted
}

func (f *fakeStore) ArtistsNeedingLocation(limit int, reattemptCutoff *time.Time) ([]catalogm.Artist, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	f.lastCutoff = reattemptCutoff
	var out []catalogm.Artist
	for _, a := range f.artists {
		// Mirror the real memo filter: include when never attempted (NULL) or attempted
		// before the cutoff; exclude artists attempted within the window.
		if reattemptCutoff != nil && a.LocationEnrichAttemptedAt != nil &&
			!a.LocationEnrichAttemptedAt.Before(*reattemptCutoff) {
			continue
		}
		out = append(out, a)
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeStore) UpdateArtistLocation(id uint, fields map[string]interface{}) error {
	if f.updates == nil {
		f.updates = map[uint]map[string]interface{}{}
	}
	f.updates[id] = fields
	return nil
}

func (f *fakeStore) StampLocationAttempted(ids []uint, at time.Time) error {
	if f.stamped == nil {
		f.stamped = map[uint]time.Time{}
	}
	for _, id := range ids {
		f.stamped[id] = at
	}
	return nil
}

func (f *fakeStore) ArtistByID(id uint) (*catalogm.Artist, error) {
	for i := range f.artists {
		if f.artists[i].ID == id {
			a := f.artists[i]
			return &a, nil
		}
	}
	return nil, nil
}

type fakeBandcamp struct{ byURL map[string]string }

func (f fakeBandcamp) ResolveProfileLocation(_ context.Context, profileURL string) (string, bool) {
	loc, ok := f.byURL[profileURL]
	return loc, ok
}

type fakeMB struct {
	byName map[string][]pipeline.MBArtistResult
	err    error
	calls  int // how many times SearchArtistCandidates was invoked (breaker test)
}

func (f *fakeMB) SearchArtistCandidates(_ context.Context, name string) ([]pipeline.MBArtistResult, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.byName[name], nil
}

func TestBackfillArtistLocations(t *testing.T) {
	t.Run("musicbrainz primary, bandcamp fallback, fill-when-empty", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{
			// Has BOTH sources — MusicBrainz must win the tie.
			{ID: 1, Name: "Both Band", Social: catalogm.Social{Bandcamp: strptr("https://both.bandcamp.com/")}},
			// No MusicBrainz entry → falls back to Bandcamp.
			{ID: 2, Name: "BC Fallback Band", Social: catalogm.Social{Bandcamp: strptr("https://bc.bandcamp.com/")}},
			{ID: 3, Name: "Unknown Band"}, // neither source
			{ID: 4, Name: "Located Band", City: strptr("X"), State: strptr("CA"), Country: strptr("US")}, // nothing empty
		}}
		bc := fakeBandcamp{byURL: map[string]string{
			"https://both.bandcamp.com/": "Phoenix, Arizona", // should be overridden by MB
			"https://bc.bandcamp.com/":   "Tokyo, Japan",
		}}
		mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
			"Both Band":    {{Name: "Both Band", BeginArea: &pipeline.MBArea{Name: "Chicago", Type: "City"}, Area: &pipeline.MBArea{Name: "Illinois", Type: "Subdivision"}, Country: "US"}},
			"Located Band": {{Name: "Located Band", BeginArea: &pipeline.MBArea{Name: "Oakland", Type: "City"}}},
		}}

		report, err := backfillArtistLocations(context.Background(), store, bc, mb, Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.ArtistsScanned != 4 {
			t.Fatalf("scanned = %d, want 4", report.ArtistsScanned)
		}
		if report.FilledMusicBrainz != 1 || report.FilledBandcamp != 1 {
			t.Fatalf("filled mb=%d bandcamp=%d, want 1/1", report.FilledMusicBrainz, report.FilledBandcamp)
		}
		if report.Missed != 1 {
			t.Fatalf("missed = %d, want 1 (Unknown Band)", report.Missed)
		}
		if report.ResolvedNoFill != 1 {
			t.Fatalf("resolvedNoFill = %d, want 1 (Located Band, fields already set)", report.ResolvedNoFill)
		}
		// Precedence: artist 1 has a Bandcamp location too, but MusicBrainz wins.
		if store.updates[1]["city"] != "Chicago" || store.updates[1]["state"] != "IL" {
			t.Fatalf("artist 1 should be MB (Chicago, IL), got %+v", store.updates[1])
		}
		if store.updates[1]["data_source"] != DataSourceMusicBrainz {
			t.Fatalf("artist 1 provenance should be musicbrainz, got %+v", store.updates[1])
		}
		// Fallback: artist 2 has no MB entry → Bandcamp fills it.
		if store.updates[2]["city"] != "Tokyo" || store.updates[2]["country"] != "Japan" {
			t.Fatalf("artist 2 should be Bandcamp (Tokyo, Japan), got %+v", store.updates[2])
		}
		if store.updates[2]["data_source"] != DataSourceBandcamp {
			t.Fatalf("artist 2 provenance should be bandcamp, got %+v", store.updates[2])
		}
	})

	t.Run("PSY-1249: stamps MBID on a fresh match, keeps a set one, MBID-only when nothing new fills", func(t *testing.T) {
		const (
			snailMBID = "11111111-1111-1111-1111-111111111111"
			turnMBID  = "22222222-2222-2222-2222-222222222222"
			locMBID   = "33333333-3333-3333-3333-333333333333"
		)
		store := &fakeStore{artists: []catalogm.Artist{
			// Fresh artist, exact-name MB match → location AND MBID stamped.
			{ID: 1, Name: "Snail Mail"},
			// Already has an MBID → location still fills, but the MBID is not clobbered.
			{ID: 2, Name: "Turnstile", MusicBrainzArtistID: strptr("99999999-9999-9999-9999-999999999999")},
			// City-less (so the production `city IS NULL` gate selects it) but state +
			// country already set, and MB resolves only that same state/country → no
			// location field fills → MBID-only write.
			{ID: 3, Name: "Located", State: strptr("CA"), Country: strptr("US")},
		}}
		mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
			"Snail Mail": {{ID: snailMBID, Name: "Snail Mail", BeginArea: &pipeline.MBArea{Name: "Baltimore", Type: "City"}, Country: "US"}},
			"Turnstile":  {{ID: turnMBID, Name: "Turnstile", BeginArea: &pipeline.MBArea{Name: "Baltimore", Type: "City"}, Country: "US"}},
			"Located":    {{ID: locMBID, Name: "Located", Area: &pipeline.MBArea{Name: "California", Type: "Subdivision"}, Country: "US"}},
		}}

		report, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Artist 1: fresh exact-name match → location + MBID both written.
		if store.updates[1]["musicbrainz_artist_id"] != snailMBID {
			t.Fatalf("artist 1 should get its MBID stamped, got %+v", store.updates[1])
		}
		if store.updates[1]["city"] != "Baltimore" {
			t.Fatalf("artist 1 should also fill location, got %+v", store.updates[1])
		}
		// Artist 2: a set MBID is never overwritten (location still fills).
		if _, ok := store.updates[2]["musicbrainz_artist_id"]; ok {
			t.Fatalf("artist 2's set MBID must not be overwritten, got %v", store.updates[2]["musicbrainz_artist_id"])
		}
		if store.updates[2]["city"] != "Baltimore" {
			t.Fatalf("artist 2 location should still fill, got %+v", store.updates[2])
		}
		// Artist 3: city-less but state/country set → MBID-only write, no location/provenance.
		if store.updates[3]["musicbrainz_artist_id"] != locMBID {
			t.Fatalf("artist 3 should get an MBID-only stamp, got %+v", store.updates[3])
		}
		for _, k := range []string{"city", "state", "country", "data_source"} {
			if _, ok := store.updates[3][k]; ok {
				t.Fatalf("artist 3 MBID-only write must not include %s, got %+v", k, store.updates[3])
			}
		}
		// Artist 3 is resolved-no-fill for LOCATION purposes; both 1 and 3 stamped an MBID.
		if report.ResolvedNoFill != 1 {
			t.Fatalf("resolvedNoFill = %d, want 1 (artist 3)", report.ResolvedNoFill)
		}
		if report.StampedMBID != 2 {
			t.Fatalf("stampedMBID = %d, want 2 (artists 1 and 3; artist 2 kept its own)", report.StampedMBID)
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{
			{ID: 1, Name: "Band", Social: catalogm.Social{Bandcamp: strptr("https://b.bandcamp.com/")}},
		}}
		bc := fakeBandcamp{byURL: map[string]string{"https://b.bandcamp.com/": "Tokyo, Japan"}}
		report, err := backfillArtistLocations(context.Background(), store, bc, &fakeMB{}, Options{DryRun: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.FilledBandcamp != 1 {
			t.Fatalf("dry-run should still report 1 planned fill, got %d", report.FilledBandcamp)
		}
		if len(store.updates) != 0 {
			t.Fatalf("dry-run must not write, got %+v", store.updates)
		}
	})

	t.Run("bandcamp-only skips musicbrainz", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{{ID: 1, Name: "MB Band"}}}
		mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
			"MB Band": {{Name: "MB Band", BeginArea: &pipeline.MBArea{Name: "Denver", Type: "City"}}},
		}}
		report, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{BandcampOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.FilledMusicBrainz != 0 || report.Missed != 1 {
			t.Fatalf("bandcamp-only should skip MB: mb=%d missed=%d", report.FilledMusicBrainz, report.Missed)
		}
	})

	t.Run("musicbrainz error surfaces in report", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{{ID: 1, Name: "Band"}}}
		mb := &fakeMB{err: errors.New("rate limited (HTTP 503)")}
		report, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{})
		if err != nil {
			t.Fatalf("orchestrator should not hard-fail on a per-artist MB error: %v", err)
		}
		if len(report.Errors) != 1 {
			t.Fatalf("expected 1 captured error, got %v", report.Errors)
		}
		if report.Missed != 1 {
			t.Fatalf("errored artist should count as missed, got %d", report.Missed)
		}
	})

	t.Run("country conflict between sources is skipped, not filled", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{
			// MB name-matches an Italian namesake; the band's own Bandcamp says
			// Phoenix (US state) → countries disagree → skip for review.
			{ID: 1, Name: "Yellowcake", Social: catalogm.Social{Bandcamp: strptr("https://yc.bandcamp.com/")}},
			// MB (LA, US) vs the page's Seattle (US) → SAME country → MB wins.
			{ID: 2, Name: "Tool", Social: catalogm.Social{Bandcamp: strptr("https://tool.bandcamp.com/")}},
		}}
		bc := fakeBandcamp{byURL: map[string]string{
			"https://yc.bandcamp.com/":   "Phoenix, Arizona",
			"https://tool.bandcamp.com/": "Seattle, Washington",
		}}
		mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
			"Yellowcake": {{Name: "Yellowcake", Area: &pipeline.MBArea{Name: "Italy", Type: "Country"}}},
			"Tool":       {{Name: "Tool", BeginArea: &pipeline.MBArea{Name: "Los Angeles", Type: "City"}, Country: "US"}},
		}}
		report, err := backfillArtistLocations(context.Background(), store, bc, mb, Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(report.Conflicts) != 1 || report.Conflicts[0].ArtistID != 1 {
			t.Fatalf("expected 1 conflict for Yellowcake, got %+v", report.Conflicts)
		}
		if _, wrote := store.updates[1]; wrote {
			t.Fatalf("conflicted artist must NOT be written, got %+v", store.updates[1])
		}
		// Tool: same country → MB wins (Los Angeles), not skipped.
		if store.updates[2]["city"] != "Los Angeles" || store.updates[2]["data_source"] != DataSourceMusicBrainz {
			t.Fatalf("Tool should fill from MB (Los Angeles), got %+v", store.updates[2])
		}
		if report.FilledMusicBrainz != 1 {
			t.Fatalf("expected 1 MB fill (Tool), got %d", report.FilledMusicBrainz)
		}
	})

	t.Run("musicbrainz error does NOT suppress the bandcamp fallback", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{
			{ID: 1, Name: "Band", Social: catalogm.Social{Bandcamp: strptr("https://b.bandcamp.com/")}},
		}}
		mb := &fakeMB{err: errors.New("rate limited (HTTP 503)")}
		bc := fakeBandcamp{byURL: map[string]string{"https://b.bandcamp.com/": "Austin, Texas"}}
		report, err := backfillArtistLocations(context.Background(), store, bc, mb, Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Recovered via Bandcamp despite the MB error.
		if report.FilledBandcamp != 1 || report.Missed != 0 {
			t.Fatalf("expected bandcamp recovery: filled=%d missed=%d", report.FilledBandcamp, report.Missed)
		}
		if store.updates[1]["state"] != "TX" {
			t.Fatalf("expected Texas from bandcamp, got %+v", store.updates[1])
		}
		// The MB error fed the breaker but is NOT recorded as a run error — the
		// artist resolved fine.
		if len(report.Errors) != 0 {
			t.Fatalf("recovered artist must not record an error, got %v", report.Errors)
		}
	})

	t.Run("circuit breaker disables musicbrainz after sustained errors", func(t *testing.T) {
		var artists []catalogm.Artist
		for i := 1; i <= 10; i++ {
			artists = append(artists, catalogm.Artist{ID: uint(i), Name: fmt.Sprintf("Band %d", i)})
		}
		store := &fakeStore{artists: artists}
		mb := &fakeMB{err: errors.New("rate limited (HTTP 503)")}
		report, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// MB is called only until the breaker trips on the Nth consecutive error;
		// the remaining artists make no doomed MB call.
		if mb.calls != mbErrorBreakerThreshold {
			t.Fatalf("MB called %d times, want %d (breaker should stop further calls)", mb.calls, mbErrorBreakerThreshold)
		}
		if report.Missed != 10 {
			t.Fatalf("missed = %d, want 10", report.Missed)
		}
		var disabled bool
		for _, e := range report.Errors {
			if strings.Contains(e, "musicbrainz disabled after") {
				disabled = true
			}
		}
		if !disabled {
			t.Fatalf("expected a 'musicbrainz disabled' notice, got %v", report.Errors)
		}
	})
}
