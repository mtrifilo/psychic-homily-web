package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

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
			
			// Get token from Authorization header
			authHeader := r.Header.Get("Authorization")
			log.Printf("DEBUG: JWT Middleware - Auth header: %s", authHeader)
			
			if authHeader == "" {
				log.Printf("DEBUG: JWT Middleware - No auth header")
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			// Extract token from "Bearer <token>"
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
				log.Printf("DEBUG: JWT Middleware - Invalid auth header format")
				http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
				return
			}

			token := tokenParts[1]

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

// GetUserFromContext extracts user from request context
func GetUserFromContext(ctx context.Context) *models.User {
	if user, ok := ctx.Value(UserContextKey).(*models.User); ok {
		return user
	}
	return nil
}
