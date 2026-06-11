'use client'

import { useState } from 'react'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { StatsList } from '@/components/shared/StatsList'
import { BracketLink } from '@/components/shared/BracketLink'
import { ActivityHeatmap } from './ActivityHeatmap'
import { ContributionStatsGrid } from './ContributionStatsGrid'
import { PercentileRankings } from './PercentileRankings'
import type { ContributionStats } from '@/features/auth'

interface ProfileStatsSidebarProps {
  username: string
  stats?: ContributionStats
  /** count_only fallback when full stats are privacy-gated. */
  statsCount?: number
  /** Total public collections (from the collections query), shown as a headline stat. */
  collectionsTotal?: number
}

/**
 * The right-rail statistics card (PSY-1045). The What.cd-style dashboard is
 * deliberately DEMOTED here: a few headline numbers always visible, with the
 * full counters + activity heatmap + percentile rankings behind a collapsed
 * "Show all stats" expander. This inverts the old layout where the ~20-counter
 * grid led the page.
 */
export function ProfileStatsSidebar({
  username,
  stats,
  statsCount,
  collectionsTotal,
}: ProfileStatsSidebarProps) {
  const [expanded, setExpanded] = useState(false)

  const hasFullStats = stats !== undefined
  const hasCountOnly = !hasFullStats && statsCount !== undefined && statsCount > 0

  // Nothing visible at all (stats hidden, no count) — no card.
  if (!hasFullStats && !hasCountOnly) return null

  const headline = hasFullStats
    ? [
        { label: 'Shows attended', value: stats.shows_attended },
        ...(collectionsTotal !== undefined
          ? [{ label: 'Collections', value: collectionsTotal }]
          : []),
        { label: 'Contributions', value: stats.total_contributions },
        ...(stats.approval_rate !== undefined && stats.approval_rate !== null
          ? [
              {
                label: 'Approval rate',
                value: `${Math.round(stats.approval_rate)}%`,
              },
            ]
          : []),
      ]
    : [{ label: 'Contributions', value: statsCount! }]

  return (
    <section aria-label="Statistics">
      <SectionHeader
        title="Statistics"
        action={
          hasFullStats ? (
            <BracketLink
              label={expanded ? 'Hide stats' : 'All stats'}
              onClick={() => setExpanded(e => !e)}
              active={expanded}
              ariaLabel={
                expanded
                  ? 'Hide the full statistics dashboard'
                  : 'Show all statistics, activity heatmap and rankings'
              }
            />
          ) : undefined
        }
      />
      <StatsList items={headline} />

      {hasFullStats && expanded && (
        <div className="mt-4 space-y-6">
          <ContributionStatsGrid stats={stats} />
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
