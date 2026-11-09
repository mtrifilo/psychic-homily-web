package engagement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// statusWithReadsAndReset answers GetTokenStatus with every field of the
// status set, the feed URLs included, so a response missing one is the
// handler withholding it.
func statusWithReadsAndReset(userID uint, apiBaseURL string) (*contracts.CalendarTokenStatusResponse, error) {
	status, err := statusWithReads(userID, apiBaseURL)
	if err != nil {
		return nil, err
	}
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	reason := engagementm.CalendarLinkResetPasswordReset
	status.LinkResetAt = &at
	status.LinkResetReason = &reason
	return status, nil
}

// feedStatusJSON is the address-free status handler's body for ctx, as the
// wire carries it.
func feedStatusJSON(t *testing.T, ctx context.Context) (map[string]any, string) {
	t.Helper()
	h := NewCalendarHandler(&testhelpers.MockCalendarService{GetTokenStatusFn: statusWithReadsAndReset}, testCalendarConfig())
	resp, err := h.GetCalendarFeedStatusHandler(ctx, &GetCalendarFeedStatusRequest{})
	if err != nil {
		t.Fatalf("feed status: %v", err)
	}
	if resp.CacheControl != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", resp.CacheControl)
	}
	raw, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return fields, string(raw)
}

// No caller gets a feed address from the feed status, though the service
// answered with both URLs.
func TestGetCalendarFeedStatusHandler_NoAddressForAnyCaller(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"session":   testhelpers.CtxWithSession(calendarUser(), time.Now().Add(-time.Hour)),
		"API token": testhelpers.CtxWithSessionAuthTime(calendarUser(), time.Time{}),
	} {
		t.Run(name, func(t *testing.T) {
			fields, raw := feedStatusJSON(t, ctx)
			for _, key := range []string{"feed_url", "follows_feed_url", "url_recoverable", "token"} {
				if _, present := fields[key]; present {
					t.Errorf("%s must be absent from the feed status, got %s", key, raw)
				}
			}
			for _, fragment := range []string{"phcal_", "/feeds/", "http"} {
				if strings.Contains(raw, fragment) {
					t.Errorf("the feed status carries %q: %s", fragment, raw)
				}
			}
		})
	}
}

// A session gets the reads and the link-reset marker; an API-token principal
// gets only whether a token exists and when it was minted, as on the token
// status.
func TestGetCalendarFeedStatusHandler_ReadsAndResetOnlyForASession(t *testing.T) {
	session, raw := feedStatusJSON(t, testhelpers.CtxWithSession(calendarUser(), time.Now().Add(-time.Hour)))
	for _, key := range []string{"has_token", "created_at", "reads", "link_reset_at", "link_reset_reason"} {
		if _, present := session[key]; !present {
			t.Errorf("a session gets %s, got %s", key, raw)
		}
	}
	if reads, _ := session["reads"].([]any); len(reads) != 2 {
		t.Errorf("a session gets both reads, got %s", raw)
	}

	apiToken, raw := feedStatusJSON(t, testhelpers.CtxWithSessionAuthTime(calendarUser(), time.Time{}))
	if len(apiToken) != 2 || apiToken["has_token"] != true || apiToken["created_at"] == nil {
		t.Errorf("an API-token principal gets has_token and created_at only, got %s", raw)
	}
}

func TestGetCalendarFeedStatusHandler_NoAuthAndServiceError(t *testing.T) {
	h := NewCalendarHandler(&testhelpers.MockCalendarService{}, testCalendarConfig())
	_, err := h.GetCalendarFeedStatusHandler(t.Context(), &GetCalendarFeedStatusRequest{})
	testhelpers.AssertHumaError(t, err, 401)

	h = NewCalendarHandler(&testhelpers.MockCalendarService{
		GetTokenStatusFn: func(uint, string) (*contracts.CalendarTokenStatusResponse, error) {
			return nil, errors.New("db down")
		},
	}, testCalendarConfig())
	_, err = h.GetCalendarFeedStatusHandler(
		testhelpers.CtxWithSession(calendarUser(), time.Now()), &GetCalendarFeedStatusRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// forget is one ForgetFeedRead call the handler made.
type forget struct {
	userID uint
	client engagementm.CalendarFeedClient
}

func calendarHandlerRecordingForgets(calls *[]forget, forgetErr error) *CalendarHandler {
	return NewCalendarHandler(&testhelpers.MockCalendarService{
		ForgetFeedReadFn: func(userID uint, client engagementm.CalendarFeedClient) error {
			*calls = append(*calls, forget{userID: userID, client: client})
			return forgetErr
		},
	}, testCalendarConfig())
}

// A session forgets the client it names, for its own account.
func TestForgetCalendarFeedReadHandler_SessionForgetsTheNamedClient(t *testing.T) {
	var calls []forget
	h := calendarHandlerRecordingForgets(&calls, nil)

	resp, err := h.ForgetCalendarFeedReadHandler(
		testhelpers.CtxWithSession(calendarUser(), time.Now().Add(-2*time.Hour)),
		&ForgetCalendarFeedReadRequest{Client: "apple_ios"},
	)
	if err != nil {
		t.Fatalf("a session must forget: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("success = false, want true")
	}
	want := []forget{{userID: calendarUser().ID, client: engagementm.CalendarFeedClientAppleIOS}}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("ForgetFeedRead calls = %+v, want %+v", calls, want)
	}
}

// An API-token principal is refused exactly as the mint refuses it, and the
// service is not reached.
func TestForgetCalendarFeedReadHandler_APITokenPrincipalRefused(t *testing.T) {
	var calls []forget
	h := calendarHandlerRecordingForgets(&calls, nil)

	_, err := h.ForgetCalendarFeedReadHandler(
		testhelpers.CtxWithSessionAuthTime(calendarUser(), time.Time{}),
		&ForgetCalendarFeedReadRequest{Client: "google"},
	)
	var refusal *middleware.SessionCredentialRequiredError
	if !errors.As(err, &refusal) {
		t.Fatalf("expected a session-credential refusal, got %v", err)
	}
	if refusal.GetStatus() != http.StatusForbidden || refusal.ErrorCode != autherrors.CodeSessionCredentialRequired {
		t.Errorf("refusal = %d %q, want 403 %q", refusal.GetStatus(), refusal.ErrorCode, autherrors.CodeSessionCredentialRequired)
	}
	if len(calls) != 0 {
		t.Errorf("a refused request must not reach the calendar service, got %+v", calls)
	}
}

// A client outside the vocabulary is refused, not forgotten as a no-op.
func TestForgetCalendarFeedReadHandler_UnknownClientRefused(t *testing.T) {
	for _, client := range []string{"", "outlook", "Google", "google "} {
		var calls []forget
		h := calendarHandlerRecordingForgets(&calls, nil)

		_, err := h.ForgetCalendarFeedReadHandler(
			testhelpers.CtxWithSession(calendarUser(), time.Now()),
			&ForgetCalendarFeedReadRequest{Client: client},
		)
		testhelpers.AssertHumaError(t, err, 422)
		if len(calls) != 0 {
			t.Errorf("client %q reached the calendar service: %+v", client, calls)
		}
	}
}

func TestForgetCalendarFeedReadHandler_NoAuthAndServiceError(t *testing.T) {
	var calls []forget
	_, err := calendarHandlerRecordingForgets(&calls, nil).ForgetCalendarFeedReadHandler(
		t.Context(), &ForgetCalendarFeedReadRequest{Client: "google"})
	testhelpers.AssertHumaError(t, err, 401)

	_, err = calendarHandlerRecordingForgets(&calls, errors.New("db down")).ForgetCalendarFeedReadHandler(
		testhelpers.CtxWithSession(calendarUser(), time.Now()), &ForgetCalendarFeedReadRequest{Client: "google"})
	testhelpers.AssertHumaError(t, err, 500)
}

// The forget parameter's enum tag lists exactly the model's client
// vocabulary, in order, so the published schema admits every family the
// status can list and no other.
func TestForgetCalendarFeedReadRequest_EnumTagMatchesTheClientVocabulary(t *testing.T) {
	var clients []string
	for _, c := range engagementm.CalendarFeedClients {
		clients = append(clients, string(c))
	}
	field, ok := reflect.TypeOf(ForgetCalendarFeedReadRequest{}).FieldByName("Client")
	if !ok {
		t.Fatal("ForgetCalendarFeedReadRequest has no Client field")
	}
	if got, want := field.Tag.Get("enum"), strings.Join(clients, ","); got != want {
		t.Errorf("Client enum tag = %q, want %q", got, want)
	}
}
