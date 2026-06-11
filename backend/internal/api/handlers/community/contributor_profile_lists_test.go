package community

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// PSY-1046 public profile list handlers
// (GetUserFollowingHandler / GetUserAttendedShowsHandler /
//  GetUserFieldNotesHandler)
//
// The full privacy matrix (stored defaults, hidden, count_only, owner
// bypass, private-profile master gate, SQL semantics) is exercised against a
// real database by the integration suite. These unit tests cover the
// handler-level branches integration can't reach without forcing a live
// service to fail — the service-error → 500 mappings — plus the exact
// service arguments the count-only path sends.
// ============================================================================

// listsMockUserService returns a MockUserService whose lookup resolves to a
// public-profile user with the given (optional) stored privacy settings.
// settings == nil leaves PrivacySettings NULL, so granularPrivacy falls back
// to the defaults (all list fields visible since PSY-1045 flipped
// following/attendance to visible, 2026-06-09).
func listsMockUserService(t *testing.T, id uint, settings *contracts.PrivacySettings) *testhelpers.MockUserService {
	t.Helper()
	return &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) {
			u := &authm.User{ID: id, ProfileVisibility: "public"}
			if settings != nil {
				raw, err := json.Marshal(settings)
				if err != nil {
					t.Fatalf("marshal privacy settings: %v", err)
				}
				rawMsg := json.RawMessage(raw)
				u.PrivacySettings = &rawMsg
			}
			return u, nil
		},
	}
}

// listsVisibleSettings returns stored settings with the two list-relevant
// fields explicitly visible (redundant with the post-PSY-1045 defaults, but
// explicit so these tests don't silently change meaning if defaults move).
func listsVisibleSettings() *contracts.PrivacySettings {
	s := contracts.DefaultPrivacySettings()
	s.Following = contracts.PrivacyVisible
	s.Attendance = contracts.PrivacyVisible
	return &s
}

// listsCountOnlySettings returns stored settings with the two list-relevant
// fields explicitly count_only (no longer the default after PSY-1045).
func listsCountOnlySettings() *contracts.PrivacySettings {
	s := contracts.DefaultPrivacySettings()
	s.Following = contracts.PrivacyCountOnly
	s.Attendance = contracts.PrivacyCountOnly
	return &s
}

// --- GetUserFollowingHandler ---

func TestGetUserFollowing_LookupError(t *testing.T) {
	mockUsers := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) {
			return nil, errors.New("db down")
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, nil, nil, nil)

	_, err := h.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "anyone", Type: "all", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetUserFollowing_ServiceError(t *testing.T) {
	mockUsers := listsMockUserService(t, 7, listsVisibleSettings())
	mockFollow := &testhelpers.MockFollowService{
		GetUserFollowingFn: func(uint, string, int, int) ([]*contracts.FollowingEntityResponse, int64, error) {
			return nil, 0, errors.New("query failed")
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, mockFollow, nil, nil)

	_, err := h.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "publicuser", Type: "all", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetUserFollowing_CountOnlyArgsAndShape(t *testing.T) {
	// Explicit count_only stored settings (the default is visible post-PSY-1045).
	mockUsers := listsMockUserService(t, 7, listsCountOnlySettings())
	var gotType string
	var gotLimit, gotOffset int
	mockFollow := &testhelpers.MockFollowService{
		GetUserFollowingFn: func(_ uint, entityType string, limit, offset int) ([]*contracts.FollowingEntityResponse, int64, error) {
			gotType, gotLimit, gotOffset = entityType, limit, offset
			return []*contracts.FollowingEntityResponse{{Name: "must not leak"}}, 42, nil
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, mockFollow, nil, nil)

	resp, err := h.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "publicuser", Type: "all", Limit: 20, Offset: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Count-only fetches a minimal page ("all" maps to the unfiltered "").
	if gotType != "" || gotLimit != 1 || gotOffset != 0 {
		t.Errorf("expected count-only service call (\"\", 1, 0), got (%q, %d, %d)", gotType, gotLimit, gotOffset)
	}
	// The total survives; any fetched items must NOT leak; the response
	// echoes the request's pagination, not the internal count-call's.
	if resp.Body.Total != 42 {
		t.Errorf("expected total=42, got %d", resp.Body.Total)
	}
	if len(resp.Body.Following) != 0 {
		t.Errorf("count_only must return no items, got %d", len(resp.Body.Following))
	}
	if resp.Body.Limit != 20 || resp.Body.Offset != 5 {
		t.Errorf("expected echoed pagination (20, 5), got (%d, %d)", resp.Body.Limit, resp.Body.Offset)
	}
}

func TestGetUserFollowing_CountOnlyServiceError(t *testing.T) {
	mockUsers := listsMockUserService(t, 7, listsCountOnlySettings())
	mockFollow := &testhelpers.MockFollowService{
		GetUserFollowingFn: func(uint, string, int, int) ([]*contracts.FollowingEntityResponse, int64, error) {
			return nil, 0, errors.New("count failed")
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, mockFollow, nil, nil)

	_, err := h.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "publicuser", Type: "all", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- GetUserAttendedShowsHandler ---

func TestGetUserAttendedShows_ServiceError(t *testing.T) {
	mockUsers := listsMockUserService(t, 7, listsVisibleSettings())
	mockAttendance := &testhelpers.MockAttendanceService{
		GetUserAttendedShowsFn: func(uint, int, int) ([]*contracts.AttendingShowResponse, int64, error) {
			return nil, 0, errors.New("query failed")
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, nil, mockAttendance, nil)

	_, err := h.GetUserAttendedShowsHandler(context.Background(), &GetUserAttendedShowsRequest{
		Username: "publicuser", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetUserAttendedShows_CountOnlyServiceError(t *testing.T) {
	settings := contracts.DefaultPrivacySettings()
	settings.Attendance = contracts.PrivacyCountOnly
	mockUsers := listsMockUserService(t, 7, &settings)
	mockAttendance := &testhelpers.MockAttendanceService{
		GetUserAttendedShowsFn: func(uint, int, int) ([]*contracts.AttendingShowResponse, int64, error) {
			return nil, 0, errors.New("count failed")
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, nil, mockAttendance, nil)

	_, err := h.GetUserAttendedShowsHandler(context.Background(), &GetUserAttendedShowsRequest{
		Username: "publicuser", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- GetUserFieldNotesHandler ---

func TestGetUserFieldNotes_ServiceError(t *testing.T) {
	mockUsers := listsMockUserService(t, 7, nil) // field-notes has no granular gate
	mockFieldNotes := &testhelpers.MockFieldNoteService{
		ListFieldNotesByAuthorFn: func(uint, int, int) ([]*contracts.AuthoredFieldNote, int64, error) {
			return nil, 0, errors.New("query failed")
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, nil, nil, mockFieldNotes)

	_, err := h.GetUserFieldNotesHandler(context.Background(), &GetUserFieldNotesRequest{
		Username: "publicuser", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(t, err, 500)
}
