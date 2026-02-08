package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
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
func JWTMiddleware(jwtService *services.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := logger.GetRequestID(ctx)

			logger.AuthDebug(ctx, "jwt_middleware_start",
				"path", r.URL.Path,
			)

			var token string
			var tokenSource string

			// First, try to get token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				// Extract token from "Bearer <token>"
				tokenParts := strings.Split(authHeader, " ")
				if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
					token = tokenParts[1]
					tokenSource = "header"
				}
			}

			// If no token in header, try to get from HTTP-only cookie
			if token == "" {
				cookie, err := r.Cookie("auth_token")
				if err == nil && cookie.Value != "" {
					token = cookie.Value
					tokenSource = "cookie"
				}
			}

			if token == "" {
				logger.AuthWarn(ctx, "jwt_token_missing",
					"path", r.URL.Path,
				)
				writeJWTError(w, requestID, autherrors.CodeTokenMissing, "Authentication required", http.StatusUnauthorized)
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
				writeJWTError(w, requestID, errorCode, message, http.StatusUnauthorized)
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

// APITokenPrefix is the prefix for API tokens (used to identify token type)
const APITokenPrefix = "phk_"

// HumaJWTMiddleware validates JWT tokens or API tokens (Huma middleware version)
// API tokens are identified by the "phk_" prefix and validated separately
func HumaJWTMiddleware(jwtService *services.JWTService, sessionConfig ...config.SessionConfig) func(ctx huma.Context, next func(huma.Context)) {
	// Get session config if provided (for clearing cookies on auth failure)
	var sessConfig *config.SessionConfig
	if len(sessionConfig) > 0 {
		sessConfig = &sessionConfig[0]
	}

	// Create API token service for API token validation
	apiTokenService := services.NewAPITokenService(nil)

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

		var token string
		var tokenSource string

		// First, try to get token from Authorization header
		authHeader := ctx.Header("Authorization")
		if authHeader != "" {
			// Extract token from "Bearer <token>"
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
				token = tokenParts[1]
				tokenSource = "header"
			}
		}

		// If no token in header, try to get from HTTP-only cookie
		if token == "" {
			if cookie := ctx.Header("Cookie"); cookie != "" {
				req := &http.Request{Header: http.Header{"Cookie": []string{cookie}}}
				if c, err := req.Cookie("auth_token"); err == nil && c.Value != "" {
					token = c.Value
					tokenSource = "cookie"
				}
			}
		}

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

		var user *models.User

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
func LenientHumaJWTMiddleware(jwtService *services.JWTService, gracePeriod time.Duration) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		url := ctx.URL()

		var requestID string
		if id, ok := ctx.Context().Value(logger.RequestIDContextKey).(string); ok {
			requestID = id
		}

		logger.AuthDebug(ctx.Context(), "lenient_jwt_middleware_start",
			"path", url.Path,
		)

		var token string
		var tokenSource string

		// Try Authorization header first
		authHeader := ctx.Header("Authorization")
		if authHeader != "" {
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
				token = tokenParts[1]
				tokenSource = "header"
			}
		}

		// Fall back to cookie
		if token == "" {
			if cookie := ctx.Header("Cookie"); cookie != "" {
				req := &http.Request{Header: http.Header{"Cookie": []string{cookie}}}
				if c, err := req.Cookie("auth_token"); err == nil && c.Value != "" {
					token = c.Value
					tokenSource = "cookie"
				}
			}
		}

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
func OptionalHumaJWTMiddleware(jwtService *services.JWTService) func(ctx huma.Context, next func(huma.Context)) {
	apiTokenService := services.NewAPITokenService(nil)

	return func(ctx huma.Context, next func(huma.Context)) {
		var token string

		// Try Authorization header
		authHeader := ctx.Header("Authorization")
		if authHeader != "" {
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
				token = tokenParts[1]
			}
		}

		// Try cookie
		if token == "" {
			if cookie := ctx.Header("Cookie"); cookie != "" {
				req := &http.Request{Header: http.Header{"Cookie": []string{cookie}}}
				if c, err := req.Cookie("auth_token"); err == nil && c.Value != "" {
					token = c.Value
				}
			}
		}

		// No token — continue without user context
		if token == "" {
			next(ctx)
			return
		}

		var user *models.User

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
func GetUserFromContext(ctx context.Context) *models.User {
	if user, ok := ctx.Value(UserContextKey).(*models.User); ok {
		return user
	}
	return nil
}

// writeJWTError writes a JSON error response for JWT authentication failures
func writeJWTError(w http.ResponseWriter, requestID, errorCode, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(JWTErrorResponse{
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

	resp := JWTErrorResponse{
		Success:   false,
		Message:   message,
		ErrorCode: errorCode,
		RequestID: requestID,
	}
	data, _ := json.Marshal(resp)
	ctx.BodyWriter().Write(data)
}
