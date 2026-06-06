package community

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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
		return requireField("artist", "name", p.Name)
	case EntityRequestRelease:
		p, err := UnmarshalPayload[ReleaseRequestPayload](raw)
		if err != nil {
			return err
		}
		return requireField("release", "title", p.Title)
	case EntityRequestLabel:
		p, err := UnmarshalPayload[LabelRequestPayload](raw)
		if err != nil {
			return err
		}
		return requireField("label", "name", p.Name)
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
		return requireField("venue", "state", p.State)
	case EntityRequestShow:
		p, err := UnmarshalPayload[ShowRequestPayload](raw)
		if err != nil {
			return err
		}
		if err := requireField("show", "title", p.Title); err != nil {
			return err
		}
		return requireField("show", "event_date", p.EventDate)
	case EntityRequestFestival:
		p, err := UnmarshalPayload[FestivalRequestPayload](raw)
		if err != nil {
			return err
		}
		if err := requireField("festival", "name", p.Name); err != nil {
			return err
		}
		if err := requireField("festival", "start_date", p.StartDate); err != nil {
			return err
		}
		return requireField("festival", "end_date", p.EndDate)
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
