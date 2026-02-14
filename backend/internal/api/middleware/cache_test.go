package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

// --- Chi middleware tests ---

func TestCacheControl_GETRequest(t *testing.T) {
	handler := CacheControl(300)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/shows/upcoming", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	got := rr.Header().Get("Cache-Control")
	want := "public, max-age=300, stale-while-revalidate=1800"
	if got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}

func TestCacheControl_NonGETRequests(t *testing.T) {
	handler := CacheControl(300)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/shows", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if got := rr.Header().Get("Cache-Control"); got != "" {
				t.Errorf("Cache-Control for %s = %q, want empty", method, got)
			}
		})
	}
}

func TestCacheControl_DifferentMaxAge(t *testing.T) {
	tests := []struct {
		name   string
		maxAge int
		want   string
	}{
		{"2min", 120, "public, max-age=120, stale-while-revalidate=720"},
		{"10min", 600, "public, max-age=600, stale-while-revalidate=3600"},
		{"30min", 1800, "public, max-age=1800, stale-while-revalidate=10800"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := CacheControl(tt.maxAge)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if got := rr.Header().Get("Cache-Control"); got != tt.want {
				t.Errorf("Cache-Control = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCacheControl_CallsNextHandler(t *testing.T) {
	called := false
	handler := CacheControl(300)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("next handler was not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// --- Huma middleware tests ---

func TestHumaCacheControl_GETRequest(t *testing.T) {
	mw := HumaCacheControl(300)

	req := httptest.NewRequest(http.MethodGet, "/shows/upcoming", nil)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	called := false
	mw(ctx, func(next huma.Context) {
		called = true
	})

	if !called {
		t.Error("next handler was not called")
	}

	got := rr.Header().Get("Cache-Control")
	want := "public, max-age=300, stale-while-revalidate=1800"
	if got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}

func TestHumaCacheControl_NonGETRequests(t *testing.T) {
	mw := HumaCacheControl(300)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/shows", nil)
			rr := httptest.NewRecorder()
			ctx := humatest.NewContext(nil, req, rr)

			mw(ctx, func(next huma.Context) {})

			if got := rr.Header().Get("Cache-Control"); got != "" {
				t.Errorf("Cache-Control for %s = %q, want empty", method, got)
			}
		})
	}
}

func TestHumaCacheControl_DifferentMaxAge(t *testing.T) {
	tests := []struct {
		name   string
		maxAge int
		want   string
	}{
		{"2min", 120, "public, max-age=120, stale-while-revalidate=720"},
		{"10min", 600, "public, max-age=600, stale-while-revalidate=3600"},
		{"30min", 1800, "public, max-age=1800, stale-while-revalidate=10800"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := HumaCacheControl(tt.maxAge)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			ctx := humatest.NewContext(nil, req, rr)

			mw(ctx, func(next huma.Context) {})

			if got := rr.Header().Get("Cache-Control"); got != tt.want {
				t.Errorf("Cache-Control = %q, want %q", got, tt.want)
			}
		})
	}
}
