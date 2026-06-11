'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { BracketLink } from '@/components/shared/BracketLink'
import { useArtistFestivalTrajectory } from '../hooks/useFestivals'
import { getBillingTierLabel } from '../types'
import type { ArtistTrajectory } from '../types'

interface ArtistTrajectoryChartProps {
  artistIdOrSlug: string | number
  enabled?: boolean
  /**
   * Start collapsed (PSY-644 dense main column). Renders the header with a
   * `[Show]`/`[Hide]` toggle; the body renders only when expanded. The data
   * fetch stays eager (matches the pre-PSY-644 behavior and the established
   * sidebar/main-column pattern); when there's no festival history the
   * component returns null so the section disappears entirely.
   */
  defaultCollapsed?: boolean
}

function AppearanceRows({ data }: { data: ArtistTrajectory }) {
  return (
    <>
      {/* Dense rows per the Figma Artists board (PSY-1070): year · full
          linked festival name · billed-as. The previous tier-width bars
          encoded billing tier as decoration while truncating the festival
          name — the actual content — to 140px. */}
      <div className="flex items-baseline gap-4 pb-1.5 border-b border-border/60 text-[11px] uppercase tracking-wider text-muted-foreground">
        <span className="w-12 shrink-0">Year</span>
        <span className="min-w-0 flex-1">Festival</span>
        <span className="w-28 shrink-0">Billed as</span>
      </div>
      <div className="divide-y divide-border/60">
        {data.appearances.map(entry => (
          <div
            key={`${entry.festival_slug}-${entry.year}`}
            className="flex items-baseline gap-4 py-2 text-sm"
          >
            <span className="w-12 shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
              {entry.year}
            </span>
            <span className="min-w-0 flex-1 font-medium">
              <Link
                href={`/festivals/${entry.festival_slug}`}
                className="hover:text-primary hover:underline"
              >
                {entry.festival_name}
              </Link>
            </span>
            <span className="w-28 shrink-0 text-muted-foreground">
              {getBillingTierLabel(entry.tier)}
            </span>
          </div>
        ))}
      </div>
      {data.total_appearances > 1 && (
        <div className="mt-2 font-mono text-[11px] text-muted-foreground">
          {/* Factual count only — Rising/Steady trend judgments were removed
              (PSY-1056): our lineup history is incomplete, so any trend read
              off it is a guess. */}
          <span>{data.total_appearances} festival appearances</span>
        </div>
      )}
    </>
  )
}

export function ArtistTrajectoryChart({
  artistIdOrSlug,
  enabled = true,
  defaultCollapsed = false,
}: ArtistTrajectoryChartProps) {
  const [open, setOpen] = useState(!defaultCollapsed)
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

  if (defaultCollapsed) {
    return (
      <div>
        <div className="flex items-baseline justify-between gap-2 mb-2">
          <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            Festival appearances
          </h2>
          <BracketLink
            label={open ? 'Hide' : 'Show'}
            onClick={() => setOpen(!open)}
          />
        </div>
        {open && <AppearanceRows data={data} />}
      </div>
    )
  }

  return (
    <div>
      <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-3">
        Festival appearances
      </h2>
      <AppearanceRows data={data} />
    </div>
  )
}
