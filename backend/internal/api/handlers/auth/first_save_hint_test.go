package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
)

// The first-save hint dismissal endpoint. The first-wins rule is pinned in the
// service's integration suite; these assert the transport mapping.

func TestDismissFirstSaveHintHandler_NoAuth(t *testing.T) {
	called := false
	h := userPrefsHandler(&testhelpers.MockUserService{
		DismissFirstSaveHintFn: func(uint) (time.Time, error) {
			called = true
			return time.Now(), nil
		},
	})

	_, err := h.DismissFirstSaveHintHandler(context.Background(), &DismissFirstSaveHintRequest{})
	testhelpers.AssertHumaError(t, err, 401)
	if called {
		t.Fatal("service must not be reached without a session")
	}
}

// The handler writes for the SESSION user and reports the time the service
// stored, not its own clock: on a repeat call those differ.
func TestDismissFirstSaveHintHandler_ReportsTheStoredTime(t *testing.T) {
	stored := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	var gotUser uint
	h := userPrefsHandler(&testhelpers.MockUserService{
		DismissFirstSaveHintFn: func(userID uint) (time.Time, error) {
			gotUser = userID
			return stored, nil
		},
	})

	resp, err := h.DismissFirstSaveHintHandler(authedPrefsCtx(), &DismissFirstSaveHintRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUser != 7 {
		t.Errorf("wrote for user %d, want the session user 7", gotUser)
	}
	if !resp.Body.Success {
		t.Error("success = false, want true")
	}
	if !resp.Body.DismissedAt.Equal(stored) {
		t.Errorf("first_save_hint_dismissed_at = %v, want the stored %v", resp.Body.DismissedAt, stored)
	}
}

// A failed write is ours, so it is a 5xx whose detail stays in the log.
func TestDismissFirstSaveHintHandler_ServiceError(t *testing.T) {
	h := userPrefsHandler(&testhelpers.MockUserService{
		DismissFirstSaveHintFn: func(uint) (time.Time, error) {
			return time.Time{}, errors.New("pq: connection refused")
		},
	})

	_, err := h.DismissFirstSaveHintHandler(authedPrefsCtx(), &DismissFirstSaveHintRequest{})
	testhelpers.AssertHumaErrorWithDetail(t, err, 500, "Failed to record the first-save hint")
}
