'use client'

import Link from 'next/link'

export type EntityCardTitleDensity = 'compact' | 'comfortable' | 'expanded'

export interface EntityCardTitleProps {
  /** Entity display name. Used as the visible title, the `title=` tooltip,
   *  and (when {@link ariaLabel} is omitted) the link's accessible name. */
  name: string

  /** Destination href for the wrapping `<Link>`. The primitive always
   *  emits a single outer `<Link>` so cards stay Playwright-strict-mode
   *  safe — never nest a second `<Link>` to the same href in a card. */
  href: string

  /**
   * Density variant. Controls heading size + clamp behaviour:
   *  - `compact`: `font-medium text-sm` + `truncate` (single-line row)
   *  - `comfortable` (or omitted): `font-bold text-base` + `line-clamp-2`
   *  - `expanded`: `font-bold text-xl` + `line-clamp-2`
   *
   * Omitted defaults to comfortable-equivalent. Cards without their own
   * density toggle (RadioShowCard, RadioStationCard) leave it omitted.
   */
  density?: EntityCardTitleDensity

  /** Override for the link's accessible name. Falls back to {@link name}.
   *  Useful when the card's accessible label needs extra context (e.g.
   *  "Album Name (album)"). */
  ariaLabel?: string
}

const HEADING_CLASSES: Record<EntityCardTitleDensity, string> = {
  compact: 'font-medium text-sm truncate',
  comfortable: 'font-bold text-base line-clamp-2',
  expanded: 'font-bold text-xl line-clamp-2',
}

/**
 * Shared title block for entity cards. Renders a single outer `<Link>`
 * wrapping an `<h3>` heading with density-appropriate clamping plus a
 * `title=` tooltip fallback for names that overflow.
 *
 * The single-outer-Link contract is load-bearing: nesting two `<Link>`
 * elements with the same `href` in a card trips Playwright's strict-mode
 * `getByRole('link', {name})` resolution and breaks E2E selectors. Cards
 * needing additional inline anchors (markdown notes, attribution lines)
 * must place those OUTSIDE this primitive — see
 * `frontend/features/collections/components/CollectionItemCard.tsx` for
 * the canonical reference.
 */
export function EntityCardTitle({
  name,
  href,
  density = 'comfortable',
  ariaLabel,
}: EntityCardTitleProps) {
  const headingClass = HEADING_CLASSES[density]

  return (
    <Link
      href={href}
      className="block group"
      aria-label={ariaLabel ?? name}
    >
      <h3
        className={`${headingClass} text-foreground group-hover:text-primary transition-colors`}
        title={name}
      >
        {name}
      </h3>
    </Link>
  )
}
