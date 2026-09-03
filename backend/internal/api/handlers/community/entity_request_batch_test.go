package community

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
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
		req.Body.Items = append(req.Body.Items, EntityRequestBatchItem{
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
	names := make([]string, maxEntityRequestBatchItems+1)
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
	names := make([]string, maxEntityRequestBatchItems)
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
	if len(resp.Body.Results) != maxEntityRequestBatchItems {
		t.Fatalf("expected %d results, got %d", maxEntityRequestBatchItems, len(resp.Body.Results))
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
	if got := items.Tag.Get("maxItems"); got != strconv.Itoa(maxEntityRequestBatchItems) {
		t.Errorf("schema maxItems is %q but the handler caps at %d", got, maxEntityRequestBatchItems)
	}
	if got := items.Tag.Get("minItems"); got != "1" {
		t.Errorf("an empty batch is refused by the handler, so the schema must say minItems 1, got %q", got)
	}
}

// The payload doc string is the only contract a producer sees for either route
// (the payload is json.RawMessage, so its shape never reaches the OpenAPI
// document). Two spellings of it would let one route document a rule the other
// does not enforce.
func TestBatchPayloadDocMatchesTheSingleRoute(t *testing.T) {
	single, ok := reflect.TypeOf(CreateEntityRequestRequest{}).Field(0).Type.FieldByName("Payload")
	if !ok {
		t.Fatal("the single route's body must carry a Payload field")
	}
	batch, ok := reflect.TypeOf(EntityRequestBatchItem{}).FieldByName("Payload")
	if !ok {
		t.Fatal("a batch item must carry a Payload field")
	}
	if single.Tag.Get("doc") != batch.Tag.Get("doc") {
		t.Error("the batch item's payload doc must be the single route's, verbatim")
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
	req.Body.Items = append(req.Body.Items, EntityRequestBatchItem{
		EntityType: "artist",
		Payload:    json.RawMessage(`{"name":""}`),
	})
	req.Body.Items = append(req.Body.Items, EntityRequestBatchItem{
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
// Tests: the batch refuses what the single route refuses
// ============================================================================

// Each case is one the single route answers with a 422 at the same boundary
// (entity_request_test.go asserts the single-route half). Sharing the inputs is
// the point: the batch route's contract is that an item is a body.
func TestCreateEntityRequestBatch_RefusesWhatTheSingleRouteRefuses(t *testing.T) {
	cases := []struct {
		name string
		item EntityRequestBatchItem
	}{
		{
			name: "unknown entity type",
			item: EntityRequestBatchItem{EntityType: "wizard", Payload: json.RawMessage(`{"name":"x"}`)},
		},
		{
			name: "unknown source context",
			item: EntityRequestBatchItem{
				EntityType:    "artist",
				Payload:       artistPayload(t, "Boris"),
				SourceContext: "carrier_pigeon",
			},
		},
		{
			name: "blank payload",
			item: EntityRequestBatchItem{EntityType: "artist", Payload: json.RawMessage("   ")},
		},
		{
			name: "payload missing its required field",
			item: EntityRequestBatchItem{EntityType: "artist", Payload: json.RawMessage(`{"name":""}`)},
		},
		{
			name: "payload with an unknown field",
			item: EntityRequestBatchItem{
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
			req.Body.Items = []EntityRequestBatchItem{tc.item}

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
