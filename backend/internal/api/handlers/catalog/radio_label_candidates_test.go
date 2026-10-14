package catalog

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

type fakeRadioLabelCandidatesLister struct {
	got     contracts.RadioLabelCandidatesQuery
	listing *contracts.RadioLabelCandidateListing
	err     error
}

func (f *fakeRadioLabelCandidatesLister) ListLabelCandidates(_ context.Context, q contracts.RadioLabelCandidatesQuery) (*contracts.RadioLabelCandidateListing, error) {
	f.got = q
	return f.listing, f.err
}

func TestAdminListRadioLabelCandidates_PassesQuery(t *testing.T) {
	listing := &contracts.RadioLabelCandidateListing{Total: 3}
	fake := &fakeRadioLabelCandidatesLister{listing: listing}
	h := NewRadioLabelCandidatesHandler(fake)

	resp, err := h.AdminListRadioLabelCandidatesHandler(context.Background(), &AdminListRadioLabelCandidatesRequest{Lane: "human", Sort: "velocity", Limit: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := contracts.RadioLabelCandidatesQuery{Lane: contracts.RadioCandidateLaneFilterHuman, Sort: contracts.RadioLabelCandidateSortVelocity, Limit: 25}
	if fake.got != want {
		t.Errorf("got query %+v, want %+v", fake.got, want)
	}
	if resp.Body != listing {
		t.Errorf("response body is not the service listing")
	}
}

func TestAdminListRadioLabelCandidates_Defaults(t *testing.T) {
	fake := &fakeRadioLabelCandidatesLister{listing: &contracts.RadioLabelCandidateListing{}}
	h := NewRadioLabelCandidatesHandler(fake)

	if _, err := h.AdminListRadioLabelCandidatesHandler(context.Background(), &AdminListRadioLabelCandidatesRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := contracts.RadioLabelCandidatesQuery{Lane: contracts.RadioCandidateLaneFilterAll, Sort: contracts.RadioLabelCandidateSortScore, Limit: 50}
	if fake.got != want {
		t.Errorf("got query %+v, want %+v", fake.got, want)
	}
}

func TestAdminListRadioLabelCandidates_ServiceError(t *testing.T) {
	h := NewRadioLabelCandidatesHandler(&fakeRadioLabelCandidatesLister{err: errors.New("database error")})
	_, err := h.AdminListRadioLabelCandidatesHandler(context.Background(), &AdminListRadioLabelCandidatesRequest{Lane: "all", Sort: "score", Limit: 10})
	testhelpers.AssertHumaError(t, err, 500)
}
