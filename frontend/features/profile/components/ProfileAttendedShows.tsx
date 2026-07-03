'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Lock } from 'lucide-react'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { ProfileSectionAction } from './ProfileSectionAction'
import { useUserAttendedShows } from '@/features/auth'
import { showDisplayTitle } from '@/lib/utils/showDisplayTitle'

interface ProfileAttendedShowsProps {
  username: string
}

function formatDiaryDate(dateString: string): string {
  return new Date(dateString)
    .toLocaleDateString('en-US', {
      month: 'short',
      day: '2-digit',
      year: 'numeric',
    })
    .toUpperCase()
}

/**
 * The concert diary on the public profile (PSY-1045): past approved shows
 * the user marked "going", most recent first.
 *
 * Privacy shapes (server-gated by the `attendance` setting):
 * - visible → dense date/show/venue rows
 * - count_only → total + empty list → single count line
 * - hidden → 404 → a lock notice (per the privacy-applied design board): the
 *   section names itself so the page doesn't silently look incomplete.
 */
// Collapsed row budget per the design board's diary density.
const COLLAPSED_COUNT = 10

export function ProfileAttendedShows({ username }: ProfileAttendedShowsProps) {
  // Fetch the API max up front and slice client-side: the hook's query key
  // doesn't include limit, so a refetch-on-expand would be served from cache
  // and silently no-op. "View all →" reveals the fetched rows in place
  // (decision 2026-06-10: no dedicated per-user list routes yet).
  const [expanded, setExpanded] = useState(false)
  const { data, error } = useUserAttendedShows(username, { limit: 100 })

  const isHidden = (error as { status?: number } | null)?.status === 404

  if (error && !isHidden) return null
  if (!isHidden && (!data || data.total === 0)) return null

  const isCountOnly = Boolean(data && data.total > 0 && data.shows.length === 0)

  return (
    <section aria-label="Shows attended">
      <SectionHeader
        title="Shows attended"
        as="h2"
        size="md"
        variant="title"
        action={
          !expanded && data && data.shows.length > COLLAPSED_COUNT ? (
            <ProfileSectionAction
              label="View all →"
              onClick={() => setExpanded(true)}
              ariaLabel={
                data.total > data.shows.length
                  ? `View the first ${data.shows.length} of ${data.total} attended shows`
                  : `View all ${data.total} attended shows`
              }
            />
          ) : undefined
        }
      />
      {isHidden ? (
        <p className="mt-2 flex items-center gap-1.5 text-sm text-muted-foreground">
          <Lock className="h-3.5 w-3.5" aria-hidden />
          This member keeps their attendance private.
        </p>
      ) : isCountOnly ? (
        <p className="text-sm text-muted-foreground mt-2">
          <span className="text-foreground font-medium tabular-nums">
            {data!.total}
          </span>{' '}
          {data!.total === 1 ? 'show attended' : 'shows attended'} — the diary
          is hidden by this member.
        </p>
      ) : (
        <div className="mt-1 divide-y divide-border/60">
          {(expanded ? data!.shows : data!.shows.slice(0, COLLAPSED_COUNT)).map(show => (
            <div
              key={show.show_id}
              className="flex items-baseline gap-4 py-2 text-sm"
            >
              <span className="w-28 shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
                {formatDiaryDate(show.event_date)}
              </span>
              <span className="min-w-0 flex-1 truncate">
                {show.slug ? (
                  <Link
                    href={`/shows/${show.slug}`}
                    className="hover:text-primary hover:underline"
                  >
                    {showDisplayTitle(show.title, null)}
                  </Link>
                ) : (
                  showDisplayTitle(show.title, null)
                )}
                {show.venue_name && (
                  <span className="text-muted-foreground">
                    {' '}
                    ·{' '}
                    {show.venue_slug ? (
                      <Link
                        href={`/venues/${show.venue_slug}`}
                        className="hover:text-primary hover:underline"
                      >
                        {show.venue_name}
                      </Link>
                    ) : (
                      show.venue_name
                    )}
                  </span>
                )}
                {show.city && (
                  <span className="text-muted-foreground"> · {show.city}</span>
                )}
              </span>
            </div>
          ))}
          {expanded && data!.total > data!.shows.length && (
            <p className="py-2 text-xs text-muted-foreground">
              + {data!.total - data!.shows.length} more
            </p>
          )}
        </div>
      )}
    </section>
  )
}
