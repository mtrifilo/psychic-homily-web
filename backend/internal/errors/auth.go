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
	// CodeAccountLocked indicates the account is locked due to too many failed attempts
	CodeAccountLocked = "ACCOUNT_LOCKED"
	// CodeAccountInactive indicates the account exists but has been deactivated
	// (is_active = false). Distinct from CodeInvalidCredentials so the frontend
	// can special-case the copy; the password check still ran, so this is not an
	// enumeration oracle beyond what login already exposes.
	CodeAccountInactive = "ACCOUNT_INACTIVE"
	// CodeNoPasswordSet indicates the user has no password (OAuth-only account)
	CodeNoPasswordSet = "NO_PASSWORD_SET"
	// CodeTermsAcceptanceRequired indicates an OAuth signup arrived without the
	// required Terms of Service / Privacy Policy consent.
	CodeTermsAcceptanceRequired = "TERMS_ACCEPTANCE_REQUIRED"
	// CodeAgeConfirmationRequired indicates a signup arrived without the required
	// minimum-age confirmation (PSY-1023).
	CodeAgeConfirmationRequired = "AGE_CONFIRMATION_REQUIRED"
	// CodeInvalidReplyPermission indicates an unrecognized default-reply-permission value.
	CodeInvalidReplyPermission = "INVALID_REPLY_PERMISSION"
	// CodeUsernameTaken indicates a username unique-constraint violation on profile update.
	CodeUsernameTaken = "USERNAME_TAKEN"
)

// AuthError represents an authentication-related error with additional context.
type AuthError struct {
	Code      string // Error code (e.g., "INVALID_CREDENTIALS")
	Message   string // User-facing message
	Internal  error  // Original error (logged, not exposed to client)
	RequestID string // Request ID for correlation
	Minutes   int    // Lock duration in minutes (used with CodeAccountLocked)
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
		Minutes:   e.Minutes,
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

// ErrUserNotFoundByID creates a user-not-found error for the by-ID lookup
// case (session refresh after the principal was hard- or soft-deleted).
//
// Distinct from ErrUserNotFound(email) because the by-ID lookup does NOT have
// the enumeration-safety concern that maps CodeUserNotFound to
// CodeInvalidCredentials externally — the user already proved possession of
// a token signed for this userID, so there is no email-enumeration oracle to
// protect. Routes that need "this session principal is gone" semantics
// (RefreshTokenHandler, eventually GetProfileHandler) inspect the typed
// AuthError and emit HTTP 401 + CodeUnauthorized so the client clears the
// session and redirects to login rather than retrying against a 5xx.
func ErrUserNotFoundByID(userID uint, internal error) *AuthError {
	return NewAuthError(CodeUserNotFound, "User not found", fmt.Errorf("no user with id %d: %w", userID, internal))
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

// ErrAccountLocked creates an account locked error with the unlock time.
func ErrAccountLocked(message string) *AuthError {
	return NewAuthError(CodeAccountLocked, message, nil)
}

// ErrAccountLockedWithMinutes creates an account locked error with the remaining lock duration.
func ErrAccountLockedWithMinutes(minutes int) *AuthError {
	return &AuthError{
		Code:    CodeAccountLocked,
		Message: fmt.Sprintf("Account temporarily locked due to too many failed login attempts. Please try again in %d minute(s).", minutes),
		Minutes: minutes,
	}
}

// ErrAccountInactive creates an error for a deactivated account (is_active = false).
// The user-facing message is intentionally vague ("Account unavailable. Please
// contact support.") rather than naming deactivation: the password check has
// already run, so this carries no enumeration signal beyond what login leaks,
// but the vaguer copy avoids confirming the deactivation reason to a guesser.
func ErrAccountInactive() *AuthError {
	return &AuthError{
		Code:    CodeAccountInactive,
		Message: "Account unavailable. Please contact support.",
	}
}

// ErrNoPasswordSet creates an error for OAuth-only accounts that don't have a password.
func ErrNoPasswordSet() *AuthError {
	return NewAuthError(CodeNoPasswordSet, "Cannot change password for OAuth-only accounts", nil)
}

// ErrTermsAcceptanceRequired creates an OAuth-signup terms-not-accepted error.
// The detail distinguishes "no consent / not accepted" from "version missing"
// so logs keep the cause without the handler string-matching the message.
func ErrTermsAcceptanceRequired(detail string) *AuthError {
	return NewAuthError(CodeTermsAcceptanceRequired, "Please accept the Terms of Service and Privacy Policy before creating an account.", fmt.Errorf("%s", detail))
}

// ErrAgeConfirmationRequired creates a signup age-confirmation-missing error.
// The detail distinguishes "not confirmed" from "attested age below minimum"
// so logs keep the cause without the handler string-matching the message.
// NOTE: the "16" in the user-facing copy is kept in sync by hand with
// auth-handler MinSignupAge / user-service minOAuthSignupAge; if the minimum
// changes, update this string too (the threshold is enforced by those
// constants, not parsed from this message).
func ErrAgeConfirmationRequired(detail string) *AuthError {
	return NewAuthError(CodeAgeConfirmationRequired, "You must confirm that you are at least 16 years old to create an account.", fmt.Errorf("%s", detail))
}

// ErrInvalidReplyPermission creates an invalid default-reply-permission error.
func ErrInvalidReplyPermission(permission string) *AuthError {
	return NewAuthError(CodeInvalidReplyPermission, "Invalid reply permission", fmt.Errorf("invalid reply_permission: %s", permission))
}

// ErrUsernameTaken creates a username unique-constraint-violation error.
func ErrUsernameTaken(internal error) *AuthError {
	return NewAuthError(CodeUsernameTaken, "Username is already taken", internal)
}

// ToExternalCode converts internal error codes to external (safe) codes.
// This prevents leaking information like whether an email exists.
func ToExternalCode(code string) string {
	switch code {
	case CodeUserNotFound:
		return CodeInvalidCredentials
	case CodeAccountInactive:
		// Deactivated accounts are NOT enumeration-sensitive: the password
		// check already ran, so login already leaks existence on a correct
		// password. Keep the code distinct so the frontend can special-case it.
		return CodeAccountInactive
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
	case CodeAccountLocked:
		return "Account temporarily locked. Please try again later."
	case CodeAccountInactive:
		return "Account unavailable. Please contact support."
	case CodeNoPasswordSet:
		return "Cannot change password for OAuth-only accounts"
	default:
		return "An error occurred"
	}
}
