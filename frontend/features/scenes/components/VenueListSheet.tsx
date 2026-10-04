'use client'

import { useMemo } from 'react'
import { BottomSheet, type BottomSheetDetent } from '@/components/ui/bottom-sheet'
import type { VenueWithShowCount } from '@/features/venues/types'
import {
  ATLAS_SHEET_TOP_INSET_PX,
  cityRailStats,
  venueSheetPeekLine,
  venueSheetTitle,
  venueStackScopeLine,
  venuesSpanMetro,
  type CityVenueFilters,
  type VenuePinStack,
} from '../cityView'
import {
  VenueRailFilters,
  VenueRailList,
  VenueRailProvenance,
  VenueRailTruncationNote,
} from './VenueRail'

interface VenueListSheetProps {
  /** The metro's principal city on its own, e.g. "Chicago". */
  principalCity: string
  /** Venues AFTER filtering: the same array the map pins. */
  venues: readonly VenueWithShowCount[]
  /** Venues BEFORE filtering, for the title count and the genre menu. */
  allVenues: readonly VenueWithShowCount[]
  /** The API's total for the city; above `allVenues.length` means truncated. */
  totalVenueCount?: number
  loading?: boolean
  fetchFailed?: boolean
  filters: CityVenueFilters
  onFiltersChange: (filters: CityVenueFilters) => void
  selectedVenueId: number | null
  onVenueSelect: (venueId: number) => void
  detent: BottomSheetDetent
  onDetentChange: (detent: BottomSheetDetent) => void
  /** Points where several of `venues` pin, for the Peek line. */
  stacks: readonly VenuePinStack[]
  /** Pins inside the map's viewport; null until the map reports bounds. */
  inViewCount: number | null
  /** When set, the rows are only this stack's venues. */
  scopedStack: VenuePinStack | null
  onClearScope: () => void
  /** Kept mounted but not displayed (another sheet is in front). */
  hidden?: boolean
}

/**
 * The city view's venue list as a bottom sheet: the desktop rail's rows and
 * filters for panes too narrow to put the rail beside the map. Every listed
 * venue is a row here, including the ones that share a pin or have none, so
 * the whole list is reachable without touching the map.
 *
 * It has no close control: it is the city view's standing list, as the rail
 * is on desktop. Escape collapses it to Peek.
 */
export function VenueListSheet({
  principalCity,
  venues,
  allVenues,
  totalVenueCount,
  loading = false,
  fetchFailed = false,
  filters,
  onFiltersChange,
  selectedVenueId,
  onVenueSelect,
  detent,
  onDetentChange,
  stacks,
  inViewCount,
  scopedStack,
  onClearScope,
  hidden = false,
}: VenueListSheetProps) {
  const stats = useMemo(() => cityRailStats(allVenues), [allVenues])
  const spansMetro = useMemo(
    () => venuesSpanMetro(allVenues, principalCity),
    [allVenues, principalCity],
  )
  const rows = useMemo(() => {
    if (!scopedStack) return venues
    const ids = new Set(scopedStack.venueIds)
    return venues.filter((v) => ids.has(v.id))
  }, [venues, scopedStack])
  const peekLine = venueSheetPeekLine({ inViewCount, stacks })

  return (
    <BottomSheet
      title={venueSheetTitle(principalCity, stats.venueCount, spansMetro)}
      label={`${principalCity} venues`}
      aria-label={`Venues in ${principalCity}`}
      data-testid="atlas-venue-sheet"
      detent={detent}
      onDetentChange={onDetentChange}
      onDismiss={() => onDetentChange('peek')}
      topInsetPx={ATLAS_SHEET_TOP_INSET_PX}
      flushBody
      className={hidden ? 'hidden' : undefined}
    >
      {detent === 'peek' && peekLine && (
        <p
          data-testid="venue-sheet-peek-line"
          className="px-4 pt-1 font-mono text-[11px] leading-4 text-muted-foreground"
        >
          {peekLine}
        </p>
      )}
      {/* Mounted at every detent and only hidden at Peek, so expanding keeps
          the rows and their scroll position instead of rebuilding them. */}
      <div className={detent === 'peek' ? 'hidden' : undefined}>
        <div className="space-y-2 px-4 pb-3">
          {scopedStack ? (
            <div className="flex items-center justify-between gap-3">
              <p
                data-testid="venue-sheet-scope-line"
                className="font-mono text-[11px] leading-4 text-muted-foreground"
              >
                {venueStackScopeLine(scopedStack)}
              </p>
              <button
                type="button"
                onClick={onClearScope}
                className="min-h-6 shrink-0 rounded-sm px-2 font-mono text-[11px] text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                Show all
              </button>
            </div>
          ) : (
            <VenueRailTruncationNote
              listedCount={allVenues.length}
              totalVenueCount={totalVenueCount}
            />
          )}
          <VenueRailFilters
            allVenues={allVenues}
            filters={filters}
            onFiltersChange={onFiltersChange}
            touch
          />
        </div>
        <div className="border-t border-border">
          <VenueRailList
            venues={rows}
            allVenues={allVenues}
            principalCity={principalCity}
            totalVenueCount={totalVenueCount}
            loading={loading}
            fetchFailed={fetchFailed}
            filters={filters}
            selectedVenueId={selectedVenueId}
            onVenueSelect={onVenueSelect}
          />
        </div>
        <div className="border-t border-border px-4 py-3">
          <VenueRailProvenance allVenues={allVenues} />
        </div>
      </div>
    </BottomSheet>
  )
}
