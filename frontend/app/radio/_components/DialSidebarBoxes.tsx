'use client'

import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { BracketLink } from '@/components/shared/BracketLink'
import { StatsList } from '@/components/shared/StatsList'
import { getNewReleaseHref } from '@/features/radio'
import type { RadioNewReleaseRadarEntry, RadioStats } from '@/features/radio'

/**
 * Sidebar boxes for The Dial hub (PSY-1049): hairline-bordered Gazelle-style
 * stat boxes with mono headers. Presentational — the hub owns the fetches.
 */

function SidebarBox({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="rounded-md border border-border">
      <h2 className="border-b border-border bg-muted/40 px-3.5 py-2 font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
        {title}
      </h2>
      <div className="px-3.5 py-3">{children}</div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// New Release Radar
// ---------------------------------------------------------------------------

export function NewReleaseRadarBox({
  releases,
  isLoading,
}: {
  releases: RadioNewReleaseRadarEntry[] | undefined
  isLoading: boolean
}) {
  if (isLoading) {
    return (
      <SidebarBox title="New release radar">
        <div className="flex justify-center py-3">
          <Loader2 className="size-4 animate-spin text-muted-foreground" />
          <span className="sr-only">Loading new release radar</span>
        </div>
      </SidebarBox>
    )
  }

  if (!releases || releases.length === 0) return null

  return (
    <SidebarBox title="New release radar">
      <ul className="flex flex-col gap-2.5">
        {releases.map((entry, idx) => {
          const title = entry.album_title
            ? `${entry.artist_name} — ${entry.album_title}`
            : entry.artist_name
          // Link the whole "Artist — Album" line to the release when matched,
          // else to the artist, else plain text (no dead links). Shared with
          // the /radio/new-releases full view (PSY-1076).
          const href = getNewReleaseHref(entry)
          const subline = [
            entry.label_name,
            `${entry.play_count} ${entry.play_count === 1 ? 'play' : 'plays'}`,
            `${entry.station_count} ${entry.station_count === 1 ? 'station' : 'stations'}`,
          ]
            .filter(Boolean)
            .join(' · ')

          return (
            <li
              key={`${entry.artist_name}-${entry.album_title}-${idx}`}
              className="min-w-0"
            >
              {href ? (
                <Link
                  href={href}
                  className="text-sm font-medium text-primary transition-colors hover:underline"
                >
                  {title}
                </Link>
              ) : (
                <span className="text-sm font-medium text-foreground">
                  {title}
                </span>
              )}
              <p className="font-mono text-[11px] leading-4 text-muted-foreground">
                {subline}
              </p>
            </li>
          )
        })}
      </ul>
      {/* PSY-1076: the box is a capped teaser — link to the full radar view. */}
      <div className="mt-3 border-t border-border pt-2.5">
        <BracketLink
          label="full radar →"
          href="/radio/new-releases"
          className="font-mono text-xs"
        />
      </div>
    </SidebarBox>
  )
}

// ---------------------------------------------------------------------------
// Dial stats
// ---------------------------------------------------------------------------

/**
 * Lifetime dial stats. PSY-1048 did NOT ship weekly windowed stats, so the
 * mock's "THIS WEEK ON THE DIAL" renders honestly as all-time numbers.
 */
export function DialStatsBox({ stats }: { stats: RadioStats | undefined }) {
  if (!stats) return null

  return (
    <SidebarBox title="On the dial — all time">
      <StatsList
        items={[
          { label: 'playlists tracked', value: stats.total_episodes },
          { label: 'plays logged', value: stats.total_plays },
          { label: 'plays matched', value: stats.matched_plays },
          { label: 'unique artists', value: stats.unique_artists },
        ]}
      />
    </SidebarBox>
  )
}
