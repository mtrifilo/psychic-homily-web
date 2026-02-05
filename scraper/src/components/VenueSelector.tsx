import { useState, useMemo } from 'react'
import type { VenueConfig } from '../lib/types'

interface Props {
  venues: VenueConfig[]
  selectedVenues: VenueConfig[]
  onSelect: (venues: VenueConfig[]) => void
  onNext: () => void
}

export function VenueSelector({ venues, selectedVenues, onSelect, onNext }: Props) {
  const [cityFilter, setCityFilter] = useState<string | null>(null)

  // Get unique cities with counts
  const cities = useMemo(() => {
    const cityMap = new Map<string, { city: string; state: string; count: number }>()
    for (const venue of venues) {
      const key = `${venue.city}, ${venue.state}`
      const existing = cityMap.get(key)
      if (existing) {
        existing.count++
      } else {
        cityMap.set(key, { city: venue.city, state: venue.state, count: 1 })
      }
    }
    return Array.from(cityMap.entries()).sort((a, b) => a[0].localeCompare(b[0]))
  }, [venues])

  // Filter venues by selected city
  const filteredVenues = useMemo(() => {
    if (!cityFilter) return venues
    return venues.filter(v => `${v.city}, ${v.state}` === cityFilter)
  }, [venues, cityFilter])

  // Group filtered venues by city
  const venuesByCity = useMemo(() => {
    const groups = new Map<string, VenueConfig[]>()
    for (const venue of filteredVenues) {
      const key = `${venue.city}, ${venue.state}`
      const existing = groups.get(key) || []
      existing.push(venue)
      groups.set(key, existing)
    }
    return Array.from(groups.entries()).sort((a, b) => a[0].localeCompare(b[0]))
  }, [filteredVenues])

  const isSelected = (venue: VenueConfig) =>
    selectedVenues.some(v => v.slug === venue.slug)

  const toggleVenue = (venue: VenueConfig) => {
    if (isSelected(venue)) {
      onSelect(selectedVenues.filter(v => v.slug !== venue.slug))
    } else {
      onSelect([...selectedVenues, venue])
    }
  }

  const selectAll = () => {
    // Select all visible (filtered) venues
    const newSelection = [...selectedVenues]
    for (const venue of filteredVenues) {
      if (!newSelection.some(v => v.slug === venue.slug)) {
        newSelection.push(venue)
      }
    }
    onSelect(newSelection)
  }

  const selectNone = () => {
    // Deselect all visible (filtered) venues
    const slugsToRemove = new Set(filteredVenues.map(v => v.slug))
    onSelect(selectedVenues.filter(v => !slugsToRemove.has(v.slug)))
  }

  // Count of selected in current filter
  const selectedInFilter = filteredVenues.filter(v => isSelected(v)).length

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-gray-900">Select Venues</h2>
        <p className="text-sm text-gray-500 mt-1">
          Choose which venues to scrape events from ({venues.length} available)
        </p>
      </div>

      {/* City Filter */}
      {cities.length > 1 && (
        <div className="flex flex-wrap gap-2">
          <button
            onClick={() => setCityFilter(null)}
            className={`px-3 py-1.5 rounded-full text-sm font-medium transition-colors ${
              cityFilter === null
                ? 'bg-blue-600 text-white'
                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            All ({venues.length})
          </button>
          {cities.map(([key, { city, state, count }]) => (
            <button
              key={key}
              onClick={() => setCityFilter(key)}
              className={`px-3 py-1.5 rounded-full text-sm font-medium transition-colors ${
                cityFilter === key
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
              }`}
            >
              {city}, {state} ({count})
            </button>
          ))}
        </div>
      )}

      <div className="flex gap-2">
        <button
          onClick={selectAll}
          className="text-sm text-blue-600 hover:text-blue-700"
        >
          Select All{cityFilter ? ` in ${cityFilter}` : ''}
        </button>
        <span className="text-gray-300">|</span>
        <button
          onClick={selectNone}
          className="text-sm text-gray-500 hover:text-gray-700"
        >
          Clear{cityFilter ? ` in ${cityFilter}` : ''}
        </button>
        {selectedInFilter > 0 && (
          <>
            <span className="text-gray-300">|</span>
            <span className="text-sm text-gray-500">
              {selectedInFilter} of {filteredVenues.length} selected
            </span>
          </>
        )}
      </div>

      {/* Venues grouped by city */}
      <div className="space-y-6">
        {venuesByCity.map(([cityKey, cityVenues]) => (
          <div key={cityKey}>
            {/* City header (only show if not filtering by city) */}
            {!cityFilter && cities.length > 1 && (
              <h3 className="text-sm font-medium text-gray-600 mb-3 flex items-center gap-2">
                <span className="w-2 h-2 bg-blue-500 rounded-full" />
                {cityKey}
              </h3>
            )}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {cityVenues.map(venue => (
                <div
                  key={venue.slug}
                  onClick={() => toggleVenue(venue)}
                  className={`p-4 rounded-lg border-2 cursor-pointer transition-all ${
                    isSelected(venue)
                      ? 'border-blue-500 bg-blue-50'
                      : 'border-gray-200 bg-white hover:border-gray-300'
                  }`}
                >
                  <div className="flex items-start justify-between">
                    <div>
                      <h3 className="font-medium text-gray-900">{venue.name}</h3>
                      <p className="text-xs text-gray-500 mt-1">
                        {venue.city}, {venue.state} • {venue.scraperType}
                      </p>
                    </div>
                    <div
                      className={`w-5 h-5 rounded border-2 flex items-center justify-center ${
                        isSelected(venue)
                          ? 'bg-blue-500 border-blue-500 text-white'
                          : 'border-gray-300'
                      }`}
                    >
                      {isSelected(venue) && (
                        <svg className="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                          <path
                            fillRule="evenodd"
                            d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                            clipRule="evenodd"
                          />
                        </svg>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>

      <div className="flex justify-end">
        <button
          onClick={onNext}
          disabled={selectedVenues.length === 0}
          className={`px-4 py-2 rounded-lg font-medium transition-colors ${
            selectedVenues.length > 0
              ? 'bg-blue-600 text-white hover:bg-blue-700'
              : 'bg-gray-200 text-gray-400 cursor-not-allowed'
          }`}
        >
          Preview Events ({selectedVenues.length} venue{selectedVenues.length !== 1 ? 's' : ''})
        </button>
      </div>
    </div>
  )
}
