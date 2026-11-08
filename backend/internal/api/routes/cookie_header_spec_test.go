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
// header on POST /auth/login; a multi-cookie response serializes its cookies
// instead (setCookieLines in handlers/auth).
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

	// The multi-cookie routes must be among those walked, or the walk proves
	// nothing about them.
	unseen := map[string]bool{
		"post /auth/login":               true,
		"post /auth/oauth/link-by-email": true,
		"post /auth/oauth/finish-signup": true,
	}
	for path, operations := range spec.Paths {
		for method, operation := range operations {
			for status, response := range operation.Responses {
				setsCookie := false
				for header := range response.Headers {
					if http.CanonicalHeaderKey(header) == "Set-Cookie" {
						setsCookie = true
					}
				}
				if !setsCookie {
					continue
				}
				delete(unseen, method+" "+path)
				for header := range response.Headers {
					if cookieFields[http.CanonicalHeaderKey(header)] {
						t.Errorf("%s %s %s declares http.Cookie field %s as a header beside Set-Cookie; serialize the cookies into a []string header field instead", method, path, status, header)
					}
				}
			}
		}
	}
	for operation := range unseen {
		t.Errorf("served spec has no Set-Cookie response for %s", operation)
	}
}
