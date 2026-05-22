package shared

import (
	"errors"

	"github.com/danielgtaylor/huma/v2"

	apperrors "psychic-homily-backend/internal/errors"
)

// MapTagError converts a TagError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.TagError, leaving the caller free
// to fall through to other error mappers.
//
// PSY-524 convention: semantic violations of the tag domain (invalid name,
// merge rule violation, hierarchy rule violation, bulk-action enum) all map
// to 422 — the request parsed fine; the value is rejected by domain rules.
// 4xx codes for "not found" / "forbidden" / "conflict" are unchanged.
func MapTagError(err error) error {
	var tagErr *apperrors.TagError
	if errors.As(err, &tagErr) {
		switch tagErr.Code {
		case apperrors.CodeTagNotFound:
			return huma.Error404NotFound(tagErr.Message)
		case apperrors.CodeTagExists, apperrors.CodeTagAliasExists, apperrors.CodeEntityTagExists:
			return huma.Error409Conflict(tagErr.Message)
		case apperrors.CodeEntityTagNotFound:
			return huma.Error404NotFound(tagErr.Message)
		case apperrors.CodeTagCreationForbidden:
			return huma.Error403Forbidden(tagErr.Message)
		case apperrors.CodeTagNameInvalid:
			return huma.Error422UnprocessableEntity(tagErr.Message)
		case apperrors.CodeTagMergeInvalid:
			return huma.Error422UnprocessableEntity(tagErr.Message)
		case apperrors.CodeTagMergeAliasConflict:
			return huma.Error409Conflict(tagErr.Message)
		case apperrors.CodeTagHierarchyCycle:
			return huma.Error422UnprocessableEntity(tagErr.Message)
		case apperrors.CodeTagHierarchyNotGenre:
			return huma.Error422UnprocessableEntity(tagErr.Message)
		case apperrors.CodeTagBulkActionInvalid:
			return huma.Error422UnprocessableEntity(tagErr.Message)
		}
	}
	return nil
}

// MapCollectionError converts a CollectionError to an appropriate Huma HTTP
// error. Returns nil if err is not a *apperrors.CollectionError.
//
// PSY-524 convention: semantic violations of the collection domain
// (invalid request, tag-limit exceeded) map to 422 — request parsed fine;
// value rejected by domain rules. Not-found / forbidden / conflict codes
// are unchanged.
//
// PSY-358: CodeCollectionLimitExceeded maps to 403 (the request is well-
// formed, but the caller's authorization/tier does not permit it). The
// structured tier / used / limit / soft_cap_kind fields are surfaced in
// the Huma `errors[]` slot via collectionLimitDetail so the frontend can
// format messages without parsing the human string.
func MapCollectionError(err error) error {
	var collectionErr *apperrors.CollectionError
	if errors.As(err, &collectionErr) {
		switch collectionErr.Code {
		case apperrors.CodeCollectionNotFound:
			return huma.Error404NotFound(collectionErr.Message)
		case apperrors.CodeCollectionForbidden:
			return huma.Error403Forbidden(collectionErr.Message)
		case apperrors.CodeCollectionItemExists:
			return huma.Error409Conflict(collectionErr.Message)
		case apperrors.CodeCollectionItemNotFound:
			return huma.Error404NotFound(collectionErr.Message)
		case apperrors.CodeCollectionInvalidRequest:
			return huma.Error422UnprocessableEntity(collectionErr.Message)
		case apperrors.CodeCollectionTagLimitExceeded:
			return huma.Error422UnprocessableEntity(collectionErr.Message)
		case apperrors.CodeCollectionLimitExceeded:
			return huma.Error403Forbidden(
				collectionErr.Message,
				collectionLimitDetail(collectionErr),
			)
		}
	}
	return nil
}

// collectionLimitDetail builds an *huma.ErrorDetail that carries the
// structured limit fields (tier / used / limit / soft_cap_kind) under
// `errors[].value` so the frontend has direct programmatic access without
// re-parsing the human message. PSY-358.
func collectionLimitDetail(e *apperrors.CollectionError) *huma.ErrorDetail {
	return &huma.ErrorDetail{
		Message:  e.Message,
		Location: "body",
		Value: map[string]any{
			"code":          e.Code,
			"tier":          e.Tier,
			"used":          e.Used,
			"limit":         e.Limit,
			"soft_cap_kind": e.SoftCapKind,
		},
	}
}

// MapFollowError converts a FollowError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.FollowError.
//
// PSY-761: replaces the 422-for-everything behaviour of the follow handlers.
// Follow/unfollow are idempotent (no 404, no conflict) so the only mapped
// conditions are an invalid entity type (422 semantic validation) and an
// infrastructure fault (500). The handler still falls through to a generic
// 500 for any unrecognised error.
func MapFollowError(err error) error {
	var followErr *apperrors.FollowError
	if errors.As(err, &followErr) {
		switch followErr.Code {
		case apperrors.CodeFollowInvalidEntityType:
			return huma.Error422UnprocessableEntity(followErr.Message)
		case apperrors.CodeFollowInternal:
			return huma.Error500InternalServerError(followErr.Message)
		}
	}
	return nil
}

// MapAttendanceError converts an AttendanceError to an appropriate Huma HTTP
// error. Returns nil if err is not a *apperrors.AttendanceError.
//
// PSY-761: replaces the 422-for-everything behaviour of the attendance
// handlers. Show-not-found → 404; invalid status → 422 (semantic validation);
// infrastructure fault → 500. Idempotent set/clear has no conflict path.
func MapAttendanceError(err error) error {
	var attendanceErr *apperrors.AttendanceError
	if errors.As(err, &attendanceErr) {
		switch attendanceErr.Code {
		case apperrors.CodeAttendanceShowNotFound:
			return huma.Error404NotFound(attendanceErr.Message)
		case apperrors.CodeAttendanceInvalidStatus:
			return huma.Error422UnprocessableEntity(attendanceErr.Message)
		case apperrors.CodeAttendanceInternal:
			return huma.Error500InternalServerError(attendanceErr.Message)
		}
	}
	return nil
}

// MapNotificationFilterError converts a NotificationFilterError to an
// appropriate Huma HTTP error. Returns nil if err is not a
// *apperrors.NotificationFilterError.
//
// Replaces the 422-for-everything behaviour of the filter CRUD handlers.
// Filter-not-found → 404; domain-validation (no criteria, per-user cap) →
// 422; infrastructure fault → 500. The handler still falls through to a
// generic 500 for any unrecognised error.
func MapNotificationFilterError(err error) error {
	var filterErr *apperrors.NotificationFilterError
	if errors.As(err, &filterErr) {
		switch filterErr.Code {
		case apperrors.CodeFilterNotFound:
			return huma.Error404NotFound(filterErr.Message)
		case apperrors.CodeFilterValidation:
			return huma.Error422UnprocessableEntity(filterErr.Message)
		case apperrors.CodeFilterInternal:
			return huma.Error500InternalServerError(filterErr.Message)
		}
	}
	return nil
}

// MapArtistError converts an ArtistError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.ArtistError, leaving the caller free
// to fall through to a generic 500.
//
// Not-found (artist or alias) → 404; name/alias collision and delete-blocked-
// by-shows → 409 conflict; merge-into-self → 422 (semantic validation).
// HasShows is 409 here — intentionally distinct from venue HasShows (422) —
// to preserve each handler's pre-existing status contract.
func MapArtistError(err error) error {
	var artistErr *apperrors.ArtistError
	if errors.As(err, &artistErr) {
		switch artistErr.Code {
		case apperrors.CodeArtistNotFound, apperrors.CodeArtistAliasNotFound:
			return huma.Error404NotFound(artistErr.Message)
		case apperrors.CodeArtistExists, apperrors.CodeArtistAliasExists, apperrors.CodeArtistHasShows:
			return huma.Error409Conflict(artistErr.Message)
		case apperrors.CodeArtistMergeSelf:
			return huma.Error422UnprocessableEntity(artistErr.Message)
		}
	}
	return nil
}

// MapVenueError converts a VenueError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.VenueError.
//
// Not-found → 404; HasShows → 422 (the "cannot delete, associated with shows"
// status — intentionally distinct from artist HasShows, which is 409).
func MapVenueError(err error) error {
	var venueErr *apperrors.VenueError
	if errors.As(err, &venueErr) {
		switch venueErr.Code {
		case apperrors.CodeVenueNotFound:
			return huma.Error404NotFound(venueErr.Message)
		case apperrors.CodeVenueHasShows:
			return huma.Error422UnprocessableEntity(venueErr.Message)
		}
	}
	return nil
}

// MapFestivalError converts a FestivalError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.FestivalError.
//
// Festival/artist/venue not-found (and not-in-lineup / not-in-festival) all
// map to 404; an already-exists festival → 409.
func MapFestivalError(err error) error {
	var festivalErr *apperrors.FestivalError
	if errors.As(err, &festivalErr) {
		switch festivalErr.Code {
		case apperrors.CodeFestivalNotFound,
			apperrors.CodeFestivalArtistNotFound,
			apperrors.CodeFestivalArtistNotInLineup,
			apperrors.CodeFestivalVenueNotFound,
			apperrors.CodeFestivalVenueNotInFestival:
			return huma.Error404NotFound(festivalErr.Message)
		case apperrors.CodeFestivalExists:
			return huma.Error409Conflict(festivalErr.Message)
		}
	}
	return nil
}

// MapFestivalIntelligenceError converts a FestivalIntelligenceError to an
// appropriate Huma HTTP error. Returns nil if err is not a
// *apperrors.FestivalIntelligenceError.
//
// Referenced-entity / no-festivals-for-series → 404; too-few-years → 422
// (semantic validation). Each typed error carries its own message so the
// series-comparison handler's caller-supplied copy is surfaced verbatim.
func MapFestivalIntelligenceError(err error) error {
	var intelErr *apperrors.FestivalIntelligenceError
	if errors.As(err, &intelErr) {
		switch intelErr.Code {
		case apperrors.CodeFestivalIntelNotFound, apperrors.CodeFestivalIntelNoFestivals:
			return huma.Error404NotFound(intelErr.Message)
		case apperrors.CodeFestivalIntelInsufficientYears:
			return huma.Error422UnprocessableEntity(intelErr.Message)
		}
	}
	return nil
}

// MapSceneError converts a SceneError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.SceneError.
//
// Scene-not-found → 404; database faults fall through to the generic 500.
func MapSceneError(err error) error {
	var sceneErr *apperrors.SceneError
	if errors.As(err, &sceneErr) {
		switch sceneErr.Code {
		case apperrors.CodeSceneNotFound:
			return huma.Error404NotFound(sceneErr.Message)
		}
	}
	return nil
}
