'use client'

import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { useSeriesComparison } from '../hooks/useFestivals'
import { getBillingTierLabel } from '../types'
import { Badge } from '@/components/ui/badge'

interface SeriesHistoryProps {
  seriesSlug: string
  editions: { year: number }[]
  enabled?: boolean
}

export function SeriesHistory({ seriesSlug, editions, enabled = true }: SeriesHistoryProps) {
  const years = editions.map((e) => e.year).sort((a, b) => a - b)

  const { data, isLoading } = useSeriesComparison({
    seriesSlug,
    years,
    enabled: enabled && years.length >= 2,
  })

  if (years.length < 2) {
    return null
  }

  if (isLoading) {
    return (
      <div className="flex justify-center py-4">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!data) {
    return null
  }

  const latestYear = years[years.length - 1]

  return (
    <div className="space-y-6">
      {/* Stats Summary */}
      <div className="flex flex-wrap gap-4">
        {data.editions.map((edition) => (
          <Link
            key={edition.festival_id}
            href={`/festivals/${edition.slug}`}
            className="flex flex-col items-center p-3 rounded-lg border border-border/50 bg-card hover:bg-muted/50 transition-colors min-w-[80px]"
          >
            <span className="text-lg font-bold">{edition.year}</span>
            <span className="text-xs text-muted-foreground">
              {edition.artist_count} artists
            </span>
          </Link>
        ))}
      </div>

      {/* Metrics */}
      <div className="flex flex-wrap gap-6 text-sm">
        <div>
          <span className="text-muted-foreground">Retention:</span>{' '}
          <span className="font-medium">{(data.retention_rate * 100).toFixed(0)}%</span>
        </div>
        <div>
          <span className="text-muted-foreground">Lineup Growth:</span>{' '}
          <span className="font-medium">
            {data.lineup_growth > 0 ? '+' : ''}
            {(data.lineup_growth * 100).toFixed(0)}%
          </span>
        </div>
      </div>

      {/* Returning Artists */}
      {data.returning_artists.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-3">
            Returning Artists ({data.returning_artists.length})
          </h3>
          <div className="space-y-2">
            {data.returning_artists.slice(0, 20).map((ra) => (
              <div
                key={ra.artist.id}
                className="flex items-center gap-3 text-sm"
              >
                <Link
                  href={`/artists/${ra.artist.slug}`}
                  className="font-medium text-foreground hover:text-primary transition-colors min-w-[120px]"
                >
                  {ra.artist.name}
                </Link>
                <div className="flex items-center gap-1 text-xs text-muted-foreground flex-wrap">
                  {ra.years.map((year, i) => {
                    const tier = ra.tiers[String(year)]
                    return (
                      <span key={year} className="flex items-center gap-0.5">
                        {i > 0 && <span className="text-muted-foreground/40 mx-0.5">&rarr;</span>}
                        <span>{getBillingTierLabel(tier).toLowerCase()}</span>
                      </span>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Newcomers */}
      {data.newcomers.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-3">
            {latestYear} Newcomers ({data.newcomers.length})
          </h3>
          <div className="flex flex-wrap gap-1.5">
            {data.newcomers.map((n) => (
              <Link
                key={n.artist.id}
                href={`/artists/${n.artist.slug}`}
              >
                <Badge variant="secondary" className="text-xs hover:bg-muted">
                  {n.artist.name}
                  <span className="text-muted-foreground/60 ml-1">
                    {getBillingTierLabel(n.tier).toLowerCase()}
                  </span>
                </Badge>
              </Link>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
