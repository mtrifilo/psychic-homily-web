'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Trophy } from 'lucide-react'
import { usePercentileRankings } from '@/features/auth'
import type { PercentileRanking } from '@/features/auth'

function getPercentileColor(percentile: number): string {
  if (percentile >= 75) return 'bg-green-500'
  if (percentile >= 50) return 'bg-yellow-500'
  if (percentile >= 25) return 'bg-orange-500'
  return 'bg-red-500'
}

function getPercentileTextColor(percentile: number): string {
  if (percentile >= 75) return 'text-green-500'
  if (percentile >= 50) return 'text-yellow-500'
  if (percentile >= 25) return 'text-orange-500'
  return 'text-red-500'
}

function formatDimensionValue(value: number): string {
  if (value >= 1000) return `${(value / 1000).toFixed(1)}k`
  return String(value)
}

function RankingBar({ ranking }: { ranking: PercentileRanking }) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">{ranking.label}</span>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {formatDimensionValue(ranking.value)}
          </span>
          <span className={`text-xs font-medium ${getPercentileTextColor(ranking.percentile)}`}>
            Top {100 - ranking.percentile}%
          </span>
        </div>
      </div>
      <div className="h-2 w-full rounded-full bg-muted">
        <div
          className={`h-full rounded-full transition-all ${getPercentileColor(ranking.percentile)}`}
          style={{ width: `${Math.max(ranking.percentile, 2)}%` }}
        />
      </div>
    </div>
  )
}

function OverallBadge({ score }: { score: number }) {
  return (
    <div className="flex items-center gap-2 rounded-lg border border-border/50 bg-muted/30 px-3 py-2">
      <Trophy className={`h-4 w-4 ${getPercentileTextColor(score)}`} />
      <span className="text-sm font-medium">
        Top {100 - score}% overall
      </span>
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
      <Card className="bg-muted/30 border-border/50">
        <CardHeader className="pb-3">
          <Skeleton className="h-5 w-40" />
        </CardHeader>
        <CardContent className="space-y-4">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="space-y-2">
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-2 w-full" />
            </div>
          ))}
        </CardContent>
      </Card>
    )
  }

  // Don't render anything on error or no data (e.g., not enough users, hidden)
  if (error || !rankings) {
    return null
  }

  const hasDetailedRankings = rankings.rankings.length > 0

  return (
    <Card className="bg-muted/30 border-border/50">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base font-semibold">Rankings</CardTitle>
          <OverallBadge score={rankings.overall_score} />
        </div>
      </CardHeader>
      {hasDetailedRankings && (
        <CardContent className="space-y-3">
          {rankings.rankings.map((ranking) => (
            <RankingBar key={ranking.dimension} ranking={ranking} />
          ))}
        </CardContent>
      )}
    </Card>
  )
}
