import type { ChartRankModule, ChartWindow } from './types'

/**
 * Figma-approved module phrases for the entity-page rank badge (node 996:16).
 * Keep these lowercase — they sit after the mono "No. N" prefix.
 */
export const CHART_RANK_MODULE_COPY: Record<ChartRankModule, string> = {
  'most-anticipated': 'most-saved upcoming show',
  'most-active-artists': 'hardest-working artists',
  'busiest-venues': 'busiest venues',
  'new-releases': 'new releases',
}

/** Human window label for badge copy (v1 rolling windows). */
export function chartRankWindowLabel(window: string): string {
  switch (window) {
    case 'month':
      return 'this month'
    case 'all_time':
      return 'all time'
    case 'quarter':
    default:
      return 'this quarter'
  }
}

export function chartRankHref(
  module: ChartRankModule,
  window: ChartWindow | string
): string {
  const params = new URLSearchParams({ window: String(window) })
  return `/charts/${module}?${params.toString()}`
}

export function chartRankLineCopy(
  module: ChartRankModule,
  window: string
): string {
  return `${CHART_RANK_MODULE_COPY[module]} — ${chartRankWindowLabel(window)} →`
}
