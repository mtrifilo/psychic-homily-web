'use client'

import { useMemo, useRef, useState, type RefObject } from 'react'
import { Check, Search } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatCount } from '@/components/shared/paginationChrome'
import {
  KEYBOARD_INSET_VAR,
  KEYBOARD_VISIBLE_HEIGHT_VAR,
  usePinAboveSoftKeyboard,
} from '@/lib/hooks/common/usePinAboveSoftKeyboard'
import { cn } from '@/lib/utils'
import { cityKey, cityLabel } from './cityParams'
import type { CityState, CityWithCount } from './CityFilters'

/** Singular and plural of whatever the page below the filter lists. */
export interface ResultNoun {
  singular: string
  plural: string
}

export interface CityFilterSheetProps {
  /** Ordered as the rows should read; the sheet does not re-order them. */
  cities: CityWithCount[]
  /** The applied selection; the sheet opens with it and edits a copy. */
  selectedCities: CityState[]
  /** Called once, with the edited copy, when the apply button is pressed. */
  onApply: (cities: CityState[]) => void
  open: boolean
  onOpenChange: (open: boolean) => void
  resultNoun: ResultNoun
  /**
   * Increments on every open. Keys the body, so reopening inside the exit
   * animation, which keeps the subtree mounted, still starts from the applied
   * selection rather than resurrecting the abandoned edit.
   */
  openSeq: number
  /** Focus returns here on close, whatever dismissed the sheet. */
  triggerRef: RefObject<HTMLElement | null>
}

/** Share of the screen a content-sized sheet may occupy. */
const SHEET_MAX_HEIGHT = '66dvh'

/**
 * The rows: every city the page knows about, plus any selected city the page
 * has since stopped listing. Without the second half a selection that has
 * dropped out of the count list has no row, so it cannot be unticked from here
 * and the sheet shows a selection the reader cannot see.
 */
function rowsFor(
  cities: CityWithCount[],
  selectedCities: CityState[]
): CityWithCount[] {
  const listed = new Set(cities.map(cityKey))
  const orphans = selectedCities
    .filter(c => !listed.has(cityKey(c)))
    .map(c => ({ city: c.city, state: c.state, count: 0 }))
  return orphans.length === 0 ? cities : [...cities, ...orphans]
}

function CityFilterSheetBody({
  cities,
  selectedCities,
  onApply,
  resultNoun,
}: Pick<
  CityFilterSheetProps,
  'cities' | 'selectedCities' | 'onApply' | 'resultNoun'
>) {
  const [pending, setPending] = useState<CityState[]>(selectedCities)
  const [query, setQuery] = useState('')

  const pendingKeys = useMemo(() => new Set(pending.map(cityKey)), [pending])

  const rows = useMemo(
    () => rowsFor(cities, selectedCities),
    [cities, selectedCities]
  )

  const visibleRows = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return rows
    return rows.filter(c => cityLabel(c).toLowerCase().includes(needle))
  }, [rows, query])

  // Every number in the sheet comes from the same per-city counts, so the
  // button's total is the sum of the rows the reader has ticked, and the sum of
  // every row when nothing is ticked.
  const pendingTotal = useMemo(() => {
    const counted =
      pendingKeys.size === 0 ? rows : rows.filter(c => pendingKeys.has(cityKey(c)))
    return counted.reduce((sum, c) => sum + c.count, 0)
  }, [rows, pendingKeys])

  const toggleCity = (city: CityWithCount) => {
    const key = cityKey(city)
    setPending(prev =>
      prev.some(c => cityKey(c) === key)
        ? prev.filter(c => cityKey(c) !== key)
        : [...prev, { city: city.city, state: city.state }]
    )
  }

  const applyLabel = `Show ${formatCount(pendingTotal)} ${
    pendingTotal === 1 ? resultNoun.singular : resultNoun.plural
  }`

  return (
    <>
      <SheetClose
        // Narrow enough that the strip above the title is not a dismiss target,
        // and tall enough to be one where it is drawn.
        className="mx-auto flex w-16 shrink-0 items-center justify-center rounded-md py-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        data-testid="city-filter-sheet-handle"
      >
        <span
          className="h-[3px] w-9 rounded-full bg-muted-foreground/70"
          aria-hidden
        />
        <span className="sr-only">Close filter by city</span>
      </SheetClose>

      <div className="flex shrink-0 items-center justify-between gap-2 px-4 pb-2">
        <SheetTitle className="text-base">Filter by city</SheetTitle>
        <SheetDescription className="sr-only">
          Pick one or more cities, then apply them to the list.
        </SheetDescription>
        <SheetClose
          className="-my-1 rounded-md px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          data-testid="city-filter-sheet-close"
        >
          Close
        </SheetClose>
      </div>

      <div className="shrink-0 px-4 pb-2">
        <div className="flex h-10 items-center gap-2 rounded-md border border-border/50 bg-muted/40 px-3 focus-within:ring-2 focus-within:ring-ring">
          <Search className="size-4 shrink-0 opacity-50" aria-hidden />
          <input
            type="search"
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Search cities..."
            aria-label="Search cities"
            data-testid="city-filter-sheet-search"
            className="h-full w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
        </div>
      </div>

      {/* Typing narrows the list and can empty it without moving focus, so the
          result of the search is announced rather than only drawn. */}
      <p
        role="status"
        className="sr-only"
        data-testid="city-filter-sheet-status"
      >
        {visibleRows.length === 0
          ? 'No cities found.'
          : `${formatCount(visibleRows.length)} cities. ${applyLabel}.`}
      </p>

      <div
        role="group"
        aria-label="Cities"
        className="min-h-0 flex-1 overflow-y-auto px-4"
        data-testid="city-filter-sheet-list"
      >
        {visibleRows.length === 0 ? (
          <p
            aria-hidden
            className="py-6 text-center text-sm text-muted-foreground"
            data-testid="city-filter-sheet-empty"
          >
            No cities found.
          </p>
        ) : (
          visibleRows.map(city => {
            const key = cityKey(city)
            const isPending = pendingKeys.has(key)
            return (
              <label
                key={key}
                className={cn(
                  'flex h-[38px] cursor-pointer items-center gap-2 rounded-md px-2 text-sm',
                  'has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring',
                  isPending && 'bg-secondary font-medium'
                )}
              >
                <input
                  type="checkbox"
                  className="sr-only"
                  checked={isPending}
                  onChange={() => toggleCity(city)}
                  // The visible name and count beside it are decorative; the
                  // control's own name carries both, with the count's unit.
                  aria-label={`${cityLabel(city)}, ${formatCount(city.count)} ${
                    city.count === 1 ? resultNoun.singular : resultNoun.plural
                  }`}
                  data-testid={`city-sheet-option-${city.city}-${city.state}`
                    .toLowerCase()
                    .replace(/\s+/g, '-')}
                />
                <Check
                  className={cn(
                    'size-4 shrink-0 text-primary',
                    isPending ? 'opacity-100' : 'opacity-0'
                  )}
                  aria-hidden
                />
                <span className="flex-1 truncate" aria-hidden>
                  {cityLabel(city)}
                </span>
                <span
                  className="shrink-0 tabular-nums text-muted-foreground"
                  aria-hidden
                >
                  {formatCount(city.count)}
                </span>
              </label>
            )
          })
        )}
      </div>

      {/* The sheet's own bottom padding is dropped by the `p-0` below, so the
          only control in reach of the home indicator adds the inset back. */}
      <div className="shrink-0 border-t border-border/50 px-4 pt-3 pb-[calc(1rem+env(safe-area-inset-bottom))]">
        <Button
          type="button"
          size="lg"
          className="w-full"
          data-testid="city-filter-sheet-apply"
          onClick={() => onApply(pending)}
        >
          {applyLabel}
        </Button>
      </div>
    </>
  )
}

/**
 * The touch-viewport form of the city filter: a bottom sheet whose search field
 * stays pinned while the city list scrolls under it, and whose one apply button
 * carries the total its current ticks would show.
 *
 * The rows are native checkboxes rather than the `Command` items the popover
 * uses, because `aria-selected` on a cmdk item tracks its highlight and this
 * list needs it for its ticks. The cost is that this list filters by substring
 * where the popover filters by cmdk's score.
 */
export function CityFilterSheet({
  open,
  onOpenChange,
  triggerRef,
  onApply,
  openSeq,
  ...bodyProps
}: CityFilterSheetProps) {
  const contentRef = useRef<HTMLDivElement | null>(null)
  usePinAboveSoftKeyboard(open, contentRef)

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        ref={contentRef}
        side="bottom"
        // The handle and the header's Close carry dismissal, so the shared
        // corner X would be a third control sitting over the grab handle.
        showCloseButton={false}
        tabIndex={-1}
        className="gap-0 overflow-hidden rounded-t-lg p-0 pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)]"
        style={{
          bottom: `var(${KEYBOARD_INSET_VAR}, 0px)`,
          maxHeight: `min(${SHEET_MAX_HEIGHT}, var(${KEYBOARD_VISIBLE_HEIGHT_VAR}, 100dvh))`,
        }}
        data-testid="city-filter-sheet"
        onOpenAutoFocus={event => {
          // The first tabbable child is a dismiss control, and landing on it
          // puts Enter one keystroke from discarding the edit.
          event.preventDefault()
          contentRef.current?.focus()
        }}
        onCloseAutoFocus={event => {
          // Radix restores focus to whatever was focused at open, which a touch
          // press may never have focused at all.
          event.preventDefault()
          triggerRef.current?.focus()
        }}
      >
        <CityFilterSheetBody
          key={openSeq}
          {...bodyProps}
          onApply={cities => {
            onApply(cities)
            onOpenChange(false)
          }}
        />
      </SheetContent>
    </Sheet>
  )
}
