package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Tests: batch queue-create — POST /entity-requests/batch (PSY-2005)
//
// These share erUserCtx / artistPayload / pendingRequest / approvedRequest /
// supersededSubmission with entity_request_test.go on purpose: the route's whole
// claim is that an item is treated exactly as the single route treats a body, and
// two fixture sets would let the two drift while both suites stayed green.
// ============================================================================

// batchRequest builds a batch body from artist payloads, the shape the paste flow
// sends.
func batchRequest(t *testing.T, names ...string) *CreateEntityRequestBatchRequest {
	t.Helper()
	req := &CreateEntityRequestBatchRequest{}
	for _, name := range names {
		req.Body.Items = append(req.Body.Items, EntityRequestSubmission{
			EntityType:    "artist",
			Payload:       artistPayload(t, name),
			SourceContext: communitym.EntityRequestSourcePasteMode,
		})
	}
	return req
}

func TestCreateEntityRequestBatch_NoUser(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	_, err := h.CreateEntityRequestBatchHandler(context.Background(), batchRequest(t, "Boris"))
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCreateEntityRequestBatch_EmptyItems(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	_, err := h.CreateEntityRequestBatchHandler(erUserCtx(), &CreateEntityRequestBatchRequest{})
	testhelpers.AssertHumaError(t, err, 422)
}

// The cap refuses the whole batch rather than truncating it: a caller that sent
// 201 lines must not be told 200 of them were filed and left to work out which.
func TestCreateEntityRequestBatch_OverTheCapRefusesTheWholeBatch(t *testing.T) {
	names := make([]string, maxEntityRequestSubmissions+1)
	for i := range names {
		names[i] = fmt.Sprintf("Artist %d", i)
	}
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				t.Fatal("no item may be stored when the batch itself is refused")
				return nil, nil, nil
			},
		},
		nil, nil,
	)
	_, err := h.CreateEntityRequestBatchHandler(erUserCtx(), batchRequest(t, names...))
	testhelpers.AssertHumaError(t, err, 422)
}

// A batch AT the cap is accepted: the guard is > and not >=, and an off-by-one
// there refuses exactly the paste size the route was built for.
func TestCreateEntityRequestBatch_AtTheCapIsAccepted(t *testing.T) {
	names := make([]string, maxEntityRequestSubmissions)
	for i := range names {
		names[i] = fmt.Sprintf("Artist %d", i)
	}
	var mu sync.Mutex
	stored := 0
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				mu.Lock()
				stored++
				id := uint(stored)
				mu.Unlock()
				return pendingRequest(id, "artist"), nil, nil
			},
		},
		nil, nil,
	)
	resp, err := h.CreateEntityRequestBatchHandler(erUserCtx(), batchRequest(t, names...))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Results) != maxEntityRequestSubmissions {
		t.Fatalf("expected %d results, got %d", maxEntityRequestSubmissions, len(resp.Body.Results))
	}
}

// The handler guard and the schema tag state the same number. A struct tag cannot
// be built from a constant, so nothing but this test holds them together, and a
// silent disagreement means huma refuses a batch the handler would have accepted
// (or the reverse).
func TestBatchItemCapMatchesTheSchema(t *testing.T) {
	body := reflect.TypeOf(CreateEntityRequestBatchRequest{}).Field(0).Type
	items, ok := body.FieldByName("Items")
	if !ok {
		t.Fatal("the batch body must carry an Items field")
	}
	if got := items.Tag.Get("maxItems"); got != strconv.Itoa(maxEntityRequestSubmissions) {
		t.Errorf("schema maxItems is %q but the handler caps at %d", got, maxEntityRequestSubmissions)
	}
	if got := items.Tag.Get("minItems"); got != "1" {
		t.Errorf("an empty batch is refused by the handler, so the schema must say minItems 1, got %q", got)
	}
}

// The two routes accept the SAME submission type, so the payload doc string, the
// field set and every tag are one declaration rather than two kept in step. This
// is what stands in for the drift test that would otherwise be needed: a field or
// a doc string can no longer differ between them, because there is only one.
func TestBothRoutesAcceptTheSameSubmissionType(t *testing.T) {
	body, ok := reflect.TypeOf(CreateEntityRequestRequest{}).FieldByName("Body")
	if !ok {
		t.Fatal("the single route must carry a Body field")
	}
	items, ok := reflect.TypeOf(CreateEntityRequestBatchRequest{}).Field(0).Type.FieldByName("Items")
	if !ok {
		t.Fatal("the batch body must carry an Items field")
	}
	submission := reflect.TypeOf(EntityRequestSubmission{})
	if body.Type != submission {
		t.Errorf("the single route's body must BE the submission type, got %s", body.Type)
	}
	if items.Type.Elem() != submission {
		t.Errorf("a batch item must BE the submission type, got %s", items.Type.Elem())
	}
}

// ============================================================================
// Tests: per-item results
// ============================================================================

// The route's central claim: one refused item files its siblings anyway, every
// item is answered at its own index, and nothing is silently dropped.
func TestCreateEntityRequestBatch_ARefusedItemDoesNotBlockItsSiblings(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(_ *authm.User, _ string, payload []byte, _ string, _ []byte, _ bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				mu.Lock()
				seen = append(seen, string(payload))
				id := uint(len(seen))
				mu.Unlock()
				return pendingRequest(id, "artist"), nil, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)

	req := batchRequest(t, "Boris")
	// An empty name is refused by the payload validator, exactly as it is on the
	// single route (TestCreateEntityRequest_PayloadMissingRequiredField).
	req.Body.Items = append(req.Body.Items, EntityRequestSubmission{
		EntityType: "artist",
		Payload:    json.RawMessage(`{"name":""}`),
	})
	req.Body.Items = append(req.Body.Items, EntityRequestSubmission{
		EntityType: "artist",
		Payload:    artistPayload(t, "Earth"),
	})

	resp, err := h.CreateEntityRequestBatchHandler(erUserCtx(), req)
	if err != nil {
		t.Fatalf("a batch whose items were refused is still a well-formed batch: %v", err)
	}
	if len(resp.Body.Results) != 3 {
		t.Fatalf("expected one result per item, got %d", len(resp.Body.Results))
	}
	for i, r := range resp.Body.Results {
		if r.Index != i {
			t.Errorf("result %d reports index %d", i, r.Index)
		}
	}
	if resp.Body.Results[0].Status != entityRequestBatchCreated {
		t.Errorf("item 0 should be created, got %s", resp.Body.Results[0].Status)
	}
	if resp.Body.Results[1].Status != entityRequestBatchRefused {
		t.Errorf("item 1 should be refused, got %s", resp.Body.Results[1].Status)
	}
	if resp.Body.Results[1].ErrorStatus != 422 {
		t.Errorf("a rejected payload is the 422 the single route answers, got %d",
			resp.Body.Results[1].ErrorStatus)
	}
	if resp.Body.Results[1].Error == nil || *resp.Body.Results[1].Error == "" {
		t.Error("a refused item must say why")
	}
	if resp.Body.Results[1].ID != nil {
		t.Error("a refused item stored nothing, so it carries no id")
	}
	if resp.Body.Results[2].Status != entityRequestBatchCreated {
		t.Errorf("the item AFTER a refusal must still be filed, got %s", resp.Body.Results[2].Status)
	}
	if len(seen) != 2 {
		t.Errorf("the refused item must never reach the service; stored %d payloads", len(seen))
	}
}

// A replacement is reported as such per item, the same fact the single route
// carries as replaced: true. A client that reads every stored item as a first
// filing cannot tell a correction landed.
func TestCreateEntityRequestBatch_ReplacementIsReportedPerItem(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(_ *authm.User, _ string, payload []byte, _ string, _ []byte, _ bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				if string(payload) == string(artistPayload(t, "Boris")) {
					return pendingRequest(7, "artist"), supersededSubmission(), nil
				}
				return pendingRequest(8, "artist"), nil, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)

	resp, err := h.CreateEntityRequestBatchHandler(erUserCtx(), batchRequest(t, "Boris", "Earth"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Results[0].Status != entityRequestBatchReplaced {
		t.Errorf("expected replaced, got %s", resp.Body.Results[0].Status)
	}
	if resp.Body.Results[0].ID == nil || *resp.Body.Results[0].ID != 7 {
		t.Error("a replacement carries the queued request's own id")
	}
	if resp.Body.Results[1].Status != entityRequestBatchCreated {
		t.Errorf("expected created, got %s", resp.Body.Results[1].Status)
	}
}

// An auto-approving tier's item carries the catalog entity it was fulfilled into,
// which is what lets a caller stage the new entity in the same step. Without it
// the batch route would be unusable by the AI filler, whose whole create-and-add
// flow reads created_entity_id.
func TestCreateEntityRequestBatch_AutoApprovedItemCarriesTheCreatedEntity(t *testing.T) {
	approved := approvedRequest(9, "artist")
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				return approved, nil, nil
			},
			RecordFulfillmentFn: func(uint, uint) error { return nil },
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return &contracts.ArtistDetailResponse{ID: 42}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	resp, err := h.CreateEntityRequestBatchHandler(erAdminCtx(), batchRequest(t, "Boris"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := resp.Body.Results[0]
	if got.Status != entityRequestBatchCreated {
		t.Errorf("expected created, got %s", got.Status)
	}
	if got.DecisionState == nil || *got.DecisionState != string(communitym.EntityRequestStateApproved) {
		t.Error("an auto-approved item must report its approved state")
	}
	if got.CreatedEntityID == nil || *got.CreatedEntityID != 42 {
		t.Error("an auto-approved item must carry the catalog entity it created")
	}
}

// One audit row per item, so a batch leaves the same trail the same submissions
// filed one at a time would.
func TestCreateEntityRequestBatch_WritesOneAuditRowPerItem(t *testing.T) {
	written := make(chan uint, 4)
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(_ *authm.User, _ string, payload []byte, _ string, _ []byte, _ bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				if string(payload) == string(artistPayload(t, "Boris")) {
					return pendingRequest(7, "artist"), nil, nil
				}
				return pendingRequest(8, "artist"), nil, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{
			LogActionFn: func(_ uint, _ string, _ string, entityID uint, _ map[string]interface{}) {
				written <- entityID
			},
		},
	)

	if _, err := h.CreateEntityRequestBatchHandler(erUserCtx(), batchRequest(t, "Boris", "Earth")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := map[uint]bool{}
	for i := 0; i < 2; i++ {
		got[<-written] = true
	}
	if !got[7] || !got[8] {
		t.Errorf("expected an audit row naming each stored request, got %v", got)
	}
}

// ============================================================================
// Tests: the image-host guard, per item
// ============================================================================

// artistPayloadWithImage builds an artist payload carrying an image_url, the one
// payload field the share-card renderer fetches server-side.
func artistPayloadWithImage(t *testing.T, name, imageURL string) json.RawMessage {
	t.Helper()
	url := imageURL
	raw, err := communitym.MarshalPayload(communitym.ArtistRequestPayload{
		Name:     name,
		ImageURL: &url,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

// The literal address forms are refused PER ITEM and the siblings are still
// filed. This is the half of the SSRF guard the batch route runs itself, and it
// is the half an attacker writes directly.
func TestCreateEntityRequestBatch_RefusesALiteralInternalImageHostPerItem(t *testing.T) {
	var stored []string
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(_ *authm.User, _ string, payload []byte, _ string, _ []byte, _ bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				stored = append(stored, string(payload))
				return pendingRequest(uint(len(stored)), "artist"), nil, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)

	req := &CreateEntityRequestBatchRequest{}
	req.Body.Items = []EntityRequestSubmission{
		{EntityType: "artist", Payload: artistPayload(t, "Fine One")},
		{
			EntityType: "artist",
			Payload:    artistPayloadWithImage(t, "Hostile", "http://169.254.169.254/latest/meta-data/"),
		},
		{EntityType: "artist", Payload: artistPayload(t, "Fine Two")},
	}

	resp, err := h.CreateEntityRequestBatchHandler(erUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Results[1].Status != entityRequestBatchRefused {
		t.Errorf("a link-local image host must be refused, got %s", resp.Body.Results[1].Status)
	}
	if resp.Body.Results[1].ErrorStatus != 422 {
		t.Errorf("expected 422, got %d", resp.Body.Results[1].ErrorStatus)
	}
	if resp.Body.Results[0].Status != entityRequestBatchCreated ||
		resp.Body.Results[2].Status != entityRequestBatchCreated {
		t.Error("the refused item must not withhold its siblings")
	}
	if len(stored) != 2 {
		t.Errorf("the hostile item must never reach the service; stored %d", len(stored))
	}
}

// A QUEUEING tier's item does not pay a DNS lookup on the batch route: a queued
// payload is fetched by nothing, and the decide handler resolves it pre-claim
// before an approve can fulfil it. The fulfiller is what proves nothing was
// created here.
func TestCreateEntityRequestBatch_QueuedItemDefersTheHostLookup(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			WillAutoApproveFn: func(*authm.User, bool) bool { return false },
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				return pendingRequest(7, "artist"), nil, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				t.Fatal("a queued request must not be fulfilled")
				return nil, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &CreateEntityRequestBatchRequest{}
	req.Body.Items = []EntityRequestSubmission{{
		EntityType: "artist",
		// The SAME hostname the auto-approving case is refused for: it passes the
		// literal check and the stub resolves it to a metadata address. Storing it
		// here is what proves the resolving check did NOT run on this path.
		Payload: artistPayloadWithImage(t, "Deferred", "https://rebind.example.test/x.jpg"),
	}}

	resp, err := h.CreateEntityRequestBatchHandler(erUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Results[0].Status != entityRequestBatchCreated {
		t.Errorf("expected created, got %s (%v)",
			resp.Body.Results[0].Status, resp.Body.Results[0].Error)
	}
}

// An item that will AUTO-APPROVE is resolved BEFORE the row is written, whatever
// the route's policy. A row is stamped approved before its insert and fulfilled
// into a live entity in the same request, so a check after the insert could only
// refuse a row that already exists - and it would leave that row
// approved-but-unfulfilled holding a value nothing resolved, on the one queue
// whose fulfil path does not re-check it.
//
// The mock service reports the auto-approving tier and fails if the row is ever
// written, which is the whole assertion: refused, and nothing stored.
func TestCreateEntityRequestBatch_AutoApprovingItemIsResolvedBeforeItIsStored(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			WillAutoApproveFn: func(*authm.User, bool) bool { return true },
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				t.Fatal("an auto-approving item must be refused BEFORE its row is written")
				return nil, nil, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				t.Fatal("an unresolved image host must not reach the fulfiller")
				return nil, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &CreateEntityRequestBatchRequest{}
	req.Body.Items = []EntityRequestSubmission{{
		EntityType: "artist",
		// A hostname the LITERAL check passes and the stub resolver answers with
		// a metadata address (see this package's TestMain): only the resolving
		// check refuses it, so a refusal here proves that check ran.
		Payload: artistPayloadWithImage(t, "Hostile", "https://rebind.example.test/x.jpg"),
	}}

	resp, err := h.CreateEntityRequestBatchHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Results[0].Status != entityRequestBatchRefused {
		t.Errorf("expected refused, got %s", resp.Body.Results[0].Status)
	}
	if resp.Body.Results[0].ID != nil {
		t.Error("nothing was stored, so the result carries no id")
	}
}

// A refusal AFTER the row was written says so, by carrying the id. A client told
// only "refused" about a request that exists cannot find it again. This is what
// an auto-approving tier's fulfilment conflict leaves behind: the request is
// approved, its catalog entity was not created, and it is the rescue queue's.
func TestCreateEntityRequestBatch_ARefusalAfterTheWriteCarriesTheRequestID(t *testing.T) {
	approved := approvedRequest(9, "artist")
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			WillAutoApproveFn: func(*authm.User, bool) bool { return true },
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				return approved, nil, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return nil, apperrors.ErrArtistExists("Boris")
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	resp, err := h.CreateEntityRequestBatchHandler(erAdminCtx(), batchRequest(t, "Boris"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := resp.Body.Results[0]
	if got.Status != entityRequestBatchRefused {
		t.Fatalf("expected refused, got %s", got.Status)
	}
	if got.ErrorStatus != 409 {
		t.Errorf("a duplicate catalog entity is the single route's 409, got %d", got.ErrorStatus)
	}
	if got.ID == nil || *got.ID != 9 {
		t.Error("a refusal after the write must name the request that exists")
	}
	if got.DecisionState == nil || *got.DecisionState != string(communitym.EntityRequestStateApproved) {
		t.Error("the stored row's state must be reported alongside its id")
	}
}

// An error that is not an HTTP error at all reads as a 500, which is what keeps a
// client's retryable-versus-terminal split honest: error_status is what the paste
// picker branches on, and a server fault must stay retryable.
func TestCreateEntityRequestBatch_AnUntypedFailureReadsAs500(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				return nil, nil, errors.New("connection reset")
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)

	resp, err := h.CreateEntityRequestBatchHandler(erUserCtx(), batchRequest(t, "Boris"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := resp.Body.Results[0]
	if got.Status != entityRequestBatchRefused {
		t.Fatalf("expected refused, got %s", got.Status)
	}
	if got.ErrorStatus != 500 {
		t.Errorf("expected 500, got %d", got.ErrorStatus)
	}
	if got.ID != nil {
		t.Error("nothing was stored, so the result carries no id")
	}
}

// ============================================================================
// Tests: the batch refuses what the single route refuses
// ============================================================================

// Each case is one the single route answers with a 422 at the same boundary
// (entity_request_test.go asserts the single-route half). Sharing the inputs is
// the point: the batch route's contract is that an item is a body.
func TestCreateEntityRequestBatch_RefusesWhatTheSingleRouteRefuses(t *testing.T) {
	cases := []struct {
		name string
		item EntityRequestSubmission
	}{
		{
			name: "unknown entity type",
			item: EntityRequestSubmission{EntityType: "wizard", Payload: json.RawMessage(`{"name":"x"}`)},
		},
		{
			name: "unknown source context",
			item: EntityRequestSubmission{
				EntityType:    "artist",
				Payload:       artistPayload(t, "Boris"),
				SourceContext: "carrier_pigeon",
			},
		},
		{
			name: "blank payload",
			item: EntityRequestSubmission{EntityType: "artist", Payload: json.RawMessage("   ")},
		},
		{
			name: "payload missing its required field",
			item: EntityRequestSubmission{EntityType: "artist", Payload: json.RawMessage(`{"name":""}`)},
		},
		{
			name: "payload with an unknown field",
			item: EntityRequestSubmission{
				EntityType: "artist",
				Payload:    json.RawMessage(`{"name":"Boris","sneaky":"x"}`),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewEntityRequestHandler(
				&testhelpers.MockEntityRequestService{
					CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
						t.Fatal("service must NOT be called for an item the boundary refuses")
						return nil, nil, nil
					},
				},
				nil,
				&testhelpers.MockAuditLogService{},
			)
			req := &CreateEntityRequestBatchRequest{}
			req.Body.Items = []EntityRequestSubmission{tc.item}

			resp, err := h.CreateEntityRequestBatchHandler(erUserCtx(), req)
			if err != nil {
				t.Fatalf("an item's refusal is per item, not the batch's: %v", err)
			}
			if resp.Body.Results[0].Status != entityRequestBatchRefused {
				t.Fatalf("expected refused, got %s", resp.Body.Results[0].Status)
			}
			if resp.Body.Results[0].ErrorStatus != 422 {
				t.Errorf("expected the single route's 422, got %d", resp.Body.Results[0].ErrorStatus)
			}
		})
	}
}
