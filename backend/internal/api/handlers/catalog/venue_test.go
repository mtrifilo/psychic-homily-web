package catalog

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

func testVenueHandler() *VenueHandler {
	return NewVenueHandler(nil, nil, nil, nil)
}

func TestUpdateVenueHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &UpdateVenueRequest{VenueID: "abc"}

	_, err := h.UpdateVenueHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- DeleteVenueHandler ---

func TestDeleteVenueHandler_NoAuth(t *testing.T) {
	h := testVenueHandler()
	req := &DeleteVenueRequest{VenueID: "1"}

	_, err := h.DeleteVenueHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestDeleteVenueHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &DeleteVenueRequest{VenueID: "abc"}

	_, err := h.DeleteVenueHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// ============================================================================
// ID Parsing Boundary Tests
// ============================================================================

// The venue-detail read goes through GetVenueDetail, which owns the
// id-or-slug resolution (and the provenance stamp). These pin that every odd
// path param still lands on a clean 404 rather than a 500 or a nil response.
func TestGetVenueHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueDetailFn: func(string) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(0)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetVenueHandler_VeryLargeID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueDetailFn: func(string) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(0)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "4294967295"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetVenueHandler_OverflowID(t *testing.T) {
	// Too large for uint32 — GetVenueDetail's ParseUint fails and it falls
	// through to the slug branch, which finds nothing.
	var got string
	mock := &testhelpers.MockVenueService{
		GetVenueDetailFn: func(idOrSlug string) (*contracts.VenueDetailResponse, error) {
			got = idOrSlug
			return nil, apperrors.ErrVenueNotFound(0)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "99999999999"})
	testhelpers.AssertHumaError(t, err, 404)
	if got != "99999999999" {
		t.Errorf("service received %q, want the raw path param", got)
	}
}

// The detail read must carry the provenance stamp straight through.
func TestGetVenueHandler_CarriesProvenance(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueDetailFn: func(string) (*contracts.VenueDetailResponse, error) {
			return &contracts.VenueDetailResponse{
				ID:   7,
				Name: "Hotel Vegas",
				Provenance: &contracts.VenueProvenance{
					EditCount: 4, ContributorCount: 2, ConfirmationCount: 7,
					Sources: []string{contracts.VenueProvenanceSourceCommunity},
				},
			}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	resp, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "7"})
	if err != nil {
		t.Fatalf("GetVenueHandler: %v", err)
	}
	if resp.Body.Provenance == nil || resp.Body.Provenance.ConfirmationCount != 7 {
		t.Errorf("provenance = %+v, want the stamp the service returned", resp.Body.Provenance)
	}
}

func TestUpdateVenueHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		UpdateVenueFn: func(venueID uint, _ *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	_, err := h.UpdateVenueHandler(ctx, &UpdateVenueRequest{VenueID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateVenueHandler_CarriesCapacity(t *testing.T) {
	// Regression (PSY-1179): capacity was dropped on update — the HTTP body
	// struct + handler->service mapping omitted it. Assert it's forwarded.
	capacity := 600
	var gotReq *contracts.UpdateVenueRequest
	mock := &testhelpers.MockVenueService{
		UpdateVenueFn: func(_ uint, req *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
			gotReq = req
			return &contracts.VenueDetailResponse{ID: 42}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil) // nil revisionService -> no GetVenue snapshot call
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &UpdateVenueRequest{VenueID: "42"}
	req.Body.Capacity = &capacity

	if _, err := h.UpdateVenueHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq == nil || gotReq.Capacity == nil || *gotReq.Capacity != 600 {
		t.Errorf("capacity not forwarded to service: %+v", gotReq)
	}
}

func TestUpdateVenueHandler_CarriesAgePolicy(t *testing.T) {
	// Same defect class as the capacity regression above: the body struct and
	// the handler->service mapping are two hand-maintained lists, and a field
	// present in one but not the other is silently ignored rather than
	// rejected. The service-layer tests cannot catch it because they call
	// VenueService directly.
	policy := "all ages"
	var gotReq *contracts.UpdateVenueRequest
	mock := &testhelpers.MockVenueService{
		UpdateVenueFn: func(_ uint, req *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
			gotReq = req
			return &contracts.VenueDetailResponse{ID: 42}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &UpdateVenueRequest{VenueID: "42"}
	req.Body.AgePolicy = &policy

	if _, err := h.UpdateVenueHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq == nil || gotReq.AgePolicy == nil || *gotReq.AgePolicy != "all ages" {
		t.Errorf("age policy not forwarded to service: %+v", gotReq)
	}
}

func TestUpdateVenueHandler_ForwardsClearedAgePolicy(t *testing.T) {
	// An empty string is the CLEAR gesture, NOT "unset". The handler must
	// forward the non-nil empty pointer so the service can normalize it to
	// SQL NULL; swallowing it would make the policy unclearable.
	empty := ""
	var gotReq *contracts.UpdateVenueRequest
	mock := &testhelpers.MockVenueService{
		UpdateVenueFn: func(_ uint, req *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
			gotReq = req
			return &contracts.VenueDetailResponse{ID: 42}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &UpdateVenueRequest{VenueID: "42"}
	req.Body.AgePolicy = &empty

	if _, err := h.UpdateVenueHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq == nil || gotReq.AgePolicy == nil {
		t.Fatalf("cleared age policy must reach the service as a non-nil empty string: %+v", gotReq)
	}
	if *gotReq.AgePolicy != "" {
		t.Errorf("expected empty age policy, got %q", *gotReq.AgePolicy)
	}
}

func TestUpdateVenueHandler_RejectsOverlongAgePolicy(t *testing.T) {
	// The update body carries no maxLength schema tag (this handler validates
	// body lengths inline, as it does for description), so the 422 is the only
	// thing standing between a caller and the column's 100-char bound.
	long := strings.Repeat("a", 101)
	mock := &testhelpers.MockVenueService{
		UpdateVenueFn: func(_ uint, _ *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
			t.Error("service must not be called when the age policy is too long")
			return &contracts.VenueDetailResponse{ID: 42}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &UpdateVenueRequest{VenueID: "42"}
	req.Body.AgePolicy = &long

	_, err := h.UpdateVenueHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// TestUpdateVenueHandler_RejectsOutOfRangeCapacity and its create-side twin are
// the two tests that prove the bound BEHAVIORALLY. The huma minimum/maximum
// tags on both bodies are real, but they only fire on a full huma round trip,
// and every handler test in this package calls the handler directly. Without
// the inline guard these two exercise, the tags would be the only enforcement
// and nothing here could tell whether they still worked.
//
// TestVenueHandlers_AcceptCapacityOnBothBounds below is NOT one of them: it is a
// no-overshoot guard and passes with the feature removed.
func TestUpdateVenueHandler_RejectsOutOfRangeCapacity(t *testing.T) {
	for _, bad := range []int{0, -1, contracts.MaxVenueCapacity + 1} {
		t.Run(fmt.Sprintf("capacity_%d", bad), func(t *testing.T) {
			capacity := bad
			mock := &testhelpers.MockVenueService{
				UpdateVenueFn: func(_ uint, _ *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
					t.Error("service must not be called for an out-of-range capacity")
					return &contracts.VenueDetailResponse{ID: 42}, nil
				},
			}
			h := NewVenueHandler(mock, nil, nil, nil)
			ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
			req := &UpdateVenueRequest{VenueID: "42"}
			req.Body.Capacity = &capacity

			_, err := h.UpdateVenueHandler(ctx, req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

func TestAdminCreateVenue_RejectsOutOfRangeCapacity(t *testing.T) {
	capacity := 0
	mock := &testhelpers.MockVenueService{
		CreateVenueFn: func(_ *contracts.CreateVenueRequest, _ bool) (*contracts.VenueDetailResponse, error) {
			t.Error("service must not be called for an out-of-range capacity")
			return &contracts.VenueDetailResponse{ID: 1}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Valley Bar"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Capacity = &capacity

	_, err := h.AdminCreateVenueHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestVenueHandlers_AcceptCapacityOnBothBounds(t *testing.T) {
	// The guard must not overshoot: both ends of the range are legal.
	for _, ok := range []int{contracts.MinVenueCapacity, contracts.MaxVenueCapacity} {
		capacity := ok
		called := false
		mock := &testhelpers.MockVenueService{
			UpdateVenueFn: func(_ uint, _ *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
				called = true
				return &contracts.VenueDetailResponse{ID: 42}, nil
			},
		}
		h := NewVenueHandler(mock, nil, nil, nil)
		ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
		req := &UpdateVenueRequest{VenueID: "42"}
		req.Body.Capacity = &capacity

		if _, err := h.UpdateVenueHandler(ctx, req); err != nil {
			t.Fatalf("capacity %d is on the bound and must pass: %v", ok, err)
		}
		if !called {
			t.Errorf("capacity %d never reached the service", ok)
		}
	}
}

// TestVenueCapacitySchemaTagsMatchContract closes the drift class the bound
// otherwise invites: huma schema tags take string LITERALS, so the admin
// create/update bodies cannot reference contracts.MinVenueCapacity /
// MaxVenueCapacity directly the way the contributor suggest-edit path does.
// Without this test, moving the constant would leave the two admin routes
// advertising the old range in the OpenAPI document.
//
// It does NOT prove enforcement; the three tests above do that. Note the
// frontend repeats the same pair in VENUE_CAPACITY_BOUNDS
// (frontend/features/contributions/types.ts) and no test can see it from here.
func TestVenueCapacitySchemaTagsMatchContract(t *testing.T) {
	wantMin := strconv.Itoa(contracts.MinVenueCapacity)
	wantMax := strconv.Itoa(contracts.MaxVenueCapacity)

	bodies := map[string]reflect.Type{
		"AdminCreateVenueRequest": reflect.TypeOf(AdminCreateVenueRequest{}.Body),
		"UpdateVenueRequest":      reflect.TypeOf(UpdateVenueRequest{}.Body),
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			field, ok := body.FieldByName("Capacity")
			if !ok {
				t.Fatalf("%s has no Capacity field", name)
			}
			if got := field.Tag.Get("minimum"); got != wantMin {
				t.Errorf("%s.Capacity minimum tag = %q, want %q", name, got, wantMin)
			}
			if got := field.Tag.Get("maximum"); got != wantMax {
				t.Errorf("%s.Capacity maximum tag = %q, want %q", name, got, wantMax)
			}
		})
	}
}

func TestDeleteVenueHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueModelFn: func(venueID uint) (*catalogm.Venue, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.DeleteVenueHandler(ctx, &DeleteVenueRequest{VenueID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteVenueHandler_OverflowID(t *testing.T) {
	h := testVenueHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.DeleteVenueHandler(ctx, &DeleteVenueRequest{VenueID: "99999999999"})
	testhelpers.AssertHumaError(t, err, 400)
}

// ============================================================================
// GetVenueGenresHandler
// ============================================================================

func TestGetVenueGenres_ByID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueGenreProfileFn: func(venueID uint) ([]contracts.GenreCount, error) {
			if venueID != 5 {
				t.Errorf("expected venueID=5, got %d", venueID)
			}
			return []contracts.GenreCount{{TagID: 1, Name: "punk", Slug: "punk", Count: 10}}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	resp, err := h.GetVenueGenresHandler(context.Background(), &GetVenueGenresRequest{VenueID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Genres) != 1 || resp.Body.Genres[0].Name != "punk" {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
}

func TestGetVenueGenres_BySlug(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBySlugFn: func(slug string) (*contracts.VenueDetailResponse, error) {
			return &contracts.VenueDetailResponse{ID: 10}, nil
		},
		GetVenueGenreProfileFn: func(venueID uint) ([]contracts.GenreCount, error) {
			if venueID != 10 {
				t.Errorf("expected resolved venueID=10, got %d", venueID)
			}
			// nil genres → handler coerces to empty (non-nil) slice.
			return nil, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	resp, err := h.GetVenueGenresHandler(context.Background(), &GetVenueGenresRequest{VenueID: "valley-bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Genres == nil {
		t.Error("expected non-nil empty genres slice")
	}
}

func TestGetVenueGenres_SlugNotFound(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBySlugFn: func(_ string) (*contracts.VenueDetailResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueGenresHandler(context.Background(), &GetVenueGenresRequest{VenueID: "ghost"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetVenueGenres_ServiceError(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueGenreProfileFn: func(_ uint) ([]contracts.GenreCount, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueGenresHandler(context.Background(), &GetVenueGenresRequest{VenueID: "5"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetVenueBillNetworkHandler
// ============================================================================

func TestGetVenueBillNetwork_Success(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBillNetworkFn: func(venueID uint, window string, year *int) (*contracts.VenueBillNetworkResponse, error) {
			if venueID != 5 || window != "all" || year != nil {
				t.Errorf("unexpected params venueID=%d window=%q year=%v", venueID, window, year)
			}
			return &contracts.VenueBillNetworkResponse{}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueBillNetworkHandler(context.Background(), &GetVenueBillNetworkRequest{VenueID: "5", Window: "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetVenueBillNetwork_YearWindowRequiresYear(t *testing.T) {
	h := NewVenueHandler(&testhelpers.MockVenueService{}, nil, nil, nil)
	// window=year with no year → 422 before the service is consulted.
	_, err := h.GetVenueBillNetworkHandler(context.Background(), &GetVenueBillNetworkRequest{VenueID: "5", Window: "year"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestGetVenueBillNetwork_NotFound(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBillNetworkFn: func(_ uint, _ string, _ *int) (*contracts.VenueBillNetworkResponse, error) {
			return nil, apperrors.ErrVenueNotFound(99)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueBillNetworkHandler(context.Background(), &GetVenueBillNetworkRequest{VenueID: "99", Window: "all"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetVenueBillNetwork_ServiceError(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBillNetworkFn: func(_ uint, _ string, _ *int) (*contracts.VenueBillNetworkResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueBillNetworkHandler(context.Background(), &GetVenueBillNetworkRequest{VenueID: "5", Window: "all"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// AdminCreateVenueHandler
// ============================================================================

func TestAdminCreateVenue_Success(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		CreateVenueFn: func(req *contracts.CreateVenueRequest, isAdmin bool) (*contracts.VenueDetailResponse, error) {
			if !isAdmin {
				t.Error("expected isAdmin=true for admin create")
			}
			if req.Name != "Valley Bar" || req.SubmittedBy == nil || *req.SubmittedBy != 1 {
				t.Errorf("unexpected service request: %+v", req)
			}
			return &contracts.VenueDetailResponse{ID: 1, Name: req.Name}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Valley Bar"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"

	resp, err := h.AdminCreateVenueHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Valley Bar" {
		t.Errorf("expected name='Valley Bar', got %q", resp.Body.Name)
	}
}

func TestAdminCreateVenue_CarriesCapacityAndDescription(t *testing.T) {
	// Regression (PSY-1179): capacity + description were silently DROPPED on
	// create because the HTTP body struct + handler->service mapping omitted them,
	// even though the service contract + CLI sent them. Assert the handler now
	// forwards both to the service.
	capacity := 550
	desc := "All-ages rock club."
	policy := "all ages"
	var gotReq *contracts.CreateVenueRequest
	mock := &testhelpers.MockVenueService{
		CreateVenueFn: func(req *contracts.CreateVenueRequest, _ bool) (*contracts.VenueDetailResponse, error) {
			gotReq = req
			return &contracts.VenueDetailResponse{ID: 1, Name: req.Name}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Valley Bar"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Capacity = &capacity
	req.Body.Description = &desc
	req.Body.AgePolicy = &policy

	if _, err := h.AdminCreateVenueHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq == nil || gotReq.Capacity == nil || *gotReq.Capacity != 550 {
		t.Errorf("capacity not forwarded to service: %+v", gotReq)
	}
	if gotReq.Description == nil || *gotReq.Description != "All-ages rock club." {
		t.Errorf("description not forwarded to service: %+v", gotReq)
	}
	// Age policy joins the same forwarding list, and fails the same way.
	if gotReq.AgePolicy == nil || *gotReq.AgePolicy != "all ages" {
		t.Errorf("age policy not forwarded to service: %+v", gotReq)
	}
}

func TestAdminCreateVenue_InvalidSocialURL(t *testing.T) {
	// Social-URL validation runs before the service call; a non-http scheme
	// is rejected without ever reaching CreateVenue.
	h := NewVenueHandler(&testhelpers.MockVenueService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	bad := "javascript:alert(1)"
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Valley Bar"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Instagram = &bad

	_, err := h.AdminCreateVenueHandler(ctx, req)
	if err == nil {
		t.Fatal("expected error for javascript: URL, got nil")
	}
}

func TestAdminCreateVenue_ServiceError(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		CreateVenueFn: func(_ *contracts.CreateVenueRequest, _ bool) (*contracts.VenueDetailResponse, error) {
			return nil, fmt.Errorf("duplicate venue")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Valley Bar"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"

	_, err := h.AdminCreateVenueHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// VenueYearArchiveExistsHandler (PSY-1770)
// ============================================================================
//
// The whole design of the year-archive probe rests on WHICH status each outcome
// produces, because the frontend proxy reads nothing else: a 404 becomes a real
// HTTP 404 for the reader, while a 500 makes it fail OPEN and let the page
// render. Collapsing the service-error path into a 404 would turn every
// `/venues/{slug}/shows/{year}` on the site into a hard 404 during a database
// blip — invisible to the service integration tests, which never run the
// handler.

func TestVenueYearArchiveExists_ByID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		HasPastShowsInYearFn: func(venueID uint, year int) (bool, error) {
			if venueID != 5 || year != 2019 {
				t.Errorf("expected venueID=5 year=2019, got %d/%d", venueID, year)
			}
			return true, nil
		},
		// A numeric id resolves without touching the slug lookup. Failing here
		// would mean the probe pays a needless query on the id path.
		GetVenueBySlugFn: func(slug string) (*contracts.VenueDetailResponse, error) {
			t.Errorf("numeric id must not go through the slug resolver, got %q", slug)
			return nil, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)

	resp, err := h.VenueYearArchiveExistsHandler(context.Background(), &VenueYearArchiveExistsRequest{
		VenueID: "5", Year: 2019,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
}

func TestVenueYearArchiveExists_BySlug(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBySlugFn: func(slug string) (*contracts.VenueDetailResponse, error) {
			return &contracts.VenueDetailResponse{ID: 10}, nil
		},
		HasPastShowsInYearFn: func(venueID uint, year int) (bool, error) {
			if venueID != 10 {
				t.Errorf("expected resolved venueID=10, got %d", venueID)
			}
			return true, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)

	_, err := h.VenueYearArchiveExistsHandler(context.Background(), &VenueYearArchiveExistsRequest{
		VenueID: "valley-bar", Year: 2019,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A year with no archived shows is the 404 the proxy turns into a real one.
func TestVenueYearArchiveExists_EmptyYearIs404(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		HasPastShowsInYearFn: func(uint, int) (bool, error) { return false, nil },
	}
	h := NewVenueHandler(mock, nil, nil, nil)

	_, err := h.VenueYearArchiveExistsHandler(context.Background(), &VenueYearArchiveExistsRequest{
		VenueID: "5", Year: 1999,
	})
	testhelpers.AssertHumaErrorWithDetail(t, err, 404, venueYearArchiveAbsent)
}

// An unknown venue answers with the SAME status AND the same detail as an empty
// year. Not cosmetic: huma writes the error body even for a HEAD and Go derives
// a Content-Length from it, so two different messages are two different response
// sizes — enough for a crawler walking the year space to tell the cases apart
// off a body it never receives. The detail assertion is what holds that line.
func TestVenueYearArchiveExists_UnknownVenueIsIndistinguishableFromAnEmptyYear(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBySlugFn: func(string) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(0)
		},
		HasPastShowsInYearFn: func(uint, int) (bool, error) {
			t.Error("an unresolved venue must not reach the probe")
			return false, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)

	_, err := h.VenueYearArchiveExistsHandler(context.Background(), &VenueYearArchiveExistsRequest{
		VenueID: "no-such-venue", Year: 2019,
	})
	testhelpers.AssertHumaErrorWithDetail(t, err, 404, venueYearArchiveAbsent)
}

// A failed READ is not an answer. 500 is what makes the proxy fail OPEN; a 404
// here would hard-404 a real archive over a transient fault.
func TestVenueYearArchiveExists_ProbeErrorIs500(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		HasPastShowsInYearFn: func(uint, int) (bool, error) {
			return false, fmt.Errorf("database down")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)

	_, err := h.VenueYearArchiveExistsHandler(context.Background(), &VenueYearArchiveExistsRequest{
		VenueID: "5", Year: 2019,
	})
	testhelpers.AssertHumaError(t, err, 500)
}

// The same rule one layer up: a slug lookup that FAILED (rather than resolving
// to nothing) must not be reported as absent.
func TestVenueYearArchiveExists_SlugLookupErrorIs500(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBySlugFn: func(string) (*contracts.VenueDetailResponse, error) {
			return nil, fmt.Errorf("database down")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)

	_, err := h.VenueYearArchiveExistsHandler(context.Background(), &VenueYearArchiveExistsRequest{
		VenueID: "valley-bar", Year: 2019,
	})
	testhelpers.AssertHumaError(t, err, 500)
}
