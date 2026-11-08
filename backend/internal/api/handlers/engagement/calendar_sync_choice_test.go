package engagement

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

// GET /calendar/token carries the calendar sync choice as sync_choice, so the
// calendar surfaces read the token state and the decision in one request.

func storedConnectedChoice() *authm.CalendarSync {
	return &authm.CalendarSync{
		Version:   authm.CalendarSyncVersion,
		State:     authm.CalendarSyncConnected,
		Providers: []string{authm.CalendarProviderGoogleSubscribe, authm.CalendarProviderAppleSubscribe},
		DecidedAt: time.Date(2026, 11, 8, 18, 30, 0, 0, time.UTC),
	}
}

func choiceReader(choice *authm.CalendarSync, gotUser *uint) *testhelpers.MockUserService {
	return &testhelpers.MockUserService{
		GetCalendarSyncFn: func(userID uint) (*authm.CalendarSync, error) {
			*gotUser = userID
			return choice, nil
		},
	}
}

func statusFor(t *testing.T, ctx context.Context, users *testhelpers.MockUserService) *GetCalendarTokenStatusResponse {
	t.Helper()
	calendar := &testhelpers.MockCalendarService{GetTokenStatusFn: recoverableTokenStatus}
	h := NewCalendarHandler(calendar, users, testCalendarConfig())
	resp, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return resp
}

func TestGetCalendarTokenStatusHandler_CarriesTheSessionUsersSyncChoice(t *testing.T) {
	var gotUser uint
	choice := storedConnectedChoice()

	resp := statusFor(t, testhelpers.CtxWithSession(&authm.User{ID: 42}, time.Now()), choiceReader(choice, &gotUser))

	if gotUser != 42 {
		t.Errorf("read the choice for user %d, want the session user 42", gotUser)
	}
	if resp.Body.SyncChoice != choice {
		t.Errorf("sync_choice = %+v, want the stored choice", resp.Body.SyncChoice)
	}
	if !resp.Body.URLRecoverable {
		t.Errorf("the token fields must survive beside the choice, got %+v", resp.Body)
	}
}

// A caller that may not see the feed URLs still sees the choice: it is a
// preference, not a credential.
func TestGetCalendarTokenStatusHandler_SyncChoiceSurvivesTheURLRedaction(t *testing.T) {
	var gotUser uint
	choice := storedConnectedChoice()

	resp := statusFor(t, testhelpers.CtxWithUser(&authm.User{ID: 42}), choiceReader(choice, &gotUser))

	if resp.Body.FeedURL != "" || resp.Body.URLRecoverable {
		t.Fatalf("precondition: a non-session caller is not shown the URLs, got %+v", resp.Body)
	}
	if resp.Body.SyncChoice != choice {
		t.Errorf("sync_choice = %+v, want the stored choice", resp.Body.SyncChoice)
	}
}

// No decision yet is an explicit null on the wire, not an absent key.
func TestGetCalendarTokenStatusHandler_UndecidedIsNull(t *testing.T) {
	var gotUser uint
	resp := statusFor(t, testhelpers.CtxWithSession(&authm.User{ID: 42}, time.Now()), choiceReader(nil, &gotUser))

	encoded, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"sync_choice":null`) {
		t.Errorf("expected sync_choice null, got %s", encoded)
	}
}

// A failed read of the choice fails the status rather than reporting
// "undecided", which would make the client ask a user who already answered.
func TestGetCalendarTokenStatusHandler_SyncChoiceReadFailureIs500(t *testing.T) {
	calendar := &testhelpers.MockCalendarService{GetTokenStatusFn: recoverableTokenStatus}
	users := &testhelpers.MockUserService{
		GetCalendarSyncFn: func(uint) (*authm.CalendarSync, error) {
			return nil, errors.New("connection refused on 10.0.0.4:5432")
		},
	}
	h := NewCalendarHandler(calendar, users, testCalendarConfig())

	_, err := h.GetCalendarTokenStatusHandler(testhelpers.CtxWithSession(&authm.User{ID: 42}, time.Now()), &GetCalendarTokenStatusRequest{})

	testhelpers.AssertHumaError(t, err, 500)
}
