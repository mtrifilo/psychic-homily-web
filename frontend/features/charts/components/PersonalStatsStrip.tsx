'use client'

import Link from 'next/link'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useContributeOpportunities,
  FOLLOWED_LOOSE_ENDS_KEY,
} from '@/features/contributions'
import { usePersonalChartsStats } from '../hooks'
import type { PersonalChartsStats } from '../types'

function firstActivityLabel(value: string | null): string | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return date.toLocaleDateString('en-US', {
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  })
}

function hasPersonalHistory(stats: PersonalChartsStats): boolean {
  return (
    stats.saved_shows > 0 ||
    stats.artists_followed > 0 ||
    stats.top_venue !== null ||
    stats.first_activity_at !== null
  )
}

function PersonalStats({ stats }: { stats: PersonalChartsStats }) {
  if (!hasPersonalHistory(stats)) {
    return <p>Mark shows you&apos;re going to and this fills in</p>
  }

  const facts = [
    `${stats.saved_shows} ${stats.saved_shows === 1 ? 'show' : 'shows'} marked`,
    `${stats.artists_followed} ${stats.artists_followed === 1 ? 'artist' : 'artists'} followed`,
  ]

  if (stats.top_venue) {
    facts.push(
      `top venue: ${stats.top_venue.name} (${stats.top_venue.saved_show_count})`
    )
  }

  const firstActivity = firstActivityLabel(stats.first_activity_at)
  if (firstActivity) facts.push(`first logged: ${firstActivity}`)

  return <p>{facts.join(' · ')}</p>
}

export function PersonalStatsStrip() {
  const { isAuthenticated, isLoading: isAuthLoading, user } = useAuthContext()
  const stats = usePersonalChartsStats(
    user?.id,
    isAuthenticated && !isAuthLoading
  )
  // PSY-1484: deep link from the Broadsheet into the /contribute "Loose Ends"
  // band when the viewer follows artists that are missing streaming links.
  // Gated on auth so anonymous chart viewers don't fire the request; the
  // followed loose-ends category is itself authed-only server-side.
  const opportunities = useContributeOpportunities({
    enabled: isAuthenticated && !isAuthLoading,
  })
  const followedLooseEndsCount =
    opportunities.data?.categories?.find(
      (category) => category.key === FOLLOWED_LOOSE_ENDS_KEY
    )?.count ?? 0

  if (!isAuthenticated || stats.isError) return null
  if (!isAuthLoading && !stats.isLoading && !stats.data) return null

  return (
    <section
      aria-label="Your chart stats"
      className="flex min-h-[38px] flex-wrap items-center gap-x-4 gap-y-1 border border-primary bg-primary/10 px-3.5 py-2.5 font-mono text-xs leading-normal"
    >
      <span className="shrink-0 text-[11px] font-bold tracking-[0.06em] text-primary">
        YOU
      </span>
      <div className="min-w-0 flex-1">
        {stats.isLoading || isAuthLoading ? (
          <span
            aria-label="Loading your chart stats"
            className="block h-3 w-3/4 animate-pulse rounded-sm bg-primary/15"
          />
        ) : stats.data ? (
          <PersonalStats stats={stats.data} />
        ) : null}
      </div>
      {followedLooseEndsCount > 0 ? (
        <Link
          href="/contribute"
          className="shrink-0 font-bold text-primary hover:underline focus-visible:underline focus-visible:outline-none"
        >
          {followedLooseEndsCount} loose{' '}
          {followedLooseEndsCount === 1 ? 'end' : 'ends'} in your follows →
        </Link>
      ) : null}
    </section>
  )
}
