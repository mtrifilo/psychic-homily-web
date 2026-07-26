'use client'

import { useMemo } from 'react'
import type { VenueWithShowCount } from '@/features/venues/types'
import { formatTimeAgo } from '@/lib/formatTimeAgo'
import { GENRE_FAMILIES } from '../genreFamilies'
import {
  cityDataUpdatedAt,
  cityGenreFamilies,
  cityRailStats,
  formatNextShowDate,
  nextShowBill,
  type CityVenueFilters,
} from '../cityView'

const GENRE_LABEL_BY_KEY: ReadonlyMap<string, string> = new Map(
  // Lowercased: the rail's meta line is running mono text ("… · punk &
  // hardcore"), not a legend entry, so the legend's title case would shout.
  GENRE_FAMILIES.map((f) => [f.key, f.label.toLowerCase()]),
)

interface VenueRailProps {
  /** Display name of the city the camera is on, e.g. "Austin, TX". */
  cityLabel: string
  /** Venues AFTER filtering — the same array the map pins. */
  venues: readonly VenueWithShowCount[]
  /** Venues BEFORE filtering, for the header stats and the genre menu. */
  allVenues: readonly VenueWithShowCount[]
  /** Scene-level roster size ("35 LOCAL ARTISTS"); undefined while loading. */
  localArtistCount?: number
  loading?: boolean
  filters: CityVenueFilters
  onFiltersChange: (filters: CityVenueFilters) => void
  selectedVenueId: number | null
  onVenueSelect: (venueId: number) => void
  /** "← globe": fly the camera back out to the globe view. */
  onBackToGlobe: () => void
}

/**
 * The Atlas city view's left rail (PSY-1539) — a dense list of the city's
 * venues, synced with the pins beside it.
 *
 * It sits BESIDE the map, not over it: the map pane is narrowed by exactly
 * this rail's width. That is a licensing constraint as much as a layout one —
 * the map's bottom-left OpenStreetMap attribution is required by the ODbL, and
 * PSY-1543's review found a full-height right-edge panel hiding it outright.
 *
 * The rail renders `venues` (already filtered) and derives its header counts
 * from `allVenues`, so the header describes the city while the list shows the
 * current cut of it.
 */
export function VenueRail({
  cityLabel,
  venues,
  allVenues,
  localArtistCount,
  loading = false,
  filters,
  onFiltersChange,
  selectedVenueId,
  onVenueSelect,
  onBackToGlobe,
}: VenueRailProps) {
  const stats = useMemo(() => cityRailStats(allVenues), [allVenues])
  const genreFamilies = useMemo(() => cityGenreFamilies(allVenues), [allVenues])
  const updatedAt = useMemo(() => cityDataUpdatedAt(allVenues), [allVenues])

  const activeGenreLabel = filters.genreFamily
    ? (GENRE_LABEL_BY_KEY.get(filters.genreFamily) ?? 'All genres')
    : 'All genres'

  return (
    <aside
      aria-label={`Venues in ${cityLabel}`}
      data-testid="atlas-venue-rail"
      className="flex h-full w-[360px] shrink-0 flex-col border-r border-border bg-card"
    >
      <header className="border-b border-border px-4 pb-3 pt-4">
        <div className="flex items-baseline gap-3">
          <h2 className="text-xl font-semibold text-foreground">{cityLabel}</h2>
          <button
            type="button"
            onClick={onBackToGlobe}
            className="font-mono text-xs text-muted-foreground underline-offset-4 transition-colors hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            ← globe
          </button>
        </div>

        <p className="mt-2 font-mono text-[11px] uppercase leading-4 tracking-wide text-muted-foreground">
          {stats.venueCount} {stats.venueCount === 1 ? 'venue' : 'venues'} ·{' '}
          {stats.upcomingCount} upcoming · {stats.thisWeekCount} this week
          {localArtistCount !== undefined && (
            <> · {localArtistCount} local artists</>
          )}
        </p>

        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          <FilterChip
            active={filters.thisWeekOnly}
            onClick={() =>
              onFiltersChange({ ...filters, thisWeekOnly: !filters.thisWeekOnly })
            }
          >
            This week
          </FilterChip>

          {/* A native select styled as a chip: the menu is a list of the
              families actually present, so every option leads somewhere.
              The select comes FIRST in the DOM (and sits on top via z-10) so
              the visible chip can be its Tailwind `peer` — the select itself
              is transparent, so its own focus ring is invisible and a
              keyboard user would otherwise get no focus indicator at all. */}
          <span className="relative inline-flex">
            <select
              aria-label="Filter venues by genre"
              value={filters.genreFamily ?? ''}
              onChange={(e) =>
                onFiltersChange({
                  ...filters,
                  genreFamily: e.target.value === '' ? null : e.target.value,
                })
              }
              className="peer absolute inset-0 z-10 cursor-pointer opacity-0"
            >
              <option value="">All genres</option>
              {genreFamilies.map((f) => (
                <option key={f.key} value={f.key}>
                  {f.label}
                </option>
              ))}
            </select>
            <span
              aria-hidden="true"
              className={`${chipClass(filters.genreFamily !== null)} peer-focus-visible:ring-2 peer-focus-visible:ring-ring`}
            >
              {activeGenreLabel} ⌄
            </span>
          </span>

          {/* Disabled placeholders, exactly as the mock draws them. "All ages"
              has no data model yet (venues carry no all-ages field, and
              whether it becomes a tag or a column is an open decision);
              "Record stores" is a later chapter of the travel-mode project. */}
          <FilterChip disabled title="Age policy isn’t recorded yet">
            All ages
          </FilterChip>
          <FilterChip disabled title="Record stores aren’t on the Atlas yet">
            Record stores
          </FilterChip>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {loading && venues.length === 0 ? (
          <p className="px-4 py-6 text-sm text-muted-foreground">
            Loading venues…
          </p>
        ) : venues.length === 0 ? (
          <p className="px-4 py-6 text-sm text-muted-foreground">
            {allVenues.length === 0
              ? 'No venues listed here yet.'
              : 'No venues match these filters.'}
          </p>
        ) : (
          <ul>
            {venues.map((venue) => (
              <li key={venue.id}>
                <VenueRow
                  venue={venue}
                  selected={venue.id === selectedVenueId}
                  onSelect={() => onVenueSelect(venue.id)}
                />
              </li>
            ))}
          </ul>
        )}
      </div>

      <footer className="border-t border-border px-4 py-3">
        {/* Provenance. The timestamp is real — the newest updated_at across the
            listed venues. The edit/contributor counts the mock also shows, and
            the confirm action, are PSY-1542's data; the control is rendered
            disabled here so the shape is right and the wiring lands there. */}
        <p
          data-testid="rail-provenance"
          className="font-mono text-[11px] leading-4 text-muted-foreground"
        >
          {/* Full-strength muted, not a dimmed variant: at 11px an opacity
              step lands under the 4.5:1 contrast floor. The line's hierarchy
              comes from the VALUE being brighter, not the label dimmer. */}
          <span className="text-muted-foreground">DATA</span>{' '}
          {updatedAt ? `updated ${formatTimeAgo(updatedAt)}` : 'no update recorded'}
        </p>
        <button
          type="button"
          disabled
          title="Confirming a city’s listings isn’t available yet"
          className="mt-1 cursor-not-allowed font-mono text-[11px] text-primary/50"
        >
          ✓ Confirm this list is current
        </button>
      </footer>
    </aside>
  )
}

function chipClass(active: boolean, disabled = false): string {
  const base =
    'inline-flex items-center rounded-sm px-2 py-0.5 font-mono text-[11px] leading-4 transition-colors'
  if (disabled) return `${base} bg-muted/50 text-muted-foreground/50`
  if (active) return `${base} bg-primary text-background`
  return `${base} bg-muted text-foreground hover:bg-muted/70`
}

function FilterChip({
  children,
  active = false,
  disabled = false,
  title,
  onClick,
}: {
  children: React.ReactNode
  active?: boolean
  disabled?: boolean
  title?: string
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      title={title}
      aria-pressed={disabled ? undefined : active}
      onClick={onClick}
      className={`${chipClass(active, disabled)} ${
        disabled
          ? 'cursor-not-allowed'
          : 'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
      }`}
    >
      {children}
    </button>
  )
}

function VenueRow({
  venue,
  selected,
  onSelect,
}: {
  venue: VenueWithShowCount
  selected: boolean
  onSelect: () => void
}) {
  const nextDate = formatNextShowDate(venue.next_show_date)
  const bill = nextShowBill(venue)
  const genre = venue.dominant_genre
    ? GENRE_LABEL_BY_KEY.get(venue.dominant_genre)
    : undefined

  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={selected ? 'true' : undefined}
      className={`w-full border-b border-border/60 px-4 py-2.5 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring ${
        selected ? 'bg-muted/60' : 'hover:bg-muted/30'
      }`}
    >
      <div className="flex items-baseline justify-between gap-3">
        <span className="truncate text-sm text-foreground">{venue.name}</span>
        <span className="shrink-0 font-mono text-xs text-primary">
          {venue.upcoming_show_count} upcoming
        </span>
      </div>
      {nextDate ? (
        <p className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground">
          {/* See the DATA label: full-strength muted for contrast; the date
              carries the emphasis by being brighter. */}
          <span className="text-muted-foreground">NEXT</span>{' '}
          <span className="text-foreground/70">{nextDate}</span>
          {bill && <> · {bill}</>}
          {genre && <> · {genre}</>}
        </p>
      ) : (
        <p className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground">
          nothing on the calendar
          {genre && <> · {genre}</>}
        </p>
      )}
    </button>
  )
}
