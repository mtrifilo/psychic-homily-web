package errors

import (
	"fmt"
)

// Collection error codes
const (
	CodeCollectionNotFound     = "COLLECTION_NOT_FOUND"
	CodeCollectionForbidden    = "COLLECTION_FORBIDDEN"
	CodeCollectionItemExists   = "COLLECTION_ITEM_EXISTS"
	CodeCollectionItemNotFound = "COLLECTION_ITEM_NOT_FOUND"
)

// CollectionError represents a collection-related error with additional context.
type CollectionError struct {
	Code         string
	Message      string
	Internal     error
	CollectionID uint
}

// Error implements the error interface.
func (e *CollectionError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *CollectionError) Unwrap() error {
	return e.Internal
}

// ErrCollectionNotFound creates a collection not found error.
func ErrCollectionNotFound(slug string) *CollectionError {
	return &CollectionError{
		Code:    CodeCollectionNotFound,
		Message: fmt.Sprintf("Collection '%s' not found", slug),
	}
}

// ErrCollectionForbidden creates a collection forbidden error.
func ErrCollectionForbidden(slug string) *CollectionError {
	return &CollectionError{
		Code:    CodeCollectionForbidden,
		Message: fmt.Sprintf("Access denied for collection '%s'", slug),
	}
}

// ErrCollectionItemExists creates a duplicate collection item error.
func ErrCollectionItemExists(collectionID uint, entityType string, entityID uint) *CollectionError {
	return &CollectionError{
		Code:         CodeCollectionItemExists,
		Message:      fmt.Sprintf("Item %s:%d already exists in collection", entityType, entityID),
		CollectionID: collectionID,
	}
}

// ErrCollectionItemNotFound creates a collection item not found error.
func ErrCollectionItemNotFound(itemID uint) *CollectionError {
	return &CollectionError{
		Code:    CodeCollectionItemNotFound,
		Message: fmt.Sprintf("Collection item %d not found", itemID),
	}
}
