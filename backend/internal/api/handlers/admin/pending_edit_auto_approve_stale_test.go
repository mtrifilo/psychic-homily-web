package admin

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
)

// suggestDescriptionEdit builds the body the inline description editors send.
func suggestDescriptionEdit() *SuggestEntityEditRequest {
	req := &SuggestEntityEditRequest{EntityID: "10"}
	req.Body.Changes = []adminm.FieldChange{
		{Field: "description", OldValue: "Original blurb.", NewValue: "A better blurb."},
	}
	req.Body.Summary = "Updated description via inline editor"
	return req
}

func staleAtApprove() error {
	return apperrors.ErrPendingEditStaleValueAtApprove([]apperrors.StaleFieldValue{
		{Field: "description", Current: "Somebody else got there first."},
	})
}

// The trusted-tier path submits and applies in one gesture, so the entity can
// move between the submit-time derivation and the locked re-read inside the
// approval. That refusal reaches the caller as the 409 the submit-time check
// would have produced, not as a success: reporting the edit as queued would
// close the inline editor over a change that was never applied.
func TestSuggestEdit_StaleAtAutoApproveAnswersConflict(t *testing.T) {
	cancelled := []uint{}
	h := NewPendingEditHandler(
		&testhelpers.MockPendingEditService{
			CreatePendingEditFn: func(*contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return &contracts.PendingEditResponse{ID: 7}, nil
			},
			ApprovePendingEditFn: func(context.Context, uint, uint) (*contracts.PendingEditResponse, error) {
				return nil, staleAtApprove()
			},
			CancelPendingEditFn: func(editID uint, _ uint) error {
				cancelled = append(cancelled, editID)
				return nil
			},
		},
		nil,
	)

	_, err := h.SuggestArtistEditHandler(pendingEditTrustedCtx(), suggestDescriptionEdit())
	testhelpers.AssertHumaError(t, err, 409)

	// The row records a previous value nothing will re-stamp, so no approval
	// can ever apply it, and the one-pending-edit-per-submitter index would
	// refuse this user's next attempt at the field.
	if len(cancelled) != 1 || cancelled[0] != 7 {
		t.Errorf("cancelled = %v, want the just-created edit 7 removed", cancelled)
	}
}

// Every other approval failure keeps the queued row and reports the edit as
// submitted, which is the pre-existing contract for this path.
func TestSuggestEdit_OtherAutoApproveFailureStillQueues(t *testing.T) {
	h := NewPendingEditHandler(
		&testhelpers.MockPendingEditService{
			CreatePendingEditFn: func(*contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return &contracts.PendingEditResponse{ID: 7}, nil
			},
			ApprovePendingEditFn: func(context.Context, uint, uint) (*contracts.PendingEditResponse, error) {
				return nil, apperrors.ErrPendingEditInvalidRequest("cannot approve: Spotify URL must be a link on spotify.com")
			},
			CancelPendingEditFn: func(uint, uint) error {
				t.Error("a refusal that leaves a reviewable row must not remove it")
				return nil
			},
		},
		nil,
	)

	out, err := h.SuggestArtistEditHandler(pendingEditTrustedCtx(), suggestDescriptionEdit())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.Applied {
		t.Error("applied = true, want false: the approval failed")
	}
}

// The refusal that a non-trusted submission never reaches: without the
// auto-apply step there is no second derivation, so the row stays queued for a
// moderator exactly as before.
func TestSuggestEdit_UntrustedSubmissionStillQueues(t *testing.T) {
	h := NewPendingEditHandler(
		&testhelpers.MockPendingEditService{
			CreatePendingEditFn: func(*contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return &contracts.PendingEditResponse{ID: 7}, nil
			},
			ApprovePendingEditFn: func(context.Context, uint, uint) (*contracts.PendingEditResponse, error) {
				t.Error("a contributor's submission must not be auto-approved")
				return nil, nil
			},
		},
		nil,
	)

	out, err := h.SuggestArtistEditHandler(pendingEditContributorCtx(), suggestDescriptionEdit())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.Applied {
		t.Error("applied = true, want false")
	}
}
