import { formatPrice } from './formatters'

/**
 * The two price columns, as every show-shaped payload on the site spells them.
 *
 * Structurally typed rather than tied to one response type on purpose: the same
 * pair arrives on `ShowResponse`, `VenueShowResponse`, `ArtistShowResponse`,
 * `SceneShowSummary` and the rails' own row type, and a shared derivation that
 * named one of them would have to be re-plumbed for every new list surface.
 *
 * THE COST, stated rather than hidden: both fields are OPTIONAL, so a payload
 * type carrying no `door_price` at all satisfies this interface and renders the
 * advance half alone — the exact under-reporting the shared derivation exists
 * to prevent, with no compile error. `door_price?` cannot be made required
 * because `SceneShowSummary` serializes it with `omitempty`. So adding a new
 * list contract means checking that it actually SERVES the column; the type
 * will not check it for you.
 */
export interface ShowPrices {
  price?: number | null
  door_price?: number | null
}

/**
 * The prices this show actually STATES, advance first: `[]`, `[35]`, or the
 * pair `[35, 40]`.
 *
 * MIRRORED in two other languages, which render the same column to the same
 * reader with no compiler holding the three together: `ShowPriceText` in
 * `backend/internal/services/shared/show_price.go` (the ICS feeds) and
 * `Show.statedPrices` in `ios/PsychicHomily/Models/Show.swift` (the iOS list
 * and detail screens). A collapse rule changed here needs the same change in
 * both.
 *
 * THE one derivation of "what does this show cost", and the reason a list and
 * the detail page cannot disagree about the answer. They render it differently
 * — a list says `$35/$40`, the detail page says `$35 ADV · DOOR $40` — but they
 * both start here, so a rule added below reaches every surface at once.
 *
 * Equal prices COLLAPSE to one. Nothing stops a curator entering the same
 * number twice (the door field's placeholder says "only if it differs", but
 * that is a hint, not a constraint, and an importer has no hint at all), and
 * `$35/$35` spends two slots and a separator to say one thing — it reads as a
 * rendering bug.
 *
 * A lone DOOR price comes back as a single entry, indistinguishable from a lone
 * advance price, and that is deliberate: with only one number there is nothing
 * to tell it apart FROM, so qualifying it would add a word without adding a
 * fact.
 *
 * Zero is a price ("Free"), not silence, which is why the guards test `!= null`
 * rather than truthiness.
 */
export function statedShowPrices(show: ShowPrices): number[] {
  const advance = show.price
  const door = show.door_price
  if (advance != null && door != null && advance !== door) {
    return [advance, door]
  }
  const only = advance ?? door
  return only != null ? [only] : []
}

/**
 * What a DENSE LIST row renders for a price, or null when the show records
 * none.
 *
 * `text` is the register (user decision, PSY-1962): `$35/$40` for a pair,
 * advance first, and a bare `$35` or `Free` for a single price. Both numbers
 * rather than the advance half alone, because a list that showed `$35` for a
 * show whose door is $40 was not merely under-reporting — it was telling a
 * reader the wrong thing about money, and they found out at the door.
 *
 * `title` is present ONLY for a pair, spelling out which number is which for a
 * reader who cannot infer it from a slash. The `ShowPrice` component renders it
 * as both `title` and `aria-label`, so a screen reader is not left announcing a
 * fact about money as "thirty five slash forty"; the detail page is where the
 * pair is qualified in words instead.
 *
 * Most callers should render {@link ShowPrice} rather than call this directly —
 * it is where the a11y half of the decision lives. Reach for this function when
 * the surface has no element to hang a label on and needs the string alone; use
 * {@link showPriceText} for that, which says so in its name.
 */
export interface ShowPriceLabel {
  text: string
  title?: string
}

export function showPriceLabel(show: ShowPrices): ShowPriceLabel | null {
  const prices = statedShowPrices(show)
  if (prices.length === 0) return null
  if (prices.length === 1) return { text: formatPrice(prices[0]) }
  const [advance, door] = prices.map(formatPrice)
  return {
    text: `${advance}/${door}`,
    title: `${advance} advance, ${door} at the door`,
  }
}

/**
 * The list register as a bare string: `$35`, `Free`, `$35/$40`, or null.
 *
 * For a surface that composes its price into a joined line and genuinely has
 * no element of its own to carry a label. The atlas venue panel is the only
 * one: it middot-joins time and price into a single string.
 *
 * THAT IS A NARROW CASE, and it was briefly written down as a wider one. The
 * scene day rows, the scene calendar and the discovery rails all render the
 * price into their own `<span>`, so they carry the label after all — the first
 * two through `ShowPrice`, the rails through `RailRowData.figureLabel`,
 * because that column also holds status tokens that are not prices.
 *
 * So: reach for this ONLY when there is no element. If there is one, use the
 * `ShowPrice` component, which keeps the label.
 */
export function showPriceText(show: ShowPrices): string | null {
  return showPriceLabel(show)?.text ?? null
}

/**
 * Whether this show states a price at all — i.e. whether `ShowPrice` will
 * render anything for it.
 *
 * For the SEPARATOR next to a price, which has to appear exactly when the price
 * does. Spelling that guard as `show.price != null` is the bug this exists to
 * prevent: a door-only show has a price to show and a null `price`, so the
 * middot vanishes and the row reads `$15 21+`.
 *
 * True for a FREE show. Zero is a price the site prints as "Free", so it needs
 * its separator like any other.
 */
export function hasStatedPrice(show: ShowPrices): boolean {
  return statedShowPrices(show).length > 0
}

/**
 * The ONE number a surface should quote when it can only carry one: the advance
 * price, falling back to the door price when no advance price is recorded.
 *
 * For `schema.org` Offers, which have a single `price` field. Without the
 * fallback a door-only show emits NO Offer at all — the builder gates the whole
 * block on a price — so a show with a perfectly well-known $15 door drops out
 * of search-result pricing entirely.
 *
 * The opposite job from {@link showPriceLabel}, and deliberately a separate
 * function rather than a mode on it: a list has room to state both facts and
 * should never pick, while an Offer has one slot and must. Folding both shapes
 * of the question into one function is how a caller ends up quoting the wrong
 * half.
 *
 * `undefined` rather than null, because that is what the JSON-LD builders drop.
 *
 * The backend sibling is `effectiveShowPriceCents` in
 * `internal/services/notification/filter_service.go`, which answers the same
 * question for the notification price cap and falls back the same way.
 */
export function offerShowPrice(show: ShowPrices): number | undefined {
  return show.price ?? show.door_price ?? undefined
}
