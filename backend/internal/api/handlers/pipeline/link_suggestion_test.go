package pipeline

import (
	"errors"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

// errBoom is a generic service failure used to assert the 500 fallback path.
var errBoom = errors.New("boom")

// linkSuggestionHandler builds the handler with the supplied mock service and a
// no-op audit log (nil is tolerated by the handler).
func linkSuggestionHandler(svc contracts.LinkSuggestionServiceInterface) *LinkSuggestionHandler {
	return NewLinkSuggestionHandler(svc, nil)
}

// ──────────────────────────────────────────────
// GET /admin/link-suggestions
// ──────────────────────────────────────────────

func TestListLinkSuggestionsHandler_Success(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		ListPendingSuggestionsFn: func(limit, offset int) (*contracts.LinkSuggestionListResult, error) {
			return &contracts.LinkSuggestionListResult{
				Suggestions: []contracts.LinkSuggestionEntry{
					{ID: 1, ArtistID: 10, ArtistName: "Alpha", Platform: "spotify", Confidence: "high"},
				},
				Total: 1,
			}, nil
		},
	})

	resp, err := h.ListLinkSuggestionsHandler(adminCtx(), &ListLinkSuggestionsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Suggestions) != 1 || resp.Body.Total != 1 {
		t.Errorf("expected 1 suggestion / total 1, got %d / %d", len(resp.Body.Suggestions), resp.Body.Total)
	}
}

func TestListLinkSuggestionsHandler_ServiceError(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		ListPendingSuggestionsFn: func(int, int) (*contracts.LinkSuggestionListResult, error) {
			return nil, errBoom
		},
	})
	_, err := h.ListLinkSuggestionsHandler(adminCtx(), &ListLinkSuggestionsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ──────────────────────────────────────────────
// Accept
// ──────────────────────────────────────────────

func TestAcceptLinkSuggestionHandler_Success(t *testing.T) {
	now := time.Now().UTC()
	reviewer := uint(1)
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		AcceptSuggestionFn: func(suggestionID, reviewerUserID uint) (*contracts.LinkSuggestionReviewResult, error) {
			return &contracts.LinkSuggestionReviewResult{
				ID: suggestionID, ArtistID: 10, Status: "accepted",
				ReviewedAt: &now, ReviewedByUserID: &reviewerUserID,
			}, nil
		},
	})

	resp, err := h.AcceptLinkSuggestionHandler(adminCtx(), &AcceptLinkSuggestionRequest{ID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "accepted" || resp.Body.ID != 42 {
		t.Errorf("expected accepted/42, got %s/%d", resp.Body.Status, resp.Body.ID)
	}
	if resp.Body.ReviewedByUserID == nil || *resp.Body.ReviewedByUserID != reviewer {
		t.Errorf("expected reviewer stamp %d, got %v", reviewer, resp.Body.ReviewedByUserID)
	}
}

func TestAcceptLinkSuggestionHandler_InvalidID(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{})
	_, err := h.AcceptLinkSuggestionHandler(adminCtx(), &AcceptLinkSuggestionRequest{ID: "not-a-number"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAcceptLinkSuggestionHandler_NotFound(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		AcceptSuggestionFn: func(uint, uint) (*contracts.LinkSuggestionReviewResult, error) {
			return nil, contracts.ErrLinkSuggestionNotFound
		},
	})
	_, err := h.AcceptLinkSuggestionHandler(adminCtx(), &AcceptLinkSuggestionRequest{ID: "42"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAcceptLinkSuggestionHandler_AlreadyReviewed(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		AcceptSuggestionFn: func(uint, uint) (*contracts.LinkSuggestionReviewResult, error) {
			return nil, contracts.ErrLinkSuggestionAlreadyReviewed
		},
	})
	_, err := h.AcceptLinkSuggestionHandler(adminCtx(), &AcceptLinkSuggestionRequest{ID: "42"})
	testhelpers.AssertHumaError(t, err, 409)
}

func TestAcceptLinkSuggestionHandler_InvalidURL(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		AcceptSuggestionFn: func(uint, uint) (*contracts.LinkSuggestionReviewResult, error) {
			return nil, contracts.ErrLinkSuggestionInvalidURL
		},
	})
	_, err := h.AcceptLinkSuggestionHandler(adminCtx(), &AcceptLinkSuggestionRequest{ID: "42"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAcceptLinkSuggestionHandler_ServiceError(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		AcceptSuggestionFn: func(uint, uint) (*contracts.LinkSuggestionReviewResult, error) {
			return nil, errBoom
		},
	})
	_, err := h.AcceptLinkSuggestionHandler(adminCtx(), &AcceptLinkSuggestionRequest{ID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ──────────────────────────────────────────────
// Reject
// ──────────────────────────────────────────────

func TestRejectLinkSuggestionHandler_Success(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		RejectSuggestionFn: func(suggestionID, reviewerUserID uint) (*contracts.LinkSuggestionReviewResult, error) {
			return &contracts.LinkSuggestionReviewResult{ID: suggestionID, ArtistID: 10, Status: "rejected"}, nil
		},
	})
	resp, err := h.RejectLinkSuggestionHandler(adminCtx(), &RejectLinkSuggestionRequest{ID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "rejected" {
		t.Errorf("expected rejected, got %s", resp.Body.Status)
	}
}

func TestRejectLinkSuggestionHandler_InvalidID(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{})
	_, err := h.RejectLinkSuggestionHandler(adminCtx(), &RejectLinkSuggestionRequest{ID: "x"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRejectLinkSuggestionHandler_NotFound(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		RejectSuggestionFn: func(uint, uint) (*contracts.LinkSuggestionReviewResult, error) {
			return nil, contracts.ErrLinkSuggestionNotFound
		},
	})
	_, err := h.RejectLinkSuggestionHandler(adminCtx(), &RejectLinkSuggestionRequest{ID: "42"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestRejectLinkSuggestionHandler_AlreadyReviewed(t *testing.T) {
	h := linkSuggestionHandler(&testhelpers.MockLinkSuggestionService{
		RejectSuggestionFn: func(uint, uint) (*contracts.LinkSuggestionReviewResult, error) {
			return nil, contracts.ErrLinkSuggestionAlreadyReviewed
		},
	})
	_, err := h.RejectLinkSuggestionHandler(adminCtx(), &RejectLinkSuggestionRequest{ID: "42"})
	testhelpers.AssertHumaError(t, err, 409)
}
