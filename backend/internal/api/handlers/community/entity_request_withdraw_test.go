package community

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Tests: withdraw, POST /entity-requests/{id}/withdraw (PSY-1992)
// ============================================================================

func withdrawRequest(id string) *WithdrawEntityRequestRequest {
	return &WithdrawEntityRequestRequest{ID: id}
}

// withdrawnRequest is what the service answers with on a successful withdrawal:
// the row in its new state, decided by the requester themselves.
func withdrawnRequest(id uint) *communitym.EntityRequest {
	r := pendingRequest(id, "artist")
	r.DecisionState = communitym.EntityRequestStateWithdrawn
	requester := r.RequesterID
	r.DecidedBy = &requester
	return r
}

func TestWithdrawEntityRequest_NoUser(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	_, err := h.WithdrawEntityRequestHandler(context.Background(), withdrawRequest("7"))
	testhelpers.AssertHumaError(t, err, 401)
}

func TestWithdrawEntityRequest_InvalidID(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	_, err := h.WithdrawEntityRequestHandler(erUserCtx(), withdrawRequest("not-a-number"))
	testhelpers.AssertHumaError(t, err, 400)
}

// The requester's own pending row withdraws, and the response carries the state
// it landed in rather than the one the caller asked for.
func TestWithdrawEntityRequest_OwnPendingSucceeds(t *testing.T) {
	var gotRequestID, gotRequesterID uint
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			WithdrawFn: func(requestID, requesterID uint) (*communitym.EntityRequest, error) {
				gotRequestID, gotRequesterID = requestID, requesterID
				return withdrawnRequest(7), nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)

	resp, err := h.WithdrawEntityRequestHandler(erUserCtx(), withdrawRequest("7"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRequestID != 7 {
		t.Errorf("expected request 7, got %d", gotRequestID)
	}
	// The handler passes the AUTHENTICATED user as the requester; a client-named
	// one would let anyone withdraw anyone's request.
	if gotRequesterID != 2 {
		t.Errorf("expected the authenticated user (2) as the requester, got %d", gotRequesterID)
	}
	if resp.Body.Request.DecisionState != communitym.EntityRequestStateWithdrawn {
		t.Errorf("expected withdrawn, got %s", resp.Body.Request.DecisionState)
	}
}

// Another user's row and a row that does not exist are ONE answer. A distinct
// status for someone else's row confirms it exists, which turns the id space
// into an oracle for who has requested what.
func TestWithdrawEntityRequest_AnotherUsersRowIs404(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			WithdrawFn: func(requestID, _ uint) (*communitym.EntityRequest, error) {
				return nil, apperrors.ErrEntityRequestNotFound(requestID)
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)
	_, err := h.WithdrawEntityRequestHandler(erUserCtx(), withdrawRequest("7"))
	testhelpers.AssertHumaError(t, err, 404)
}

// The caller's OWN decided row is a 409 naming the state: it is their row, so
// there is nothing to withhold, and it is the only answer that says why the
// affordance did nothing.
func TestWithdrawEntityRequest_OwnDecidedRowIs409(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			WithdrawFn: func(requestID, _ uint) (*communitym.EntityRequest, error) {
				return nil, apperrors.ErrEntityRequestInvalidState(
					requestID, string(communitym.EntityRequestStateApproved))
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)
	_, err := h.WithdrawEntityRequestHandler(erUserCtx(), withdrawRequest("7"))
	testhelpers.AssertHumaError(t, err, 409)
}

// A withdrawal is in the same log as the filing it retracts, under its own
// action so nothing reads it as a moderation decision.
func TestWithdrawEntityRequest_AuditsAsAWithdrawal(t *testing.T) {
	type logged struct {
		actorID    uint
		action     string
		entityType string
		entityID   uint
		metadata   map[string]interface{}
	}
	written := make(chan logged, 1)

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			WithdrawFn: func(uint, uint) (*communitym.EntityRequest, error) {
				return withdrawnRequest(7), nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{
			LogActionFn: func(actorID uint, action, entityType string, entityID uint, md map[string]interface{}) {
				written <- logged{actorID, action, entityType, entityID, md}
			},
		},
	)

	if _, err := h.WithdrawEntityRequestHandler(erUserCtx(), withdrawRequest("7")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := <-written
	if got.action != "withdraw_entity_request" {
		t.Errorf("expected action=withdraw_entity_request, got %s", got.action)
	}
	if got.actorID != 2 {
		t.Errorf("the withdrawal is the requester's own act, got actor %d", got.actorID)
	}
	if got.entityID != 7 {
		t.Errorf("expected the request's id, got %d", got.entityID)
	}
	if got.metadata["decision_state"] != string(communitym.EntityRequestStateWithdrawn) {
		t.Errorf("expected the state it landed in, got %v", got.metadata["decision_state"])
	}
	// The row still holds the payload and the name; restating either here would
	// publish them somewhere with a different readership.
	if _, ok := got.metadata["payload"]; ok {
		t.Error("a withdrawal's audit row must not carry the payload")
	}
}

// ============================================================================
// Tests: the admin queue can ask for withdrawn rows
// ============================================================================

// The list endpoint validates its state filter against the model enum, so a
// value the column can hold but the enum does not name is a 422 the admin queue
// cannot get past. Withdrawn rows would then be unreachable from every surface.
func TestAdminListEntityRequests_AcceptsTheWithdrawnState(t *testing.T) {
	if !communitym.IsValidEntityRequestState(string(communitym.EntityRequestStateWithdrawn)) {
		t.Fatal("withdrawn must be a recognized decision_state")
	}

	var gotState string
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			ListRequestsFn: func(filters *contracts.EntityRequestFilters) ([]communitym.EntityRequest, int64, error) {
				gotState = filters.State
				return nil, 0, nil
			},
		},
		nil, nil,
	)

	req := &AdminListEntityRequestsRequest{State: string(communitym.EntityRequestStateWithdrawn)}
	if _, err := h.AdminListEntityRequestsHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotState != string(communitym.EntityRequestStateWithdrawn) {
		t.Errorf("expected the withdrawn filter to reach the query, got %q", gotState)
	}
}

// The queue's DEFAULT view excludes withdrawn rows, because the service defaults
// an empty filter to pending. Pinned here rather than assumed: a default that
// widened would put every retracted request back in front of a moderator.
func TestAdminListEntityRequests_DefaultsToPendingOnly(t *testing.T) {
	var gotState string
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			ListRequestsFn: func(filters *contracts.EntityRequestFilters) ([]communitym.EntityRequest, int64, error) {
				gotState = filters.State
				return nil, 0, nil
			},
		},
		nil, nil,
	)

	if _, err := h.AdminListEntityRequestsHandler(erAdminCtx(), &AdminListEntityRequestsRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The handler passes the filter through untouched; the service is what
	// substitutes pending for an empty state (entityrequest_list.go).
	if gotState != "" {
		t.Errorf("the handler must not invent a state filter, got %q", gotState)
	}
}
