package engagement

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// POST /calendar/token mints or rotates the feed token for any session,
// however long ago it authenticated: the status endpoint already shows the
// current URL to any session, so a freshness gate on rotation would protect
// nothing. An API-token principal is refused the mint and shown no URLs.
// Reading and deleting the token stay open to every caller.

func calendarHandlerRecordingMints(minted *bool) *CalendarHandler {
	return NewCalendarHandler(&testhelpers.MockCalendarService{
		CreateTokenFn: func(userID uint, _ *time.Time, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error) {
			*minted = true
			return &contracts.CalendarTokenCreateResponse{Token: "phcal_test"}, nil
		},
		GetTokenStatusFn: func(_ uint, apiBaseURL string) (*contracts.CalendarTokenStatusResponse, error) {
			createdAt := time.Now()
			return &contracts.CalendarTokenStatusResponse{
				HasToken:       true,
				CreatedAt:      &createdAt,
				FeedURL:        apiBaseURL + "/feeds/phcal_test/saved-shows.ics",
				FollowsFeedURL: apiBaseURL + "/feeds/phcal_test/follows.atom",
				URLRecoverable: true,
			}, nil
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
		testhelpers.CtxWithSession(calendarUser(), time.Now().Add(-1*time.Minute)),
		&CreateCalendarTokenRequest{},
	); err != nil {
		t.Fatalf("a fresh session must mint: %v", err)
	}
	if !minted {
		t.Error("expected the calendar service to be reached")
	}
}

func TestCreateCalendarTokenHandler_StaleAndRefreshedSessionsMint(t *testing.T) {
	user := calendarUser()
	stale := time.Now().Add(-middleware.RecentSessionWindow - time.Hour).Truncate(time.Second)

	// A refreshed session arrives carrying the same authentication time it had,
	// so it reaches this handler as the stale value below; that the renewal
	// carries rather than moves it is established by
	// TestRequireRecentSessionAuth_RenewalBuysNoFreshness in api/middleware.
	for name, authAt := range map[string]time.Time{
		"stale session":                 stale,
		"no authentication time at all": {},
	} {
		t.Run(name, func(t *testing.T) {
			var minted bool
			h := calendarHandlerRecordingMints(&minted)

			if _, err := h.CreateCalendarTokenHandler(
				testhelpers.CtxWithSession(user, authAt),
				&CreateCalendarTokenRequest{},
			); err != nil {
				t.Fatalf("a session must mint however long ago it authenticated: %v", err)
			}
			if !minted {
				t.Error("expected the calendar service to be reached")
			}
		})
	}
}

// The protected group admits API-token principals with no authentication
// time and no session credential. Such a principal must not mint or rotate
// the feed token, and the refusal is not the sign-in-again one.
func TestCreateCalendarTokenHandler_APITokenPrincipalRefused(t *testing.T) {
	var minted bool
	h := calendarHandlerRecordingMints(&minted)

	_, err := h.CreateCalendarTokenHandler(
		testhelpers.CtxWithSessionAuthTime(calendarUser(), time.Time{}),
		&CreateCalendarTokenRequest{},
	)
	var refusal *middleware.SessionCredentialRequiredError
	if !errors.As(err, &refusal) {
		t.Fatalf("expected a session-credential refusal, got %v", err)
	}
	if refusal.GetStatus() != http.StatusForbidden {
		t.Errorf("status = %d, want 403", refusal.GetStatus())
	}
	if refusal.ErrorCode != autherrors.CodeSessionCredentialRequired {
		t.Errorf("error_code = %q, want %q", refusal.ErrorCode, autherrors.CodeSessionCredentialRequired)
	}
	if refusal.ErrorCode == autherrors.CodeReauthRequired {
		t.Error("an API token must not be told to sign in again")
	}
	if minted {
		t.Error("a refused request must not reach the calendar service")
	}
}

// Reading the token's status and deleting it are not issuance: a stale session
// still has to be able to see that a feed exists and to turn it off.
func TestCalendarTokenReadAndDelete_UngatedByFreshness(t *testing.T) {
	var minted bool
	h := calendarHandlerRecordingMints(&minted)
	ctx := testhelpers.CtxWithSession(calendarUser(), time.Now().Add(-2*time.Hour))

	status, err := h.GetCalendarTokenStatusHandler(ctx, &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("reading token status from a stale session: %v", err)
	}
	if !status.Body.URLRecoverable || status.Body.FeedURL == "" {
		t.Errorf("a stale session still reads the feed URLs, got %+v", status.Body)
	}
	if _, err := h.DeleteCalendarTokenHandler(ctx, &DeleteCalendarTokenRequest{}); err != nil {
		t.Errorf("deleting a token from a stale session: %v", err)
	}

	t.Run("API-token principal gets no URLs from status", func(t *testing.T) {
		apiTokenCtx := testhelpers.CtxWithSessionAuthTime(calendarUser(), time.Time{})
		status, err := h.GetCalendarTokenStatusHandler(apiTokenCtx, &GetCalendarTokenStatusRequest{})
		if err != nil {
			t.Fatalf("reading token status as an API-token principal: %v", err)
		}
		if !status.Body.HasToken || status.Body.CreatedAt == nil {
			t.Errorf("an API-token principal still learns that a token exists and when, got %+v", status.Body)
		}
		if status.Body.FeedURL != "" || status.Body.FollowsFeedURL != "" || status.Body.URLRecoverable {
			t.Errorf("an API-token principal must get no URLs, got %+v", status.Body)
		}
		if status.CacheControl != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", status.CacheControl)
		}
	})
}
