package community

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared"
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// isFulfillUnsupported reports whether err is the typed "fulfillment
// unsupported" error fulfillEntity returns for entity types whose catalog
// Create contracts need associations the request payload doesn't carry
// (show — its Create needs venue + artist associations the payload lacks,
// tracked as a PSY-998 follow-up; festival IS fulfilled). Callers use it
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
		// Re-validate the stored payload before fulfilling. Festival became
		// fulfillable (PSY-998) after rows could already have been queued under
		// the looser PSY-997 rules, so fulfill is a second trust boundary: a
		// malformed start/end date must surface as a 422 here, not a 500 when it
		// hits the DATE column at INSERT. The sibling branches intentionally skip
		// this — only festival parses a stored string (start_date) into a derived
		// value, so only festival's second trust boundary is load-bearing.
		if verr := communitym.ValidateEntityRequestPayload(req.EntityType, raw); verr != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, verr)
		}
		p, err := communitym.UnmarshalPayload[communitym.FestivalRequestPayload](raw)
		if err != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, err)
		}
		// edition_year: use the payload value, else fall back to the start_date's
		// calendar year (start_date is required and validated YYYY-MM-DD above,
		// so the parse succeeds; TrimSpace mirrors requireDate's own trimming).
		editionYear := p.EditionYear
		if editionYear == 0 {
			if t, perr := time.Parse("2006-01-02", strings.TrimSpace(p.StartDate)); perr == nil {
				editionYear = t.Year()
			}
		}
		if editionYear <= 0 {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, fmt.Errorf("festival edition_year must be positive"))
		}
		// series_slug is derived from the name (the payload carries no series). A
		// name with no ASCII-alphanumeric characters slugifies to "" — the same
		// result the festival's own display-slug derivation produces — which is
		// acceptable on this rarely-hit path; an admin can re-link the series via
		// the festival edit endpoint.
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
