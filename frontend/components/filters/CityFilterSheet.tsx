'use client'

import { useMemo, useState, type RefObject } from 'react'
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
import { useKeyboardSafeBounds } from '@/lib/hooks/common/useKeyboardSafeBounds'
import { cn } from '@/lib/utils'
import type { CityState, CityWithCount } from './CityFilters'

/** Singular and plural of whatever the page below the filter lists. */
export interface ResultNoun {
  singular: string
  plural: string
}

export interface CityFilterSheetProps {
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

function cityKey(c: CityState): string {
  return `${c.city}|${c.state}`
}

function cityLabel(c: CityState): string {
  return `${c.city}, ${c.state}`
}

/**
 * The sheet is content-sized and capped at the proportion of the screen the
 * approved frame occupies; `useKeyboardSafeBounds` narrows that cap further
 * whenever a keyboard is up.
 */
const SHEET_MAX_HEIGHT = '66dvh'

function CityFilterSheetBody({
  cities,
  selectedCities,
  onApply,
  onOpenChange,
  resultNoun,
}: Pick<
  CityFilterSheetProps,
  'cities' | 'selectedCities' | 'onApply' | 'onOpenChange' | 'resultNoun'
>) {
  // Mounted only while the sheet is open, so opening always starts from the
  // applied selection and dismissing without applying discards the edit.
  const [pending, setPending] = useState<CityState[]>(selectedCities)
  const [query, setQuery] = useState('')

  const pendingKeys = useMemo(
    () => new Set(pending.map(cityKey)),
    [pending]
  )

  const sortedCities = useMemo(
    () => [...cities].sort((a, b) => b.count - a.count),
    [cities]
  )

  const visibleCities = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return sortedCities
    return sortedCities.filter(c =>
      cityLabel(c).toLowerCase().includes(needle)
    )
  }, [sortedCities, query])

  // Every number in the sheet comes from the same per-city counts, so the
  // button's total is the sum of the rows the reader has ticked, and the sum of
  // every row when nothing is ticked.
  const pendingTotal = useMemo(() => {
    const counted =
      pendingKeys.size === 0
        ? sortedCities
        : sortedCities.filter(c => pendingKeys.has(cityKey(c)))
    return counted.reduce((sum, c) => sum + c.count, 0)
  }, [sortedCities, pendingKeys])

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
        className="flex shrink-0 justify-center pt-2 pb-1 focus-visible:outline-none"
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

      <div className="shrink-0 border-t border-border/50 px-4 pt-3 pb-4">
        <Button
          type="button"
          size="lg"
          className="w-full"
          data-testid="city-filter-sheet-apply"
          onClick={() => {
            onApply(pending)
            onOpenChange(false)
          }}
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
 */
export function CityFilterSheet({
  open,
  onOpenChange,
  triggerRef,
  ...bodyProps
}: CityFilterSheetProps) {
  const bounds = useKeyboardSafeBounds(open)

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="bottom"
        // The sheet's own handle and Close carry the dismissal, so the shared
        // corner X would be a third control over the grab handle.
        className="gap-0 overflow-hidden rounded-t-lg p-0 [&>button:last-child]:hidden"
        style={{
          bottom: bounds ? bounds.bottom : undefined,
          maxHeight: bounds
            ? `min(${SHEET_MAX_HEIGHT}, ${bounds.maxHeight}px)`
            : SHEET_MAX_HEIGHT,
        }}
        data-testid="city-filter-sheet"
        onCloseAutoFocus={event => {
          // Radix restores focus to whatever was focused at open, which a touch
          // press may never have focused at all.
          event.preventDefault()
          triggerRef.current?.focus()
        }}
      >
        <CityFilterSheetBody {...bodyProps} onOpenChange={onOpenChange} />
      </SheetContent>
    </Sheet>
  )
}
