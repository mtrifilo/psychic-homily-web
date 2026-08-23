package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// PSY-1907: account-level alert preferences endpoints.

func alertBoolPtr(b bool) *bool { return &b }

func shippedAlertPreferences() *authm.AlertPreferences {
	return &authm.AlertPreferences{
		AlertDefaults: authm.ResolveAccountAlertDefaults(nil),
	}
}

func alertPrefsHandler(mock *testhelpers.MockUserService) *UserPreferencesHandler {
	return NewUserPreferencesHandler(mock, "secret")
}

func authedAlertCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 7, IsActive: true})
}

// Every one of the three endpoints is session-scoped; none may answer without
// a user, since all of them read or write that user's own preferences.
func TestAlertPreferencesHandlers_NoAuth(t *testing.T) {
	h := alertPrefsHandler(&testhelpers.MockUserService{})

	_, err := h.GetAlertPreferencesHandler(context.Background(), &GetAlertPreferencesRequest{})
	testhelpers.AssertHumaError(t, err, 401)

	_, err = h.SetHomeMetroHandler(context.Background(), &SetHomeMetroRequest{})
	testhelpers.AssertHumaError(t, err, 401)

	_, err = h.SetAlertDefaultsHandler(context.Background(), &SetAlertDefaultsRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

// A user who has configured nothing reads the shipped defaults and no home
// area, rather than an empty body the client would have to interpret.
func TestGetAlertPreferencesHandler_UnsetYieldsShippedDefaults(t *testing.T) {
	h := alertPrefsHandler(&testhelpers.MockUserService{
		GetAlertPreferencesFn: func(uint) (*authm.AlertPreferences, error) {
			return shippedAlertPreferences(), nil
		},
	})

	resp, err := h.GetAlertPreferencesHandler(authedAlertCtx(), &GetAlertPreferencesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.HomeMetro != nil {
		t.Errorf("expected no home metro, got %q", *resp.Body.HomeMetro)
	}
	if !resp.Body.AlertDefaults.Shows.InApp || resp.Body.AlertDefaults.Shows.Email {
		t.Errorf("shows should default in-app ON / email OFF, got %+v", resp.Body.AlertDefaults.Shows)
	}
	if !resp.Body.AlertDefaults.Releases.InApp || resp.Body.AlertDefaults.Releases.Email {
		t.Errorf("releases should default in-app ON / email OFF, got %+v", resp.Body.AlertDefaults.Releases)
	}
}

func TestSetHomeMetroHandler_StoresAndEchoesResolvedState(t *testing.T) {
	metro := "38060"
	var stored *string
	h := alertPrefsHandler(&testhelpers.MockUserService{
		SetHomeMetroFn: func(_ uint, m *string) error {
			stored = m
			return nil
		},
		GetAlertPreferencesFn: func(uint) (*authm.AlertPreferences, error) {
			prefs := shippedAlertPreferences()
			prefs.HomeMetro = stored
			return prefs, nil
		},
	})

	req := &SetHomeMetroRequest{}
	req.Body.Metro = &metro

	resp, err := h.SetHomeMetroHandler(authedAlertCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored == nil || *stored != metro {
		t.Fatalf("expected %q to reach the service, got %v", metro, stored)
	}
	if resp.Body.HomeMetro == nil || *resp.Body.HomeMetro != metro {
		t.Errorf("expected the response to echo the stored metro, got %v", resp.Body.HomeMetro)
	}
}

// Omitting metro is how the client clears the home area, and it must reach the
// service as nil rather than being rejected as a missing field.
func TestSetHomeMetroHandler_AbsentMetroClears(t *testing.T) {
	called := false
	h := alertPrefsHandler(&testhelpers.MockUserService{
		SetHomeMetroFn: func(_ uint, m *string) error {
			called = true
			if m != nil {
				t.Errorf("expected nil metro, got %q", *m)
			}
			return nil
		},
		GetAlertPreferencesFn: func(uint) (*authm.AlertPreferences, error) {
			return shippedAlertPreferences(), nil
		},
	})

	resp, err := h.SetHomeMetroHandler(authedAlertCtx(), &SetHomeMetroRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected the clear to reach the service")
	}
	if resp.Body.HomeMetro != nil {
		t.Errorf("expected no home metro after clearing, got %q", *resp.Body.HomeMetro)
	}
}

// An unrecognized CBSA is a client error, not a server one: storing it would
// leave near-me scoping matching nothing while looking configured.
func TestSetHomeMetroHandler_UnknownMetroIs422(t *testing.T) {
	h := alertPrefsHandler(&testhelpers.MockUserService{
		SetHomeMetroFn: func(uint, *string) error {
			return autherrors.ErrUnknownHomeMetro("99999")
		},
	})

	metro := "99999"
	req := &SetHomeMetroRequest{}
	req.Body.Metro = &metro

	_, err := h.SetHomeMetroHandler(authedAlertCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	if strings.Contains(err.Error(), "99999") {
		t.Errorf("the rejected value must stay in the logs, not the response: %v", err)
	}
}

// A failed WRITE is not a rejected value. Reporting one as a 422 would tell the
// user their input was bad, stop the client retrying, and record a 4xx for a
// server fault, and echoing the error would leak driver text.
func TestSetHomeMetroHandler_WriteFailureIs500(t *testing.T) {
	h := alertPrefsHandler(&testhelpers.MockUserService{
		SetHomeMetroFn: func(uint, *string) error {
			return errors.New("failed to update home_metro: pq: connection reset by peer")
		},
	})

	metro := "38060"
	req := &SetHomeMetroRequest{}
	req.Body.Metro = &metro

	_, err := h.SetHomeMetroHandler(authedAlertCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
	if strings.Contains(err.Error(), "pq:") {
		t.Errorf("driver text must not reach the client: %v", err)
	}
}

// A nil result with a nil error is a broken implementation, not something to
// render: dereferencing it would panic the request instead of failing it.
func TestAlertPreferences_NilResultIs500(t *testing.T) {
	h := alertPrefsHandler(&testhelpers.MockUserService{
		GetAlertPreferencesFn: func(uint) (*authm.AlertPreferences, error) {
			return nil, nil
		},
	})

	_, err := h.GetAlertPreferencesHandler(authedAlertCtx(), &GetAlertPreferencesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// The request carries only the cells the user changed; the response carries
// the merged result, read back from the service.
func TestSetAlertDefaultsHandler_PartialUpdateReachesServiceAsPointers(t *testing.T) {
	var got authm.AccountAlertDefaultsUpdate
	h := alertPrefsHandler(&testhelpers.MockUserService{
		SetAccountAlertDefaultsFn: func(_ uint, update authm.AccountAlertDefaultsUpdate) error {
			got = update
			return nil
		},
		GetAlertPreferencesFn: func(uint) (*authm.AlertPreferences, error) {
			prefs := shippedAlertPreferences()
			prefs.AlertDefaults.Shows.Email = true
			return prefs, nil
		},
	})

	req := &SetAlertDefaultsRequest{}
	req.Body.Shows = &AlertChannelDefaultsInput{Email: alertBoolPtr(true)}

	resp, err := h.SetAlertDefaultsHandler(authedAlertCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Releases != nil {
		t.Error("an alert type the request omitted must stay nil, not be pinned")
	}
	if got.Shows == nil || got.Shows.Email == nil || !*got.Shows.Email {
		t.Fatalf("expected shows.email=true to reach the service, got %+v", got.Shows)
	}
	if got.Shows.InApp != nil {
		t.Error("a channel the request omitted must stay nil so it keeps inheriting")
	}
	if !resp.Body.AlertDefaults.Shows.Email {
		t.Error("expected the response to carry the merged state")
	}
}

// An explicit false is a real override and must not be mistaken for "unset".
// This is the exact cell where a bool-shaped API would lose the user's choice.
func TestSetAlertDefaultsHandler_ExplicitFalseIsAnOverride(t *testing.T) {
	var got authm.AccountAlertDefaultsUpdate
	h := alertPrefsHandler(&testhelpers.MockUserService{
		SetAccountAlertDefaultsFn: func(_ uint, update authm.AccountAlertDefaultsUpdate) error {
			got = update
			return nil
		},
		GetAlertPreferencesFn: func(uint) (*authm.AlertPreferences, error) {
			return shippedAlertPreferences(), nil
		},
	})

	req := &SetAlertDefaultsRequest{}
	req.Body.Releases = &AlertChannelDefaultsInput{InApp: alertBoolPtr(false)}

	if _, err := h.SetAlertDefaultsHandler(authedAlertCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Releases == nil || got.Releases.InApp == nil || *got.Releases.InApp {
		t.Fatalf("expected releases.in_app=false to reach the service, got %+v", got.Releases)
	}
}

// A body that pins nothing is rejected rather than silently accepted, so a
// client that mis-serialised its request finds out.
func TestSetAlertDefaultsHandler_EmptyUpdateIs422(t *testing.T) {
	h := alertPrefsHandler(&testhelpers.MockUserService{
		SetAccountAlertDefaultsFn: func(uint, authm.AccountAlertDefaultsUpdate) error {
			t.Fatal("service must not be called for an empty update")
			return nil
		},
	})

	everyCellUnset := &SetAlertDefaultsRequest{}
	everyCellUnset.Body.Shows = &AlertChannelDefaultsInput{}
	everyCellUnset.Body.Releases = &AlertChannelDefaultsInput{}

	for name, req := range map[string]*SetAlertDefaultsRequest{
		"no alert types":               {},
		"present but every cell unset": everyCellUnset,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.SetAlertDefaultsHandler(authedAlertCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

// A failed read-back must not report success with an empty matrix: the client
// would render every channel as its zero value and show email opt-ins as off.
func TestAlertPreferences_ReadBackFailureIs500(t *testing.T) {
	h := alertPrefsHandler(&testhelpers.MockUserService{
		GetAlertPreferencesFn: func(uint) (*authm.AlertPreferences, error) {
			return nil, errors.New("db down")
		},
	})

	_, err := h.GetAlertPreferencesHandler(authedAlertCtx(), &GetAlertPreferencesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}
