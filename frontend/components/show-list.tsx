'use client'

import { useUpcomingShows } from '@/lib/hooks/useShows'
import type { ShowResponse } from '@/lib/types/show'
import Link from 'next/link'

/**
 * Format a date string to "Mon, Dec 1" format
 */
function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Format a date string to "7:30 PM" format
 */
function formatTime(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * Format price as $XX.XX
 */
function formatPrice(price: number): string {
  return `$${price.toFixed(2)}`
}

interface ShowCardProps {
  show: ShowResponse
}

function ShowCard({ show }: ShowCardProps) {
  const venue = show.venues[0] // Primary venue
  const artists = show.artists

  return (
    <article className="border-b border-border/50 py-5 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date and Location */}
        <div className="w-full md:w-1/5 md:pr-4 mb-2 md:mb-0">
          <h2 className="text-sm font-bold tracking-wide text-primary">
            {formatDate(show.event_date)}
          </h2>
          <h3 className="text-xs text-muted-foreground mt-0.5">
            {show.city}, {show.state}
          </h3>
        </div>

        {/* Right column: Artists, Venue, Details */}
        <div className="w-full md:w-4/5 md:pl-4">
          {/* Artists */}
          <h1 className="text-lg font-semibold leading-tight tracking-tight">
            {artists.map((artist, index) => (
              <span key={artist.id}>
                {index > 0 && (
                  <span className="text-muted-foreground/60 font-normal">
                    &nbsp;•&nbsp;
                  </span>
                )}
                {artist.socials?.instagram ? (
                  <a
                    href={`https://instagram.com/${artist.socials.instagram}`}
                    className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {artist.name}
                  </a>
                ) : (
                  <span>{artist.name}</span>
                )}
              </span>
            ))}
          </h1>

          {/* Venue and Details */}
          <div className="text-sm mt-1.5 text-muted-foreground">
            {venue && (
              <Link
                href={`/venues/${venue.id}`}
                className="text-primary/80 hover:text-primary font-medium transition-colors"
              >
                {venue.name}
              </Link>
            )}
            {show.price != null && (
              <span>&nbsp;•&nbsp;{formatPrice(show.price)}</span>
            )}
            {show.age_requirement && (
              <span>&nbsp;•&nbsp;{show.age_requirement}</span>
            )}
            <span>&nbsp;•&nbsp;{formatTime(show.event_date)}</span>
          </div>
        </div>
      </div>
    </article>
  )
}

export function ShowList() {
  const { data, isLoading, error } = useUpcomingShows({
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center items-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load shows. Please try again later.</p>
      </div>
    )
  }

  if (!data?.shows || data.shows.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <p>No upcoming shows at this time.</p>
      </div>
    )
  }

  return (
    <section className="w-full max-w-4xl">
      {data.shows.map(show => (
        <ShowCard key={show.id} show={show} />
      ))}

      {data.pagination.has_more && (
        <div className="text-center py-6">
          <p className="text-muted-foreground text-sm">
            More shows available...
          </p>
        </div>
      )}
    </section>
  )
}
