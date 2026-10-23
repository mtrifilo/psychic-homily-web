package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// No response body and no schema in the served OpenAPI document carries a
// `$schema` member, and no response links its schema with a describedBy header.
func TestResponsesCarryNoSchemaLinks(t *testing.T) {
	router := newTestRouter(t)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /health = %d, want 200", rr.Code)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /health: %v", err)
	}
	if _, ok := body["$schema"]; ok {
		t.Errorf("GET /health body carries $schema: %s", rr.Body.String())
	}
	if link := rr.Header().Get("Link"); strings.Contains(link, "describedBy") {
		t.Errorf("GET /health carries a describedBy Link header: %q", link)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json = %d, want 200", rr.Code)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if len(spec.Components.Schemas) == 0 {
		t.Fatal("served spec declares no component schemas")
	}
	for name, schema := range spec.Components.Schemas {
		if _, ok := schema.Properties["$schema"]; ok {
			t.Errorf("schema %s declares a $schema property", name)
		}
	}
}
