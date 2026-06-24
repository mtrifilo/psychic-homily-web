package shared

import (
	"errors"
	"net/http"

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

// MapLabelError converts a LabelError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.LabelError. not-found → 404;
// already-exists → 409. Mirrors MapArtistError/MapVenueError; used by the
// entity-request fulfillment path so a duplicate label surfaces as a 409.
func MapLabelError(err error) error {
	var labelErr *apperrors.LabelError
	if errors.As(err, &labelErr) {
		switch labelErr.Code {
		case apperrors.CodeLabelNotFound:
			return huma.Error404NotFound(labelErr.Message)
		case apperrors.CodeLabelExists:
			return huma.Error409Conflict(labelErr.Message)
		}
	}
	return nil
}

// MapReleaseError converts a ReleaseError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.ReleaseError. not-found → 404;
// already-exists → 409. Used by the entity-request fulfillment path so a
// duplicate release surfaces as a 409.
func MapReleaseError(err error) error {
	var releaseErr *apperrors.ReleaseError
	if errors.As(err, &releaseErr) {
		switch releaseErr.Code {
		case apperrors.CodeReleaseNotFound:
			return huma.Error404NotFound(releaseErr.Message)
		case apperrors.CodeReleaseExists:
			return huma.Error409Conflict(releaseErr.Message)
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

// MapShowError converts a ShowError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.ShowError.
//
// Create/validation failures map to 422, matching the direct show-create
// handler's contract (a duplicate headliner at the same venue/date surfaces
// as SHOW_CREATE_FAILED → 422 there too); not-found maps to 404. The other
// show codes (update/delete/unauthorized/invalid-id) are intentionally
// unmapped — this mapper serves the entity_request fulfillment path, which
// only creates.
func MapShowError(err error) error {
	var showErr *apperrors.ShowError
	if errors.As(err, &showErr) {
		switch showErr.Code {
		case apperrors.CodeShowNotFound:
			return huma.Error404NotFound(showErr.Message)
		case apperrors.CodeShowCreateFailed, apperrors.CodeShowValidationFailed:
			return huma.Error422UnprocessableEntity(showErr.Message)
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

// MapEntityReportError converts an EntityReportError to an appropriate Huma
// HTTP error. Returns nil if err is not a *apperrors.EntityReportError.
//
// Reported-entity-not-found / report-not-found → 404; duplicate-pending /
// already-reviewed → 409; invalid entity/report type → 422 (semantic
// validation); infrastructure fault → 500.
func MapEntityReportError(err error) error {
	var reportErr *apperrors.EntityReportError
	if errors.As(err, &reportErr) {
		switch reportErr.Code {
		case apperrors.CodeEntityReportEntityNotFound, apperrors.CodeEntityReportNotFound:
			return huma.Error404NotFound(reportErr.Message)
		case apperrors.CodeEntityReportDuplicatePending, apperrors.CodeEntityReportAlreadyReviewed:
			return huma.Error409Conflict(reportErr.Message)
		case apperrors.CodeEntityReportInvalidEntityType, apperrors.CodeEntityReportInvalidReportType:
			return huma.Error422UnprocessableEntity(reportErr.Message)
		case apperrors.CodeEntityReportInternal:
			return huma.Error500InternalServerError(reportErr.Message)
		}
	}
	return nil
}

// MapEntityRequestError converts an EntityRequestError to an appropriate Huma
// HTTP error. Returns nil if err is not a *apperrors.EntityRequestError so the
// caller can fall through to a 500. PSY-997.
//
// not-found → 404; already-decided (invalid state for a decision) → 409;
// invalid type / source / decision / empty payload (semantic validation) → 422.
func MapEntityRequestError(err error) error {
	var reqErr *apperrors.EntityRequestError
	if errors.As(err, &reqErr) {
		switch reqErr.Code {
		case apperrors.CodeEntityRequestNotFound:
			return huma.Error404NotFound(reqErr.Message)
		case apperrors.CodeEntityRequestInvalidState,
			apperrors.CodeEntityRequestNotRescuable:
			return huma.Error409Conflict(reqErr.Message)
		case apperrors.CodeEntityRequestInvalidType,
			apperrors.CodeEntityRequestInvalidSource,
			apperrors.CodeEntityRequestEmptyPayload,
			apperrors.CodeEntityRequestInvalidDecision,
			apperrors.CodeEntityRequestFulfillUnsupported:
			return huma.Error422UnprocessableEntity(reqErr.Message)
		case apperrors.CodeEntityRequestPayloadInvalid:
			// Stored payload corruption — server-side data fault, not the
			// admin's input.
			return huma.Error500InternalServerError(reqErr.Message)
		}
	}
	return nil
}

// MapLeaderboardError converts a LeaderboardError to an appropriate Huma HTTP
// error. Returns nil if err is not a *apperrors.LeaderboardError.
//
// Invalid dimension / period → 422 (semantic validation); infra fault → 500.
// There is no not-found path (an empty board is a 200).
func MapLeaderboardError(err error) error {
	var lbErr *apperrors.LeaderboardError
	if errors.As(err, &lbErr) {
		switch lbErr.Code {
		case apperrors.CodeLeaderboardInvalidDimension, apperrors.CodeLeaderboardInvalidPeriod:
			return huma.Error422UnprocessableEntity(lbErr.Message)
		case apperrors.CodeLeaderboardInternal:
			return huma.Error500InternalServerError(lbErr.Message)
		}
	}
	return nil
}

// MapDataQualityError converts a DataQualityError to an appropriate Huma HTTP
// error. Returns nil if err is not a *apperrors.DataQualityError.
//
// Unknown category → 422 (semantic validation); infra fault → 500. Shared by
// the admin data-quality dashboard and the public contribute surface, which
// both consume DataQualityService.
func MapDataQualityError(err error) error {
	var dqErr *apperrors.DataQualityError
	if errors.As(err, &dqErr) {
		switch dqErr.Code {
		case apperrors.CodeDataQualityUnknownCategory:
			return huma.Error422UnprocessableEntity(dqErr.Message)
		case apperrors.CodeDataQualityInternal:
			return huma.Error500InternalServerError(dqErr.Message)
		}
	}
	return nil
}

// MapAutoPromotionError converts an AutoPromotionError to an appropriate Huma
// HTTP error. Returns nil if err is not a *apperrors.AutoPromotionError.
//
// User-not-found → 404; infra fault → 500.
func MapAutoPromotionError(err error) error {
	var apErr *apperrors.AutoPromotionError
	if errors.As(err, &apErr) {
		switch apErr.Code {
		case apperrors.CodeAutoPromotionUserNotFound:
			return huma.Error404NotFound(apErr.Message)
		case apperrors.CodeAutoPromotionInternal:
			return huma.Error500InternalServerError(apErr.Message)
		}
	}
	return nil
}

// MapProfileError converts a ProfileError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.ProfileError.
//
// Section-not-found → 404; section-validation → 422 (the message is
// user-facing and surfaced verbatim); infra fault → 500.
func MapProfileError(err error) error {
	var profileErr *apperrors.ProfileError
	if errors.As(err, &profileErr) {
		switch profileErr.Code {
		case apperrors.CodeProfileSectionNotFound:
			return huma.Error404NotFound(profileErr.Message)
		case apperrors.CodeProfileSectionInvalid:
			return huma.Error422UnprocessableEntity(profileErr.Message)
		case apperrors.CodeProfileInternal:
			return huma.Error500InternalServerError(profileErr.Message)
		}
	}
	return nil
}

// MapCommentError converts a CommentError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.CommentError.
//
// Status semantics:
//   - NotFound / ParentNotFound / EntityNotFound      → 404
//   - Forbidden (author-only, edit window, replies-
//     disabled, followers-only-denied)                → 403
//   - InvalidEntityType / InvalidReplyPermission /
//     BodyRequired / BodyTooLong / MaxDepthExceeded /
//     ParentMismatch / NotThreadRoot / FieldValidation → 400
//   - RateLimitedEntity / RateLimitedHourly           → 429 with Retry-After
//     (60 / 3600 respectively, RFC 7231 §7.1.3)
//   - Internal                                         → 500
//
// 429 responses MUST carry a Retry-After header so the inline comment /
// reply / field-note rate-limit banners can populate countdown copy
// without parsing the body.
func MapCommentError(err error) error {
	var commentErr *apperrors.CommentError
	if errors.As(err, &commentErr) {
		switch commentErr.Code {
		case apperrors.CodeCommentNotFound,
			apperrors.CodeCommentParentNotFound,
			apperrors.CodeCommentEntityNotFound:
			return huma.Error404NotFound(commentErr.Message)
		case apperrors.CodeCommentForbidden:
			return huma.Error403Forbidden(commentErr.Message)
		case apperrors.CodeCommentInvalidEntityType,
			apperrors.CodeCommentInvalidReplyPermission,
			apperrors.CodeCommentBodyRequired,
			apperrors.CodeCommentBodyTooLong,
			apperrors.CodeCommentMaxDepthExceeded,
			apperrors.CodeCommentParentMismatch,
			apperrors.CodeCommentNotThreadRoot,
			apperrors.CodeCommentFieldValidation:
			return huma.Error400BadRequest(commentErr.Message)
		case apperrors.CodeCommentRateLimitedEntity:
			return huma.ErrorWithHeaders(
				huma.Error429TooManyRequests(commentErr.Message),
				http.Header{"Retry-After": []string{"60"}},
			)
		case apperrors.CodeCommentRateLimitedHourly:
			return huma.ErrorWithHeaders(
				huma.Error429TooManyRequests(commentErr.Message),
				http.Header{"Retry-After": []string{"3600"}},
			)
		case apperrors.CodeCommentInternal:
			return huma.Error500InternalServerError(commentErr.Message)
		}
	}
	return nil
}

// MapCommentAdminError converts a CommentAdminError to an appropriate Huma
// HTTP error. Returns nil if err is not a *apperrors.CommentAdminError.
//
// Comment-admin paths (Hide / Restore / Approve / Reject / edit-history)
// discriminate state-transition failures here. The comment-not-found path
// stays on CommentError since the underlying row lookup is the same.
//
//   - AlreadyVisible    → 409 (Restore on already-visible comment)
//   - NotPending        → 409 (Approve / Reject on non-pending comment)
//   - AccessRequired    → 403 (non-admin requesting edit history)
func MapCommentAdminError(err error) error {
	var adminErr *apperrors.CommentAdminError
	if errors.As(err, &adminErr) {
		switch adminErr.Code {
		case apperrors.CodeCommentAdminAlreadyVisible,
			apperrors.CodeCommentAdminNotPending:
			return huma.Error409Conflict(adminErr.Message)
		case apperrors.CodeCommentAdminAccessRequired:
			return huma.Error403Forbidden(adminErr.Message)
		}
	}
	return nil
}

// MapFieldNoteError converts a FieldNoteError to an appropriate Huma HTTP
// error. Returns nil if err is not a *apperrors.FieldNoteError.
//
// Covers the show / past-show / artist-on-bill gates unique to field-note
// creation. Body / sound_quality / crowd_energy / rate-limit codes stay on
// CommentError and flow through MapCommentError.
//
//   - ShowNotFound     → 404
//   - ShowFuture       → 400 (semantic 400, NOT 422, to match the
//     pre-typed-error client contract)
//   - ArtistNotOnBill  → 400 (ditto)
func MapFieldNoteError(err error) error {
	var fnErr *apperrors.FieldNoteError
	if errors.As(err, &fnErr) {
		switch fnErr.Code {
		case apperrors.CodeFieldNoteShowNotFound:
			return huma.Error404NotFound(fnErr.Message)
		case apperrors.CodeFieldNoteShowFuture,
			apperrors.CodeFieldNoteArtistNotOnBill:
			return huma.Error400BadRequest(fnErr.Message)
		}
	}
	return nil
}

// MapCommentVoteError converts a CommentVoteError to an appropriate Huma HTTP
// error. Returns nil if err is not a *apperrors.CommentVoteError.
//
// Self-vote rejection (HN/Lobsters convention) stays 403 — the caller is
// authenticated but cannot perform this specific action on their own
// content. Comment-not-found is 404. InvalidDirection is 400.
func MapCommentVoteError(err error) error {
	var voteErr *apperrors.CommentVoteError
	if errors.As(err, &voteErr) {
		switch voteErr.Code {
		case apperrors.CodeCommentVoteCommentNotFound:
			return huma.Error404NotFound(voteErr.Message)
		case apperrors.CodeCommentVoteSelfVote:
			return huma.Error403Forbidden(voteErr.Message)
		case apperrors.CodeCommentVoteInvalidDirection:
			return huma.Error400BadRequest(voteErr.Message)
		case apperrors.CodeCommentVoteInternal:
			return huma.Error500InternalServerError(voteErr.Message)
		}
	}
	return nil
}

// MapPendingEditError converts a PendingEditError to an appropriate Huma HTTP
// error. Returns nil if err is not a *apperrors.PendingEditError.
//
// Entity-not-found (create) → 404; entity-gone (approve) → 422; edit-not-found
// → 404; not-pending / duplicate → 409; not-submitter → 403 (cancel only);
// invalid entity type / malformed request → 422; infra fault → 500.
//
// The disallowed-fields auto-rejection is a separate sentinel
// (adminm.ErrPendingEditDisallowedFields → 400) handled by the approve
// handler before this mapper runs.
func MapPendingEditError(err error) error {
	var editErr *apperrors.PendingEditError
	if errors.As(err, &editErr) {
		switch editErr.Code {
		case apperrors.CodePendingEditEntityNotFound, apperrors.CodePendingEditNotFound:
			return huma.Error404NotFound(editErr.Message)
		case apperrors.CodePendingEditNotPending, apperrors.CodePendingEditDuplicate:
			return huma.Error409Conflict(editErr.Message)
		case apperrors.CodePendingEditNotSubmitter:
			return huma.Error403Forbidden(editErr.Message)
		case apperrors.CodePendingEditEntityGone,
			apperrors.CodePendingEditInvalidEntityType,
			apperrors.CodePendingEditInvalidRequest:
			return huma.Error422UnprocessableEntity(editErr.Message)
		case apperrors.CodePendingEditInternal:
			return huma.Error500InternalServerError(editErr.Message)
		}
	}
	return nil
}
