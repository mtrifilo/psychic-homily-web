package engagement

import (
	"errors"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// POST /calendar/token issues a feed URL whose query string carries a bearer
// token, and rotates an existing one, so the account has to have been proven
// recently. Reading and deleting the token are not issuance and stay open.

func calendarHandlerRecordingMints(minted *bool) *CalendarHandler {
	return NewCalendarHandler(&testhelpers.MockCalendarService{
		CreateTokenFn: func(userID uint, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error) {
			*minted = true
			return &contracts.CalendarTokenCreateResponse{Token: "phcal_test"}, nil
		},
		GetTokenStatusFn: func(uint) (*contracts.CalendarTokenStatusResponse, error) {
			return &contracts.CalendarTokenStatusResponse{HasToken: true}, nil
		},
		DeleteTokenFn: func(uint) error { return nil },
	}, testCalendarConfig())
}

func calendarUser() *authm.User {
	hash := "$2a$not-a-real-hash"
	return &authm.User{ID: 1, IsActive: true, PasswordHash: &hash}
}

func TestCreateCalendarTokenHandler_FreshSessionMints(t *testing.T) {
	var minted bool
	h := calendarHandlerRecordingMints(&minted)

	if _, err := h.CreateCalendarTokenHandler(
		testhelpers.CtxWithSessionAuthTime(calendarUser(), time.Now().Add(-1*time.Minute)),
		&CreateCalendarTokenRequest{},
	); err != nil {
		t.Fatalf("a fresh session must mint: %v", err)
	}
	if !minted {
		t.Error("expected the calendar service to be reached")
	}
}

func TestCreateCalendarTokenHandler_StaleAndRefreshedSessionsRefused(t *testing.T) {
	user := calendarUser()
	stale := time.Now().Add(-2 * time.Hour).Truncate(time.Second)

	// A refreshed stale session presents the same authentication time it
	// arrived with, which is what shared.TestRequireRecentSessionAuth_
	// RenewalBuysNoFreshness runs the renewal to establish.
	for name, authAt := range map[string]time.Time{
		"stale session, refreshed or not": stale,
		"no authentication time at all":   {},
	} {
		t.Run(name, func(t *testing.T) {
			var minted bool
			h := calendarHandlerRecordingMints(&minted)

			_, err := h.CreateCalendarTokenHandler(
				testhelpers.CtxWithSessionAuthTime(user, authAt),
				&CreateCalendarTokenRequest{},
			)
			var refusal *shared.ReauthRequiredError
			if !errors.As(err, &refusal) {
				t.Fatalf("expected a re-authentication refusal, got %v", err)
			}
			if minted {
				t.Error("a refused request must not reach the calendar service")
			}
		})
	}
}

// Reading the token's status and deleting it are not issuance: a stale session
// still has to be able to see that a feed exists and to turn it off.
func TestCalendarTokenReadAndDelete_UngatedByFreshness(t *testing.T) {
	var minted bool
	h := calendarHandlerRecordingMints(&minted)
	ctx := testhelpers.CtxWithSessionAuthTime(calendarUser(), time.Now().Add(-2*time.Hour))

	if _, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{}); err != nil {
		t.Errorf("reading token status from a stale session: %v", err)
	}
	if _, err := h.DeleteCalendarTokenHandler(ctx, &DeleteCalendarTokenRequest{}); err != nil {
		t.Errorf("deleting a token from a stale session: %v", err)
	}
}
