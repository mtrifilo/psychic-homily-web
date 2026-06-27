package enrich

import (
	"context"
	"errors"
	"strings"
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/pipeline"
)

// --- fakes -------------------------------------------------------------------

// fakeStateStore implements stateArtistStore without a DB, recording the updates
// a live run would write.
type fakeStateStore struct {
	artists []catalogm.Artist
	updates map[uint]map[string]interface{}
}

func (f *fakeStateStore) ArtistsWithCityMissingState(limit int) ([]catalogm.Artist, error) {
	if limit > 0 && limit < len(f.artists) {
		return f.artists[:limit], nil
	}
	return f.artists, nil
}

func (f *fakeStateStore) UpdateArtistLocation(id uint, fields map[string]interface{}) error {
	if f.updates == nil {
		f.updates = map[uint]map[string]interface{}{}
	}
	f.updates[id] = fields
	return nil
}

// fakeGeo implements geo.Geocoder. Only ResolveUSState matters here: a city is
// either unambiguous (one US state), ambiguous (multi-state), or not found.
type fakeGeo struct {
	unambiguous map[string]string // lowercased city -> state
	ambiguous   map[string]bool   // lowercased city -> true
}

func (f fakeGeo) Resolve(string, string, string) (geo.Result, bool) { return geo.Result{}, false }

func (f fakeGeo) ResolveUSState(city string) (string, geo.USStateStatus) {
	key := strings.ToLower(strings.TrimSpace(city))
	if st, ok := f.unambiguous[key]; ok {
		return st, geo.USStateUnambiguous
	}
	if f.ambiguous[key] {
		return "", geo.USStateAmbiguous
	}
	return "", geo.USStateNotFound
}

// fakeStateMB implements mbStateResolver, counting calls so a test can assert the
// area-rels lookup was (or was not) reached.
type fakeStateMB struct {
	candidates  map[string][]pipeline.MBArtistResult
	areaRels    map[string][]pipeline.MBAreaRelation
	searchErr   error
	areaErr     error
	searchCalls int
	areaCalls   int
}

func (f *fakeStateMB) SearchArtistCandidates(_ context.Context, name string) ([]pipeline.MBArtistResult, error) {
	f.searchCalls++
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.candidates[name], nil
}

func (f *fakeStateMB) LookupAreaRelations(_ context.Context, areaID string) ([]pipeline.MBAreaRelation, error) {
	f.areaCalls++
	if f.areaErr != nil {
		return nil, f.areaErr
	}
	return f.areaRels[areaID], nil
}

func sp(s string) *string { return &s }

func cityArea(name, id string) *pipeline.MBArea {
	return &pipeline.MBArea{ID: id, Name: name, Type: "City"}
}

func subdivisionRel(name string) pipeline.MBAreaRelation {
	return pipeline.MBAreaRelation{Type: "part of", Direction: "backward",
		Area: &pipeline.MBArea{Name: name, Type: "Subdivision"}}
}

// --- tests -------------------------------------------------------------------

// TestBackfillArtistStates_GeoUnambiguous: a single-state city name fills offline
// with geonames provenance and never touches MusicBrainz.
func TestBackfillArtistStates_GeoUnambiguous(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Rezn", City: sp("Chicago")},
	}}
	g := fakeGeo{unambiguous: map[string]string{"chicago": "IL"}}
	mb := &fakeStateMB{}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledGeo != 1 || rep.FilledMusicBrainz != 0 {
		t.Fatalf("FilledGeo=%d FilledMusicBrainz=%d, want 1/0", rep.FilledGeo, rep.FilledMusicBrainz)
	}
	if mb.searchCalls != 0 {
		t.Errorf("unambiguous city should not call MusicBrainz, got %d searches", mb.searchCalls)
	}
	u := store.updates[1]
	if u["state"] != "IL" {
		t.Errorf("state = %v, want IL", u["state"])
	}
	if u["data_source"] != DataSourceGeoNames {
		t.Errorf("data_source = %v, want geonames", u["data_source"])
	}
}

// TestBackfillArtistStates_AmbiguousViaAreaRels is the core fix: a multi-state
// city name (Pasadena) is resolved to the RIGHT state via the parent Subdivision,
// not a population guess.
func TestBackfillArtistStates_AmbiguousViaAreaRels(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Some LA Band", City: sp("Pasadena")},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			"Some LA Band": {{
				Name: "Some LA Band", Country: "US",
				BeginArea: cityArea("Pasadena", "area-pasadena-ca"),
			}},
		},
		areaRels: map[string][]pipeline.MBAreaRelation{
			"area-pasadena-ca": {subdivisionRel("California")},
		},
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledMusicBrainz != 1 {
		t.Fatalf("FilledMusicBrainz = %d, want 1", rep.FilledMusicBrainz)
	}
	if got := store.updates[1]["state"]; got != "CA" {
		t.Errorf("state = %v, want CA (NOT a population-guess TX)", got)
	}
	if store.updates[1]["data_source"] != DataSourceMusicBrainz {
		t.Errorf("data_source = %v, want musicbrainz", store.updates[1]["data_source"])
	}
	if mb.areaCalls != 1 {
		t.Errorf("areaCalls = %d, want 1 (parent lookup)", mb.areaCalls)
	}
}

// TestBackfillArtistStates_HomonymGuard is the regression that the blocked PR1
// lacked: a same-named band whose MusicBrainz city DIFFERS from the artist's
// stored city must NOT write that other city's state.
func TestBackfillArtistStates_HomonymGuard(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Mirror Band", City: sp("Pasadena")}, // our band is Pasadena, CA
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			// A different same-named band that MusicBrainz places in Houston, TX.
			"Mirror Band": {{
				Name: "Mirror Band", Country: "US",
				BeginArea: cityArea("Houston", "area-houston-tx"),
			}},
		},
		areaRels: map[string][]pipeline.MBAreaRelation{
			"area-houston-tx": {subdivisionRel("Texas")},
		},
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledMusicBrainz != 0 || rep.AmbiguousUnresolved != 1 {
		t.Fatalf("FilledMusicBrainz=%d AmbiguousUnresolved=%d, want 0/1", rep.FilledMusicBrainz, rep.AmbiguousUnresolved)
	}
	if _, wrote := store.updates[1]; wrote {
		t.Errorf("must not write a state for a city that doesn't match (homonym), wrote %v", store.updates[1])
	}
	if mb.areaCalls != 0 {
		t.Errorf("city mismatch should short-circuit before the area lookup, got %d", mb.areaCalls)
	}
}

// TestBackfillArtistStates_SubdivisionOnSearchResult: when the search result
// already carries the parent Subdivision, no area-rels lookup is needed.
func TestBackfillArtistStates_SubdivisionOnSearchResult(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Tagged Band", City: sp("Pasadena")},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			"Tagged Band": {{
				Name:      "Tagged Band",
				Country:   "US",
				BeginArea: cityArea("Pasadena", "area-pasadena-ca"),
				Area:      &pipeline.MBArea{Name: "California", Type: "Subdivision"},
			}},
		},
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledMusicBrainz != 1 || store.updates[1]["state"] != "CA" {
		t.Fatalf("want CA via search result; FilledMB=%d state=%v", rep.FilledMusicBrainz, store.updates[1]["state"])
	}
	if mb.areaCalls != 0 {
		t.Errorf("Subdivision already present should skip the area lookup, got %d", mb.areaCalls)
	}
}

// TestBackfillArtistStates_NonUSAndGeocoderOnly covers the skip paths: a non-US
// band is never given a US state, and GeocoderOnly leaves ambiguous names for a
// later pass instead of calling MusicBrainz.
func TestBackfillArtistStates_NonUSAndGeocoderOnly(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Boris", City: sp("Tokyo"), Country: sp("Japan")},
		{ID: 2, Name: "Ambig Band", City: sp("Pasadena")},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{GeocoderOnly: true})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (Tokyo/Japan)", rep.Skipped)
	}
	if rep.AmbiguousUnresolved != 1 {
		t.Errorf("AmbiguousUnresolved = %d, want 1 (Pasadena left for later)", rep.AmbiguousUnresolved)
	}
	if mb.searchCalls != 0 {
		t.Errorf("GeocoderOnly must not call MusicBrainz, got %d", mb.searchCalls)
	}
	if len(store.updates) != 0 {
		t.Errorf("no fills expected, got %v", store.updates)
	}
}

// TestBackfillArtistStates_DryRun computes fills but writes nothing.
func TestBackfillArtistStates_DryRun(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{{ID: 1, Name: "Rezn", City: sp("Chicago")}}}
	g := fakeGeo{unambiguous: map[string]string{"chicago": "IL"}}

	rep, err := backfillArtistStates(context.Background(), store, g, &fakeStateMB{}, StateOptions{DryRun: true})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledGeo != 1 || len(rep.Fills) != 1 {
		t.Fatalf("FilledGeo=%d Fills=%d, want 1/1", rep.FilledGeo, len(rep.Fills))
	}
	if len(store.updates) != 0 {
		t.Errorf("dry-run must not write, wrote %v", store.updates)
	}
}

// TestBackfillArtistStates_ProvenancePreserved: an artist that already has a
// data_source gets only its state filled — the existing provenance is untouched.
func TestBackfillArtistStates_ProvenancePreserved(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Rezn", City: sp("Chicago"), DataSource: sp("bandcamp")},
	}}
	g := fakeGeo{unambiguous: map[string]string{"chicago": "IL"}}

	if _, err := backfillArtistStates(context.Background(), store, g, &fakeStateMB{}, StateOptions{}); err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	u := store.updates[1]
	if u["state"] != "IL" {
		t.Errorf("state = %v, want IL", u["state"])
	}
	if _, ok := u["data_source"]; ok {
		t.Errorf("must not overwrite an existing data_source, got %v", u["data_source"])
	}
}

// TestBackfillArtistStates_MBErrorTripsBreaker: consecutive MusicBrainz errors
// disable the pass and surface an error, leaving the artists unresolved.
func TestBackfillArtistStates_MBErrorTripsBreaker(t *testing.T) {
	artists := make([]catalogm.Artist, mbErrorBreakerThreshold+2)
	for i := range artists {
		artists[i] = catalogm.Artist{ID: uint(i + 1), Name: "Ambig", City: sp("Pasadena")}
	}
	store := &fakeStateStore{artists: artists}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{searchErr: errors.New("503 service unavailable")}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.AmbiguousUnresolved != len(artists) {
		t.Errorf("AmbiguousUnresolved = %d, want %d", rep.AmbiguousUnresolved, len(artists))
	}
	// After the breaker trips, later artists are not searched: search calls are
	// capped at the threshold, not one per artist.
	if mb.searchCalls > mbErrorBreakerThreshold {
		t.Errorf("breaker should cap searches at %d, got %d", mbErrorBreakerThreshold, mb.searchCalls)
	}
}
