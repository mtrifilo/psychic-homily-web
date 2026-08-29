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

// billFieldBody / billFieldPayload name the input a bill 422 is about, so the
// message points at something its reader can actually edit (PSY-1858). An admin
// who sent no show_artists must not be told their show_artists is malformed.
const (
	billFieldBody    = "show_artists"
	billFieldPayload = "payload artists"
)

// validateShowPayloadBillRoles checks each role on a show payload's bill
// against the curated set_type vocabulary (PSY-1858), at the queue-create trust
// boundary.
//
// It lives here rather than in ValidateEntityRequestPayload because the
// vocabulary lives in services/contracts, which imports the models package.
// See validateShowPayloadBill for the full note. The payload model checks the
// bill's structure (count, names, duplicates); this closes the one rule it
// cannot reach, against the same contracts.IsValidSetType the admin path uses,
// so a contributor can never submit a role an admin approve would later reject.
//
// Rejecting at submit is the whole point: a typo'd role caught here is a 422 on
// a request that was never filed, while the same typo caught at fulfillment is
// caught after the decide call has claimed the row: an approved-but-unfulfilled
// orphan that only the rescue endpoint can clear.
//
// Non-show types carry no bill and return nil. A payload that fails to decode
// also returns nil: the caller runs ValidateEntityRequestPayload first, which
// reports decode failures with a better message than this could.
func validateShowPayloadBillRoles(entityType string, raw json.RawMessage) error {
	artists, err := communitym.ShowPayloadArtists(entityType, raw)
	if err != nil {
		return nil
	}
	return showPayloadBillRoleError(artists)
}

// validateShowPayloadBill runs BOTH halves of a STORED bill's validation:
// structure (count, per-act name, no act named twice) via the model validator,
// then role vocabulary. It is the pre-claim guard on the decide and rescue
// paths, and it runs whether or not the admin also typed a bill (PSY-1858).
//
// Running it unconditionally is the point. The body's bill supersedes the
// payload's for FULFILLMENT, but fulfillEntity still re-validates the whole
// stored payload, and on the decide path that runs AFTER the row is claimed. A
// structurally broken stored bill discovered there is an approved-but-unfulfilled
// orphan that no decide call can re-process. Checked here it is a clean 422 on a
// row that is still pending.
//
// Such a row is not repairable by its contributor (no endpoint can edit a queued
// payload, PSY-1948); the admin's route is the rescue endpoint's void. That is
// the same dead end the row already had, reached earlier and with a readable
// message instead of an orphan.
//
// A payload that fails to decode returns nil for the same reason
// validateShowPayloadBillRoles does: the decode failure has a better message
// elsewhere, and inventing a second error channel here would only give one
// corrupt row two different 422s.
func validateShowPayloadBill(entityType string, raw json.RawMessage) error {
	artists, err := communitym.ShowPayloadArtists(entityType, raw)
	if err != nil {
		return nil
	}
	if verr := communitym.ValidateShowPayloadArtists(artists); verr != nil {
		return huma.Error422UnprocessableEntity(verr.Error())
	}
	return showPayloadBillRoleError(artists)
}

// showPayloadBillRoleError is the role-vocabulary half, shared by the
// queue-create and pre-claim callers so both produce one message shape.
func showPayloadBillRoleError(artists []communitym.ShowRequestArtist) error {
	for i := range artists {
		if _, serr := curatedShowArtistSetType(billFieldPayload, i, artists[i].SetType); serr != nil {
			return serr
		}
	}
	return nil
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
// runaway script from flooding one CreateShow transaction. Aliased to the
// payload's own cap (PSY-1858) so the queue and the approve path cannot drift
// into a bill that is submittable but not approvable.
const maxShowArtistInputs = communitym.MaxShowRequestArtists

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
//
// billField names where the bill came from (billFieldBody / billFieldPayload,
// PSY-1858), so a 422 about a bill the admin never typed does not blame their
// show_artists. resolveShowBill decides which it is.
func buildShowAssociations(venue *ShowVenueInput, artists []ShowArtistInput, billField string) (*showAssociations, error) {
	if venue == nil && len(artists) == 0 {
		return nil, nil
	}
	if venue == nil || len(artists) == 0 {
		// Deliberately names both fields whatever billField says: exactly one of
		// the two is missing, and the admin supplies BOTH of them, since the
		// payload has no venue and a payload bill only ever fills in for a
		// show_artists the admin omitted.
		return nil, huma.Error422UnprocessableEntity("Approving a show requires both show_venue and show_artists")
	}
	// Sanity cap on the bill size — guards a buggy script/automation from
	// driving an unbounded number of artist find-or-creates in one CreateShow
	// transaction. 50 comfortably covers a festival bill.
	if len(artists) > maxShowArtistInputs {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("%s is capped at %d entries", billField, maxShowArtistInputs))
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
	names := make([]string, 0, len(artists))
	for i, a := range artists {
		name := strings.TrimSpace(a.Name)
		// Name is required even when an ID is supplied: the show service's
		// duplicate-headliner pre-check matches on artist NAME, so an ID-only
		// entry would silently bypass it (the DB unique index still backstops,
		// but with a generic error instead of the readable conflict message).
		if name == "" {
			return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Each %s entry requires a name", billField))
		}
		if len(name) > communitym.MaxShowRequestArtistNameLen {
			return nil, huma.Error422UnprocessableEntity(fmt.Sprintf(
				"%s name must be %d characters or fewer", billField, communitym.MaxShowRequestArtistNameLen))
		}
		setType, serr := curatedShowArtistSetType(billField, i, a.SetType)
		if serr != nil {
			return nil, serr
		}
		if setType != nil {
			billIsCurated = true
		}
		names = append(names, name)
		out.artists = append(out.artists, contracts.CreateShowArtist{
			ID:          a.ID,
			Name:        name,
			IsHeadliner: a.IsHeadliner,
			SetType:     setType,
		})
	}
	// One act, named twice, is a 422 here and an INSERT failure anywhere later:
	// artists are find-or-created on a case-insensitive name match and
	// show_artists is PRIMARY KEY (show_id, artist_id), so "Boris" and "boris"
	// on one bill resolve to ONE artist and collide. On the decide path that
	// collision lands after the row is claimed, stranding it, and it survives a
	// retry because the rescue endpoint re-reads the same bill (PSY-1858).
	//
	// Name-based, so it does not catch two entries pinning the same artist ID
	// under different names. Only the admin form sends IDs, it sends each artist
	// once, and that case still fails safely (post-claim, as it always has); the
	// contributor-reachable half is the one this closes.
	if dupe, ok := communitym.FirstDuplicateArtistName(names); ok {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("%s names an act twice (%q)", billField, dupe))
	}
	if billIsCurated {
		suppressPositionInference(out.artists)
	}
	return out, nil
}

// resolveShowBill picks which of the two possible bills gets fulfilled: the one
// the admin submitted, or the one the contributor stored on the request payload
// (PSY-1858). It returns the bill and the request field it came from, so a 422
// about that bill names an input its reader can act on.
//
// The rule, in one line: the BODY is authoritative, and the payload prefills
// only a bill the body does not state at all.
//
// "Does not state at all" means the key is absent (or null). An explicit
// "show_artists": [] is a STATED bill, of zero acts, and prefills nothing: by
// the deletion rule below, an empty array is an admin who removed every act,
// and a bill with no acts is not fulfillable, so it 422s exactly as it did
// before any of this existed.
//
// The payload bill exists so the moderation form can be PREFILLED with what the
// contributor recorded, which is a data-entry saving, not a new authority.
// PSY-1037's posture is untouched: nothing is ever fulfilled from the payload
// alone. The admin still confirms the request, and still supplies the venue,
// which the payload has no field for, so an approve that omits show_venue is
// still a 422 no matter how complete the payload's bill is.
//
// Acts are NOT merged per-act, and a role is NOT borrowed from the payload for
// an act the body left uncurated. The reason is DELETION. The form the admin
// submitted was seeded from this same payload, so an act present in the payload
// and missing from the body is an act the admin deliberately REMOVED. Union
// semantics would resurrect it, and nothing an admin could do would then drop a
// hallucinated act off an AI-extracted bill, precisely the failure the
// human-confirmation posture exists to prevent. A body that states one act
// where the payload had five is a correction, not an addition.
//
// The converse also matters: because the body wins WHOLE, an admin who wants
// the contributor's bill sends it back (the prefilled form does exactly that),
// and one who wants a different bill sends that. Neither outcome depends on
// what the payload happens to contain.
//
// The returned inputs still flow through buildShowAssociations, so a payload
// bill is validated (names, cap, roles, duplicates) and
// position-inference-suppressed on exactly the same terms as an admin-typed
// one: a payload bill that states one role does not get a second,
// position-inferred headliner.
//
// The caller decides whether the request is even ELIGIBLE to prefill: the decide
// path passes nil for a row that is not pending, so an already-decided row gets
// Decide's 409 rather than a 422 about a bill nobody typed. See
// AdminDecideEntityRequestHandler.
func resolveShowBill(bodyArtists []ShowArtistInput, req *communitym.EntityRequest) ([]ShowArtistInput, string) {
	// nil, not len == 0: an explicit "show_artists": [] is a STATED bill of zero
	// acts, which prefills nothing and 422s as an unfulfillable bill, exactly as
	// it did before any of this existed.
	if bodyArtists != nil {
		return bodyArtists, billFieldBody
	}
	return payloadShowBill(req), billFieldPayload
}

// payloadShowBill converts a stored show payload's bill into the admin-shaped
// bill inputs, for prefill (PSY-1858). Nil for a non-show request, a request
// with no payload, or a payload that does not decode.
//
// A decode failure is deliberately silent rather than an error: the paths that
// call this then fall through to "no bill", which surfaces as the existing
// "approving a show requires show_venue and show_artists" 422, and fulfillment
// re-validates the stored payload anyway and reports the decode failure with a
// far better message than a prefill helper could. Inventing a second error
// channel here would only give the same corrupt row two different 422s
// depending on whether the admin also typed a bill.
//
// The converted inputs carry no ID (the payload has no ID field: contributors
// have no artist picker, so fulfillment find-or-creates by name) and no
// is_headliner (the payload states roles, not flags).
func payloadShowBill(req *communitym.EntityRequest) []ShowArtistInput {
	if req == nil || req.Payload == nil {
		return nil
	}
	artists, err := communitym.ShowPayloadArtists(req.EntityType, *req.Payload)
	if err != nil || len(artists) == 0 {
		return nil
	}
	out := make([]ShowArtistInput, 0, len(artists))
	for i := range artists {
		out = append(out, ShowArtistInput{
			Name:    artists[i].Name,
			SetType: artists[i].SetType,
		})
	}
	return out
}

// suppressPositionInference pins is_headliner=false on the acts the admin left
// uncurated, and is called only once SOME act on the bill states a role.
//
// resolveArtistRole reads position 0 as the headliner, but only as a last resort
// for an act that carries no other signal. That fallback is right for a bill
// nobody described and wrong the moment somebody does. Without this, an admin
// who marks the SECOND act "headliner" and leaves the first alone gets TWO rows
// with set_type='headliner'. Headliner resolution picks
// `set_type='headliner' ORDER BY position ASC LIMIT 1` (see SearchShows), so the
// act nobody designated wins on the tie and the curated one is discarded,
// silently corrupting the single fact PSY-1705 exists to record.
//
// The rule this encodes: a stated bill is a complete statement, so first-in-list
// is not a second opinion. ConfirmShowImport reaches the same outcome for
// markdown exports, which always state a label, but NOT by the same mechanism:
// it pins the flag false on every frontmatter entry unconditionally, so an
// unlabelled import file gets a bill with no headliner row at all. The direct
// show-CREATE handler is immune for that same blunter reason: initializeArtist,
// called at the top of CreateShowRequestBody.Resolve, pins the flag false on
// every act before the show service ever sees the bill. The show UPDATE handler had
// the same exposure (it forwards a nil IsHeadliner through replaceShowArtists ->
// associateArtists -> resolveArtistRole) and was fixed separately, in the show
// service rather than the handler, by
// catalog.suppressPositionInferenceWhenHeadlinerNamed (PSY-1860). That one arms
// only when some act NAMES itself the headliner, rather than on any stated role,
// because suppressing on a described bill where nobody claims the top would make
// the shape PSY-1704 calls a write-path defect routine; see its doc comment for
// the open disagreement between the two rules. The product's own show form was
// unaffected either way because it derives an explicit is_headliner per act.
//
// KNOWN GAP on THIS endpoint, not fixed by that ticket: buildShowAssociations
// arms billIsCurated from a stated set_type ALONE and never reads IsHeadliner,
// so a bill stated only through the legacy flag -- [{Earth}, {Boris,
// is_headliner:true}] -- never reaches this function and still writes two
// set_type='headliner' rows. That is the same corruption PSY-1860 fixed next
// door. The ShowArtistInput doc tags disclose the gap rather than promising the
// fix, so what remains is the code change and its test.
//
// Scoped deliberately narrower than initializeArtist: acts that state their own
// set_type or is_headliner are left untouched, and a bill where NOBODY states
// anything is left untouched as a whole, so no caller that predates set_type on
// this endpoint can see a different outcome. Pinning unconditionally would
// instead turn an undescribed bill into a bill with no headliner at all.
//
// A bill that ends up with no headliner row is still safe, and is sometimes the
// honest answer (an admin who states only "performer" and "dj" has not named a
// headliner). Readers COALESCE the headliner lookup down to plain
// `ORDER BY position ASC LIMIT 1`, and checkDuplicateHeadlinerConflicts falls
// back to artists[0] for the dedup key, so such a show still renders and still
// dedups. It just stops ASSERTING a headliner nobody chose.
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
// action on a request that now reads as approved with nothing created. The
// decide and rescue endpoints both run buildShowAssociations pre-claim,
// alongside the venue/name/image_url checks that live there for the same reason.
//
// field names the list being validated in the error message (billFieldBody for
// an admin body, billFieldPayload for a contributor's stored bill), so a 422
// points at the thing its reader can actually edit.
func curatedShowArtistSetType(field string, index int, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if !contracts.IsValidSetType(*value) {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf(
			"%s[%d].set_type %q is not a valid set type (allowed: %s)",
			field, index, *value, contracts.SetTypeVocabularyCSV(),
		))
	}
	return value, nil
}

// parseShowEventDate parses a show payload's event_date. An RFC3339 value is
// taken as-is; a date-only value (YYYY-MM-DD) is anchored at
// utils.DateOnlyEventHour in the state's assumed IANA zone (utils.EventLocation,
// which defaults unknown/empty states per its documented fallback), the same
// "date-only means that evening, venue-local" convention the ingest CLI and the
// PSY-987 re-anchor logic use, so a date-only show doesn't render as the
// previous day.
//
// KNOWN GAP, stated rather than left to be rediscovered (PSY-1873): the state
// is the ONLY input here, and the state map answers America/Phoenix for every
// venue outside the US. A date-only request approved for a Leeds venue
// therefore lands at 20:00 Phoenix, hours and a calendar day off, exactly the
// defect the CLI and web writers were moved off. Since CreateShow now dates the
// SLUG from the venue's real zone, such a row is also slugged a day later than
// the date the requester typed.
//
// Not closed here because this layer has no venue row and no database handle:
// showAssociations carries a name/city/state the admin typed, and resolving it
// to venues.timezone needs a *gorm.DB on EntityRequestHandler, which its
// constructor does not take. The durable fix is the explicit date-only
// precision signal on the show contract that utils.DateOnlyEventHour argues for
// (PSY-1861), which would move this decision into the show service, where the
// venue IS resolved. Until then a non-US show request needs an RFC3339
// event_date, which this function honours verbatim.
func parseShowEventDate(value, state string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t, nil
	}
	if d, err := time.Parse("2006-01-02", trimmed); err == nil {
		loc := utils.EventLocation(nil, state)
		return time.Date(d.Year(), d.Month(), d.Day(), utils.DateOnlyEventHour, 0, 0, 0, loc), nil
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
