'use client'

import { Skeleton } from '@/components/ui/skeleton'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { usePercentileRankings } from '@/features/auth'
import type { PercentileRanking } from '@/features/auth'

/**
 * Percentile-ranking bars for the profile stats expander (design board G):
 * editorial single-hue treatment — primary-colored fill on a muted track,
 * "top N%" in mono — replacing the old traffic-light Card (PSY-1058). Bar
 * width is the user's percentile standing, so "top 4%" renders nearly full.
 */
function RankingBar({ ranking }: { ranking: PercentileRanking }) {
  return (
    <div className="space-y-1">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-sm">{ranking.label}</span>
        <span className="shrink-0 font-mono text-xs tabular-nums">
          top {100 - ranking.percentile}%
        </span>
      </div>
      <div className="h-1.5 w-full bg-muted">
        <div
          className="h-full bg-primary transition-all"
          style={{ width: `${Math.max(ranking.percentile, 2)}%` }}
        />
      </div>
    </div>
  )
}

interface PercentileRankingsProps {
  username: string
}

export function PercentileRankings({ username }: PercentileRankingsProps) {
  const { data: rankings, isLoading, error } = usePercentileRankings(username)

  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-4 w-40" />
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="space-y-1.5">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-1.5 w-full" />
          </div>
        ))}
      </div>
    )
  }

  // Don't render anything on error or no data (e.g., not enough users, hidden)
  if (error || !rankings) {
    return null
  }

  const hasDetailedRankings = rankings.rankings.length > 0

  return (
    <div>
      <SectionHeader
        title="Percentile rankings"
        action={
          <span className="font-mono text-xs text-primary">
            overall · top {100 - rankings.overall_score}%
          </span>
        }
      />
      {hasDetailedRankings && (
        <div className="mt-1 space-y-2.5">
          {rankings.rankings.map(ranking => (
            <RankingBar key={ranking.dimension} ranking={ranking} />
          ))}
        </div>
      )}
    </div>
  )
}
