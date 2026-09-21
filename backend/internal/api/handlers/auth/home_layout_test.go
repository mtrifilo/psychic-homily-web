package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

// PSY-386: the two home-layout endpoints. The document's rules are pinned in
// models/auth; what these assert is the TRANSPORT mapping on top of them,
// which is where a rejected document and a failed write would otherwise blur
// into one status.

func homeLayoutHandler(mock *testhelpers.MockUserService) *UserPreferencesHandler {
	return NewUserPreferencesHandler(mock, "secret")
}

func authedHomeLayoutCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 7, IsActive: true})
}

func homeLayoutRequest() *SetHomeLayoutRequest {
	req := &SetHomeLayoutRequest{}
	req.Body = authm.HomeLayout{
		Version: authm.HomeLayoutVersion,
		Sections: []authm.HomeLayoutSection{
			{ID: authm.HomeSectionSavedShows, Visible: true},
			{ID: authm.HomeSectionCityGraph, Visible: false},
		},
	}
	return req
}

// Both endpoints act on the session user's own preferences, so neither may
// answer without a session.
func TestHomeLayoutHandlers_NoAuth(t *testing.T) {
	h := homeLayoutHandler(&testhelpers.MockUserService{})

	_, err := h.SetHomeLayoutHandler(context.Background(), homeLayoutRequest())
	testhelpers.AssertHumaError(t, err, 401)

	_, err = h.ClearHomeLayoutHandler(context.Background(), &ClearHomeLayoutRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

// The response echoes what the SERVICE stored, not the request body, so a
// future normalisation in the service reaches the client without the handler
// changing.
func TestSetHomeLayoutHandler_EchoesTheStoredDocument(t *testing.T) {
	var got *authm.HomeLayout
	stored := &authm.HomeLayout{
		Version:  authm.HomeLayoutVersion,
		Sections: []authm.HomeLayoutSection{{ID: authm.HomeSectionRadioShows, Visible: true}},
	}
	h := homeLayoutHandler(&testhelpers.MockUserService{
		SetHomeLayoutFn: func(_ uint, layout *authm.HomeLayout) (*authm.HomeLayout, error) {
			got = layout
			return stored, nil
		},
	})

	resp, err := h.SetHomeLayoutHandler(authedHomeLayoutCtx(), homeLayoutRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || len(got.Sections) != 2 {
		t.Fatalf("handler should forward the whole document, got %+v", got)
	}
	if resp.Body.Layout != stored {
		t.Errorf("expected the stored document echoed, got %+v", resp.Body.Layout)
	}
	if !resp.Body.Success {
		t.Errorf("expected success")
	}
}

// A rejected document is the client's mistake: 422, and the reason is echoed
// so the caller can see which rule it broke.
func TestSetHomeLayoutHandler_InvalidDocumentIs422(t *testing.T) {
	rejection := fmt.Errorf("%w: unknown section id %q", authm.ErrInvalidHomeLayout, "mixtapes")
	h := homeLayoutHandler(&testhelpers.MockUserService{
		SetHomeLayoutFn: func(uint, *authm.HomeLayout) (*authm.HomeLayout, error) {
			return nil, rejection
		},
	})

	_, err := h.SetHomeLayoutHandler(authedHomeLayoutCtx(), homeLayoutRequest())

	testhelpers.AssertHumaErrorWithDetail(t, err, 422, rejection.Error())
}

// A failed write is OUR mistake: 5xx, and its detail stays in the log rather
// than being handed to the caller as though they could fix it.
func TestSetHomeLayoutHandler_WriteFailureIs500(t *testing.T) {
	h := homeLayoutHandler(&testhelpers.MockUserService{
		SetHomeLayoutFn: func(uint, *authm.HomeLayout) (*authm.HomeLayout, error) {
			return nil, errors.New("connection refused on 10.0.0.4:5432")
		},
	})

	_, err := h.SetHomeLayoutHandler(authedHomeLayoutCtx(), homeLayoutRequest())

	testhelpers.AssertHumaErrorWithDetail(t, err, 500, "Failed to save home layout")
}

// The reset reports a null layout, which is the client's signal to render the
// shipped defaults rather than an empty home.
func TestClearHomeLayoutHandler_ReportsNull(t *testing.T) {
	cleared := false
	h := homeLayoutHandler(&testhelpers.MockUserService{
		ClearHomeLayoutFn: func(uint) error {
			cleared = true
			return nil
		},
	})

	resp, err := h.ClearHomeLayoutHandler(authedHomeLayoutCtx(), &ClearHomeLayoutRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cleared {
		t.Errorf("expected the service to be asked to clear the layout")
	}
	if resp.Body.Layout != nil {
		t.Errorf("expected a null layout, got %+v", resp.Body.Layout)
	}
	if !resp.Body.Success {
		t.Errorf("expected success")
	}
}

func TestClearHomeLayoutHandler_WriteFailureIs500(t *testing.T) {
	h := homeLayoutHandler(&testhelpers.MockUserService{
		ClearHomeLayoutFn: func(uint) error {
			return errors.New("connection refused on 10.0.0.4:5432")
		},
	})

	_, err := h.ClearHomeLayoutHandler(authedHomeLayoutCtx(), &ClearHomeLayoutRequest{})

	testhelpers.AssertHumaErrorWithDetail(t, err, 500, "Failed to reset home layout")
}
