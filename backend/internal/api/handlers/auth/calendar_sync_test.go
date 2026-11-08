package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
)

// The two calendar-sync endpoints. The document's rules are pinned in
// models/auth; these assert the transport mapping on top of them.

func connectedCalendarSyncRequest() *SetCalendarSyncRequest {
	req := &SetCalendarSyncRequest{}
	req.Body = authm.CalendarSync{
		Version:   authm.CalendarSyncVersion,
		State:     authm.CalendarSyncConnected,
		Providers: []string{authm.CalendarProviderGoogleSubscribe},
		DecidedAt: time.Date(2026, 11, 8, 18, 30, 0, 0, time.UTC),
	}
	return req
}

func TestCalendarSyncHandlers_NoAuth(t *testing.T) {
	h := userPrefsHandler(&testhelpers.MockUserService{})

	_, err := h.SetCalendarSyncHandler(context.Background(), connectedCalendarSyncRequest())
	testhelpers.AssertHumaError(t, err, 401)

	_, err = h.ClearCalendarSyncHandler(context.Background(), &ClearCalendarSyncRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

// The handler forwards the whole document for the session user and renders
// what the service stored, not the request body it was given.
func TestSetCalendarSyncHandler_EchoesTheStoredDocument(t *testing.T) {
	var gotUser uint
	var got *authm.CalendarSync
	stored := &authm.CalendarSync{
		Version:   authm.CalendarSyncVersion,
		State:     authm.CalendarSyncNever,
		DecidedAt: time.Date(2026, 11, 8, 18, 30, 0, 0, time.UTC),
	}
	h := userPrefsHandler(&testhelpers.MockUserService{
		SetCalendarSyncFn: func(userID uint, choice *authm.CalendarSync) (*authm.CalendarSync, error) {
			gotUser, got = userID, choice
			return stored, nil
		},
	})

	resp, err := h.SetCalendarSyncHandler(authedPrefsCtx(), connectedCalendarSyncRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUser != 7 {
		t.Errorf("expected the session user, got %d", gotUser)
	}
	if got == nil || got.State != authm.CalendarSyncConnected || len(got.Providers) != 1 {
		t.Fatalf("handler should forward the whole document, got %+v", got)
	}
	if resp.Body.Choice != stored {
		t.Errorf("expected the stored document echoed, got %+v", resp.Body.Choice)
	}
	if !resp.Body.Success {
		t.Errorf("expected success")
	}
}

// A rejected document is the client's mistake: 422 with the broken rule
// echoed, the same mapping home-layout uses.
func TestSetCalendarSyncHandler_InvalidDocumentIs422(t *testing.T) {
	rejection := fmt.Errorf("%w: providers are required with connected", authm.ErrInvalidCalendarSync)
	h := userPrefsHandler(&testhelpers.MockUserService{
		SetCalendarSyncFn: func(uint, *authm.CalendarSync) (*authm.CalendarSync, error) {
			return nil, rejection
		},
	})

	_, err := h.SetCalendarSyncHandler(authedPrefsCtx(), connectedCalendarSyncRequest())

	testhelpers.AssertHumaErrorWithDetail(t, err, 422, rejection.Error())
}

func TestSetCalendarSyncHandler_NilResultIs500(t *testing.T) {
	h := userPrefsHandler(&testhelpers.MockUserService{
		SetCalendarSyncFn: func(uint, *authm.CalendarSync) (*authm.CalendarSync, error) {
			return nil, nil
		},
	})

	_, err := h.SetCalendarSyncHandler(authedPrefsCtx(), connectedCalendarSyncRequest())

	testhelpers.AssertHumaErrorWithDetail(t, err, 500, "Failed to save calendar choice")
}

func TestSetCalendarSyncHandler_WriteFailureIs500(t *testing.T) {
	h := userPrefsHandler(&testhelpers.MockUserService{
		SetCalendarSyncFn: func(uint, *authm.CalendarSync) (*authm.CalendarSync, error) {
			return nil, errors.New("connection refused on 10.0.0.4:5432")
		},
	})

	_, err := h.SetCalendarSyncHandler(authedPrefsCtx(), connectedCalendarSyncRequest())

	testhelpers.AssertHumaErrorWithDetail(t, err, 500, "Failed to save calendar choice")
}

func TestClearCalendarSyncHandler_ReportsNoChoice(t *testing.T) {
	cleared := false
	h := userPrefsHandler(&testhelpers.MockUserService{
		ClearCalendarSyncFn: func(uint) error {
			cleared = true
			return nil
		},
	})

	resp, err := h.ClearCalendarSyncHandler(authedPrefsCtx(), &ClearCalendarSyncRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cleared {
		t.Errorf("expected the service to be asked to clear the choice")
	}
	if resp.Body.Choice != nil {
		t.Errorf("expected no choice on a reset, got %+v", resp.Body.Choice)
	}
}

func TestClearCalendarSyncHandler_WriteFailureIs500(t *testing.T) {
	h := userPrefsHandler(&testhelpers.MockUserService{
		ClearCalendarSyncFn: func(uint) error {
			return errors.New("connection refused on 10.0.0.4:5432")
		},
	})

	_, err := h.ClearCalendarSyncHandler(authedPrefsCtx(), &ClearCalendarSyncRequest{})

	testhelpers.AssertHumaErrorWithDetail(t, err, 500, "Failed to reset calendar choice")
}

// The schema tags are the first line of defence, ahead of Validate: they are
// what a client meets for an unknown state or provider, a repeated provider
// or a missing required field. This drives the registered operation so the
// tags are tested as huma applies them, against a service that accepts
// anything, so a 422 here can only have come from the transport.
func TestPutCalendarSync_SchemaRejections(t *testing.T) {
	writes := 0
	h := userPrefsHandler(&testhelpers.MockUserService{
		SetCalendarSyncFn: func(_ uint, choice *authm.CalendarSync) (*authm.CalendarSync, error) {
			writes++
			return choice, nil
		},
	})
	_, api := humatest.New(t, huma.DefaultConfig("calendar-sync", "1.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithValue(ctx, middleware.UserContextKey, &authm.User{ID: 7, IsActive: true}))
	})
	huma.Put(api, "/auth/preferences/calendar-sync", h.SetCalendarSyncHandler)

	const decided = `"decided_at":"2026-11-08T18:30:00Z"`
	cases := []struct {
		name string
		body string
	}{
		{"unknown state", `{"version":1,"state":"maybe",` + decided + `}`},
		{"unknown provider", `{"version":1,"state":"connected","providers":["outlook"],` + decided + `}`},
		{"duplicate provider", `{"version":1,"state":"connected","providers":["copied","copied"],` + decided + `}`},
		{"empty provider list", `{"version":1,"state":"connected","providers":[],` + decided + `}`},
		{"missing decided_at", `{"version":1,"state":"never"}`},
		{"unknown field", `{"version":1,"state":"never","provider":"copied",` + decided + `}`},
		{"declines above the cap", `{"version":1,"state":"not_now","declined_until":"2026-12-08T18:30:00Z","declines":3,` + decided + `}`},
		{"negative declines", `{"version":1,"state":"not_now","declined_until":"2026-12-08T18:30:00Z","declines":-1,` + decided + `}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := api.Put("/auth/preferences/calendar-sync", strings.NewReader(tc.body))
			if resp.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body %s", resp.Code, resp.Body.String())
			}
		})
	}
	if writes != 0 {
		t.Errorf("a schema rejection must not reach the service, got %d writes", writes)
	}

	// The happy path through the same wiring, so the cases above are not
	// passing because every request fails.
	ok := api.Put("/auth/preferences/calendar-sync", strings.NewReader(
		`{"version":1,"state":"connected","providers":["google_subscribe","apple_subscribe"],`+decided+`}`))
	if ok.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", ok.Code, ok.Body.String())
	}
	var body struct {
		CalendarSync authm.CalendarSync `json:"calendar_sync"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.CalendarSync.Providers) != 2 {
		t.Errorf("expected both providers stored, got %+v", body.CalendarSync)
	}
}
