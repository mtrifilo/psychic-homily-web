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

interface CityFiltersProps {
  cities: CityWithCount[]
  selectedCity: string | null
  selectedState: string | null
  onFilterChange: (city: string | null, state: string | null) => void
  allLabel?: string
}

export function CityFilters({
  cities,
  selectedCity,
  selectedState,
  onFilterChange,
  allLabel = 'All Cities',
}: CityFiltersProps) {
  const isAllSelected = !selectedCity && !selectedState

  return (
    <div className="flex flex-wrap gap-2 mb-6">
      <FilterChip
        label={allLabel}
        isActive={isAllSelected}
        onClick={() => onFilterChange(null, null)}
      />
      {cities.map(city => {
        const isActive =
          selectedCity === city.city && selectedState === city.state
        const label = `${city.city}, ${city.state}`

        return (
          <FilterChip
            key={`${city.city}-${city.state}`}
            label={label}
            isActive={isActive}
            onClick={() => onFilterChange(city.city, city.state)}
            count={city.count}
          />
        )
      })}
    </div>
  )
}
