package community

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1858: a queued show request can carry its bill, so the admin approving it
// does not re-type the acts out of the source excerpt. The bill is PREFILL: the
// admin's submitted body stays authoritative, and nothing is ever fulfilled
// from the payload alone (the payload has no venue, and an admin still decides).

// billNames flattens a created bill to (name, role) pairs, with "-" for an act
// that stated no role, so assertions read as the bill does.
func billNames(artists []contracts.CreateShowArtist) []string {
	out := make([]string, 0, len(artists))
	for _, a := range artists {
		role := "-"
		if a.SetType != nil {
			role = *a.SetType
		}
		out = append(out, a.Name+"/"+role)
	}
	return out
}

// ============================================================================
// Queue-create: the bill is validated at submit
// ============================================================================

// A bill with a role outside the curated vocabulary is rejected at SUBMIT. This
// is the acceptance criterion that matters most: caught at fulfillment instead,
// the same typo lands after the decide call has claimed the row, leaving an
// approved-but-unfulfilled orphan no decide call can re-process.
func TestCreateEntityRequest_ShowBillInvalidRole422AtSubmit(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				t.Fatal("the request must NOT be queued with an invalid bill role")
				return nil, nil
			},
		},
		nil, nil,
	)

	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "show"
	req.Body.Payload = showRequestPayload(t, "Boris",
		communitym.ShowRequestArtist{Name: "Boris"},
		communitym.ShowRequestArtist{Name: "Earth", SetType: shared.PtrString("support")},
	)

	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaErrorWithDetail(t, err, 422,
		communitym.ShowPayloadBillField+`[1].set_type "support" is not a valid set type (allowed: `+
			contracts.SetTypeVocabularyCSV()+`)`)
}

// An empty string is not "role unknown". Only an absent key is, matching the
// admin path exactly -- a contributor must not be able to store a value the
// admin form's own schema enum would reject.
func TestCreateEntityRequest_ShowBillBlankRoleRejected(t *testing.T) {
	for _, blank := range []string{"", "   ", "Headliner", " headliner "} {
		h := NewEntityRequestHandler(
			&testhelpers.MockEntityRequestService{
				CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
					t.Fatalf("the request must NOT be queued with role %q", blank)
					return nil, nil
				},
			},
			nil, nil,
		)
		req := &CreateEntityRequestRequest{}
		req.Body.EntityType = "show"
		req.Body.Payload = showRequestPayload(t, "Boris",
			communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(blank)})

		_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
	}
}

// Every vocabulary value is submittable, so the queue accepts exactly what the
// approve path accepts.
func TestCreateEntityRequest_ShowBillAcceptsEveryVocabularyRole(t *testing.T) {
	for _, role := range contracts.SetTypeVocabulary() {
		queued := false
		h := NewEntityRequestHandler(
			&testhelpers.MockEntityRequestService{
				CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
					queued = true
					// The bill must reach the STORE verbatim -- the handler
					// validates it, it does not rewrite it.
					assert.Contains(t, string(payload), `"set_type":"`+role+`"`)
					return pendingRequest(1, "show"), nil
				},
			},
			nil, &testhelpers.MockAuditLogService{},
		)
		req := &CreateEntityRequestRequest{}
		req.Body.EntityType = "show"
		req.Body.Payload = showRequestPayload(t, "Boris",
			communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(role)})

		_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
		require.NoError(t, err, "role %q must be submittable", role)
		assert.True(t, queued, "role %q must reach the store", role)
	}
}

// A bill over the cap is rejected at submit, not left to blow up the approve
// path -- the queue and the fulfiller share one number.
func TestCreateEntityRequest_ShowBillOverCapRejected(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				t.Fatal("an over-cap bill must NOT be queued")
				return nil, nil
			},
		},
		nil, nil,
	)

	acts := make([]communitym.ShowRequestArtist, 0, communitym.MaxShowRequestArtists+1)
	for i := 0; i <= communitym.MaxShowRequestArtists; i++ {
		acts = append(acts, communitym.ShowRequestArtist{Name: fmt.Sprintf("Act %d", i)})
	}
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "show"
	req.Body.Payload = showRequestPayload(t, "Too Many Acts", acts...)

	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// A bill that names one act twice is rejected at SUBMIT. Artists are
// find-or-created on a case-insensitive name match and show_artists is
// PRIMARY KEY (show_id, artist_id), so "Boris" and "boris" resolve to ONE
// artist and collide at INSERT -- post-claim, which strands the row.
func TestCreateEntityRequest_ShowBillDuplicateActRejected(t *testing.T) {
	for _, dupe := range []string{"boris", "BORIS", "  Boris  "} {
		h := NewEntityRequestHandler(
			&testhelpers.MockEntityRequestService{
				CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
					t.Fatalf("a bill naming an act twice (%q) must NOT be queued", dupe)
					return nil, nil
				},
			},
			nil, nil,
		)
		req := &CreateEntityRequestRequest{}
		req.Body.EntityType = "show"
		req.Body.Payload = showRequestPayload(t, "Boris",
			communitym.ShowRequestArtist{Name: "Boris"},
			communitym.ShowRequestArtist{Name: dupe},
		)

		_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
	}
}

// The admin's own bill is held to the same rule, at the other boundary. This
// half predates the payload bill (an admin could always type an act twice); the
// ticket makes it contributor-reachable, so both boundaries close together.
func TestBuildShowAssociations_DuplicateActRejected(t *testing.T) {
	validVenue := &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	_, err := buildShowAssociations(validVenue, []ShowArtistInput{
		{Name: "Boris"},
		{Name: "boris"},
	}, billSourceBody)
	testhelpers.AssertHumaErrorWithDetail(t, err, 422, `show_artists names an act twice ("boris")`)

	// A payload-sourced duplicate names the payload, not a show_artists the
	// admin never sent, and it names it with the SAME label the model validator
	// uses, so one defect never answers to two field names.
	_, err = buildShowAssociations(validVenue, []ShowArtistInput{
		{Name: "Boris"},
		{Name: "Boris "},
	}, billSourcePayload)
	testhelpers.AssertHumaErrorWithDetail(t, err, 422,
		communitym.ShowPayloadBillField+` names an act twice ("Boris")`)

	stored := communitym.ValidateShowPayloadArtists([]communitym.ShowRequestArtist{
		{Name: "Boris"}, {Name: "Boris "},
	})
	require.Error(t, stored)
	assert.Equal(t, communitym.ShowPayloadBillField+` names an act twice ("Boris")`, stored.Error(),
		"the stored-bill validator and the bill builder must word the same defect identically")
}

// The property the shared cap constant exists for: a bill of exactly the size
// the QUEUE accepts is still approvable, and one act more is not. It has teeth
// against the approve path re-stating a smaller number, which is the drift the
// aliasing prevents; it says nothing about the cap's value.
func TestBuildShowAssociations_AcceptsAFullQueueSizedBill(t *testing.T) {
	validVenue := &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	acts := make([]ShowArtistInput, 0, communitym.MaxShowRequestArtists)
	for i := 0; i < communitym.MaxShowRequestArtists; i++ {
		acts = append(acts, ShowArtistInput{Name: fmt.Sprintf("Act %d", i)})
	}

	assoc, err := buildShowAssociations(validVenue, acts, billSourceBody)
	require.NoError(t, err, "the largest submittable bill must still be approvable")
	assert.Len(t, assoc.artists, communitym.MaxShowRequestArtists)

	_, err = buildShowAssociations(validVenue, append(acts, ShowArtistInput{Name: "One Too Many"}), billSourceBody)
	testhelpers.AssertHumaError(t, err, 422)
}

// A trusted tier's show request lands already-approved, and a bill on its
// payload does NOT make it self-fulfilling: the create still defers, because the
// venue is the admin's to supply and PSY-1037's confirmation posture is what
// this ticket must not weaken. The row lands where it always did, on the rescue
// path, which is where the payload bill then earns its keep.
func TestCreateEntityRequest_AutoApproveShowWithPayloadBillStillDefers(t *testing.T) {
	approved := approvedRequest(83, communitym.EntityRequestShow)

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				return approved, nil
			},
			RecordFulfillmentFn: func(requestID, createdEntityID uint) error {
				t.Fatal("a deferred show must not record a fulfillment")
				return nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				t.Fatal("a payload bill must NOT make an auto-approved show self-fulfilling: no venue was ever confirmed")
				return nil, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "show"
	req.Body.Payload = showRequestPayload(t, "Auto Approved",
		communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
	)

	resp, err := h.CreateEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err, "the deferral must stay graceful")
	assert.Nil(t, resp.Body.CreatedEntityID, "nothing is created from the payload alone")
	assert.Equal(t, communitym.EntityRequestStateApproved, resp.Body.DecisionState)
}

// The payload is json.RawMessage, so its shape reaches no OpenAPI schema and no
// generated type: the create endpoint's doc string is the entire contract a
// producer author gets. A struct tag cannot be built from constants, so this is
// the join that keeps the published text from drifting off the values it
// restates, mirroring TestShowArtistInputSetTypeEnumTagMatchesVocabulary.
func TestCreateEntityRequestPayloadDocMatchesTheRules(t *testing.T) {
	body, ok := reflect.TypeOf(CreateEntityRequestRequest{}).FieldByName("Body")
	require.True(t, ok, "CreateEntityRequestRequest.Body must exist")
	field, ok := body.Type.FieldByName("Payload")
	require.True(t, ok, "the request body must carry a Payload field")
	doc := field.Tag.Get("doc")

	assert.Contains(t, doc, fmt.Sprintf("at most %d acts", communitym.MaxShowRequestArtists),
		"the documented cap must be the cap ValidateShowBill enforces")
	assert.Contains(t, doc, contracts.SetTypeVocabularyCSV(),
		"the documented roles must be exactly the vocabulary, in order")
	assert.Contains(t, doc, "NEVER infers a headliner from list order",
		"the headliner rule is the one thing a producer cannot discover by trying it: "+
			"omitting set_type and stating 'performer' produce identical rows, so a "+
			"producer who assumes bill order names the headliner ships shows with none")
}

// ============================================================================
// resolveShowBill: the body-vs-payload rule, stated as tests
// ============================================================================

func TestResolveShowBill(t *testing.T) {
	payloadBill := []communitym.ShowRequestArtist{
		{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
		{Name: "Earth"},
	}

	showReq := func() *communitym.EntityRequest {
		raw := showRequestPayload(t, "Boris with Earth", payloadBill...)
		r := pendingRequest(1, communitym.EntityRequestShow)
		r.Payload = &raw
		return r
	}

	t.Run("the flag adopts the payload's bill", func(t *testing.T) {
		got, field, err := resolveShowBill(nil, showReq(), true)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "Boris", got[0].Name)
		require.NotNil(t, got[0].SetType)
		assert.Equal(t, contracts.SetTypeHeadliner, *got[0].SetType)
		assert.Nil(t, got[1].SetType, "an act with no stated role stays uncurated")
		assert.Nil(t, got[0].ID, "the payload carries no artist ids")
		assert.Nil(t, got[0].IsHeadliner, "the payload states roles, not flags")
		assert.Equal(t, billSourcePayload, field,
			"a 422 about this bill must name the payload, not a show_artists the admin never sent")
	})

	t.Run("WITHOUT the flag the payload's bill is never adopted", func(t *testing.T) {
		// The heart of the fail-closed rule. An omitted show_artists cannot carry
		// intent: it looks identical coming from a client that predates the
		// field. So it yields no bill, which the caller turns into the same 422
		// it was before any of this existed.
		got, field, err := resolveShowBill(nil, showReq(), false)
		require.NoError(t, err)
		assert.Empty(t, got, "a bill must never be adopted by default")
		assert.Equal(t, billSourceBody, field,
			"the missing input is the admin's show_artists, not the contributor's payload")
	})

	t.Run("a body bill is used as-is and acts are never merged", func(t *testing.T) {
		// The admin's form is prefilled from this same payload, so an act in the
		// payload and not in the body is an act the admin REMOVED. Resurrecting
		// it would make a hallucinated act on an AI-extracted bill undeletable.
		got, field, err := resolveShowBill([]ShowArtistInput{{Name: "Sunn O)))"}}, showReq(), false)
		require.NoError(t, err)
		require.Len(t, got, 1, "the payload's acts must not be appended to the body's")
		assert.Equal(t, "Sunn O)))", got[0].Name)
		assert.Equal(t, billSourceBody, field)
	})

	t.Run("a role is never borrowed from the payload for a body act", func(t *testing.T) {
		// Same act, named in both, curated only in the payload: the body's
		// silence is a statement, not a gap to fill.
		got, _, err := resolveShowBill([]ShowArtistInput{{Name: "Boris"}}, showReq(), false)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Nil(t, got[0].SetType, "the body left this act uncurated, so it stays uncurated")
	})

	t.Run("body bill AND flag together is a 422, not a precedence puzzle", func(t *testing.T) {
		// They say contradictory things. Refusing costs one retry; picking a
		// winner would fulfil a bill the admin may not have meant.
		_, _, err := resolveShowBill([]ShowArtistInput{{Name: "Sunn O)))"}}, showReq(), true)
		testhelpers.AssertHumaError(t, err, 422)
		assert.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("an empty body bill conflicts with the flag exactly as a full one does", func(t *testing.T) {
		// "show_artists": [] decodes to an empty non-nil slice, which is still a
		// STATED bill. It must not be read as "no bill given, so adopt".
		_, _, err := resolveShowBill([]ShowArtistInput{}, showReq(), true)
		testhelpers.AssertHumaError(t, err, 422)
	})

	t.Run("an empty body bill on its own is a stated bill of zero acts", func(t *testing.T) {
		got, field, err := resolveShowBill([]ShowArtistInput{}, showReq(), false)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.Equal(t, billSourceBody, field)
	})

	t.Run("the flag against a payload with no bill names the payload", func(t *testing.T) {
		// Not "a show requires show_venue and show_artists": that would tell an
		// admin who deliberately omitted show_artists to go fill it in.
		raw := showRequestPayload(t, "No Bill Known")
		r := pendingRequest(1, communitym.EntityRequestShow)
		r.Payload = &raw
		_, _, err := resolveShowBill(nil, r, true)
		testhelpers.AssertHumaError(t, err, 422)
		assert.Contains(t, err.Error(), "payload carries no artists")
	})

	t.Run("a non-show request has no bill to adopt", func(t *testing.T) {
		raw := showRequestPayload(t, "Boris", payloadBill...)
		r := pendingRequest(1, communitym.EntityRequestArtist)
		r.Payload = &raw
		_, _, err := resolveShowBill(nil, r, true)
		testhelpers.AssertHumaError(t, err, 422)
	})

	t.Run("an ineligible row yields no bill rather than an error", func(t *testing.T) {
		// nil is how the decide path says "this row is not eligible to adopt"
		// (it is not pending). It must NOT become a complaint about the payload,
		// or an already-decided row would answer a 422 where Decide owes a 409.
		got, _, err := resolveShowBill(nil, nil, true)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("a payload-less or corrupt row never yields a bill", func(t *testing.T) {
		noPayload := &communitym.EntityRequest{ID: 1, EntityType: communitym.EntityRequestShow}
		_, _, err := resolveShowBill(nil, noPayload, true)
		testhelpers.AssertHumaError(t, err, 422)

		corrupt := json.RawMessage(`{"title":`)
		bad := pendingRequest(1, communitym.EntityRequestShow)
		bad.Payload = &corrupt
		_, _, err = resolveShowBill(nil, bad, true)
		testhelpers.AssertHumaError(t, err, 422)
	})
}

// ============================================================================
// Decide: the payload bill fulfills a show the admin did not re-type
// ============================================================================

// decideHandler wires a decide handler over one pending row, capturing the
// CreateShow contract the fulfillment produces.
func decideHandler(t *testing.T, pending *communitym.EntityRequest, got **contracts.CreateShowRequest, decideCalled *bool) *EntityRequestHandler {
	t.Helper()
	approved := *pending
	approved.DecisionState = communitym.EntityRequestStateApproved
	return NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return pending, nil },
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				if decideCalled != nil {
					*decideCalled = true
				}
				return &approved, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				*got = req
				return &contracts.ShowResponse{ID: 900}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
}

// The headline behavior: the admin supplies the venue (which the payload cannot
// carry) and nothing else, and the contributor's bill rides through with its
// roles intact.
func TestAdminDecide_ApproveShow_AdoptedPayloadBillCarriesItsRoles(t *testing.T) {
	raw := showRequestPayload(t, "Boris with Earth",
		communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
		communitym.ShowRequestArtist{Name: "Earth", SetType: shared.PtrString(contracts.SetTypeDirectSupport)},
		communitym.ShowRequestArtist{Name: "Local Act"},
	)
	pending := pendingRequest(70, communitym.EntityRequestShow)
	pending.Payload = &raw

	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, nil)

	req := &AdminDecideEntityRequestRequest{ID: "70"}
	req.Body.Decision = "approved"
	req.Body.UsePayloadArtists = true
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err)
	require.NotNil(t, got, "CreateShow must be called")
	assert.Equal(t, []string{"Boris/headliner", "Earth/direct_support", "Local Act/-"}, billNames(got.Artists))

	// PSY-1705's rule applies to a payload bill exactly as to an admin-typed
	// one: the bill states a role, so the position-0 fallback is suppressed and
	// the uncurated act cannot become a second headliner.
	require.NotNil(t, got.Artists[2].IsHeadliner)
	assert.False(t, *got.Artists[2].IsHeadliner,
		"an uncurated act on a curated payload bill must be pinned non-headliner")
}

// The disagreement rule, end to end through the endpoint: the admin's bill is
// the bill, and the payload's extra acts are not resurrected.
func TestAdminDecide_ApproveShow_BodyBillBeatsPayloadBill(t *testing.T) {
	raw := showRequestPayload(t, "Contributor's Bill",
		communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
		communitym.ShowRequestArtist{Name: "Hallucinated Act", SetType: shared.PtrString(contracts.SetTypeDJ)},
	)
	pending := pendingRequest(71, communitym.EntityRequestShow)
	pending.Payload = &raw

	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, nil)

	req := &AdminDecideEntityRequestRequest{ID: "71"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris"}}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"Boris/-"}, billNames(got.Artists),
		"the admin dropped an act and left the survivor uncurated; both edits must stand")
}

// The venue is still the admin's to supply. A payload bill does not make a show
// fulfillable on its own -- PSY-1037's confirmation posture is unchanged.
func TestAdminDecide_ApproveShow_PayloadBillStillNeedsVenue(t *testing.T) {
	raw := showRequestPayload(t, "Boris",
		communitym.ShowRequestArtist{Name: "Boris"})
	pending := pendingRequest(72, communitym.EntityRequestShow)
	pending.Payload = &raw

	decideCalled := false
	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, &decideCalled)

	req := &AdminDecideEntityRequestRequest{ID: "72"}
	req.Body.Decision = "approved"

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	assert.False(t, decideCalled, "the row must not be claimed without a venue")
	assert.Nil(t, got)
}

// A show request with no bill anywhere behaves exactly as it did before this
// existed: the approve is refused, pre-claim.
func TestAdminDecide_ApproveShow_NoBillAnywhereStill422(t *testing.T) {
	raw := showRequestPayload(t, "No Bill Known")
	pending := pendingRequest(73, communitym.EntityRequestShow)
	pending.Payload = &raw

	decideCalled := false
	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, &decideCalled)

	req := &AdminDecideEntityRequestRequest{ID: "73"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	assert.False(t, decideCalled)
}

// A row queued before the roles were validated (or written by a client that got
// around it) is caught PRE-CLAIM, so a bad stored role can never produce an
// approved-but-unfulfilled orphan.
func TestAdminDecide_ApproveShow_StoredBadRole422BeforeClaim(t *testing.T) {
	// Built by hand: the create path would have rejected this bill, which is
	// exactly the row shape this guard exists for.
	raw := json.RawMessage(`{"title":"Legacy Row","event_date":"2026-09-12T21:00:00-07:00","artists":[{"name":"Boris","set_type":"co-headliner"}]}`)
	pending := pendingRequest(74, communitym.EntityRequestShow)
	pending.Payload = &raw

	decideCalled := false
	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, &decideCalled)

	req := &AdminDecideEntityRequestRequest{ID: "74"}
	req.Body.Decision = "approved"
	req.Body.UsePayloadArtists = true
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	assert.False(t, decideCalled, "a bad stored role must never claim the row")
	assert.Nil(t, got)
}

// An already-decided row gets Decide's 409, NOT a 422 about a bill the admin
// never typed.
//
// This is the regression the prefill introduced and review caught: with the
// prefill ungated, a re-decide that sent no show_artists picked up the payload's
// bill, then failed the venue check pre-claim and reported "approving a show
// requires both show_venue and show_artists" -- a message implying the call
// could be fixed by adding a venue, on a row that can never be decided again.
func TestAdminDecide_ApproveShow_AlreadyDecidedRowWithPayloadBillIs409(t *testing.T) {
	raw := showRequestPayload(t, "Already Approved",
		communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
	)
	decided := approvedUnfulfilledRequest(79, communitym.EntityRequestShow)
	decided.Payload = &raw

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return decided, nil },
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return nil, apperrors.ErrEntityRequestInvalidState(requestID, string(communitym.EntityRequestStateApproved))
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				t.Fatal("an already-decided row must never be fulfilled")
				return nil, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "79"}
	req.Body.Decision = "approved"

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

// An explicit empty bill on the body is a STATED bill of zero acts, so the
// payload does not refill it and the approve is refused pre-claim. Only an
// absent show_artists is a gap the payload may fill.
func TestAdminDecide_ApproveShow_EmptyBodyBillIsStillAStatedBill(t *testing.T) {
	raw := showRequestPayload(t, "Contributor's Bill",
		communitym.ShowRequestArtist{Name: "Boris"},
	)
	pending := pendingRequest(80, communitym.EntityRequestShow)
	pending.Payload = &raw

	decideCalled := false
	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, &decideCalled)

	req := &AdminDecideEntityRequestRequest{ID: "80"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	assert.False(t, decideCalled, "an emptied bill must not claim the row")
	assert.Nil(t, got)
}

// THE FAIL-CLOSED RULE, end to end and on both endpoints: a request whose
// payload carries a perfectly good bill is still refused when the admin does not
// adopt it. This is the behavior an omitted show_artists had before PSY-1858 and
// must keep having, so no approve can create artists out of contributor text
// nobody affirmed.
func TestAdminAdoption_WithoutTheFlagAPayloadBillIsNeverFulfilled(t *testing.T) {
	bill := []communitym.ShowRequestArtist{
		{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
		{Name: "Earth"},
	}
	venue := &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	t.Run("decide", func(t *testing.T) {
		raw := showRequestPayload(t, "Unadopted", bill...)
		pending := pendingRequest(90, communitym.EntityRequestShow)
		pending.Payload = &raw

		decideCalled := false
		var got *contracts.CreateShowRequest
		h := decideHandler(t, pending, &got, &decideCalled)

		req := &AdminDecideEntityRequestRequest{ID: "90"}
		req.Body.Decision = "approved"
		req.Body.ShowVenue = venue

		_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
		assert.False(t, decideCalled, "an unadopted bill must not claim the row")
		assert.Nil(t, got, "no show may be created from a bill no admin asked for")
	})

	t.Run("rescue", func(t *testing.T) {
		// Matters more here: this is where a trusted tier's auto-approved show
		// lands, so the row may never have been reviewed by anyone at all.
		raw := showRequestPayload(t, "Unadopted", bill...)
		orphan := approvedUnfulfilledRequest(91, communitym.EntityRequestShow)
		orphan.Payload = &raw

		created := false
		h := rescueHandlerFor(t, orphan, &created)

		req := &AdminFulfillEntityRequestRequest{ID: "91"}
		req.Body.ShowVenue = venue

		_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
		assert.False(t, created)
	})
}

// Sending both inputs is refused rather than resolved by precedence: they state
// contradictory intents, and guessing would fulfil a bill the admin may not have
// meant. Asserted on both endpoints because both accept both fields.
func TestAdminAdoption_BodyBillAndFlagTogetherIs422(t *testing.T) {
	raw := showRequestPayload(t, "Both", communitym.ShowRequestArtist{Name: "Boris"})
	venue := &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	t.Run("decide", func(t *testing.T) {
		pending := pendingRequest(92, communitym.EntityRequestShow)
		pending.Payload = &raw

		decideCalled := false
		var got *contracts.CreateShowRequest
		h := decideHandler(t, pending, &got, &decideCalled)

		req := &AdminDecideEntityRequestRequest{ID: "92"}
		req.Body.Decision = "approved"
		req.Body.ShowVenue = venue
		req.Body.ShowArtists = []ShowArtistInput{{Name: "Sunn O)))"}}
		req.Body.UsePayloadArtists = true

		_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
		assert.Contains(t, err.Error(), "mutually exclusive")
		assert.False(t, decideCalled)
		assert.Nil(t, got)
	})

	t.Run("rescue", func(t *testing.T) {
		orphan := approvedUnfulfilledRequest(93, communitym.EntityRequestShow)
		orphan.Payload = &raw

		created := false
		h := rescueHandlerFor(t, orphan, &created)

		req := &AdminFulfillEntityRequestRequest{ID: "93"}
		req.Body.ShowVenue = venue
		req.Body.ShowArtists = []ShowArtistInput{{Name: "Sunn O)))"}}
		req.Body.UsePayloadArtists = true

		_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
		assert.Contains(t, err.Error(), "mutually exclusive")
		assert.False(t, created)
	})
}

// Adopting a bill that is not there names the payload, not the admin's own
// show_artists. Telling an admin who deliberately omitted show_artists to go
// fill it in would be advice against the thing they just asked for.
func TestAdminAdoption_FlagWithNoStoredBillNamesThePayload(t *testing.T) {
	raw := showRequestPayload(t, "No Bill Known")
	pending := pendingRequest(94, communitym.EntityRequestShow)
	pending.Payload = &raw

	decideCalled := false
	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, &decideCalled)

	req := &AdminDecideEntityRequestRequest{ID: "94"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.UsePayloadArtists = true

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	assert.Contains(t, err.Error(), "payload carries no artists")
	assert.False(t, decideCalled)
	assert.Nil(t, got)
}

// The flag does not weaken the PENDING gate: an already-decided row still gets
// Decide's 409, not a 422 about a payload the admin asked to adopt.
func TestAdminDecide_ApproveShow_AlreadyDecidedRowWithFlagIs409(t *testing.T) {
	raw := showRequestPayload(t, "Already Approved",
		communitym.ShowRequestArtist{Name: "Boris"})
	decided := approvedUnfulfilledRequest(95, communitym.EntityRequestShow)
	decided.Payload = &raw

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return decided, nil },
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return nil, apperrors.ErrEntityRequestInvalidState(requestID, string(communitym.EntityRequestStateApproved))
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				t.Fatal("an already-decided row must never be fulfilled")
				return nil, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "95"}
	req.Body.Decision = "approved"
	req.Body.UsePayloadArtists = true

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

// A names-only PAYLOAD bill never gets the position-0 headliner guess. The
// ticket's rule is "an act with no stated role resolves to performer", and a
// contributor's list ORDER is not a role: it is whatever they typed, or whatever
// an extractor emitted. Inferring from it would assert a headliner nobody chose.
//
// This is the one place the payload bill deliberately differs from an
// admin-typed one, where an entirely undescribed bill still falls back to
// position 0 (PSY-1705). See buildShowAssociations.
func TestAdminDecide_ApproveShow_NamesOnlyPayloadBillInfersNoHeadliner(t *testing.T) {
	raw := showRequestPayload(t, "Nobody Stated A Role",
		communitym.ShowRequestArtist{Name: "Boris"},
		communitym.ShowRequestArtist{Name: "Earth"},
	)
	pending := pendingRequest(81, communitym.EntityRequestShow)
	pending.Payload = &raw

	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, nil)

	req := &AdminDecideEntityRequestRequest{ID: "81"}
	req.Body.Decision = "approved"
	req.Body.UsePayloadArtists = true
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err)
	require.NotNil(t, got)
	for i, a := range got.Artists {
		require.NotNil(t, a.IsHeadliner, "artists[%d] must be pinned, not left to bill order", i)
		assert.False(t, *a.IsHeadliner,
			"artists[%d] stated no role, so it must not be inferred as the headliner", i)
	}
}

// The admin path's own behavior is UNCHANGED by that rule: an entirely
// undescribed bill an admin typed is still left alone, so no caller predating
// set_type on this endpoint sees a different outcome (PSY-1705). The asymmetry
// is deliberate and this is the pair that pins it.
func TestBuildShowAssociations_UndescribedBodyBillStillDefersToPosition(t *testing.T) {
	validVenue := &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	assoc, err := buildShowAssociations(validVenue, []ShowArtistInput{
		{Name: "Boris"},
		{Name: "Earth"},
	}, billSourceBody)
	require.NoError(t, err)
	for i, a := range assoc.artists {
		assert.Nil(t, a.IsHeadliner,
			"artists[%d].IsHeadliner must stay nil on an admin's undescribed bill", i)
	}
}

// A stored bill naming one act twice is refused PRE-CLAIM, even though the admin
// sent their own bill: the body wins for fulfillment, but fulfillEntity
// re-validates the stored payload post-claim, so a duplicate found there would
// strand the row instead of 422-ing it.
func TestAdminDecide_ApproveShow_StoredDuplicateActs422BeforeClaim(t *testing.T) {
	raw := json.RawMessage(`{"title":"Legacy Row","event_date":"2026-09-12T21:00:00-07:00",` +
		`"artists":[{"name":"Boris"},{"name":"boris"}]}`)
	pending := pendingRequest(82, communitym.EntityRequestShow)
	pending.Payload = &raw

	decideCalled := false
	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, &decideCalled)

	req := &AdminDecideEntityRequestRequest{ID: "82"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris"}}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	assert.False(t, decideCalled, "a broken stored bill must never claim the row")
	assert.Nil(t, got)
}

// An over-cap stored bill is refused pre-claim too, for the same reason.
func TestAdminDecide_ApproveShow_StoredOverCapBill422BeforeClaim(t *testing.T) {
	acts := make([]string, 0, communitym.MaxShowRequestArtists+1)
	for i := 0; i <= communitym.MaxShowRequestArtists; i++ {
		acts = append(acts, fmt.Sprintf(`{"name":"Act %d"}`, i))
	}
	raw := json.RawMessage(`{"title":"Legacy Row","event_date":"2026-09-12T21:00:00-07:00","artists":[` +
		strings.Join(acts, ",") + `]}`)
	pending := pendingRequest(75, communitym.EntityRequestShow)
	pending.Payload = &raw

	decideCalled := false
	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, &decideCalled)

	req := &AdminDecideEntityRequestRequest{ID: "75"}
	req.Body.Decision = "approved"
	req.Body.UsePayloadArtists = true
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	assert.False(t, decideCalled)
}

// ============================================================================
// Rescue: the same prefill, on the path that recovers an auto-approved show
// ============================================================================

// The rescue path is where an auto-approved show lands (its create could never
// supply associations), so it is the path a contributor's own bill most often
// reaches. It shares resolveShowBill, so it behaves identically.
func TestAdminFulfill_Show_AdoptedPayloadBillCarriesItsRoles(t *testing.T) {
	raw := showRequestPayload(t, "Deferred Show",
		communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeSpecialGuest)},
		communitym.ShowRequestArtist{Name: "Earth"},
	)
	orphan := approvedUnfulfilledRequest(76, communitym.EntityRequestShow)
	orphan.Payload = &raw

	var got *contracts.CreateShowRequest
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn:             func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) { return true, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				got = req
				return &contracts.ShowResponse{ID: 901}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminFulfillEntityRequestRequest{ID: "76"}
	req.Body.UsePayloadArtists = true
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"Boris/special_guest", "Earth/-"}, billNames(got.Artists))
}

// Body wins on the rescue path too.
func TestAdminFulfill_Show_BodyBillBeatsPayloadBill(t *testing.T) {
	raw := showRequestPayload(t, "Deferred Show",
		communitym.ShowRequestArtist{Name: "Boris"},
		communitym.ShowRequestArtist{Name: "Wrong Act"},
	)
	orphan := approvedUnfulfilledRequest(77, communitym.EntityRequestShow)
	orphan.Payload = &raw

	var got *contracts.CreateShowRequest
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn:             func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) { return true, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				got = req
				return &contracts.ShowResponse{ID: 902}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	dj := contracts.SetTypeDJ
	req := &AdminFulfillEntityRequestRequest{ID: "77"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris", SetType: &dj}}

	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"Boris/dj"}, billNames(got.Artists))
}

// rescueHandlerFor wires a rescue handler over one approved-but-unfulfilled row,
// recording whether the catalog create was reached.
func rescueHandlerFor(t *testing.T, orphan *communitym.EntityRequest, created *bool) *EntityRequestHandler {
	t.Helper()
	return NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn:             func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) { return true, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				*created = true
				return &contracts.ShowResponse{ID: 904}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
}

// The rescue path runs the same pre-claim stored-bill check as decide, with no
// PENDING gate (a rescuable row is approved-but-unfulfilled by definition), so a
// structurally broken stored bill is refused BEFORE the catalog create rather
// than failing inside it and leaving a second orphan behind.
func TestAdminFulfill_Show_StoredStructuralDefects422BeforeCreate(t *testing.T) {
	cases := map[string]string{
		"duplicate acts": `[{"name":"Boris"},{"name":"boris"}]`,
		"blank name":     `[{"name":"Boris"},{"name":"   "}]`,
	}
	for name, bill := range cases {
		t.Run(name, func(t *testing.T) {
			raw := json.RawMessage(`{"title":"Legacy Row","event_date":"2026-09-12T21:00:00-07:00","artists":` + bill + `}`)
			orphan := approvedUnfulfilledRequest(84, communitym.EntityRequestShow)
			orphan.Payload = &raw

			created := false
			h := rescueHandlerFor(t, orphan, &created)

			req := &AdminFulfillEntityRequestRequest{ID: "84"}
			req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

			_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
			assert.False(t, created, "a broken stored bill must not reach the catalog create")
		})
	}
}

// A stored ROLE the vocabulary no longer accepts must NOT block a rescue whose
// body carries a good bill of its own. The body supersedes the payload, and
// fulfillEntity's re-validation checks structure but not roles, so there is no
// post-claim failure to pre-empt: refusing here would turn a rescuable row into
// a void-only one, discarding the contributor's request and their attribution.
func TestAdminFulfill_Show_StoredBadRoleDoesNotBlockAnAdminsOwnBill(t *testing.T) {
	raw := json.RawMessage(`{"title":"Legacy Row","event_date":"2026-09-12T21:00:00-07:00",` +
		`"artists":[{"name":"Boris","set_type":"co-headliner"}]}`)
	orphan := approvedUnfulfilledRequest(85, communitym.EntityRequestShow)
	orphan.Payload = &raw

	created := false
	h := rescueHandlerFor(t, orphan, &created)

	req := &AdminFulfillEntityRequestRequest{ID: "85"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris"}}

	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err, "the admin's own bill is what gets fulfilled; the stored role is never read")
	assert.True(t, created)
}

// The same rule on the decide path: a stored role nothing reads must not refuse
// an approve that carries its own bill.
func TestAdminDecide_ApproveShow_StoredBadRoleDoesNotBlockAnAdminsOwnBill(t *testing.T) {
	raw := json.RawMessage(`{"title":"Legacy Row","event_date":"2026-09-12T21:00:00-07:00",` +
		`"artists":[{"name":"Boris","set_type":"co-headliner"}]}`)
	pending := pendingRequest(86, communitym.EntityRequestShow)
	pending.Payload = &raw

	decideCalled := false
	var got *contracts.CreateShowRequest
	h := decideHandler(t, pending, &got, &decideCalled)

	req := &AdminDecideEntityRequestRequest{ID: "86"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris"}}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err)
	assert.True(t, decideCalled)
	require.NotNil(t, got)
	assert.Equal(t, []string{"Boris/-"}, billNames(got.Artists))
}

// The rescue endpoint's refusals speak in its OWN verb. A payload bill made the
// venue-missing branch newly reachable here, where the shared builder used to
// answer "Approving a show ..." for a call that approves nothing.
func TestAdminFulfill_Show_MissingVenueDoesNotSayApproving(t *testing.T) {
	raw := showRequestPayload(t, "Deferred Show", communitym.ShowRequestArtist{Name: "Boris"})
	orphan := approvedUnfulfilledRequest(87, communitym.EntityRequestShow)
	orphan.Payload = &raw

	created := false
	h := rescueHandlerFor(t, orphan, &created)

	req := &AdminFulfillEntityRequestRequest{ID: "87"}
	req.Body.UsePayloadArtists = true

	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	assert.False(t, created)
	assert.NotContains(t, strings.ToLower(err.Error()), "approving",
		"a fulfill must not be told to check an approve it never made")
}

// A non-show rescue is untouched by any of this: nothing looks for a bill, and
// a stray show_artists on the body is still ignored rather than misread.
func TestAdminFulfill_NonShow_IgnoresBillPrefill(t *testing.T) {
	orphan := approvedUnfulfilledRequest(78, communitym.EntityRequestArtist)

	created := false
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn:             func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) { return true, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				created = true
				return &contracts.ArtistDetailResponse{ID: 903}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminFulfillEntityRequestRequest{ID: "78"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Stray"}}

	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	require.NoError(t, err)
	assert.True(t, created)
}

// =============================================================================
// INTEGRATION -- a payload bill actually lands in show_artists.set_type
// =============================================================================
//
// These are methods on PSY-1705's EntityRequestSetTypeIntegrationSuite (defined
// in entity_request_set_type_test.go) rather than a second suite: they assert
// the same thing about the same table through the same real fulfiller, and the
// only difference is where the bill came from. Reusing it also keeps one
// Postgres setup instead of two.

// The acceptance criterion end to end: the contributor's roles persist, an act
// that stated none lands on 'performer' (never 'opener'), and the admin typed
// only the venue.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_AdoptedBillPersistsCuratedRoles() {
	orphan := s.showOrphan("Contributor Bill",
		communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
		communitym.ShowRequestArtist{Name: "Earth", SetType: shared.PtrString(contracts.SetTypeDirectSupport)},
		communitym.ShowRequestArtist{Name: "Unstated Act"},
	)
	h := s.rescueHandler(orphan)

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.UsePayloadArtists = true
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypeHeadliner, got[0])
	s.Equal(contracts.SetTypeDirectSupport, got[1])
	s.Equal(contracts.SetTypePerformer, got[2],
		"an act with no stated role must land on the neutral default, not 'opener'")

	// PSY-1705's rule holds for a payload bill too: exactly one headliner row,
	// and it is the act the contributor named.
	var headliners int64
	s.Require().NoError(s.deps.DB.
		Table("show_artists").
		Where("show_id = ? AND set_type = ?", *resp.Body.CreatedEntityID, contracts.SetTypeHeadliner).
		Count(&headliners).Error)
	s.EqualValues(1, headliners, "a curated payload bill must not gain a position-inferred headliner")
}

// The names-only case, proved at the COLUMN rather than at the contract struct:
// a bill on which the contributor stated no roles at all lands every act on
// 'performer', with NO headliner row. Bill order is not a role, so nothing
// infers one from it.
//
// This is the acceptance criterion "an act with no stated role resolves to
// performer" holding for every act, not for every act except the first.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_AdoptedNamesOnlyBillHasNoHeadliner() {
	orphan := s.showOrphan("Names Only",
		communitym.ShowRequestArtist{Name: "Boris"},
		communitym.ShowRequestArtist{Name: "Earth"},
	)
	h := s.rescueHandler(orphan)

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.UsePayloadArtists = true
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypePerformer, got[0],
		"the first act stated no role, so it must not become the headliner by position")
	s.Equal(contracts.SetTypePerformer, got[1])

	var headliners int64
	s.Require().NoError(s.deps.DB.
		Table("show_artists").
		Where("show_id = ? AND set_type = ?", *resp.Body.CreatedEntityID, contracts.SetTypeHeadliner).
		Count(&headliners).Error)
	s.Zero(headliners, "a bill nobody described must not assert a headliner nobody chose")
}

// A headliner stated anywhere but position 0 on a PAYLOAD bill is still the
// only headliner: the suppression is a property of the resolved bill, not of
// where the bill was typed.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_AdoptedBillSuppressesPositionInference() {
	orphan := s.showOrphan("Headliner Billed Second",
		communitym.ShowRequestArtist{Name: "Earth"},
		communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
	)
	h := s.rescueHandler(orphan)

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.UsePayloadArtists = true
	req.Body.ShowVenue = &ShowVenueInput{Name: "Crescent Ballroom", City: "Phoenix", State: "AZ"}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypePerformer, got[0],
		"the uncurated first act must not be inferred as a second headliner")
	s.Equal(contracts.SetTypeHeadliner, got[1])
}

// The disagreement rule, proved against the database: the admin's bill is what
// gets written, and an act the admin dropped is not resurrected from the
// payload, so a hallucinated act on an AI-extracted bill stays deletable.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_BodyBillReplacesPayloadBill() {
	orphan := s.showOrphan("Corrected Bill",
		communitym.ShowRequestArtist{Name: "Boris", SetType: shared.PtrString(contracts.SetTypeHeadliner)},
		communitym.ShowRequestArtist{Name: "Hallucinated Act", SetType: shared.PtrString(contracts.SetTypeDJ)},
	)
	h := s.rescueHandler(orphan)

	headliner := contracts.SetTypeHeadliner
	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Rebel Lounge", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris", SetType: &headliner}}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	var names []string
	s.Require().NoError(s.deps.DB.
		Table("show_artists").
		Select("artists.name").
		Joins("JOIN artists ON artists.id = show_artists.artist_id").
		Where("show_artists.show_id = ?", *resp.Body.CreatedEntityID).
		Order("show_artists.position ASC").
		Scan(&names).Error)
	s.Equal([]string{"Boris"}, names, "the act the admin dropped must not come back from the payload")

	var strays int64
	s.Require().NoError(s.deps.DB.Model(&catalogm.Artist{}).
		Where("name = ?", "Hallucinated Act").Count(&strays).Error)
	s.Zero(strays, "a dropped act must not even be find-or-created")
}
