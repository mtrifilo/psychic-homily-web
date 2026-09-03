package pipeline

import (
	"regexp"
	"strconv"
	"strings"
)

// statedAmount matches one dollar amount a listing states. The SIGN is
// required, and that is what keeps a door time or an age restriction sharing
// the string from reading as a price.
//
// This pattern is DELIBERATELY LOOSE about what the digits look like, and
// amountShape decides. A pattern strict enough to reject a malformed amount
// would match a PREFIX of it instead: "$1234567" would read as $123456 and
// "$1,20" as $1. A wrong number about money is worse than no number, so the
// whole token is captured here and refused there.
var statedAmount = regexp.MustCompile(`\$\s*((?:\d+,)*\d+(?:\.\d+)?)`)

// amountShape is the whole of what counts as a written amount, anchored so a
// malformed one is refused rather than read up to its first flaw. It also
// rejects the shapes ParseFloat alone would take -- "1e10", "Infinity", "NaN",
// "-5", "0x1p10" -- none of which is a price.
var amountShape = regexp.MustCompile(`^(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d{1,2})?$`)

// statedRange matches a span between two figures. Only the lower bound of a
// range carries a sign in some listings, so a range cannot be recognised by
// counting signed amounts: SeeTickets prefixes ONE "$" to whatever text sits in
// span.price, which turns "10.00-30.00" into "$10.00-30.00" and leaves a single
// stated amount whose value is the bottom of a tier spread. Reading it would
// tell a reader $10 about a $30 show.
var statedRange = regexp.MustCompile(`(?:-|–|—|\bto\b)\s*\$?\s*\d`)

// doorLabel matches the words a listing uses to mark an amount as the price AT
// THE DOOR. "doors open at 8" is excluded by the lookalike guard in
// nearestLabelledAmount rather than here, because Go's RE2 has no lookahead.
var doorLabel = regexp.MustCompile(`(?i)\b(?:at\s+the\s+door|doors?|d\.o\.s\.?|dos|day[\s-]of(?:[\s-]the)?[\s-]show)\b`)

// advanceLabel matches the words a listing uses to mark an amount as the price
// bought AHEAD of the night. Nothing here names a ticket TIER: "GA" and "VIP"
// say which seat, not which day, and reading a tier as the advance half is what
// would publish a three dollar advance for "$25 at the door (plus $3 fees)".
//
// The longest phrasings come FIRST so a match starts at the word nearest its
// amount, the same reason "at the door" leads doorLabel: in "$20 in advance,
// $25 ...", a match on "advance" alone sits closer to the SECOND amount.
var advanceLabel = regexp.MustCompile(`(?i)\b(?:in\s+advance|adv|advance|pre-?\s?sale|presale)\b`)

// doorLabelLookalike matches the door word used for the DOORS TIME rather than
// a price. A listing reading "$20, doors at 7" states one price, not two.
//
// It matches a CLOCK, not any digit. An age restriction is the commonest thing
// to follow a real door price ("$25 door 21+"), and refusing every digit threw
// away both halves of exactly the listings this parser exists to read.
var doorLabelLookalike = regexp.MustCompile(`(?i)^\s*(?:open|at\b|@|\d{1,2}(?::\d{2})?\s*[ap]\.?m\.?|\d{1,2}:\d{2})`)

// freeToken matches a free-admission word anywhere in the string.
var freeToken = regexp.MustCompile(`(?i)\b(?:free|no\s+cover)\b`)

// digitLetterRun matches a label written flush against its amount, as in
// "$20adv/$25door", so a space can be inserted and the word-boundary anchors on
// the label patterns can see it. A digit and a letter are both word characters,
// so there is no boundary between them for `\b` to find.
var digitLetterRun = regexp.MustCompile(`(\d)([a-zA-Z])`)

// maxPriceDigits bounds the whole-dollar part of a stated amount. Six digits
// is far above MaxShowPrice, so this is a "that is not a price" guard rather
// than a second copy of that rail.
const maxPriceDigits = 6

// maxPriceTextBytes bounds the price string this parser will look at at all.
// nearestLabelledAmount compares every label against every amount, so cost
// grows with the square of the input, and nothing between a scraped venue page
// and this function bounds the field. A price is a few dozen bytes; anything
// longer is not one, and reading none of it is the truthful answer.
const maxPriceTextBytes = 200

// labelReach bounds how far from an amount a label may sit and still describe
// it, measured in bytes of the gap between them. A label further away is
// talking about something else, and the amount goes unlabelled. Both sides of
// the bound are pinned by TestParseEventPrices_SourceShapes.
const labelReach = 24

// parseEventPrices reads the ADVANCE price and the DOOR price out of the single
// price string a scrape reports.
//
// EVERY HALF IS READ FROM A LABEL, NEVER FROM A POSITION. A split needs exactly
// two amounts, one that a word calls the door price and one that a word calls
// the advance price. Taking "the other amount" as the advance half instead
// publishes a three dollar advance for "$25 at the door (plus $3 fees)" and the
// bottom of a tier spread for "$20/$25 GA, $30 at the door".
//
// A LONE amount goes to the half its own label names, and to the advance half
// when nothing labels it. That is not cosmetic: a re-scrape reading "$30 at the
// door" against a stored $20/$25 must move the door price, and putting it in
// the advance column instead would publish "$30 advance, $25 at the door".
//
// ANYTHING ELSE READS AS NO PRICE AT ALL: a range ("$10.00-$30.00", which
// SeeTickets serves verbatim out of span.price), three or more amounts, a
// free-admission word sitting beside a figure, an amount this parser cannot
// read. Each of those is a string whose meaning is genuinely unclear, and
// silence is a truthful answer where a guess is a wrong number about money.
//
// A nil return on either half means "not stated". Zero means free, so both are
// pointers.
func parseEventPrices(s string) (advance, door *float64) {
	if len(s) > maxPriceTextBytes {
		return nil, nil
	}

	normalized := normalizePriceText(s)
	if normalized == "" {
		return nil, nil
	}

	lower := strings.ToLower(normalized)
	if lower == "free" || lower == "no cover" {
		val := 0.0
		return &val, nil
	}

	// A free word in a longer string is ambiguous: "Free / $10 at the door",
	// "Free before 10pm, $10 after" and "$20 free parking" each mean something
	// different, and none of them means the figure is the show's price.
	if freeToken.MatchString(normalized) {
		return nil, nil
	}

	if statedRange.MatchString(normalized) {
		return nil, nil
	}

	amounts := statedAmount.FindAllStringSubmatchIndex(normalized, -1)
	if len(amounts) == 0 {
		return plausibleAmount(lower), nil
	}
	if len(amounts) > 2 {
		return nil, nil
	}

	// One implausible amount refuses the whole string. A money-shaped token
	// this parser cannot read is a reason to state no price, never a reason to
	// go on and report the neighbours as if the string were understood.
	values := make([]*float64, len(amounts))
	for i, m := range amounts {
		values[i] = plausibleAmount(normalized[m[2]:m[3]])
		if values[i] == nil {
			return nil, nil
		}
	}

	doorIdx := nearestLabelledAmount(normalized, amounts, doorLabel, doorLabelLookalike)

	if len(values) == 1 {
		if doorIdx == 0 {
			return nil, values[0]
		}
		return values[0], nil
	}

	advanceIdx := nearestLabelledAmount(normalized, amounts, advanceLabel, nil)
	if doorIdx < 0 || advanceIdx < 0 || doorIdx == advanceIdx {
		return nil, nil
	}
	return values[advanceIdx], values[doorIdx]
}

// nearestLabelledAmount returns the index in amounts of the one a label
// describes, or -1 when the string labels none within labelReach. A label
// between two amounts describes the NEARER of the two, which is what separates
// "$20 adv / $25 door" from "adv $20 / door $25" without knowing the order.
//
// An exact tie resolves to the amount the label FOLLOWS, because "$20 adv" is
// the order listings write far more often than "adv $20". A tie can only put
// both halves on one amount, which parseEventPrices reads as no split rather
// than as an inverted one.
//
// lookalike, when non-nil, is tested against the text after a match and skips
// the label when it holds.
func nearestLabelledAmount(s string, amounts [][]int, label, lookalike *regexp.Regexp) int {
	best := -1
	bestDistance := labelReach + 1

	for _, at := range label.FindAllStringIndex(s, -1) {
		if lookalike != nil && lookalike.MatchString(s[at[1]:]) {
			continue
		}
		for i, m := range amounts {
			distance := 0
			switch {
			case at[1] <= m[0]:
				distance = m[0] - at[1]
			case at[0] >= m[1]:
				distance = at[0] - m[1]
			}
			if distance < bestDistance {
				bestDistance = distance
				best = i
			}
		}
	}
	return best
}

// plausibleAmount reads a matched amount as a number, or reports nil when the
// digits cannot be a price. The separators are grouping, not part of the
// number. An over-long whole part or more than two decimal places is refused
// outright rather than trimmed to fit: a trimmed price is a wrong number.
func plausibleAmount(text string) *float64 {
	if !amountShape.MatchString(text) {
		return nil
	}
	digits := strings.ReplaceAll(text, ",", "")
	whole, _, _ := strings.Cut(digits, ".")
	if len(whole) > maxPriceDigits {
		return nil
	}
	val, err := strconv.ParseFloat(digits, 64)
	if err != nil {
		return nil
	}
	return &val
}

// normalizePriceText puts a scraped price string into the one shape the
// patterns above are written against.
//
// Whitespace folding is load-bearing rather than tidy: RE2's `\s` does NOT
// cover U+00A0, which scraped venue HTML renders inside price strings, so
// "$ 20" would not match statedAmount's `\$\s*` at all and the amount
// would go unseen. strings.Fields splits on unicode.IsSpace, which does cover
// it.
//
// The space between a digit and a letter is what lets the `\b` anchors on the
// label patterns see a label written flush against its amount.
func normalizePriceText(s string) string {
	spaced := digitLetterRun.ReplaceAllString(s, "$1 $2")
	return strings.Join(strings.Fields(spaced), " ")
}
