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
		}
	}
	return nil
}
