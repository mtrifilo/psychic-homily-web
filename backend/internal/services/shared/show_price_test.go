package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The dense-money register, pinned where every backend surface can reach it.
//
// The ICS feeds are the caller that made this urgent: a calendar entry persists
// in a subscriber's phone long after the page view that created it, so serving
// the advance half of a split price there under-reported the cost until the
// reader was at the door. The register matches the site's list surfaces exactly,
// so the number seen on /shows is the number in the calendar.
func TestShowPriceText(t *testing.T) {
	price := func(v float64) *float64 { return &v }

	tests := []struct {
		name          string
		advance, door *float64
		want          string
	}{
		{name: "no recorded price says nothing", want: ""},
		{name: "advance only renders bare", advance: price(35), want: "$35"},
		{name: "door only renders bare too", door: price(40), want: "$40"},
		{name: "a genuine pair renders both", advance: price(35), door: price(40), want: "$35/$40"},
		// Nothing stops a curator or an importer entering the same number
		// twice, and "$35/$35" spends two slots saying one thing.
		{name: "equal prices collapse", advance: price(35), door: price(35), want: "$35"},
		// Free is a price the site spells out, not an absence.
		{name: "zero is Free, not silence", advance: price(0), want: "Free"},
		{name: "a free advance with a paid door keeps both", advance: price(0), door: price(10), want: "Free/$10"},
		// Whole amounts drop the cents; a real fractional price keeps them.
		// "$%.0f" alone printed 12.50 as "$12" and, worse, printed a fifty-cent
		// door as "$0" — free, in a subscriber's calendar. Mirrors formatPrice
		// in frontend/lib/utils/formatters.ts.
		{name: "whole amounts drop the cents", advance: price(35), want: "$35"},
		{name: "a fractional price keeps them", advance: price(12.5), want: "$12.50"},
		{name: "and is never rounded", advance: price(12.75), want: "$12.75"},
		{name: "a sub-dollar price is not free", advance: price(0.5), want: "$0.50"},
		// Unusual but not impossible, and the order is stated rather than
		// sorted: advance first, whichever number is larger.
		{name: "a cheaper door keeps advance first", advance: price(40), door: price(30), want: "$40/$30"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ShowPriceText(tc.advance, tc.door))
		})
	}
}
