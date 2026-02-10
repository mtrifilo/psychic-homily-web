package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"

	"psychic-homily-backend/internal/logger"
)

// --- RequestIDMiddleware tests (http.Handler version) ---

func TestRequestIDMiddleware_GeneratesUUID(t *testing.T) {
	var capturedRequestID string

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = logger.GetRequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should generate a valid UUID
	if capturedRequestID == "" {
		t.Fatal("expected request ID in context, got empty")
	}
	if _, err := uuid.Parse(capturedRequestID); err != nil {
		t.Errorf("generated request ID %q is not a valid UUID: %v", capturedRequestID, err)
	}

	// Should also set the response header
	headerID := rr.Header().Get(RequestIDHeader)
	if headerID != capturedRequestID {
		t.Errorf("response header %q != context ID %q", headerID, capturedRequestID)
	}
}

func TestRequestIDMiddleware_UsesClientProvidedID(t *testing.T) {
	clientID := "client-request-123"
	var capturedRequestID string

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = logger.GetRequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, clientID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedRequestID != clientID {
		t.Errorf("request ID = %q, want %q", capturedRequestID, clientID)
	}

	headerID := rr.Header().Get(RequestIDHeader)
	if headerID != clientID {
		t.Errorf("response header = %q, want %q", headerID, clientID)
	}
}

func TestRequestIDMiddleware_SetsLoggerInContext(t *testing.T) {
	var hasLogger bool

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())
		hasLogger = log != nil
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !hasLogger {
		t.Error("expected logger in context")
	}
}

func TestRequestIDMiddleware_CallsNextHandler(t *testing.T) {
	called := false

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("next handler was not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// --- HumaRequestIDMiddleware tests ---

func TestHumaRequestIDMiddleware_GeneratesUUID(t *testing.T) {
	var capturedRequestID string

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	HumaRequestIDMiddleware(ctx, func(next huma.Context) {
		if id, ok := next.Context().Value(logger.RequestIDContextKey).(string); ok {
			capturedRequestID = id
		}
	})

	if capturedRequestID == "" {
		t.Fatal("expected request ID in context, got empty")
	}
	if _, err := uuid.Parse(capturedRequestID); err != nil {
		t.Errorf("generated request ID %q is not a valid UUID: %v", capturedRequestID, err)
	}

	// Check response header
	headerID := rr.Header().Get(RequestIDHeader)
	if headerID != capturedRequestID {
		t.Errorf("response header %q != context ID %q", headerID, capturedRequestID)
	}
}

func TestHumaRequestIDMiddleware_UsesClientProvidedID(t *testing.T) {
	clientID := "huma-client-456"
	var capturedRequestID string

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, clientID)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	HumaRequestIDMiddleware(ctx, func(next huma.Context) {
		if id, ok := next.Context().Value(logger.RequestIDContextKey).(string); ok {
			capturedRequestID = id
		}
	})

	if capturedRequestID != clientID {
		t.Errorf("request ID = %q, want %q", capturedRequestID, clientID)
	}

	headerID := rr.Header().Get(RequestIDHeader)
	if headerID != clientID {
		t.Errorf("response header = %q, want %q", headerID, clientID)
	}
}

func TestHumaRequestIDMiddleware_SetsLoggerInContext(t *testing.T) {
	var hasLogger bool

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	HumaRequestIDMiddleware(ctx, func(next huma.Context) {
		if log, ok := next.Context().Value(logger.LoggerContextKey).(*interface{}); ok {
			_ = log // just check it was set
			hasLogger = true
		}
		// The logger is stored as *slog.Logger; check via FromContext
		log := logger.FromContext(next.Context())
		hasLogger = log != nil
	})

	if !hasLogger {
		t.Error("expected logger in context")
	}
}

func TestHumaRequestIDMiddleware_CallsNextHandler(t *testing.T) {
	called := false

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	HumaRequestIDMiddleware(ctx, func(next huma.Context) {
		called = true
	})

	if !called {
		t.Error("next handler was not called")
	}
}

// --- GetRequestIDFromContext tests ---

func TestGetRequestIDFromContext_WithID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	// Wrap context with request ID
	ctxWithID := huma.WithValue(ctx, logger.RequestIDContextKey, "test-req-789")

	got := GetRequestIDFromContext(ctxWithID)
	if got != "test-req-789" {
		t.Errorf("GetRequestIDFromContext = %q, want %q", got, "test-req-789")
	}
}

func TestGetRequestIDFromContext_WithoutID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	got := GetRequestIDFromContext(ctx)
	if got != "" {
		t.Errorf("GetRequestIDFromContext = %q, want empty", got)
	}
}

func TestGetRequestIDFromContext_WrongType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	// Store a non-string value at the request ID key
	ctxWithWrong := huma.WithValue(ctx, logger.RequestIDContextKey, 12345)

	got := GetRequestIDFromContext(ctxWithWrong)
	if got != "" {
		t.Errorf("GetRequestIDFromContext = %q, want empty for wrong type", got)
	}
}

// --- Integration: HumaRequestIDMiddleware + GetRequestIDFromContext ---

func TestRequestID_HumaRoundTrip(t *testing.T) {
	clientID := "round-trip-id"
	var extractedID string

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, clientID)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	HumaRequestIDMiddleware(ctx, func(next huma.Context) {
		extractedID = GetRequestIDFromContext(next)
	})

	if extractedID != clientID {
		t.Errorf("round-trip request ID = %q, want %q", extractedID, clientID)
	}
}

// --- Uniqueness test ---

func TestRequestIDMiddleware_UniquePerRequest(t *testing.T) {
	var ids []string

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids = append(ids, logger.GetRequestID(r.Context()))
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// All IDs should be unique
	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate request ID: %s", id)
		}
		seen[id] = true
	}
}

// --- Context propagation test ---

func TestRequestIDMiddleware_ContextKeyType(t *testing.T) {
	// Verify that the logger context key type is correct (string-keyed context values
	// don't collide with logger.contextKey)
	ctx := context.Background()
	ctx = context.WithValue(ctx, logger.RequestIDContextKey, "from-logger-key")

	// Retrieving with a plain string "request_id" should NOT find it (different type)
	if val, ok := ctx.Value("request_id").(string); ok {
		t.Errorf("plain string key should not retrieve logger context value, got %q", val)
	}

	// But logger.GetRequestID should work
	if got := logger.GetRequestID(ctx); got != "from-logger-key" {
		t.Errorf("logger.GetRequestID = %q, want %q", got, "from-logger-key")
	}
}
