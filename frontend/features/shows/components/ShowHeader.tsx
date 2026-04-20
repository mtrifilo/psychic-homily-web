'use client'

import Link from 'next/link'
import { ExternalLink, MapPin } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { formatShowDate, formatShowTime, formatPrice } from '@/lib/utils/formatters'
import type { ShowResponse } from '../types'

interface ShowHeaderProps {
  show: ShowResponse
  /**
   * Action cluster rendered on the right side of the header (desktop) or
   * below the artist/venue block (mobile). Typically a `<ShowActions />`.
   */
  actions?: React.ReactNode
}

/**
 * ShowDetail-specific header block. Owns the bill-position artist rendering
 * (headliners as h1, support as "w/ ..." row), venue prominence block
 * (name link + MapPin + "see more shows at {venue}" link), date + sold-out
 * badge row, show meta row (time / price / age), ticket URL CTA, and
 * description paragraph.
 *
 * This intentionally diverges from the generic `EntityHeader` — the bill
 * position semantics (`set_type`) and the co-primary venue entity don't
 * fit into `EntityHeader`'s single-string `title` / subtitle-badge shape.
 * See `docs/learnings/entity-detail-layout-migration.md` for rationale.
 */
export function ShowHeader({ show, actions }: ShowHeaderProps) {
  const venue = show.venues[0]
  const artists = show.artists

  const headliners = artists.filter(
    a => a.set_type === 'headliner' || a.is_headliner === true
  )
  const support = artists.filter(
    a => a.set_type !== 'headliner' && a.is_headliner !== true
  )
  const effectiveHeadliners =
    headliners.length > 0 ? headliners : artists.length > 0 ? [artists[0]] : []
  const effectiveSupport = headliners.length > 0 ? support : artists.slice(1)

  return (
    <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
      <div className="flex-1 min-w-0">
        {/* Date and Status Badges */}
        <div className="flex items-center gap-2 mb-2">
          <span className="text-lg font-bold text-primary">
            {formatShowDate(show.event_date, show.state)}
          </span>
          {show.is_sold_out && (
            <Badge
              variant="secondary"
              className="text-xs font-semibold bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400"
            >
              SOLD OUT
            </Badge>
          )}
        </div>

        {/* Artists — grouped by billing */}
        <h1 className="text-2xl md:text-3xl font-bold leading-8 md:leading-9">
          {effectiveHeadliners.map((artist, index) => (
            <span key={artist.id}>
              {index > 0 && (
                <span className="text-muted-foreground/60 font-normal">
                  {' '}
                  &bull;{' '}
                </span>
              )}
              {artist.slug ? (
                <Link
                  href={`/artists/${artist.slug}`}
                  className="hover:text-primary transition-colors"
                >
                  {artist.name}
                </Link>
              ) : (
                <span>{artist.name}</span>
              )}
            </span>
          ))}
        </h1>
        {effectiveSupport.length > 0 && (
          <div className="text-lg text-muted-foreground mt-1">
            <span className="italic">w/</span>{' '}
            {effectiveSupport.map((artist, index) => (
              <span key={artist.id}>
                {index > 0 && (
                  <span className="text-muted-foreground/50">, </span>
                )}
                {artist.slug ? (
                  <Link
                    href={`/artists/${artist.slug}`}
                    className="hover:text-primary/80 transition-colors"
                  >
                    {artist.name}
                  </Link>
                ) : (
                  <span>{artist.name}</span>
                )}
                {artist.set_type === 'special_guest' && (
                  <span className="text-sm text-muted-foreground/70 italic">
                    {' '}
                    (special guest)
                  </span>
                )}
              </span>
            ))}
          </div>
        )}

        {/* Venue and Location */}
        {venue && (
          <div className="mt-2">
            {venue.slug ? (
              <Link
                href={`/venues/${venue.slug}`}
                className="text-lg text-primary/80 hover:text-primary font-medium transition-colors"
              >
                {venue.name}
              </Link>
            ) : (
              <span className="text-lg text-primary/80 font-medium">
                {venue.name}
              </span>
            )}
            <div className="flex items-center gap-1 text-muted-foreground mt-1">
              <MapPin className="h-4 w-4" />
              <span>
                {venue.city}, {venue.state}
              </span>
            </div>
            {venue.slug && (
              <Link
                href={`/venues/${venue.slug}`}
                className="inline-block text-sm text-muted-foreground hover:text-primary transition-colors mt-1"
              >
                See more shows at {venue.name} &rarr;
              </Link>
            )}
          </div>
        )}

        {/* Show Details */}
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground mt-3">
          <span>{formatShowTime(show.event_date, show.state)}</span>
          {show.price != null && <span>{formatPrice(show.price)}</span>}
          {show.age_requirement && <span>{show.age_requirement}</span>}
        </div>

        {/* Ticket URL */}
        {show.ticket_url && (
          <div className="mt-3">
            <a
              href={
                show.ticket_url.startsWith('http')
                  ? show.ticket_url
                  : `https://${show.ticket_url}`
              }
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
            >
              Buy Tickets
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          </div>
        )}

        {/* Description */}
        {show.description && (
          <p className="mt-4 text-muted-foreground">{show.description}</p>
        )}
      </div>

      {/* Action cluster (attendance + save + admin status toggles) */}
      {actions && (
        <div className="flex flex-col items-start sm:items-end gap-2 sm:shrink-0">
          {actions}
        </div>
      )}
    </div>
  )
}
