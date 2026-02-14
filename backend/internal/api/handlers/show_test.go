package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/models"
)

// assertHumaError checks that an error is a huma.ErrorModel with the expected status code.
// Shared by show_test.go, venue_test.go, admin_test.go.
func assertHumaError(t *testing.T, err error, expectedStatus int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var he *huma.ErrorModel
	if !errors.As(err, &he) {
		t.Fatalf("expected *huma.ErrorModel, got %T: %v", err, err)
	}
	if he.Status != expectedStatus {
		t.Errorf("expected status %d, got %d (detail: %s)", expectedStatus, he.Status, he.Detail)
	}
}

func testShowHandler() *ShowHandler {
	return NewShowHandler(nil, nil, nil, nil, nil)
}

// --- NewShowHandler ---

func TestNewShowHandler(t *testing.T) {
	h := testShowHandler()
	if h == nil {
		t.Fatal("expected non-nil ShowHandler")
	}
}

// --- CreateShowHandler ---

func TestCreateShowHandler_UnverifiedEmail(t *testing.T) {
	h := testShowHandler()
	user := &models.User{ID: 1, IsAdmin: false, EmailVerified: false}
	ctx := ctxWithUser(user)
	req := &CreateShowRequest{}

	_, err := h.CreateShowHandler(ctx, req)
	assertHumaError(t, err, 403)
}

// --- UpdateShowHandler ---

func TestUpdateShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &UpdateShowRequest{ShowID: "1"}

	_, err := h.UpdateShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestUpdateShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UpdateShowRequest{ShowID: "abc"}

	_, err := h.UpdateShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- DeleteShowHandler ---

func TestDeleteShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &DeleteShowRequest{ShowID: "1"}

	_, err := h.DeleteShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestDeleteShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &DeleteShowRequest{ShowID: "abc"}

	_, err := h.DeleteShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- UnpublishShowHandler ---

func TestUnpublishShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &UnpublishShowRequest{ShowID: "1"}

	_, err := h.UnpublishShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestUnpublishShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnpublishShowRequest{ShowID: "abc"}

	_, err := h.UnpublishShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- MakePrivateShowHandler ---

func TestMakePrivateShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &MakePrivateShowRequest{ShowID: "1"}

	_, err := h.MakePrivateShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestMakePrivateShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &MakePrivateShowRequest{ShowID: "abc"}

	_, err := h.MakePrivateShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- PublishShowHandler ---

func TestPublishShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &PublishShowRequest{ShowID: "1"}

	_, err := h.PublishShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestPublishShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &PublishShowRequest{ShowID: "abc"}

	_, err := h.PublishShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- GetMySubmissionsHandler ---

func TestGetMySubmissionsHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &GetMySubmissionsRequest{}

	_, err := h.GetMySubmissionsHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

// --- SetShowSoldOutHandler ---

func TestSetShowSoldOutHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &SetShowSoldOutRequest{ShowID: "1"}

	_, err := h.SetShowSoldOutHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestSetShowSoldOutHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &SetShowSoldOutRequest{ShowID: "abc"}

	_, err := h.SetShowSoldOutHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- SetShowCancelledHandler ---

func TestSetShowCancelledHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &SetShowCancelledRequest{ShowID: "1"}

	_, err := h.SetShowCancelledHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestSetShowCancelledHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &SetShowCancelledRequest{ShowID: "abc"}

	_, err := h.SetShowCancelledHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- Resolve validation: InstagramHandle ---

func TestResolve_InstagramHandleTooLong(t *testing.T) {
	longHandle := make([]byte, 101)
	for i := range longHandle {
		longHandle[i] = 'x'
	}
	handleStr := string(longHandle)
	name := "Test Artist"
	body := &CreateShowRequestBody{
		EventDate: time.Now().UTC().AddDate(0, 0, 7),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []Venue{{Name: &name}},
		Artists:   []Artist{{Name: &name, InstagramHandle: &handleStr}},
	}

	errs := body.Resolve(nil)

	found := false
	for _, e := range errs {
		detail, ok := e.(*huma.ErrorDetail)
		if ok && detail.Location == "body.artists[0].instagram_handle" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error at body.artists[0].instagram_handle, got errors: %v", errs)
	}
}

func TestResolve_InstagramHandleValid(t *testing.T) {
	handle := "@valid_handle"
	name := "Test Artist"
	venueName := "Test Venue"
	body := &CreateShowRequestBody{
		EventDate: time.Now().UTC().AddDate(0, 0, 7),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []Venue{{Name: &venueName}},
		Artists:   []Artist{{Name: &name, InstagramHandle: &handle}},
	}

	errs := body.Resolve(nil)

	for _, e := range errs {
		detail, ok := e.(*huma.ErrorDetail)
		if ok && detail.Location == "body.artists[0].instagram_handle" {
			t.Errorf("unexpected instagram_handle validation error: %v", detail)
		}
	}
}

// --- ExportShowHandler ---

func TestExportShowHandler_NonDevEnvironment(t *testing.T) {
	h := testShowHandler()
	t.Setenv("ENVIRONMENT", "production")
	req := &ExportShowRequest{ShowID: "1"}

	_, err := h.ExportShowHandler(context.Background(), req)
	assertHumaError(t, err, 404)
}
