package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func confirmVenueHandler(svc *testhelpers.MockVenueConfirmService) *VenueConfirmHandler {
	return NewVenueConfirmHandler(svc)
}

// An anonymous caller must never reach the service. rc.Protected already
// rejects it, but the handler is the layer that owns "who is allowed to write",
// so it refuses on its own rather than trusting route wiring to stay correct.
func TestConfirmVenueHandler_NoAuth(t *testing.T) {
	called := false
	svc := &testhelpers.MockVenueConfirmService{
		ConfirmVenueFn: func(uint, uint) (*contracts.VenueConfirmationResponse, error) {
			called = true
			return nil, nil
		},
	}
	_, err := confirmVenueHandler(svc).ConfirmVenueHandler(context.Background(), &ConfirmVenueRequest{VenueID: "1"})
	testhelpers.AssertHumaError(t, err, 401)
	if called {
		t.Error("anonymous confirm reached the service; it must be refused before any write")
	}
}

// Any authenticated user at any trust tier can confirm — that is the point of
// the mechanic. A brand-new, untrusted, non-admin account must succeed.
func TestConfirmVenueHandler_UntrustedUserMayConfirm(t *testing.T) {
	last := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	var gotVenueID, gotUserID uint
	svc := &testhelpers.MockVenueConfirmService{
		ConfirmVenueFn: func(venueID, userID uint) (*contracts.VenueConfirmationResponse, error) {
			gotVenueID, gotUserID = venueID, userID
			return &contracts.VenueConfirmationResponse{
				ConfirmationCount: 4, LastConfirmedAt: &last, ViewerHasConfirmed: true,
			}, nil
		},
	}
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 77})

	resp, err := confirmVenueHandler(svc).ConfirmVenueHandler(ctx, &ConfirmVenueRequest{VenueID: "42"})
	if err != nil {
		t.Fatalf("confirm by an untrusted user: %v", err)
	}
	if gotVenueID != 42 || gotUserID != 77 {
		t.Errorf("service called with (venue=%d, user=%d), want (42, 77)", gotVenueID, gotUserID)
	}
	if resp.Body.ConfirmationCount != 4 || !resp.Body.ViewerHasConfirmed {
		t.Errorf("response = %+v, want the post-mutation aggregate with viewer state", resp.Body)
	}
}

// The confirm path is numeric-id only: a slug must not open a second
// addressable identity for the same row on a rate-limited write.
func TestConfirmVenueHandler_RejectsSlug(t *testing.T) {
	called := false
	svc := &testhelpers.MockVenueConfirmService{
		ConfirmVenueFn: func(uint, uint) (*contracts.VenueConfirmationResponse, error) {
			called = true
			return nil, nil
		},
	}
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := confirmVenueHandler(svc).ConfirmVenueHandler(ctx, &ConfirmVenueRequest{VenueID: "valley-bar-phoenix-az"})
	testhelpers.AssertHumaError(t, err, 400)
	if called {
		t.Error("slug reached the service; the confirm path must accept numeric ids only")
	}
}

func TestConfirmVenueHandler_NotFound(t *testing.T) {
	svc := &testhelpers.MockVenueConfirmService{
		ConfirmVenueFn: func(venueID, _ uint) (*contracts.VenueConfirmationResponse, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := confirmVenueHandler(svc).ConfirmVenueHandler(ctx, &ConfirmVenueRequest{VenueID: "9"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestConfirmVenueHandler_ServiceFailureIs500(t *testing.T) {
	svc := &testhelpers.MockVenueConfirmService{
		ConfirmVenueFn: func(uint, uint) (*contracts.VenueConfirmationResponse, error) {
			return nil, fmt.Errorf("db exploded")
		},
	}
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := confirmVenueHandler(svc).ConfirmVenueHandler(ctx, &ConfirmVenueRequest{VenueID: "9"})
	testhelpers.AssertHumaError(t, err, 500)
}
