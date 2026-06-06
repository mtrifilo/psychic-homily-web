package community

import (
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-997: EntityRequestFulfiller composes the per-entity catalog create
// services behind the narrow contracts.EntityRequestFulfillerInterface used by
// the decide-approve handler. It is a pure adapter — each method delegates to
// the matching catalog service — so the handler depends on one small interface
// instead of four fat catalog service interfaces, and fulfillment can be
// mocked in handler tests via the generated MockEntityRequestFulfiller.
type EntityRequestFulfiller struct {
	artist  contracts.ArtistServiceInterface
	venue   contracts.VenueServiceInterface
	label   contracts.LabelServiceInterface
	release contracts.ReleaseServiceInterface
}

// NewEntityRequestFulfiller wires the catalog create services into the adapter.
func NewEntityRequestFulfiller(
	artist contracts.ArtistServiceInterface,
	venue contracts.VenueServiceInterface,
	label contracts.LabelServiceInterface,
	release contracts.ReleaseServiceInterface,
) *EntityRequestFulfiller {
	return &EntityRequestFulfiller{artist: artist, venue: venue, label: label, release: release}
}

func (f *EntityRequestFulfiller) CreateArtist(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
	return f.artist.CreateArtist(req)
}

func (f *EntityRequestFulfiller) CreateVenue(req *contracts.CreateVenueRequest, isAdmin bool) (*contracts.VenueDetailResponse, error) {
	return f.venue.CreateVenue(req, isAdmin)
}

func (f *EntityRequestFulfiller) CreateLabel(req *contracts.CreateLabelRequest) (*contracts.LabelDetailResponse, error) {
	return f.label.CreateLabel(req)
}

func (f *EntityRequestFulfiller) CreateRelease(req *contracts.CreateReleaseRequest) (*contracts.ReleaseDetailResponse, error) {
	return f.release.CreateRelease(req)
}

// Compile-time assertion the adapter satisfies the fulfiller interface.
var _ contracts.EntityRequestFulfillerInterface = (*EntityRequestFulfiller)(nil)
