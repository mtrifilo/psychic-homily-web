'use client'

import Link from 'next/link'
import { cn } from '@/lib/utils'
import { useChartEntityRank } from '../hooks/useCharts'
import { chartRankHref, chartRankLineCopy } from '../rankCopy'
import type { ChartRankEntityType, ChartWindow } from '../types'

export interface EntityChartRankBadgeProps {
  entityType: ChartRankEntityType
  entityId: number
  /** v1 default is quarter (global scope only). */
  window?: ChartWindow
  className?: string
}

/**
 * Sidebar CHARTS block (PSY-1420 / Figma 996:16). Renders only when the
 * entity has a non-null rank — zero empty-state chrome. Non-blocking:
 * fetch is client-side and failures/loading hide the block.
 */
export function EntityChartRankBadge({
  entityType,
  entityId,
  window = 'quarter',
  className,
}: EntityChartRankBadgeProps) {
  const { data, isSuccess } = useChartEntityRank(entityType, entityId, window)

  if (!isSuccess || data.rank == null) return null

  const href = chartRankHref(data.module, data.window)
  const copy = chartRankLineCopy(data.module, data.window)

  return (
    <section
      data-testid="entity-chart-rank-badge"
      aria-label="Charts"
      className={cn(className)}
    >
      <p className="mb-2 border-b border-border/50 pb-1 font-mono text-[10px] font-bold uppercase tracking-[0.06em] text-muted-foreground">
        Charts
      </p>
      <Link
        href={href}
        className="flex items-start gap-2 text-xs leading-normal text-foreground hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span className="shrink-0 font-mono tabular-nums text-primary">
          No. {data.rank}
        </span>
        <span>{copy}</span>
      </Link>
    </section>
  )
}
