package catalog

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func testShowHandler() *ShowHandler {
	return NewShowHandler(nil, nil, nil, nil, nil, nil, nil, nil)
}

// --- CreateShowHandler ---

func TestCreateShowHandler_UnverifiedEmail(t *testing.T) {
	h := testShowHandler()
	user := &authm.User{ID: 1, IsAdmin: false, EmailVerified: false}
	ctx := testhelpers.CtxWithUser(user)
	req := &CreateShowRequest{}

	_, err := h.CreateShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

// --- UpdateShowHandler ---

func TestUpdateShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &UpdateShowRequest{ShowID: "1"}

	_, err := h.UpdateShowHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUpdateShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateShowRequest{ShowID: "abc"}

	_, err := h.UpdateShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- DeleteShowHandler ---

func TestDeleteShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &DeleteShowRequest{ShowID: "1"}

	_, err := h.DeleteShowHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestDeleteShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &DeleteShowRequest{ShowID: "abc"}

	_, err := h.DeleteShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- UnpublishShowHandler ---

func TestUnpublishShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &UnpublishShowRequest{ShowID: "1"}

	_, err := h.UnpublishShowHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUnpublishShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnpublishShowRequest{ShowID: "abc"}

	_, err := h.UnpublishShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- MakePrivateShowHandler ---

func TestMakePrivateShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &MakePrivateShowRequest{ShowID: "1"}

	_, err := h.MakePrivateShowHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestMakePrivateShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &MakePrivateShowRequest{ShowID: "abc"}

	_, err := h.MakePrivateShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- PublishShowHandler ---

func TestPublishShowHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &PublishShowRequest{ShowID: "1"}

	_, err := h.PublishShowHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestPublishShowHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &PublishShowRequest{ShowID: "abc"}

	_, err := h.PublishShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- GetMySubmissionsHandler ---

func TestGetMySubmissionsHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &GetMySubmissionsRequest{}

	_, err := h.GetMySubmissionsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

// --- SetShowSoldOutHandler ---

func TestSetShowSoldOutHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &SetShowSoldOutRequest{ShowID: "1"}

	_, err := h.SetShowSoldOutHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetShowSoldOutHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetShowSoldOutRequest{ShowID: "abc"}

	_, err := h.SetShowSoldOutHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- SetShowCancelledHandler ---

func TestSetShowCancelledHandler_NoAuth(t *testing.T) {
	h := testShowHandler()
	req := &SetShowCancelledRequest{ShowID: "1"}

	_, err := h.SetShowCancelledHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetShowCancelledHandler_InvalidID(t *testing.T) {
	h := testShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetShowCancelledRequest{ShowID: "abc"}

	_, err := h.SetShowCancelledHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
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
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// Mock-based tests: GetShowHandler
// ============================================================================

func TestGetShowHandler_ByID(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowFn: func(showID uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: showID, Status: "approved"}, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	resp, err := h.GetShowHandler(context.Background(), &GetShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 42 {
		t.Errorf("expected ID=42, got %d", resp.Body.ID)
	}
}

func TestGetShowHandler_BySlug(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowBySlugFn: func(slug string) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 10, Slug: slug, Status: "approved"}, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	resp, err := h.GetShowHandler(context.Background(), &GetShowRequest{ShowID: "cool-show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Slug != "cool-show" {
		t.Errorf("expected slug='cool-show', got %q", resp.Body.Slug)
	}
}

func TestGetShowHandler_NotFound(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	_, err := h.GetShowHandler(context.Background(), &GetShowRequest{ShowID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetShowHandler_NonApproved_Admin(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, Status: "pending"}, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	resp, err := h.GetShowHandler(ctx, &GetShowRequest{ShowID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "pending" {
		t.Errorf("expected status='pending', got %q", resp.Body.Status)
	}
}

func TestGetShowHandler_NonApproved_Submitter(t *testing.T) {
	userID := uint(5)
	mock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, Status: "pending", SubmittedBy: &userID}, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

	resp, err := h.GetShowHandler(ctx, &GetShowRequest{ShowID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected ID=1, got %d", resp.Body.ID)
	}
}

func TestGetShowHandler_NonApproved_Denied(t *testing.T) {
	otherUser := uint(99)
	mock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, Status: "pending", SubmittedBy: &otherUser}, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

	_, err := h.GetShowHandler(ctx, &GetShowRequest{ShowID: "1"})
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// Mock-based tests: GetShowsHandler
// ============================================================================

func TestGetShowsHandler_Success(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowsFn: func(filters map[string]interface{}) ([]*contracts.ShowResponse, error) {
			return []*contracts.ShowResponse{{ID: 1}, {ID: 2}}, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	resp, err := h.GetShowsHandler(context.Background(), &GetShowsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body) != 2 {
		t.Errorf("expected 2 shows, got %d", len(resp.Body))
	}
}

func TestGetShowsHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowsFn: func(_ map[string]interface{}) ([]*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	_, err := h.GetShowsHandler(context.Background(), &GetShowsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: GetShowCitiesHandler
// ============================================================================

func TestGetShowCitiesHandler_Success(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowCitiesFn: func(timezone string) ([]contracts.ShowCityResponse, error) {
			return []contracts.ShowCityResponse{{City: "Phoenix", State: "AZ", ShowCount: 5}}, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	resp, err := h.GetShowCitiesHandler(context.Background(), &GetShowCitiesRequest{Timezone: "America/Phoenix"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Cities) != 1 {
		t.Errorf("expected 1 city, got %d", len(resp.Body.Cities))
	}
}

func TestGetShowCitiesHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowCitiesFn: func(_ string) ([]contracts.ShowCityResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	_, err := h.GetShowCitiesHandler(context.Background(), &GetShowCitiesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: GetUpcomingShowsHandler
// ============================================================================

func TestGetUpcomingShowsHandler_Success(t *testing.T) {
	nextCursor := "abc123"
	mock := &testhelpers.MockShowService{
		GetUpcomingShowsFn: func(timezone, cursor string, limit int, includeNonApproved bool, filters *contracts.UpcomingShowsFilter) ([]*contracts.ShowResponse, *string, error) {
			return []*contracts.ShowResponse{{ID: 1}}, &nextCursor, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	resp, err := h.GetUpcomingShowsHandler(context.Background(), &GetUpcomingShowsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Shows) != 1 {
		t.Errorf("expected 1 show, got %d", len(resp.Body.Shows))
	}
	if !resp.Body.Pagination.HasMore {
		t.Error("expected has_more=true")
	}
}

func TestGetUpcomingShowsHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetUpcomingShowsFn: func(_, _ string, _ int, _ bool, _ *contracts.UpcomingShowsFilter) ([]*contracts.ShowResponse, *string, error) {
			return nil, nil, fmt.Errorf("db error")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)

	_, err := h.GetUpcomingShowsHandler(context.Background(), &GetUpcomingShowsRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: CreateShowHandler
// ============================================================================

func TestCreateShowHandler_Success(t *testing.T) {
	showMock := &testhelpers.MockShowService{
		CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 100, Title: req.Title, Status: "pending"}, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, &testhelpers.MockSavedShowService{}, &testhelpers.MockDiscordService{}, &testhelpers.MockMusicDiscoveryService{}, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, EmailVerified: true})

	venueID := uint(1)
	artistName := "Test Band"
	req := &CreateShowRequest{}
	req.Body.EventDate = time.Now().Add(24 * time.Hour)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venueID}}
	req.Body.Artists = []Artist{{Name: &artistName}}

	resp, err := h.CreateShowHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 100 {
		t.Errorf("expected ID=100, got %d", resp.Body.ID)
	}
}

func TestCreateShowHandler_AutoSave(t *testing.T) {
	var savedUserID, savedShowID uint
	showMock := &testhelpers.MockShowService{
		CreateShowFn: func(_ *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 50, Status: "pending"}, nil
		},
	}
	savedShowMock := &testhelpers.MockSavedShowService{
		SaveShowFn: func(userID, showID uint) error {
			savedUserID = userID
			savedShowID = showID
			return nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, savedShowMock, &testhelpers.MockDiscordService{}, &testhelpers.MockMusicDiscoveryService{}, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, EmailVerified: true})

	venueID := uint(1)
	artistName := "Band"
	req := &CreateShowRequest{}
	req.Body.EventDate = time.Now().Add(24 * time.Hour)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venueID}}
	req.Body.Artists = []Artist{{Name: &artistName}}

	_, err := h.CreateShowHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if savedUserID != 7 {
		t.Errorf("expected auto-save userID=7, got %d", savedUserID)
	}
	if savedShowID != 50 {
		t.Errorf("expected auto-save showID=50, got %d", savedShowID)
	}
}

func TestCreateShowHandler_ServiceError(t *testing.T) {
	showMock := &testhelpers.MockShowService{
		CreateShowFn: func(_ *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, &testhelpers.MockDiscordService{}, &testhelpers.MockMusicDiscoveryService{}, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, EmailVerified: true})

	venueID := uint(1)
	artistName := "Band"
	req := &CreateShowRequest{}
	req.Body.EventDate = time.Now().Add(24 * time.Hour)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venueID}}
	req.Body.Artists = []Artist{{Name: &artistName}}

	_, err := h.CreateShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Mock-based tests: UpdateShowHandler
// ============================================================================

func TestUpdateShowHandler_OwnerSuccess(t *testing.T) {
	userID := uint(5)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID, Status: "pending"}, nil
		},
		UpdateShowWithRelationsFn: func(showID uint, _ *contracts.UpdateShowRequest, _ []contracts.CreateShowVenue, _ []contracts.CreateShowArtist, _ bool) (*contracts.ShowResponse, []contracts.OrphanedArtist, error) {
			return &contracts.ShowResponse{ID: showID, Title: "Updated"}, nil, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})
	title := "Updated"
	req := &UpdateShowRequest{ShowID: "1"}
	req.Body.Title = &title

	resp, err := h.UpdateShowHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Title != "Updated" {
		t.Errorf("expected title='Updated', got %q", resp.Body.Title)
	}
}

func TestUpdateShowHandler_AdminSuccess(t *testing.T) {
	otherUser := uint(99)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &otherUser, Status: "approved"}, nil
		},
		UpdateShowWithRelationsFn: func(showID uint, _ *contracts.UpdateShowRequest, _ []contracts.CreateShowVenue, _ []contracts.CreateShowArtist, _ bool) (*contracts.ShowResponse, []contracts.OrphanedArtist, error) {
			return &contracts.ShowResponse{ID: showID}, nil, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	title := "Admin Update"
	req := &UpdateShowRequest{ShowID: "1"}
	req.Body.Title = &title

	_, err := h.UpdateShowHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateShowHandler_NotFound(t *testing.T) {
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UpdateShowHandler(ctx, &UpdateShowRequest{ShowID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateShowHandler_Unauthorized(t *testing.T) {
	otherUser := uint(99)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &otherUser}, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

	_, err := h.UpdateShowHandler(ctx, &UpdateShowRequest{ShowID: "1"})
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUpdateShowHandler_ServiceError(t *testing.T) {
	userID := uint(1)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID}, nil
		},
		UpdateShowWithRelationsFn: func(_ uint, _ *contracts.UpdateShowRequest, _ []contracts.CreateShowVenue, _ []contracts.CreateShowArtist, _ bool) (*contracts.ShowResponse, []contracts.OrphanedArtist, error) {
			return nil, nil, fmt.Errorf("update failed")
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	title := "New"
	req := &UpdateShowRequest{ShowID: "1"}
	req.Body.Title = &title

	_, err := h.UpdateShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// PSY-563: Summary field is accepted on the request body and passed through
// to RecordRevision. Validation rejects oversized summaries.

func TestUpdateShowHandler_SummaryTooLong(t *testing.T) {
	userID := uint(1)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID}, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	longSummary := make([]byte, 5001)
	for i := range longSummary {
		longSummary[i] = 'a'
	}
	summary := string(longSummary)
	req := &UpdateShowRequest{ShowID: "1"}
	req.Body.Summary = &summary

	_, err := h.UpdateShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// PSY-563: when fields change and revisionService is wired, the handler
// records a revision row with the diff and the user-supplied Summary.
func TestUpdateShowHandler_RecordsRevisionOnChange(t *testing.T) {
	userID := uint(5)
	oldDescription := "Old description"
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{
				ID:          1,
				SubmittedBy: &userID,
				Title:       "Original Title",
				Description: &oldDescription,
				Status:      "pending",
			}, nil
		},
		UpdateShowWithRelationsFn: func(showID uint, _ *contracts.UpdateShowRequest, _ []contracts.CreateShowVenue, _ []contracts.CreateShowArtist, _ bool) (*contracts.ShowResponse, []contracts.OrphanedArtist, error) {
			newDescription := "New description"
			return &contracts.ShowResponse{
				ID:          showID,
				Title:       "Updated Title",
				Description: &newDescription,
			}, nil, nil
		},
	}

	var (
		mu              sync.Mutex
		recordedType    string
		recordedID      uint
		recordedUserID  uint
		recordedChanges []adminm.FieldChange
		recordedSummary string
	)
	wg := sync.WaitGroup{}
	wg.Add(1)
	revisionMock := &testhelpers.MockRevisionService{
		RecordRevisionFn: func(entityType string, entityID uint, uID uint, changes []adminm.FieldChange, summary string) error {
			mu.Lock()
			recordedType = entityType
			recordedID = entityID
			recordedUserID = uID
			recordedChanges = changes
			recordedSummary = summary
			mu.Unlock()
			wg.Done()
			return nil
		},
	}

	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, revisionMock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})
	title := "Updated Title"
	desc := "New description"
	summary := "Fixed misspelled title"
	req := &UpdateShowRequest{ShowID: "1"}
	req.Body.Title = &title
	req.Body.Description = &desc
	req.Body.Summary = &summary

	_, err := h.UpdateShowHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if recordedType != "show" {
		t.Errorf("expected entityType='show', got %q", recordedType)
	}
	if recordedID != 1 {
		t.Errorf("expected entityID=1, got %d", recordedID)
	}
	if recordedUserID != 5 {
		t.Errorf("expected userID=5, got %d", recordedUserID)
	}
	if recordedSummary != "Fixed misspelled title" {
		t.Errorf("expected summary='Fixed misspelled title', got %q", recordedSummary)
	}
	if len(recordedChanges) != 2 {
		t.Fatalf("expected 2 field changes (title, description), got %d", len(recordedChanges))
	}
	// Verify the two expected fields are present (order may vary).
	gotFields := map[string]bool{}
	for _, c := range recordedChanges {
		gotFields[c.Field] = true
	}
	if !gotFields["title"] {
		t.Error("expected 'title' in change diff")
	}
	if !gotFields["description"] {
		t.Error("expected 'description' in change diff")
	}
}

// PSY-563: when no fields change, RecordRevision is NOT called (the
// revisionService.RecordRevision short-circuits on len(changes)==0; we
// short-circuit one level up to avoid the goroutine entirely).
func TestUpdateShowHandler_SkipsRevisionWhenNoChanges(t *testing.T) {
	userID := uint(5)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{
				ID:          1,
				SubmittedBy: &userID,
				Title:       "Same Title",
				Status:      "pending",
			}, nil
		},
		UpdateShowWithRelationsFn: func(showID uint, _ *contracts.UpdateShowRequest, _ []contracts.CreateShowVenue, _ []contracts.CreateShowArtist, _ bool) (*contracts.ShowResponse, []contracts.OrphanedArtist, error) {
			return &contracts.ShowResponse{
				ID:    showID,
				Title: "Same Title",
			}, nil, nil
		},
	}

	var called bool
	revisionMock := &testhelpers.MockRevisionService{
		RecordRevisionFn: func(_ string, _ uint, _ uint, _ []adminm.FieldChange, _ string) error {
			called = true
			return nil
		},
	}

	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, revisionMock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})
	title := "Same Title"
	summary := "Pointless save"
	req := &UpdateShowRequest{ShowID: "1"}
	req.Body.Title = &title
	req.Body.Summary = &summary

	_, err := h.UpdateShowHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give the goroutine a chance — but it should never fire because
	// computeShowChanges returns []. We sleep briefly to make sure no async
	// call slips in.
	time.Sleep(20 * time.Millisecond)
	if called {
		t.Error("expected RecordRevision to NOT be called when no fields changed")
	}
}

// PSY-563: computeShowChanges returns a non-empty diff for every supported
// scalar field that differs between old and new. Pure function, no async.
func TestComputeShowChanges_DiffShape(t *testing.T) {
	oldCity := "Phoenix"
	newCity := "Mesa"
	oldDesc := "old desc"
	newDesc := "new desc"
	oldPrice := 10.0
	newPrice := 15.0
	oldImage := "https://old.example/flyer.png"
	newImage := "https://new.example/flyer.png"
	oldDate := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	newDate := time.Date(2026, 5, 2, 21, 0, 0, 0, time.UTC)

	old := &contracts.ShowResponse{
		Title:       "Old Title",
		EventDate:   oldDate,
		City:        &oldCity,
		Price:       &oldPrice,
		Description: &oldDesc,
		ImageURL:    &oldImage,
	}
	updated := &contracts.ShowResponse{
		Title:       "New Title",
		EventDate:   newDate,
		City:        &newCity,
		Price:       &newPrice,
		Description: &newDesc,
		ImageURL:    &newImage,
	}

	changes := computeShowChanges(old, updated)
	expected := map[string]bool{
		"title":       true,
		"event_date":  true,
		"city":        true,
		"price":       true,
		"description": true,
		"image_url":   true,
	}
	if len(changes) != len(expected) {
		t.Fatalf("expected %d changes, got %d: %+v", len(expected), len(changes), changes)
	}
	for _, c := range changes {
		if !expected[c.Field] {
			t.Errorf("unexpected field in diff: %q", c.Field)
		}
	}
}

func TestComputeShowChanges_NoChanges(t *testing.T) {
	city := "Phoenix"
	desc := "desc"
	now := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	old := &contracts.ShowResponse{Title: "T", EventDate: now, City: &city, Description: &desc}
	updated := &contracts.ShowResponse{Title: "T", EventDate: now, City: &city, Description: &desc}

	changes := computeShowChanges(old, updated)
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %d: %+v", len(changes), changes)
	}
}

func TestComputeShowChanges_NilToValueAndBack(t *testing.T) {
	desc := "now has a description"
	old := &contracts.ShowResponse{Title: "T"}                     // Description nil
	updated := &contracts.ShowResponse{Title: "T", Description: &desc} // Description set

	changes := computeShowChanges(old, updated)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (description nil -> value), got %d", len(changes))
	}
	if changes[0].Field != "description" {
		t.Errorf("expected description field, got %q", changes[0].Field)
	}
	if changes[0].OldValue != "" {
		t.Errorf("expected old description to be empty string (nil → \"\"), got %v", changes[0].OldValue)
	}
	if changes[0].NewValue != desc {
		t.Errorf("expected new description=%q, got %v", desc, changes[0].NewValue)
	}
}

// ============================================================================
// Mock-based tests: DeleteShowHandler
// ============================================================================

func TestDeleteShowHandler_OwnerSuccess(t *testing.T) {
	userID := uint(5)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID}, nil
		},
		DeleteShowFn: func(showID uint) error {
			if showID != 1 {
				t.Errorf("expected showID=1, got %d", showID)
			}
			return nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

	_, err := h.DeleteShowHandler(ctx, &DeleteShowRequest{ShowID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteShowHandler_AdminSuccess(t *testing.T) {
	otherUser := uint(99)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &otherUser}, nil
		},
		DeleteShowFn: func(_ uint) error { return nil },
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	_, err := h.DeleteShowHandler(ctx, &DeleteShowRequest{ShowID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteShowHandler_NotFound(t *testing.T) {
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteShowHandler(ctx, &DeleteShowRequest{ShowID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteShowHandler_Unauthorized(t *testing.T) {
	otherUser := uint(99)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &otherUser}, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

	_, err := h.DeleteShowHandler(ctx, &DeleteShowRequest{ShowID: "1"})
	testhelpers.AssertHumaError(t, err, 403)
}

func TestDeleteShowHandler_ServiceError(t *testing.T) {
	userID := uint(1)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID}, nil
		},
		DeleteShowFn: func(_ uint) error { return fmt.Errorf("delete failed") },
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteShowHandler(ctx, &DeleteShowRequest{ShowID: "1"})
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Mock-based tests: UnpublishShowHandler
// ============================================================================

func TestUnpublishShowHandler_Success(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		UnpublishShowFn: func(showID, userID uint, isAdmin bool) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: showID, Status: "pending", Title: "Test"}, nil
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.UnpublishShowHandler(ctx, &UnpublishShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "pending" {
		t.Errorf("expected status='pending', got %q", resp.Body.Status)
	}
}

func TestUnpublishShowHandler_NotFound(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		UnpublishShowFn: func(_, _ uint, _ bool) (*contracts.ShowResponse, error) {
			return nil, apperrors.ErrShowNotFound(42)
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UnpublishShowHandler(ctx, &UnpublishShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUnpublishShowHandler_Unauthorized(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		UnpublishShowFn: func(_, _ uint, _ bool) (*contracts.ShowResponse, error) {
			return nil, apperrors.ErrShowUnpublishUnauthorized(42)
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

	_, err := h.UnpublishShowHandler(ctx, &UnpublishShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 403)
}

// ============================================================================
// Mock-based tests: MakePrivateShowHandler
// ============================================================================

func TestMakePrivateShowHandler_Success(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		MakePrivateShowFn: func(showID, userID uint, isAdmin bool) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: showID, Status: "private", Title: "Test"}, nil
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.MakePrivateShowHandler(ctx, &MakePrivateShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "private" {
		t.Errorf("expected status='private', got %q", resp.Body.Status)
	}
}

func TestMakePrivateShowHandler_NotFound(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		MakePrivateShowFn: func(_, _ uint, _ bool) (*contracts.ShowResponse, error) {
			return nil, apperrors.ErrShowNotFound(42)
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.MakePrivateShowHandler(ctx, &MakePrivateShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestMakePrivateShowHandler_Unauthorized(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		MakePrivateShowFn: func(_, _ uint, _ bool) (*contracts.ShowResponse, error) {
			return nil, apperrors.ErrShowMakePrivateUnauthorized(42)
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

	_, err := h.MakePrivateShowHandler(ctx, &MakePrivateShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 403)
}

// ============================================================================
// Mock-based tests: PublishShowHandler
// ============================================================================

func TestPublishShowHandler_Success(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		PublishShowFn: func(showID, userID uint, isAdmin bool) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: showID, Status: "approved", Title: "Test"}, nil
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.PublishShowHandler(ctx, &PublishShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "approved" {
		t.Errorf("expected status='approved', got %q", resp.Body.Status)
	}
}

func TestPublishShowHandler_NotFound(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		PublishShowFn: func(_, _ uint, _ bool) (*contracts.ShowResponse, error) {
			return nil, apperrors.ErrShowNotFound(42)
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.PublishShowHandler(ctx, &PublishShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestPublishShowHandler_Unauthorized(t *testing.T) {
	stateMock := &testhelpers.MockShowStateService{
		PublishShowFn: func(_, _ uint, _ bool) (*contracts.ShowResponse, error) {
			return nil, apperrors.ErrShowPublishUnauthorized(42)
		},
	}
	h := NewShowHandler(nil, stateMock, nil, nil, &testhelpers.MockDiscordService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

	_, err := h.PublishShowHandler(ctx, &PublishShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 403)
}

// ============================================================================
// Mock-based tests: SetShowSoldOutHandler
// ============================================================================

func TestSetShowSoldOutHandler_Success(t *testing.T) {
	userID := uint(5)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID}, nil
		},
	}
	stateMock := &testhelpers.MockShowStateService{
		SetShowSoldOutFn: func(showID uint, value bool) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: showID, IsSoldOut: value}, nil
		},
	}
	h := NewShowHandler(showMock, stateMock, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})
	req := &SetShowSoldOutRequest{ShowID: "1"}
	req.Body.Value = true

	resp, err := h.SetShowSoldOutHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.IsSoldOut {
		t.Error("expected is_sold_out=true")
	}
}

func TestSetShowSoldOutHandler_NotOwner(t *testing.T) {
	otherUser := uint(99)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &otherUser}, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})
	req := &SetShowSoldOutRequest{ShowID: "1"}
	req.Body.Value = true

	_, err := h.SetShowSoldOutHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

// ============================================================================
// Mock-based tests: SetShowCancelledHandler
// ============================================================================

func TestSetShowCancelledHandler_Success(t *testing.T) {
	userID := uint(5)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID}, nil
		},
	}
	stateMock := &testhelpers.MockShowStateService{
		SetShowCancelledFn: func(showID uint, value bool) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: showID, IsCancelled: value}, nil
		},
	}
	h := NewShowHandler(showMock, stateMock, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})
	req := &SetShowCancelledRequest{ShowID: "1"}
	req.Body.Value = true

	resp, err := h.SetShowCancelledHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.IsCancelled {
		t.Error("expected is_cancelled=true")
	}
}

func TestSetShowCancelledHandler_NotOwner(t *testing.T) {
	otherUser := uint(99)
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(_ uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &otherUser}, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})
	req := &SetShowCancelledRequest{ShowID: "1"}
	req.Body.Value = true

	_, err := h.SetShowCancelledHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

// ============================================================================
// Mock-based tests: GetMySubmissionsHandler
// ============================================================================

func TestGetMySubmissionsHandler_Success(t *testing.T) {
	showMock := &testhelpers.MockShowService{
		GetUserSubmissionsFn: func(userID uint, limit, offset int) ([]contracts.ShowResponse, int, error) {
			return []contracts.ShowResponse{{ID: 1}, {ID: 2}}, 2, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetMySubmissionsHandler(ctx, &GetMySubmissionsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
}

func TestGetMySubmissionsHandler_ServiceError(t *testing.T) {
	showMock := &testhelpers.MockShowService{
		GetUserSubmissionsFn: func(_ uint, _, _ int) ([]contracts.ShowResponse, int, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetMySubmissionsHandler(ctx, &GetMySubmissionsRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: AIProcessShowHandler
// ============================================================================

func TestAIProcessShowHandler_Success(t *testing.T) {
	extractMock := &testhelpers.MockExtractionService{
		ExtractShowFn: func(req *contracts.ExtractShowRequest) (*contracts.ExtractShowResponse, error) {
			return &contracts.ExtractShowResponse{Success: true}, nil
		},
	}
	h := NewShowHandler(nil, nil, nil, nil, nil, nil, extractMock, nil)

	req := &AIProcessShowRequest{}
	req.Body.Type = "text"
	req.Body.Text = "Band at Venue tonight"

	resp, err := h.AIProcessShowHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

// ============================================================================
// ID Parsing Boundary Tests
// ============================================================================

func TestGetShowHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowFn: func(showID uint) (*contracts.ShowResponse, error) {
			if showID != 0 {
				t.Errorf("expected showID=0, got %d", showID)
			}
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
	_, err := h.GetShowHandler(context.Background(), &GetShowRequest{ShowID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetShowHandler_VeryLargeID(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowFn: func(showID uint) (*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
	_, err := h.GetShowHandler(context.Background(), &GetShowRequest{ShowID: "4294967295"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetShowHandler_OverflowID(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowBySlugFn: func(slug string) (*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
	_, err := h.GetShowHandler(context.Background(), &GetShowRequest{ShowID: "99999999999"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateShowHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowFn: func(showID uint) (*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.UpdateShowHandler(ctx, &UpdateShowRequest{ShowID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteShowHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowFn: func(showID uint) (*contracts.ShowResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.DeleteShowHandler(ctx, &DeleteShowRequest{ShowID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteShowHandler_OverflowID(t *testing.T) {
	h := testShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.DeleteShowHandler(ctx, &DeleteShowRequest{ShowID: "99999999999"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAIProcessShowHandler_ServiceError(t *testing.T) {
	extractMock := &testhelpers.MockExtractionService{
		ExtractShowFn: func(_ *contracts.ExtractShowRequest) (*contracts.ExtractShowResponse, error) {
			return nil, fmt.Errorf("AI service down")
		},
	}
	h := NewShowHandler(nil, nil, nil, nil, nil, nil, extractMock, nil)

	req := &AIProcessShowRequest{}
	req.Body.Type = "text"
	req.Body.Text = "test"

	resp, err := h.AIProcessShowHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// AI errors return success=false in body, not huma errors
	if resp.Body.Success {
		t.Error("expected success=false on extraction error")
	}
}
