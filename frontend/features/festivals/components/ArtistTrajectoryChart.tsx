'use client'

import Link from 'next/link'
import { Loader2, TrendingUp, Minus } from 'lucide-react'
import { useArtistFestivalTrajectory } from '../hooks/useFestivals'
import { getBillingTierLabel, getTierBarWidth } from '../types'

interface ArtistTrajectoryChartProps {
  artistIdOrSlug: string | number
  enabled?: boolean
}

export function ArtistTrajectoryChart({ artistIdOrSlug, enabled = true }: ArtistTrajectoryChartProps) {
  const { data, isLoading } = useArtistFestivalTrajectory({
    artistIdOrSlug,
    enabled,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center py-4">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!data || data.total_appearances === 0) {
    return null
  }

  const isRising = data.breakout_score > 0

  return (
    <div>
      <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-3">
        Festival History
      </h3>
      <div className="space-y-2">
        {data.appearances.map((entry) => (
          <div key={`${entry.festival_slug}-${entry.year}`} className="flex items-center gap-3">
            <span className="text-xs text-muted-foreground w-10 text-right tabular-nums">
              {entry.year}
            </span>
            <div className="flex-1">
              <div
                className="h-5 rounded bg-primary/20 flex items-center px-2"
                style={{ width: `${getTierBarWidth(entry.tier)}%` }}
              >
                <span className="text-[10px] text-foreground/80 truncate">
                  {getBillingTierLabel(entry.tier)}
                </span>
              </div>
            </div>
            <Link
              href={`/festivals/${entry.festival_slug}`}
              className="text-xs text-muted-foreground hover:text-primary transition-colors truncate max-w-[140px]"
            >
              {entry.festival_name}
            </Link>
          </div>
        ))}
      </div>
      {data.total_appearances > 1 && (
        <div className="mt-3 flex items-center gap-1.5 text-xs text-muted-foreground">
          {isRising ? (
            <>
              <TrendingUp className="h-3.5 w-3.5 text-green-500" />
              <span>
                Rising — {data.breakout_score.toFixed(1)} tier improvements/year
              </span>
            </>
          ) : (
            <>
              <Minus className="h-3.5 w-3.5" />
              <span>Steady — {data.total_appearances} festival appearances</span>
            </>
          )}
        </div>
      )}
    </div>
  )
}
