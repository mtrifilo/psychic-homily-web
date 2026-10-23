package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// The header rides on a served JSON document and on the router's own 404 and
// 405 answers, which no handler writes.
func TestRobotsNoIndexMarksEveryResponse(t *testing.T) {
	router := chi.NewRouter()
	router.Use(RobotsNoIndex)
	router.Get("/shows", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"shows":[]}`))
	})

	cases := []struct {
		name, method, path string
		status             int
	}{
		{"json endpoint", http.MethodGet, "/shows", http.StatusOK},
		{"not found", http.MethodGet, "/does-not-exist", http.StatusNotFound},
		{"method not allowed", http.MethodPost, "/shows", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, httptest.NewRequest(c.method, c.path, nil))
			if rr.Code != c.status {
				t.Fatalf("%s %s = %d, want %d", c.method, c.path, rr.Code, c.status)
			}
			if got := rr.Header().Get(RobotsTagHeader); got != RobotsTagNoIndex {
				t.Errorf("%s = %q, want %q", RobotsTagHeader, got, RobotsTagNoIndex)
			}
		})
	}
}
