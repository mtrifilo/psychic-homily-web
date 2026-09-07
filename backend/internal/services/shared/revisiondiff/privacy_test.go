package revisiondiff

import (
	"strings"
	"testing"

	adminm "psychic-homily-backend/internal/models/admin"
)

// RedactVenueChanges is the field-name half of the unverified-venue address
// gate. A miss here is not a wrong render, it is a published home address, so
// the cases below pin the exact field set and that BOTH sides are masked.
func TestRedactVenueChanges(t *testing.T) {
	tests := []struct {
		name        string
		changes     []adminm.FieldChange
		wantMasked  []string
		wantIntactN int
	}{
		{
			name: "masks address",
			changes: []adminm.FieldChange{
				{Field: "address", OldValue: "", NewValue: "1234 Secret St"},
			},
			wantMasked:  []string{"address"},
			wantIntactN: 0,
		},
		{
			name: "masks zipcode",
			changes: []adminm.FieldChange{
				{Field: "zipcode", OldValue: "85004", NewValue: "85006"},
			},
			wantMasked:  []string{"zipcode"},
			wantIntactN: 0,
		},
		{
			name: "masks only the private fields in a mixed diff",
			changes: []adminm.FieldChange{
				{Field: "name", OldValue: "Old Room", NewValue: "New Room"},
				{Field: "address", OldValue: "1 Old St", NewValue: "1234 Secret St"},
				{Field: "capacity", OldValue: 100, NewValue: 150},
				{Field: "zipcode", OldValue: "", NewValue: "85004"},
			},
			wantMasked:  []string{"address", "zipcode"},
			wantIntactN: 2,
		},
		{
			name: "leaves a diff with no private field alone",
			changes: []adminm.FieldChange{
				{Field: "name", OldValue: "Old Room", NewValue: "New Room"},
				{Field: "capacity", OldValue: 100, NewValue: 150},
			},
			wantIntactN: 2,
		},
		{
			name:        "empty diff",
			changes:     []adminm.FieldChange{},
			wantIntactN: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := RedactVenueChanges(tt.changes)

			if len(out) != len(tt.changes) {
				t.Fatalf("len(out) = %d, want %d", len(out), len(tt.changes))
			}

			masked := map[string]bool{}
			for _, f := range tt.wantMasked {
				masked[f] = true
			}
			intact := 0
			for _, c := range out {
				if masked[c.Field] {
					if c.OldValue != RedactedValue || c.NewValue != RedactedValue {
						t.Errorf("field %q: old=%v new=%v, want both %q", c.Field, c.OldValue, c.NewValue, RedactedValue)
					}
					continue
				}
				if c.OldValue == RedactedValue || c.NewValue == RedactedValue {
					t.Errorf("field %q was masked but should not be", c.Field)
				}
				intact++
			}
			if intact != tt.wantIntactN {
				t.Errorf("intact fields = %d, want %d", intact, tt.wantIntactN)
			}
		})
	}
}

// The stored row must survive redaction untouched: rollback reads the same
// unmarshalled changes and would otherwise write "(hidden)" into the column.
func TestRedactVenueChanges_DoesNotMutateInput(t *testing.T) {
	in := []adminm.FieldChange{
		{Field: "address", OldValue: "1 Old St", NewValue: "1234 Secret St"},
	}

	out := RedactVenueChanges(in)
	if out[0].NewValue != RedactedValue {
		t.Fatalf("expected redaction, got %v", out[0].NewValue)
	}

	if in[0].OldValue != "1 Old St" || in[0].NewValue != "1234 Secret St" {
		t.Errorf("input mutated: %+v", in[0])
	}
}

// A masked field name that either writing vocabulary can no longer produce
// turns the gate into a silent no-op. ValidateAll must reject that at init, and
// it must reject it for BOTH vocabularies: the contributor edit path, not the
// admin diff path, is the writer that actually records an unverified venue's
// address.
func TestValidatePrivateFields(t *testing.T) {
	private := map[string]struct{}{"address": {}}
	diffFields := []Field{{Name: "address", Path: "Address"}}
	editable := map[string]bool{"address": true}

	t.Run("passes when the name is in both vocabularies", func(t *testing.T) {
		if err := validatePrivateFields(private, diffFields, editable); err != nil {
			t.Fatalf("got %v, want nil", err)
		}
	})

	t.Run("rejects a name absent from the diff field list", func(t *testing.T) {
		err := validatePrivateFields(private, []Field{{Name: "city", Path: "City"}}, editable)
		if err == nil {
			t.Fatal("want an error")
		}
		if !strings.Contains(err.Error(), "VenueFields") {
			t.Errorf("error should name the list that is missing it, got %v", err)
		}
	})

	t.Run("rejects a name the contributor path cannot record", func(t *testing.T) {
		err := validatePrivateFields(private, diffFields, map[string]bool{"city": true})
		if err == nil {
			t.Fatal("want an error")
		}
		if !strings.Contains(err.Error(), "VenueAllowedEditFields") {
			t.Errorf("error should name the list that is missing it, got %v", err)
		}
	})

	t.Run("the real vocabularies are consistent", func(t *testing.T) {
		if err := validateVenuePrivateFields(); err != nil {
			t.Fatalf("got %v, want nil", err)
		}
	})
}
