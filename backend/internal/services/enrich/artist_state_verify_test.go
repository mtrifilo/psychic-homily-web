package enrich

import (
	"context"
	"errors"
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/pipeline"
)

// ArtistsWithCityAndState lets fakeStateStore (defined in artist_state_test.go)
// also drive the verify pass.
func (f *fakeStateStore) ArtistsWithCityAndState(limit int) ([]catalogm.Artist, error) {
	if limit > 0 && limit < len(f.artists) {
		return f.artists[:limit], nil
	}
	return f.artists, nil
}

// confirmedCandidate builds a name+city MB candidate that resolves to `state`
// (Subdivision on the search result) and whose url-rels share `link`.
func confirmedCandidate(id, name, city, subdivision, link string) (pipeline.MBArtistResult, []pipeline.MBURLRelation) {
	return pipeline.MBArtistResult{
		ID: id, Name: name, Country: "US",
		BeginArea: cityArea(city, id+"-area"),
		Area:      &pipeline.MBArea{Name: subdivision, Type: "Subdivision"},
	}, []pipeline.MBURLRelation{spotifyRel(link)}
}

// TestVerifyArtistStates_CorrectsWrongGuess is the point of the pass: a bad
// highest-pop guess (Pasadena→TX) is overwritten with the identity-confirmed
// MusicBrainz state (CA).
func TestVerifyArtistStates_CorrectsWrongGuess(t *testing.T) {
	cand, rels := confirmedCandidate("c1", "LA Band", "Pasadena", "California", spotifyA)
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "LA Band", City: sp("Pasadena"), State: sp("TX"), // wrong guess
			Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{"LA Band": {cand}},
		urlRels:    map[string][]pipeline.MBURLRelation{"c1": rels},
	}

	rep, err := verifyArtistStates(context.Background(), store, g, mb, VerifyOptions{})
	if err != nil {
		t.Fatalf("verifyArtistStates: %v", err)
	}
	if rep.Corrected != 1 || len(rep.Corrections) != 1 {
		t.Fatalf("Corrected=%d corrections=%d, want 1/1", rep.Corrected, len(rep.Corrections))
	}
	u := store.updates[1]
	if u["state"] != "CA" || u["data_source"] != DataSourceMusicBrainz {
		t.Errorf("got state=%v source=%v, want CA/musicbrainz", u["state"], u["data_source"])
	}
	if c := rep.Corrections[0]; c.OldState != "TX" || c.NewState != "CA" {
		t.Errorf("correction = %s->%s, want TX->CA", c.OldState, c.NewState)
	}
}

// TestVerifyArtistStates_LeavesCorrectGuess: a guess MusicBrainz agrees with
// (Austin→TX) is confirmed and never rewritten.
func TestVerifyArtistStates_LeavesCorrectGuess(t *testing.T) {
	cand, rels := confirmedCandidate("c1", "ATX Band", "Austin", "Texas", spotifyA)
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "ATX Band", City: sp("Austin"), State: sp("TX"),
			Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"austin": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{"ATX Band": {cand}},
		urlRels:    map[string][]pipeline.MBURLRelation{"c1": rels},
	}

	rep, err := verifyArtistStates(context.Background(), store, g, mb, VerifyOptions{})
	if err != nil {
		t.Fatalf("verifyArtistStates: %v", err)
	}
	if rep.Confirmed != 1 || rep.Corrected != 0 {
		t.Fatalf("Confirmed=%d Corrected=%d, want 1/0", rep.Confirmed, rep.Corrected)
	}
	if len(store.updates) != 0 {
		t.Errorf("a confirmed-correct state must not be rewritten, wrote %v", store.updates)
	}
}

// TestVerifyArtistStates_SkipsDefiniteOK: a geocoder-unambiguous city matching
// its state is correct without any MusicBrainz call.
func TestVerifyArtistStates_SkipsDefiniteOK(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Rezn", City: sp("Chicago"), State: sp("IL"), Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{unambiguous: map[string]string{"chicago": "IL"}}
	mb := &fakeStateMB{}

	rep, err := verifyArtistStates(context.Background(), store, g, mb, VerifyOptions{})
	if err != nil {
		t.Fatalf("verifyArtistStates: %v", err)
	}
	if rep.DefiniteOK != 1 {
		t.Errorf("DefiniteOK = %d, want 1", rep.DefiniteOK)
	}
	if mb.searchCalls != 0 {
		t.Errorf("a geocoder-confirmed state needs no MusicBrainz call, got %d", mb.searchCalls)
	}
}

// TestVerifyArtistStates_UnconfirmableLeftAsIs: an artist MusicBrainz can't
// confirm (no link) keeps its (possibly wrong) state — the pass never NULLs.
func TestVerifyArtistStates_UnconfirmableLeftAsIs(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Linkless", City: sp("Pasadena"), State: sp("TX")}, // no link
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{}

	rep, err := verifyArtistStates(context.Background(), store, g, mb, VerifyOptions{})
	if err != nil {
		t.Fatalf("verifyArtistStates: %v", err)
	}
	if rep.Unverified != 1 || len(store.updates) != 0 {
		t.Fatalf("unconfirmable artist must be left as-is; Unverified=%d updates=%v", rep.Unverified, store.updates)
	}
}

// TestVerifyArtistStates_NonUSSkipped: a non-US band's US state is out of scope —
// left as-is, no MusicBrainz call.
func TestVerifyArtistStates_NonUSSkipped(t *testing.T) {
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "Boris", City: sp("Tokyo"), State: sp("CA"), Country: sp("Japan"),
			Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{}
	mb := &fakeStateMB{}

	rep, err := verifyArtistStates(context.Background(), store, g, mb, VerifyOptions{})
	if err != nil {
		t.Fatalf("verifyArtistStates: %v", err)
	}
	if rep.Unverified != 1 || mb.searchCalls != 0 || len(store.updates) != 0 {
		t.Fatalf("non-US must be skipped; Unverified=%d searches=%d updates=%v", rep.Unverified, mb.searchCalls, store.updates)
	}
}

// TestVerifyArtistStates_DryRun computes corrections but writes nothing.
func TestVerifyArtistStates_DryRun(t *testing.T) {
	cand, rels := confirmedCandidate("c1", "LA Band", "Pasadena", "California", spotifyA)
	store := &fakeStateStore{artists: []catalogm.Artist{
		{ID: 1, Name: "LA Band", City: sp("Pasadena"), State: sp("TX"), Social: catalogm.Social{Spotify: sp(spotifyA)}},
	}}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{
		candidates: map[string][]pipeline.MBArtistResult{"LA Band": {cand}},
		urlRels:    map[string][]pipeline.MBURLRelation{"c1": rels},
	}

	rep, err := verifyArtistStates(context.Background(), store, g, mb, VerifyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("verifyArtistStates: %v", err)
	}
	if rep.Corrected != 1 || len(rep.Corrections) != 1 {
		t.Fatalf("dry-run should still REPORT the correction; Corrected=%d", rep.Corrected)
	}
	if len(store.updates) != 0 {
		t.Errorf("dry-run must not write, wrote %v", store.updates)
	}
}

// TestVerifyArtistStates_MBErrorTripsBreaker: consecutive MusicBrainz errors
// disable the pass; later artists are left unverified, not searched.
func TestVerifyArtistStates_MBErrorTripsBreaker(t *testing.T) {
	artists := make([]catalogm.Artist, mbErrorBreakerThreshold+2)
	for i := range artists {
		artists[i] = catalogm.Artist{ID: uint(i + 1), Name: "Ambig", City: sp("Pasadena"), State: sp("TX"),
			Social: catalogm.Social{Spotify: sp(spotifyA)}}
	}
	store := &fakeStateStore{artists: artists}
	g := fakeGeo{ambiguous: map[string]bool{"pasadena": true}}
	mb := &fakeStateMB{searchErr: errors.New("503")}

	rep, err := verifyArtistStates(context.Background(), store, g, mb, VerifyOptions{})
	if err != nil {
		t.Fatalf("verifyArtistStates: %v", err)
	}
	if rep.Unverified != len(artists) {
		t.Errorf("Unverified = %d, want %d", rep.Unverified, len(artists))
	}
	if mb.searchCalls > mbErrorBreakerThreshold {
		t.Errorf("breaker should cap searches at %d, got %d", mbErrorBreakerThreshold, mb.searchCalls)
	}
}
