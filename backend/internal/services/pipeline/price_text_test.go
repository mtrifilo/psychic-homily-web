package pipeline

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// priceOf renders a parsed half for a failure message without dereferencing nil.
func priceOf(p *float64) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%.2f", *p)
}

func TestParseEventPrices_SourceShapes(t *testing.T) {
	floatPtr := func(v float64) *float64 { return &v }

	cases := []struct {
		name    string
		in      string
		advance *float64
		door    *float64
	}{
		// The shapes that already worked, which must keep working.
		{"bare dollar amount", "$18", floatPtr(18), nil},
		{"cents", "$23.81", floatPtr(23.81), nil},
		{"no dollar sign", "20", floatPtr(20), nil},
		{"free", "Free", floatPtr(0), nil},
		{"no cover", "No Cover", floatPtr(0), nil},
		{"empty", "", nil, nil},
		{"unpriced text", "TBA", nil, nil},

		// The data loss this parser exists to fix: every one of these stored
		// NULL for BOTH halves, because the whole string was handed to
		// ParseFloat.
		{"adv slash door", "$20 adv / $25 door", floatPtr(20), floatPtr(25)},
		{"advance spelled out", "$20 advance / $25 at the door", floatPtr(20), floatPtr(25)},
		{"presale comma door", "$20 presale, $25 at the door", floatPtr(20), floatPtr(25)},
		{"labels lead the amounts", "adv $20 / door $25", floatPtr(20), floatPtr(25)},
		{"door stated first", "$25 door / $20 adv", floatPtr(20), floatPtr(25)},
		{"day of show", "$20 / $25 day of show", floatPtr(20), floatPtr(25)},
		{"non-breaking spaces", "$20\u00a0adv\u00a0/\u00a0$25\u00a0door", floatPtr(20), floatPtr(25)},
		{"cents on both halves", "$20.00 adv / $25.50 door", floatPtr(20), floatPtr(25.5)},

		// A thousands separator is not a second amount and not a decimal point.
		// Reading "$1,200" as $1 understates a price a hundredfold, which is
		// worse than reading none at all.
		{"thousands separator", "$1,200", floatPtr(1200), nil},
		{"thousands separator on both halves", "$1,200 adv / $1,500 door", floatPtr(1200), floatPtr(1500)},
		{"thousands separator with cents", "$1,200.50", floatPtr(1200.5), nil},
		{"no separator", "$1200", floatPtr(1200), nil},
		{"space after the sign", "$ 20 adv / $ 25 door", floatPtr(20), floatPtr(25)},

		// Several amounts with no door label are a ticket-tier range, and
		// neither bound is the cost of getting in. SeeTickets serves these
		// verbatim out of span.price -- "$10.00-$30.00" was read off the live
		// Rebel Lounge calendar -- and both halves stay unstated, which is what
		// this parser already did with them.
		{"tier range", "$10.00-$30.00", nil, nil},
		{"bare slash pair", "$20/$25", nil, nil},
		{"price plus a fee", "$20 (plus $3 fees)", nil, nil},

		// A door word that belongs to the doors TIME states no second price.
		{"doors time", "$20, doors at 7", floatPtr(20), nil},
		{"doors open", "$20 doors open 8pm", floatPtr(20), nil},
		{"doors time beside a range", "$20/$25, doors at 7", nil, nil},

		// A lone amount is the show's price whatever word sits beside it.
		{"lone door amount", "$25 at the door", floatPtr(25), nil},
		{"lone advance amount", "$20 adv", floatPtr(20), nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			advance, door := parseEventPrices(tc.in)
			assert.Equal(t, tc.advance, advance, "advance: got %s", priceOf(advance))
			assert.Equal(t, tc.door, door, "door: got %s", priceOf(door))
		})
	}
}

// The explicit-only rule, stated as a property rather than a table row: no
// input that names a single amount may ever produce a door price. Inventing one
// would publish a wrong number about money, which is the failure the poster
// extraction refuses in the same words.
func TestParseEventPrices_NeverInfersADoorPriceFromOneAmount(t *testing.T) {
	for _, in := range []string{
		"$20", "20", "$20 adv", "$25 at the door", "$20 door", "Free", "$20 general admission",
	} {
		_, door := parseEventPrices(in)
		assert.Nil(t, door, "%q states one price and must yield no door price", in)
	}
}
