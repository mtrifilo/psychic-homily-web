package catalog

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

type fakeRadioCandidatesLister struct {
	got          contracts.RadioArtistCandidatesQuery
	listing      *contracts.RadioArtistCandidateListing
	gotTracks    contracts.RadioTrackCandidatesQuery
	trackListing *contracts.RadioTrackCandidateListing
	err          error
}

func (f *fakeRadioCandidatesLister) ListArtistCandidates(_ context.Context, q contracts.RadioArtistCandidatesQuery) (*contracts.RadioArtistCandidateListing, error) {
	f.got = q
	return f.listing, f.err
}

func (f *fakeRadioCandidatesLister) ListTrackCandidates(_ context.Context, q contracts.RadioTrackCandidatesQuery) (*contracts.RadioTrackCandidateListing, error) {
	f.gotTracks = q
	return f.trackListing, f.err
}

func TestAdminListRadioArtistCandidates_PassesQuery(t *testing.T) {
	listing := &contracts.RadioArtistCandidateListing{Total: 3}
	fake := &fakeRadioCandidatesLister{listing: listing}
	h := NewRadioCandidatesHandler(fake)

	resp, err := h.AdminListRadioArtistCandidatesHandler(context.Background(), &AdminListRadioArtistCandidatesRequest{Lane: "auto", Sort: "stations", Limit: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := contracts.RadioArtistCandidatesQuery{Lane: contracts.RadioCandidateLaneFilterAuto, Sort: contracts.RadioCandidateSortStations, Limit: 25}
	if fake.got != want {
		t.Errorf("got query %+v, want %+v", fake.got, want)
	}
	if resp.Body != listing {
		t.Errorf("response body is not the service listing")
	}
}

func TestAdminListRadioArtistCandidates_Defaults(t *testing.T) {
	fake := &fakeRadioCandidatesLister{listing: &contracts.RadioArtistCandidateListing{}}
	h := NewRadioCandidatesHandler(fake)

	if _, err := h.AdminListRadioArtistCandidatesHandler(context.Background(), &AdminListRadioArtistCandidatesRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := contracts.RadioArtistCandidatesQuery{Lane: contracts.RadioCandidateLaneFilterAll, Sort: contracts.RadioCandidateSortScore, Limit: 50}
	if fake.got != want {
		t.Errorf("got query %+v, want %+v", fake.got, want)
	}
}

func TestAdminListRadioArtistCandidates_ServiceError(t *testing.T) {
	h := NewRadioCandidatesHandler(&fakeRadioCandidatesLister{err: errors.New("database error")})
	_, err := h.AdminListRadioArtistCandidatesHandler(context.Background(), &AdminListRadioArtistCandidatesRequest{Lane: "all", Sort: "score", Limit: 10})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestAdminListRadioTrackCandidates_PassesQuery(t *testing.T) {
	listing := &contracts.RadioTrackCandidateListing{Total: 3}
	fake := &fakeRadioCandidatesLister{trackListing: listing}
	h := NewRadioCandidatesHandler(fake)

	resp, err := h.AdminListRadioTrackCandidatesHandler(context.Background(), &AdminListRadioTrackCandidatesRequest{View: "unmatched", Lane: "human", Sort: "plays", Limit: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := contracts.RadioTrackCandidatesQuery{
		View: contracts.RadioTrackCandidateViewUnmatched, Lane: contracts.RadioCandidateLaneFilterHuman,
		Sort: contracts.RadioCandidateSortPlays, Limit: 25,
	}
	if fake.gotTracks != want {
		t.Errorf("got query %+v, want %+v", fake.gotTracks, want)
	}
	if resp.Body != listing {
		t.Errorf("response body is not the service listing")
	}
}

func TestAdminListRadioTrackCandidates_Defaults(t *testing.T) {
	fake := &fakeRadioCandidatesLister{trackListing: &contracts.RadioTrackCandidateListing{}}
	h := NewRadioCandidatesHandler(fake)

	if _, err := h.AdminListRadioTrackCandidatesHandler(context.Background(), &AdminListRadioTrackCandidatesRequest{View: "unlinked"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := contracts.RadioTrackCandidatesQuery{
		View: contracts.RadioTrackCandidateViewUnlinked, Lane: contracts.RadioCandidateLaneFilterAll,
		Sort: contracts.RadioCandidateSortScore, Limit: 50,
	}
	if fake.gotTracks != want {
		t.Errorf("got query %+v, want %+v", fake.gotTracks, want)
	}
}

func TestAdminListRadioTrackCandidates_ServiceError(t *testing.T) {
	h := NewRadioCandidatesHandler(&fakeRadioCandidatesLister{err: errors.New("database error")})
	_, err := h.AdminListRadioTrackCandidatesHandler(context.Background(), &AdminListRadioTrackCandidatesRequest{View: "unlinked", Lane: "all", Sort: "score", Limit: 10})
	testhelpers.AssertHumaError(t, err, 500)
}
