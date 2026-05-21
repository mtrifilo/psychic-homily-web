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
