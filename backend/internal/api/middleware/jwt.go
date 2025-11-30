package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

type contextKey string

const UserContextKey contextKey = "user"

// JWTMiddleware validates JWT tokens
func JWTMiddleware(jwtService *services.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("DEBUG: JWT Middleware - Processing request to %s", r.URL.Path)

			var token string

			// First, try to get token from Authorization header
			authHeader := r.Header.Get("Authorization")
			log.Printf("DEBUG: JWT Middleware - Auth header: %s", authHeader)

			if authHeader != "" {
				// Extract token from "Bearer <token>"
				tokenParts := strings.Split(authHeader, " ")
				if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
					token = tokenParts[1]
					log.Printf("DEBUG: JWT Middleware - Token found in Authorization header")
				}
			}

			// If no token in header, try to get from HTTP-only cookie
			if token == "" {
				cookie, err := r.Cookie("auth_token")
				if err == nil && cookie.Value != "" {
					token = cookie.Value
					log.Printf("DEBUG: JWT Middleware - Token found in cookie")
				}
			}

			if token == "" {
				log.Printf("DEBUG: JWT Middleware - No token found in header or cookie")
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Validate token
			user, err := jwtService.ValidateToken(token)
			if err != nil {
				log.Printf("DEBUG: JWT Middleware - Token validation failed: %v", err)
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			log.Printf("DEBUG: JWT Middleware - Token validated successfully for user: %+v", user)

			// Add user to context
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func HumaJWTMiddleware(jwtService *services.JWTService) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// Access request through context interface
		url := ctx.URL()
		log.Printf("DEBUG: Huma JWT Middleware - Processing request to %s", url.Path)

		var token string

		// First, try to get token from Authorization header
		authHeader := ctx.Header("Authorization")
		log.Printf("DEBUG: Huma JWT Middleware - Auth header: %s", authHeader)

		if authHeader != "" {
			// Extract token from "Bearer <token>"
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
				token = tokenParts[1]
				log.Printf("DEBUG: Huma JWT Middleware - Token found in Authorization header")
			}
		}

		// If no token in header, try to get from HTTP-only cookie
		if token == "" {
			if cookie := ctx.Header("Cookie"); cookie != "" {
				req := &http.Request{Header: http.Header{"Cookie": []string{cookie}}}
				if c, err := req.Cookie("auth_token"); err == nil && c.Value != "" {
					token = c.Value
					log.Printf("DEBUG: Huma JWT Middleware - Token found in cookie")
				}
			}
		}

		if token == "" {
			log.Printf("DEBUG: Huma JWT Middleware - No token found in header or cookie")
			ctx.SetStatus(http.StatusUnauthorized)
			ctx.BodyWriter().Write([]byte(`{"message":"Authentication required"}`))
			return
		}

		// Validate token
		user, err := jwtService.ValidateToken(token)
		if err != nil {
			log.Printf("DEBUG: Huma JWT Middleware - Token validation failed: %v", err)
			ctx.SetStatus(http.StatusUnauthorized)
			ctx.BodyWriter().Write([]byte(`{"message":"Invalid token"}`))
			return
		}

		log.Printf("DEBUG: Huma JWT Middleware - Token validated successfully for user: %+v", user)

		// Store user in context for handlers to access
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
