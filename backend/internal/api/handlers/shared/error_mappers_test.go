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

func TestMapEntityReportError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.EntityReportError
		status int
	}{
		{"entity not found", apperrors.ErrEntityReportEntityNotFound("artist", 9), 404},
		{"report not found", apperrors.ErrEntityReportNotFound(), 404},
		{"duplicate pending", apperrors.ErrEntityReportDuplicatePending(), 409},
		{"already reviewed", apperrors.ErrEntityReportAlreadyReviewed("resolved"), 409},
		{"invalid entity type", apperrors.ErrEntityReportInvalidEntityType("planet"), 422},
		{"invalid report type", apperrors.ErrEntityReportInvalidReportType("nope", "artist"), 422},
		{"internal", apperrors.ErrEntityReportInternal(stderrors.New("db down")), 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapEntityReportError(tc.err)
			if got == nil {
				t.Fatalf("MapEntityReportError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapEntityReportError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapEntityReportError_NonEntityReportErrorReturnsNil(t *testing.T) {
	if got := MapEntityReportError(stderrors.New("boom")); got != nil {
		t.Errorf("MapEntityReportError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.EntityReportError{Code: "ENTITY_REPORT_NEW_CODE", Message: "x"}
	if got := MapEntityReportError(unknown); got != nil {
		t.Errorf("MapEntityReportError(unknown code) = %v, want nil", got)
	}
}

func TestMapLeaderboardError_CodeToStatus(t *testing.T) {
	if got := MapLeaderboardError(apperrors.ErrLeaderboardInvalidDimension("bogus")); got == nil {
		t.Fatal("MapLeaderboardError(invalid dimension) = nil, want 422")
	} else if s := statusOf(t, got); s != 422 {
		t.Errorf("invalid-dimension status = %d, want 422", s)
	}

	if got := MapLeaderboardError(apperrors.ErrLeaderboardInvalidPeriod("decade")); got == nil {
		t.Fatal("MapLeaderboardError(invalid period) = nil, want 422")
	} else if s := statusOf(t, got); s != 422 {
		t.Errorf("invalid-period status = %d, want 422", s)
	}

	if got := MapLeaderboardError(apperrors.ErrLeaderboardInternal(stderrors.New("db down"))); got == nil {
		t.Fatal("MapLeaderboardError(internal) = nil, want 500")
	} else if s := statusOf(t, got); s != 500 {
		t.Errorf("internal status = %d, want 500", s)
	}
}

func TestMapLeaderboardError_NonLeaderboardErrorReturnsNil(t *testing.T) {
	if got := MapLeaderboardError(stderrors.New("boom")); got != nil {
		t.Errorf("MapLeaderboardError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.LeaderboardError{Code: "LEADERBOARD_NEW_CODE", Message: "x"}
	if got := MapLeaderboardError(unknown); got != nil {
		t.Errorf("MapLeaderboardError(unknown code) = %v, want nil", got)
	}
}

func TestMapDataQualityError_CodeToStatus(t *testing.T) {
	if got := MapDataQualityError(apperrors.ErrDataQualityUnknownCategory("nope")); got == nil {
		t.Fatal("MapDataQualityError(unknown category) = nil, want 422")
	} else if s := statusOf(t, got); s != 422 {
		t.Errorf("unknown-category status = %d, want 422", s)
	}

	if got := MapDataQualityError(apperrors.ErrDataQualityInternal(stderrors.New("db down"))); got == nil {
		t.Fatal("MapDataQualityError(internal) = nil, want 500")
	} else if s := statusOf(t, got); s != 500 {
		t.Errorf("internal status = %d, want 500", s)
	}
}

func TestMapDataQualityError_NonDataQualityErrorReturnsNil(t *testing.T) {
	if got := MapDataQualityError(stderrors.New("boom")); got != nil {
		t.Errorf("MapDataQualityError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.DataQualityError{Code: "DATA_QUALITY_NEW_CODE", Message: "x"}
	if got := MapDataQualityError(unknown); got != nil {
		t.Errorf("MapDataQualityError(unknown code) = %v, want nil", got)
	}
}

func TestMapAutoPromotionError_CodeToStatus(t *testing.T) {
	if got := MapAutoPromotionError(apperrors.ErrAutoPromotionUserNotFound()); got == nil {
		t.Fatal("MapAutoPromotionError(user not found) = nil, want 404")
	} else if s := statusOf(t, got); s != 404 {
		t.Errorf("user-not-found status = %d, want 404", s)
	}

	if got := MapAutoPromotionError(apperrors.ErrAutoPromotionInternal(stderrors.New("db down"))); got == nil {
		t.Fatal("MapAutoPromotionError(internal) = nil, want 500")
	} else if s := statusOf(t, got); s != 500 {
		t.Errorf("internal status = %d, want 500", s)
	}
}

func TestMapAutoPromotionError_NonAutoPromotionErrorReturnsNil(t *testing.T) {
	if got := MapAutoPromotionError(stderrors.New("boom")); got != nil {
		t.Errorf("MapAutoPromotionError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.AutoPromotionError{Code: "AUTO_PROMOTION_NEW_CODE", Message: "x"}
	if got := MapAutoPromotionError(unknown); got != nil {
		t.Errorf("MapAutoPromotionError(unknown code) = %v, want nil", got)
	}
}

func TestMapProfileError_CodeToStatus(t *testing.T) {
	if got := MapProfileError(apperrors.ErrProfileSectionNotFound()); got == nil {
		t.Fatal("MapProfileError(section not found) = nil, want 404")
	} else if s := statusOf(t, got); s != 404 {
		t.Errorf("section-not-found status = %d, want 404", s)
	}

	if got := MapProfileError(apperrors.ErrProfileSectionInvalid("title too long")); got == nil {
		t.Fatal("MapProfileError(section invalid) = nil, want 422")
	} else if s := statusOf(t, got); s != 422 {
		t.Errorf("section-invalid status = %d, want 422", s)
	}

	if got := MapProfileError(apperrors.ErrProfileInternal(stderrors.New("db down"))); got == nil {
		t.Fatal("MapProfileError(internal) = nil, want 500")
	} else if s := statusOf(t, got); s != 500 {
		t.Errorf("internal status = %d, want 500", s)
	}
}

func TestMapProfileError_NonProfileErrorReturnsNil(t *testing.T) {
	if got := MapProfileError(stderrors.New("boom")); got != nil {
		t.Errorf("MapProfileError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.ProfileError{Code: "PROFILE_NEW_CODE", Message: "x"}
	if got := MapProfileError(unknown); got != nil {
		t.Errorf("MapProfileError(unknown code) = %v, want nil", got)
	}
}

func TestMapPendingEditError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.PendingEditError
		status int
	}{
		{"entity not found (create)", apperrors.ErrPendingEditEntityNotFound("artist", 9), 404},
		{"entity gone (approve)", apperrors.ErrPendingEditEntityGone("artist", 9), 422},
		{"edit not found", apperrors.ErrPendingEditNotFound(), 404},
		{"not pending", apperrors.ErrPendingEditNotPending("approved"), 409},
		{"duplicate", apperrors.ErrPendingEditDuplicate(stderrors.New("unique constraint")), 409},
		{"not submitter", apperrors.ErrPendingEditNotSubmitter(), 403},
		{"invalid entity type", apperrors.ErrPendingEditInvalidEntityType("show"), 422},
		{"invalid request", apperrors.ErrPendingEditInvalidRequest("no changes provided"), 422},
		{"internal", apperrors.ErrPendingEditInternal(stderrors.New("db down")), 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapPendingEditError(tc.err)
			if got == nil {
				t.Fatalf("MapPendingEditError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapPendingEditError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapPendingEditError_NonPendingEditErrorReturnsNil(t *testing.T) {
	if got := MapPendingEditError(stderrors.New("boom")); got != nil {
		t.Errorf("MapPendingEditError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.PendingEditError{Code: "PENDING_EDIT_NEW_CODE", Message: "x"}
	if got := MapPendingEditError(unknown); got != nil {
		t.Errorf("MapPendingEditError(unknown code) = %v, want nil", got)
	}
}

func TestMapCommentError_CodeToStatus(t *testing.T) {
	// Cover every CommentError code so adding a code without a status mapping
	// is caught. The 429 + Retry-After paths have a dedicated test below
	// (TestMapCommentError_RateLimitedCarriesRetryAfter) because the
	// assertion is more involved than a status check.
	cases := []struct {
		name   string
		err    *apperrors.CommentError
		status int
	}{
		{"not found", apperrors.ErrCommentNotFound(), 404},
		{"parent not found", apperrors.ErrCommentParentNotFound(), 404},
		{"entity not found", apperrors.ErrCommentEntityNotFound("show", 7), 404},
		{"user not found", apperrors.ErrCommentUserNotFound(), 404},
		{"thread root not found", apperrors.ErrCommentThreadRootNotFound(), 404},
		{"forbidden", apperrors.ErrCommentForbidden("only the comment author"), 403},
		{"invalid entity type", apperrors.ErrCommentInvalidEntityType("nope"), 400},
		{"invalid reply permission", apperrors.ErrCommentInvalidReplyPermission("blorp"), 400},
		{"unsupported reply permission on parent", apperrors.ErrCommentUnsupportedReplyPermission("legacy"), 400},
		{"body required", apperrors.ErrCommentBodyRequired("comment body is required"), 400},
		{"body too long", apperrors.ErrCommentBodyTooLong("exceeds max"), 400},
		{"max depth", apperrors.ErrCommentMaxDepthExceeded(2), 400},
		{"parent mismatch", apperrors.ErrCommentParentMismatch(), 400},
		{"not thread root", apperrors.ErrCommentNotThreadRoot(), 400},
		{"field validation", apperrors.ErrCommentFieldValidation("sound_quality must be between 1 and 5"), 400},
		{"internal", apperrors.ErrCommentInternal("failed to load comment", stderrors.New("db down")), 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapCommentError(tc.err)
			if got == nil {
				t.Fatalf("MapCommentError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapCommentError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

// 429 must carry Retry-After so the inline comment / reply / field-note
// rate-limit banners populate countdown copy without parsing the body.
// 60s for the per-entity cooldown, 3600s for the tier-based hourly cap.
func TestMapCommentError_RateLimitedCarriesRetryAfter(t *testing.T) {
	cases := []struct {
		name       string
		err        *apperrors.CommentError
		wantStatus int
		wantHeader string
	}{
		{"entity cooldown", apperrors.ErrCommentRateLimitedEntity(), 429, "60"},
		{"hourly cap", apperrors.ErrCommentRateLimitedHourly(5, "new"), 429, "3600"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapCommentError(tc.err)
			if got == nil {
				t.Fatalf("MapCommentError(%v) = nil, want %d", tc.err, tc.wantStatus)
			}
			if s := statusOf(t, got); s != tc.wantStatus {
				t.Errorf("status = %d, want %d", s, tc.wantStatus)
			}
			var he huma.HeadersError
			if !stderrors.As(got, &he) {
				t.Fatalf("expected huma.HeadersError, got %T", got)
			}
			if v := he.GetHeaders().Get("Retry-After"); v != tc.wantHeader {
				t.Errorf("Retry-After = %q, want %q", v, tc.wantHeader)
			}
		})
	}
}

func TestMapCommentError_NonCommentErrorReturnsNil(t *testing.T) {
	if got := MapCommentError(stderrors.New("boom")); got != nil {
		t.Errorf("MapCommentError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.CommentError{Code: "COMMENT_NEW_CODE", Message: "x"}
	if got := MapCommentError(unknown); got != nil {
		t.Errorf("MapCommentError(unknown code) = %v, want nil", got)
	}
}

func TestMapCommentAdminError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.CommentAdminError
		status int
	}{
		{"already visible", apperrors.ErrCommentAdminAlreadyVisible(), 409},
		{"not pending", apperrors.ErrCommentAdminNotPending(), 409},
		{"access required", apperrors.ErrCommentAdminAccessRequired(), 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapCommentAdminError(tc.err)
			if got == nil {
				t.Fatalf("MapCommentAdminError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapCommentAdminError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapCommentAdminError_NonCommentAdminErrorReturnsNil(t *testing.T) {
	if got := MapCommentAdminError(stderrors.New("boom")); got != nil {
		t.Errorf("MapCommentAdminError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.CommentAdminError{Code: "COMMENT_ADMIN_NEW_CODE", Message: "x"}
	if got := MapCommentAdminError(unknown); got != nil {
		t.Errorf("MapCommentAdminError(unknown code) = %v, want nil", got)
	}
}

func TestMapFieldNoteError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.FieldNoteError
		status int
	}{
		{"show not found", apperrors.ErrFieldNoteShowNotFound(), 404},
		{"show future", apperrors.ErrFieldNoteShowFuture(), 400},
		{"artist not on bill", apperrors.ErrFieldNoteArtistNotOnBill(), 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapFieldNoteError(tc.err)
			if got == nil {
				t.Fatalf("MapFieldNoteError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapFieldNoteError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapFieldNoteError_NonFieldNoteErrorReturnsNil(t *testing.T) {
	if got := MapFieldNoteError(stderrors.New("boom")); got != nil {
		t.Errorf("MapFieldNoteError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.FieldNoteError{Code: "FIELD_NOTE_NEW_CODE", Message: "x"}
	if got := MapFieldNoteError(unknown); got != nil {
		t.Errorf("MapFieldNoteError(unknown code) = %v, want nil", got)
	}
}

func TestMapCommentVoteError_CodeToStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    *apperrors.CommentVoteError
		status int
	}{
		{"comment not found", apperrors.ErrCommentVoteCommentNotFound(), 404},
		{"self vote", apperrors.ErrCommentVoteSelfVote(), 403},
		{"invalid direction", apperrors.ErrCommentVoteInvalidDirection(), 400},
		{"internal", apperrors.ErrCommentVoteInternal("failed to vote", stderrors.New("db down")), 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapCommentVoteError(tc.err)
			if got == nil {
				t.Fatalf("MapCommentVoteError(%v) = nil, want status %d", tc.err, tc.status)
			}
			if s := statusOf(t, got); s != tc.status {
				t.Errorf("MapCommentVoteError(%v) status = %d, want %d", tc.err, s, tc.status)
			}
		})
	}
}

func TestMapCommentVoteError_NonCommentVoteErrorReturnsNil(t *testing.T) {
	if got := MapCommentVoteError(stderrors.New("boom")); got != nil {
		t.Errorf("MapCommentVoteError(plain error) = %v, want nil", got)
	}
	unknown := &apperrors.CommentVoteError{Code: "COMMENT_VOTE_NEW_CODE", Message: "x"}
	if got := MapCommentVoteError(unknown); got != nil {
		t.Errorf("MapCommentVoteError(unknown code) = %v, want nil", got)
	}
}
