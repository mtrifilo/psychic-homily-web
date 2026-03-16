'use client'

import Link from 'next/link'
import { Loader2, TrendingUp, Star } from 'lucide-react'
import { useFestivalBreakouts } from '../hooks/useFestivals'
import { getBillingTierLabel, getMilestoneLabel } from '../types'
import { Badge } from '@/components/ui/badge'

interface RisingArtistsProps {
  festivalIdOrSlug: string | number
  enabled?: boolean
}

export function RisingArtists({ festivalIdOrSlug, enabled = true }: RisingArtistsProps) {
  const { data, isLoading } = useFestivalBreakouts({
    festivalIdOrSlug,
    enabled,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center py-4">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!data) return null

  const hasBreakouts = data.breakouts.length > 0
  const hasMilestones = data.milestones.length > 0

  if (!hasBreakouts && !hasMilestones) {
    return null
  }

  return (
    <div>
      <h2 className="text-lg font-semibold mb-4">Rising Artists</h2>
      <div className="space-y-3">
        {data.breakouts.map((b) => (
          <div
            key={b.artist.id}
            className="rounded-lg border border-border/50 bg-card p-4"
          >
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <TrendingUp className="h-4 w-4 text-green-500" />
                <Link
                  href={`/artists/${b.artist.slug}`}
                  className="font-medium text-foreground hover:text-primary transition-colors"
                >
                  {b.artist.name}
                </Link>
              </div>
              <Badge variant="secondary" className="text-xs">
                {getBillingTierLabel(b.current_tier)}
              </Badge>
            </div>
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground flex-wrap">
              {b.trajectory.map((entry, i) => (
                <span key={`${entry.festival_slug}-${entry.year}`} className="flex items-center gap-1">
                  {i > 0 && <span className="text-muted-foreground/40">&rarr;</span>}
                  <span>
                    {entry.festival_name.length > 20
                      ? entry.festival_name.slice(0, 20) + '...'
                      : entry.festival_name}{' '}
                    &apos;{String(entry.year).slice(-2)}{' '}
                    <span className="text-foreground/70">{getBillingTierLabel(entry.tier).toLowerCase()}</span>
                  </span>
                </span>
              ))}
            </div>
          </div>
        ))}

        {hasMilestones && (
          <div className="rounded-lg border border-border/50 bg-card p-4">
            <h3 className="text-sm font-semibold text-muted-foreground mb-3">Milestones</h3>
            <div className="space-y-2">
              {data.milestones.map((m) => (
                <div key={`${m.artist.id}-${m.milestone}`} className="flex items-center gap-2 text-sm">
                  <Star className="h-3.5 w-3.5 text-amber-500" />
                  <Link
                    href={`/artists/${m.artist.slug}`}
                    className="text-foreground hover:text-primary transition-colors"
                  >
                    {m.artist.name}
                  </Link>
                  <Badge variant="outline" className="text-[10px] px-1.5 py-0">
                    {getMilestoneLabel(m.milestone)}
                  </Badge>
                  <span className="text-muted-foreground text-xs">
                    ({getBillingTierLabel(m.tier).toLowerCase()})
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
