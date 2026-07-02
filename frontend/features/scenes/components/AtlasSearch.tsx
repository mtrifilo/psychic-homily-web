'use client'

import { useCallback, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { Search } from 'lucide-react'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { isPlaceableScene, type PlaceableScene } from './globeTypes'
import type { SceneListItem } from '../types'

interface AtlasSearchProps {
  /** ALL scenes (placeable or not) — an unplaceable match still navigates. */
  scenes: SceneListItem[]
  /** Fly-to + open-preview for a scene the globe can place (PSY-1308 seam). */
  onPick: (scene: PlaceableScene) => void
}

/**
 * Directed search on the Atlas globe (PSY-1310): a type-ahead over the
 * already-loaded scenes list — the top usability gap in the reference product
 * (radio.garden's critiques: spatial browsing only, no path for "take me to
 * Minneapolis"). Selecting a placeable scene flies the camera + opens its
 * preview; an unplaceable one (no coords) navigates to its scene page instead
 * of pretending to fly. Client-side only — no new requests.
 *
 * Reuses the CityFilters combobox idiom (Popover + cmdk Command), so keyboard
 * operability (arrows/Enter/Esc) comes from cmdk. `/` opens it from anywhere on
 * the page (unless typing in another field) — this is also the keyboard path
 * INTO scenes on /atlas, since canvas dots aren't focusable (PSY-1313 pairing).
 */
export function AtlasSearch({ scenes, onPick }: AtlasSearchProps) {
  const router = useRouter()
  const [open, setOpen] = useState(false)

  // Most-active-first so the list leads with the liveliest scenes before any
  // query is typed; cmdk's built-in filter takes over as the user types.
  const sorted = [...scenes].sort(
    (a, b) => b.upcoming_show_count - a.upcoming_show_count,
  )

  const handleSelect = useCallback(
    (scene: SceneListItem) => {
      setOpen(false)
      if (isPlaceableScene(scene)) {
        onPick(scene)
      } else {
        router.push(`/scenes/${scene.slug}`)
      }
    },
    [onPick, router],
  )

  // `/` opens the search (the common map/list idiom), ignored while typing in
  // any other field so it never hijacks real input.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return
      const target = e.target as HTMLElement | null
      if (
        target &&
        (target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.isContentEditable)
      ) {
        return
      }
      e.preventDefault()
      setOpen(true)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          role="combobox"
          aria-expanded={open}
          aria-label="Search scenes"
          className="absolute left-4 top-4 z-10 flex items-center gap-2 rounded-full border border-border bg-background/90 px-3 py-1.5 text-sm text-muted-foreground backdrop-blur transition-colors hover:border-primary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Search className="h-3.5 w-3.5 shrink-0 opacity-60" aria-hidden />
          <span>Search scenes</span>
          <kbd
            className="rounded border border-border px-1 font-mono text-[10px] leading-4"
            aria-hidden
          >
            /
          </kbd>
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-[260px] p-0" align="start">
        <Command>
          <CommandInput placeholder="City or state…" />
          <CommandList>
            <CommandEmpty>No scenes found.</CommandEmpty>
            <CommandGroup>
              {sorted.map((scene) => (
                <CommandItem
                  key={scene.slug}
                  value={`${scene.city}, ${scene.state}`}
                  onSelect={() => handleSelect(scene)}
                >
                  <span className="flex-1 truncate">
                    {scene.city}, {scene.state}
                  </span>
                  <span className="ml-2 font-mono text-xs text-muted-foreground">
                    {scene.upcoming_show_count}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
