package community

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	communitym "psychic-homily-backend/internal/models/community"
)

// ============================================================================
// User: Queue many entity-creation requests at once — POST /entity-requests/batch
// ============================================================================

// maxEntityRequestBatchItems caps one batch. It is the paste flow's own design
// size: AddItemsPicker resolves a whole textarea in one pass and files every
// zero-result line together, and its comments size that textarea at 200 lines.
//
// The cap is stated TWICE: as the items schema's maxItems, which huma refuses an
// oversized body against before the handler runs, and as the handler's own
// guard, which is what a caller reaching the handler directly meets. A struct tag
// cannot be built from a constant, so TestBatchItemCapMatchesTheSchema is what
// holds the two spellings together.
const maxEntityRequestBatchItems = 200

// EntityRequestBatchItem is one contributor submission as it arrives, before any
// of it has been checked. Field for field the single route's body, so a producer
// that can build one can build the other, and it is what submitEntityRequest
// takes for BOTH routes: the single route's body is converted to one.
type EntityRequestBatchItem struct {
	EntityType string `json:"entity_type" doc:"Entity type to request (artist, venue, label, release, show, festival)"`
	// The payload doc string is the single route's, verbatim: the payload is
	// json.RawMessage on both routes, so nothing about its shape reaches the
	// generated OpenAPI document and the doc string is the whole contract a
	// producer author sees. TestBatchPayloadDocMatchesTheSingleRoute pins the two
	// together.
	Payload       json.RawMessage                       `json:"payload" doc:"Typed creation payload for the entity_type. The name (or title) is required on every type and must be 255 characters or fewer; a venue's city must be 100 characters or fewer and its state 10; a festival's edition_year must be between 0 and 9999, where 0 or an absent value means the edition year is taken from start_date. Lengths count CHARACTERS and are measured before trimming, so trailing whitespace counts. A show payload may carry the bill as artists: [{name, set_type?}], name only, no id, at most 50 acts. A payload bill NEVER infers a headliner from list order: an act with no set_type is stored as 'performer', so a bill naming no 'headliner' creates a show with no headliner row. State set_type 'headliner' explicitly when the source names one. When set_type is present it must be one of: headliner,direct_support,opener,special_guest,dj,performer."`
	SourceContext string                                `json:"source_context" required:"false" doc:"How the request originated (ai_extraction, paste_mode, manual); defaults to manual"`
	SourceDetail  *communitym.EntityRequestSourceDetail `json:"source_detail" required:"false" doc:"Optional origin context (source URL + excerpt), chiefly for AI extraction; shown in the admin moderation queue"`
	Confirmed     bool                                  `json:"confirmed" required:"false" doc:"FE-side confirm step (only relevant to trusted_contributor tier)"`
}

// CreateEntityRequestBatchRequest is the Huma request for
// POST /entity-requests/batch.
type CreateEntityRequestBatchRequest struct {
	Body struct {
		Items []EntityRequestBatchItem `json:"items" minItems:"1" maxItems:"200" doc:"The submissions to file, at most 200. Each is validated, deduped and stored on its own: a refused item never withholds its siblings, and every item has exactly one result at its own index."`
	}
}

// Per-item outcomes. A batch answers 200 whatever the items did, so these are
// the only place an item's fate is stated.
const (
	// entityRequestBatchCreated: this submission filed a NEW row.
	entityRequestBatchCreated = "created"
	// entityRequestBatchReplaced: this submission landed on the requester's own
	// colliding pending row and overwrote its submission, the single route's
	// replaced: true.
	entityRequestBatchReplaced = "replaced"
	// entityRequestBatchRefused: nothing was stored for this submission, and
	// error says why in the words the single route would have answered with.
	entityRequestBatchRefused = "refused"
)

// EntityRequestBatchResult is one item's outcome, at the index it was sent at.
//
// created/replaced carry the same three facts the single route's body carries
// about a stored row: its id, the state it is in, and the catalog entity an
// auto-approving tier's request was fulfilled into. A caller that renders a
// per-row chip from the single route reads the same fields here.
type EntityRequestBatchResult struct {
	Index  int    `json:"index" doc:"The zero-based position of this item in the request's items array. Results are returned in that order and every item has exactly one."`
	Status string `json:"status" enum:"created,replaced,refused" doc:"created = a new request was filed; replaced = this submission overwrote the requester's own queued request under the same dedup key (a correction); refused = nothing was stored, see error."`
	ID     *uint  `json:"id,omitempty" doc:"The stored request's id. Present on created and replaced, absent on refused. On a replacement it is the id of the row that already existed."`
	// DecisionState is what the single route's decision_state says: 'pending'
	// for a queued request, 'approved' when the requester's tier auto-approves.
	DecisionState *string `json:"decision_state,omitempty" doc:"The stored request's decision_state (pending or approved). Absent on refused."`
	// CreatedEntityID mirrors the single route's: an auto-approved request is
	// fulfilled inline, and this is the catalog entity it created. Absent when
	// the request is queued, and absent for an auto-approved show, whose catalog
	// create needs associations only an admin supplies.
	CreatedEntityID *uint   `json:"created_entity_id,omitempty" doc:"The catalog entity an auto-approved request was fulfilled into. Absent for a queued request."`
	Error           *string `json:"error,omitempty" doc:"Why this item was refused, in the single route's own words. Present only on refused."`
	// ErrorStatus is the HTTP status the single route would have answered this
	// submission with. It is what tells a client whether re-sending the same item
	// can ever succeed: a 4xx is the item's own content and will be refused again,
	// a 5xx is not.
	ErrorStatus int `json:"error_status,omitempty" doc:"The HTTP status the single route would have answered this item with (422 for a rejected payload, 409 for a conflicting catalog entity, 500 for a server fault). Present only on refused."`
}

// CreateEntityRequestBatchResponse is the Huma response for the batch route.
type CreateEntityRequestBatchResponse struct {
	Body struct {
		Results []EntityRequestBatchResult `json:"results" doc:"One result per submitted item, in the order the items were sent."`
	}
}

// CreateEntityRequestBatchHandler handles POST /entity-requests/batch.
//
// It files each item through submitEntityRequest, the same function the single
// route files its one submission through, so validation, the dedup key and the
// replace-on-resubmit contract are not restated here and cannot diverge from the
// single route's.
//
// EACH ITEM IS ITS OWN UNIT OF WORK. There is no batch-wide transaction: an item
// that is refused leaves its siblings stored, and a batch of 200 in which one
// name is oversized files 199 rows and reports one refusal. The alternative,
// refusing the whole paste over one bad line, is the behaviour this route exists
// to remove.
//
// The response is 200 whenever the batch itself was well-formed, including when
// every item was refused. An item's fate is on its own result, never on the
// status line: a batch has no single outcome to report there.
//
// One audit row per item, written by submitEntityRequest, so a batch is
// indistinguishable from the same submissions filed one at a time in the audit
// log and on the contributions timeline.
func (h *EntityRequestHandler) CreateEntityRequestBatchHandler(ctx context.Context, req *CreateEntityRequestBatchRequest) (*CreateEntityRequestBatchResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	items := req.Body.Items
	if len(items) == 0 {
		return nil, huma.Error422UnprocessableEntity("items is required and must contain at least one entry")
	}
	if len(items) > maxEntityRequestBatchItems {
		return nil, huma.Error422UnprocessableEntity(
			"A batch may carry at most " + strconv.Itoa(maxEntityRequestBatchItems) + " items")
	}

	results := make([]EntityRequestBatchResult, 0, len(items))
	for i := range items {
		created, replaced, err := h.submitEntityRequest(ctx, user, items[i])
		if err != nil {
			results = append(results, refusedBatchResult(i, err))
			continue
		}

		status := entityRequestBatchCreated
		if replaced {
			status = entityRequestBatchReplaced
		}
		id := created.ID
		state := string(created.DecisionState)
		results = append(results, EntityRequestBatchResult{
			Index:           i,
			Status:          status,
			ID:              &id,
			DecisionState:   &state,
			CreatedEntityID: created.CreatedEntityID,
		})
	}

	resp := &CreateEntityRequestBatchResponse{}
	resp.Body.Results = results
	return resp, nil
}

// refusedBatchResult converts one item's error into its result.
//
// submitEntityRequest returns HTTP errors, so the status and the message a
// caller would have received from the single route are both readable off the
// error itself. An error that is not a huma.StatusError is a fault this handler
// did not anticipate, and it reads as a 500 rather than being reported with a
// status it does not have.
func refusedBatchResult(index int, err error) EntityRequestBatchResult {
	message := err.Error()
	status := 500
	var statusErr huma.StatusError
	if errors.As(err, &statusErr) {
		status = statusErr.GetStatus()
	}
	return EntityRequestBatchResult{
		Index:       index,
		Status:      entityRequestBatchRefused,
		Error:       &message,
		ErrorStatus: status,
	}
}
