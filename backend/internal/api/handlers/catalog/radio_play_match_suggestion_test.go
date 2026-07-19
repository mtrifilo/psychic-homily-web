package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func matchSuggestionUserCtx(userID uint, isAdmin bool) context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: userID, IsAdmin: isAdmin})
}

func TestCreateRadioPlayMatchSuggestionHandler_Success(t *testing.T) {
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		CreateSuggestionFn: func(playID, submitterID uint, req *contracts.CreateRadioPlayMatchSuggestionRequest) (*contracts.RadioPlayMatchSuggestionEntry, error) {
			return &contracts.RadioPlayMatchSuggestionEntry{
				ID: playID, PlayID: playID, SuggestedArtistID: req.ArtistID,
				SubmittedBy: submitterID, Status: "pending",
			}, nil
		},
	}, nil)

	resp, err := h.CreateRadioPlayMatchSuggestionHandler(
		matchSuggestionUserCtx(7, false),
		&CreateRadioPlayMatchSuggestionRequest{
			PlayID: 42,
			Body:   contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: 9},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "pending" || resp.Body.SuggestedArtistID != 9 {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
}

func TestCreateRadioPlayMatchSuggestionHandler_DuplicatePending(t *testing.T) {
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		CreateSuggestionFn: func(uint, uint, *contracts.CreateRadioPlayMatchSuggestionRequest) (*contracts.RadioPlayMatchSuggestionEntry, error) {
			return nil, contracts.ErrRadioPlayMatchSuggestionDuplicatePending
		},
	}, nil)
	_, err := h.CreateRadioPlayMatchSuggestionHandler(
		matchSuggestionUserCtx(1, false),
		&CreateRadioPlayMatchSuggestionRequest{PlayID: 1, Body: contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: 2}},
	)
	testhelpers.AssertHumaError(t, err, 409)
}

func TestCreateRadioPlayMatchSuggestionHandler_NotSuggestable(t *testing.T) {
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		CreateSuggestionFn: func(uint, uint, *contracts.CreateRadioPlayMatchSuggestionRequest) (*contracts.RadioPlayMatchSuggestionEntry, error) {
			return nil, contracts.ErrRadioPlayMatchSuggestionPlayNotSuggestable
		},
	}, nil)
	_, err := h.CreateRadioPlayMatchSuggestionHandler(
		matchSuggestionUserCtx(1, false),
		&CreateRadioPlayMatchSuggestionRequest{PlayID: 1, Body: contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: 2}},
	)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAcceptRadioPlayMatchSuggestionHandler_Success(t *testing.T) {
	now := time.Now().UTC()
	reviewer := uint(1)
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		AcceptSuggestionFn: func(suggestionID, reviewerUserID uint, req *contracts.AcceptRadioPlayMatchSuggestionRequest) (*contracts.RadioPlayMatchSuggestionReviewResult, error) {
			return &contracts.RadioPlayMatchSuggestionReviewResult{
				ID: suggestionID, PlayID: 10, SuggestedArtistID: 3, SubmittedBy: 7,
				Status: "accepted", ReviewedAt: &now, ReviewedBy: &reviewerUserID,
			}, nil
		},
	}, nil)

	resp, err := h.AcceptRadioPlayMatchSuggestionHandler(
		matchSuggestionUserCtx(reviewer, true),
		&AcceptRadioPlayMatchSuggestionRequest{ID: "5", Body: contracts.AcceptRadioPlayMatchSuggestionRequest{}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "accepted" || resp.Body.ID != 5 {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
}

func TestAcceptRadioPlayMatchSuggestionHandler_AlreadyReviewed(t *testing.T) {
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		AcceptSuggestionFn: func(uint, uint, *contracts.AcceptRadioPlayMatchSuggestionRequest) (*contracts.RadioPlayMatchSuggestionReviewResult, error) {
			return nil, contracts.ErrRadioPlayMatchSuggestionAlreadyReviewed
		},
	}, nil)
	_, err := h.AcceptRadioPlayMatchSuggestionHandler(
		matchSuggestionUserCtx(1, true),
		&AcceptRadioPlayMatchSuggestionRequest{ID: "5"},
	)
	testhelpers.AssertHumaError(t, err, 409)
}

func TestRejectRadioPlayMatchSuggestionHandler_ReasonRequired(t *testing.T) {
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		RejectSuggestionFn: func(uint, uint, *contracts.RejectRadioPlayMatchSuggestionRequest) (*contracts.RadioPlayMatchSuggestionReviewResult, error) {
			return nil, contracts.ErrRadioPlayMatchSuggestionRejectReasonRequired
		},
	}, nil)
	_, err := h.RejectRadioPlayMatchSuggestionHandler(
		matchSuggestionUserCtx(1, true),
		&RejectRadioPlayMatchSuggestionRequest{ID: "5", Body: contracts.RejectRadioPlayMatchSuggestionRequest{}},
	)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestRejectRadioPlayMatchSuggestionHandler_Success(t *testing.T) {
	now := time.Now().UTC()
	reason := "nope"
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		RejectSuggestionFn: func(suggestionID, reviewerUserID uint, req *contracts.RejectRadioPlayMatchSuggestionRequest) (*contracts.RadioPlayMatchSuggestionReviewResult, error) {
			return &contracts.RadioPlayMatchSuggestionReviewResult{
				ID: suggestionID, PlayID: 10, Status: "rejected",
				ReviewedAt: &now, ReviewedBy: &reviewerUserID, RejectionReason: &reason,
			}, nil
		},
	}, nil)
	resp, err := h.RejectRadioPlayMatchSuggestionHandler(
		matchSuggestionUserCtx(1, true),
		&RejectRadioPlayMatchSuggestionRequest{ID: "8", Body: contracts.RejectRadioPlayMatchSuggestionRequest{Reason: reason}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "rejected" || resp.Body.RejectionReason == nil || *resp.Body.RejectionReason != reason {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
}

func TestListRadioPlayMatchSuggestionsHandler_ServiceError(t *testing.T) {
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		ListPendingSuggestionsFn: func(int, int) (*contracts.RadioPlayMatchSuggestionListResult, error) {
			return nil, errors.New("boom")
		},
	}, nil)
	_, err := h.ListRadioPlayMatchSuggestionsHandler(
		matchSuggestionUserCtx(1, true),
		&ListRadioPlayMatchSuggestionsRequest{Limit: 50},
	)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetOwnRadioPlayMatchSuggestionHandler_None(t *testing.T) {
	h := NewRadioPlayMatchSuggestionHandler(&testhelpers.MockRadioPlayMatchSuggestionService{
		GetOwnPendingSuggestionFn: func(uint, uint) (*contracts.RadioPlayMatchSuggestionEntry, error) {
			return nil, nil
		},
	}, nil)
	resp, err := h.GetOwnRadioPlayMatchSuggestionHandler(
		matchSuggestionUserCtx(3, false),
		&GetOwnRadioPlayMatchSuggestionRequest{PlayID: 9},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Suggestion != nil {
		t.Errorf("expected nil suggestion, got %+v", resp.Body.Suggestion)
	}
}
