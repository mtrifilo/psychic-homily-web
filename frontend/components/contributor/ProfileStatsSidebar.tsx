'use client'

import { useState } from 'react'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { statNumberFormatter } from '@/components/shared/StatsList'
import { ActivityHeatmap } from './ActivityHeatmap'
import { PercentileRankings } from './PercentileRankings'
import type { ContributionStats } from '@/features/auth'

interface ProfileStatsSidebarProps {
  username: string
  stats?: ContributionStats
  /** count_only fallback when full stats are privacy-gated. */
  statsCount?: number
  /** Total public collections (from the collections query), shown as a headline stat. */
  collectionsTotal?: number
  /**
   * Viewer owns this profile. Gates the zero-state onboarding hint, whose copy
   * addresses the profile owner ("Log a show…"), not a visitor.
   */
  isOwner?: boolean
  /**
   * Whether the "Show all stats" expander is available. The claim state
   * (/users/me) has no username yet, so the heatmap / rankings endpoints the
   * expanded panel fetches from can't resolve — it passes false.
   */
  expandable?: boolean
}

interface HeadlineStat {
  label: string
  value: string
}

/**
 * The breakdown rows for the expanded "All contributions" panel (design board
 * G). Shows attended and approval rate are headline stats, so they're not
 * repeated here. NOTE: deliberately NOT the icon-card <ContributionStatsGrid>
 * — that component still serves the /profile preview card (PSY-1061 owns its
 * fate); this dense list is the sidebar's own rendering.
 */
const ALL_CONTRIBUTION_ROWS: {
  label: string
  key: Exclude<keyof ContributionStats, 'approval_rate'>
}[] = [
  { label: 'Shows submitted', key: 'shows_submitted' },
  { label: 'Venues submitted', key: 'venues_submitted' },
  { label: 'Venue edits', key: 'venue_edits_submitted' },
  { label: 'Releases created', key: 'releases_created' },
  { label: 'Labels created', key: 'labels_created' },
  { label: 'Festivals created', key: 'festivals_created' },
  { label: 'Artists edited', key: 'artists_edited' },
  { label: 'Revisions', key: 'revisions_made' },
  { label: 'Pending edits', key: 'pending_edits_submitted' },
  { label: 'Tag votes', key: 'tag_votes_cast' },
  { label: 'Relationship votes', key: 'relationship_votes_cast' },
  { label: 'Request votes', key: 'request_votes_cast' },
  { label: 'Collection items', key: 'collection_items_added' },
  { label: 'Subscriptions', key: 'collection_subscriptions' },
  { label: 'Reports filed', key: 'reports_filed' },
  { label: 'Reports resolved', key: 'reports_resolved' },
  { label: 'Followers', key: 'followers_count' },
  { label: 'Following', key: 'following_count' },
  { label: 'Moderation actions', key: 'moderation_actions' },
]

function AllContributionsList({ stats }: { stats: ContributionStats }) {
  const rows = ALL_CONTRIBUTION_ROWS.map(({ label, key }) => ({
    label,
    value: stats[key],
  })).filter(row => row.value !== 0)

  return (
    <div>
      <SectionHeader title="All contributions" />
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">No contributions yet.</p>
      ) : (
        <dl>
          {rows.map(row => (
            <div
              key={row.label}
              className="flex items-baseline justify-between gap-2 border-b border-border/40 py-1.5"
            >
              <dt className="text-sm">{row.label}</dt>
              <dd className="font-mono text-sm tabular-nums">
                {statNumberFormatter.format(row.value)}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  )
}

/**
 * The right-rail statistics card (PSY-1045, restyled to design boards A/B/G in
 * PSY-1058). The What.cd-style dashboard is deliberately DEMOTED here: a few
 * headline numbers always visible, with the full counters + activity heatmap +
 * percentile rankings behind a collapsed "Show all stats" expander. This
 * inverts the old layout where the ~20-counter grid led the page.
 */
export function ProfileStatsSidebar({
  username,
  stats,
  statsCount,
  collectionsTotal,
  isOwner = false,
  expandable = true,
}: ProfileStatsSidebarProps) {
  const [expanded, setExpanded] = useState(false)

  const hasFullStats = stats !== undefined
  const hasCountOnly = !hasFullStats && statsCount !== undefined && statsCount > 0

  // Nothing visible at all (stats hidden, no count) — no card.
  if (!hasFullStats && !hasCountOnly) return null

  const headline: HeadlineStat[] = hasFullStats
    ? [
        {
          label: 'shows attended',
          value: statNumberFormatter.format(stats.shows_attended),
        },
        ...(collectionsTotal !== undefined
          ? [
              {
                label: 'collections',
                value: statNumberFormatter.format(collectionsTotal),
              },
            ]
          : []),
        {
          label: 'contributions',
          value: statNumberFormatter.format(stats.total_contributions),
        },
        ...(stats.approval_rate !== undefined && stats.approval_rate !== null
          ? [
              {
                label: 'approval rate',
                // approval_rate is a 0–1 fraction (backend contract).
                value: `${Math.round(stats.approval_rate * 100)}%`,
              },
            ]
          : []),
      ]
    : [
        {
          label: 'contributions',
          value: statNumberFormatter.format(statsCount!),
        },
      ]

  // Brand-new profile (design board B): zeroed numerals, no expander — the
  // expanded panel would be all empty states. Owners get the onboarding hint.
  const isAllZero =
    hasFullStats &&
    stats.shows_attended === 0 &&
    (collectionsTotal ?? 0) === 0 &&
    stats.total_contributions === 0
  const canExpand = expandable && hasFullStats && !isAllZero

  return (
    <section
      aria-label="Statistics"
      className="rounded-md border border-border bg-card p-5"
    >
      <SectionHeader
        title="Statistics"
        action={
          canExpand && expanded ? (
            <button
              type="button"
              onClick={() => setExpanded(false)}
              aria-expanded={true}
              aria-label="Hide the full statistics dashboard"
              className="font-mono text-xs text-primary hover:underline"
            >
              ▴ Hide
            </button>
          ) : undefined
        }
      />

      <div className="mt-3 space-y-2.5">
        {headline.map(item => (
          <div key={item.label} className="flex items-baseline gap-3">
            <span className="w-14 shrink-0 text-right font-mono text-2xl font-bold leading-none tabular-nums">
              {item.value}
            </span>
            <span className="min-w-0 text-sm text-muted-foreground">
              {item.label}
            </span>
          </div>
        ))}
      </div>

      {canExpand && !expanded && (
        <div className="mt-4 border-t border-border/50 pt-3">
          <button
            type="button"
            onClick={() => setExpanded(true)}
            aria-expanded={false}
            aria-label="Show all statistics, activity heatmap and rankings"
            className="text-sm font-medium text-primary hover:underline"
          >
            ▸ Show all stats
          </button>
          <p className="mt-1 font-mono text-xs leading-relaxed text-muted-foreground">
            full counters · 365-day activity heatmap · percentile rankings
          </p>
        </div>
      )}

      {isAllZero && isOwner && (
        <p className="mt-4 border-t border-border/50 pt-3 text-sm leading-relaxed text-muted-foreground">
          Log a show or follow an artist and your profile starts filling in.
        </p>
      )}

      {canExpand && expanded && (
        <div className="mt-4 space-y-6">
          <AllContributionsList stats={stats} />
          <div>
            <SectionHeader title="Activity" />
            <ActivityHeatmap username={username} />
          </div>
          <PercentileRankings username={username} />
        </div>
      )}
    </section>
  )
}
