package community

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// isFulfillUnsupported reports whether err is the typed "fulfillment
// unsupported" error fulfillEntity returns when a show request has no
// admin-supplied associations (its Create needs venue + artists the payload
// lacks; the decide endpoint collects them — PSY-1037). Only the auto-approve
// create path calls this (it swallows the error — the request stays
// filed-and-approved with creation deferred); the admin decide path never
// reaches the error for shows (it pre-claim-guards) and classifies any
// fulfillment error via mapFulfillmentError instead.
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
// → 409; ShowCreateFailed → 422). Without the catalog mappers, a benign
// "already exists" conflict on the inline create-and-add path would surface as
// a 500 leaking the internal error code. Returns nil when err is none of these
// so the caller falls back to a 500.
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
	if mapped := shared.MapShowError(err); mapped != nil {
		return mapped
	}
	return nil
}

// maxShowArtistInputs caps the admin-supplied bill size on a show approve
// (PSY-1037) — large enough for any festival bill, small enough to stop a
// runaway script from flooding one CreateShow transaction.
const maxShowArtistInputs = 50

// showAssociations carries the admin-supplied venue + artists (already
// converted to the catalog contract types) from the decide endpoint to the
// show fulfillment branch (PSY-1037). nil means "none supplied" — the show
// branch then defers via FulfillUnsupported.
type showAssociations struct {
	venue   contracts.CreateShowVenue
	artists []contracts.CreateShowArtist
}

// buildShowAssociations validates + converts the decide endpoint's optional
// show-association inputs. Returns (nil, nil) when neither is supplied (a
// non-show decide, or a show approve that will defer); a Huma 422 when input
// is present but malformed — surfaced BEFORE the row is claimed, so bad input
// never produces an approved-but-unfulfilled row.
func buildShowAssociations(venue *ShowVenueInput, artists []ShowArtistInput) (*showAssociations, error) {
	if venue == nil && len(artists) == 0 {
		return nil, nil
	}
	if venue == nil || len(artists) == 0 {
		return nil, huma.Error422UnprocessableEntity("Approving a show requires both show_venue and show_artists")
	}
	// Sanity cap on the bill size — guards a buggy script/automation from
	// driving an unbounded number of artist find-or-creates in one CreateShow
	// transaction. 50 comfortably covers a festival bill.
	if len(artists) > maxShowArtistInputs {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("show_artists is capped at %d entries", maxShowArtistInputs))
	}
	if strings.TrimSpace(venue.Name) == "" || strings.TrimSpace(venue.City) == "" || strings.TrimSpace(venue.State) == "" {
		return nil, huma.Error422UnprocessableEntity("show_venue requires name, city, and state")
	}
	// Length caps mirror the venues/artists columns (name/city VARCHAR(255),
	// state VARCHAR(10), address VARCHAR(500)) — an over-long value must 422
	// here, pre-claim, not blow up at INSERT after Decide has run.
	v := contracts.CreateShowVenue{
		Name:    strings.TrimSpace(venue.Name),
		City:    strings.TrimSpace(venue.City),
		State:   strings.TrimSpace(venue.State),
		Address: strings.TrimSpace(shared.Deref(venue.Address)),
	}
	if len(v.Name) > 255 || len(v.City) > 255 || len(v.State) > 10 || len(v.Address) > 500 {
		return nil, huma.Error422UnprocessableEntity("show_venue field too long (name/city ≤255, state ≤10, address ≤500)")
	}
	out := &showAssociations{venue: v}
	for _, a := range artists {
		name := strings.TrimSpace(a.Name)
		// Name is required even when an ID is supplied: the show service's
		// duplicate-headliner pre-check matches on artist NAME, so an ID-only
		// entry would silently bypass it (the DB unique index still backstops,
		// but with a generic error instead of the readable conflict message).
		if name == "" {
			return nil, huma.Error422UnprocessableEntity("Each show_artists entry requires a name")
		}
		if len(name) > 255 {
			return nil, huma.Error422UnprocessableEntity("show_artists name must be 255 characters or fewer")
		}
		out.artists = append(out.artists, contracts.CreateShowArtist{
			ID:          a.ID,
			Name:        name,
			IsHeadliner: a.IsHeadliner,
		})
	}
	return out, nil
}

// parseShowEventDate parses a show payload's event_date. An RFC3339 value is
// taken as-is; a date-only value (YYYY-MM-DD) is anchored at 20:00 in the
// state's assumed IANA zone (utils.EventLocation, which defaults unknown/empty
// states per its documented fallback) — the same "date-only means that
// evening, venue-local" convention the ingest CLI and the PSY-987 re-anchor
// logic use, so a date-only show doesn't render as the previous day.
func parseShowEventDate(value, state string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t, nil
	}
	if d, err := time.Parse("2006-01-02", trimmed); err == nil {
		loc := utils.EventLocation(nil, state)
		return time.Date(d.Year(), d.Month(), d.Day(), 20, 0, 0, 0, loc), nil
	}
	return time.Time{}, fmt.Errorf("show event_date %q is not an RFC3339 timestamp or YYYY-MM-DD date", trimmed)
}

// PSY-997: fulfillment dispatcher — turns an approved entity_request's typed
// payload into a real catalog entity via the narrow fulfiller interface.
//
// Per-type mapping is isolated here (the volatile part: catalog create
// contracts evolve independently of the request payloads). The stored payload
// is re-validated up front (fulfill is a second trust boundary — a row may have
// been queued before a validation rule existed), then each branch decodes it
// with the typed UnmarshalPayload[T] guard and maps the fields onto the catalog
// Create*Request.
//
// Field-mapping note: every payload field now maps onto its catalog Create
// contract (PSY-1038 closed the prior fidelity gap — artist image_url +
// bandcamp_embed_url, venue description/image_url, label image_url all carry
// through to the created entity). URL fields (image_url, bandcamp_embed_url,
// cover_art_url, festival website/ticket_url/flyer_url) are scheme- and
// length-validated by ValidateEntityRequestPayload, which the re-validation
// above re-runs — so the per-branch mapping can trust the stored values.
//
// festival is fulfilled by deriving the two fields its create contract needs
// beyond the payload: series_slug (from the name) and edition_year (from the
// start_date when the payload omits it). See the festival branch (PSY-998).
//
// show is fulfilled only when the admin supplied venue + artist associations
// at approve time (showAssoc != nil — collected by the decide endpoint,
// PSY-1037); the payload alone lacks them. Without associations (the
// auto-approve create path) the show branch returns a typed
// FulfillUnsupported error and the request defers gracefully.
func (h *EntityRequestHandler) fulfillEntity(req *communitym.EntityRequest, showAssoc *showAssociations) (uint, error) {
	if req.Payload == nil {
		return 0, apperrors.ErrEntityRequestEmptyPayload(req.EntityType)
	}
	raw := *req.Payload

	// Re-validate the stored payload before fulfilling. The request queue is a
	// store-now/fulfill-later boundary, so a row may have been queued before a
	// given rule existed (e.g. URL scheme/length checks added in PSY-1038, or a
	// crafted request that predates them). Re-running the boundary validation
	// here rejects malformed stored data instead of letting a hostile URL ride
	// onto the created entity or an over-long value 500 at INSERT.
	//
	// show is excluded only when no associations were supplied: the show branch
	// then defers via the unsupported stub (the auto-approve path swallows
	// that), and a malformed legacy payload must not hard-error ahead of the
	// deferral. When the admin DID supply associations, show is about to be
	// fulfilled, so its stored payload is re-validated like every other type.
	if req.EntityType != communitym.EntityRequestShow || showAssoc != nil {
		if verr := communitym.ValidateEntityRequestPayload(req.EntityType, raw); verr != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, verr)
		}
	}

	switch req.EntityType {
	case communitym.EntityRequestArtist:
		p, err := communitym.UnmarshalPayload[communitym.ArtistRequestPayload](raw)
		if err != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, err)
		}
		created, err := h.fulfiller.CreateArtist(&contracts.CreateArtistRequest{
			Name:             p.Name,
			City:             p.City,
			State:            p.State,
			Country:          p.Country,
			Description:      p.Description,
			ImageURL:         p.ImageURL,
			BandcampEmbedURL: p.BandcampEmbedURL,
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
			Name:        p.Name,
			City:        p.City,
			State:       p.State,
			Address:     p.Address,
			Country:     p.Country,
			Zipcode:     p.Zipcode,
			Description: p.Description,
			ImageURL:    p.ImageURL,
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
			ImageURL:    p.ImageURL,
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
		// Show fulfillment needs ≥1 venue + ≥1 artist that the payload doesn't
		// carry. The admin decide endpoint collects them (PSY-1037); without
		// them (auto-approve path) the request defers via the typed 422.
		if showAssoc == nil {
			return 0, apperrors.ErrEntityRequestFulfillUnsupported(req.EntityType)
		}
		p, err := communitym.UnmarshalPayload[communitym.ShowRequestPayload](raw)
		if err != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, err)
		}
		// event_date: RFC3339 is taken as-is; a date-only value is anchored at
		// 20:00 in the state's assumed zone (utils.EventLocation — the same
		// convention the ingest CLI and the PSY-987 re-anchor logic use), so a
		// date-only show doesn't render as the previous evening in venue-local
		// time. The format was validated by ValidateEntityRequestPayload above,
		// so one of the two parses succeeds.
		eventDate, perr := parseShowEventDate(p.EventDate, shared.Deref(p.State))
		if perr != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, perr)
		}
		created, err := h.fulfiller.CreateShow(&contracts.CreateShowRequest{
			Title:          p.Title,
			EventDate:      eventDate,
			City:           shared.Deref(p.City),
			State:          shared.Deref(p.State),
			Price:          p.Price,
			AgeRequirement: shared.Deref(p.AgeRequirement),
			Description:    shared.Deref(p.Description),
			TicketURL:      shared.Deref(p.TicketURL),
			ImageURL:       p.ImageURL,
			Venues:         []contracts.CreateShowVenue{showAssoc.venue},
			Artists:        showAssoc.artists,
			// Attribution goes to the requester; the approving admin makes the
			// show land approved (and any new venue admin-verified).
			SubmittedByUserID: &req.RequesterID,
			SubmitterIsAdmin:  true,
		})
		if err != nil {
			// Wrap as the typed SHOW_CREATE_FAILED the direct create handler
			// uses, so a benign create conflict (duplicate headliner at the
			// same venue/date) maps to 422 instead of a raw 500.
			return 0, apperrors.ErrShowCreateFailed(err)
		}
		return created.ID, nil

	default:
		// IsValidEntityRequestType is enforced on create, so this is
		// defense-in-depth against a future entity_type added to the registry
		// without a fulfillment branch.
		return 0, apperrors.ErrEntityRequestFulfillUnsupported(req.EntityType)
	}
}
