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
 * out as punctuation — so the pair carries a spelled-out `aria-label`, and
 * `title` gives a sighted reader the same thing on hover. That pairing was
 * open-coded at seven call sites before this existed; it is now stated once.
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
  return (
    <span className={cn(className)} title={price.title} aria-label={price.title}>
      {price.text}
    </span>
  )
}
