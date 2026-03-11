'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import type { FestivalArtist, BillingTier } from '../types'
import {
  BILLING_TIER_ORDER,
  getBillingTierLabel,
} from '../types'

interface FestivalLineupProps {
  artists: FestivalArtist[]
  /** If true, group by day_date before billing tier */
  multiDay?: boolean
}

interface TierGroup {
  tier: BillingTier
  artists: FestivalArtist[]
}

interface DayGroup {
  date: string | null
  label: string
  tiers: TierGroup[]
}

/**
 * Groups artists by billing tier, maintaining the tier ordering.
 */
function groupByTier(artists: FestivalArtist[]): TierGroup[] {
  const tierMap = new Map<string, FestivalArtist[]>()

  for (const artist of artists) {
    const tier = artist.billing_tier || 'mid_card'
    if (!tierMap.has(tier)) {
      tierMap.set(tier, [])
    }
    tierMap.get(tier)!.push(artist)
  }

  // Return in the defined display order
  const groups: TierGroup[] = []
  for (const tier of BILLING_TIER_ORDER) {
    const tierArtists = tierMap.get(tier)
    if (tierArtists && tierArtists.length > 0) {
      groups.push({ tier, artists: tierArtists })
    }
  }

  // Include any unrecognized tiers at the end
  for (const [tier, tierArtists] of tierMap) {
    if (!BILLING_TIER_ORDER.includes(tier as BillingTier) && tierArtists.length > 0) {
      groups.push({ tier: tier as BillingTier, artists: tierArtists })
    }
  }

  return groups
}

/**
 * Format a day date for display as a day header.
 */
function formatDayLabel(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  })
}

/**
 * Tiered lineup display component that visually represents festival billing.
 * Headliners get large text, sub-headliners medium, and lower tiers progressively smaller.
 * Artists are links to their artist pages.
 */
export function FestivalLineup({ artists, multiDay = false }: FestivalLineupProps) {
  const dayGroups = useMemo((): DayGroup[] => {
    if (!multiDay) {
      return [
        {
          date: null,
          label: '',
          tiers: groupByTier(artists),
        },
      ]
    }

    // Group by day_date first
    const dayMap = new Map<string, FestivalArtist[]>()
    const unassigned: FestivalArtist[] = []

    for (const artist of artists) {
      if (artist.day_date) {
        if (!dayMap.has(artist.day_date)) {
          dayMap.set(artist.day_date, [])
        }
        dayMap.get(artist.day_date)!.push(artist)
      } else {
        unassigned.push(artist)
      }
    }

    // Sort days chronologically
    const sortedDays = Array.from(dayMap.entries()).sort(([a], [b]) =>
      a.localeCompare(b)
    )

    const groups: DayGroup[] = sortedDays.map(([date, dayArtists]) => ({
      date,
      label: formatDayLabel(date),
      tiers: groupByTier(dayArtists),
    }))

    if (unassigned.length > 0) {
      groups.push({
        date: null,
        label: 'Additional Artists',
        tiers: groupByTier(unassigned),
      })
    }

    return groups
  }, [artists, multiDay])

  if (artists.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <p>No artists have been announced for this festival yet.</p>
      </div>
    )
  }

  return (
    <div className="space-y-8">
      {dayGroups.map((day, dayIdx) => (
        <div key={day.date ?? `day-${dayIdx}`}>
          {day.label && (
            <h3 className="text-lg font-semibold mb-4 pb-2 border-b border-border/50">
              {day.label}
            </h3>
          )}

          <div className="space-y-6">
            {day.tiers.map(tierGroup => (
              <TierSection key={tierGroup.tier} tierGroup={tierGroup} />
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function TierSection({ tierGroup }: { tierGroup: TierGroup }) {
  const { tier, artists } = tierGroup

  return (
    <div>
      <h4
        className={cn(
          'uppercase tracking-wider mb-3',
          tier === 'headliner' && 'text-xs font-bold text-primary/80',
          tier === 'sub_headliner' && 'text-xs font-semibold text-muted-foreground',
          tier !== 'headliner' &&
            tier !== 'sub_headliner' &&
            'text-[11px] font-medium text-muted-foreground/70'
        )}
      >
        {getBillingTierLabel(tier)}
      </h4>

      <div
        className={cn(
          'flex flex-wrap items-baseline',
          tier === 'headliner' && 'gap-x-6 gap-y-2',
          tier === 'sub_headliner' && 'gap-x-4 gap-y-1.5',
          tier !== 'headliner' &&
            tier !== 'sub_headliner' &&
            'gap-x-3 gap-y-1'
        )}
      >
        {artists.map((artist, idx) => (
          <span key={artist.id} className="inline-flex items-baseline">
            <Link
              href={artist.artist_slug ? `/artists/${artist.artist_slug}` : '#'}
              className={cn(
                'transition-colors hover:text-primary',
                tier === 'headliner' &&
                  'text-xl md:text-2xl font-black text-foreground',
                tier === 'sub_headliner' &&
                  'text-lg md:text-xl font-bold text-foreground/90',
                tier === 'mid_card' &&
                  'text-base font-semibold text-foreground/80',
                tier === 'undercard' &&
                  'text-sm font-medium text-foreground/70',
                (tier === 'local' || tier === 'dj' || tier === 'host') &&
                  'text-sm font-normal text-muted-foreground'
              )}
            >
              {artist.artist_name}
            </Link>
            {idx < artists.length - 1 && (
              <span
                className={cn(
                  'mx-1 select-none',
                  tier === 'headliner'
                    ? 'text-primary/30 text-xl'
                    : tier === 'sub_headliner'
                      ? 'text-muted-foreground/30 text-lg'
                      : 'text-muted-foreground/20 text-sm'
                )}
              >
                {'\u00b7'}
              </span>
            )}
          </span>
        ))}
      </div>
    </div>
  )
}
