'use client'

import { useState, useMemo } from 'react'
import { Search, X, Check, ChevronsUpDown } from 'lucide-react'
import { Command, CommandInput, CommandList, CommandEmpty, CommandGroup, CommandItem } from '@/components/ui/command'
import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

/**
 * Generic interface for city data with a count.
 * Both venues and shows can map their specific types to this interface.
 */
export interface CityWithCount {
  city: string
  state: string
  count: number
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
  allLabel?: string
  children?: React.ReactNode
}

function cityKey(c: CityState): string {
  return `${c.city}|${c.state}`
}

function cityLabel(c: CityState): string {
  return `${c.city}, ${c.state}`
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
  allLabel = 'All Cities',
  children,
}: CityFiltersProps) {
  const [open, setOpen] = useState(false)
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

  return (
    <div className="flex flex-col gap-2">
      {/* Filter bar: combobox + active chips + children */}
      <div className="flex flex-wrap items-center gap-2">
        {/* Searchable combobox */}
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <button
              role="combobox"
              aria-expanded={open}
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
            >
              <Search className="h-3.5 w-3.5 shrink-0 opacity-50" />
              <span className="whitespace-nowrap">Filter by city...</span>
              <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 opacity-50" />
            </button>
          </PopoverTrigger>
          <PopoverContent className="w-[240px] p-0" align="start">
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
                          ({city.count})
                        </span>
                      </CommandItem>
                    )
                  })}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>

        {/* Active filter chips */}
        {selectedCities.map(city => (
          <Badge
            key={cityKey(city)}
            variant="secondary"
            className="flex items-center gap-1 px-2.5 py-1 text-xs font-medium cursor-default"
            data-testid={`city-chip-${city.city}-${city.state}`.toLowerCase().replace(/\s+/g, '-')}
          >
            {cityLabel(city)}
            <button
              onClick={() => handleRemoveCity(city)}
              className="ml-0.5 rounded-full hover:bg-foreground/10 p-0.5 transition-colors"
              aria-label={`Remove ${cityLabel(city)} filter`}
              data-testid={`city-chip-remove-${city.city}-${city.state}`.toLowerCase().replace(/\s+/g, '-')}
            >
              <X className="h-3 w-3" />
            </button>
          </Badge>
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
                {cityLabel(city)} ({city.count})
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
