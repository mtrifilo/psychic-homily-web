package community

import (
	"context"
	"encoding/json"
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

// validatePayloadImageURL runs an entity-request payload's image_url through the
// same SSRF host guard the direct catalog endpoints apply (PSY-1675). It is the
// one place the queue's two decision points — create, and approve — agree on
// the rule, so neither can drift.
//
// It returns a huma 422 in every case, including the extraction failures
// (unknown entity type, undecodable payload) that PayloadImageURL surfaces as
// plain errors — those are unreachable from both callers, which run
// IsValidEntityRequestType and ValidateEntityRequestPayload first, but a plain
// error escaping to huma would render as a 500 blaming the server for a bad
// payload, so it is mapped here rather than left to luck.
//
// NOT called from fulfillEntity. Fulfillment happens AFTER Decide has
// atomically claimed the row, so a rejection there would leave an
// approved-but-unfulfilled row that no decide call can re-process and no
// endpoint can edit — the request would be strandable by a hostile flyer. The
// approve path checks pre-claim instead (AdminDecideEntityRequestHandler),
// which is where the show-associations guard already lives for the same reason.
func validatePayloadImageURL(ctx context.Context, entityType string, raw json.RawMessage) error {
	imageURL, err := communitym.PayloadImageURL(entityType, raw)
	if err != nil {
		return huma.Error422UnprocessableEntity(
			fmt.Sprintf("Invalid payload for %s: %v", entityType, err),
		)
	}
	return shared.ValidateImageURL(ctx, imageURL)
}

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
	billIsCurated := false
	for i, a := range artists {
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
		setType, serr := curatedShowArtistSetType(i, a.SetType)
		if serr != nil {
			return nil, serr
		}
		if setType != nil {
			billIsCurated = true
		}
		out.artists = append(out.artists, contracts.CreateShowArtist{
			ID:          a.ID,
			Name:        name,
			IsHeadliner: a.IsHeadliner,
			SetType:     setType,
		})
	}
	if billIsCurated {
		suppressPositionInference(out.artists)
	}
	return out, nil
}

// suppressPositionInference pins is_headliner=false on the acts the admin left
// uncurated, and is called only once SOME act on the bill states a role.
//
// resolveArtistRole reads position 0 as the headliner, but only as a last resort
// for an act that carries no other signal. That fallback is right for a bill
// nobody described and wrong the moment somebody does. Without this, an admin
// who marks the SECOND act "headliner" and leaves the first alone gets TWO rows
// with set_type='headliner', and every headliner reader in the codebase resolves
// `set_type = 'headliner' ORDER BY position ASC LIMIT 1`, so the act nobody
// designated wins and the curated one is discarded. That silently corrupts the
// single fact PSY-1705 exists to record.
//
// The rule this encodes: a stated bill is a complete statement, so first-in-list
// is not a second opinion. It is the same guard ConfirmShowImport applies for
// the same reason, and the direct show-create handler gets it for free because
// initializeArtist pins the flag false on every act before Resolve runs.
//
// Acts that state their own set_type or is_headliner are left untouched, and a
// bill where NOBODY states anything is untouched as a whole, so this cannot
// change the outcome for any caller that predates set_type on this endpoint.
func suppressPositionInference(artists []contracts.CreateShowArtist) {
	for i := range artists {
		if artists[i].SetType != nil || artists[i].IsHeadliner != nil {
			continue
		}
		noPositionInference := false
		artists[i].IsHeadliner = &noPositionInference
	}
}

// curatedShowArtistSetType validates one admin-supplied bill role (PSY-1705)
// and returns the value to hand the show service, or nil when the admin did not
// curate this act's slot.
//
// ONLY an absent key means "not curated". A present value must be exactly one
// of the vocabulary's members, so "" and "   " are rejected rather than read as
// absent. That is a deliberate choice to agree character-for-character with the
// generated OpenAPI enum on the field, which rejects any PRESENT value outside
// the list: were this laxer, a client sending "" for "slot unknown" would be
// 422'd by the schema while an in-process caller sending the same thing got a
// silent 'performer', and the tests, which call handlers directly and never see
// the schema, would certify the behavior nobody over HTTP can actually get.
// Callers that mean "slot unknown" omit the key, and the show service then
// applies contracts.SetTypeDefault ('performer').
//
// Nothing here ever infers a role from bill position. That inference is the
// exact defect the PSY-1673 vocabulary removed; see suppressPositionInference
// for the one place bill order still has any say.
//
// Over HTTP the schema enum rejects a bad role before this runs, so this is the
// floor beneath it for in-process callers. It is NOT redundant with the show
// service's validateShowArtistSetTypes, though: that one runs inside
// fulfillment, which on the decide path is after the row has been claimed, so
// rejecting there would turn a typo into an approved-but-unfulfilled row that
// Decide can no longer re-process (it only claims PENDING rows). Such a row is
// recoverable, via the PSY-1088 rescue endpoint, but only by a second admin
// action on a request that now reads as approved with nothing created. Both
// callers run buildShowAssociations pre-claim, alongside the venue/name/
// image_url checks that live there for the same reason.
func curatedShowArtistSetType(index int, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if !contracts.IsValidSetType(*value) {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf(
			"show_artists[%d].set_type %q is not a valid set type (allowed: %s)",
			index, *value, contracts.SetTypeVocabularyCSV(),
		))
	}
	return value, nil
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

// parseOptionalShowTime parses an optional doors_at/music_at payload value.
// Unlike event_date these are RFC3339-only: a date-only value would have to
// invent a time of day, which is the sole thing these fields carry. Absent or
// blank stays nil; anything else that fails to parse is an error rather than a
// silent drop, since fulfillment is a second trust boundary over a stored blob.
func parseOptionalShowTime(field string, value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, fmt.Errorf("show %s %q is not an RFC3339 timestamp", field, trimmed)
	}
	utc := t.UTC()
	return &utc, nil
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
func (h *EntityRequestHandler) fulfillEntity(ctx context.Context, req *communitym.EntityRequest, showAssoc *showAssociations) (uint, error) {
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
		doorsAt, perr := parseOptionalShowTime("doors_at", p.DoorsAt)
		if perr != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, perr)
		}
		musicAt, perr := parseOptionalShowTime("music_at", p.MusicAt)
		if perr != nil {
			return 0, apperrors.ErrEntityRequestPayloadInvalid(req.EntityType, perr)
		}
		created, err := h.fulfiller.CreateShow(&contracts.CreateShowRequest{
			Title:          p.Title,
			EventDate:      eventDate,
			DoorsAt:        doorsAt,
			MusicAt:        musicAt,
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
