'use client'

import { BracketLink } from '@/components/shared/BracketLink'
import { formatShortNavDate } from '@/features/radio'
import type { EpisodeNeighbors } from '@/features/radio'

interface EpisodeNavProps {
  /** Prev/next neighbors; undefined while the neighbor walk is in flight. */
  neighbors: EpisodeNeighbors | undefined
  /** Base /radio/{station}/{show} URL; episode URLs append /{air_date}. */
  showUrl: string
}

/**
 * Prev/next episode navigation for the playlist page (PSY-1051): an older
 * bracket on the left, a newer bracket on the right, and an [all episodes]
 * hop back to the show's archive. At the oldest/newest episode (or while the
 * neighbor walk is loading) the corresponding bracket renders disabled
 * rather than disappearing, so the row doesn't jump.
 */
export function EpisodeNav({ neighbors, showUrl }: EpisodeNavProps) {
  const older = neighbors?.older ?? null
  const newer = neighbors?.newer ?? null

  return (
    <nav
      aria-label="Episode navigation"
      className="flex items-center gap-3 font-mono text-xs"
    >
      <BracketLink
        label={older ? `◀ ${formatShortNavDate(older.air_date)}` : '◀ older'}
        href={older ? `${showUrl}/${older.air_date}` : undefined}
        disabled={!older}
        ariaLabel={
          older ? `Previous episode, ${formatShortNavDate(older.air_date)}` : 'No older episode'
        }
        className="text-xs"
      />
      <BracketLink
        label={newer ? `${formatShortNavDate(newer.air_date)} ▶` : 'newer ▶'}
        href={newer ? `${showUrl}/${newer.air_date}` : undefined}
        disabled={!newer}
        ariaLabel={
          newer ? `Next episode, ${formatShortNavDate(newer.air_date)}` : 'No newer episode'
        }
        className="text-xs"
      />
      <BracketLink label="all episodes" href={showUrl} className="text-xs" />
    </nav>
  )
}
