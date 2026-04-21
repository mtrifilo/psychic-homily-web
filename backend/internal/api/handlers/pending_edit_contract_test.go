package handlers

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"psychic-homily-backend/internal/models"
)

// TestAllowedEditFieldsCoversAllTypes ensures every entity type in
// models.ValidPendingEditEntityTypes() has a non-empty entry in
// allowedEditFields. Without this, adding a new entity type to the model
// allowlist silently produces a handler that rejects every user-submitted
// field (because the empty inner map returns false for every lookup).
//
// PSY-492 contract drift guard.
func TestAllowedEditFieldsCoversAllTypes(t *testing.T) {
	for _, entityType := range models.ValidPendingEditEntityTypes() {
		fields, ok := allowedEditFields[entityType]
		assert.Truef(t, ok, "allowedEditFields missing entry for %q", entityType)
		assert.NotEmptyf(t, fields, "allowedEditFields[%q] is empty", entityType)
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
	for _, entityType := range models.ValidPendingEditEntityTypes() {
		methodName := "Suggest" + capitalize(entityType) + "EditHandler"
		_, ok := hType.MethodByName(methodName)
		assert.Truef(t, ok, "PendingEditHandler missing method %q for entity type %q", methodName, entityType)
	}
}

// TestDataGapsHandlerAcceptsAllEntityTypes ensures the data-gaps switch does
// not reject any valid entity type with a 400. It calls the handler with a
// non-existent ID for each type and asserts the returned error is NOT a
// bad-request (i.e., the switch dispatches to a per-type helper which then
// 404s, not a 400 from the default branch).
//
// Uses the admin-tier entity types from ValidPendingEditEntityTypes because
// the data-gaps and pending-edit allowlists are intentionally aligned.
//
// PSY-492 contract drift guard.
func TestDataGapsHandlerAcceptsAllEntityTypes(t *testing.T) {
	// We intentionally pass nil services: the handler should reach the
	// "unknown entity type" branch BEFORE any service call for unrecognized
	// types (this is what the test is guarding against). For recognized
	// types it'll hit a nil-pointer deref, which is also "not a 400" — we
	// catch it with recover.
	h := &DataGapsHandler{}
	for _, entityType := range models.ValidPendingEditEntityTypes() {
		t.Run(entityType, func(t *testing.T) {
			defer func() {
				// Nil-service panic is acceptable — it proves we dispatched
				// past the default branch. Absence of panic (or a 400) is
				// the failure mode we care about.
				_ = recover()
			}()
			req := &GetDataGapsRequest{EntityType: entityType, IDOrSlug: "nonexistent"}
			_, err := h.GetDataGapsHandler(dataGapsCtxWithUser(), req)
			if err != nil {
				assert.NotContainsf(t, err.Error(), "Invalid entity type",
					"data-gaps handler rejected valid entity type %q with 400", entityType)
			}
		})
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
