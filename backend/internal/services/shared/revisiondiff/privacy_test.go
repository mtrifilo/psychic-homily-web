package revisiondiff

import (
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
		wantRedact  bool
		wantMasked  []string
		wantIntactN int
	}{
		{
			name: "masks address",
			changes: []adminm.FieldChange{
				{Field: "address", OldValue: "", NewValue: "1234 Secret St"},
			},
			wantRedact: true,
			wantMasked: []string{"address"},
		},
		{
			name: "masks zipcode",
			changes: []adminm.FieldChange{
				{Field: "zipcode", OldValue: "85004", NewValue: "85006"},
			},
			wantRedact: true,
			wantMasked: []string{"zipcode"},
		},
		{
			name: "masks only the private fields in a mixed diff",
			changes: []adminm.FieldChange{
				{Field: "name", OldValue: "Old Room", NewValue: "New Room"},
				{Field: "address", OldValue: "1 Old St", NewValue: "1234 Secret St"},
				{Field: "capacity", OldValue: 100, NewValue: 150},
				{Field: "zipcode", OldValue: "", NewValue: "85004"},
			},
			wantRedact:  true,
			wantMasked:  []string{"address", "zipcode"},
			wantIntactN: 2,
		},
		{
			name: "reports no redaction for a diff with no private field",
			changes: []adminm.FieldChange{
				{Field: "name", OldValue: "Old Room", NewValue: "New Room"},
				{Field: "capacity", OldValue: 100, NewValue: 150},
			},
			wantRedact:  false,
			wantIntactN: 2,
		},
		{
			name:       "empty diff",
			changes:    []adminm.FieldChange{},
			wantRedact: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, redacted := RedactVenueChanges(tt.changes)

			if redacted != tt.wantRedact {
				t.Fatalf("redacted = %v, want %v", redacted, tt.wantRedact)
			}
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

	if _, redacted := RedactVenueChanges(in); !redacted {
		t.Fatal("expected redaction")
	}

	if in[0].OldValue != "1 Old St" || in[0].NewValue != "1234 Secret St" {
		t.Errorf("input mutated: %+v", in[0])
	}
}

// Renaming a field in VenueFields without updating privacy.go turns the gate
// into a silent no-op; init() must reject that.
func TestValidateVenuePrivateFields(t *testing.T) {
	if err := validateVenuePrivateFields(); err != nil {
		t.Fatalf("validateVenuePrivateFields() = %v, want nil", err)
	}

	original := venuePrivateFields
	t.Cleanup(func() { venuePrivateFields = original })

	venuePrivateFields = map[string]struct{}{"not_a_venue_field": {}}
	if err := validateVenuePrivateFields(); err == nil {
		t.Error("expected an error for a private field absent from VenueFields")
	}
}
