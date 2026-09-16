'use client'

import { useState, useMemo, useCallback, useRef } from 'react'
import { Search, Check, ChevronsUpDown } from 'lucide-react'
import { Command, CommandInput, CommandList, CommandEmpty, CommandGroup, CommandItem } from '@/components/ui/command'
import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover'
import { RemovableFilterChip } from './RemovableFilterChip'
import { cityKey, cityLabel } from './cityParams'
import { CityFilterSheet, type ResultNoun } from './CityFilterSheet'
import { useSoftKeyboardViewport } from '@/lib/hooks/common/useSoftKeyboardViewport'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { formatCount } from '@/components/shared/paginationChrome'
import { cn } from '@/lib/utils'

/**
 * Generic interface for city data with a count.
 * Both venues and shows can map their specific types to this interface.
 */
export interface CityWithCount {
  city: string
  state: string
  count: number
  /**
   * Geocoded city centroid (PSY-981), surfaced by `/shows/cities`. Optional:
   * the venue city list doesn't carry it, and the geocoder misses some small
   * cities. When present on the show-cities list it lets `useGeoDefaultCity`
   * pick the NEAREST has-shows city for a visitor whose exact city has no
   * shows. Absent → that city is simply skipped as a distance candidate.
   */
  latitude?: number
  longitude?: number
}

/** A city+state pair used for multi-select filtering */
export interface CityState {
  city: string
  state: string
}

interface CityFiltersProps {
  cities: CityWithCount[]
  selectedCities: CityState[]
  onFilterChange: (cities: CityState[]) => void
  /**
   * What the page below this filter lists, for the bottom sheet's apply
   * button. Every surface names its own rows, so there is no default.
   */
  resultNoun: ResultNoun
  allLabel?: string
  children?: React.ReactNode
}

/** Minimum number of cities with 2+ items to show the popular row */
const MIN_POPULAR_CITIES = 3
/** Maximum popular cities to show */
const MAX_POPULAR_CITIES = 5
/** Minimum count for a city to be "popular" */
const MIN_POPULAR_COUNT = 2

export function CityFilters({
  cities,
  selectedCities,
  onFilterChange,
  resultNoun,
  allLabel = 'All Cities',
  children,
}: CityFiltersProps) {
  const [open, setOpen] = useState(false)
  // Bumped on every open, so the sheet body remounts even when a reopen lands
  // inside the exit animation that still has the old one mounted.
  const [openSeq, setOpenSeq] = useState(0)
  // Decides which overlay the trigger opens, and with it the trigger's own ARIA
  // contract, so it is subscribed rather than read at open: a reader has to be
  // able to trust `aria-haspopup` before pressing.
  const softKeyboardViewport = useSoftKeyboardViewport()
  const triggerRef = useRef<HTMLButtonElement | null>(null)

  // Crossing the breakpoint swaps which overlay exists. Closing on the flip is
  // what stops an open sheet from being replaced, mid-edit, by a popover
  // carrying the applied selection instead of the edited one.
  const [overlayViewport, setOverlayViewport] = useState(softKeyboardViewport)
  if (overlayViewport !== softKeyboardViewport) {
    setOverlayViewport(softKeyboardViewport)
    setOpen(false)
  }

  const handleOpenChange = useCallback((next: boolean) => {
    if (next) setOpenSeq(seq => seq + 1)
    setOpen(next)
  }, [])

  // Composes the two halves of `replayOnHydrate` with a ref of our own: the
  // spread below sets the marker attribute, and dropping the replay ref while
  // keeping the attribute would leave a control that looks adopted and still
  // drops pre-hydration clicks.
  const setTriggerRef = useCallback((node: HTMLButtonElement | null) => {
    triggerRef.current = node
    replayOnHydrate.ref(node)
  }, [])

  const selectedSet = useMemo(
    () => new Set(selectedCities.map(cityKey)),
    [selectedCities]
  )

  // Cities sorted by count descending for the dropdown
  const sortedCities = useMemo(
    () => [...cities].sort((a, b) => b.count - a.count),
    [cities]
  )

  // Popular cities: top N with count >= threshold
  const popularCities = useMemo(() => {
    const eligible = sortedCities.filter(c => c.count >= MIN_POPULAR_COUNT)
    if (eligible.length < MIN_POPULAR_CITIES) return []
    return eligible.slice(0, MAX_POPULAR_CITIES)
  }, [sortedCities])

  const handleToggleCity = (city: CityWithCount) => {
    const key = cityKey(city)
    if (selectedSet.has(key)) {
      onFilterChange(selectedCities.filter(c => cityKey(c) !== key))
    } else {
      onFilterChange([...selectedCities, { city: city.city, state: city.state }])
    }
  }

  const handleRemoveCity = (city: CityState) => {
    onFilterChange(selectedCities.filter(c => cityKey(c) !== cityKey(city)))
  }

  const handleClearAll = () => {
    onFilterChange([])
  }

  const handlePopularClick = (city: CityWithCount) => {
    handleToggleCity(city)
  }

  // ONE tree shape across the viewport flip. The trigger ships in server HTML
  // with the popover contract, and swapping the element out for the sheet's
  // would detach the node a pre-hydration click was buffered against, which
  // `consumePendingReplay` drops. Same element, different ARIA.
  const overlayAria: React.ComponentProps<'button'> = softKeyboardViewport
    ? {
        'aria-haspopup': 'dialog',
        'aria-expanded': open,
        // The popover content the trigger would otherwise point at is never
        // mounted on a touch viewport; the sheet names itself as a dialog.
        'aria-controls': undefined,
      }
    : { role: 'combobox', 'aria-expanded': open }

  return (
    <div className="flex flex-col gap-2">
      {/* Filter bar: trigger + active chips + children */}
      <div className="flex flex-wrap items-center gap-2">
        {/* The Popover root stays mounted on a touch viewport even though it
            can never open there: its trigger is what owns the open/close
            toggle for BOTH overlays, and the trigger has to stay one node so
            the pre-hydration click replay lands on it. */}
        <Popover open={open && !softKeyboardViewport} onOpenChange={handleOpenChange}>
          <PopoverTrigger asChild>
            <button
              // In server HTML, so it is painted and clickable for the whole
              // window before React attaches the overlay's handler - and a
              // click in that window is silently dropped, which is exactly what
              // this primitive exists for. The replay root goes on the trigger
              // rather than the surrounding bar because the trigger is the
              // element that owns the interaction.
              {...replayOnHydrate}
              ref={setTriggerRef}
              type="button"
              aria-label="Filter by city"
              data-testid="city-filter-combobox"
              className={cn(
                'flex items-center gap-2 rounded-md border border-border/50 bg-muted/50 px-3 py-1.5 text-sm transition-colors',
                'hover:bg-muted hover:border-border',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                open && 'border-border bg-muted',
                selectedCities.length === 0 && 'text-muted-foreground',
                selectedCities.length > 0 && 'text-foreground'
              )}
              {...overlayAria}
            >
              <Search className="h-3.5 w-3.5 shrink-0 opacity-50" />
              <span className="whitespace-nowrap">Filter by city...</span>
              <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 opacity-50" />
            </button>
          </PopoverTrigger>
          <PopoverContent className="w-[240px] p-0" align="start" side="bottom">
            <Command>
              <CommandInput placeholder="Search cities..." />
              <CommandList>
                <CommandEmpty>No cities found.</CommandEmpty>
                <CommandGroup>
                  {sortedCities.map(city => {
                    const key = cityKey(city)
                    const isSelected = selectedSet.has(key)
                    return (
                      <CommandItem
                        key={key}
                        value={cityLabel(city)}
                        onSelect={() => handleToggleCity(city)}
                        data-testid={`city-option-${city.city}-${city.state}`.toLowerCase().replace(/\s+/g, '-')}
                      >
                        <Check
                          className={cn(
                            'mr-2 h-4 w-4 shrink-0',
                            isSelected ? 'opacity-100' : 'opacity-0'
                          )}
                        />
                        <span className="flex-1 truncate">
                          {cityLabel(city)}
                        </span>
                        <span className="ml-2 text-xs text-muted-foreground">
                          ({formatCount(city.count)})
                        </span>
                      </CommandItem>
                    )
                  })}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>

        {softKeyboardViewport && (
          <CityFilterSheet
            cities={sortedCities}
            selectedCities={selectedCities}
            onApply={onFilterChange}
            open={open}
            onOpenChange={handleOpenChange}
            openSeq={openSeq}
            resultNoun={resultNoun}
            triggerRef={triggerRef}
          />
        )}

        {/* Active filter chips */}
        {selectedCities.map(city => (
          <RemovableFilterChip
            key={cityKey(city)}
            label={cityLabel(city)}
            onRemove={() => handleRemoveCity(city)}
            data-testid={`city-chip-${city.city}-${city.state}`.toLowerCase().replace(/\s+/g, '-')}
            removeTestId={`city-chip-remove-${city.city}-${city.state}`.toLowerCase().replace(/\s+/g, '-')}
          />
        ))}

        {/* "All Cities" button when cities are selected */}
        {selectedCities.length > 0 && (
          <button
            onClick={handleClearAll}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors whitespace-nowrap"
            data-testid="city-filter-all"
          >
            {selectedCities.length >= 2 ? 'Clear all' : allLabel}
          </button>
        )}

        {/* Children slot (e.g., SaveDefaultsButton) */}
        {children}
      </div>

      {/* Popular cities row */}
      {popularCities.length > 0 && selectedCities.length === 0 && (
        <div className="flex items-center gap-1 text-xs text-muted-foreground" data-testid="popular-cities">
          <span className="shrink-0">Popular:</span>
          {popularCities.map((city, i) => (
            <span key={cityKey(city)} className="inline-flex items-center">
              {i > 0 && <span className="mx-0.5">&middot;</span>}
              <button
                onClick={() => handlePopularClick(city)}
                className="hover:text-foreground transition-colors whitespace-nowrap"
                data-testid={`popular-city-${city.city}-${city.state}`.toLowerCase().replace(/\s+/g, '-')}
              >
                {cityLabel(city)} ({formatCount(city.count)})
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
