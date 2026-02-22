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

  const handleToggle = (city: string, state: string, e: React.MouseEvent) => {
    const key = cityKey({ city, state })
    if (e.shiftKey) {
      // Shift+click: toggle this city in the multi-selection
      if (selectedSet.has(key)) {
        onFilterChange(selectedCities.filter(c => cityKey(c) !== key))
      } else {
        onFilterChange([...selectedCities, { city, state }])
      }
    } else {
      // Single click: select only this city, or deselect if already the sole selection
      if (selectedSet.has(key) && selectedCities.length === 1) {
        onFilterChange([])
      } else {
        onFilterChange([{ city, state }])
      }
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
            onClick={(e) => handleToggle(city.city, city.state, e)}
            count={city.count}
          />
        )
      })}
      {children}
    </div>
  )
}
