package pipeline

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{"day of show", "$20 adv / $25 day of show", floatPtr(20), floatPtr(25)},
		{"day of THE show", "$20 in advance, $25 the day of the show", floatPtr(20), floatPtr(25)},
		{"day-of-show abbreviated", "$20 ADV / $25 DOS", floatPtr(20), floatPtr(25)},
		{"cents on both halves", "$20.00 adv / $25.50 door", floatPtr(20), floatPtr(25.5)},
		{"age restriction after the door label", "$20 adv / $25 door 21+", floatPtr(20), floatPtr(25)},
		{"labels flush against their amounts", "$20adv/$25door", floatPtr(20), floatPtr(25)},

		// Whitespace folding is what lets statedAmount's `\s*` see these at
		// all: RE2's `\s` does not cover U+00A0, so the sign and its digits
		// would not read as one amount.
		{"non-breaking spaces between the words", "$20\u00a0adv\u00a0/\u00a0$25\u00a0door", floatPtr(20), floatPtr(25)},
		{"non-breaking space after the sign", "$\u00a020 adv / $\u00a025 door", floatPtr(20), floatPtr(25)},

		// A thousands separator is not a second amount and not a decimal point.
		// Reading "$1,200" as $1 understates a price a hundredfold, which is
		// worse than reading none at all.
		{"thousands separator", "$1,200", floatPtr(1200), nil},
		{"thousands separator on both halves", "$1,200 adv / $1,500 door", floatPtr(1200), floatPtr(1500)},
		{"thousands separator with cents", "$1,200.50", floatPtr(1200.5), nil},
		{"no separator", "$1200", floatPtr(1200), nil},
		{"space after the sign", "$ 20 adv / $ 25 door", floatPtr(20), floatPtr(25)},

		// The split survives punctuation the labels are written with, which is
		// what a distance rule read on one label at a time got wrong.
		{"colons after the labels", "Advance: $20, Door: $25", floatPtr(20), floatPtr(25)},
		{"no punctuation at all", "Advance $20 Door $25", floatPtr(20), floatPtr(25)},
		{"labels lead, no punctuation", "adv $20 door $25", floatPtr(20), floatPtr(25)},
		{"modifiers between amount and label", "$20 online in advance, $25 cash at the door", floatPtr(20), floatPtr(25)},
		{"door stated first with modifiers", "$25 cash at the door, $20 advance online", floatPtr(20), floatPtr(25)},
		{"dash between two labelled amounts", "$20 adv - $25 door", floatPtr(20), floatPtr(25)},
		{"set-time range after a split", "$15 adv / $20 door, 6-10pm", floatPtr(15), floatPtr(20)},
		{"venue named after the door label", "$20 adv / $25 door at the venue", floatPtr(20), floatPtr(25)},
		{"day of, unfinished", "$20 advance, $25 day of", floatPtr(20), floatPtr(25)},

		// A bound is not a price. SeeTickets serves span.price verbatim and
		// prefixes ONE sign to the whole of it, so the second bound often
		// carries none: counting signed amounts would miss exactly those. Only
		// the first row is a shape observed on a live calendar; the rest are
		// the variants that same prefixing and the same phrasings produce.
		{"tier range", "$10.00-$30.00", nil, nil},
		{"tier range with one sign", "$10.00-30.00", nil, nil},
		{"short tier range", "$20-25", nil, nil},
		{"spaced tier range", "$10 - 30", nil, nil},
		{"spelled range", "$20 to $30", nil, nil},
		{"open ended", "$20+", nil, nil},
		{"open ended spelled", "$20 and up", nil, nil},
		{"range on the advance half", "$20-25 adv / $30 door", nil, nil},
		{"bare slash pair", "$20/$25", nil, nil},

		// Every half is read from a label. Nothing becomes the advance price by
		// being the amount left over, which is what published a three dollar
		// advance for a twenty-five dollar door.
		{"door label without an advance label", "$20 / $25 door", nil, nil},
		{"fee beside a door price", "$25 at the door (plus $3 fees)", nil, nil},
		{"price plus a fee", "$20 (plus $3 fees)", nil, nil},
		{"tier ahead of the split", "$30 VIP / $20 adv / $25 door", nil, nil},

		// A figure on the door side that is smaller than the advance price is
		// an INCREMENT, not a price: "$5 more at the door" says the door costs
		// twenty-five. Storing 5 there would publish a five dollar door price.
		{"increment on the door side", "$20 advance, $5 more at the door", nil, nil},
		{"increment spelled add", "$20 advance, add $5 at the door", nil, nil},
		{"fee on the door side", "$20 adv, $2 fee at the door", nil, nil},
		{"surcharge on the door side", "$20 adv, $3 surcharge at the door", nil, nil},
		{"increment on the day of show", "$20 adv / $5 more day of show", nil, nil},

		// A door word that belongs to the doors TIME states no second price,
		// and cannot take an amount the listing calls an advance price.
		{"doors time", "$20, doors at 7", floatPtr(20), nil},
		{"doors open", "$20 doors open 8pm", floatPtr(20), nil},
		{"doors time beside a pair", "$20/$25, doors at 7", nil, nil},
		{"bare hour after doors", "$20, doors 7", floatPtr(20), nil},
		{"colon before the doors time", "$20, doors: 7pm", floatPtr(20), nil},
		{"pipe and colon", "$20 | Doors: 8pm", floatPtr(20), nil},
		{"dash before the doors time", "$20 Doors - 8pm", floatPtr(20), nil},
		{"full stop before the doors time", "$20 doors. 8pm", floatPtr(20), nil},
		{"24 hour doors time", "$20 DOORS: 7:30", floatPtr(20), nil},
		{"doors time in words", "$20, doors at seven", floatPtr(20), nil},
		{"doors time written first with a dash", "$20, 8pm - doors", floatPtr(20), nil},
		{"cover beside a punctuated doors time", "$10 cover, doors: 9pm", floatPtr(10), nil},
		{"advance price beside a doors time", "$15 advance, doors 6", floatPtr(15), nil},
		{"doors time written first", "$20, 8pm doors", floatPtr(20), nil},
		{"cover beside a doors time", "$10 cover, 9pm doors", floatPtr(10), nil},
		{"sentences", "$20 advance. Doors 7. Show 8.", floatPtr(20), nil},

		// A free word beside a figure is ambiguous, and none of these readings
		// is the show's price.
		{"free with a door price", "Free / $10 at the door", nil, nil},
		{"free with an rsvp", "Free w/ RSVP, $10 door", nil, nil},
		{"free before a time", "Free before 10pm, $10 after", nil, nil},
		{"free parking", "$20 free parking", nil, nil},

		// A lone amount goes to the half its own label names. A door-only
		// listing is a real shape, and reading it as an advance price is what
		// lets a re-scrape publish an advance dearer than the door.
		{"lone door amount", "$25 at the door", nil, floatPtr(25)},
		{"lone door word", "$25 door", nil, floatPtr(25)},
		{"lone advance amount", "$20 adv", floatPtr(20), nil},
		{"lone unlabelled amount", "$20 general admission", floatPtr(20), nil},

		// The door label reaches across a phrase, but not across a sentence.
		// Both sides of labelReach, so the constant is pinned rather than
		// merely plausible.
		{"label within reach", "$20 adv / $25 cash or card at the door", floatPtr(20), floatPtr(25)},
		{"label out of reach", "$20 adv / $25 plus a two dollar service charge at the door", nil, nil},

		// A price this parser cannot read is a reason to state none, never to
		// report a trimmed number. ParseFloat alone would take every one of
		// these; none of them is a price.
		{"digits past the bound", "$1234567", nil, nil},
		{"three decimal places", "$25.999", nil, nil},
		{"one unreadable amount refuses the string", "$20 adv / $1234567 door", nil, nil},
		{"exponent", "1e10", nil, nil},
		{"infinity", "Infinity", nil, nil},
		{"not a number", "NaN", nil, nil},
		{"negative", "-5", nil, nil},
		{"hex float", "0x1p10", nil, nil},
		{"at the digit bound", "$999999", floatPtr(999999), nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			advance, door := parseEventPrices(tc.in)
			assert.Equal(t, tc.advance, advance, "advance: got %s", priceOf(advance))
			assert.Equal(t, tc.door, door, "door: got %s", priceOf(door))
		})
	}
}

// labelledDoorAmount compares every door label against every amount, so cost
// grows with the square of the input and nothing upstream bounds the field.
// The length guard is what keeps a scraped page from spending seconds here,
// and on the create path that time would be spent inside an open transaction.
func TestParseEventPrices_RefusesAnOversizedString(t *testing.T) {
	oversized := strings.Repeat("$1 door ", 4000)
	require.Greater(t, len(oversized), maxPriceTextBytes)

	start := time.Now()
	advance, door := parseEventPrices(oversized)
	elapsed := time.Since(start)

	assert.Nil(t, advance)
	assert.Nil(t, door)
	assert.Less(t, elapsed, time.Second, "an oversized string must be refused, not parsed")
}

// The explicit-only rule, stated as a property rather than a table row: no
// input may produce a price for a half no word in it names. Inventing one would
// publish a wrong number about money, which is the failure the poster
// extraction refuses in the same words.
func TestParseEventPrices_NeverInventsAHalfNoWordNames(t *testing.T) {
	for _, in := range []string{
		"$20", "20", "$20 adv", "Free", "$20 general admission", "$20 (plus $3 fees)",
	} {
		_, door := parseEventPrices(in)
		assert.Nil(t, door, "%q names no door price and must yield none", in)
	}

	for _, in := range []string{"$25 at the door", "$25 door", "$25 day of show"} {
		advance, _ := parseEventPrices(in)
		assert.Nil(t, advance, "%q names no advance price and must yield none", in)
	}
}
