'use client'

import { useSavedShows } from '@/lib/hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { redirect } from 'next/navigation'
import Link from 'next/link'
import { Heart, Loader2 } from 'lucide-react'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import type { SavedShowResponse } from '@/lib/types/show'
import { SaveButton } from '@/components/SaveButton'

function formatDate(dateString: string, state?: string | null): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatDateInTimezone(dateString, timezone)
}

function formatTime(dateString: string, state?: string | null): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatTimeInTimezone(dateString, timezone)
}

function formatPrice(price: number): string {
  return `$${price.toFixed(2)}`
}

interface SavedShowCardProps {
  show: SavedShowResponse
}

function SavedShowCard({ show }: SavedShowCardProps) {
  const venue = show.venues[0]
  const artists = show.artists

  return (
    <article className="border-b border-border/50 py-5 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date and Location */}
        <div className="w-full md:w-1/5 md:pr-4 mb-2 md:mb-0">
          <h2 className="text-sm font-bold tracking-wide text-primary">
            {formatDate(show.event_date, show.state)}
          </h2>
          <h3 className="text-xs text-muted-foreground mt-0.5">
            {show.city}, {show.state}
          </h3>
        </div>

        {/* Right column: Artists, Venue, Details */}
        <div className="w-full md:w-4/5 md:pl-4">
          <div className="flex items-start justify-between gap-2">
            {/* Artists */}
            <h1 className="text-lg font-semibold leading-tight tracking-tight flex-1">
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

            {/* Unsave Button */}
            <SaveButton showId={show.id} variant="ghost" size="sm" />
          </div>

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
            <span>&nbsp;•&nbsp;{formatTime(show.event_date, show.state)}</span>
          </div>
        </div>
      </div>
    </article>
  )
}

export default function SavedShowsPage() {
  const { isAuthenticated, isLoading: authLoading } = useAuthContext()
  const { data, isLoading, error } = useSavedShows()

  // Redirect if not authenticated
  if (!authLoading && !isAuthenticated) {
    redirect('/auth')
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="container max-w-4xl mx-auto px-4 py-12">
        <div className="text-center text-destructive">
          <p>Failed to load your saved shows. Please try again later.</p>
        </div>
      </div>
    )
  }

  const shows = data?.shows || []
  const total = data?.total || 0

  return (
    <div className="container max-w-4xl mx-auto px-4 py-12">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <Heart className="h-8 w-8 fill-red-500 text-red-500" />
          <h1 className="text-3xl font-bold tracking-tight">My List</h1>
        </div>
        <p className="text-muted-foreground">
          {total === 0
            ? 'No saved shows yet'
            : `${total} saved ${total === 1 ? 'show' : 'shows'}`}
        </p>
      </div>

      {/* Shows List */}
      {shows.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <Heart className="h-16 w-16 mx-auto mb-4 text-muted-foreground/30" />
          <p className="text-lg mb-2">Your list is empty</p>
          <p className="text-sm">
            Save shows by clicking the heart icon on any show
          </p>
          <Link
            href="/shows"
            className="inline-block mt-6 px-6 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
          >
            Browse Shows
          </Link>
        </div>
      ) : (
        <section className="w-full">
          {shows.map(show => (
            <SavedShowCard key={show.id} show={show} />
          ))}
        </section>
      )}
    </div>
  )
}
