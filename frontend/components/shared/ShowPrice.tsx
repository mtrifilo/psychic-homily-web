import { cn } from '@/lib/utils'
import { showPriceLabel, type ShowPrices } from '@/lib/utils/showPrice'

export interface ShowPriceProps {
  show: ShowPrices
  /**
   * Rendered when the show records no price at all. Omit to render nothing —
   * a table cell wants an en dash holding the column open, an inline meta row
   * usually wants silence.
   */
  fallback?: React.ReactNode
  className?: string
}

/**
 * A show's price as every dense list on the site spells it: `$35`, `Free`, or
 * the advance/door pair `$35/$40` (PSY-1962).
 *
 * ONE component rather than a span per surface, because the a11y half of the
 * decision is easy to forget and impossible to notice missing. A screen reader
 * announces `$35/$40` as "thirty five slash forty" — a fact about money read
 * out as punctuation — so the pair hides the glyphs from the accessibility tree
 * and offers the spelled-out reading instead, while `title` gives a sighted
 * reader the same thing on hover.
 *
 * A VISUALLY-HIDDEN SIBLING, not `aria-label`. That was the first attempt and it
 * does not work: a bare `<span>` has the `generic` role, for which ARIA
 * PROHIBITS a name from the author, so browsers ignore the attribute and read
 * the visible glyphs anyway — the failure this component exists to prevent,
 * silently un-prevented. `aria-hidden` plus `sr-only` is the pattern the rest of
 * this codebase uses (Pagination, BottomTabBar) and it needs no role to work.
 *
 * Assert it with `toHaveAccessibleName`, never with `toHaveAttribute`: an
 * attribute assertion passes against exactly the broken version.
 *
 * BOTH the split register and that label are NEW (PSY-1962). Before them these
 * surfaces rendered `formatPrice(show.price)` with no label and no second
 * number, so this is not a de-duplication of something that already worked —
 * it is the one place a brand-new rule is written down, before it can be
 * restated eight times and forgotten on the ninth.
 *
 * The label is present ONLY for a pair. A lone price has nothing to be
 * disambiguated from, and an `aria-label` restating the visible text would make
 * a screen reader say it twice.
 *
 * WHICH prices there are to spell is {@link showPriceLabel}, shared with the
 * show page's own ticket line, so a reader who scans a list and opens a show is
 * never shown two different prices for it. This component owns only the markup.
 */
export function ShowPrice({ show, fallback, className }: ShowPriceProps) {
  const price = showPriceLabel(show)
  if (!price) return <>{fallback ?? null}</>
  if (!price.title) return <span className={cn(className)}>{price.text}</span>
  return (
    <span className={cn(className)} title={price.title}>
      <span aria-hidden="true">{price.text}</span>
      <span className="sr-only">{price.title}</span>
    </span>
  )
}
