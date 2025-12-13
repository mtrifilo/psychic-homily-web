// Package errors provides custom error types for the application.
package errors

import (
	"fmt"
)

// Auth error codes
const (
	// CodeInvalidCredentials indicates wrong email or password
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	// CodeUserNotFound indicates email doesn't exist (internal use, maps to INVALID_CREDENTIALS externally)
	CodeUserNotFound = "USER_NOT_FOUND"
	// CodeTokenExpired indicates the JWT token has expired
	CodeTokenExpired = "TOKEN_EXPIRED"
	// CodeTokenInvalid indicates the JWT is malformed or has an invalid signature
	CodeTokenInvalid = "TOKEN_INVALID"
	// CodeTokenMissing indicates no token was provided
	CodeTokenMissing = "TOKEN_MISSING"
	// CodeServiceUnavailable indicates the database or a service is down
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	// CodeUserExists indicates an email is already registered
	CodeUserExists = "USER_EXISTS"
	// CodeValidationFailed indicates request validation failed
	CodeValidationFailed = "VALIDATION_FAILED"
	// CodeUnauthorized indicates the user is not authorized for the action
	CodeUnauthorized = "UNAUTHORIZED"
	// CodeUnknown indicates an unknown error occurred
	CodeUnknown = "UNKNOWN"
)

// AuthError represents an authentication-related error with additional context.
type AuthError struct {
	Code      string // Error code (e.g., "INVALID_CREDENTIALS")
	Message   string // User-facing message
	Internal  error  // Original error (logged, not exposed to client)
	RequestID string // Request ID for correlation
}

// Error implements the error interface.
func (e *AuthError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *AuthError) Unwrap() error {
	return e.Internal
}

// UserMessage returns the user-safe message (without internal details).
func (e *AuthError) UserMessage() string {
	return e.Message
}

// WithRequestID returns a copy of the error with the request ID set.
func (e *AuthError) WithRequestID(requestID string) *AuthError {
	return &AuthError{
		Code:      e.Code,
		Message:   e.Message,
		Internal:  e.Internal,
		RequestID: requestID,
	}
}

// NewAuthError creates a new AuthError with the given parameters.
func NewAuthError(code, message string, internal error) *AuthError {
	return &AuthError{
		Code:     code,
		Message:  message,
		Internal: internal,
	}
}

// Predefined error constructors for common auth errors

// ErrInvalidCredentials creates an invalid credentials error.
func ErrInvalidCredentials(internal error) *AuthError {
	return NewAuthError(CodeInvalidCredentials, "Invalid email or password", internal)
}

// ErrUserNotFound creates a user not found error.
// Note: For security, this should be mapped to INVALID_CREDENTIALS in responses.
func ErrUserNotFound(email string) *AuthError {
	return NewAuthError(CodeUserNotFound, "User not found", fmt.Errorf("no user with email: %s", email))
}

// ErrTokenExpired creates a token expired error.
func ErrTokenExpired(internal error) *AuthError {
	return NewAuthError(CodeTokenExpired, "Your session has expired. Please log in again.", internal)
}

// ErrTokenInvalid creates a token invalid error.
func ErrTokenInvalid(internal error) *AuthError {
	return NewAuthError(CodeTokenInvalid, "Invalid authentication token", internal)
}

// ErrTokenMissing creates a token missing error.
func ErrTokenMissing() *AuthError {
	return NewAuthError(CodeTokenMissing, "Authentication required", nil)
}

// ErrServiceUnavailable creates a service unavailable error.
func ErrServiceUnavailable(service string, internal error) *AuthError {
	return NewAuthError(CodeServiceUnavailable, "Service temporarily unavailable", fmt.Errorf("%s: %w", service, internal))
}

// ErrUserExists creates a user already exists error.
func ErrUserExists(email string) *AuthError {
	return NewAuthError(CodeUserExists, "An account with this email already exists", fmt.Errorf("duplicate email: %s", email))
}

// ErrValidationFailed creates a validation error.
func ErrValidationFailed(message string) *AuthError {
	return NewAuthError(CodeValidationFailed, message, nil)
}

// ToExternalCode converts internal error codes to external (safe) codes.
// This prevents leaking information like whether an email exists.
func ToExternalCode(code string) string {
	switch code {
	case CodeUserNotFound:
		return CodeInvalidCredentials
	default:
		return code
	}
}

// ToExternalMessage returns a safe user-facing message for the error code.
func ToExternalMessage(code string) string {
	switch code {
	case CodeInvalidCredentials, CodeUserNotFound:
		return "Invalid email or password"
	case CodeTokenExpired:
		return "Your session has expired. Please log in again."
	case CodeTokenInvalid:
		return "Invalid authentication token"
	case CodeTokenMissing:
		return "Authentication required"
	case CodeServiceUnavailable:
		return "Service temporarily unavailable. Please try again."
	case CodeUserExists:
		return "An account with this email already exists"
	case CodeValidationFailed:
		return "Validation failed"
	default:
		return "An error occurred"
	}
}
