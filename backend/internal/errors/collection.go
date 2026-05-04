package errors

import (
	"fmt"
)

// Collection error codes
const (
	CodeCollectionNotFound       = "COLLECTION_NOT_FOUND"
	CodeCollectionForbidden      = "COLLECTION_FORBIDDEN"
	CodeCollectionItemExists     = "COLLECTION_ITEM_EXISTS"
	CodeCollectionItemNotFound   = "COLLECTION_ITEM_NOT_FOUND"
	CodeCollectionInvalidRequest = "COLLECTION_INVALID_REQUEST"
	// CodeCollectionTagLimitExceeded is returned when a curator tries to add
	// an 11th tag to a collection (PSY-354). Maps to HTTP 422 (PSY-524).
	CodeCollectionTagLimitExceeded = "COLLECTION_TAG_LIMIT_EXCEEDED"
	// CodeCollectionLimitExceeded is returned when a user tries to create a
	// new collection beyond their per-tier cap or to fork beyond the soft
	// fork cap (PSY-358). Maps to HTTP 403 — the request is well-formed,
	// but the caller's authorization (their tier) does not permit it.
	CodeCollectionLimitExceeded = "COLLECTION_LIMIT_EXCEEDED"
)

// CollectionLimitKind identifies which cap the caller hit. Surfaced verbatim
// in the structured error so the frontend can format messages per kind.
const (
	CollectionLimitKindOwned = "owned"
	CollectionLimitKindFork  = "fork"
)

// CollectionError represents a collection-related error with additional context.
//
// Limit fields (Tier, Used, Limit, SoftCapKind) are populated by
// ErrCollectionLimitExceeded and are zero-valued for every other code; the
// frontend reads them off the structured error body to render
// "X of Y collections (tier — link to /help/tiers)" copy without
// re-parsing the human message.
type CollectionError struct {
	Code         string
	Message      string
	Internal     error
	CollectionID uint
	// PSY-358: structured fields for the create/fork-limit error. Populated
	// only when Code == CodeCollectionLimitExceeded; left at zero values for
	// other codes.
	Tier        string
	Used        int
	Limit       int
	SoftCapKind string
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

// ErrCollectionInvalidRequest creates an invalid-request error for the
// collection domain (bad enum value, malformed input, etc.). The message is
// surfaced verbatim to the API caller as a 422 (PSY-524).
func ErrCollectionInvalidRequest(message string) *CollectionError {
	return &CollectionError{
		Code:    CodeCollectionInvalidRequest,
		Message: message,
	}
}

// ErrCollectionTagLimitExceeded creates an error for the collection-tag cap
// (PSY-354). Surfaced verbatim to the caller as a 422 (PSY-524) so the
// curator UI can show the cap and current count.
func ErrCollectionTagLimitExceeded(currentCount, maxAllowed int) *CollectionError {
	return &CollectionError{
		Code: CodeCollectionTagLimitExceeded,
		Message: fmt.Sprintf(
			"Collections can have at most %d tags (currently %d). Remove a tag before adding another.",
			maxAllowed, currentCount,
		),
	}
}

// ErrCollectionLimitExceeded creates an error for the per-tier owned-
// collection cap or the soft fork cap (PSY-358). Maps to HTTP 403. The
// structured fields (Tier/Used/Limit/SoftCapKind) are echoed back in the
// Huma error body's `errors[].value` so the frontend can render targeted
// copy without parsing the human message.
//
// kind must be one of CollectionLimitKindOwned / CollectionLimitKindFork.
// The human message and remediation tip differ slightly per kind — fork
// callers see "fork", owned callers see "collection".
func ErrCollectionLimitExceeded(tier string, used, limit int, kind string) *CollectionError {
	var message string
	switch kind {
	case CollectionLimitKindFork:
		message = fmt.Sprintf(
			"Fork limit reached: %d of %d forks. Delete a fork before creating another.",
			used, limit,
		)
	default:
		// CollectionLimitKindOwned — also the safe default if a caller
		// passes an unknown kind, since "collection" copy still reads
		// correctly for any owned-style cap.
		message = fmt.Sprintf(
			"Collection limit reached for your tier (%s): %d of %d collections. See /help/tiers to learn how to advance.",
			tier, used, limit,
		)
	}
	return &CollectionError{
		Code:        CodeCollectionLimitExceeded,
		Message:     message,
		Tier:        tier,
		Used:        used,
		Limit:       limit,
		SoftCapKind: kind,
	}
}
