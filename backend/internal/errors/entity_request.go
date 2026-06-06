package errors

import (
	"fmt"
)

// PSY-869: errors for the polymorphic entity_requests creation queue. These
// mirror the RequestError shape (Code + Message + Internal + Unwrap) so the
// downstream PSY-853/PSY-845 handlers can map them to HTTP status the same
// way the existing request errors are mapped. The HTTP-status mapper wiring
// is intentionally deferred to those tickets (this foundation ticket has no
// HTTP handler yet).

// Entity-request error codes.
const (
	CodeEntityRequestNotFound        = "ENTITY_REQUEST_NOT_FOUND"
	CodeEntityRequestInvalidType     = "ENTITY_REQUEST_INVALID_TYPE"
	CodeEntityRequestInvalidSource   = "ENTITY_REQUEST_INVALID_SOURCE"
	CodeEntityRequestEmptyPayload    = "ENTITY_REQUEST_EMPTY_PAYLOAD"
	CodeEntityRequestInvalidDecision = "ENTITY_REQUEST_INVALID_DECISION"
	CodeEntityRequestInvalidState    = "ENTITY_REQUEST_INVALID_STATE"
	// CodeEntityRequestFulfillUnsupported: approve cannot create the entity for
	// this entity_type because the payload lacks required associations (show ⇒
	// venues + artists; festival ⇒ series_slug). PSY-997.
	CodeEntityRequestFulfillUnsupported = "ENTITY_REQUEST_FULFILL_UNSUPPORTED"
	// CodeEntityRequestPayloadInvalid: the stored payload failed to decode into
	// its typed struct on the fulfillment path (schema drift / corruption).
	CodeEntityRequestPayloadInvalid = "ENTITY_REQUEST_PAYLOAD_INVALID"
)

// EntityRequestError represents an entity-request error with context.
type EntityRequestError struct {
	Code      string
	Message   string
	Internal  error
	RequestID uint
}

// Error implements the error interface.
func (e *EntityRequestError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *EntityRequestError) Unwrap() error {
	return e.Internal
}

// ErrEntityRequestNotFound creates a not-found error.
func ErrEntityRequestNotFound(requestID uint) *EntityRequestError {
	return &EntityRequestError{
		Code:      CodeEntityRequestNotFound,
		Message:   fmt.Sprintf("Entity request %d not found", requestID),
		RequestID: requestID,
	}
}

// ErrEntityRequestInvalidType creates an unknown-entity-type error.
func ErrEntityRequestInvalidType(entityType string) *EntityRequestError {
	return &EntityRequestError{
		Code:    CodeEntityRequestInvalidType,
		Message: fmt.Sprintf("Unsupported entity request type: %q", entityType),
	}
}

// ErrEntityRequestInvalidSource creates an unknown-source-context error.
func ErrEntityRequestInvalidSource(sourceContext string) *EntityRequestError {
	return &EntityRequestError{
		Code:    CodeEntityRequestInvalidSource,
		Message: fmt.Sprintf("Unsupported source context: %q", sourceContext),
	}
}

// ErrEntityRequestEmptyPayload creates an empty-payload error.
func ErrEntityRequestEmptyPayload(entityType string) *EntityRequestError {
	return &EntityRequestError{
		Code:    CodeEntityRequestEmptyPayload,
		Message: fmt.Sprintf("Empty payload for %s entity request", entityType),
	}
}

// ErrEntityRequestInvalidDecision creates an error for a decision target that
// is not 'approved' or 'rejected'.
func ErrEntityRequestInvalidDecision(state string) *EntityRequestError {
	return &EntityRequestError{
		Code:    CodeEntityRequestInvalidDecision,
		Message: fmt.Sprintf("Invalid decision state: %q (expected approved or rejected)", state),
	}
}

// ErrEntityRequestInvalidState creates an error when deciding a request that is
// not pending.
func ErrEntityRequestInvalidState(requestID uint, currentState string) *EntityRequestError {
	return &EntityRequestError{
		Code:      CodeEntityRequestInvalidState,
		Message:   fmt.Sprintf("Entity request %d is %s, expected pending", requestID, currentState),
		RequestID: requestID,
	}
}

// ErrEntityRequestFulfillUnsupported creates an error when an approved request's
// entity_type cannot be auto-created from its payload (show / festival). PSY-997.
func ErrEntityRequestFulfillUnsupported(entityType string) *EntityRequestError {
	return &EntityRequestError{
		Code:    CodeEntityRequestFulfillUnsupported,
		Message: fmt.Sprintf("Approving a %s request cannot auto-create the entity (its payload lacks required associations); create it manually", entityType),
	}
}

// ErrEntityRequestPayloadInvalid creates an error when a stored payload fails to
// decode into its typed struct on the fulfillment path. PSY-997.
func ErrEntityRequestPayloadInvalid(entityType string, internal error) *EntityRequestError {
	return &EntityRequestError{
		Code:     CodeEntityRequestPayloadInvalid,
		Message:  fmt.Sprintf("Stored %s payload is invalid and cannot be fulfilled", entityType),
		Internal: internal,
	}
}
