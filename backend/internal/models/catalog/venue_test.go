package catalog

import "testing"

// PublicAddress is the single expression of the street-address privacy gate,
// so its truth table is pinned here rather than only through the response
// builders that call it.
func TestVenuePublicAddress(t *testing.T) {
	addr := "1234 Secret St"

	tests := []struct {
		name  string
		venue *Venue
		want  *string
	}{
		{
			name:  "unverified venue withholds its address",
			venue: &Venue{Verified: false, Address: &addr},
			want:  nil,
		},
		{
			name:  "verified venue serves its address",
			venue: &Venue{Verified: true, Address: &addr},
			want:  &addr,
		},
		{
			name:  "verified venue without an address stays nil",
			venue: &Venue{Verified: true},
			want:  nil,
		},
		{
			name:  "unverified venue without an address stays nil",
			venue: &Venue{Verified: false},
			want:  nil,
		},
		{
			// No venue relation in this package is a pointer today, so this
			// branch is unreachable rather than load-bearing. It is pinned
			// because a privacy gate is the wrong place to answer a nil
			// receiver with a panic that a caller might recover, or with an
			// address: whatever a future pointer-shaped relation does here, it
			// must fail closed.
			name:  "nil venue withholds",
			venue: nil,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.venue.PublicAddress()
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("expected no address, got %q", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("expected %q, got nil", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("expected %q, got %q", *tt.want, *got)
			}
		})
	}
}
