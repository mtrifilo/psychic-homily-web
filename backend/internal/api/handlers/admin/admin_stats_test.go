package admin

import (
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// GetActivityFeedHandler — admin-gated; middleware guarantees a user.
// =============================================================================

func TestGetActivityFeedHandler_Success(t *testing.T) {
	mock := &testhelpers.MockAdminStatsService{
		GetRecentActivityFn: func() (*contracts.ActivityFeedResponse, error) {
			return &contracts.ActivityFeedResponse{
				Events: []contracts.ActivityEvent{
					{EventType: "edit_approved", ActorName: "admin"},
				},
			}, nil
		},
	}
	h := NewAdminStatsHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	resp, err := h.GetActivityFeedHandler(ctx, &GetActivityFeedRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(resp.Body.Events))
	}
}

func TestGetActivityFeedHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockAdminStatsService{
		GetRecentActivityFn: func() (*contracts.ActivityFeedResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewAdminStatsHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	_, err := h.GetActivityFeedHandler(ctx, &GetActivityFeedRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}
