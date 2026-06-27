package enrich

import (
	"context"
	"errors"
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

	t.Run("existing data_source is not clobbered", func(t *testing.T) {
		a := &catalogm.Artist{ID: 4, DataSource: strptr("spotify")}
		updates, _ := buildArtistLocationUpdate(a, full, DataSourceBandcamp, confidenceBandcamp, now)
		if _, ok := updates["data_source"]; ok {
			t.Fatalf("data_source should be preserved, got %v", updates["data_source"])
		}
		if _, ok := updates["source_confidence"]; ok {
			t.Fatalf("source_confidence should not be set when data_source preserved")
		}
		if updates["last_verified_at"] != now {
			t.Fatalf("last_verified_at should still be bumped")
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
}

func (f fakeMB) SearchArtistCandidates(_ context.Context, name string) ([]pipeline.MBArtistResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byName[name], nil
}

func TestBackfillArtistLocations(t *testing.T) {
	t.Run("bandcamp primary, musicbrainz fallback, fill-when-empty", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{
			{ID: 1, Name: "Bandcamp Band", Social: catalogm.Social{Bandcamp: strptr("https://bcband.bandcamp.com/")}},
			{ID: 2, Name: "MB Only Band"}, // no bandcamp → MB fallback
			{ID: 3, Name: "Unknown Band"}, // neither source
			{ID: 4, Name: "Located Band", City: strptr("X"), State: strptr("CA"), Country: strptr("US")}, // nothing empty for its resolved loc
		}}
		bc := fakeBandcamp{byURL: map[string]string{
			"https://bcband.bandcamp.com/": "Phoenix, Arizona",
		}}
		mb := fakeMB{byName: map[string][]pipeline.MBArtistResult{
			"MB Only Band": {{Name: "MB Only Band", BeginArea: &pipeline.MBArea{Name: "Chicago", Type: "City"}, Country: "US"}},
			"Located Band": {{Name: "Located Band", BeginArea: &pipeline.MBArea{Name: "Oakland", Type: "City"}}},
		}}

		report, err := backfillArtistLocations(context.Background(), store, bc, mb, Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.ArtistsScanned != 4 {
			t.Fatalf("scanned = %d, want 4", report.ArtistsScanned)
		}
		if report.FilledBandcamp != 1 || report.FilledMusicBrainz != 1 {
			t.Fatalf("filled bandcamp=%d mb=%d, want 1/1", report.FilledBandcamp, report.FilledMusicBrainz)
		}
		if report.Missed != 1 {
			t.Fatalf("missed = %d, want 1 (Unknown Band)", report.Missed)
		}
		if report.ResolvedNoFill != 1 {
			t.Fatalf("resolvedNoFill = %d, want 1 (Located Band, city already set)", report.ResolvedNoFill)
		}
		// Live run wrote both fills.
		if store.updates[1]["city"] != "Phoenix" || store.updates[1]["state"] != "AZ" {
			t.Fatalf("artist 1 update wrong: %+v", store.updates[1])
		}
		if store.updates[1]["data_source"] != DataSourceBandcamp {
			t.Fatalf("artist 1 provenance wrong: %+v", store.updates[1])
		}
		if store.updates[2]["city"] != "Chicago" || store.updates[2]["data_source"] != DataSourceMusicBrainz {
			t.Fatalf("artist 2 update wrong: %+v", store.updates[2])
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		store := &fakeStore{artists: []catalogm.Artist{
			{ID: 1, Name: "Band", Social: catalogm.Social{Bandcamp: strptr("https://b.bandcamp.com/")}},
		}}
		bc := fakeBandcamp{byURL: map[string]string{"https://b.bandcamp.com/": "Tokyo, Japan"}}
		report, err := backfillArtistLocations(context.Background(), store, bc, fakeMB{}, Options{DryRun: true})
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
		mb := fakeMB{byName: map[string][]pipeline.MBArtistResult{
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
		mb := fakeMB{err: errors.New("rate limited (HTTP 503)")}
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
}
