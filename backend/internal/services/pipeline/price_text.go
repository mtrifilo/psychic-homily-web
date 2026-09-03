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

// rangedOrOpenEnded matches a figure that is a BOUND rather than a price,
// tested against the text immediately following an amount.
//
// A range cannot be recognised by counting signed amounts: SeeTickets prefixes
// ONE "$" to whatever text sits in span.price, so "10.00-30.00" arrives as
// "$10.00-30.00" and leaves a single stated amount worth the bottom of a tier
// spread. Anchoring the test to one amount's own trailing text is what tells
// that apart from a dash SEPARATING two labelled prices, which is not a range.
//
// "$20+" and "$20 and up" state a floor by the same logic. The "+" has to sit
// flush against the amount, or the age restriction in "$25 door 21+" would read
// as one.
var rangedOrOpenEnded = regexp.MustCompile(`(?i)^\s*(?:(?:-|–|—|to\b)\s*\$?\s*\d|\+|(?:and|&)\s+up\b)`)

// doorMatcher marks an amount as the price AT THE DOOR.
//
// skipAfter and skipBefore are what separate the price label from the DOORS
// TIME, which is the same word: "$20, doors at 7", "$20, doors: 7pm" and
// "$20, 8pm doors" each state one price. Both guards step over the punctuation
// a listing writes between the word and the clock, or "doors: 7pm" reads as a
// door price of twenty dollars.
//
// The forward guard matches a clock or a bare hour but NOT a figure carrying a
// "+", because an age restriction is the commonest thing to follow a real door
// price and refusing every digit threw away both halves of exactly the
// listings this parser exists to read.
var doorMatcher = labelMatcher{
	pattern:    regexp.MustCompile(`(?i)\b(?:at\s+the\s+door|doors?|d\.o\.s\.?|dos|day[\s-]of[\s-]the[\s-]show|day[\s-]of[\s-]show|day[\s-]of)\b`),
	skipAfter:  regexp.MustCompile(`(?i)^(?:[\s:.,;|(-]*(?:open|@|\d{1,2}(?::\d{2})?\s*[ap]\.?m\.?|\d{1,2}:\d{2})|\s*(?:at\s+(?:\d|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\b|\d{1,2}(?:[^\d+]|$)))`),
	skipBefore: regexp.MustCompile(`(?i)(?:\d{1,2}(?::\d{2})?\s*[ap]\.?m\.?|\d{1,2}:\d{2})[\s:.,;|-]*$`),
}

// advanceMatcher marks an amount as the price bought AHEAD of the night.
// Nothing in it names a ticket TIER: "GA" and "VIP" say which seat, not which
// day, and reading a tier as the advance half is what would publish a three
// dollar advance for "$25 at the door (plus $3 fees)".
//
// The longest phrasings come FIRST so a match starts at the word nearest its
// amount, the same reason "at the door" leads the door pattern.
var advanceMatcher = labelMatcher{
	pattern: regexp.MustCompile(`(?i)\b(?:in\s+advance|advance|adv|pre-?\s?sale|presale)\b`),
}

// labelMatcher finds a label and reports how far it sits from each stated
// amount. skipAfter and skipBefore, when set, reject a match whose surrounding
// text shows the word is not labelling a price.
type labelMatcher struct {
	pattern    *regexp.Regexp
	skipAfter  *regexp.Regexp
	skipBefore *regexp.Regexp
}

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
// A LONE amount goes to the half its own NEAREST label names, and to the
// advance half when nothing labels it. That is not cosmetic: a re-scrape
// reading "$30 at the door" against a stored $20/$25 must move the door price,
// and putting it in the advance column instead would publish "$30 advance, $25
// at the door".
//
// ANYTHING ELSE READS AS NO PRICE AT ALL: a bound rather than a price
// ("$10.00-$30.00", "$20+"), three or more amounts, a free-admission word
// sitting beside a figure, an amount this parser cannot read, a pair of labels
// whose assignment to two amounts is a tie. Each of those is a string whose
// meaning is genuinely unclear, and silence is a truthful answer where a guess
// is a wrong number about money.
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

	advanceDist := advanceMatcher.distances(normalized, amounts)
	doorDist := doorMatcher.distances(normalized, amounts)

	if len(values) == 1 {
		if rangedOrOpenEnded.MatchString(normalized[amounts[0][1]:]) {
			return nil, nil
		}
		// The amount's OWN nearest label decides its column, so a doors time
		// sitting beside a price cannot claim an amount the listing calls an
		// advance price. An unlabelled amount is the show's price.
		if doorDist[0] < advanceDist[0] {
			return nil, values[0]
		}
		return values[0], nil
	}

	// Two amounts, two labels, and each label takes a DIFFERENT amount. The
	// assignment with the smaller total distance wins, which reads
	// "$20 adv / $25 door" and "adv $20 / door $25" alike without knowing the
	// order and without a per-label tie deciding anything on its own. An exact
	// tie between the two assignments is ambiguous and states no split.
	advanceIdx, doorIdx, ok := assignLabels(advanceDist, doorDist)
	if !ok {
		return nil, nil
	}
	if rangedOrOpenEnded.MatchString(normalized[amounts[advanceIdx][1]:]) ||
		rangedOrOpenEnded.MatchString(normalized[amounts[doorIdx][1]:]) {
		return nil, nil
	}
	// An advance price ABOVE the door price is not a split any listing states:
	// the door half exists to say the price goes UP on the night. Reaching that
	// pair means the second figure was an INCREMENT, not a price, which is what
	// "$20 advance, $5 more at the door" says: the door costs twenty-five.
	if *values[advanceIdx] > *values[doorIdx] {
		return nil, nil
	}
	return values[advanceIdx], values[doorIdx]
}

// assignLabels pairs the advance label with one amount and the door label with
// the other, choosing the pairing whose distances total less. It reports false
// when neither pairing has both labels within reach, or when the two tie.
func assignLabels(advanceDist, doorDist []int) (advanceIdx, doorIdx int, ok bool) {
	inReach := func(d int) bool { return d <= labelReach }
	firstIsAdvance := inReach(advanceDist[0]) && inReach(doorDist[1])
	firstIsDoor := inReach(advanceDist[1]) && inReach(doorDist[0])

	switch {
	case firstIsAdvance && firstIsDoor:
		switch {
		case advanceDist[0]+doorDist[1] < advanceDist[1]+doorDist[0]:
			return 0, 1, true
		case advanceDist[1]+doorDist[0] < advanceDist[0]+doorDist[1]:
			return 1, 0, true
		default:
			return 0, 0, false
		}
	case firstIsAdvance:
		return 0, 1, true
	case firstIsDoor:
		return 1, 0, true
	default:
		return 0, 0, false
	}
}

// distances reports, for each stated amount, how far the nearest usable
// occurrence of this label sits from it, or labelReach+1 when none is in reach.
func (lm labelMatcher) distances(s string, amounts [][]int) []int {
	out := make([]int, len(amounts))
	for i := range out {
		out[i] = labelReach + 1
	}

	for _, at := range lm.pattern.FindAllStringIndex(s, -1) {
		if lm.skipAfter != nil && lm.skipAfter.MatchString(s[at[1]:]) {
			continue
		}
		if lm.skipBefore != nil && lm.skipBefore.MatchString(s[:at[0]]) {
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
			if distance < out[i] {
				out[i] = distance
			}
		}
	}
	return out
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
