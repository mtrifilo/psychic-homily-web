'use client'

import { BracketLink } from '@/components/shared/BracketLink'
import { airDateCellText } from '@/features/radio'
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
  // PSY-1306: nav dates render viewer-local (same derivation as the feeds);
  // hrefs stay keyed on the station-dated air_date.
  const olderDate = older
    ? airDateCellText(older.starts_at, older.ends_at, older.air_date).dateLine
    : ''
  const newerDate = newer
    ? airDateCellText(newer.starts_at, newer.ends_at, newer.air_date).dateLine
    : ''

  return (
    <nav
      aria-label="Episode navigation"
      className="flex items-center gap-3 font-mono text-xs"
    >
      <BracketLink
        label={older ? `◀ ${olderDate}` : '◀ older'}
        href={older ? `${showUrl}/${older.air_date}` : undefined}
        disabled={!older}
        ariaLabel={
          older ? `Previous episode, ${olderDate}` : 'No older episode'
        }
        className="text-xs"
      />
      <BracketLink
        label={newer ? `${newerDate} ▶` : 'newer ▶'}
        href={newer ? `${showUrl}/${newer.air_date}` : undefined}
        disabled={!newer}
        ariaLabel={
          newer ? `Next episode, ${newerDate}` : 'No newer episode'
        }
        className="text-xs"
      />
      <BracketLink label="all episodes" href={showUrl} className="text-xs" />
    </nav>
  )
}
