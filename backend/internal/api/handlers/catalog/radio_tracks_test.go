package catalog

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

type fakeRadioTracksLister struct {
	gotSort  contracts.RadioTrackSort
	gotLimit int
	listing  *contracts.RadioTrackListing
	err      error
}

func (f *fakeRadioTracksLister) ListTopRadioTracks(_ context.Context, sort contracts.RadioTrackSort, limit int) (*contracts.RadioTrackListing, error) {
	f.gotSort, f.gotLimit = sort, limit
	return f.listing, f.err
}

func TestAdminListRadioTracks_PassesSortAndLimit(t *testing.T) {
	listing := &contracts.RadioTrackListing{TotalTracks: 3, UnkeyedPlays: 2}
	fake := &fakeRadioTracksLister{listing: listing}
	h := NewRadioTracksHandler(fake)

	resp, err := h.AdminListRadioTracksHandler(context.Background(), &AdminListRadioTracksRequest{Sort: "station_families", Limit: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.gotSort != contracts.RadioTrackSortStationFamilies || fake.gotLimit != 25 {
		t.Errorf("got sort=%q limit=%d, want station_families 25", fake.gotSort, fake.gotLimit)
	}
	if resp.Body != listing {
		t.Errorf("response body is not the service listing")
	}
}

func TestAdminListRadioTracks_Defaults(t *testing.T) {
	fake := &fakeRadioTracksLister{listing: &contracts.RadioTrackListing{}}
	h := NewRadioTracksHandler(fake)

	if _, err := h.AdminListRadioTracksHandler(context.Background(), &AdminListRadioTracksRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.gotSort != contracts.RadioTrackSortPlays || fake.gotLimit != 50 {
		t.Errorf("got sort=%q limit=%d, want plays 50", fake.gotSort, fake.gotLimit)
	}
}

func TestAdminListRadioTracks_ServiceError(t *testing.T) {
	h := NewRadioTracksHandler(&fakeRadioTracksLister{err: errors.New("database error")})
	_, err := h.AdminListRadioTracksHandler(context.Background(), &AdminListRadioTracksRequest{Sort: "plays", Limit: 10})
	testhelpers.AssertHumaError(t, err, 500)
}
