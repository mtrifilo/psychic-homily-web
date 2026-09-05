package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
)

// columnBoundedRollbackFields is keyed by FIELD NAME with no entity type, which
// is only safe while each of those names belongs to exactly one entity. A second
// entity gaining a "capacity" column would inherit venues_capacity_range's range
// without having the constraint, and the gate would refuse a rollback its own
// column would have accepted.
//
// Nothing in the gate can notice that, so it is pinned here.
func TestColumnBoundedRollbackFieldsBelongToOneEntityEach(t *testing.T) {
	for field := range columnBoundedRollbackFields {
		owners := []string{}
		for _, entityType := range adminm.ValidPendingEditEntityTypes() {
			fields, ok := adminm.AllowedEditFields(entityType)
			if !ok {
				continue
			}
			if fields[field] {
				owners = append(owners, entityType)
			}
		}
		assert.Lenf(t, owners, 1,
			"%q is editable on %v; columnBoundedRollbackFields keys on the field name alone, "+
				"so it must name a column only one entity has", field, owners)
	}
}

// The gate reads its range from the same registry the column's SQL mirrors, so a
// field listed as column-bounded but absent from that registry would silently
// check nothing.
func TestColumnBoundedRollbackFieldsAreRegistered(t *testing.T) {
	registry := contracts.NumericEditFieldBounds()
	for field := range columnBoundedRollbackFields {
		_, registered := registry[field]
		assert.Truef(t, registered,
			"%q is declared column-bounded but is not in NumericEditFieldBounds, so the gate is a no-op for it",
			field)
	}
}

// The gate itself, away from a database: what it refuses, what it passes, and
// the shapes that must never be refused.
func TestColumnBoundRollbackError(t *testing.T) {
	registry := contracts.NumericEditFieldBounds()
	ptr := func(n int) *int { return &n }

	cases := []struct {
		name    string
		field   string
		value   interface{}
		refused bool
	}{
		{"below the floor is refused", "capacity", ptr(0), true},
		{"negative is refused", "capacity", ptr(-1), true},
		{"above the ceiling is refused", "capacity", ptr(contracts.MaxVenueCapacity + 1), true},
		{"the floor passes", "capacity", ptr(contracts.MinVenueCapacity), false},
		{"the ceiling passes", "capacity", ptr(contracts.MaxVenueCapacity), false},
		// The clear gesture. NULL is what the column accepts for "unknown", so
		// refusing it would block the one undo that always worked.
		{"a typed nil is the clear gesture and passes", "capacity", (*int)(nil), false},
		// narrowNumericUpdate runs first and either converts or rejects, so a
		// value still in its JSONB shape here is not this gate's to judge.
		{"an unnarrowed value is left to the earlier gate", "capacity", float64(0), false},
		// A field bounded by the API alone must stay undoable, which is the whole
		// distinction the map encodes.
		{"an out-of-range founded_year is NOT refused", "founded_year", ptr(1), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updates := map[string]interface{}{tc.field: tc.value}
			err := columnBoundRollbackError(updates, tc.field, registry)
			if tc.refused {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.field)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// An absent field is not a refusal: a revision that never recorded the field
// passes the gate untouched.
func TestColumnBoundRollbackErrorIgnoresAbsentFields(t *testing.T) {
	assert.NoError(t, columnBoundRollbackError(map[string]interface{}{}, "capacity",
		contracts.NumericEditFieldBounds()))
}
