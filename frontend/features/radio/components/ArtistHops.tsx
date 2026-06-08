'use client'

import Link from 'next/link'
import { Fragment } from 'react'
import type { ArtistHop } from '../lib/stationOverview'

interface ArtistHopsProps {
  hops: ArtistHop[]
  /** Called when a hop link is followed (e.g. to close the nav panel). */
  onNavigate?: () => void
  className?: string
}

/**
 * Renders a "·"-separated run of artist names where each linked artist is a
 * one-click hop into the knowledge graph (PSY-1016). Unlinked artists (the
 * matching engine hasn't resolved them yet) render as plain text so the row
 * never carries a dead link.
 */
export function ArtistHops({ hops, onNavigate, className }: ArtistHopsProps) {
  if (hops.length === 0) return null

  return (
    <span className={className}>
      {hops.map((hop, i) => (
        <Fragment key={`${hop.name}-${i}`}>
          {i > 0 && <span className="text-muted-foreground/50"> · </span>}
          {hop.slug ? (
            <Link
              href={`/artists/${hop.slug}`}
              onClick={onNavigate}
              className="hover:text-primary transition-colors"
            >
              {hop.name}
            </Link>
          ) : (
            <span>{hop.name}</span>
          )}
        </Fragment>
      ))}
    </span>
  )
}
