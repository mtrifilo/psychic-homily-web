import { FilterChip } from './FilterChip'

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

export function CityFilters({
  cities,
  selectedCities,
  onFilterChange,
  allLabel = 'All Cities',
  children,
}: CityFiltersProps) {
  const isAllSelected = selectedCities.length === 0
  const selectedSet = new Set(selectedCities.map(cityKey))

  const handleToggle = (city: string, state: string) => {
    const key = cityKey({ city, state })
    if (selectedSet.has(key)) {
      // Remove this city
      onFilterChange(selectedCities.filter(c => cityKey(c) !== key))
    } else {
      // Add this city
      onFilterChange([...selectedCities, { city, state }])
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-2 mb-6">
      <FilterChip
        label={allLabel}
        isActive={isAllSelected}
        onClick={() => onFilterChange([])}
      />
      {cities.map(city => {
        const isActive = selectedSet.has(cityKey(city))
        const label = `${city.city}, ${city.state}`

        return (
          <FilterChip
            key={`${city.city}-${city.state}`}
            label={label}
            isActive={isActive}
            onClick={() => handleToggle(city.city, city.state)}
            count={city.count}
          />
        )
      })}
      {children}
    </div>
  )
}
