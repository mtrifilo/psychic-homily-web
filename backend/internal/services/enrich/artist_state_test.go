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
	artists   []catalogm.Artist
	updates   map[uint]map[string]interface{}
	updateErr error // when set, UpdateArtistLocation fails (records nothing)
}

func (f *fakeStateStore) ArtistsWithCityMissingState(limit int) ([]catalogm.Artist, error) {
	if limit > 0 && limit < len(f.artists) {
		return f.artists[:limit], nil
	}
	return f.artists, nil
}

func (f *fakeStateStore) UpdateArtistLocation(id uint, fields map[string]interface{}) error {
	if f.updateErr != nil {
		return f.updateErr
	}
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

func (f fakeGeo) ResolveMetro(string, string, string) (geo.Metro, bool) { return geo.Metro{}, false }

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

// fakeStateMB implements MBStateResolver, counting calls so a test can assert
// which lookups (area-rels, url-rels) were reached.
type fakeStateMB struct {
	candidates  map[string][]pipeline.MBArtistResult
	areaRels    map[string][]pipeline.MBAreaRelation
	urlRels     map[string][]pipeline.MBURLRelation
	searchErr   error
	areaErr     error
	searchCalls int
	areaCalls   int
	urlCalls    int
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

func (f *fakeStateMB) LookupArtistURLRelations(_ context.Context, mbid string) ([]pipeline.MBURLRelation, error) {
	f.urlCalls++
	return f.urlRels[mbid], nil
}

func sp(s string) *string { return &s }

func cityArea(name, id string) *pipeline.MBArea {
	return &pipeline.MBArea{ID: id, Name: name, Type: "City"}
}

func subdivisionRel(name string) pipeline.MBAreaRelation {
	return pipeline.MBAreaRelation{Type: "part of", Direction: "backward",
		Area: &pipeline.MBArea{Name: name, Type: "Subdivision"}}
}

func spotifyRel(url string) pipeline.MBURLRelation {
	r := pipeline.MBURLRelation{Type: "streaming"}
	r.URL.Resource = url
	return r
}

const spotifyA = "https://open.spotify.com/artist/aaaaaaaaaaaaaaaaaaaaaa"
const spotifyB = "https://open.spotify.com/artist/bbbbbbbbbbbbbbbbbbbbbb"

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
	if u["state"] != "IL" || u["data_source"] != DataSourceGeoNames {
		t.Errorf("got state=%v source=%v, want IL/geonames", u["state"], u["data_source"])
	}
}

// TestBackfillArtistStates_AmbiguousIdentityConfirmed is the core fix: a
// multi-state city (Pasadena) with a SINGLE MusicBrainz match resolves to the
// right state via the parent Subdivision — but only because the candidate's
// url-rels share the artist's Spotify link (identity confirmed).
func TestBackfillArtistStates_AmbiguousIdentityConfirmed(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Some LA Band", City: sp("Pasadena"),
			Social: catalogm.Social{Spotify: sp(spotifyA + "/")}}, // trailing slash → canonicalized
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			"Some LA Band": {{
				ID: "mbid-1", Name: "Some LA Band", Country: "US",
				BeginArea: cityArea("Pasadena", "area-pasadena-ca"),
			}},
		},
		areaRels: map[string][]pipeline.MBAreaRelation{"area-pasadena-ca": {subdivisionRel("California")}},
		urlRels:  map[string][]pipeline.MBURLRelation{"mbid-1": {spotifyRel(spotifyA)}},
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledMusicBrainz != 1 || store.updates[1]["state"] != "CA" {
		t.Fatalf("want CA; FilledMB=%d state=%v", rep.FilledMusicBrainz, store.updates[1]["state"])
	}
	if mb.areaCalls != 1 || mb.urlCalls != 1 {
		t.Errorf("areaCalls=%d urlCalls=%d, want 1/1", mb.areaCalls, mb.urlCalls)
	}
}

// TestBackfillArtistStates_SameCityHomonymCaughtByIdentity is the regression the
// blocked PR1 (and its first rework) lacked: a same-named band whose MusicBrainz
// city has the SAME NAME but a different state must NOT write that state. Here the
// MB match is a "Pasadena" band too, but in TX, and its url-rels do not include
// our artist's Spotify — so identity fails and the state is left NULL.
func TestBackfillArtistStates_SameCityHomonymCaughtByIdentity(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Mirror Band", City: sp("Pasadena"), // our band: Pasadena, CA
			Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			"Mirror Band": {{
				ID: "mbid-tx", Name: "Mirror Band", Country: "US",
				BeginArea: cityArea("Pasadena", "area-pasadena-tx"), // a DIFFERENT Pasadena
			}},
		},
		areaRels: map[string][]pipeline.MBAreaRelation{"area-pasadena-tx": {subdivisionRel("Texas")}},
		urlRels:  map[string][]pipeline.MBURLRelation{"mbid-tx": {spotifyRel(spotifyB)}}, // a different artist
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledMusicBrainz != 0 || rep.Unresolved != 1 {
		t.Fatalf("FilledMusicBrainz=%d Unresolved=%d, want 0/1", rep.FilledMusicBrainz, rep.Unresolved)
	}
	if _, wrote := store.updates[1]; wrote {
		t.Errorf("must NOT write TX onto a CA artist via a same-city-name homonym, wrote %v", store.updates[1])
	}
}

// TestBackfillArtistStates_DifferentCityHomonym: a name match whose MB city
// differs short-circuits before any area or identity lookup.
func TestBackfillArtistStates_DifferentCityHomonym(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Echo Band", City: sp("Pasadena"), Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			"Echo Band": {{ID: "mbid-h", Name: "Echo Band", Country: "US", BeginArea: cityArea("Houston", "area-houston")}},
		},
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.Unresolved != 1 || len(store.updates) != 0 {
		t.Fatalf("Unresolved=%d updates=%v, want 1 / none", rep.Unresolved, store.updates)
	}
	if mb.areaCalls != 0 || mb.urlCalls != 0 {
		t.Errorf("city mismatch should short-circuit before lookups, area=%d url=%d", mb.areaCalls, mb.urlCalls)
	}
}

// TestBackfillArtistStates_AgreeingHomonymsLeaveNull is the round-2 regression:
// two MusicBrainz records that AGREE on a state must NOT write it without an
// identity match. Our band is really in Pasadena, TX (state NULL); MusicBrainz has
// two same-named Pasadena, CA bands that agree on CA — but neither shares our
// artist's Spotify link, so consensus does NOT write CA. (The old consensus
// shortcut would have corrupted this; identity is now mandatory.)
func TestBackfillArtistStates_AgreeingHomonymsLeaveNull(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Common Name", City: sp("Pasadena"), // our band: Pasadena, TX
			Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			"Common Name": {
				{ID: "c1", Name: "Common Name", Country: "US",
					BeginArea: cityArea("Pasadena", "a1"), Area: &pipeline.MBArea{Name: "California", Type: "Subdivision"}},
				{ID: "c2", Name: "Common Name", Country: "US",
					BeginArea: cityArea("Pasadena", "a2"), Area: &pipeline.MBArea{Name: "California", Type: "Subdivision"}},
			},
		},
		// Neither CA homonym shares our artist's Spotify link.
		urlRels: map[string][]pipeline.MBURLRelation{
			"c1": {spotifyRel(spotifyB)},
			"c2": {spotifyRel(spotifyB)},
		},
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledMusicBrainz != 0 || rep.Unresolved != 1 {
		t.Fatalf("agreeing homonyms must not write a state; FilledMB=%d Unresolved=%d", rep.FilledMusicBrainz, rep.Unresolved)
	}
	if _, wrote := store.updates[1]; wrote {
		t.Errorf("must NOT write CA onto a TX band via agreeing homonyms, wrote %v", store.updates[1])
	}
}

// TestBackfillArtistStates_ScansPastConfirmedStatelessCandidate pins the round-2
// continuation: when the FIRST identity-confirmed candidate yields no usable state
// (its city has no parent Subdivision), scanning continues and a LATER confirmed
// record of the same artist supplies the state.
func TestBackfillArtistStates_ScansPastConfirmedStatelessCandidate(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Two Records", City: sp("Pasadena"), Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			"Two Records": {
				// Confirmed (shares the link) but its city resolves to no Subdivision.
				{ID: "c1", Name: "Two Records", Country: "US", BeginArea: cityArea("Pasadena", "a1")},
				// Confirmed, and carries the Subdivision on the search result → CA.
				{ID: "c2", Name: "Two Records", Country: "US",
					BeginArea: cityArea("Pasadena", "a2"), Area: &pipeline.MBArea{Name: "California", Type: "Subdivision"}},
			},
		},
		areaRels: map[string][]pipeline.MBAreaRelation{"a1": {}}, // c1's city → no parent state
		urlRels: map[string][]pipeline.MBURLRelation{
			"c1": {spotifyRel(spotifyA)},
			"c2": {spotifyRel(spotifyA)},
		},
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.FilledMusicBrainz != 1 || store.updates[1]["state"] != "CA" {
		t.Fatalf("want CA from the 2nd confirmed record; FilledMB=%d state=%v", rep.FilledMusicBrainz, store.updates[1]["state"])
	}
}

// TestBackfillArtistStates_SingleCandidateNoLink: an artist with no platform link
// can't have identity confirmed, so MusicBrainz is not even searched and the
// state is left NULL.
func TestBackfillArtistStates_SingleCandidateNoLink(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Linkless Band", City: sp("Pasadena")},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{
			"Linkless Band": {{ID: "c1", Name: "Linkless Band", Country: "US",
				BeginArea: cityArea("Pasadena", "a1"), Area: &pipeline.MBArea{Name: "California", Type: "Subdivision"}}},
		},
	}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.Unresolved != 1 || len(store.updates) != 0 {
		t.Fatalf("no-link artist must leave NULL; Unresolved=%d updates=%v", rep.Unresolved, store.updates)
	}
	if mb.searchCalls != 0 {
		t.Errorf("no links → MusicBrainz not searched, got %d searches", mb.searchCalls)
	}
}

// TestBackfillArtistStates_NonUSAndGeocoderOnly covers the skip paths: a non-US
// band is never given a US state, and GeocoderOnly leaves ambiguous names
// unresolved instead of calling MusicBrainz.
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
	if rep.Unresolved != 1 {
		t.Errorf("Unresolved = %d, want 1 (Pasadena left for later)", rep.Unresolved)
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
		// A link is required for mbState to reach the (erroring) search.
		artists[i] = catalogm.Artist{ID: uint(i + 1), Name: "Ambig", City: sp("Pasadena"),
			Social: catalogm.Social{Spotify: sp(spotifyA)}}
	}
	store := &fakeStateStore{artists: artists}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{searchErr: errors.New("503 service unavailable")}

	rep, err := backfillArtistStates(context.Background(), store, g, mb, StateOptions{})
	if err != nil {
		t.Fatalf("backfillArtistStates: %v", err)
	}
	if rep.Unresolved != len(artists) {
		t.Errorf("Unresolved = %d, want %d", rep.Unresolved, len(artists))
	}
	// After the breaker trips, later artists are not searched.
	if mb.searchCalls > mbErrorBreakerThreshold {
		t.Errorf("breaker should cap searches at %d, got %d", mbErrorBreakerThreshold, mb.searchCalls)
	}
}
