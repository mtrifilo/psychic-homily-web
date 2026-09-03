package pipeline

import (
	"regexp"
	"strconv"
	"strings"
)

// statedAmount matches one dollar amount a listing states. The optional space
// after the sign is deliberate: scraped price text renders "$ 20" often enough
// to matter, and the sign is what separates an amount from a door time or an
// age restriction sharing the same string.
//
// The thousands-separated alternative comes FIRST and is not optional grouping:
// without it "$1,200" matches only "$1", and a hundredfold understatement of a
// price is worse than reading none at all.
var statedAmount = regexp.MustCompile(`\$\s*(\d{1,3}(?:,\d{3})+(?:\.\d{1,2})?|\d{1,6}(?:\.\d{1,2})?)`)

// doorLabel matches the words a listing uses to mark an amount as the price AT
// THE DOOR. "doors open at 8" is excluded by the lookalike guard in
// labelledDoorAmount rather than here, because Go's RE2 has no lookahead.
var doorLabel = regexp.MustCompile(`(?i)\b(?:at\s+the\s+door|doors?|day[\s-]of[\s-]show)\b`)

// doorLabelLookalike matches the door word used for the DOORS TIME rather than
// a price. A listing reading "$20, doors at 7" states one price, not two.
var doorLabelLookalike = regexp.MustCompile(`(?i)^\s*(?:open|at\b|@|\d)`)

// doorLabelReach bounds how far from an amount a door label may sit and still
// describe it, in bytes of the gap between them. "$20 presale, $25 at the door"
// puts eight characters between the amount and the word.
const doorLabelReach = 24

// parseEventPrices reads the ADVANCE price and the DOOR price out of the single
// price string a scrape reports.
//
// A listing states one price ("$18"), no price, "free", or two prices with the
// door half labelled ("$20 adv / $25 door", "$20 presale, $25 at the door").
//
// SEVERAL AMOUNTS WITH NO DOOR LABEL YIELD NOTHING. They are a ticket-tier
// range -- SeeTickets serves "$10.00-$30.00" verbatim out of span.price -- and
// neither bound is the cost of getting in, so publishing one would tell a
// reader $10 about a $30 show. No amount becomes a door price without a word
// naming it one, the rule the poster extraction follows in the same words (see
// extractionSystemPrompt).
//
// A nil return on either half means "not stated". Zero means free, so both are
// pointers.
func parseEventPrices(s string) (advance, door *float64) {
	normalized := normalizePriceWhitespace(s)
	if normalized == "" {
		return nil, nil
	}

	lower := strings.ToLower(normalized)
	if lower == "free" || lower == "no cover" {
		val := 0.0
		return &val, nil
	}

	amounts := statedAmount.FindAllStringSubmatchIndex(normalized, -1)
	if len(amounts) == 0 {
		// A bare number with no sign, which some listings serve.
		val, err := strconv.ParseFloat(lower, 64)
		if err != nil {
			return nil, nil
		}
		return &val, nil
	}

	// One stated amount is the show's price whatever word sits beside it.
	// "$25 at the door" and "$20 adv" each name one number, and every register
	// renders a lone price the same way regardless of which column holds it.
	if len(amounts) == 1 {
		return amountAt(normalized, amounts[0]), nil
	}

	doorIdx := labelledDoorAmount(normalized, amounts)
	if doorIdx < 0 {
		return nil, nil
	}

	advanceIdx := 0
	if doorIdx == 0 {
		advanceIdx = 1
	}
	return amountAt(normalized, amounts[advanceIdx]), amountAt(normalized, amounts[doorIdx])
}

// labelledDoorAmount returns the index in amounts of the one a door label
// describes, or -1 when the string labels none. A label between two amounts
// describes the nearer of the two, which is what separates "$20 adv / $25 door"
// from "adv $20 / door $25" without needing to know the order.
func labelledDoorAmount(s string, amounts [][]int) int {
	best := -1
	bestDistance := doorLabelReach + 1

	for _, label := range doorLabel.FindAllStringIndex(s, -1) {
		if doorLabelLookalike.MatchString(s[label[1]:]) {
			continue
		}
		for i, m := range amounts {
			distance := 0
			switch {
			case label[1] <= m[0]:
				distance = m[0] - label[1]
			case label[0] >= m[1]:
				distance = label[0] - m[1]
			}
			if distance < bestDistance {
				bestDistance = distance
				best = i
			}
		}
	}
	return best
}

// amountAt reads the capture group of one statedAmount match. The separators
// statedAmount admits in a thousands-grouped amount are not part of the number.
func amountAt(s string, match []int) *float64 {
	val, err := strconv.ParseFloat(strings.ReplaceAll(s[match[2]:match[3]], ",", ""), 64)
	if err != nil {
		return nil
	}
	return &val
}

// normalizePriceWhitespace folds every kind of space a scrape can carry into a
// single plain one and trims the ends. strings.Fields splits on unicode.IsSpace,
// so it covers the U+00A0 that scraped venue HTML renders inside price strings,
// which would otherwise put a door label out of doorLabelReach of its amount.
func normalizePriceWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
