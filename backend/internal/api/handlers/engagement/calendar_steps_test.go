package engagement

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

func calendarStepsHandler(enqueue func(context.Context, *authm.User) error) *CalendarStepsHandler {
	return NewCalendarStepsHandler(&testhelpers.MockAuthEmailOutbox{EnqueueCalendarStepsFn: enqueue})
}

func TestSendCalendarStepsEmailHandler_SessionQueuesForItsOwnAccount(t *testing.T) {
	var queuedFor *authm.User
	h := calendarStepsHandler(func(_ context.Context, user *authm.User) error {
		queuedFor = user
		return nil
	})
	caller := calendarUser()

	resp, err := h.SendCalendarStepsEmailHandler(testhelpers.CtxWithSession(caller, time.Now()), &SendCalendarStepsEmailRequest{})
	if err != nil {
		t.Fatalf("a session must be able to ask: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response")
	}
	if queuedFor != caller {
		t.Errorf("queued for %v, want the caller", queuedFor)
	}
}

// The protected group admits API-token principals; one may not send mail to
// the account's inbox, and the outbox is never reached.
func TestSendCalendarStepsEmailHandler_APITokenPrincipalRefused(t *testing.T) {
	h := calendarStepsHandler(func(context.Context, *authm.User) error {
		t.Error("an API-token principal reached the outbox")
		return nil
	})

	_, err := h.SendCalendarStepsEmailHandler(
		testhelpers.CtxWithSessionAuthTime(calendarUser(), time.Time{}),
		&SendCalendarStepsEmailRequest{},
	)
	var refusal *middleware.SessionCredentialRequiredError
	if !errors.As(err, &refusal) {
		t.Fatalf("expected a session-credential refusal, got %v", err)
	}
}

// The outbox's verification refusal reaches the client as itself, so the API
// layer renders its code and status.
func TestSendCalendarStepsEmailHandler_UnverifiedAddressRefusedWithItsCode(t *testing.T) {
	h := calendarStepsHandler(func(context.Context, *authm.User) error {
		return apperrors.ErrCalendarStepsAddressUnverified()
	})

	_, err := h.SendCalendarStepsEmailHandler(testhelpers.CtxWithSession(calendarUser(), time.Now()), &SendCalendarStepsEmailRequest{})
	var authErr *apperrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected an AuthError, got %v", err)
	}
	if authErr.Code != apperrors.CodeEmailVerificationRequired || authErr.HTTPStatus() != http.StatusForbidden {
		t.Errorf("refusal = %s %d, want %s 403", authErr.Code, authErr.HTTPStatus(), apperrors.CodeEmailVerificationRequired)
	}
}

// The cap answers 429 with its code in the error envelope and a Retry-After of
// whole seconds, rounded up, never zero.
func TestSendCalendarStepsEmailHandler_CapAnswers429WithRetryAfter(t *testing.T) {
	for name, c := range map[string]struct {
		retryAfter time.Duration
		want       string
	}{
		"whole seconds":   {20 * time.Minute, "1200"},
		"rounded up":      {1199*time.Second + time.Millisecond, "1200"},
		"a full hour":     {time.Hour, "3600"},
		"never zero":      {time.Millisecond, "1"},
		"never below one": {0, "1"},
		"never negative":  {-time.Second, "1"},
	} {
		t.Run(name, func(t *testing.T) {
			h := calendarStepsHandler(func(context.Context, *authm.User) error {
				return &apperrors.CalendarStepsRateLimitedError{Limit: 3, RetryAfter: c.retryAfter}
			})

			_, err := h.SendCalendarStepsEmailHandler(testhelpers.CtxWithSession(calendarUser(), time.Now()), &SendCalendarStepsEmailRequest{})

			var withHeaders huma.HeadersError
			if !errors.As(err, &withHeaders) {
				t.Fatalf("expected headers on the refusal, got %v", err)
			}
			if got := withHeaders.GetHeaders().Get("Retry-After"); got != c.want {
				t.Errorf("Retry-After = %q, want %q", got, c.want)
			}
			var refusal *calendarStepsRateLimitedResponse
			if !errors.As(err, &refusal) {
				t.Fatalf("expected the rate-limit refusal, got %v", err)
			}
			if refusal.GetStatus() != http.StatusTooManyRequests || refusal.ErrorCode != apperrors.CodeCalendarStepsRateLimitedHourly {
				t.Errorf("refusal = %d %s", refusal.GetStatus(), refusal.ErrorCode)
			}
		})
	}
}

// A failure to queue is reported, not hidden: the caller is the account and
// would otherwise wait for an email that never comes.
func TestSendCalendarStepsEmailHandler_QueueFailureAnswers503(t *testing.T) {
	h := calendarStepsHandler(func(context.Context, *authm.User) error {
		return errors.New("database unavailable")
	})

	_, err := h.SendCalendarStepsEmailHandler(testhelpers.CtxWithSession(calendarUser(), time.Now()), &SendCalendarStepsEmailRequest{})
	var authErr *apperrors.AuthError
	if !errors.As(err, &authErr) || authErr.HTTPStatus() != http.StatusServiceUnavailable {
		t.Fatalf("expected a 503 AuthError, got %v", err)
	}
}
