package community

import (
	"errors"

	"psychic-homily-backend/internal/api/handlers/shared"
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// isFulfillUnsupported reports whether err is the typed "fulfillment
// unsupported" error fulfillEntity returns for entity types whose catalog
// Create contracts need associations the request payload doesn't carry
// (show / festival; association-resolution tracked in PSY-998). Callers use it
// to decide whether the error is fatal: the admin decide path surfaces it (422
// → admin creates the entity manually), while the auto-approve create path
// swallows it (the request is filed-and-approved; immediate creation is just
// deferred).
func isFulfillUnsupported(err error) bool {
	var reqErr *apperrors.EntityRequestError
	if errors.As(err, &reqErr) {
		return reqErr.Code == apperrors.CodeEntityRequestFulfillUnsupported
	}
	return false
}

// mapFulfillmentError maps an error from fulfilling a request's payload into a
// catalog entity to the right HTTP status. fulfillEntity surfaces two error
// families: request-level errors (FulfillUnsupported → 422, payload corruption
// → 500, via MapEntityRequestError) and catalog-service errors bubbled up from
// the create (e.g. ArtistExists / LabelExists / ReleaseExists → 409). Without
// the catalog mappers, a benign "already exists" conflict on the inline
// create-and-add path would surface as a 500 leaking the internal error code.
// Returns nil when err is none of these so the caller falls back to a 500.
func mapFulfillmentError(err error) error {
	if mapped := shared.MapEntityRequestError(err); mapped != nil {
		return mapped
	}
	if mapped := shared.MapArtistError(err); mapped != nil {
		return mapped
	}
	if mapped := shared.MapVenueError(err); mapped != nil {
		return mapped
	}
	if mapped := shared.MapLabelError(err); mapped != nil {
		return mapped
	}
	if mapped := shared.MapReleaseError(err); mapped != nil {
		return mapped
	}
	return nil
}

// PSY-997: fulfillment dispatcher — turns an approved entity_request's typed
// payload into a real catalog entity via the narrow fulfiller interface.
//
// Per-type mapping is isolated here (the volatile part: catalog create
// contracts evolve independently of the request payloads). Each branch decodes
// the payload with the typed UnmarshalPayload[T] guard (fails loud on schema
// drift) and maps the user-supplied fields onto the catalog Create*Request.
//
// Field-mapping note: a few payload fields have no slot on the catalog Create
// contracts today (artist image_url + bandcamp_embed_url, venue
// description/image_url, label image_url). They are NOT silently dropped from
// the system — they remain on the persisted request row — but the created
// entity does not carry them until a follow-up adds those fields to the Create
// contracts or a post-create update. This is a known fidelity gap, not data
// loss of the request itself.
//
// show + festival are deliberately unsupported: CreateShowRequest requires
// venues + artists and CreateFestivalRequest requires series_slug, neither of
// which the payloads carry. Approving those returns a typed
// FulfillUnsupported error (422) so the admin creates them manually.
func (h *EntityRequestHandler) fulfillEntity(req *communitym.EntityRequest) (uint, error) {
	if req.Payload == nil {
		return 0, apperrors.ErrEntityRequestEmptyPayload(req.EntityType)
	}
	raw := *req.Payload

	switch req.EntityType {
	case communitym.EntityRequestArtist:
		p, err := communitym.UnmarshalPayload[communitym.ArtistRequestPayload](raw)
		if err != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, err)
		}
		created, err := h.fulfiller.CreateArtist(&contracts.CreateArtistRequest{
			Name:        p.Name,
			City:        p.City,
			State:       p.State,
			Country:     p.Country,
			Description: p.Description,
		})
		if err != nil {
			return 0, err
		}
		return created.ID, nil

	case communitym.EntityRequestVenue:
		p, err := communitym.UnmarshalPayload[communitym.VenueRequestPayload](raw)
		if err != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, err)
		}
		// Admin is approving, so create as an admin-verified venue.
		created, err := h.fulfiller.CreateVenue(&contracts.CreateVenueRequest{
			Name:    p.Name,
			City:    p.City,
			State:   p.State,
			Address: p.Address,
			Country: p.Country,
			Zipcode: p.Zipcode,
		}, true)
		if err != nil {
			return 0, err
		}
		return created.ID, nil

	case communitym.EntityRequestLabel:
		p, err := communitym.UnmarshalPayload[communitym.LabelRequestPayload](raw)
		if err != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, err)
		}
		created, err := h.fulfiller.CreateLabel(&contracts.CreateLabelRequest{
			Name:        p.Name,
			City:        p.City,
			State:       p.State,
			Country:     p.Country,
			FoundedYear: p.FoundedYear,
			Description: p.Description,
		})
		if err != nil {
			return 0, err
		}
		return created.ID, nil

	case communitym.EntityRequestRelease:
		p, err := communitym.UnmarshalPayload[communitym.ReleaseRequestPayload](raw)
		if err != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, err)
		}
		releaseType := ""
		if p.ReleaseType != nil {
			releaseType = *p.ReleaseType
		}
		created, err := h.fulfiller.CreateRelease(&contracts.CreateReleaseRequest{
			Title:       p.Title,
			ReleaseType: releaseType,
			ReleaseYear: p.ReleaseYear,
			ReleaseDate: p.ReleaseDate,
			CoverArtURL: p.CoverArtURL,
			Description: p.Description,
		})
		if err != nil {
			return 0, err
		}
		return created.ID, nil

	case communitym.EntityRequestShow, communitym.EntityRequestFestival:
		return 0, apperrors.ErrEntityRequestFulfillUnsupported(req.EntityType)

	default:
		// IsValidEntityRequestType is enforced on create, so this is
		// defense-in-depth against a future entity_type added to the registry
		// without a fulfillment branch.
		return 0, apperrors.ErrEntityRequestFulfillUnsupported(req.EntityType)
	}
}
