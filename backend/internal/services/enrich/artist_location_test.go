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
		{"city state country", "Brooklyn, New York, USA", ResolvedLocation{City: "Brooklyn", State: "NY", Country: "USA"}, true},
		{"city county state (trailing token is the state, not country)", "Brooklyn, Kings County, New York", ResolvedLocation{City: "Brooklyn", State: "NY"}, true},
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
			want:   ResolvedLocation{City: "Minneapolis", State: "MN", Country: "US"},
			wantOK: true,
		},
		{
			name:   "iso country only",
			result: pipeline.MBArtistResult{Country: "GB"},
			want:   ResolvedLocation{Country: "GB"},
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
			want:   ResolvedLocation{City: "Ontario", Country: "CA"},
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
}

func TestMatchMBLocation(t *testing.T) {
	candidates := []pipeline.MBArtistResult{
		{Name: "Famous Namesake", Country: "GB", Score: 100},
		{Name: "Snail Mail", BeginArea: &pipeline.MBArea{Name: "Baltimore", Type: "City"}, Country: "US"},
	}
	t.Run("exact name match (case-insensitive) wins over higher-scored namesake", func(t *testing.T) {
		loc, ok := matchMBLocation(candidates, "snail mail")
		if !ok {
			t.Fatal("expected a match")
		}
		if loc.City != "Baltimore" || loc.Country != "US" {
			t.Fatalf("got %+v", loc)
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
