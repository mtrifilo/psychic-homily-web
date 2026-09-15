'use client'

import { useId } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import {
  isShowTimezoneResolved,
  resolveShowTimezone,
} from '@/lib/utils/formatters'
import {
  QUICK_WINDOW_ORDER,
  QUICK_WINDOW_LABEL,
  civilDateInZone,
  isQuickWindowCurrent,
  quickWindowHref,
  quickWindowTargets,
} from '../quickWindows'

export interface QuickWindowChipsProps {
  /**
   * State code of the ONE metro the list is filtered to, or undefined when it
   * is filtered to several or to none.
   *
   * The state rather than a resolved zone, because deciding whether a state
   * even has a known zone is part of the decision this row makes: "tonight" is
   * a claim about a calendar day, and a guessed zone can name the wrong one.
   */
  metroState?: string
  /**
   * The params already on screen, which every chip href is built from. Typed
   * loosely enough to take `useSearchParams`'s readonly view unchanged.
   */
  params: URLSearchParams | { toString: () => string }
  /** The path being viewed, for the current-chip test. */
  pathname: string
  /** The run the route resolved from `?days=`, or `undefined` for no run. */
  currentDays: number | undefined
  className?: string
}

/**
 * The hairline square chip the row is built from, in both of its shapes.
 *
 * Geometry is identical between them, which is what lets the pre-hydration row
 * be replaced by links without moving anything on the page.
 */
const chipClass =
  'inline-flex items-center whitespace-nowrap rounded-[2px] border px-2 py-1 font-mono text-xs transition-colors'
const chipRestingClass = 'border-border text-muted-foreground'
const chipLinkClass = 'hover:border-primary hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
/**
 * The current chip. Weight and border carry the state as well as colour does,
 * because colour alone is not a sufficient distinction (WCAG 1.4.1).
 */
const chipCurrentClass = 'border-primary font-bold text-primary'

/**
 * The zone the windows are resolved in: the selected metro's when the list is
 * filtered to one whose state the zone map KNOWS, and the viewer's otherwise.
 *
 * A state outside that map falls through to the viewer rather than to
 * `resolveShowTimezone`'s default, which is a guess that can be a calendar day
 * out. The viewer's own zone is the better wrong answer of the two: it is at
 * least the day the reader is having.
 *
 * `null` when the runtime will not name a zone at all, which leaves the row
 * without links rather than anchored on a day nobody is in.
 */
function windowTimeZone(metroState: string | undefined): string | null {
  if (metroState && isShowTimezoneResolved(metroState)) {
    return resolveShowTimezone(metroState)
  }
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || null
  } catch {
    return null
  }
}

/**
 * Tonight, this weekend, the next seven days and this month, as four links to
 * absolute dated URLs.
 *
 * WHY THE ROW RENDERS TWICE. Which day "tonight" is depends on a clock and a
 * zone, and on the first render there is neither: the server has no viewer, and
 * a `?cities=`-less URL resolves its metro from favourites and IP geo that only
 * the browser knows. So the pre-hydration row is the same four chips as plain
 * text, and the commit after hydration replaces them with the links. Both
 * renders agree by construction (`useHydrated`), the boxes are the same size, so
 * nothing on the page moves, and no reader is ever shown a date computed from a
 * clock that is not theirs.
 *
 * The cost is stated rather than hidden: a crawler that runs no JavaScript sees
 * the words and not the links. That costs the crawl nothing here, because every
 * URL these chips reach is already linked from the month strip or from a day
 * heading in the list below.
 */
export function QuickWindowChips({
  metroState,
  params,
  pathname,
  currentDays,
  className,
}: QuickWindowChipsProps) {
  const hydrated = useHydrated()
  const labelId = useId()

  // Read once per render, and only after hydration. A date is a claim about
  // now, and the only `now` worth making it from is the reader's.
  const timeZone = hydrated ? windowTimeZone(metroState) : null
  const today = timeZone === null ? null : civilDateInZone(new Date(), timeZone)
  const targets = today === null ? null : quickWindowTargets(today)

  return (
    <nav
      aria-labelledby={labelId}
      className={cn(
        'flex items-center gap-2 overflow-x-auto sm:flex-wrap sm:overflow-x-visible',
        className
      )}
      data-testid="shows-quick-windows"
    >
      <span
        id={labelId}
        className="shrink-0 font-mono text-[10.5px] font-bold uppercase tracking-[1px] text-muted-foreground"
      >
        Jump to
      </span>
      <ul className="flex items-center gap-2 sm:flex-wrap sm:gap-y-1">
        {targets === null
          ? QUICK_WINDOW_ORDER.map(key => (
              <li key={key}>
                <span className={cn(chipClass, chipRestingClass)}>
                  {QUICK_WINDOW_LABEL[key]}
                </span>
              </li>
            ))
          : targets.map(target => {
              const isCurrent = isQuickWindowCurrent(target, pathname, currentDays)
              return (
                <li key={target.key}>
                  <Link
                    href={quickWindowHref(params, target)}
                    aria-current={isCurrent ? 'page' : undefined}
                    className={cn(
                      chipClass,
                      chipLinkClass,
                      isCurrent ? chipCurrentClass : chipRestingClass
                    )}
                    data-testid={`shows-quick-window-${target.key}`}
                  >
                    {target.label}
                  </Link>
                </li>
              )
            })}
      </ul>
    </nav>
  )
}
