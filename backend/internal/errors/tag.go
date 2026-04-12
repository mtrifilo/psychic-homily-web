package errors

import (
	"fmt"
)

// Tag error codes
const (
	CodeTagNotFound              = "TAG_NOT_FOUND"
	CodeTagExists                = "TAG_EXISTS"
	CodeTagAliasExists           = "TAG_ALIAS_EXISTS"
	CodeEntityTagExists          = "ENTITY_TAG_EXISTS"
	CodeEntityTagNotFound        = "ENTITY_TAG_NOT_FOUND"
	CodeTagCreationForbidden     = "TAG_CREATION_FORBIDDEN"
	CodeTagNameInvalid           = "TAG_NAME_INVALID"
)

// TagError represents a tag-related error with additional context.
type TagError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *TagError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *TagError) Unwrap() error {
	return e.Internal
}

// ErrTagNotFound creates a tag not found error.
func ErrTagNotFound(tagID uint) *TagError {
	return &TagError{
		Code:    CodeTagNotFound,
		Message: fmt.Sprintf("Tag %d not found", tagID),
	}
}

// ErrTagNotFoundBySlug creates a tag not found by slug error.
func ErrTagNotFoundBySlug(slug string) *TagError {
	return &TagError{
		Code:    CodeTagNotFound,
		Message: fmt.Sprintf("Tag '%s' not found", slug),
	}
}

// ErrTagExists creates a duplicate tag error.
func ErrTagExists(name string) *TagError {
	return &TagError{
		Code:    CodeTagExists,
		Message: fmt.Sprintf("Tag '%s' already exists", name),
	}
}

// ErrTagAliasExists creates a duplicate alias error.
func ErrTagAliasExists(alias string) *TagError {
	return &TagError{
		Code:    CodeTagAliasExists,
		Message: fmt.Sprintf("Alias '%s' already exists", alias),
	}
}

// ErrEntityTagExists creates a duplicate entity tag error.
func ErrEntityTagExists(tagID uint, entityType string, entityID uint) *TagError {
	return &TagError{
		Code:    CodeEntityTagExists,
		Message: fmt.Sprintf("Tag %d already applied to %s %d", tagID, entityType, entityID),
	}
}

// ErrEntityTagNotFound creates an entity tag not found error.
func ErrEntityTagNotFound(tagID uint, entityType string, entityID uint) *TagError {
	return &TagError{
		Code:    CodeEntityTagNotFound,
		Message: fmt.Sprintf("Tag %d not applied to %s %d", tagID, entityType, entityID),
	}
}

// ErrTagCreationForbidden creates a forbidden error for new users trying to create tags.
func ErrTagCreationForbidden() *TagError {
	return &TagError{
		Code:    CodeTagCreationForbidden,
		Message: "New users can only apply existing tags. Reach Contributor tier to create new tags.",
	}
}

// ErrTagNameInvalid creates a validation error for invalid tag names.
func ErrTagNameInvalid(reason string) *TagError {
	return &TagError{
		Code:    CodeTagNameInvalid,
		Message: fmt.Sprintf("Invalid tag name: %s", reason),
	}
}
