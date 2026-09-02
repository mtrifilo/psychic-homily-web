package community

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
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

	// dedupOccurrenceJSONKey returns the JSON key of the payload field that
	// distinguishes two requests sharing a name, or "" for a type that has none
	// (PSY-1977). It is the occurrence term of the pending-request dedup key:
	// without it, one requester's two "Open Mic" shows collide and the second
	// submission destroys the first.
	//
	// It is on the INTERFACE so that a seventh payload type has to write this
	// method to compile. Be clear about how much that buys: the compiler forces a
	// method, NOT a correct answer, and "" is both a legitimate answer and the
	// easiest one to copy from the neighbours. It puts the question in front of
	// the author; it does not answer it for them.
	//
	// FIVE of the six types answer "" today. Two of those (artist, label) are
	// uncontested. The other three (release, venue, festival) each document a
	// destructive collision this key does NOT fix, so read the comment before
	// copying the answer.
	//
	// Only a REQUIRED field belongs here. An optional one turns "queued without a
	// date, resubmitted with one" into a second request rather than the correction
	// it is — that is what rules out release_date.
	//
	// One key, not a list. That is a real limit, not a simplification: a venue
	// needs city AND state together and therefore cannot be expressed here at all.
	// See VenueRequestPayload.
	dedupOccurrenceJSONKey() string
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

// An artist is not an occurrence; two requests for one name are one request.
func (ArtistRequestPayload) dedupOccurrenceJSONKey() string { return "" }

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

// release_date is OPTIONAL, so it cannot be the occurrence term: a release
// queued without a date and resubmitted with one would file a second request
// instead of correcting the first. Two same-titled releases from one requester
// therefore still collide. Promoting release_date to required is what would make
// it eligible — revisit the dedup index in the same change.
func (ReleaseRequestPayload) dedupOccurrenceJSONKey() string { return "" }

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

// A label is not an occurrence; two requests for one name are one request.
func (LabelRequestPayload) dedupOccurrenceJSONKey() string { return "" }

// ShowRequestPayload carries the user-supplied fields to create a show.
type ShowRequestPayload struct {
	Title     string `json:"title"`
	EventDate string `json:"event_date"` // RFC3339 / YYYY-MM-DD; parsed at fulfillment
	// DoorsAt / MusicAt are optional and, unlike EventDate, must be RFC3339:
	// a bare date carries no time of day, which is the only thing these mean.
	DoorsAt        *string  `json:"doors_at,omitempty"`
	MusicAt        *string  `json:"music_at,omitempty"`
	City           *string  `json:"city,omitempty"`
	State          *string  `json:"state,omitempty"`
	Price          *float64 `json:"price,omitempty"`
	DoorPrice      *float64 `json:"door_price,omitempty"`
	AgeRequirement *string  `json:"age_requirement,omitempty"`
	Description    *string  `json:"description,omitempty"`
	TicketURL      *string  `json:"ticket_url,omitempty"`
	ImageURL       *string  `json:"image_url,omitempty"`
	// Artists is the bill the contributor knew at request time (PSY-1858).
	// Optional, and it does NOT make a show fulfillable on its own.
	//
	// It is fulfilled only when the approving admin ADOPTS it, by sending
	// use_payload_artists on the decide or fulfill endpoint. Omitting
	// show_artists does not adopt it: that is still the 422 it always was, and
	// sending both show_artists and the flag is a 422 too, since they state
	// contradictory intents. PSY-1037's posture is unchanged, and this is what
	// keeps it: a human affirms the bill, and still supplies the venue, which
	// this payload has no field for. See resolveShowBill in the entity-request
	// handlers for the exact rule.
	//
	// A producer should therefore expect the bill to be REVIEWED, not applied.
	// Note also that an adopted bill never designates a headliner by list order:
	// an act with no set_type is stored as 'performer', so a bill naming no
	// 'headliner' creates a show with no headliner row.
	//
	// DEPLOY ORDERING, because UnmarshalPayload runs DisallowUnknownFields:
	// adding this field is backward compatible (every row already queued has no
	// "artists" key, and a missing key is not an unknown one), but it is NOT
	// forward compatible. A row written WITH a bill is undecodable by a binary
	// that predates this field, so on that binary the row cannot be approved.
	//
	// BLOCKED, NOT BROKEN, which is the distinction that matters at 3am: the old
	// binary's decide path rejects PRE-claim (its own PSY-1675 image-URL guard
	// decodes the payload before Decide runs), its rescue path rejects before
	// ClaimRescueFulfillment, and its create path never stores one. No row is
	// stranded and no half-write happens; rolling forward again recovers every
	// affected row untouched. The cost of a rollback is that bill-carrying rows
	// sit unapprovable until it is undone.
	//
	// Nothing writes a bill until a producer ships, so this binds at the FIRST
	// PRODUCER, not here.
	//
	// RESUBMITTING THE SAME TITLE ON THE SAME DATE CORRECTS THE QUEUED REQUEST,
	// which is what a producer author needs to know before writing a retry loop:
	// CreateRequest dedups on (entity_type, requester, lower(trim(title)),
	// trim(event_date)), and on a collision the resubmission REPLACES the pending
	// row's whole payload rather than filing a second one (PSY-1948). So a
	// contributor who files a show with no bill, learns the bill, and resubmits
	// the same title and date now has the bill stored on the queued request, and
	// the response says replaced: true.
	//
	// Four consequences follow.
	//
	// The replacement is TOTAL, so a resubmission that drops a field the first one
	// carried drops it from the queued request — resubmit the complete show, not
	// a patch.
	//
	// A resubmission that CHANGES either half of the key files a SECOND request
	// rather than fixing the first, and only an admin decision clears the
	// original. That covers correcting a misspelled title, and also correcting the
	// date or the time of day. Two same-titled shows on different dates, which is
	// exactly what a residency or a recurring night looks like, are therefore two
	// queued requests (PSY-1977). Before that they collided, and queueing "Open
	// Mic" for October destroyed a queued September one, date and bill included.
	//
	// THE OCCURRENCE TERM IS THIS STRING, COMPARED BYTE FOR BYTE after trimming —
	// not the instant it denotes. Do not confuse it with the catalog's show dedup
	// key, which IS an instant (PSY-559) and therefore ignores spelling. Because
	// this field accepts both RFC3339 and a bare date, ONE moment has several
	// spellings, and they are different requests here:
	//
	//	"2026-09-03"                  vs "2026-09-03T20:00:00-07:00"
	//	"2026-09-03T20:00:00-07:00"   vs "2026-09-04T03:00:00Z"
	//
	// A producer that re-serializes from a JS Date hits the second one every time,
	// because toISOString() always emits Z. So a retry loop that changes spelling
	// files a new row instead of correcting, and never sees replaced: true. Send
	// the event_date string you sent the first time.
	//
	// The cost is normally a stale row an admin rejects. It is worse if the admin
	// approves BOTH: they fulfill to the same instant, so the catalog's duplicate
	// check fails the second one after its row is already claimed, leaving an
	// approved-but-unfulfilled row for the rescue flow. Canonicalizing event_date
	// at the boundary would close this, but a date-only value is anchored at 20:00
	// VENUE-LOCAL at fulfillment and the venue is not known at submit, so it
	// cannot simply be normalized to an instant. That decision is open, not made.
	//
	// SHOWS ARE NOT FULLY FIXED. Same title, same date, DIFFERENT VENUE still
	// collides destructively, and the second submission still answers replaced:
	// true. A franchise night running two cities the same evening is the real
	// case. The key cannot close it: this payload has no venue field at all (the
	// approving admin supplies the venue at fulfillment), and city/state are
	// OPTIONAL, which disqualifies them under the same rule that disqualifies
	// release_date. Title the two distinguishably, or queue the second after the
	// first is decided.
	//
	// It applies only to QUEUEING tiers: a submission that auto-approves (admin,
	// local_ambassador, a confirmed trusted_contributor) is stamped 'approved'
	// before the insert and so never meets the pending-only dedup index — it files
	// a new approved row and leaves any earlier pending one queued with its
	// original payload.
	Artists []ShowRequestArtist `json:"artists,omitempty"`
}

func (ShowRequestPayload) entityRequestType() string { return EntityRequestShow }

// A recurring night is the domain's normal case, so a show's date is what
// separates two requests sharing a title. event_date is required, so the term is
// present on every show request.
func (ShowRequestPayload) dedupOccurrenceJSONKey() string { return "event_date" }

// ShowRequestArtist is one act on the bill a contributor recorded with a show
// request (PSY-1858).
//
// There is deliberately no ID field. A contributor has no artist picker (the
// contribute flow collects typed names), so an ID here could only come from a
// client guessing at catalog ids, and fulfillment resolves names by
// find-or-create anyway (the admin-side ShowArtistInput keeps its ID because
// the moderation form does pin existing artists).
//
// SetType carries the act's role on the bill, using the same curated
// vocabulary the admin path accepts (contracts.SetTypeVocabulary). An absent
// key means "on the bill, slot unknown", which resolves to 'performer' at
// fulfillment, never 'opener'. The vocabulary itself is checked at the API
// boundary rather than here; see validateShowPayloadBillRoles for why.
//
// Omitting the key and stating 'performer' are INDISTINGUISHABLE in the created
// show: both store set_type='performer' with no headliner flag, and
// headlineSlotSQL counts both as uncurated. The difference is only in what the
// bill claims, and nothing downstream reads that claim. Stating 'headliner' is
// the only way a payload bill designates one, because bill ORDER never does.
type ShowRequestArtist struct {
	Name    string  `json:"name"`
	SetType *string `json:"set_type,omitempty"`
}

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

// UNFIXED, and the weakest of the five empty answers: chain and franchise venue
// names repeat across cities (The Fillmore, House of Blues), so two requests for
// one name are frequently two venues, and the second still destroys the first.
//
// city and state are both REQUIRED here, so unlike release_date they clear the
// eligibility rule above. What blocks them is the shape of this method: it names
// ONE key, and a venue needs city AND state together — either alone puts two
// different venues in one bucket. Widening the method to a key LIST is the fix,
// and it is a change to every implementation, so PSY-1977 left it rather than
// half-doing it for one type.
func (VenueRequestPayload) dedupOccurrenceJSONKey() string { return "" }

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

// UNFIXED, and deliberately so: two editions of one festival share a name and
// still collide destructively.
//
// start_date is required and looks like the obvious term, but it is FINER than
// the catalog's own uniqueness, which is (series_slug, edition_year) — see
// CreateFestival. Two pending rows differing only in start_date within one year
// are two requests here and one festival there, so an admin approving both gets
// a 409 on the second AFTER the row is claimed: an approved-but-unfulfilled row
// needing the rescue flow. Trading a destroyed request for an orphaned one is not
// obviously the better deal, and PSY-1977 was scoped to shows, so this stays as
// it is until someone decides it. An edition-derived term is the candidate:
// edition_year is optional (0 derives it from start_date at fulfillment), so it
// would have to be coalesced with start_date's year rather than used directly.
func (FestivalRequestPayload) dedupOccurrenceJSONKey() string { return "" }

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

// MaxRequestDateLen exposes the event_date cap so the service holding the dedup
// SQL can assert its truncation width against it (PSY-1977). The two must be
// equal: a truncation shorter than the cap would key a prefix of a value the
// boundary accepted whole, folding two legal dates into one bucket.
func MaxRequestDateLen() int { return maxRequestDateLen }

// DedupOccurrenceJSONKeys returns, sorted and deduplicated, every JSON key the
// registered payloads name as their occurrence term (PSY-1977). It is what the
// pending-request dedup key's occurrence expression must read, in the
// uq_entity_requests_pending_dedup index and in the query that mirrors it.
//
// Exported so the service holding that SQL can assert against it, against the
// index as Postgres actually built it, or both — which is the point. The
// interface method makes a new payload type STATE whether it has an occurrence;
// this is what turns a stated key the SQL does not read into a test failure
// rather than a silent return of the destructive collision.
func DedupOccurrenceJSONKeys() []string {
	seen := make(map[string]struct{}, len(payloadRegistry))
	for _, p := range payloadRegistry {
		if key := p.dedupOccurrenceJSONKey(); key != "" {
			seen[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
		// The embed URL must be a Bandcamp RELEASE page, the same rule the direct
		// artist endpoint applies (PSY-1966). The value is not confined to a
		// sandboxed iframe. It renders as an outbound link labelled Bandcamp,
		// so an off-platform host is a safety problem, not a tidiness one, and an
		// extraction that produces something else fails here, where the submitter
		// can see it, rather than on a live artist page.
		return optionalBandcampEmbedURL("artist", p.BandcampEmbedURL, maxRequestURLLen)
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
		// doors_at/music_at must be checked HERE, not at fulfillment. They are
		// RFC3339-only (a bare date carries no time of day, the sole thing they
		// mean), and music cannot precede doors. Deferring either check past the
		// claim is what produces the approved-but-unfulfilled orphan described
		// below: the decide call claims the row, fulfillment then 422s, and a
		// CLAIMED row's payload can no longer be corrected (PSY-1948's
		// resubmission replaces PENDING rows only).
		doorsAt, err := optionalRFC3339("show", "doors_at", p.DoorsAt)
		if err != nil {
			return err
		}
		musicAt, err := optionalRFC3339("show", "music_at", p.MusicAt)
		if err != nil {
			return err
		}
		if doorsAt != nil && musicAt != nil && musicAt.Before(*doorsAt) {
			return fmt.Errorf("show payload: music_at cannot be before doors_at")
		}
		// Shows are fulfillable when the admin supplies associations (PSY-1037),
		// so the payload's fields ride onto a created show — validate them with
		// the SAME caps the direct show-create handler enforces (title ≤255,
		// age_requirement ≤50, price and door_price 0–10000, description
		// ≤5000; image_url
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
		if p.DoorPrice != nil && (*p.DoorPrice < 0 || *p.DoorPrice > maxRequestPrice) {
			return fmt.Errorf("show payload: door_price must be between 0 and %d", maxRequestPrice)
		}
		if err := optionalHTTPURL("show", "image_url", p.ImageURL, maxRequestURLLen); err != nil {
			return err
		}
		if err := optionalHTTPURL("show", "ticket_url", p.TicketURL, maxRequestShortURLLen); err != nil {
			return err
		}
		if err := ValidateShowPayloadArtists(p.Artists); err != nil {
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

// PayloadImageURL returns the payload's image_url, or nil when the type has no
// such field or the value is absent.
//
// It exists so the entity-request write paths can run image_url through the
// SSRF host guard (PSY-1675) WITHOUT this package growing a dependency on a DNS
// resolver: the guard resolves hosts, which is I/O, and I/O does not belong in
// a payload model. The handler extracts the value here and validates it with
// the same shared.ValidateImageURL the direct show/venue/label endpoints use,
// so all four surfaces enforce one rule.
//
// The switch is exhaustive over the registered types on purpose. An unknown
// type is an ERROR rather than "no image URL", so adding an entity_type without
// deciding whether it carries a fetched URL fails closed instead of silently
// skipping the guard. Callers gate on IsValidEntityRequestType first, so the
// error is unreachable from the API boundary.
func PayloadImageURL(entityType string, raw json.RawMessage) (*string, error) {
	switch entityType {
	case EntityRequestArtist:
		p, err := UnmarshalPayload[ArtistRequestPayload](raw)
		if err != nil {
			return nil, err
		}
		return p.ImageURL, nil
	case EntityRequestLabel:
		p, err := UnmarshalPayload[LabelRequestPayload](raw)
		if err != nil {
			return nil, err
		}
		return p.ImageURL, nil
	case EntityRequestVenue:
		p, err := UnmarshalPayload[VenueRequestPayload](raw)
		if err != nil {
			return nil, err
		}
		return p.ImageURL, nil
	case EntityRequestShow:
		p, err := UnmarshalPayload[ShowRequestPayload](raw)
		if err != nil {
			return nil, err
		}
		return p.ImageURL, nil
	case EntityRequestRelease, EntityRequestFestival:
		// release carries cover_art_url and festival carries flyer_url; neither
		// is fetched server-side today, so neither is host-guarded. Move it here
		// if that changes.
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown entity type %q", entityType)
	}
}

// PayloadBandcampEmbedURL returns the payload's bandcamp_embed_url, or nil when
// the type has no such field or the value is absent.
//
// It exists for the same reason PayloadImageURL does, one boundary further out:
// the admin approve path has to check this value BEFORE Decide claims the row,
// because fulfilment happens after the claim and a rejection there would leave
// an approved-but-unfulfilled request that no decide call can re-process
// (PSY-1966). The extraction lives here, beside the payload shapes; the rule it
// feeds lives in utils.
//
// artist is the only type carrying the field. The switch is exhaustive over the
// registered types on purpose, and an unknown type is an ERROR rather than "no
// embed URL", so adding an entity_type without deciding whether it carries one
// fails closed instead of silently skipping the gate. Callers gate on
// IsValidEntityRequestType first, so the error is unreachable from the API
// boundary.
func PayloadBandcampEmbedURL(entityType string, raw json.RawMessage) (*string, error) {
	switch entityType {
	case EntityRequestArtist:
		p, err := UnmarshalPayload[ArtistRequestPayload](raw)
		if err != nil {
			return nil, err
		}
		return p.BandcampEmbedURL, nil
	case EntityRequestRelease, EntityRequestLabel, EntityRequestShow,
		EntityRequestVenue, EntityRequestFestival:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown entity type %q", entityType)
	}
}

// ShowPayloadBillField labels a contributor's stored bill in a 422 (PSY-1858).
// Exported so the API layer, which validates the same bill at its own trust
// boundaries, names it identically: one defect must not answer to two different
// field names depending on which boundary caught it.
const ShowPayloadBillField = "show payload: artists"

// ValidateShowBill checks the structural rules EVERY show bill obeys, whoever
// typed it (PSY-1858): the entry count, each act's name, and that no act is
// named twice. Names are expected already trimmed, which is what reaches the
// column, so trailing whitespace can neither push a legal name over the cap nor
// hide a duplicate.
//
// field labels the input in the message: ShowPayloadBillField for a
// contributor's stored bill, "show_artists" for the bill an admin submits to
// the approve endpoint.
//
// ONE implementation for both, because these are not two rules that happen to
// agree. They are one rule about what the catalog can store, applied at two
// trust boundaries. Kept as twins, a rule added to fix a data-quality problem on
// one side would leave the other creating exactly the rows it was added to stop,
// and nothing would fail.
//
// Why "no act twice" is here and not left to the database: the show service
// find-or-creates artists on a case-insensitive name match, and show_artists is
// keyed PRIMARY KEY (show_id, artist_id), so a bill naming "Boris" and "boris"
// resolves to ONE artist and violates that key at INSERT. On the decide path
// that INSERT happens after the row is claimed, so a duplicate caught any later
// than pre-claim is an approved-but-unfulfilled orphan, and one that survives a
// retry: the rescue endpoint re-reads the same bill and fails the same way.
//
// What it deliberately does NOT check is set_type's membership in the curated
// vocabulary. That vocabulary lives in services/contracts, which imports THIS
// package, so importing it back is an import cycle. Rather than duplicate the
// vocabulary here, which is drift that would stay invisible until an admin
// approve rejected a role a contributor was allowed to submit, the membership
// check runs one layer up, at the API trust boundary, against the single
// authoritative list.
func ValidateShowBill(field string, names []string) error {
	if len(names) > MaxShowRequestArtists {
		return fmt.Errorf("%s is capped at %d entries", field, MaxShowRequestArtists)
	}
	for i, name := range names {
		if name == "" {
			return fmt.Errorf("%s[%d].name is required", field, i)
		}
		if len(name) > MaxShowRequestArtistNameLen {
			return fmt.Errorf("%s[%d].name must be %d characters or fewer", field, i, MaxShowRequestArtistNameLen)
		}
	}
	if dupe, ok := firstDuplicateName(names); ok {
		return fmt.Errorf("%s names an act twice (%q)", field, dupe)
	}
	return nil
}

// ValidateShowPayloadArtists adapts a stored payload's bill onto ValidateShowBill.
//
// Called at queue-create as part of ValidateEntityRequestPayload, and EXPORTED
// so the API layer can run it again PRE-CLAIM on the decide and rescue paths.
// That second run is the load-bearing one. fulfillEntity re-validates the whole
// stored payload, but on the decide path that runs after the row has been
// claimed, so a structurally broken stored bill discovered there is an orphan no
// decide call can re-process.
func ValidateShowPayloadArtists(artists []ShowRequestArtist) error {
	return ValidateShowBill(ShowPayloadBillField, TrimmedBillNames(artists))
}

// TrimmedBillNames projects a bill onto the trimmed names ValidateShowBill
// compares, so a caller cannot accidentally validate untrimmed input.
func TrimmedBillNames(artists []ShowRequestArtist) []string {
	names := make([]string, len(artists))
	for i := range artists {
		names[i] = strings.TrimSpace(artists[i].Name)
	}
	return names
}

// firstDuplicateName reports the first name that repeats, compared
// case-insensitively to match the show service's find-or-create, which resolves
// artists on LOWER(name) in Postgres.
func firstDuplicateName(names []string) (string, bool) {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		key := strings.ToLower(name)
		if _, dupe := seen[key]; dupe {
			return name, true
		}
		seen[key] = struct{}{}
	}
	return "", false
}

// ShowPayloadArtists returns the bill stored on a show request's payload, or
// nil for any other entity type (which carries no bill).
//
// It exists for the same reason PayloadImageURL does: a caller outside this
// package needs one field out of the payload without this package taking on
// that caller's dependencies. Here the dependency is the set_type vocabulary in
// services/contracts, which imports this package. See
// validateShowPayloadBillRoles.
//
// A non-show type is (nil, nil) rather than an error: unlike PayloadImageURL,
// which fails closed so a new entity_type cannot silently skip the SSRF guard,
// "no bill" is the honest and permanently correct answer for every entity that
// is not a show.
func ShowPayloadArtists(entityType string, raw json.RawMessage) ([]ShowRequestArtist, error) {
	if entityType != EntityRequestShow {
		return nil, nil
	}
	p, err := UnmarshalPayload[ShowRequestPayload](raw)
	if err != nil {
		return nil, err
	}
	return p.Artists, nil
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

// optionalBandcampEmbedURL rejects an optional bandcamp_embed_url that is not a
// Bandcamp release page (nil is allowed). It caps length on the shared helper
// first so an absurd value is refused on the cheap rule and reports the length
// failure the same way every other payload field does, then applies the shared
// shape rule, so the queue cannot hold a value the direct artist endpoint would
// refuse.
func optionalBandcampEmbedURL(entityType string, value *string, maxLen int) error {
	if err := optionalMaxLen(entityType, utils.BandcampEmbedURLField, value, maxLen); err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	// The LABEL, not the field name, so the submit refusal and the pre-claim
	// approve refusal (validatePayloadBandcampEmbedURL) read as one sentence.
	if err := utils.ValidateBandcampEmbedURL(*value, utils.BandcampEmbedURLLabel); err != nil {
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
	// parity with the direct path), age_requirement ≤50, price and door_price
	// 0–10000.
	// city/state mirror the shows columns (VARCHAR(255)/VARCHAR(10)).
	maxRequestTitleLen = 255
	maxRequestAgeLen   = 50
	// maxRequestPrice deliberately DUPLICATES contracts.MaxShowPrice rather
	// than importing it: models must not depend on services, and this package
	// imports nothing from there.
	//
	// There is NO backstop if the two drift. The column is DECIMAL(10,2), which
	// accepts up to 99,999,999.99, so a queue-fulfilled show carrying a price
	// the direct API would refuse INSERTs cleanly and publishes. The external
	// test package holds the two together instead -- see
	// TestMaxRequestPriceMatchesContractRail in the _test package, which CAN
	// import contracts without inverting the layering.
	maxRequestPrice    = 10000
	maxRequestCityLen  = 255
	maxRequestStateLen = 10
	// maxRequestDateLen bounds a date/timestamp field that feeds the pending-dedup
	// INDEX (PSY-1977). It is a hard boundary check, not a formatting preference:
	// time.Parse accepts an RFC3339 value with an arbitrarily long fractional
	// second (it truncates past 9 digits instead of rejecting), so a multi-KB
	// event_date parses cleanly, reaches the INSERT, and blows the btree index-row
	// limit. That error is SQLSTATE 54000, which GORM does not translate to
	// ErrDuplicatedKey, so CreateRequest's dedup branch does not fire and a
	// contributor turns their own input into a 500 plus a Sentry event.
	//
	// 64 is comfortably above the longest spelling Go PRESERVES (35, with 9
	// fractional digits). RFC 3339 itself permits more — time-secfrac is
	// "." 1*DIGIT with no upper bound — and those longer values are rejected here,
	// which is the intent: Go would truncate them anyway.
	//
	// The index ALSO truncates to this same number (dedupKeyExprs), and THAT is
	// what makes the term index-safe, including for rows queued before this cap
	// existed. Keep the two equal. This check is the boundary half: it keeps junk
	// out of the stored payload and refuses the value rather than silently
	// indexing a prefix of it.
	//
	// It bounds ONLY what the index newly reads. The name/title terms have been in
	// that index since PSY-1008 and are still uncapped on five of the six types.
	// That is the same contributor-triggerable 500 on INSERT by a different door,
	// AND — verified against a live Postgres — it can abort the index build in the
	// PSY-1977 migration, because that migration adds a column to the same tuple
	// and "it fit the old index" stops being a bound. See that migration's
	// preflight. Capping those five is a boundary change with its own product call
	// about limits and is not made here.
	maxRequestDateLen = 64
)

// Bill limits (PSY-1858). Both are read ONLY by ValidateShowBill, which is the
// single enforcement point for every bill: the queue-create path reaches it
// through ValidateEntityRequestPayload, and the admin approve path reaches it
// through buildShowAssociations. So a bill that clears the queue is always one
// the fulfiller will still accept, not because two numbers are kept in step but
// because there is only one of each.
//
// The one place either number is restated is the create endpoint's payload doc
// string, which cannot be built from a constant because it is a struct tag; a
// test pins it to these values.
const (
	// MaxShowRequestArtists caps the bill a show request may carry. It stops a
	// runaway script from driving an unbounded number of artist find-or-creates
	// through one CreateShow transaction; 50 comfortably covers a festival bill.
	MaxShowRequestArtists = 50
	// MaxShowRequestArtistNameLen mirrors artists.name (VARCHAR(255)). The cap
	// is applied to the trimmed name in BYTES, which is stricter than the
	// column's 255 CHARACTERS, so a multi-byte name can never overflow at
	// INSERT.
	MaxShowRequestArtistNameLen = 255
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
	// Length BEFORE parse, and on the UNTRIMMED value. Both halves matter.
	//
	// Before parse, because time.Parse would accept an arbitrarily long fractional
	// second and hand the oversized string to an index that cannot store it.
	//
	// Untrimmed, because the two trims do not agree: Go's strings.TrimSpace strips
	// 25 Unicode space runes while SQL trim() strips ASCII 0x20 only. Measuring the
	// trimmed value would let "2026-09-03" padded with U+3000 pass at 10 bytes
	// while a 40 KB string goes into the payload column. The INDEX survives that
	// either way (dedupKeyExprs truncates), so this is not the index guard — it is
	// what keeps the stored payload from carrying kilobytes of whitespace, and
	// what keeps this check independent of SQL's trim semantics. See
	// maxRequestDateLen.
	if len(value) > maxRequestDateLen {
		return fmt.Errorf("%s payload: %s must be %d characters or fewer", entityType, field, maxRequestDateLen)
	}
	if _, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err == nil {
		return nil
	}
	return fmt.Errorf("%s payload: %s must be an RFC3339 timestamp or a YYYY-MM-DD date", entityType, field)
}

// optionalRFC3339 parses an optional RFC3339 payload field at the queue-create
// trust boundary. Absent or blank is valid and yields nil; anything else that
// does not parse is rejected here so it can never reach fulfillment, which runs
// after the row has been claimed and cannot be retried.
func optionalRFC3339(entityType, field string, value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, fmt.Errorf("%s payload: %s must be an RFC3339 timestamp", entityType, field)
	}
	return &t, nil
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
