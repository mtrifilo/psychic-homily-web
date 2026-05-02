package shared

import (
	"errors"

	"github.com/danielgtaylor/huma/v2"

	apperrors "psychic-homily-backend/internal/errors"
)

// MapTagError converts a TagError to an appropriate Huma HTTP error.
// Returns nil if err is not a *apperrors.TagError, leaving the caller free
// to fall through to other error mappers.
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
			return huma.Error400BadRequest(tagErr.Message)
		case apperrors.CodeTagMergeInvalid:
			return huma.Error400BadRequest(tagErr.Message)
		case apperrors.CodeTagMergeAliasConflict:
			return huma.Error409Conflict(tagErr.Message)
		case apperrors.CodeTagHierarchyCycle:
			return huma.Error400BadRequest(tagErr.Message)
		case apperrors.CodeTagHierarchyNotGenre:
			return huma.Error400BadRequest(tagErr.Message)
		case apperrors.CodeTagBulkActionInvalid:
			return huma.Error400BadRequest(tagErr.Message)
		}
	}
	return nil
}

// MapCollectionError converts a CollectionError to an appropriate Huma HTTP
// error. Returns nil if err is not a *apperrors.CollectionError.
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
			return huma.Error400BadRequest(collectionErr.Message)
		case apperrors.CodeCollectionTagLimitExceeded:
			return huma.Error400BadRequest(collectionErr.Message)
		}
	}
	return nil
}
