'use client'

import { useState, useRef } from 'react'
import { useRouter } from 'next/navigation'
import { Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { useVenueSearch } from '@/lib/hooks/useVenueSearch'
import { getVenueLocation } from '@/lib/types/venue'

/**
 * Venue search with autocomplete dropdown.
 * Navigates to the venue detail page on selection.
 */
export function VenueSearch() {
  const router = useRouter()
  const [query, setQuery] = useState('')
  const [isOpen, setIsOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)
  const inputRef = useRef<HTMLInputElement>(null)

  const { data: searchResults } = useVenueSearch({ query })
  const venues = searchResults?.venues ?? []

  const handleSelect = (slug: string) => {
    setQuery('')
    setIsOpen(false)
    setActiveIndex(-1)
    router.push(`/venues/${slug}`)
  }

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setQuery(value)
    setIsOpen(value.length > 0)
    setActiveIndex(-1)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!isOpen || venues.length === 0) {
      if (e.key === 'Escape') {
        setIsOpen(false)
        inputRef.current?.blur()
      }
      return
    }

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        setActiveIndex(prev => (prev < venues.length - 1 ? prev + 1 : 0))
        break
      case 'ArrowUp':
        e.preventDefault()
        setActiveIndex(prev => (prev > 0 ? prev - 1 : venues.length - 1))
        break
      case 'Enter':
        e.preventDefault()
        if (activeIndex >= 0 && activeIndex < venues.length) {
          handleSelect(venues[activeIndex].slug)
        }
        break
      case 'Escape':
        setIsOpen(false)
        setActiveIndex(-1)
        inputRef.current?.blur()
        break
    }
  }

  const handleBlur = () => {
    // Delay to allow click on dropdown items
    setTimeout(() => {
      setIsOpen(false)
      setActiveIndex(-1)
    }, 150)
  }

  return (
    <div className="relative w-full max-w-sm">
      <div className="relative">
        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
        <Input
          ref={inputRef}
          type="text"
          value={query}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onBlur={handleBlur}
          placeholder="Search venues..."
          autoComplete="off"
          className="pl-8"
        />
      </div>

      {isOpen && venues.length > 0 && (
        <div className="absolute top-full left-0 w-full z-50 mt-1 rounded-md border bg-popover text-popover-foreground shadow-md">
          <div className="max-h-[300px] overflow-y-auto p-1">
            {venues.map((venue, i) => (
              <button
                type="button"
                key={venue.id}
                className={`relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none ${
                  i === activeIndex
                    ? 'bg-accent text-accent-foreground'
                    : 'hover:bg-accent hover:text-accent-foreground'
                }`}
                onMouseDown={e => {
                  e.preventDefault()
                  handleSelect(venue.slug)
                }}
                onMouseEnter={() => setActiveIndex(i)}
              >
                <div className="flex w-full items-center justify-between gap-2">
                  <span className="truncate">{venue.name}</span>
                  {(venue.city || venue.state) && (
                    <span className="flex-shrink-0 text-xs text-muted-foreground">
                      {getVenueLocation(venue)}
                    </span>
                  )}
                </div>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
