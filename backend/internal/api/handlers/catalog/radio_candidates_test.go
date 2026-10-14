package catalog

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

type fakeRadioArtistCandidatesLister struct {
	got     contracts.RadioArtistCandidatesQuery
	listing *contracts.RadioArtistCandidateListing
	err     error
}

func (f *fakeRadioArtistCandidatesLister) ListArtistCandidates(_ context.Context, q contracts.RadioArtistCandidatesQuery) (*contracts.RadioArtistCandidateListing, error) {
	f.got = q
	return f.listing, f.err
}

func TestAdminListRadioArtistCandidates_PassesQuery(t *testing.T) {
	listing := &contracts.RadioArtistCandidateListing{Total: 3}
	fake := &fakeRadioArtistCandidatesLister{listing: listing}
	h := NewRadioCandidatesHandler(fake)

	resp, err := h.AdminListRadioArtistCandidatesHandler(context.Background(), &AdminListRadioArtistCandidatesRequest{Lane: "auto", Sort: "stations", Limit: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := contracts.RadioArtistCandidatesQuery{Lane: contracts.RadioCandidateLaneFilterAuto, Sort: contracts.RadioArtistCandidateSortStations, Limit: 25}
	if fake.got != want {
		t.Errorf("got query %+v, want %+v", fake.got, want)
	}
	if resp.Body != listing {
		t.Errorf("response body is not the service listing")
	}
}

func TestAdminListRadioArtistCandidates_Defaults(t *testing.T) {
	fake := &fakeRadioArtistCandidatesLister{listing: &contracts.RadioArtistCandidateListing{}}
	h := NewRadioCandidatesHandler(fake)

	if _, err := h.AdminListRadioArtistCandidatesHandler(context.Background(), &AdminListRadioArtistCandidatesRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := contracts.RadioArtistCandidatesQuery{Lane: contracts.RadioCandidateLaneFilterAll, Sort: contracts.RadioArtistCandidateSortScore, Limit: 50}
	if fake.got != want {
		t.Errorf("got query %+v, want %+v", fake.got, want)
	}
}

func TestAdminListRadioArtistCandidates_ServiceError(t *testing.T) {
	h := NewRadioCandidatesHandler(&fakeRadioArtistCandidatesLister{err: errors.New("database error")})
	_, err := h.AdminListRadioArtistCandidatesHandler(context.Background(), &AdminListRadioArtistCandidatesRequest{Lane: "all", Sort: "score", Limit: 10})
	testhelpers.AssertHumaError(t, err, 500)
}
