import { formatPrice } from './formatters'

/**
 * The two price columns, as every show-shaped payload on the site spells them.
 *
 * Structurally typed rather than tied to one response type on purpose: the same
 * pair arrives on `ShowResponse`, `VenueShowResponse`, `ArtistShowResponse`,
 * `SceneShowSummary` and the rails' own row type, and a shared derivation that
 * named one of them would have to be re-plumbed for every new list surface.
 */
export interface ShowPrices {
  price?: number | null
  door_price?: number | null
}

/**
 * The prices this show actually STATES, advance first: `[]`, `[35]`, or the
 * pair `[35, 40]`.
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
 * reader who cannot infer it from a slash. Rendered as the element's `title`
 * and `aria-label`: the slash form is unambiguous once you know the convention
 * and useless before that, and the detail page is where the pair is qualified
 * in words.
 *
 * One function returning both so a call site is one call and cannot render the
 * compact form while forgetting the description.
 */
export interface ShowPriceLabel {
  text: string
  title?: string
}

export function showPriceLabel(show: ShowPrices): ShowPriceLabel | null {
  const prices = statedShowPrices(show)
  if (prices.length === 0) return null
  if (prices.length === 1) return { text: formatPrice(prices[0]) }
  const [advance, door] = prices
  return {
    text: `${formatPrice(advance)}/${formatPrice(door)}`,
    title: `${formatPrice(advance)} advance, ${formatPrice(door)} at the door`,
  }
}
