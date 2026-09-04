package catalog

import (
	"strconv"
	"strings"
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// The venues_capacity_range CHECK constraint, exercised against a real column
// rather than reasoned about from the migration text.
//
// The API layer already refuses these values, so the assertions go around it
// and write the column directly: that is the path the constraint exists for,
// and a test routed through the service would pass with no constraint at all.
func TestVenuesCapacityRangeConstraint(t *testing.T) {
	db := testutil.SetupTestPostgres(t).DB

	newVenue := func(t *testing.T, name string) uint {
		t.Helper()
		v := &catalogm.Venue{Name: name, City: "Testville", State: "AZ"}
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("seed venue %q: %v", name, err)
		}
		return v.ID
	}

	// The bounds the constraint mirrors, read from the contract rather than
	// retyped.
	floor := contracts.MinVenueCapacity
	ceiling := contracts.MaxVenueCapacity

	// Read the constraint's own definition back and compare both numbers.
	//
	// The accept/reject cases below cannot do this on their own: raising the
	// FLOOR in Go without the migration leaves every one of them green, because
	// the rejected values are still below the old floor and the accepted floor
	// value is still inside the old range. Comparing the definition catches a
	// drift in either bound in either direction.
	t.Run("definition matches the contract", func(t *testing.T) {
		var definition string
		err := db.Raw(
			"SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = ?",
			"venues_capacity_range",
		).Scan(&definition).Error
		if err != nil {
			t.Fatalf("reading the constraint definition: %v", err)
		}
		if definition == "" {
			t.Fatal("venues_capacity_range does not exist; the migration did not run")
		}
		for _, want := range []string{strconv.Itoa(floor), strconv.Itoa(ceiling)} {
			if !strings.Contains(definition, want) {
				t.Errorf("constraint %q does not carry the contract bound %s; "+
					"the SQL literals and contracts.Min/MaxVenueCapacity have drifted",
					definition, want)
			}
		}
	})

	t.Run("rejects out of range", func(t *testing.T) {
		cases := []struct {
			name     string
			capacity int
		}{
			{"zero is a second spelling of unknown", 0},
			{"negative", -1},
			{"above ceiling", ceiling + 1},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				id := newVenue(t, "Cap reject "+tc.name)
				err := db.Exec("UPDATE venues SET capacity = ? WHERE id = ?", tc.capacity, id).Error
				if err == nil {
					t.Fatalf("capacity %d was stored; the constraint did not refuse it", tc.capacity)
				}
			})
		}
	})

	t.Run("accepts in range and null", func(t *testing.T) {
		cases := []struct {
			name     string
			capacity any
		}{
			{"floor", floor},
			{"typical room", 100},
			{"ceiling", ceiling},
			{"null means unknown", nil},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				id := newVenue(t, "Cap accept "+tc.name)
				if err := db.Exec("UPDATE venues SET capacity = ? WHERE id = ?", tc.capacity, id).Error; err != nil {
					t.Fatalf("capacity %v was refused: %v", tc.capacity, err)
				}
			})
		}
	})

	// INSERT as well as UPDATE: a CHECK constraint guards both, and a venue
	// created with a junk capacity is the likelier ingest failure of the two.
	t.Run("insert is constrained too", func(t *testing.T) {
		zero := 0
		v := &catalogm.Venue{Name: "Cap insert zero", City: "Testville", State: "AZ", Capacity: &zero}
		if err := db.Create(v).Error; err == nil {
			t.Fatal("a venue with capacity 0 was inserted; the constraint does not cover INSERT")
		}
	})
}
