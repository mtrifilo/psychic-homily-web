package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// No response in the served OpenAPI document that sets a cookie declares an
// http.Cookie field as a header beside Set-Cookie. Huma documents (and writes)
// the element struct's exported fields as headers of their own for a
// slice-of-http.Cookie header field, which put the session token in a Value
// header on POST /auth/login; a multi-cookie response must serialize its
// cookies instead.
func TestNoSetCookieResponseDeclaresACookieField(t *testing.T) {
	rr := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json = %d, want 200", rr.Code)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Headers map[string]json.RawMessage `json:"headers"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}

	cookieFields := map[string]bool{}
	cookieType := reflect.TypeFor[http.Cookie]()
	for i := range cookieType.NumField() {
		if field := cookieType.Field(i); field.IsExported() {
			cookieFields[http.CanonicalHeaderKey(field.Name)] = true
		}
	}

	setCookieOperations := 0
	for path, operations := range spec.Paths {
		for method, operation := range operations {
			for status, response := range operation.Responses {
				if _, setsCookie := response.Headers["Set-Cookie"]; !setsCookie {
					continue
				}
				setCookieOperations++
				for header := range response.Headers {
					if cookieFields[http.CanonicalHeaderKey(header)] {
						t.Errorf("%s %s %s declares http.Cookie field %s as a header beside Set-Cookie", method, path, status, header)
					}
				}
			}
		}
	}
	// The walk must reach the auth routes, or it proves nothing.
	if setCookieOperations == 0 {
		t.Fatal("served spec declares no Set-Cookie response header; the walk missed the auth routes")
	}
}
