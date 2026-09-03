package community

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

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

	// dedupOccurrenceTerm returns the payload value that distinguishes two of this
	// type's requests sharing a name, or the zero term for a type that has none
	// (PSY-1977; PSY-1989 gave it a fallback chain and a width so that types other
	// than show could express one). It is the occurrence term of the
	// pending-request dedup key: without it, one requester's two "Open Mic" shows
	// collide and the second submission destroys the first.
	//
	// It is on the INTERFACE so that a seventh payload type has to write this
	// method to compile. Be clear about how much that buys: the compiler forces a
	// method, NOT a correct answer, and the zero term is both a legitimate answer
	// and the easiest one to copy from the neighbours. It puts the question in
	// front of the author; it does not answer it for them.
	//
	// TWO RULES decide the answer, and the second is the one that is easy to miss.
	//
	// PRESENT ON EVERY REQUEST. A value that may be absent turns "queued without
	// it, resubmitted with it" into a second request rather than the correction it
	// is. That rules out release_date and an artist's optional city. Present is
	// not the same as required at the boundary: a festival's edition_year is
	// optional and derived from the required start_date when it is missing, so the
	// DERIVED value is always present and is what the term reads. Derive it the
	// way fulfillment derives it, or the queue partitions requests differently
	// from the catalog they become.
	//
	// NO FINER THAN THE CATALOG'S OWN UNIQUENESS. Two pending rows this key
	// separates that the catalog would merge are two requests an admin can
	// approve, and the second fulfilment then fails AFTER its row has been
	// claimed, leaving an approved-but-unfulfilled orphan for the rescue flow.
	// Trading a destroyed request for an orphaned one is not a fix. So each term
	// is read off the catalog constraint the fulfilled entity meets: a venue's is
	// venues (LOWER(name), LOWER(city)), a festival's is (series_slug,
	// edition_year). Where that constraint is the NAME alone the answer is the
	// zero term, and it is a correct answer rather than a deferral.
	//
	// THREE of the six types answer with the zero term (artist, label, release).
	// Each documents on its own struct which of the two rules decided it.
	dedupOccurrenceTerm() DedupOccurrenceTerm
}

// DedupOccurrenceTerm is a payload type's occurrence: the value the
// pending-dedup key reads out of the payload beside the name, and how it is
// normalized before it is keyed. The zero value means the type has none, and its
// key is the name alone.
//
// Exported because the SQL that renders it lives in the service layer, which
// owns every other piece of the dedup query and the index it mirrors; models
// must not grow SQL. TestDedupOccurrenceMatchesTheIndex asserts that rendering
// against the index Postgres actually built.
type DedupOccurrenceTerm struct {
	// JSONKeys is a fallback chain, tried in order: the term takes the first key
	// whose payload value is present and is not AbsentAs. More than one key
	// expresses ONE derived value (a festival's edition year, stated or taken from
	// its start date), never two independent values.
	JSONKeys []string

	// AbsentAs is a stored spelling of the FIRST key that means "not supplied",
	// beyond an empty or all-space value, which always means that for every key.
	// It exists because a JSON number field has no absent spelling: a client
	// marshalling the payload struct writes "edition_year": 0, and 0 is exactly
	// what fulfillment reads as "derive it from start_date". Without this, one
	// festival keys two different ways depending on whether the client omitted the
	// field or sent its zero value, and those two rows fulfil to the same catalog
	// edition.
	//
	// The first key only, because that is the STATED value; the keys after it are
	// the derivation that runs when nothing was stated, and a derivation has no
	// "not supplied" spelling of its own.
	AbsentAs string

	// Width is how many leading CHARACTERS of the resolved value the term keeps.
	// Characters, because Postgres left() counts characters, and requireBoundedField
	// counts runes for the same reason: the two units agree, so the pairing below
	// is exact rather than approximate.
	//
	// It serves two purposes that must not be confused. For event_date and city it
	// is INDEX SAFETY, and it equals that field's boundary cap so it can never
	// truncate a legal value — keep the two equal, since a width below the cap
	// keys a prefix of a value the boundary accepted whole and folds two distinct
	// values into one bucket. For a festival's edition year it is SEMANTIC: 4 is
	// what turns a start date into the year the catalog keys on, and there
	// maxRequestEditionYear is what keeps a legal value from being truncated.
	//
	// A character width of n bounds the indexed term at 4n BYTES, which is the unit
	// the btree row limit is counted in. That headroom is what the dedup migrations'
	// size preflights are sized against.
	Width int

	// CaseFold lowercases the value, for a term whose catalog constraint is
	// case-insensitive: venues are unique on (LOWER(name), LOWER(city)), so
	// "Phoenix" and "phoenix" are ONE venue and must be one bucket here.
	//
	// It is off by default rather than always on, and event_date is why. Folding a
	// term NARROWS the key, so two pending rows the unfolded key kept apart can
	// newly collide, and a rebuild of a live index can then fail on uniqueness
	// rather than only on size. A date carries no meaningful case, so folding it
	// would buy nothing for that risk.
	CaseFold bool
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

// The name is the whole key by the catalog's own rule: artists_lower_name_uniq
// makes an artist name globally unique, case-insensitively (PSY-1256), so two
// same-named artist requests are one catalog artist however different the bands
// are. Keying them apart on city would file two requests an admin can approve,
// and the second fulfilment would 409 after its row was claimed — a destroyed
// request traded for an orphaned one. city is optional here besides.
func (ArtistRequestPayload) dedupOccurrenceTerm() DedupOccurrenceTerm {
	return DedupOccurrenceTerm{}
}

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

// UNFIXED, and the one type whose fix this payload cannot express. Two
// same-titled releases from one requester still collide destructively, and the
// second destroys the first — which a self-titled album makes ordinary. The
// catalog permits both (uniqueReleaseSlugTx suffixes a taken slug), so unlike an
// artist these really are two entities.
//
// What separates them is their ARTIST, and this payload has no artist field —
// neither does the CreateReleaseRequest that fulfillment builds from it, so the
// credit is attached elsewhere entirely. Every other field here is optional, so
// release_date and release_year each fail the presence rule above.
//
// Adding an artist to the payload is the fix, and it is a contract change
// carrying a product call (whether a contributor must name the artist to request
// a release), so it is not made here. Revisit this term in the same change.
func (ReleaseRequestPayload) dedupOccurrenceTerm() DedupOccurrenceTerm {
	return DedupOccurrenceTerm{}
}

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

// UNFIXED, for the same reason as a release and with the same shape. The catalog
// permits two same-named labels (CreateLabel suffixes a taken slug), so two
// same-named label requests really can be two labels, and the second still
// destroys the first. Every field that would separate them — city, state,
// country, founded_year — is OPTIONAL and so fails the presence rule above.
// Two same-named labels are rarer than two same-named venues, which is why this
// one is left rather than solved by making a field required.
func (LabelRequestPayload) dedupOccurrenceTerm() DedupOccurrenceTerm {
	return DedupOccurrenceTerm{}
}

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
//
// Not case-folded, and compared as the STRING rather than the instant it
// denotes; see the field's own comment for what that costs.
func (ShowRequestPayload) dedupOccurrenceTerm() DedupOccurrenceTerm {
	return DedupOccurrenceTerm{
		JSONKeys: []string{"event_date"},
		Width:    maxRequestDateLen,
	}
}

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

// The city separates two requests sharing a name: chain and franchise venue
// names repeat across cities (The Fillmore, House of Blues), so two requests for
// one name are frequently two venues.
//
// CITY ALONE, not city and state, because the catalog's constraint is
// venues (LOWER(name), LOWER(city)) and CreateVenue refuses a second venue with
// that pair — so a state in the key would separate two rows the catalog merges,
// and the second approval would fail after its row was claimed. That refusal is a
// bare error, not a typed one, so MapVenueError does not reach it and the admin
// sees a 500 rather than the 409 an artist duplicate answers with. Either way the
// row is left approved-but-unfulfilled for the rescue flow.
//
// Case-folded to match that same constraint: "Phoenix" and "phoenix" are one
// venue there and so are one bucket here.
//
// city is required on this payload, so the term is present on every venue
// request.
func (VenueRequestPayload) dedupOccurrenceTerm() DedupOccurrenceTerm {
	return DedupOccurrenceTerm{
		JSONKeys: []string{"city"},
		Width:    maxRequestVenueCityLen,
		CaseFold: true,
	}
}

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

// The EDITION YEAR separates two requests sharing a name: two editions of one
// festival are two entities, and before this term the second destroyed the first.
//
// The year, never start_date itself, because the catalog's constraint is
// UNIQUE (series_slug, edition_year). Two pending rows differing only in
// start_date within one year are two requests here and one festival there, so
// keying on the date would trade a destroyed request for an orphaned one.
//
// It is derived exactly the way fulfillment derives it (see fulfillEntity): the
// stated edition_year, else the start_date's calendar year, with 0 read as "not
// stated" because that is what a marshalled zero value looks like and what
// fulfillment treats as absent. Width 4 is what takes the year off the front of a
// YYYY-MM-DD start_date; it also truncates a stated edition_year past four
// digits, which no real edition has.
//
// start_date is required, so the derived term is present on every festival
// request. Not case-folded: the value is digits.
//
// RESIDUAL: an edition whose stated year differs from its start date's year (a
// New Year's edition) is keyed on the stated year, which is right; but two such
// editions that OMIT the year and start in the same calendar year still collide.
func (FestivalRequestPayload) dedupOccurrenceTerm() DedupOccurrenceTerm {
	return DedupOccurrenceTerm{
		JSONKeys: []string{"edition_year", "start_date"},
		AbsentAs: "0",
		Width:    festivalEditionYearDigits,
	}
}

// festivalEditionYearDigits is the width of the festival occurrence term: four
// digits of a calendar year, which is also the YYYY prefix of a YYYY-MM-DD
// start_date. Unlike the other widths this one is semantic, not an index bound,
// so it is not paired with a boundary cap.
const festivalEditionYearDigits = 4

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

// MaxRequestVenueCityLen exposes the venue city cap for the same reason
// MaxRequestDateLen exists: a venue's city is an occurrence term, and the index's
// truncation of it must equal this cap (PSY-1989).
func MaxRequestVenueCityLen() int { return maxRequestVenueCityLen }

// MaxRequestNameLen and MaxRequestEditionYear exist so the create endpoint's
// payload doc string, which cannot be built from constants because it is a struct
// tag, can be pinned to the values the boundary actually enforces. The payload is
// json.RawMessage, so that doc string is the only contract a producer sees.
func MaxRequestNameLen() int { return maxRequestNameLen }

// MaxRequestEditionYear exposes the festival edition-year bound; see
// MaxRequestNameLen for why it is exported.
func MaxRequestEditionYear() int { return maxRequestEditionYear }

// DedupOccurrenceTermFor returns the occurrence term entityType's payload
// declares, or the zero term when it declares none or the type is unregistered
// (PSY-1977, per-type since PSY-1989).
//
// Exported so the service holding the dedup SQL renders the term from the same
// declaration the payload struct documents, rather than from a second copy of it.
// The interface method makes a new payload type STATE whether it has an
// occurrence; this is what turns a stated term the SQL does not read into a test
// failure rather than a silent return of the destructive collision.
func DedupOccurrenceTermFor(entityType string) DedupOccurrenceTerm {
	p, ok := payloadRegistry[entityType]
	if !ok {
		return DedupOccurrenceTerm{}
	}
	return p.dedupOccurrenceTerm()
}

// DedupOccurrenceTypes returns, sorted, the entity types whose payload declares
// an occurrence term. It is the set of branches the dedup index's per-type
// expression must carry; every other type keys on the name alone.
func DedupOccurrenceTypes() []string {
	types := make([]string, 0, len(payloadRegistry))
	for entityType, p := range payloadRegistry {
		if len(p.dedupOccurrenceTerm().JSONKeys) > 0 {
			types = append(types, entityType)
		}
	}
	sort.Strings(types)
	return types
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
		if err := requireBoundedField("artist", "name", p.Name, maxRequestNameLen); err != nil {
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
		if err := requireBoundedField("release", "title", p.Title, maxRequestNameLen); err != nil {
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
		if err := requireBoundedField("label", "name", p.Name, maxRequestNameLen); err != nil {
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
		if err := requireBoundedField("venue", "name", p.Name, maxRequestNameLen); err != nil {
			return err
		}
		// city is the venue's OCCURRENCE term (PSY-1989), so its cap bounds an index
		// term and not just the venues.city column. state is column parity only.
		if err := requireBoundedField("venue", "city", p.City, maxRequestVenueCityLen); err != nil {
			return err
		}
		if err := requireBoundedField("venue", "state", p.State, maxRequestVenueStateLen); err != nil {
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
		if err := requireBoundedField("show", "title", p.Title, maxRequestNameLen); err != nil {
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
		// checked above with the other name terms, age_requirement ≤50, price and
		// door_price 0–10000, description ≤5000; image_url VARCHAR(2048),
		// ticket_url VARCHAR(500)). A value that slipped past here would 500 at
		// INSERT after the row is claimed, leaving an approved-but-unfulfilled row
		// no decide call can re-process.
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
		if err := requireBoundedField("festival", "name", p.Name, maxRequestNameLen); err != nil {
			return err
		}
		// edition_year is optional (0 → derived from start_date at fulfill), but it
		// is bounded on BOTH sides. A negative value is never valid and would
		// otherwise surface from the fulfiller as a server-side 500. A value past
		// four digits is worse than invalid: the dedup occurrence term keeps four
		// digits, so 20261 and 2026 share a bucket and the second submission
		// destroys the first. See maxRequestEditionYear.
		if p.EditionYear < 0 || p.EditionYear > maxRequestEditionYear {
			return fmt.Errorf("festival payload: edition_year must be between 0 and %d", maxRequestEditionYear)
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
	// The RAW name is bounded first, because ValidateShowBill measures the trimmed
	// one and the two trims do not agree: strings.TrimSpace strips 25 Unicode space
	// runes, so a five-character act name padded with U+3000 measures 5 there and
	// is stored in full. It then flows untrimmed through fulfillment into
	// artists.name VARCHAR(255) and fails at INSERT with SQLSTATE 22001, after the
	// decide path has claimed the row. Same rule, same reason, as
	// requireBoundedField: bound what is STORED, not what a trim makes of it.
	//
	// Only the queue path measures the raw value. ValidateShowBill is shared with
	// the admin approve path, which supplies names already trimmed.
	for i := range artists {
		if utf8.RuneCountInString(artists[i].Name) > MaxShowRequestArtistNameLen {
			return fmt.Errorf("%s[%d].name must be %d characters or fewer",
				ShowPayloadBillField, i, MaxShowRequestArtistNameLen)
		}
	}
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

// requireBoundedField is requireField plus a ceiling, for a required field whose
// value reaches a bounded destination column, the pending-dedup INDEX, or both.
//
// It measures the UNTRIMMED value, and that is the load-bearing half. Go's
// strings.TrimSpace strips 25 Unicode space runes while SQL trim() strips ASCII
// 0x20 only, so a name measured after Go's trim can be 5 characters here and
// kilobytes in the expression Postgres indexes. Measuring the raw value is
// stricter than either trim and independent of both, so no future change to
// either one can open a gap. Same rule, same reason, as requireDateTimeOrDate.
//
// It counts RUNES, unlike optionalMaxLen, which counts bytes. Every cap passed
// here mirrors a VARCHAR(n), and VARCHAR counts characters, so runes is the unit
// that makes this check agree with the column exactly: it refuses nothing the
// column would hold and admits nothing it would not. Bytes would be stricter, and
// stricter is wrong HERE specifically, because fulfillEntity re-runs this
// validation over the STORED payload after the decide path has claimed the row —
// so a cap that refuses a legal value turns an already-queued request into an
// approved-but-unfulfilled orphan that no decide call can re-process. The direct
// venue handler counts runes for the same reason.
//
// Index safety survives the change: a rune cap of n bounds the indexed term at
// 4n bytes worst case, and the occurrence terms truncate in CHARACTERS too
// (Postgres left()), so the boundary and the index agree on units.
//
// Emptiness is reported before length so a field of nothing but whitespace reads
// as missing rather than oversized.
func requireBoundedField(entityType, field, value string, max int) error {
	if err := requireField(entityType, field, value); err != nil {
		return err
	}
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%s payload: %s must be %d characters or fewer", entityType, field, max)
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
	// maxRequestNameLen caps the payload field the pending-dedup key's NAME term
	// reads: name on artist, label, venue and festival, title on release and show.
	//
	// 255 is the destination column on every one of those types (artists.name,
	// labels.name, venues.name, festivals.name, releases.title are all
	// VARCHAR(255); shows.title is VARCHAR(500), where 255 keeps boundary parity
	// with the direct show-create handler's Resolve limit). One constant rather
	// than six, because the value it bounds is one thing: the term
	// uq_entity_requests_pending_dedup indexes.
	//
	// It is an INDEX bound before it is a column bound. The name term has been in
	// that index since PSY-1008 UNTRUNCATED, so an oversized value is not a
	// tidiness problem: it blows the btree index-row limit (SQLSTATE 54000, which
	// GORM does not translate to ErrDuplicatedKey, so the dedup branch does not
	// fire and a contributor turns their own input into a 500), and it can abort
	// the build of any migration that touches that index. See maxRequestDateLen
	// for the same reasoning applied to the occurrence term.
	//
	// Unlike the occurrence term, the index does NOT truncate this one, so this
	// cap does not bound rows queued before it existed. Those still need the
	// preflight the dedup migrations carry.
	maxRequestNameLen = 255
	maxRequestAgeLen  = 50
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
	maxRequestPrice = 10000
	// city/state mirror their destination columns. Both widths cover the show
	// payload's optional pair (shows.city VARCHAR(255), shows.state VARCHAR(10))
	// and the venue payload's required pair (venues.city VARCHAR(255),
	// venues.state VARCHAR(10)).
	//
	// maxRequestCityLen and maxRequestStateLen bound a SHOW's optional pair,
	// mirroring shows.city VARCHAR(255) and shows.state VARCHAR(10).
	maxRequestCityLen  = 255
	maxRequestStateLen = 10
	// maxRequestVenueCityLen bounds a VENUE's required city, which is a different
	// number from a show's for a reason: venues.city is VARCHAR(255) but the direct
	// admin create body declares maxLength 100, and the strictest applicable limit
	// is the rule this block follows. A queue-fulfilled venue must not hold a city
	// the direct API would refuse.
	//
	// It is ALSO an index bound: city is the venue's dedup occurrence term
	// (PSY-1989), and that term's Width equals this cap, asserted by
	// TestDedupOccurrenceTermsAreBoundedAtTheBoundary. Both count CHARACTERS —
	// requireBoundedField counts runes and Postgres left() counts characters — so
	// the index can never key a prefix of a value the boundary accepted whole.
	maxRequestVenueCityLen = 100
	// maxRequestVenueStateLen takes the COLUMN rather than the direct create
	// body's maxLength 100, because venues.state is VARCHAR(10) and the wider tag
	// lets the direct path fail at INSERT. A venue's state is deliberately not in
	// the dedup key, since the catalog's constraint is (LOWER(name), LOWER(city)).
	maxRequestVenueStateLen = 10
	// maxRequestEditionYear bounds a festival's edition_year, which is not a
	// formatting preference: the festival occurrence term keeps four digits, so a
	// five-digit year is truncated to its first four and shares a dedup bucket with
	// a real edition. A requester who mistypes 20261 and then submits 2026 would
	// otherwise DESTROY the first request, which is the exact collision PSY-1989
	// removes. Four digits is what a calendar year has; this is not a policy call
	// about how far ahead an edition may be announced.
	maxRequestEditionYear = 9999
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
	// The name/title term is bounded by maxRequestNameLen, which is the same rule
	// one door over: that term is indexed UNTRUNCATED, so the boundary cap is the
	// only thing keeping it index-safe, and it does not reach rows queued before
	// the cap existed. A dedup migration that widens the index tuple can therefore
	// still abort on an old row, which is why each of them carries a preflight.
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
