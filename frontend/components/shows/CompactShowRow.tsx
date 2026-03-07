'use client'

import Link from 'next/link'
import { formatPrice, formatShowTime } from '@/lib/utils/formatters'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'

interface CompactShowArtist {
  id: number
  name: string
  slug?: string | null
}

interface CompactShowData {
  id: number
  slug?: string | null
  event_date: string
  price?: number | null
  artists: CompactShowArtist[]
}

interface CompactShowVenue {
  name: string
  slug?: string | null
  city?: string | null
  state?: string | null
}

interface CompactShowRowProps {
  show: CompactShowData
  state: string | null | undefined
  isPastShow?: boolean
  showDetailsLink?: boolean
  showVenueLine?: boolean
  venue?: CompactShowVenue | null
  primaryLine?: 'artists' | 'venue'
  secondaryArtists?: CompactShowArtist[]
  secondaryArtistsPrefix?: string
}

function ArtistLinks({ artists }: { artists: CompactShowArtist[] }) {
  if (artists.length === 0) {
    return <>TBA</>
  }

  return (
    <>
      {artists.map((artist, index) => (
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
              className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
            >
              {artist.name}
            </Link>
          ) : (
            <span>{artist.name}</span>
          )}
        </span>
      ))}
    </>
  )
}

function VenueLine({ venue }: { venue: CompactShowVenue | null | undefined }) {
  if (!venue) {
    return null
  }

  return (
    <div className="text-sm text-muted-foreground mt-0.5">
      {venue.slug ? (
        <Link
          href={`/venues/${venue.slug}`}
          className="text-primary/80 hover:text-primary font-medium transition-colors"
        >
          {venue.name}
        </Link>
      ) : (
        <span className="text-primary/80 font-medium">{venue.name}</span>
      )}
      {(venue.city || venue.state) && (
        <span className="text-muted-foreground/80">
          {' '}
          &middot; {[venue.city, venue.state].filter(Boolean).join(', ')}
        </span>
      )}
    </div>
  )
}

export function CompactShowRow({
  show,
  state,
  isPastShow = false,
  showDetailsLink = true,
  showVenueLine = false,
  venue,
  primaryLine = 'artists',
  secondaryArtists = [],
  secondaryArtistsPrefix = 'w/',
}: CompactShowRowProps) {
  const timezoneState = state || 'AZ'
  const detailsHref = `/shows/${show.slug || show.id}`
  const dateBadge = formatShowDateBadge(show.event_date, timezoneState)

  return (
    <div className="py-2.5 border-b border-border/30 last:border-b-0">
      <div className="flex gap-3">
        {/* Compact date badge */}
        <Link
          href={detailsHref}
          className="shrink-0 flex flex-col items-center justify-center w-12 rounded bg-muted/40 hover:bg-muted transition-colors py-1.5"
        >
          <span className="text-[9px] font-bold tracking-widest uppercase text-primary leading-none">
            {dateBadge.dayOfWeek}
          </span>
          <span className="text-[11px] font-semibold text-muted-foreground leading-tight mt-0.5">
            {dateBadge.monthDay}
          </span>
        </Link>

        {/* Content */}
        <div className="flex-1 min-w-0 flex items-start justify-between gap-2">
          <div className="flex-1 min-w-0">
            {primaryLine === 'artists' ? (
              <>
                <div className="text-sm font-semibold truncate">
                  <ArtistLinks artists={show.artists} />
                </div>
                {showVenueLine && <VenueLine venue={venue} />}
              </>
            ) : (
              <>
                {venue ? (
                  <div>
                    {venue.slug ? (
                      <Link
                        href={`/venues/${venue.slug}`}
                        className="text-sm font-semibold hover:text-primary transition-colors"
                      >
                        {venue.name}
                      </Link>
                    ) : (
                      <span className="text-sm font-semibold">{venue.name}</span>
                    )}
                    {(venue.city || venue.state) && (
                      <span className="text-muted-foreground/80 text-sm">
                        {' '}
                        &middot; {[venue.city, venue.state].filter(Boolean).join(', ')}
                      </span>
                    )}
                  </div>
                ) : (
                  <div className="text-sm font-semibold">Venue TBA</div>
                )}
              </>
            )}

            {secondaryArtists.length > 0 && (
              <div className="text-xs text-muted-foreground mt-0.5">
                <span className="italic">{secondaryArtistsPrefix}</span>{' '}
                <ArtistLinks artists={secondaryArtists} />
              </div>
            )}
          </div>

          <div className="text-right text-xs text-muted-foreground shrink-0">
            <div className="font-medium text-foreground/80">
              {formatShowTime(show.event_date, timezoneState)}
            </div>
            {show.price != null && <div>{formatPrice(show.price)}</div>}
            {showDetailsLink && (
              <div className="mt-0.5">
                <Link
                  href={detailsHref}
                  className="text-primary/80 hover:text-primary underline underline-offset-2 transition-colors"
                >
                  Details
                </Link>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
