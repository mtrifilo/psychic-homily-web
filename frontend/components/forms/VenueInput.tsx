'use client'

import { useState } from 'react'
import { type AnyFieldApi } from '@tanstack/react-form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useVenueSearch } from '@/lib/hooks/useVenueSearch'
import { getVenueLocation } from '@/lib/types/venue'
import type { Venue } from '@/lib/types/venue'
import { FieldInfo } from './FormField'

interface VenueInputProps {
  field: AnyFieldApi
  onVenueSelect?: (venue: Venue | null) => void
  onVenueNameChange?: (name: string) => void
}

/**
 * Venue input with autocomplete dropdown
 * Searches existing venues as user types
 * Can auto-fill city/state when a known venue is selected
 */
export function VenueInput({
  field,
  onVenueSelect,
  onVenueNameChange,
}: VenueInputProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')

  const { data: searchResults } = useVenueSearch({
    query: searchValue,
  })

  const filteredVenues = searchResults?.venues || []

  // Handle venue selection from dropdown
  const handleVenueSelect = (venue: Venue) => {
    field.handleChange(venue.name)
    onVenueSelect?.(venue)
    setIsOpen(false)
    setSearchValue('')
  }

  // Handle confirming current input value (new venue)
  const handleConfirm = () => {
    const value = field.state.value?.trim()
    if (value) {
      // Check for exact match and use proper casing
      const exactMatch = filteredVenues.find(
        venue => venue.name.toLowerCase() === value.toLowerCase()
      )
      if (exactMatch) {
        field.handleChange(exactMatch.name)
        onVenueSelect?.(exactMatch)
      } else {
        // New venue - no auto-fill
        onVenueSelect?.(null)
      }
    }
    setIsOpen(false)
    setSearchValue('')
  }

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    field.handleChange(value)
    setSearchValue(value)
    setIsOpen(value.length > 0)
    // Clear venue selection when typing
    onVenueSelect?.(null)
    // Notify parent of venue name change
    onVenueNameChange?.(value)
  }

  const handleBlur = () => {
    // Delay closing to allow click on dropdown items
    setTimeout(() => {
      if (field.state.value?.trim()) {
        handleConfirm()
      } else {
        setIsOpen(false)
        setSearchValue('')
      }
      field.handleBlur()
    }, 150)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleConfirm()
    }
    if (e.key === 'Escape') {
      setIsOpen(false)
    }
  }

  const showAddNew =
    searchValue &&
    !filteredVenues.some(
      venue => venue.name.toLowerCase() === searchValue.toLowerCase()
    )

  return (
    <div className="space-y-2">
      <Label htmlFor={field.name}>Venue</Label>
      <div className="relative">
        <Input
          type="text"
          id={field.name}
          name={field.name}
          value={field.state.value}
          onBlur={handleBlur}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          placeholder="Enter venue name"
          autoComplete="off"
          aria-invalid={field.state.meta.errors.length > 0}
        />

        {/* Autocomplete dropdown */}
        {isOpen && (
          <div className="absolute top-full left-0 w-full z-50 mt-1 rounded-md border bg-popover text-popover-foreground shadow-md">
            <div className="max-h-[300px] overflow-y-auto">
              {/* Existing venues section */}
              {filteredVenues.length > 0 && (
                <div className="p-1">
                  <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                    Existing Venues
                  </div>
                  {filteredVenues.map(venue => (
                    <button
                      type="button"
                      key={venue.id}
                      className="relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground"
                      onMouseDown={e => {
                        e.preventDefault()
                        handleVenueSelect(venue)
                      }}
                    >
                      <div className="flex w-full items-center justify-between gap-2">
                        <span className="truncate">{venue.name}</span>
                        <span className="flex-shrink-0 text-xs tracking-wider text-muted-foreground">
                          {getVenueLocation(venue)}
                        </span>
                      </div>
                    </button>
                  ))}
                </div>
              )}

              {/* Add new venue option */}
              {showAddNew && (
                <div className="p-1 border-t border-border">
                  <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                    Add New Venue
                  </div>
                  <button
                    type="button"
                    className="relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground"
                    onMouseDown={e => {
                      e.preventDefault()
                      handleConfirm()
                    }}
                  >
                    <span className="truncate">
                      &quot;{field.state.value}&quot;
                    </span>
                  </button>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
      <FieldInfo field={field} />
    </div>
  )
}
