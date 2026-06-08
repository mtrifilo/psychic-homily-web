package community

import (
	"errors"
	"strconv"
	"strings"

	"psychic-homily-backend/internal/api/handlers/shared"
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
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
// the create (e.g. ArtistExists / LabelExists / ReleaseExists / FestivalExists
// → 409). Without the catalog mappers, a benign "already exists" conflict on
// the inline create-and-add path would surface as a 500 leaking the internal
// error code. Returns nil when err is none of these so the caller falls back
// to a 500.
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
	if mapped := shared.MapFestivalError(err); mapped != nil {
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
// festival is fulfilled by deriving the two fields its create contract needs
// beyond the payload: series_slug (from the name) and edition_year (from the
// start_date when the payload omits it). See the festival branch (PSY-998).
//
// show remains deliberately unsupported: CreateShowRequest requires ≥1 venue +
// ≥1 artist (with positions) that the payload doesn't carry and that an admin
// must supply at approve time. Approving a show returns a typed
// FulfillUnsupported error (422) so the admin creates it manually; wiring the
// admin association-resolution step is a PSY-998 follow-up.
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

	case communitym.EntityRequestFestival:
		p, err := communitym.UnmarshalPayload[communitym.FestivalRequestPayload](raw)
		if err != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, err)
		}
		// Derive the two fields CreateFestival requires that the payload does
		// not carry: series_slug (slugified from the name — a recurring series
		// can be re-linked later via the festival edit endpoint) and a non-zero
		// edition_year. The request schema doesn't require edition_year, so fall
		// back to the start_date's calendar year (start_date is required and
		// stored YYYY-MM-DD); leaving it 0 only if start_date can't be parsed.
		editionYear := p.EditionYear
		if editionYear == 0 {
			if y, perr := strconv.Atoi(strings.SplitN(p.StartDate, "-", 2)[0]); perr == nil {
				editionYear = y
			}
		}
		created, err := h.fulfiller.CreateFestival(&contracts.CreateFestivalRequest{
			Name:         p.Name,
			SeriesSlug:   utils.GenerateSlug(p.Name),
			EditionYear:  editionYear,
			Description:  p.Description,
			LocationName: p.LocationName,
			City:         p.City,
			State:        p.State,
			Country:      p.Country,
			StartDate:    p.StartDate,
			EndDate:      p.EndDate,
			Website:      p.Website,
			TicketURL:    p.TicketURL,
			FlyerURL:     p.FlyerURL,
		})
		if err != nil {
			return 0, err
		}
		return created.ID, nil

	case communitym.EntityRequestShow:
		// Show fulfillment needs ≥1 venue + ≥1 artist (with positions) that the
		// request payload doesn't carry — an admin must supply them at approve
		// time. Deferred to a PSY-998 follow-up; returns a typed 422 so the
		// admin creates the show manually meanwhile.
		return 0, apperrors.ErrEntityRequestFulfillUnsupported(req.EntityType)

	default:
		// IsValidEntityRequestType is enforced on create, so this is
		// defense-in-depth against a future entity_type added to the registry
		// without a fulfillment branch.
		return 0, apperrors.ErrEntityRequestFulfillUnsupported(req.EntityType)
	}
}
