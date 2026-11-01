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

	// maxInboundRequestIDLength is the longest inbound X-Request-ID a request
	// keeps as its id.
	maxInboundRequestIDLength = 128
)

// RequestIDMiddleware gives each request one id: the inbound X-Request-ID when
// requestIDFromHeader accepts it, else a fresh UUID. The context id, the
// context logger and the response header all carry it.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFromHeader(r.Header.Get(RequestIDHeader))

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
//
// A request's id is, in order: the id already on the request context, which
// RequestIDMiddleware puts there when it runs ahead of Huma; else the inbound
// X-Request-ID header when requestIDFromHeader accepts it; else a fresh UUID.
// The response header, the context id and the context logger all carry that
// one id, so a request keeps the id it arrived with however many times this
// runs.
func HumaRequestIDMiddleware(ctx huma.Context, next func(huma.Context)) {
	requestID := logger.GetRequestID(ctx.Context())
	if requestID == "" {
		requestID = requestIDFromHeader(ctx.Header(RequestIDHeader))
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

// requestIDFromHeader returns the inbound X-Request-ID when it is 1 to
// maxInboundRequestIDLength characters, each an ASCII letter or digit, '.', '_'
// or '-'; otherwise it returns a fresh UUID. The id is echoed into the response
// header, every log line of the request and the Sentry tag, so a client cannot
// make it long or put spaces or separators into it.
func requestIDFromHeader(inbound string) string {
	if isAcceptableInboundRequestID(inbound) {
		return inbound
	}
	return uuid.New().String()
}

func isAcceptableInboundRequestID(id string) bool {
	if id == "" || len(id) > maxInboundRequestIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
