package community

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"psychic-homily-backend/internal/utils"
)

// PSY-869: typed payload schemas for the polymorphic entity_requests table.
//
// Architectural decision (LOCKED 2026-05-26): polymorphism lives in the
// table (one row shape, JSONB payload, entity_type discriminator); typing
// lives HERE in Go — one struct per entity_type. The DB stores the payload
// opaquely; this file is the single source of truth for what each
// entity_type's payload looks like.
//
// These payloads describe the USER-SUPPLIED fields for creating an entity —
// the data a requester provides. Server-controlled fields (slug, provenance,
// approval-workflow status, verified flags, IDs) are intentionally NOT part
// of the payload: they're set when the request is fulfilled into a real
// catalog row, not when it's requested.

// EntityRequestEntityType enumerates the entity_type discriminator values.
// MUST stay aligned with:
//   - the CHECK constraint in the entity_requests migration, and
//   - payloadRegistry below (the CI parity check
//     scripts/check_entity_request_payloads.sh fails the build on drift).
const (
	EntityRequestArtist   = "artist"
	EntityRequestRelease  = "release"
	EntityRequestLabel    = "label"
	EntityRequestShow     = "show"
	EntityRequestVenue    = "venue"
	EntityRequestFestival = "festival"
)

// EntityRequestPayload is the marker interface implemented by every
// per-entity payload struct. It exists purely to make the payload registry
// and the generic UnmarshalPayload helper type-safe and discoverable.
type EntityRequestPayload interface {
	// entityRequestType returns the entity_type discriminator the payload
	// belongs to. Unexported so the set of payload types is closed to this
	// package — a new entity_type must add a struct here, which the CI
	// parity check enforces against the migration's CHECK constraint.
	entityRequestType() string
}

// ArtistRequestPayload carries the user-supplied fields to create an artist.
type ArtistRequestPayload struct {
	Name             string  `json:"name"`
	City             *string `json:"city,omitempty"`
	State            *string `json:"state,omitempty"`
	Country          *string `json:"country,omitempty"`
	Description      *string `json:"description,omitempty"`
	ImageURL         *string `json:"image_url,omitempty"`
	BandcampEmbedURL *string `json:"bandcamp_embed_url,omitempty"`
}

func (ArtistRequestPayload) entityRequestType() string { return EntityRequestArtist }

// ReleaseRequestPayload carries the user-supplied fields to create a release.
type ReleaseRequestPayload struct {
	Title       string  `json:"title"`
	ReleaseType *string `json:"release_type,omitempty"` // 'lp', 'ep', 'single', etc. (defaults applied at fulfillment)
	ReleaseYear *int    `json:"release_year,omitempty"`
	ReleaseDate *string `json:"release_date,omitempty"` // YYYY-MM-DD
	CoverArtURL *string `json:"cover_art_url,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (ReleaseRequestPayload) entityRequestType() string { return EntityRequestRelease }

// LabelRequestPayload carries the user-supplied fields to create a label.
type LabelRequestPayload struct {
	Name        string  `json:"name"`
	City        *string `json:"city,omitempty"`
	State       *string `json:"state,omitempty"`
	Country     *string `json:"country,omitempty"`
	FoundedYear *int    `json:"founded_year,omitempty"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
}

func (LabelRequestPayload) entityRequestType() string { return EntityRequestLabel }

// ShowRequestPayload carries the user-supplied fields to create a show.
type ShowRequestPayload struct {
	Title          string   `json:"title"`
	EventDate      string   `json:"event_date"` // RFC3339 / YYYY-MM-DD; parsed at fulfillment
	City           *string  `json:"city,omitempty"`
	State          *string  `json:"state,omitempty"`
	Price          *float64 `json:"price,omitempty"`
	AgeRequirement *string  `json:"age_requirement,omitempty"`
	Description    *string  `json:"description,omitempty"`
	TicketURL      *string  `json:"ticket_url,omitempty"`
	ImageURL       *string  `json:"image_url,omitempty"`
}

func (ShowRequestPayload) entityRequestType() string { return EntityRequestShow }

// VenueRequestPayload carries the user-supplied fields to create a venue.
// City + State are required on the catalog model, so they are non-pointer here.
type VenueRequestPayload struct {
	Name        string  `json:"name"`
	City        string  `json:"city"`
	State       string  `json:"state"`
	Address     *string `json:"address,omitempty"`
	Country     *string `json:"country,omitempty"`
	Zipcode     *string `json:"zipcode,omitempty"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
}

func (VenueRequestPayload) entityRequestType() string { return EntityRequestVenue }

// FestivalRequestPayload carries the user-supplied fields to create a festival.
type FestivalRequestPayload struct {
	Name         string  `json:"name"`
	EditionYear  int     `json:"edition_year"`
	StartDate    string  `json:"start_date"` // YYYY-MM-DD
	EndDate      string  `json:"end_date"`   // YYYY-MM-DD
	Description  *string `json:"description,omitempty"`
	LocationName *string `json:"location_name,omitempty"`
	City         *string `json:"city,omitempty"`
	State        *string `json:"state,omitempty"`
	Country      *string `json:"country,omitempty"`
	Website      *string `json:"website,omitempty"`
	TicketURL    *string `json:"ticket_url,omitempty"`
	FlyerURL     *string `json:"flyer_url,omitempty"`
}

func (FestivalRequestPayload) entityRequestType() string { return EntityRequestFestival }

// payloadRegistry is the authoritative map from entity_type discriminator to
// a zero-value of its payload struct. It is the runtime mirror of the
// migration's CHECK constraint and the anchor for the CI parity check
// (scripts/check_entity_request_payloads.sh greps the keys of this literal
// against the CHECK constraint's IN-list). Adding an entity_type WITHOUT
// adding it here is the exact drift the CI check exists to block.
var payloadRegistry = map[string]EntityRequestPayload{
	EntityRequestArtist:   ArtistRequestPayload{},
	EntityRequestRelease:  ReleaseRequestPayload{},
	EntityRequestLabel:    LabelRequestPayload{},
	EntityRequestShow:     ShowRequestPayload{},
	EntityRequestVenue:    VenueRequestPayload{},
	EntityRequestFestival: FestivalRequestPayload{},
}

// IsValidEntityRequestType reports whether entityType has a registered payload
// struct. Use at the trust boundary (request creation) before persisting.
func IsValidEntityRequestType(entityType string) bool {
	_, ok := payloadRegistry[entityType]
	return ok
}

// ValidateEntityRequestPayload checks that raw decodes cleanly into the typed
// struct for entityType AND that the type's required field(s) are present.
// PSY-997: called at the queue-create trust boundary so a malformed payload
// (unknown fields, wrong shape, missing required Name/Title) is rejected with a
// 422 at submit time instead of being stored as junk in the queue and failing
// confusingly when an admin later approves it.
//
// Returns nil for a well-formed payload. The error is descriptive of the
// decode/required-field failure (it does not wrap an EntityRequestError —
// the caller maps it to a 422). entityType MUST be a registered type
// (IsValidEntityRequestType) — an unknown type returns an error.
func ValidateEntityRequestPayload(entityType string, raw json.RawMessage) error {
	switch entityType {
	case EntityRequestArtist:
		p, err := UnmarshalPayload[ArtistRequestPayload](raw)
		if err != nil {
			return err
		}
		if err := requireField("artist", "name", p.Name); err != nil {
			return err
		}
		if err := optionalHTTPURL("artist", "image_url", p.ImageURL, maxRequestURLLen); err != nil {
			return err
		}
		if err := optionalMaxLen("artist", "description", p.Description, maxRequestDescriptionLen); err != nil {
			return err
		}
		// Scheme-validate the embed URL (the security floor — keeps a hostile
		// scheme off the created artist). This is intentionally looser than the
		// direct artist endpoint's bandcamp.com/album|track domain check
		// (isValidBandcampURL): that check is unexported in the catalog handler
		// and is a content-quality gate, not a safety one, so requiring it here
		// would risk rejecting otherwise-valid extracted embeds.
		return optionalHTTPURL("artist", "bandcamp_embed_url", p.BandcampEmbedURL, maxRequestURLLen)
	case EntityRequestRelease:
		p, err := UnmarshalPayload[ReleaseRequestPayload](raw)
		if err != nil {
			return err
		}
		if err := requireField("release", "title", p.Title); err != nil {
			return err
		}
		if err := optionalHTTPURL("release", "cover_art_url", p.CoverArtURL, maxRequestURLLen); err != nil {
			return err
		}
		return optionalMaxLen("release", "description", p.Description, maxRequestDescriptionLen)
	case EntityRequestLabel:
		p, err := UnmarshalPayload[LabelRequestPayload](raw)
		if err != nil {
			return err
		}
		if err := requireField("label", "name", p.Name); err != nil {
			return err
		}
		if err := optionalHTTPURL("label", "image_url", p.ImageURL, maxRequestURLLen); err != nil {
			return err
		}
		return optionalMaxLen("label", "description", p.Description, maxRequestDescriptionLen)
	case EntityRequestVenue:
		p, err := UnmarshalPayload[VenueRequestPayload](raw)
		if err != nil {
			return err
		}
		if err := requireField("venue", "name", p.Name); err != nil {
			return err
		}
		if err := requireField("venue", "city", p.City); err != nil {
			return err
		}
		if err := requireField("venue", "state", p.State); err != nil {
			return err
		}
		if err := optionalHTTPURL("venue", "image_url", p.ImageURL, maxRequestURLLen); err != nil {
			return err
		}
		return optionalMaxLen("venue", "description", p.Description, maxRequestDescriptionLen)
	case EntityRequestShow:
		p, err := UnmarshalPayload[ShowRequestPayload](raw)
		if err != nil {
			return err
		}
		if err := requireField("show", "title", p.Title); err != nil {
			return err
		}
		// event_date drives the created show's timestamp (PSY-1037): RFC3339 is
		// used as-is, a date-only value is anchored at 20:00 venue-local at
		// fulfillment — so it must parse as one of the two here (422), not fail
		// at fulfill (500).
		if err := requireDateTimeOrDate("show", "event_date", p.EventDate); err != nil {
			return err
		}
		// Shows are fulfillable when the admin supplies associations (PSY-1037),
		// so the payload's fields ride onto a created show — validate them with
		// the SAME caps the direct show-create handler enforces (title ≤255,
		// age_requirement ≤50, price 0–10000, description ≤5000; image_url
		// VARCHAR(2048), ticket_url VARCHAR(500)). A value that slipped past
		// here would 500 at INSERT after the row is claimed, leaving an
		// approved-but-unfulfilled row no decide call can re-process.
		if err := optionalMaxLen("show", "title", &p.Title, maxRequestTitleLen); err != nil {
			return err
		}
		if err := optionalMaxLen("show", "age_requirement", p.AgeRequirement, maxRequestAgeLen); err != nil {
			return err
		}
		if err := optionalMaxLen("show", "city", p.City, maxRequestCityLen); err != nil {
			return err
		}
		if err := optionalMaxLen("show", "state", p.State, maxRequestStateLen); err != nil {
			return err
		}
		if p.Price != nil && (*p.Price < 0 || *p.Price > maxRequestPrice) {
			return fmt.Errorf("show payload: price must be between 0 and %d", maxRequestPrice)
		}
		if err := optionalHTTPURL("show", "image_url", p.ImageURL, maxRequestURLLen); err != nil {
			return err
		}
		if err := optionalHTTPURL("show", "ticket_url", p.TicketURL, maxRequestShortURLLen); err != nil {
			return err
		}
		return optionalMaxLen("show", "description", p.Description, maxRequestDescriptionLen)
	case EntityRequestFestival:
		p, err := UnmarshalPayload[FestivalRequestPayload](raw)
		if err != nil {
			return err
		}
		if err := requireField("festival", "name", p.Name); err != nil {
			return err
		}
		// edition_year is optional (0 → derived from start_date at fulfill), but a
		// negative value is never valid — reject it at the boundary (422) instead
		// of letting the fulfiller surface it as a server-side 500.
		if p.EditionYear < 0 {
			return fmt.Errorf("festival payload: edition_year must not be negative")
		}
		// start_date/end_date feed a DATE column, and start_date drives the
		// derived edition_year (PSY-998) — reject a malformed date here (422)
		// rather than letting it fail at INSERT or yield a wrong edition_year.
		if err := requireDate("festival", "start_date", p.StartDate); err != nil {
			return err
		}
		if err := requireDate("festival", "end_date", p.EndDate); err != nil {
			return err
		}
		if err := optionalHTTPURL("festival", "website", p.Website, maxRequestShortURLLen); err != nil {
			return err
		}
		if err := optionalHTTPURL("festival", "ticket_url", p.TicketURL, maxRequestShortURLLen); err != nil {
			return err
		}
		if err := optionalHTTPURL("festival", "flyer_url", p.FlyerURL, maxRequestShortURLLen); err != nil {
			return err
		}
		return optionalMaxLen("festival", "description", p.Description, maxRequestDescriptionLen)
	default:
		return fmt.Errorf("unsupported entity request type: %q", entityType)
	}
}

// requireField returns an error when a required string field is empty (after
// trimming). Keeps ValidateEntityRequestPayload's required-field checks terse.
func requireField(entityType, field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s payload: %s is required", entityType, field)
	}
	return nil
}

// optionalHTTPURL validates an optional URL field: nil/empty is allowed, but a
// present value must be a well-formed http/https URL no longer than maxLen.
// Scheme validation here at the request trust boundary keeps a hostile scheme
// (javascript:, data:) from riding the payload onto the created entity when the
// request is fulfilled (PSY-1038). maxLen is the strictest limit for the
// destination field — a VARCHAR column's bound where one exists (image_url
// VARCHAR(2048); festival website/ticket_url/flyer_url VARCHAR(500)), else a
// policy cap for a TEXT column — so an over-long value is rejected here (422)
// rather than failing at INSERT (500). fulfillEntity re-validates the stored
// payload, so this also guards rows queued before these checks existed.
func optionalHTTPURL(entityType, field string, value *string, maxLen int) error {
	if value == nil {
		return nil
	}
	if len(*value) > maxLen {
		return fmt.Errorf("%s payload: %s must be %d characters or fewer", entityType, field, maxLen)
	}
	if err := utils.ValidateHTTPURL(*value, field); err != nil {
		return fmt.Errorf("%s payload: %w", entityType, err)
	}
	return nil
}

// optionalMaxLen rejects an optional text field that exceeds max characters
// (nil is allowed). Mirrors the length caps the direct catalog create/update
// handlers enforce, so a fulfilled entity can't hold text the direct API would
// reject.
func optionalMaxLen(entityType, field string, value *string, max int) error {
	if value != nil && len(*value) > max {
		return fmt.Errorf("%s payload: %s must be %d characters or fewer", entityType, field, max)
	}
	return nil
}

// Field-length caps for entity_request payloads, sized to the destination
// catalog column / direct-handler limit the fulfilled entity lands in:
//   - 2048: image_url (VARCHAR(2048)); cover_art_url + bandcamp_embed_url are
//     TEXT columns, so 2048 there is a policy cap (these URLs are short in
//     practice).
//   - 500: festival website / ticket_url / flyer_url (VARCHAR(500)).
//   - 5000: description (the limit the direct create/update handlers enforce).
const (
	maxRequestURLLen         = 2048
	maxRequestShortURLLen    = 500
	maxRequestDescriptionLen = 5000
	// Show-specific caps, mirroring the direct show-create handler's Resolve
	// limits (PSY-1037): title ≤255 (column is VARCHAR(500); 255 keeps boundary
	// parity with the direct path), age_requirement ≤50, price 0–10000.
	// city/state mirror the shows columns (VARCHAR(255)/VARCHAR(10)).
	maxRequestTitleLen = 255
	maxRequestAgeLen   = 50
	maxRequestPrice    = 10000
	maxRequestCityLen  = 255
	maxRequestStateLen = 10
)

// requireDate validates a required date field is present AND well-formed
// (YYYY-MM-DD). Used where the value reaches a DATE column or drives a derived
// year, so a malformed date must be rejected at the trust boundary (422)
// instead of failing later at INSERT (500). time.Parse also rejects
// impossible calendar dates (e.g. month 13).
func requireDate(entityType, field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s payload: %s is required", entityType, field)
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return fmt.Errorf("%s payload: %s must be a valid YYYY-MM-DD date", entityType, field)
	}
	return nil
}

// requireDateTimeOrDate validates a required timestamp field that accepts
// either a full RFC3339 timestamp (explicit show time) or a date-only
// YYYY-MM-DD value (anchored to an evening time at fulfillment — PSY-1037).
func requireDateTimeOrDate(entityType, field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s payload: %s is required", entityType, field)
	}
	if _, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err == nil {
		return nil
	}
	return fmt.Errorf("%s payload: %s must be an RFC3339 timestamp or a YYYY-MM-DD date", entityType, field)
}

// ValidEntityRequestTypes returns the registered entity_type discriminators.
// Order is not guaranteed (map iteration); callers that need a stable order
// should sort.
func ValidEntityRequestTypes() []string {
	out := make([]string, 0, len(payloadRegistry))
	for k := range payloadRegistry {
		out = append(out, k)
	}
	return out
}

// MarshalPayload serializes a typed payload to json.RawMessage for storage in
// the entity_requests.payload JSONB column. It is the write-side counterpart
// to UnmarshalPayload.
func MarshalPayload(p EntityRequestPayload) (json.RawMessage, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", p.entityRequestType(), err)
	}
	return json.RawMessage(raw), nil
}

// UnmarshalPayload decodes a stored JSONB payload into the typed struct T,
// failing LOUDLY on schema drift rather than silently returning a zero value.
//
// "Fail loud" means: an unknown field in the stored JSON (a field the struct
// T does not declare) is an ERROR, not a silently-dropped value. This is the
// schema-drift guard the ticket requires — if the on-disk payload shape ever
// diverges from the Go struct (e.g. a producer wrote a field a later struct
// version removed, or the wrong T is used for the row's entity_type), the
// caller gets an error instead of a struct missing data.
//
// nil/empty input is an error: a creation request with no payload is invalid,
// and the column is NOT NULL, so empty here signals corruption.
func UnmarshalPayload[T EntityRequestPayload](raw json.RawMessage) (T, error) {
	var out T
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, fmt.Errorf("unmarshal %s payload: empty payload", out.entityRequestType())
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, fmt.Errorf("unmarshal %s payload: %w", out.entityRequestType(), err)
	}
	// Reject trailing data after the first JSON value (e.g. concatenated
	// objects) — another corruption signal DisallowUnknownFields won't catch.
	if dec.More() {
		return out, fmt.Errorf("unmarshal %s payload: unexpected trailing data", out.entityRequestType())
	}
	return out, nil
}
