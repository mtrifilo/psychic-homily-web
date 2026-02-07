import { useState, useMemo } from 'react'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { VenueCard } from './discovery/VenueCard'
import { cn } from '../lib/utils'
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
    const newSelection = [...selectedVenues]
    for (const venue of filteredVenues) {
      if (!newSelection.some(v => v.slug === venue.slug)) {
        newSelection.push(venue)
      }
    }
    onSelect(newSelection)
  }

  const selectNone = () => {
    const slugsToRemove = new Set(filteredVenues.map(v => v.slug))
    onSelect(selectedVenues.filter(v => !slugsToRemove.has(v.slug)))
  }

  const selectedInFilter = filteredVenues.filter(v => isSelected(v)).length

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-foreground">Select Venues</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Choose which venues to scrape events from ({venues.length} available)
        </p>
      </div>

      {/* City Filter */}
      {cities.length > 1 && (
        <div className="flex flex-wrap gap-2">
          <button
            onClick={() => setCityFilter(null)}
            className={cn(
              'px-3 py-1.5 rounded-full text-sm font-medium transition-colors',
              cityFilter === null
                ? 'bg-primary text-primary-foreground'
                : 'bg-secondary text-secondary-foreground hover:bg-secondary/80'
            )}
          >
            All ({venues.length})
          </button>
          {cities.map(([key, { city, state, count }]) => (
            <button
              key={key}
              onClick={() => setCityFilter(key)}
              className={cn(
                'px-3 py-1.5 rounded-full text-sm font-medium transition-colors',
                cityFilter === key
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-secondary text-secondary-foreground hover:bg-secondary/80'
              )}
            >
              {city}, {state} ({count})
            </button>
          ))}
        </div>
      )}

      <div className="flex items-center gap-2">
        <Button variant="link" size="sm" onClick={selectAll} className="px-0">
          Select All{cityFilter ? ` in ${cityFilter}` : ''}
        </Button>
        <span className="text-muted-foreground">|</span>
        <Button variant="link" size="sm" onClick={selectNone} className="px-0 text-muted-foreground">
          Clear{cityFilter ? ` in ${cityFilter}` : ''}
        </Button>
        {selectedInFilter > 0 && (
          <>
            <span className="text-muted-foreground">|</span>
            <span className="text-sm text-muted-foreground">
              {selectedInFilter} of {filteredVenues.length} selected
            </span>
          </>
        )}
      </div>

      {/* Venues grouped by city */}
      <div className="space-y-6">
        {venuesByCity.map(([cityKey, cityVenues]) => (
          <div key={cityKey}>
            {!cityFilter && cities.length > 1 && (
              <h3 className="text-sm font-medium text-muted-foreground mb-3 flex items-center gap-2">
                <span className="w-2 h-2 bg-primary rounded-full" />
                {cityKey}
              </h3>
            )}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {cityVenues.map(venue => (
                <VenueCard
                  key={venue.slug}
                  venue={venue}
                  selected={isSelected(venue)}
                  onToggle={() => toggleVenue(venue)}
                />
              ))}
            </div>
          </div>
        ))}
      </div>

      <div className="flex justify-end">
        <Button onClick={onNext} disabled={selectedVenues.length === 0}>
          Preview Events ({selectedVenues.length} venue{selectedVenues.length !== 1 ? 's' : ''})
        </Button>
      </div>
    </div>
  )
}
