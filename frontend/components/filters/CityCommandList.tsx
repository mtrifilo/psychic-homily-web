'use client'

import { useMemo } from 'react'
import { Check } from 'lucide-react'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { formatCount } from '@/components/shared/paginationChrome'
import { cn } from '@/lib/utils'
import { cityKey, cityLabel } from './cityParams'
import type { CityState, CityWithCount } from './CityFilters'

/**
 * `selected` with `city` added, or removed when it is already there. Picking a
 * city in any picker toggles it; there is no single-select mode.
 */
export function toggleCitySelection(
  selected: CityState[],
  city: CityState
): CityState[] {
  const key = cityKey(city)
  return selected.some(c => cityKey(c) === key)
    ? selected.filter(c => cityKey(c) !== key)
    : [...selected, { city: city.city, state: city.state }]
}

/**
 * A picker's rows: every city the page knows about, plus any selected city the
 * page has since stopped listing. Without the second half a selection that has
 * dropped out of the count list has no row, so it cannot be unticked from the
 * picker and the picker shows a selection the reader cannot see.
 */
export function citiesWithSelected(
  cities: CityWithCount[],
  selectedCities: CityState[]
): CityWithCount[] {
  const listed = new Set(cities.map(cityKey))
  const orphans = selectedCities
    .filter(c => !listed.has(cityKey(c)))
    .map(c => ({ city: c.city, state: c.state, count: 0 }))
  return orphans.length === 0 ? cities : [...cities, ...orphans]
}

/** The order every city picker lists its cities in: most shows first. */
export function sortCitiesByCount(cities: CityWithCount[]): CityWithCount[] {
  return [...cities].sort((a, b) => b.count - a.count)
}

/**
 * The searchable city list a picker popover holds: a row per city in the
 * order given, a check on the selected ones, the count beside each. Pair it
 * with `CommandPopoverContent`, which bounds it to the room on screen.
 */
export function CityCommandList({
  cities,
  selectedCities,
  onToggle,
}: {
  /** Listed as given; callers order them with {@link sortCitiesByCount}. */
  cities: CityWithCount[]
  selectedCities: CityState[]
  onToggle: (city: CityWithCount) => void
}) {
  const selectedKeys = useMemo(
    () => new Set(selectedCities.map(cityKey)),
    [selectedCities]
  )

  return (
    <Command>
      <CommandInput placeholder="Search cities..." />
      <CommandList>
        <CommandEmpty>No cities found.</CommandEmpty>
        <CommandGroup>
          {cities.map(city => {
            const key = cityKey(city)
            return (
              <CommandItem
                key={key}
                value={cityLabel(city)}
                onSelect={() => onToggle(city)}
                data-testid={`city-option-${city.city}-${city.state}`
                  .toLowerCase()
                  .replace(/\s+/g, '-')}
              >
                <Check
                  className={cn(
                    'mr-2 h-4 w-4 shrink-0',
                    selectedKeys.has(key) ? 'opacity-100' : 'opacity-0'
                  )}
                />
                <span className="flex-1 truncate">{cityLabel(city)}</span>
                <span className="ml-2 text-xs text-muted-foreground">
                  ({formatCount(city.count)})
                </span>
              </CommandItem>
            )
          })}
        </CommandGroup>
      </CommandList>
    </Command>
  )
}
