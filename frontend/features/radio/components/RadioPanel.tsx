'use client'

import { useState } from 'react'
import { Loader2, Radio } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useRadioStations } from '../hooks/useRadioStations'
import { isStationVisibleOnIndex } from '../types'
import { RadioStationList } from './RadioStationList'
import { RadioStationOverview } from './RadioStationOverview'

interface RadioPanelProps {
  /**
   * Called when an internal link inside the panel is followed. The nav popover
   * passes a close handler so navigating dismisses the panel; the /radio page
   * leaves it undefined.
   */
  onNavigate?: () => void
  className?: string
}

/**
 * The Option-D2 "station overview" two-pane panel (PSY-1016): a selectable
 * station list (left) + the selected station's overview — identity, Now
 * Playing, recent shows (right). Click-to-open, keyboard-operable.
 *
 * Shared verbatim between the Radio nav popover (RadioMenu) and the /radio
 * page; the page just renders it in a wider container. Real stations only
 * (KEXP / WFMU / NTS) — non-flagship network siblings are filtered out the
 * same way the /radio index does.
 */
export function RadioPanel({ onNavigate, className }: RadioPanelProps) {
  const { data, isLoading, error } = useRadioStations()
  const stations = (data?.stations ?? []).filter(isStationVisibleOnIndex)

  // Only the explicit user choice is stored; the effective selection is
  // derived below (defaulting to the first station). Deriving rather than
  // syncing via an effect avoids a cascading-render setState-in-effect.
  const [chosenSlug, setChosenSlug] = useState<string | null>(null)

  // The chosen station if it's still present, else the first visible station.
  const selectedSlug =
    (chosenSlug && stations.some(s => s.slug === chosenSlug) ? chosenSlug : null) ??
    stations[0]?.slug ??
    ''

  if (isLoading) {
    return (
      <div className={cn('flex items-center justify-center py-16', className)}>
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || stations.length === 0) {
    return (
      <div
        className={cn(
          'flex flex-col items-center justify-center gap-1 py-16 text-center',
          className
        )}
      >
        <Radio className="size-7 text-muted-foreground/30" />
        <p className="text-sm text-muted-foreground">
          {error ? "Couldn't load radio stations." : 'No radio stations yet.'}
        </p>
      </div>
    )
  }

  return (
    <div className={cn('flex items-stretch', className)}>
      <div className="w-[220px] shrink-0 border-r border-border px-3.5 py-[18px]">
        <RadioStationList
          stations={stations}
          selectedSlug={selectedSlug}
          onSelect={setChosenSlug}
        />
      </div>
      {selectedSlug && (
        <RadioStationOverview stationSlug={selectedSlug} onNavigate={onNavigate} />
      )}
    </div>
  )
}
