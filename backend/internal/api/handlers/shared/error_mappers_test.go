package shared

import (
	stderrors "errors"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	apperrors "psychic-homily-backend/internal/errors"
)

// statusOf type-asserts a Huma error to its HTTP status code. The mappers
// return values produced by huma.Error4XX(...), all of which satisfy
// huma.StatusError, so a failed assertion is itself a test failure.
func statusOf(t *testing.T, err error) int {
	t.Helper()
	var statusErr huma.StatusError
	if !stderrors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T (%v)", err, err)
	}
	return statusErr.GetStatus()
}

func TestMapTagError_CodeToStatus(t *testing.T) {
	// Every tag code maps to exactly one status; cover the whole switch so a
	// future code added without a status mapping is caught.
	cases := []struct {
		name   string
		err    *apperrors.TagError
		status int
	}{
		{"not found", apperrors.ErrTagNotFound(7), 404},
		{"not found by slug", apperrors.ErrTagNotFoundBySlug("punk"), 404},
		{"exists", apperrors.ErrTagExists("punk"), 409},
		{"alias exists", apperrors.ErrTagAliasExists("hardcore"), 409},
		{"entity tag exists", apperrors.ErrEntityTagExists(1, "artist", 2), 409},
		{"entity tag not found", apperrors.ErrEntityTagNotFound(1, "artist", 2), 404},
		{"creation forbidden", apperrors.ErrTagCreationForbidden(), 403},
		{"name invalid", apperrors.ErrTagNameInvalid("too long"), 422},
		{"merge invalid", apperrors.ErrTagMergeInvalid("self-merge"), 422},
		{"merge alias conflict", apperrors.ErrTagMergeAliasConflict("hc", 3), 409},
		{"hierarchy cycle", apperrors.ErrTagHierarchyCycle("would loop"), 422},
		{"hierarchy not genre", apperrors.ErrTagHierarchyNotGenre("blue", "mood"), 422},
		{"bulk action invalid", apperrors.ErrTagBulkActionInvalid("unknown verb"), 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapTagError(tc.err)
			if got == nil {
				t.Fatalf("MapTagError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapTagError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapTagError_NonTagErrorReturnsNil(t *testing.T) {
	// A plain error must fall through so the caller can try other mappers.
	if got := MapTagError(stderrors.New("boom")); got != nil {
		t.Errorf("MapTagError(plain error) = %v, want nil", got)
	}
	// An unknown tag code is not in the switch and also falls through.
	unknown := &apperrors.TagError{Code: "TAG_SOMETHING_NEW", Message: "x"}
	if got := MapTagError(unknown); got != nil {
		t.Errorf("MapTagError(unknown code) = %v, want nil", got)
	}
}

func TestMapCollectionError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.CollectionError
		status int
	}{
		{"not found", apperrors.ErrCollectionNotFound("mix"), 404},
		{"forbidden", apperrors.ErrCollectionForbidden("mix"), 403},
		{"item exists", apperrors.ErrCollectionItemExists(1, "artist", 2), 409},
		{"item not found", apperrors.ErrCollectionItemNotFound(3), 404},
		{"invalid request", apperrors.ErrCollectionInvalidRequest("bad sort"), 422},
		{"tag limit exceeded", apperrors.ErrCollectionTagLimitExceeded(10, 10), 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapCollectionError(tc.err)
			if got == nil {
				t.Fatalf("MapCollectionError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapCollectionError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapCollectionError_LimitExceededCarriesStructuredDetail(t *testing.T) {
	// CodeCollectionLimitExceeded → 403 AND attaches a structured ErrorDetail
	// (via collectionLimitDetail) so the frontend reads tier/used/limit/kind
	// off errors[].value without parsing the human string.
	src := apperrors.ErrCollectionLimitExceeded("free", 5, 5, apperrors.CollectionLimitKindOwned)
	got := MapCollectionError(src)
	if got == nil {
		t.Fatal("MapCollectionError(limit exceeded) = nil, want 403 with detail")
	}
	if s := statusOf(t, got); s != 403 {
		t.Errorf("limit-exceeded status = %d, want 403", s)
	}

	model, ok := got.(*huma.ErrorModel)
	if !ok {
		t.Fatalf("expected *huma.ErrorModel, got %T", got)
	}
	if len(model.Errors) != 1 {
		t.Fatalf("expected 1 structured error detail, got %d", len(model.Errors))
	}
	detail := model.Errors[0]
	if detail.Location != "body" {
		t.Errorf("detail.Location = %q, want \"body\"", detail.Location)
	}
	value, ok := detail.Value.(map[string]any)
	if !ok {
		t.Fatalf("detail.Value type = %T, want map[string]any", detail.Value)
	}
	if value["code"] != apperrors.CodeCollectionLimitExceeded {
		t.Errorf("detail code = %v, want %s", value["code"], apperrors.CodeCollectionLimitExceeded)
	}
	if value["tier"] != "free" {
		t.Errorf("detail tier = %v, want \"free\"", value["tier"])
	}
	if value["used"] != 5 {
		t.Errorf("detail used = %v, want 5", value["used"])
	}
	if value["limit"] != 5 {
		t.Errorf("detail limit = %v, want 5", value["limit"])
	}
	if value["soft_cap_kind"] != apperrors.CollectionLimitKindOwned {
		t.Errorf("detail soft_cap_kind = %v, want %s", value["soft_cap_kind"], apperrors.CollectionLimitKindOwned)
	}
}

func TestMapCollectionError_NonCollectionErrorReturnsNil(t *testing.T) {
	if got := MapCollectionError(stderrors.New("boom")); got != nil {
		t.Errorf("MapCollectionError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.CollectionError{Code: "COLLECTION_NEW_CODE", Message: "x"}
	if got := MapCollectionError(unknown); got != nil {
		t.Errorf("MapCollectionError(unknown code) = %v, want nil", got)
	}
}

func TestMapFollowError_CodeToStatus(t *testing.T) {
	if got := MapFollowError(apperrors.ErrFollowInvalidEntityType("genre")); got == nil {
		t.Fatal("MapFollowError(invalid type) = nil, want 422")
	} else if s := statusOf(t, got); s != 422 {
		t.Errorf("invalid-type status = %d, want 422", s)
	}

	if got := MapFollowError(apperrors.ErrFollowInternal(stderrors.New("db down"))); got == nil {
		t.Fatal("MapFollowError(internal) = nil, want 500")
	} else if s := statusOf(t, got); s != 500 {
		t.Errorf("internal status = %d, want 500", s)
	}
}

func TestMapFollowError_NonFollowErrorReturnsNil(t *testing.T) {
	if got := MapFollowError(stderrors.New("boom")); got != nil {
		t.Errorf("MapFollowError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.FollowError{Code: "FOLLOW_NEW_CODE", Message: "x"}
	if got := MapFollowError(unknown); got != nil {
		t.Errorf("MapFollowError(unknown code) = %v, want nil", got)
	}
}

func TestMapAttendanceError_CodeToStatus(t *testing.T) {
	if got := MapAttendanceError(apperrors.ErrAttendanceShowNotFound()); got == nil {
		t.Fatal("MapAttendanceError(show not found) = nil, want 404")
	} else if s := statusOf(t, got); s != 404 {
		t.Errorf("show-not-found status = %d, want 404", s)
	}

	if got := MapAttendanceError(apperrors.ErrAttendanceInvalidStatus("maybe")); got == nil {
		t.Fatal("MapAttendanceError(invalid status) = nil, want 422")
	} else if s := statusOf(t, got); s != 422 {
		t.Errorf("invalid-status status = %d, want 422", s)
	}

	if got := MapAttendanceError(apperrors.ErrAttendanceInternal(stderrors.New("db down"))); got == nil {
		t.Fatal("MapAttendanceError(internal) = nil, want 500")
	} else if s := statusOf(t, got); s != 500 {
		t.Errorf("internal status = %d, want 500", s)
	}
}

func TestMapAttendanceError_NonAttendanceErrorReturnsNil(t *testing.T) {
	if got := MapAttendanceError(stderrors.New("boom")); got != nil {
		t.Errorf("MapAttendanceError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.AttendanceError{Code: "ATTENDANCE_NEW_CODE", Message: "x"}
	if got := MapAttendanceError(unknown); got != nil {
		t.Errorf("MapAttendanceError(unknown code) = %v, want nil", got)
	}
}

func TestMapNotificationFilterError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.NotificationFilterError
		status int
	}{
		{"not found", apperrors.ErrFilterNotFound(), 404},
		{"validation", apperrors.ErrFilterValidation("at least one filter criteria is required"), 422},
		{"internal", apperrors.ErrFilterInternal(stderrors.New("db down")), 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapNotificationFilterError(tc.err)
			if got == nil {
				t.Fatalf("MapNotificationFilterError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapNotificationFilterError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapArtistError_CodeToStatus(t *testing.T) {
	// Cover the whole switch so a future code added without a status mapping is
	// caught. HasShows is 409 (preserves the artist delete handler's existing
	// conflict status — intentionally distinct from venue HasShows, which is 422).
	cases := []struct {
		name   string
		err    *apperrors.ArtistError
		status int
	}{
		{"not found", apperrors.ErrArtistNotFound(7), 404},
		{"alias not found", apperrors.ErrArtistAliasNotFound(), 404},
		{"exists", apperrors.ErrArtistExists("Frozen Soul"), 409},
		{"alias exists", apperrors.ErrArtistAliasExists("alias 'x' already exists"), 409},
		{"has shows", apperrors.ErrArtistHasShows(7, 3), 409},
		{"merge self", apperrors.ErrArtistMergeSelf(), 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapArtistError(tc.err)
			if got == nil {
				t.Fatalf("MapArtistError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapArtistError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapNotificationFilterError_NonFilterErrorReturnsNil(t *testing.T) {
	if got := MapNotificationFilterError(stderrors.New("boom")); got != nil {
		t.Errorf("MapNotificationFilterError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.NotificationFilterError{Code: "FILTER_NEW_CODE", Message: "x"}
	if got := MapNotificationFilterError(unknown); got != nil {
		t.Errorf("MapNotificationFilterError(unknown code) = %v, want nil", got)
	}
}

func TestMapArtistError_NonArtistErrorReturnsNil(t *testing.T) {
	if got := MapArtistError(stderrors.New("boom")); got != nil {
		t.Errorf("MapArtistError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.ArtistError{Code: "ARTIST_NEW_CODE", Message: "x"}
	if got := MapArtistError(unknown); got != nil {
		t.Errorf("MapArtistError(unknown code) = %v, want nil", got)
	}
}

func TestMapVenueError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.VenueError
		status int
	}{
		{"not found", apperrors.ErrVenueNotFound(7), 404},
		{"has shows", apperrors.ErrVenueHasShows(7, 3), 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapVenueError(tc.err)
			if got == nil {
				t.Fatalf("MapVenueError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapVenueError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapVenueError_NonVenueErrorReturnsNil(t *testing.T) {
	if got := MapVenueError(stderrors.New("boom")); got != nil {
		t.Errorf("MapVenueError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.VenueError{Code: "VENUE_NEW_CODE", Message: "x"}
	if got := MapVenueError(unknown); got != nil {
		t.Errorf("MapVenueError(unknown code) = %v, want nil", got)
	}
}

func TestMapFestivalError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.FestivalError
		status int
	}{
		{"not found", apperrors.ErrFestivalNotFound(7), 404},
		{"artist not found", apperrors.ErrFestivalArtistNotFound(), 404},
		{"artist not in lineup", apperrors.ErrFestivalArtistNotInLineup(), 404},
		{"venue not found", apperrors.ErrFestivalVenueNotFound(), 404},
		{"venue not in festival", apperrors.ErrFestivalVenueNotInFestival(), 404},
		{"exists", apperrors.ErrFestivalExists("M3F 2026"), 409},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapFestivalError(tc.err)
			if got == nil {
				t.Fatalf("MapFestivalError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapFestivalError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapFestivalError_NonFestivalErrorReturnsNil(t *testing.T) {
	if got := MapFestivalError(stderrors.New("boom")); got != nil {
		t.Errorf("MapFestivalError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.FestivalError{Code: "FESTIVAL_NEW_CODE", Message: "x"}
	if got := MapFestivalError(unknown); got != nil {
		t.Errorf("MapFestivalError(unknown code) = %v, want nil", got)
	}
}

func TestMapFestivalIntelligenceError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.FestivalIntelligenceError
		status int
	}{
		{"not found", apperrors.ErrFestivalIntelNotFound("festival A not found"), 404},
		{"no festivals", apperrors.ErrFestivalIntelNoFestivals("m3f"), 404},
		{"insufficient years", apperrors.ErrFestivalIntelInsufficientYears(), 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapFestivalIntelligenceError(tc.err)
			if got == nil {
				t.Fatalf("MapFestivalIntelligenceError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapFestivalIntelligenceError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapFestivalIntelligenceError_NonIntelErrorReturnsNil(t *testing.T) {
	if got := MapFestivalIntelligenceError(stderrors.New("boom")); got != nil {
		t.Errorf("MapFestivalIntelligenceError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.FestivalIntelligenceError{Code: "FESTIVAL_INTEL_NEW_CODE", Message: "x"}
	if got := MapFestivalIntelligenceError(unknown); got != nil {
		t.Errorf("MapFestivalIntelligenceError(unknown code) = %v, want nil", got)
	}
}

func TestMapSceneError_CodeToStatus(t *testing.T) {
	if got := MapSceneError(apperrors.ErrSceneNotFound("scene not found: Phoenix, AZ")); got == nil {
		t.Fatal("MapSceneError(scene not found) = nil, want 404")
	} else if s := statusOf(t, got); s != 404 {
		t.Errorf("scene-not-found status = %d, want 404", s)
	}
}

func TestMapSceneError_NonSceneErrorReturnsNil(t *testing.T) {
	if got := MapSceneError(stderrors.New("boom")); got != nil {
		t.Errorf("MapSceneError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.SceneError{Code: "SCENE_NEW_CODE", Message: "x"}
	if got := MapSceneError(unknown); got != nil {
		t.Errorf("MapSceneError(unknown code) = %v, want nil", got)
	}
}
