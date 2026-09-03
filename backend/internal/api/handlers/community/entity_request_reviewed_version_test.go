package community

import (
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1974: the decide body carries the version of the row the caller reviewed,
// so a payload the requester replaced while the queue was on screen is refused
// rather than decided.

// reviewedAt is a stored-looking version: microsecond resolution, which is what
// a timestamptz column holds and therefore what a client echoes back.
var reviewedAt = time.Date(2026, 9, 3, 18, 4, 5, 123456000, time.UTC)

// pendingAt is a pending row stamped with a given version, as the pre-claim read
// returns it.
func pendingAt(id uint, entityType string, updatedAt time.Time) *communitym.EntityRequest {
	r := pendingRequest(id, entityType)
	r.UpdatedAt = updatedAt
	return r
}

func TestAdminDecide_ApproveWithStaleClientVersion_Is409(t *testing.T) {
	decideCalled := false
	createCalled := false
	// A claim that succeeds, so removing the refusal fails this test on its
	// assertions rather than on a nil dereference downstream.
	claimed := pendingAt(7, "artist", reviewedAt.Add(time.Second))
	claimed.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(uint) (*communitym.EntityRequest, error) {
				return pendingAt(7, "artist", reviewedAt.Add(time.Second)), nil
			},
			DecideFn: func(uint, uint, communitym.EntityRequestDecisionState, *string, *time.Time) (*communitym.EntityRequest, error) {
				decideCalled = true
				return claimed, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				createCalled = true
				return &contracts.ArtistDetailResponse{ID: 1}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "7"}
	req.Body.Decision = "approved"
	req.Body.ExpectedUpdatedAt = &reviewedAt

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
	if decideCalled {
		t.Error("a revised row must be refused before the claim, not by it")
	}
	if createCalled {
		t.Error("no entity may be created from a payload the caller never read")
	}
}

// The refusal beats the pre-claim payload guards. Ordering matters: the last of
// them resolves DNS, and a caller holding a version the row no longer has is
// already answerable from the read.
func TestAdminDecide_StaleClientVersionRefusesAheadOfThePayloadGuards(t *testing.T) {
	hostile := "http://169.254.169.254/latest/meta-data/"
	payload, err := communitym.MarshalPayload(communitym.ArtistRequestPayload{
		Name:     "Boris",
		ImageURL: &hostile,
	})
	if err != nil {
		t.Fatalf("marshal artist payload: %v", err)
	}
	stored := pendingAt(8, "artist", reviewedAt.Add(time.Second))
	stored.Payload = &payload

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(uint) (*communitym.EntityRequest, error) { return stored, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "8"}
	req.Body.Decision = "approved"
	req.Body.ExpectedUpdatedAt = &reviewedAt

	// 409, not the 422 the stored image_url would otherwise earn.
	_, derr := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, derr, 409)
}

// The ticket's torn show: a bill adopted from payload A fulfilled against the
// scalars of payload B. The version refuses the whole approve, so no show is
// created from two payloads. Driven on a SHOW with use_payload_artists, because
// that is the only shape where the adopted bill and the scalars are read
// separately.
func TestAdminDecide_StaleClientVersionRefusesAnAdoptedShowBill(t *testing.T) {
	headliner := "headliner"
	payload, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title:     "Repro Night",
		EventDate: "2026-11-14T21:00:00-07:00",
		Artists:   []communitym.ShowRequestArtist{{Name: "Boris", SetType: &headliner}},
	})
	if err != nil {
		t.Fatalf("marshal show payload: %v", err)
	}
	stored := pendingAt(13, "show", reviewedAt.Add(time.Second))
	stored.Payload = &payload

	decideCalled := false
	createCalled := false
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(uint) (*communitym.EntityRequest, error) { return stored, nil },
			DecideFn: func(uint, uint, communitym.EntityRequestDecisionState, *string, *time.Time) (*communitym.EntityRequest, error) {
				decideCalled = true
				return stored, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(*contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				createCalled = true
				return &contracts.ShowResponse{ID: 1}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "13"}
	req.Body.Decision = "approved"
	req.Body.ExpectedUpdatedAt = &reviewedAt
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.UsePayloadArtists = true

	_, derr := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, derr, 409)
	if decideCalled {
		t.Error("the row must not be claimed against a payload the caller never read")
	}
	if createCalled {
		t.Error("no show may be assembled from a bill and scalars the caller never read together")
	}
}

func TestAdminDecide_ApproveWithMatchingClientVersion_ClaimsWithIt(t *testing.T) {
	var claimedWith *time.Time
	decided := pendingAt(9, "artist", reviewedAt)
	decided.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(uint) (*communitym.EntityRequest, error) {
				return pendingAt(9, "artist", reviewedAt), nil
			},
			DecideFn: func(_, _ uint, _ communitym.EntityRequestDecisionState, _ *string, expectedUpdatedAt *time.Time) (*communitym.EntityRequest, error) {
				claimedWith = expectedUpdatedAt
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return &contracts.ArtistDetailResponse{ID: 42}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "9"}
	req.Body.Decision = "approved"
	req.Body.ExpectedUpdatedAt = &reviewedAt

	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimedWith == nil || !claimedWith.Equal(reviewedAt) {
		t.Errorf("expected the claim to carry the reviewed version %v, got %v", reviewedAt, claimedWith)
	}
}

// With no version on the body the handler still defends its OWN read, which is
// the guarantee PSY-1948 shipped and this ticket must not narrow.
func TestAdminDecide_ApproveWithoutClientVersion_ClaimsWithTheServersRead(t *testing.T) {
	var claimedWith *time.Time
	serverRead := reviewedAt.Add(90 * time.Second)
	decided := pendingAt(10, "artist", serverRead)
	decided.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(uint) (*communitym.EntityRequest, error) {
				return pendingAt(10, "artist", serverRead), nil
			},
			DecideFn: func(_, _ uint, _ communitym.EntityRequestDecisionState, _ *string, expectedUpdatedAt *time.Time) (*communitym.EntityRequest, error) {
				claimedWith = expectedUpdatedAt
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return &contracts.ArtistDetailResponse{ID: 43}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "10"}
	req.Body.Decision = "approved"

	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimedWith == nil || !claimedWith.Equal(serverRead) {
		t.Errorf("expected the claim to carry the server's read %v, got %v", serverRead, claimedWith)
	}
}

// A rejection takes no pre-claim read, so its version reaches the claim
// untouched. Rejecting a submission that replaced the one the admin read refuses
// a correction nobody looked at.
func TestAdminDecide_RejectCarriesTheClientVersionToTheClaim(t *testing.T) {
	var claimedWith *time.Time
	getCalled := false
	rejected := pendingAt(11, "artist", reviewedAt)
	rejected.DecisionState = communitym.EntityRequestStateRejected

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(uint) (*communitym.EntityRequest, error) {
				getCalled = true
				return pendingAt(11, "artist", reviewedAt), nil
			},
			DecideFn: func(_, _ uint, _ communitym.EntityRequestDecisionState, _ *string, expectedUpdatedAt *time.Time) (*communitym.EntityRequest, error) {
				claimedWith = expectedUpdatedAt
				return rejected, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "11"}
	req.Body.Decision = "rejected"
	note := "not a real band"
	req.Body.Note = &note
	req.Body.ExpectedUpdatedAt = &reviewedAt

	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if getCalled {
		t.Error("a rejection reads nothing off the row; it must not take the pre-claim read")
	}
	if claimedWith == nil || !claimedWith.Equal(reviewedAt) {
		t.Errorf("expected the claim to carry the reviewed version %v, got %v", reviewedAt, claimedWith)
	}
}

// An ALREADY-DECIDED row answers with its own state, not with the stale
// conflict: "someone decided this" and "the requester revised this" are
// different instructions, and only the second one says re-read and decide again.
func TestAdminDecide_DecidedRowAnswersItsStateNotStale(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(uint) (*communitym.EntityRequest, error) {
				r := pendingAt(12, "artist", reviewedAt.Add(time.Second))
				r.DecisionState = communitym.EntityRequestStateApproved
				return r, nil
			},
			DecideFn: func(requestID, _ uint, _ communitym.EntityRequestDecisionState, _ *string, _ *time.Time) (*communitym.EntityRequest, error) {
				return nil, apperrors.ErrEntityRequestInvalidState(requestID, "approved")
			},
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "12"}
	req.Body.Decision = "approved"
	req.Body.ExpectedUpdatedAt = &reviewedAt

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
	if err == nil || !strings.Contains(err.Error(), "expected pending") {
		t.Errorf("expected the already-decided conflict, got %v", err)
	}
}
