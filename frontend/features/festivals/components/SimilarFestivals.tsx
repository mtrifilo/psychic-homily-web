'use client'

import Link from 'next/link'
import { Loader2, Users } from 'lucide-react'
import { useSimilarFestivals } from '../hooks/useFestivals'
import { getBillingTierLabel } from '../types'
import { Badge } from '@/components/ui/badge'

interface SimilarFestivalsProps {
  festivalIdOrSlug: string | number
  enabled?: boolean
}

export function SimilarFestivals({ festivalIdOrSlug, enabled = true }: SimilarFestivalsProps) {
  const { data, isLoading } = useSimilarFestivals({
    festivalIdOrSlug,
    limit: 5,
    enabled,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center py-4">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!data?.similar || data.similar.length === 0) {
    return null
  }

  return (
    <div>
      <h2 className="text-lg font-semibold mb-4">Similar Festivals</h2>
      <div className="space-y-3">
        {data.similar.map((sf) => (
          <div
            key={sf.festival.id}
            className="rounded-lg border border-border/50 bg-card p-4"
          >
            <div className="flex items-center justify-between mb-2">
              <Link
                href={`/festivals/${sf.festival.slug}`}
                className="font-medium text-foreground hover:text-primary transition-colors"
              >
                {sf.festival.name}
              </Link>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <Users className="h-3.5 w-3.5" />
                <span>{sf.shared_artist_count} shared</span>
                <span className="text-muted-foreground/60">
                  {(sf.jaccard * 100).toFixed(1)}% overlap
                </span>
              </div>
            </div>
            {sf.top_shared.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {sf.top_shared.map((artist) => (
                  <Link
                    key={artist.artist_id}
                    href={`/artists/${artist.slug}`}
                    className="inline-flex items-center gap-1"
                  >
                    <Badge variant="secondary" className="text-[10px] px-1.5 py-0 hover:bg-muted">
                      {artist.name}
                      <span className="text-muted-foreground/60 ml-1">
                        {getBillingTierLabel(artist.tier_at_target)}
                      </span>
                    </Badge>
                  </Link>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
