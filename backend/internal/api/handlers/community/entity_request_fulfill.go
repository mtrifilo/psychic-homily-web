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

// billSource records where a resolved bill came from, and is the label a 422
// about that bill uses (PSY-1858). An admin who sent no show_artists must not
// be told their show_artists is malformed. A named type rather than a bare
// string so a third label cannot be invented at a call site.
type billSource string

const (
	billSourceBody billSource = "show_artists"
	// The payload label is defined in the models package and borrowed here, so
	// the same defect answers to the same field name whichever boundary catches
	// it: the model validator's message and this one are the same string.
	billSourcePayload billSource = communitym.ShowPayloadBillField
)

// validateShowPayloadBillRoles checks a show payload's bill against the curated
// set_type vocabulary (PSY-1858). It is the QUEUE-CREATE boundary's check, and
// only that one.
//
// The vocabulary cannot be checked inside ValidateEntityRequestPayload: it lives
// in services/contracts, which imports the models package, so importing it back
// is a cycle. Duplicating the vocabulary there would be drift that stays
// invisible until an admin approve rejected a role a contributor was allowed to
// submit, so the membership check runs one layer up, here, against the single
// authoritative contracts.IsValidSetType.
//
// Rejecting at submit is the whole point: a typo'd role caught here is a 422 on
// a request that was never filed. A role that got through is repairable only
// while the row is PENDING, by resubmitting the same title AND the same
// event_date string (PSY-1948, narrowed by PSY-1977 — change either and you file
// a second request instead of repairing the first, leaving the broken one
// queued); once an admin claims the row, nothing can correct its payload.
//
// NOT re-run pre-claim on the admin paths, deliberately. buildShowAssociations
// validates the roles of whichever bill actually wins, with this same function
// and the same message, so an adopted bill is still covered. Re-running it over
// the STORED bill there would instead reject an approve whose body carried a
// perfectly good bill of its own, over a stored role that is never read: the
// body supersedes the payload, and fulfillEntity's re-validation checks the
// bill's structure but NOT its roles, so there is no post-claim failure to
// pre-empt. That would turn a rescuable row into a void-only one.
//
// Non-show types carry no bill and return nil. A payload that fails to DECODE
// also returns nil: ValidateEntityRequestPayload reports the corruption with a
// better message, and a second error channel here would only give one corrupt
// row two different 422s.
func validateShowPayloadBillRoles(entityType string, raw json.RawMessage) error {
	artists, err := communitym.ShowPayloadArtists(entityType, raw)
	if err != nil {
		return nil
	}
	for i := range artists {
		if _, serr := curatedShowArtistSetType(billSourcePayload, i, artists[i].SetType); serr != nil {
			return serr
		}
	}
	return nil
}

// validateStoredShowBill checks the STRUCTURE of a stored bill (count, per-act
// name, no act named twice) at the admin paths' PRE-CLAIM boundary, whether or
// not the admin also typed a bill of their own (PSY-1858).
//
// Unconditional because this half, unlike the role half above, IS re-validated
// after the claim: fulfillEntity re-runs ValidateEntityRequestPayload over the
// whole stored payload, and on the decide path that runs once the row is already
// approved. A structurally broken stored bill discovered there is an
// approved-but-unfulfilled orphan no decide call can re-process. Checked here it
// is a clean 422 on a row that is still pending: the same dead end the row
// already had, reached earlier and with a readable message.
//
// Decode failures and non-show types return nil, as above.
func validateStoredShowBill(entityType string, raw json.RawMessage) error {
	artists, err := communitym.ShowPayloadArtists(entityType, raw)
	if err != nil {
		return nil
	}
	if verr := communitym.ValidateShowPayloadArtists(artists); verr != nil {
		return huma.Error422UnprocessableEntity(verr.Error())
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

// showAssociations carries the admin-supplied venue + artists (already
// converted to the catalog contract types) from the decide endpoint to the
// show fulfillment branch (PSY-1037). nil means "none supplied" — the show
// branch then defers via FulfillUnsupported.
type showAssociations struct {
	venue   contracts.CreateShowVenue
	artists []contracts.CreateShowArtist
	// billSource records where the fulfilled bill came from, for the audit row
	// (PSY-1858). Not used to make any decision; by the time this is set the bill
	// has already been chosen.
	billSource billSource
}

// buildShowAssociations validates + converts the decide endpoint's optional
// show-association inputs. Returns (nil, nil) when neither is supplied (a
// non-show decide, or a show approve that will defer); a Huma 422 when input
// is present but malformed — surfaced BEFORE the row is claimed, so bad input
// never produces an approved-but-unfulfilled row.
//
// billField names where the bill came from (billSourceBody / billSourcePayload,
// PSY-1858), so a 422 about a bill the admin never typed does not blame their
// show_artists. resolveShowBill decides which it is.
//
// The per-act structural rules are NOT restated here: they are delegated to
// communitym.ValidateShowBill, the one implementation the stored-bill validator
// also uses. This is the only validation an admin-typed bill ever gets (the
// model validator never sees a ShowArtistInput), so a rule added on the payload
// side and not here would leave admin-typed bills creating exactly the rows it
// was added to stop.
func buildShowAssociations(venue *ShowVenueInput, artists []ShowArtistInput, billField billSource) (*showAssociations, error) {
	if venue == nil && len(artists) == 0 {
		return nil, nil
	}
	if venue == nil || len(artists) == 0 {
		// Named separately, because the combined "supply them together" wording
		// sent an adopting admin into a loop: the venue is the thing they are
		// missing, but the message told them to add show_artists, and doing so
		// earns the mutually-exclusive 422 instead. No verb either: this same
		// function answers an approve (decide) and a fulfill (rescue), and
		// "Approving" named a call the rescue admin never made.
		if venue == nil {
			return nil, huma.Error422UnprocessableEntity("show_venue is required to fulfil a show")
		}
		// "a bill" and not "the bill on this request's payload": a non-show decide
		// that carries a stray show_venue reaches this branch, and that request's
		// payload has no bill to adopt. Promising one there would send the reader
		// looking for a field that does not exist on their entity type.
		return nil, huma.Error422UnprocessableEntity(
			"supply show_artists, or set use_payload_artists to adopt a bill stored on the request's payload")
	}
	// Names are validated as a whole bill, BEFORE any conversion work: the cap
	// exists to stop a runaway script from driving an unbounded number of artist
	// find-or-creates through one CreateShow transaction, so it should reject
	// before the loop, not inside it.
	names := communitym.TrimmedBillNames(showBillArtists(artists))
	if verr := communitym.ValidateShowBill(string(billField), names); verr != nil {
		return nil, huma.Error422UnprocessableEntity(verr.Error())
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
	// Name is required even when an ID is supplied (enforced by ValidateShowBill
	// above): the show service's duplicate-headliner pre-check matches on artist
	// NAME, so an ID-only entry would silently bypass it (the DB unique index
	// still backstops, but with a generic error instead of the readable conflict
	// message).
	//
	// KNOWN GAP, stated rather than left to be rediscovered: the duplicate check
	// is name-based, so two entries pinning the same artist ID under different
	// names still collide on PRIMARY KEY (show_id, artist_id) post-claim. Only
	// the admin form sends IDs and it sends each artist once, so that half stays
	// as exposed as it has always been; the contributor-reachable half, which is
	// name-only by construction, is closed.
	out := &showAssociations{venue: v}
	billIsCurated := false
	for i, a := range artists {
		setType, serr := curatedShowArtistSetType(billField, i, a.SetType)
		if serr != nil {
			return nil, serr
		}
		if setType != nil {
			billIsCurated = true
		}
		out.artists = append(out.artists, contracts.CreateShowArtist{
			ID:          a.ID,
			Name:        names[i],
			IsHeadliner: a.IsHeadliner,
			SetType:     setType,
		})
	}
	// A PAYLOAD bill never gets the position-0 headliner guess, whether or not
	// any act states a role (PSY-1858). For an admin-typed bill the guess is a
	// last resort on a bill nobody described, and it is unreachable in practice
	// because the moderation form sends an explicit is_headliner per act. A
	// contributor's bill is the first input where list ORDER reaches
	// resolveArtistRole, and order is exactly the signal PSY-1673 removed: it is
	// whatever the contributor typed, or whatever an extractor happened to emit.
	// Inferring from it would assert a headliner nobody chose, silently, on the
	// one field PSY-1705 exists to record. A bill that names no headliner is the
	// honest answer and stays safe: readers COALESCE down to position order.
	//
	// This is also what makes the ticket's "an act with no stated role resolves
	// to performer" hold at the column for a names-only bill, rather than for
	// every act except the first.
	if billIsCurated || billField == billSourcePayload {
		suppressPositionInference(out.artists)
	}
	return out, nil
}

// showBillArtists projects the admin-shaped bill inputs onto the model's bill
// shape, so the one structural validator can be handed either.
func showBillArtists(artists []ShowArtistInput) []communitym.ShowRequestArtist {
	out := make([]communitym.ShowRequestArtist, len(artists))
	for i := range artists {
		out[i] = communitym.ShowRequestArtist{Name: artists[i].Name, SetType: artists[i].SetType}
	}
	return out
}

// resolveShowBill picks which bill gets fulfilled: the one the admin submitted,
// or the one the contributor stored on the request payload (PSY-1858). It
// returns the bill and the field it came from, so a 422 about that bill names an
// input its reader can act on.
//
// THE RULE, in three lines:
//   - show_artists present  -> that bill, whatever the payload holds.
//   - use_payload_artists   -> the payload's bill, adopted wholesale.
//   - neither               -> no bill, which is a 422. Nothing is adopted by
//     default.
//
// Sending BOTH is a 422, not a precedence puzzle. The two say contradictory
// things ("fulfil what the contributor recorded" and "fulfil what I typed"), and
// the stricter answer is the safe one: refusing costs one retry, while silently
// picking a winner would fulfil a bill the admin may not have meant.
//
// So "body wins" survives only where it still means something: over a STORED
// bill the admin did not adopt. Body plus the flag is refused outright, not
// resolved in the body's favour.
//
// WHY A FLAG, and not an omitted show_artists (superseded design, recorded so
// the change is not undone by someone reading only the ticket): an omitted key
// is indistinguishable from a client that never knew about the field, so it
// cannot carry intent. Reading it as adoption made an approve FAIL OPEN: on the
// pre-PSY-1858 code an approve without a bill was a hard 422, and it would have
// become a success that created up to 50 catalog artists from contributor or
// AI-extracted text, with the venue requirement as the only human step, and that
// checks the venue, not the bill. Worse on the rescue endpoint, where a trusted
// tier's auto-approved row may never have been reviewed by anyone. The flag
// makes adoption an act, so the queue stays fail-closed: PSY-1037's posture is
// that a human affirms the request, and this keeps it.
//
// The flag adopts, it does not merge. Acts are not combined per-act and a role
// is never borrowed from the payload for an act the body left uncurated, because
// once the moderation form seeds from this payload an act present in the payload
// and missing from the body is one the admin deliberately REMOVED. Union
// semantics would resurrect it, and nothing an admin could do would then drop a
// hallucinated act off an AI-extracted bill.
//
// The admin still supplies the venue, which the payload has no field for, so an
// approve that omits show_venue is a 422 however complete the adopted bill is.
//
// The returned inputs still flow through buildShowAssociations, so an adopted
// bill is validated (names, cap, roles, duplicates) and
// position-inference-suppressed on exactly the same terms as an admin-typed one.
//
// Whether the request is ELIGIBLE to adopt at all is admitShowBill's question,
// not this one's: a nil req means "not eligible", and the two endpoints answer
// that differently. Adoption asked for against a nil req yields no bill rather
// than an error, so a row that cannot be acted on reports why it cannot be acted
// on (Decide's 409) instead of complaining about its payload.
func resolveShowBill(bodyArtists []ShowArtistInput, req *communitym.EntityRequest, adoptPayloadBill bool) ([]ShowArtistInput, billSource, error) {
	// nil, not len == 0: an explicit "show_artists": [] is a STATED bill of zero
	// acts. It conflicts with the flag exactly as a populated one does, and on
	// its own it is an unfulfillable bill rather than a request to adopt.
	if adoptPayloadBill && bodyArtists != nil {
		return nil, billSourceBody, huma.Error422UnprocessableEntity(
			"show_artists and use_payload_artists are mutually exclusive: send the bill to fulfil, or set the flag to adopt the one on the request's payload, not both")
	}
	if bodyArtists != nil {
		return bodyArtists, billSourceBody, nil
	}
	if !adoptPayloadBill {
		// No bill anywhere. Named as the BODY's field: show_artists is what this
		// caller would have to fill in, and the payload is not their input.
		return nil, billSourceBody, nil
	}
	bill, derr := payloadShowBill(req)
	if derr != nil {
		// "Carries no artists" would be a MISDIAGNOSIS here: the payload may hold
		// a full bill that simply cannot be read, which is exactly the state the
		// DisallowUnknownFields rollback window produces (see the Artists field's
		// doc). Report the corruption, in the same words validatePayloadImageURL
		// uses for it, because on this path that check runs later and on the
		// rescue path it does not run at all.
		return nil, billSourcePayload, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Invalid payload for %s: %v", req.EntityType, derr))
	}
	if len(bill) == 0 && req != nil {
		// Honest about which input is empty. Falling through to the caller's
		// "a show needs a venue and a bill" refusal would tell an admin who DID
		// ask for the payload's bill to go find a field they deliberately left out.
		//
		// The req != nil half is NOT redundant, though both callers now pass a
		// non-nil row: nil means "this row is not eligible to adopt", and a row
		// nobody can act on must not answer with a complaint about its payload.
		// That is the exact 422-instead-of-409 this endpoint has already had to
		// fix twice. Keep the guard even if the callers look like they make it
		// unreachable.
		return nil, billSourcePayload, huma.Error422UnprocessableEntity(
			"use_payload_artists was set, but this request's payload carries no artists")
	}
	return bill, billSourcePayload, nil
}

// admitShowBill is the pre-claim admission check for THE BILL, and only the
// bill, that both admin endpoints run before anything is claimed or created. It
// lives in one place because they drifted apart the moment they each assembled
// it by hand (PSY-1858).
//
// Scope, stated because the name invites over-reading: it unifies the bill's
// checks, NOT every pre-claim check over the stored payload. The decide path
// still runs validatePayloadImageURL itself, and the rescue path still does not
// run it at all (PSY-1956) — that gap predates this and is not closed here.
//
// It runs, in this order:
//
//  1. validate the STORED bill's structure, whether or not the body carries a
//     bill of its own, because fulfillEntity re-validates it after the claim,
//     where a rejection is an orphan instead of a 422.
//  2. resolve which bill is being fulfilled: the body's, or the payload's when
//     the admin adopted it with use_payload_artists. Never both; NEITHER is a
//     supported input, yielding no bill, which each caller turns into its own
//     refusal (and for a non-show decide, into a plain (nil, nil)).
//  3. validate + convert whichever won, naming its source in any 422.
//
// Step 1 is deliberately STRUCTURE only. Roles are checked in step 3, against
// the bill that actually gets fulfilled. Checking the stored bill's roles here
// too would reject an approve whose body carried a perfectly good bill, over a
// stored role nothing reads: unlike structure, roles are NOT re-validated after
// the claim, so there is no post-claim failure to pre-empt, and the rejection
// would turn a rescuable row into a void-only one.
//
// eligible is the request whose bill may be adopted, and nil when it may not be. Decide
// passes nil for a row that is not PENDING, because Decide claims only pending
// rows and the honest answer for anything else is its 409; rescue always passes
// the row, since a rescuable row is approved-but-unfulfilled by definition.
// Passing nil skips step 1 too, and correctly: a row this call cannot act on
// should not be reporting complaints about its payload.
//
// Returns (nil, nil) when neither a venue nor a bill exists anywhere, which is
// every non-show decide and a show approve that will defer. Each caller phrases
// its own "a show needs these" refusal, since the two verbs differ.
func admitShowBill(
	eligible *communitym.EntityRequest,
	venue *ShowVenueInput,
	bodyArtists []ShowArtistInput,
	adoptPayloadBill bool,
) (*showAssociations, error) {
	// The flag is a SHOW input, ignored for every other entity type exactly as
	// show_venue and show_artists are (both endpoints' docs promise that). Without
	// this, a client that sends the flag as a constant, which is the obvious shape
	// once one checkbox drives a shared form, would hard-block every non-show
	// approve with a complaint about artists on an entity that has none. Rescue
	// gates on the type at its call site; this is where decide gets the same
	// answer, so the two paths cannot diverge on identical input.
	if eligible != nil && eligible.EntityType != communitym.EntityRequestShow {
		adoptPayloadBill = false
	}
	if eligible != nil && eligible.Payload != nil {
		if verr := validateStoredShowBill(eligible.EntityType, *eligible.Payload); verr != nil {
			return nil, verr
		}
	}
	bill, source, berr := resolveShowBill(bodyArtists, eligible, adoptPayloadBill)
	if berr != nil {
		return nil, berr
	}
	assoc, aerr := buildShowAssociations(venue, bill, source)
	if aerr != nil || assoc == nil {
		return assoc, aerr
	}
	// Recorded so the audit row can say WHICH bill was fulfilled. The flag's whole
	// premise is that adoption is an act with provenance, and after the fact
	// nothing else distinguishes an admin who typed and vetted a bill from one who
	// adopted contributor text.
	assoc.billSource = source
	return assoc, nil
}

// payloadShowBill converts a stored show payload's bill into the admin-shaped
// bill inputs, for adoption (PSY-1858). Empty for a non-show request or a
// request with no payload; a DECODE failure is returned, not swallowed.
//
// Returning the decode error is the difference between "this payload has no
// bill" and "this payload cannot be read", which are different problems with
// different fixes and which an admin adopting a bill must be able to tell apart.
// Its only caller runs under an explicit adoption request, so there is no
// fall-through path left to report the corruption later: on decide this now runs
// before validatePayloadImageURL, and on rescue nothing else decodes the payload
// at all (PSY-1956).
//
// The converted inputs carry no ID (the payload has no ID field: contributors
// have no artist picker, so fulfillment find-or-creates by name) and no
// is_headliner (the payload states roles, not flags).
func payloadShowBill(req *communitym.EntityRequest) ([]ShowArtistInput, error) {
	if req == nil || req.Payload == nil {
		return nil, nil
	}
	artists, err := communitym.ShowPayloadArtists(req.EntityType, *req.Payload)
	if err != nil {
		return nil, err
	}
	out := make([]ShowArtistInput, 0, len(artists))
	for i := range artists {
		out = append(out, ShowArtistInput{
			Name:    artists[i].Name,
			SetType: artists[i].SetType,
		})
	}
	return out, nil
}

// suppressPositionInference pins is_headliner=false on the acts nobody curated.
//
// Called on two conditions, which differ by who typed the bill (see the call
// site in buildShowAssociations): for an ADMIN-typed bill, only once some act
// states a role; for a PAYLOAD bill, always. The asymmetry is the point. An
// admin's list order is a considered ordering from a form that also sends an
// explicit is_headliner per act, while a contributor's is whatever they typed or
// an extractor emitted, so order is evidence in the first case and noise in the
// second.
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
// set_type or is_headliner are left untouched. For an ADMIN-typed bill the
// caller adds a second narrowing, leaving a bill where NOBODY states anything
// untouched as a whole, so no caller that predates set_type on this endpoint can
// see a different outcome; pinning those unconditionally would turn an
// undescribed admin bill into a bill with no headliner at all. A PAYLOAD bill
// gets exactly that outcome on purpose, because there the alternative is
// asserting a headliner nobody chose.
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
// For an ADMIN-typed bill the schema enum rejects a bad role over HTTP before
// this runs, so for billSourceBody this is the floor beneath it, catching
// in-process callers. For billSourcePayload there is no floor and no ceiling:
// the request payload is json.RawMessage, so no schema constrains it and this
// function is the ONLY vocabulary check that bill ever gets (PSY-1858).
//
// It is NOT redundant with the show
// service's validateShowArtistSetTypes, though: that one runs inside
// fulfillment, which on the decide path is after the row has been claimed, so
// rejecting there would turn a typo into an approved-but-unfulfilled row that
// Decide can no longer re-process (it only claims PENDING rows). Such a row is
// recoverable, via the PSY-1088 rescue endpoint, but only by a second admin
// action on a request that now reads as approved with nothing created. The
// decide and rescue endpoints both run buildShowAssociations pre-claim,
// alongside the venue/name/image_url checks that live there for the same reason.
//
// field names the list being validated in the error message (billSourceBody for
// an admin body, billSourcePayload for a contributor's stored bill), so a 422
// points at the thing its reader can actually edit.
func curatedShowArtistSetType(field billSource, index int, value *string) (*string, error) {
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
			DoorPrice:      p.DoorPrice,
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
