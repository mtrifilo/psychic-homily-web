package shared

import (
	"fmt"
	"math"
)

// ShowPriceText renders a show's advance/door pair as the site's dense register:
// "$35/$40" for a pair, a bare "$35" or "Free" for a single price, "" when no
// price is recorded (PSY-1962).
//
// EXPORTED AND SHARED on purpose. The rule stated on catalog.Show.Price is that
// a surface must render the pair through the common derivation rather than
// reading Price alone, and a rule pointing at an unexported helper is a rule
// nobody can follow. This is the Go half of it; the frontend half is
// lib/utils/showPrice.ts, and the two spell the same register.
//
// It lives here rather than in the one service that first needed it because
// "what does this show cost" is not calendar-specific. The ICS feeds are the
// current caller and the worst place to get it wrong -- a calendar entry
// OUTLIVES a page view, sitting in a subscriber's phone until the event passes,
// so serving the advance half alone left a reader budgeting $35 for a $40 door
// with no way to have known.
//
// The collapse rules mirror the show page's ticket line exactly, and for the
// same reasons. Equal numbers spend two slots saying one thing, so they collapse.
// A lone door price renders bare, because with one number there is nothing to
// tell it apart FROM. Zero is a price ("Free"), not silence, which is why the
// guards test nil rather than truthiness.
//
// NOT the register for a surface that must reduce the pair to ONE number. The
// notification price-cap filter is the standing example: it judges a show by the
// ADVANCE price (see effectiveShowPriceCents), because a filter has to pick, and
// this function exists precisely to avoid picking.
func ShowPriceText(price, doorPrice *float64) string {
	if price != nil && doorPrice != nil && *price != *doorPrice {
		return showPriceAmount(*price) + "/" + showPriceAmount(*doorPrice)
	}
	only := price
	if only == nil {
		only = doorPrice
	}
	if only == nil {
		return ""
	}
	return showPriceAmount(*only)
}

// showPriceAmount is one number in the site's money register: "Free" for zero,
// "$35" for a whole amount, "$12.50" when there really are cents.
//
// It must stay byte-identical to formatPrice in frontend/lib/utils/formatters.ts,
// because the two render THE SAME COLUMN to the same reader — the calendar entry
// a subscriber keeps and the /shows row they saw it on. That is a duplication
// across languages with no compiler holding it together, so the rule is stated
// on both sides and each names the other by path.
//
// The whole-number test is what keeps them in step, and it is easy to get wrong:
// a bare "$%.0f" (which is what the ICS description used before PSY-1962) prints
// $12.50 as "$12" and, worse, prints $0.50 as "$0" — a fifty-cent door published
// to a subscriber's calendar as free.
func showPriceAmount(v float64) string {
	if v == 0 {
		return "Free"
	}
	if v == math.Trunc(v) {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}
