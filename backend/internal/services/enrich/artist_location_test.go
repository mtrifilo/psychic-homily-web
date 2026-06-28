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
		{ID: "namesake-mbid", Name: "Famous Namesake", Country: "GB", Score: 100},
		{ID: "snail-mbid", Name: "Snail Mail", BeginArea: &pipeline.MBArea{Name: "Baltimore", Type: "City"}, Country: "US"},
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
		if loc.MBID != "snail-mbid" {
			t.Fatalf("MBID = %q, want the exact-name match's id (not the namesake's)", loc.MBID)
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

// --- orchestrator fakes ---

type fakeStore struct {
	artists []catalogm.Artist
	updates map[uint]map[string]interface{}
	loadErr error
}

func (f *fakeStore) ArtistsNeedingLocation(limit int) ([]catalogm.Artist, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	if limit > 0 && limit < len(f.artists) {
		return f.artists[:limit], nil
	}
	return f.artists, nil
}

func (f *fakeStore) UpdateArtistLocation(id uint, fields map[string]interface{}) error {
	if f.updates == nil {
		f.updates = map[uint]map[string]interface{}{}
	}
	f.updates[id] = fields
	return nil
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

	t.Run("PSY-1249: stamps MBID on a fresh match, keeps a set one, MBID-only when located", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{
			// Fresh artist, exact-name MB match → location AND MBID stamped.
			{ID: 1, Name: "Snail Mail"},
			// Already has an MBID → location still fills, but the MBID is not clobbered.
			{ID: 2, Name: "Turnstile", MusicBrainzArtistID: strptr("11111111-2222-3333-4444-555555555555")},
			// Fully located, MBID-less → MBID-only write (no location, no provenance).
			{ID: 3, Name: "Located", City: strptr("Oakland"), State: strptr("CA"), Country: strptr("US")},
		}}
		mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
			"Snail Mail": {{ID: "snail-mbid", Name: "Snail Mail", BeginArea: &pipeline.MBArea{Name: "Baltimore", Type: "City"}, Country: "US"}},
			"Turnstile":  {{ID: "turnstile-mbid", Name: "Turnstile", BeginArea: &pipeline.MBArea{Name: "Baltimore", Type: "City"}, Country: "US"}},
			"Located":    {{ID: "located-mbid", Name: "Located", BeginArea: &pipeline.MBArea{Name: "Oakland", Type: "City"}, Country: "US"}},
		}}

		report, err := backfillArtistLocations(context.Background(), store, fakeBandcamp{}, mb, Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Artist 1: fresh exact-name match → location + MBID both written.
		if store.updates[1]["musicbrainz_artist_id"] != "snail-mbid" {
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
		// Artist 3: fully located, MBID-less → MBID-only write, no location/provenance.
		if store.updates[3]["musicbrainz_artist_id"] != "located-mbid" {
			t.Fatalf("artist 3 should get an MBID-only stamp, got %+v", store.updates[3])
		}
		for _, k := range []string{"city", "state", "country", "data_source"} {
			if _, ok := store.updates[3][k]; ok {
				t.Fatalf("artist 3 MBID-only write must not include %s, got %+v", k, store.updates[3])
			}
		}
		// It still resolved-no-fill for LOCATION purposes (the MBID isn't a location fill).
		if report.ResolvedNoFill != 1 {
			t.Fatalf("resolvedNoFill = %d, want 1 (artist 3)", report.ResolvedNoFill)
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
