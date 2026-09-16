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
  /** Focus returns here on close, whatever dismissed the sheet. */
  triggerRef: RefObject<HTMLElement | null>
}

/** Share of the screen a content-sized sheet may occupy. */
const SHEET_MAX_HEIGHT = '66dvh'

function CityFilterSheetBody({
  cities,
  selectedCities,
  onApply,
  resultNoun,
}: Pick<
  CityFilterSheetProps,
  'cities' | 'selectedCities' | 'onApply' | 'resultNoun'
>) {
  // Mounted only while the sheet is open, so opening always starts from the
  // applied selection and dismissing without applying discards the edit.
  const [pending, setPending] = useState<CityState[]>(selectedCities)
  const [query, setQuery] = useState('')

  const pendingKeys = useMemo(() => new Set(pending.map(cityKey)), [pending])

  const visibleCities = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return cities
    return cities.filter(c => cityLabel(c).toLowerCase().includes(needle))
  }, [cities, query])

  // Every number in the sheet comes from the same per-city counts, so the
  // button's total is the sum of the rows the reader has ticked, and the sum of
  // every row when nothing is ticked.
  const pendingTotal = useMemo(() => {
    const counted =
      pendingKeys.size === 0
        ? cities
        : cities.filter(c => pendingKeys.has(cityKey(c)))
    return counted.reduce((sum, c) => sum + c.count, 0)
  }, [cities, pendingKeys])

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
        // Radix focuses the first tabbable child on open, which is this, so it
        // owes a visible ring as much as any other control.
        className="flex shrink-0 justify-center rounded-md pt-2 pb-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        data-testid="city-filter-sheet-handle"
      >
        <span className="h-[3px] w-9 rounded-full bg-border" aria-hidden />
        <span className="sr-only">Close filter by city</span>
      </SheetClose>

      <div className="flex shrink-0 items-center justify-between gap-2 px-4 pb-2">
        <SheetTitle className="text-base">Filter by city</SheetTitle>
        <SheetDescription className="sr-only">
          Pick one or more cities, then apply them to the list.
        </SheetDescription>
        <SheetClose
          className="rounded-md text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          data-testid="city-filter-sheet-close"
        >
          Close
        </SheetClose>
      </div>

      <div className="shrink-0 px-4 pb-2">
        <div className="flex h-10 items-center gap-2 rounded-md border border-border/50 bg-muted/40 px-3 focus-within:border-border">
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

      <div
        className="min-h-0 flex-1 overflow-y-auto px-4"
        data-testid="city-filter-sheet-list"
      >
        {visibleCities.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            No cities found.
          </p>
        ) : (
          visibleCities.map(city => {
            const key = cityKey(city)
            const isPending = pendingKeys.has(key)
            return (
              <label
                key={key}
                className={cn(
                  'flex h-[38px] cursor-pointer items-center gap-2 rounded-md px-2 text-sm',
                  'has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring',
                  isPending && 'bg-secondary text-primary'
                )}
              >
                <input
                  type="checkbox"
                  className="sr-only"
                  checked={isPending}
                  onChange={() => toggleCity(city)}
                  // The visible count beside the name is decorative; the
                  // control's own name carries it with its unit.
                  aria-label={`${cityLabel(city)}, ${formatCount(city.count)} ${
                    city.count === 1 ? resultNoun.singular : resultNoun.plural
                  }`}
                  data-testid={`city-sheet-option-${city.city}-${city.state}`
                    .toLowerCase()
                    .replace(/\s+/g, '-')}
                />
                <Check
                  className={cn(
                    'size-4 shrink-0',
                    isPending ? 'opacity-100' : 'opacity-0'
                  )}
                  aria-hidden
                />
                <span className="flex-1 truncate">{cityLabel(city)}</span>
                <span
                  className={cn(
                    'shrink-0 tabular-nums',
                    isPending ? 'text-primary' : 'text-muted-foreground'
                  )}
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
 * uses: cmdk drives `aria-selected` from its own highlight, which a
 * multi-select list needs for its ticks. The cost is that this list filters by
 * substring where the popover filters by cmdk's score.
 */
export function CityFilterSheet({
  open,
  onOpenChange,
  triggerRef,
  onApply,
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
        className="gap-0 overflow-hidden rounded-t-lg p-0"
        style={{
          bottom: `var(${KEYBOARD_INSET_VAR}, 0px)`,
          maxHeight: `min(${SHEET_MAX_HEIGHT}, var(${KEYBOARD_VISIBLE_HEIGHT_VAR}, 100dvh))`,
        }}
        data-testid="city-filter-sheet"
        onCloseAutoFocus={event => {
          // Radix restores focus to whatever was focused at open, which a touch
          // press may never have focused at all.
          event.preventDefault()
          triggerRef.current?.focus()
        }}
      >
        <CityFilterSheetBody
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
