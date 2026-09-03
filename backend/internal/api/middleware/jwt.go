package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/respond"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
)

type contextKey string

const UserContextKey contextKey = "user"

// JWTErrorResponse represents the error response for JWT authentication failures
type JWTErrorResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code"`
	RequestID string `json:"request_id,omitempty"`
}

// JWTMiddleware validates JWT tokens (standard http.Handler version)
func JWTMiddleware(jwtService *auth.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := logger.GetRequestID(ctx)

			logger.AuthDebug(ctx, "jwt_middleware_start",
				"path", r.URL.Path,
			)

			token, tokenSource := credentialFromRequest(r)

			if token == "" {
				logger.AuthWarn(ctx, "jwt_token_missing",
					"path", r.URL.Path,
				)
				writeJWTError(ctx, w, requestID, autherrors.CodeTokenMissing, "Authentication required", http.StatusUnauthorized)
				return
			}

			logger.AuthDebug(ctx, "jwt_token_found",
				"source", tokenSource,
			)

			// Validate token
			user, err := jwtService.ValidateToken(token)
			if err != nil {
				errorCode := autherrors.CodeTokenInvalid
				message := "Invalid token"

				var authErr *autherrors.AuthError
				if errors.As(err, &authErr) && authErr.Code == autherrors.CodeTokenExpired {
					errorCode = autherrors.CodeTokenExpired
					message = authErr.UserMessage()
				}

				logger.AuthWarn(ctx, "jwt_validation_failed",
					"error", err.Error(),
					"error_code", errorCode,
				)
				writeJWTError(ctx, w, requestID, errorCode, message, http.StatusUnauthorized)
				return
			}

			logger.AuthDebug(ctx, "jwt_validation_success",
				"user_id", user.ID,
			)

			// Add user to context
			ctx = context.WithValue(ctx, UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APITokenPrefix identifies an API-token credential. Defined as the prefix the
// token generator actually mints, so a change there cannot leave a reader
// classifying live tokens as something else.
const APITokenPrefix = adminsvc.TokenPrefix

// bearerTokenFromHeader returns the credential in a "Bearer <token>"
// Authorization header, or "" for anything else: another scheme, a different
// case, more than the two fields, or a separator that is not a single space.
//
// The accepted set is narrower than RFC 7235 allows. It is the set the
// authenticating middleware accepted before this became the shared parse, so a
// lowercase scheme or a tab separator is rejected here exactly as it always
// was, rather than newly accepted somewhere it once was not.
func bearerTokenFromHeader(authHeader string) string {
	scheme, token, ok := strings.Cut(authHeader, " ")
	if !ok || scheme != "Bearer" || token == "" || strings.ContainsRune(token, ' ') {
		return ""
	}
	return token
}

// credentialFromRequest returns the credential a request presents and where it
// came from ("header", "cookie", or "" for neither): a parseable Bearer header
// wins, otherwise the auth cookie.
//
// Every middleware that resolves a caller reads the request through this
// function or through credentialFromHumaContext, and the two agree on every
// header shape (TestCredentialReadersAgree). Two readers that disagreed would
// let a request be exempted from a rate limit as one principal while being
// authenticated as another, which is a bypass rather than a parsing detail.
//
// validatedAPIToken is the one deliberate exception: it reads only the
// Authorization header, which can only meter a caller this would have exempted,
// never the reverse. Its doc says why.
func credentialFromRequest(r *http.Request) (token, source string) {
	if t := bearerTokenFromHeader(r.Header.Get("Authorization")); t != "" {
		return t, "header"
	}
	if c, err := r.Cookie(config.AuthCookieName); err == nil && c.Value != "" {
		return c.Value, "cookie"
	}
	return "", ""
}

// credentialFromHumaContext is credentialFromRequest for a huma.Context.
//
// huma.ReadCookie searches EVERY Cookie header line, which is what net/http's
// Request.Cookie does and what this middleware previously did not: it read the
// first line only, so an auth cookie on a second line went unseen here while
// the limiter saw it. Reading every line widens what the huma-authenticated
// routes accept, to exactly what the net/http middleware next to them already
// accepted.
func credentialFromHumaContext(ctx huma.Context) (token, source string) {
	if t := bearerTokenFromHeader(ctx.Header("Authorization")); t != "" {
		return t, "header"
	}
	if c, err := huma.ReadCookie(ctx, config.AuthCookieName); err == nil && c.Value != "" {
		return c.Value, "cookie"
	}
	return "", ""
}

// HumaJWTMiddleware validates JWT tokens or API tokens (Huma middleware version)
// API tokens are identified by the "phk_" prefix and validated separately
func HumaJWTMiddleware(jwtService *auth.JWTService, sessionConfig ...config.SessionConfig) func(ctx huma.Context, next func(huma.Context)) {
	// Get session config if provided (for clearing cookies on auth failure)
	var sessConfig *config.SessionConfig
	if len(sessionConfig) > 0 {
		sessConfig = &sessionConfig[0]
	}

	// Create API token service for API token validation
	apiTokenService := adminsvc.NewAPITokenService(nil)

	return func(ctx huma.Context, next func(huma.Context)) {
		url := ctx.URL()

		// Get request ID from context (set by HumaRequestIDMiddleware)
		var requestID string
		if id, ok := ctx.Context().Value(logger.RequestIDContextKey).(string); ok {
			requestID = id
		}

		logger.AuthDebug(ctx.Context(), "huma_jwt_middleware_start",
			"path", url.Path,
		)

		token, tokenSource := credentialFromHumaContext(ctx)

		if token == "" {
			logger.AuthWarn(ctx.Context(), "huma_jwt_token_missing",
				"path", url.Path,
			)
			writeHumaJWTError(ctx, requestID, autherrors.CodeTokenMissing, "Authentication required", nil)
			return
		}

		logger.AuthDebug(ctx.Context(), "huma_jwt_token_found",
			"source", tokenSource,
		)

		var user *authm.User

		// Check if this is an API token (starts with "phk_")
		if strings.HasPrefix(token, APITokenPrefix) {
			// Validate API token
			apiUser, _, err := apiTokenService.ValidateToken(token)
			if err != nil {
				logger.AuthWarn(ctx.Context(), "huma_api_token_validation_failed",
					"error", err.Error(),
				)
				writeHumaJWTError(ctx, requestID, autherrors.CodeTokenInvalid, err.Error(), nil)
				return
			}

			user = apiUser
			logger.AuthInfo(ctx.Context(), "huma_api_token_validation_success",
				"user_id", user.ID,
			)
		} else {
			// Validate JWT token
			jwtUser, err := jwtService.ValidateToken(token)
			if err != nil {
				errorCode := autherrors.CodeTokenInvalid
				message := "Invalid token"

				var authErr *autherrors.AuthError
				if errors.As(err, &authErr) && authErr.Code == autherrors.CodeTokenExpired {
					errorCode = autherrors.CodeTokenExpired
					message = authErr.UserMessage()
				}

				logger.AuthWarn(ctx.Context(), "huma_jwt_validation_failed",
					"error", err.Error(),
					"error_code", errorCode,
				)
				// Clear the invalid cookie if we have session config
				writeHumaJWTError(ctx, requestID, errorCode, message, sessConfig)
				return
			}

			user = jwtUser
			logger.AuthInfo(ctx.Context(), "huma_jwt_validation_success",
				"user_id", user.ID,
			)
		}

		// Store user in context for handlers to access
		ctxWithUser := huma.WithValue(ctx, UserContextKey, user)

		next(ctxWithUser)
	}
}

// LenientHumaJWTMiddleware validates JWT tokens with a grace period for expired tokens.
// This allows recently-expired tokens to reach the refresh endpoint so the client can
// obtain a new token without forcing a full re-login.
func LenientHumaJWTMiddleware(jwtService *auth.JWTService, gracePeriod time.Duration) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		url := ctx.URL()

		var requestID string
		if id, ok := ctx.Context().Value(logger.RequestIDContextKey).(string); ok {
			requestID = id
		}

		logger.AuthDebug(ctx.Context(), "lenient_jwt_middleware_start",
			"path", url.Path,
		)

		token, tokenSource := credentialFromHumaContext(ctx)

		if token == "" {
			logger.AuthWarn(ctx.Context(), "lenient_jwt_token_missing",
				"path", url.Path,
			)
			writeHumaJWTError(ctx, requestID, autherrors.CodeTokenMissing, "Authentication required", nil)
			return
		}

		logger.AuthDebug(ctx.Context(), "lenient_jwt_token_found",
			"source", tokenSource,
		)

		// Validate with grace period for expired tokens
		user, err := jwtService.ValidateTokenLenient(token, gracePeriod)
		if err != nil {
			errorCode := autherrors.CodeTokenInvalid
			message := "Invalid token"

			var authErr *autherrors.AuthError
			if errors.As(err, &authErr) && authErr.Code == autherrors.CodeTokenExpired {
				errorCode = autherrors.CodeTokenExpired
				message = authErr.UserMessage()
			}

			logger.AuthWarn(ctx.Context(), "lenient_jwt_validation_failed",
				"error", err.Error(),
				"error_code", errorCode,
			)
			writeHumaJWTError(ctx, requestID, errorCode, message, nil)
			return
		}

		logger.AuthInfo(ctx.Context(), "lenient_jwt_validation_success",
			"user_id", user.ID,
		)

		ctxWithUser := huma.WithValue(ctx, UserContextKey, user)
		next(ctxWithUser)
	}
}

// OptionalHumaJWTMiddleware extracts user from JWT/API token if present,
// but allows unauthenticated requests to proceed without user context.
// Use this for endpoints that are public but behave differently for authenticated users.
func OptionalHumaJWTMiddleware(jwtService *auth.JWTService) func(ctx huma.Context, next func(huma.Context)) {
	apiTokenService := adminsvc.NewAPITokenService(nil)

	return func(ctx huma.Context, next func(huma.Context)) {
		token, _ := credentialFromHumaContext(ctx)

		// No token — continue without user context
		if token == "" {
			next(ctx)
			return
		}

		var user *authm.User

		if strings.HasPrefix(token, APITokenPrefix) {
			apiUser, _, err := apiTokenService.ValidateToken(token)
			if err != nil {
				logger.AuthDebug(ctx.Context(), "optional_auth_api_token_invalid",
					"error", err.Error(),
				)
				next(ctx)
				return
			}
			user = apiUser
		} else {
			jwtUser, err := jwtService.ValidateToken(token)
			if err != nil {
				logger.AuthDebug(ctx.Context(), "optional_auth_jwt_invalid",
					"error", err.Error(),
				)
				next(ctx)
				return
			}
			user = jwtUser
		}

		ctxWithUser := huma.WithValue(ctx, UserContextKey, user)
		next(ctxWithUser)
	}
}

// GetUserFromContext extracts user from request context
func GetUserFromContext(ctx context.Context) *authm.User {
	if user, ok := ctx.Value(UserContextKey).(*authm.User); ok {
		return user
	}
	return nil
}

// GetShowViewerFromContext reduces the request's user to the two facts a
// show-visibility gate is allowed to read (PSY-1939).
//
// Returns the zero viewer — neither an admin nor anybody's submitter — for an
// anonymous caller, which is every caller on a route NOT registered on an
// optional-auth group. A route that gates on this and forgets the middleware
// therefore under-serves rather than over-serves: submitters lose their own
// content until the group is fixed, and nobody gains anyone else's.
//
// Here beside GetUserFromContext rather than in handlers/shared because it is
// the same kind of thing — reading what the middleware planted — and because
// handlers/shared must not depend on this package: services/admin's tests
// import handlers/shared, and this package imports services/admin.
//
// Named rather than re-spelled at each handler: an inline
// `user != nil && user.IsAdmin` is the shape a later edit drops the nil check
// from, and that edit would be a nil dereference on an anonymous request.
func GetShowViewerFromContext(ctx context.Context) contracts.ShowViewer {
	user := GetUserFromContext(ctx)
	if user == nil {
		return contracts.ShowViewer{}
	}
	return contracts.ShowViewer{UserID: user.ID, IsAdmin: user.IsAdmin}
}

// writeJWTError writes a JSON error response for JWT authentication failures
func writeJWTError(ctx context.Context, w http.ResponseWriter, requestID, errorCode, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	respond.SafeEncode(ctx, w, JWTErrorResponse{
		Success:   false,
		Message:   message,
		ErrorCode: errorCode,
		RequestID: requestID,
	})
}

// writeHumaJWTError writes a JSON error response for Huma JWT authentication failures
func writeHumaJWTError(ctx huma.Context, requestID, errorCode, message string, sessConfig *config.SessionConfig) {
	ctx.SetStatus(http.StatusUnauthorized)

	// Clear the invalid cookie if session config is provided
	if sessConfig != nil {
		clearCookie := sessConfig.ClearAuthCookie()
		ctx.SetHeader("Set-Cookie", clearCookie.String())
	}

	respond.SafeEncode(ctx.Context(), ctx.BodyWriter(), JWTErrorResponse{
		Success:   false,
		Message:   message,
		ErrorCode: errorCode,
		RequestID: requestID,
	})
}
