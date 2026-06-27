package admin

import (
	"reflect"
	"strings"
	"testing"

	adminm "psychic-homily-backend/internal/models/admin"

	"github.com/stretchr/testify/assert"
)

// TestAllowedEditFieldsCoversAllTypes ensures every entity type in
// adminm.ValidPendingEditEntityTypes() has a non-empty entry in
// allowedEditFields. Without this, adding a new entity type to the model
// allowlist silently produces a handler that rejects every user-submitted
// field (because the empty inner map returns false for every lookup).
//
// PSY-492 contract drift guard.
func TestAllowedEditFieldsCoversAllTypes(t *testing.T) {
	for _, entityType := range adminm.ValidPendingEditEntityTypes() {
		fields, ok := adminm.AllowedEditFields(entityType)
		assert.Truef(t, ok, "AllowedEditFields missing entry for %q", entityType)
		assert.NotEmptyf(t, fields, "AllowedEditFields(%q) is empty", entityType)
	}
}

// TestSuggestEditHandlerExistsForAllTypes ensures every entity type has a
// corresponding Suggest{Type}EditHandler method on PendingEditHandler.
// Route registration still has to be updated manually in routes.go, but
// at least the handler method — which is the main hook the route refers
// to — can't go missing without failing this test.
//
// PSY-492 contract drift guard.
func TestSuggestEditHandlerExistsForAllTypes(t *testing.T) {
	hType := reflect.TypeOf(&PendingEditHandler{})
	for _, entityType := range adminm.ValidPendingEditEntityTypes() {
		methodName := "Suggest" + capitalize(entityType) + "EditHandler"
		_, ok := hType.MethodByName(methodName)
		assert.Truef(t, ok, "PendingEditHandler missing method %q for entity type %q", methodName, entityType)
	}
}

// capitalize returns s with its first letter uppercased. Avoids strings.Title
// (deprecated) and the unicode-aware cases package (overkill for ASCII slugs).
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
