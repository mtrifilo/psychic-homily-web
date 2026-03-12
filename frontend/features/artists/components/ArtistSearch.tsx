'use client'

import { useState, useRef } from 'react'
import { useRouter } from 'next/navigation'
import { Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { useArtistSearch } from '../hooks/useArtistSearch'
import { getArtistLocation } from '../types'

/**
 * Artist search with autocomplete dropdown.
 * Navigates to the artist detail page on selection.
 */
export function ArtistSearch() {
  const router = useRouter()
  const [query, setQuery] = useState('')
  const [isOpen, setIsOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)
  const inputRef = useRef<HTMLInputElement>(null)

  const { data: searchResults } = useArtistSearch({ query })
  const artists = searchResults?.artists ?? []

  const handleSelect = (slug: string) => {
    setQuery('')
    setIsOpen(false)
    setActiveIndex(-1)
    router.push(`/artists/${slug}`)
  }

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setQuery(value)
    setIsOpen(value.length > 0)
    setActiveIndex(-1)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!isOpen || artists.length === 0) {
      if (e.key === 'Escape') {
        setIsOpen(false)
        inputRef.current?.blur()
      }
      return
    }

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        setActiveIndex(prev => (prev < artists.length - 1 ? prev + 1 : 0))
        break
      case 'ArrowUp':
        e.preventDefault()
        setActiveIndex(prev => (prev > 0 ? prev - 1 : artists.length - 1))
        break
      case 'Enter':
        e.preventDefault()
        if (activeIndex >= 0 && activeIndex < artists.length) {
          handleSelect(artists[activeIndex].slug)
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
          placeholder="Search artists..."
          autoComplete="off"
          className="pl-8"
        />
      </div>

      {isOpen && artists.length > 0 && (
        <div className="absolute top-full left-0 w-full z-50 mt-1 rounded-md border bg-popover text-popover-foreground shadow-md">
          <div className="max-h-[300px] overflow-y-auto p-1">
            {artists.map((artist, i) => (
              <button
                type="button"
                key={artist.id}
                className={`relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none ${
                  i === activeIndex
                    ? 'bg-accent text-accent-foreground'
                    : 'hover:bg-accent hover:text-accent-foreground'
                }`}
                onMouseDown={e => {
                  e.preventDefault()
                  handleSelect(artist.slug)
                }}
                onMouseEnter={() => setActiveIndex(i)}
              >
                <div className="flex w-full items-center justify-between gap-2">
                  <span className="truncate">{artist.name}</span>
                  {(artist.city || artist.state) && (
                    <span className="flex-shrink-0 text-xs text-muted-foreground">
                      {getArtistLocation(artist)}
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
