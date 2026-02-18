'use client'

import { useState } from 'react'
import { type AnyFieldApi } from '@tanstack/react-form'
import { X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { useArtistSearch } from '@/lib/hooks/useArtistSearch'
import { getArtistLocation } from '@/lib/types/artist'
import { FieldInfo } from './FormField'

interface ArtistInputProps {
  field: AnyFieldApi
  index: number
  onRemove?: () => void
  showRemoveButton?: boolean
  /** Called when the artist match status changes (id = matched existing artist, undefined = new artist) */
  onArtistMatch?: (artistId: number | undefined) => void
}

/**
 * Artist input with autocomplete dropdown
 * Searches existing artists as user types
 */
export function ArtistInput({
  field,
  index,
  onRemove,
  showRemoveButton,
  onArtistMatch,
}: ArtistInputProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')

  const { data: searchResults } = useArtistSearch({
    query: searchValue,
  })

  const filteredArtists = searchResults?.artists || []

  // Handle artist selection from dropdown
  const handleArtistSelect = (artistName: string, artistId?: number) => {
    field.handleChange(artistName)
    onArtistMatch?.(artistId)
    setIsOpen(false)
    setSearchValue('')
  }

  // Handle confirming current input value
  const handleConfirm = () => {
    const value = field.state.value?.trim()
    if (value) {
      // Check for exact match and use proper casing
      const exactMatch = filteredArtists.find(
        artist => artist.name.toLowerCase() === value.toLowerCase()
      )
      if (exactMatch) {
        if (exactMatch.name !== value) {
          field.handleChange(exactMatch.name)
        }
        onArtistMatch?.(exactMatch.id)
      } else {
        onArtistMatch?.(undefined)
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
    // Clear match when user types (they may be entering a new artist)
    onArtistMatch?.(undefined)
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
    !filteredArtists.some(
      artist => artist.name.toLowerCase() === searchValue.toLowerCase()
    )

  return (
    <div className="space-y-2">
      <Label htmlFor={field.name}>Artist {index + 1}</Label>
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Input
            type="text"
            id={field.name}
            name={field.name}
            value={field.state.value}
            onBlur={handleBlur}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            placeholder="Enter artist name"
            autoComplete="off"
            aria-invalid={field.state.meta.errors.length > 0}
          />

          {/* Autocomplete dropdown */}
          {isOpen && (
            <div className="absolute top-full left-0 w-full z-50 mt-1 rounded-md border bg-popover text-popover-foreground shadow-md">
              <div className="max-h-[300px] overflow-y-auto">
                {/* Existing artists section */}
                {filteredArtists.length > 0 && (
                  <div className="p-1">
                    <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                      Existing Artists
                    </div>
                    {filteredArtists.map(artist => (
                      <button
                        type="button"
                        key={artist.id}
                        className="relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground"
                        onMouseDown={e => {
                          e.preventDefault()
                          handleArtistSelect(artist.name, artist.id)
                        }}
                      >
                        <div className="flex w-full items-center justify-between gap-2">
                          <span className="truncate">{artist.name}</span>
                          <span className="flex-shrink-0 text-xs tracking-wider text-muted-foreground">
                            {getArtistLocation(artist)}
                          </span>
                        </div>
                      </button>
                    ))}
                  </div>
                )}

                {/* Add new artist option */}
                {showAddNew && (
                  <div className="p-1 border-t border-border">
                    <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                      Add New Artist
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

        {showRemoveButton && onRemove && (
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={onRemove}
            aria-label="Remove artist"
          >
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>
      <FieldInfo field={field} />
    </div>
  )
}

