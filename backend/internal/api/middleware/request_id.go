package middleware

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"psychic-homily-backend/internal/logger"
)

const (
	// RequestIDHeader is the HTTP header name for request ID
	RequestIDHeader = "X-Request-ID"
)

// RequestIDMiddleware adds a unique request ID to each request.
// If the client provides an X-Request-ID header, it will be used.
// Otherwise, a new UUID will be generated.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store request ID in context
		ctx := logger.SetRequestID(r.Context(), requestID)

		// Create a logger with the request ID and store in context
		log := logger.WithRequestID(logger.Default(), requestID)
		ctx = logger.NewContext(ctx, log)

		// Set request ID in response header for client correlation
		w.Header().Set(RequestIDHeader, requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// HumaRequestIDMiddleware is the Huma-compatible version of RequestIDMiddleware.
func HumaRequestIDMiddleware(ctx huma.Context, next func(huma.Context)) {
	requestID := ctx.Header(RequestIDHeader)
	if requestID == "" {
		requestID = uuid.New().String()
	}

	// Store request ID in context using Huma's context value mechanism
	ctxWithRequestID := huma.WithValue(ctx, logger.RequestIDContextKey, requestID)

	// Create a logger with the request ID
	log := logger.WithRequestID(logger.Default(), requestID)
	ctxWithLogger := huma.WithValue(ctxWithRequestID, logger.LoggerContextKey, log)

	// Set request ID in response header for client correlation
	ctx.SetHeader(RequestIDHeader, requestID)

	next(ctxWithLogger)
}

// GetRequestIDFromContext extracts the request ID from the context.
// Returns empty string if not found.
func GetRequestIDFromContext(ctx huma.Context) string {
	if id, ok := ctx.Context().Value(logger.RequestIDContextKey).(string); ok {
		return id
	}
	return ""
}
