package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAllowedEditFields_KnownTypes verifies every entity type returned by
// ValidPendingEditEntityTypes() has an entry. Pairs with the
// ValidPendingEditEntityTypes contract test in handlers/admin to make
// sure adding a new entity type triggers a build/test failure rather
// than a silent "no fields editable" surface.
func TestAllowedEditFields_KnownTypes(t *testing.T) {
	for _, entityType := range ValidPendingEditEntityTypes() {
		fields, ok := AllowedEditFields(entityType)
		assert.Truef(t, ok, "AllowedEditFields missing entry for %q", entityType)
		assert.NotEmptyf(t, fields, "AllowedEditFields(%q) is empty", entityType)
	}
}

func TestAllowedEditFields_UnknownTypeReturnsFalse(t *testing.T) {
	fields, ok := AllowedEditFields("show")
	assert.False(t, ok)
	assert.Nil(t, fields)

	fields, ok = AllowedEditFields("")
	assert.False(t, ok)
	assert.Nil(t, fields)

	fields, ok = AllowedEditFields("user")
	assert.False(t, ok)
	assert.Nil(t, fields)
}

// TestAllowedEditFields_DangerousColumnsExcluded asserts that obviously
// privileged or identity-shaping columns are NOT on any entity's
// allowlist. This guards against a contributor (or a malformed pending
// edit row from any source) flipping is_admin, password_hash, trust_tier,
// or rewriting slug / submitted_by / data_source via the pending-edit
// pipeline. The 'shows' table reuses the same column families, so the
// guard applies even though show is not yet a pending_edit entity.
func TestAllowedEditFields_DangerousColumnsExcluded(t *testing.T) {
	// Columns that should never be reachable via pending_edits regardless
	// of entity type. Mix of:
	//   - auth/identity columns from users table (not on entity tables,
	//     but if the JSONB lied about entity_type, GORM would still try
	//     to set them on whatever table — so the allowlist must reject
	//     them per-entity)
	//   - internal provenance columns (data_source, source_confidence)
	//   - identity columns (id, slug, submitted_by, created_at)
	//   - status / verified columns gated to admins only
	dangerous := []string{
		"id",
		"slug",
		"submitted_by",
		"created_at",
		"updated_at",
		"data_source",
		"source_confidence",
		"last_verified_at",
		"is_admin",
		"is_active",
		"password_hash",
		"trust_tier",
		"user_tier",
		"verified",
		"auto_approve",
		"status", // (admin-only on festival; not editable elsewhere)
	}
	for _, entityType := range ValidPendingEditEntityTypes() {
		fields, ok := AllowedEditFields(entityType)
		if !ok {
			t.Fatalf("expected allowlist for %q to exist", entityType)
		}
		for _, col := range dangerous {
			assert.Falsef(t, fields[col],
				"%q must NOT be allowlisted for %s — privileged or identity column",
				col, entityType,
			)
		}
	}
}

func TestFilterAllowedFields_AllAllowed(t *testing.T) {
	changes := []FieldChange{
		{Field: "name", NewValue: "New Name"},
		{Field: "city", NewValue: "Phoenix"},
	}
	filtered, rejected := FilterAllowedFields(PendingEditEntityArtist, changes)
	assert.Empty(t, rejected)
	assert.Len(t, filtered, 2)
	assert.Equal(t, "name", filtered[0].Field)
	assert.Equal(t, "city", filtered[1].Field)
}

func TestFilterAllowedFields_SomeRejected(t *testing.T) {
	changes := []FieldChange{
		{Field: "name", NewValue: "New Name"},
		{Field: "is_admin", NewValue: true},
		{Field: "city", NewValue: "Phoenix"},
		{Field: "password_hash", NewValue: "pwned"},
	}
	filtered, rejected := FilterAllowedFields(PendingEditEntityArtist, changes)
	assert.ElementsMatch(t, []string{"is_admin", "password_hash"}, rejected)
	assert.Len(t, filtered, 2)
	// In-allowlist fields preserved in original order with their values intact.
	assert.Equal(t, "name", filtered[0].Field)
	assert.Equal(t, "New Name", filtered[0].NewValue)
	assert.Equal(t, "city", filtered[1].Field)
}

func TestFilterAllowedFields_AllRejectedKnownEntity(t *testing.T) {
	changes := []FieldChange{
		{Field: "is_admin", NewValue: true},
		{Field: "password_hash", NewValue: "pwned"},
	}
	filtered, rejected := FilterAllowedFields(PendingEditEntityArtist, changes)
	assert.ElementsMatch(t, []string{"is_admin", "password_hash"}, rejected)
	assert.Empty(t, filtered)
}

func TestFilterAllowedFields_UnknownEntityRejectsAll(t *testing.T) {
	changes := []FieldChange{
		{Field: "name", NewValue: "x"},
		{Field: "city", NewValue: "y"},
	}
	filtered, rejected := FilterAllowedFields("show", changes)
	assert.Nil(t, filtered)
	assert.ElementsMatch(t, []string{"name", "city"}, rejected)
}

func TestFilterAllowedFields_EmptyChanges(t *testing.T) {
	filtered, rejected := FilterAllowedFields(PendingEditEntityArtist, nil)
	assert.Empty(t, rejected)
	assert.Empty(t, filtered)

	filtered, rejected = FilterAllowedFields(PendingEditEntityArtist, []FieldChange{})
	assert.Empty(t, rejected)
	assert.Empty(t, filtered)
}
