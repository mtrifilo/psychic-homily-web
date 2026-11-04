'use client'

import { useMemo, useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { CommandPopoverContent } from '@/components/ui/command'
import { Popover, PopoverTrigger } from '@/components/ui/popover'
import {
  CityCommandList,
  citiesWithSelected,
  sortCitiesByCount,
  toggleCitySelection,
} from '@/components/filters/CityCommandList'
import { cityLabel } from '@/components/filters/cityParams'
import type { HomeShowCitySelection } from '@/features/shows/hooks/useHomeShowCitySelection'
import { cn } from '@/lib/utils'

/**
 * The nearby section's phone city control: the city the rows were fetched
 * for, as a button that opens the city picker in a popover.
 *
 * Stands in for the filter bar the list hides below `md`, so it reads and
 * writes the SAME selection the list was fetched with; the two cannot name
 * different cities. The picker lists every selected city too, even one the
 * count list has dropped, because with no chips here the picker is the only
 * way to untick it.
 *
 * Renders nothing when the filter bar would (no cities known).
 */
export function NearbyCityControl({
  selection,
  className,
}: {
  selection: HomeShowCitySelection
  className?: string
}) {
  const { cities, effectiveCities, onFilterChange } = selection
  const [open, setOpen] = useState(false)
  const panelRef = useRef<HTMLDivElement>(null)

  const pickerCities = useMemo(
    () => citiesWithSelected(sortCitiesByCount(cities), effectiveCities),
    [cities, effectiveCities]
  )

  if (cities.length === 0) return null

  const label =
    effectiveCities.length > 0
      ? effectiveCities.map(cityLabel).join(' · ')
      : 'All Cities'

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          // Contains the visible city (WCAG 2.5.3); the verb says what the
          // button does, which the chevron only implies.
          aria-label={`Change city, now ${label}`}
          data-testid="nearby-city-control"
          className={cn(
            'flex h-9 max-w-[55vw] items-center gap-1.5 rounded-md border border-border bg-background px-3 text-sm text-foreground transition-colors',
            'hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            className
          )}
        >
          <span className="truncate">{label}</span>
          <ChevronDown aria-hidden className="h-4 w-4 shrink-0 opacity-60" />
        </button>
      </PopoverTrigger>
      <CommandPopoverContent
        className="w-[240px] p-0 focus-visible:outline-none"
        align="end"
        side="bottom"
        // Only rendered below md, where focusing the search field on open
        // raises the soft keyboard over the list. Focus lands on the panel
        // instead; the field is one Tab or tap away.
        ref={panelRef}
        tabIndex={-1}
        onOpenAutoFocus={event => {
          event.preventDefault()
          panelRef.current?.focus()
        }}
      >
        <CityCommandList
          cities={pickerCities}
          selectedCities={effectiveCities}
          onToggle={city =>
            onFilterChange(toggleCitySelection(effectiveCities, city))
          }
        />
      </CommandPopoverContent>
    </Popover>
  )
}
