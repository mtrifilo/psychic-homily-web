import { describe, expect, it } from 'vitest'
import {
  CHART_MODULE_CONFIG,
  CHART_MODULE_SLUGS,
  isChartModuleSlug,
} from './moduleConfig'

describe('chart drill-down module config', () => {
  it('defines payload-supported columns for all six modules', () => {
    expect(CHART_MODULE_SLUGS).toHaveLength(6)
    expect(
      Object.fromEntries(
        CHART_MODULE_SLUGS.map(slug => [
          slug,
          CHART_MODULE_CONFIG[slug].columns.map(column => column.label),
        ])
      )
    ).toEqual({
      'most-active-artists': ['Artist', 'Shows', 'Headline %', 'Last show'],
      'on-the-radio': ['Artist', 'Plays', 'Stations', 'Rotation'],
      'most-anticipated': ['Show', 'Date', 'Venue', 'Saves'],
      'busiest-venues': ['Venue', 'Location', 'Shows'],
      'new-releases': ['Release', 'Artists', 'Labels', 'Released', 'Added'],
      'openers-to-watch': ['Artist', 'Location', 'Support slots'],
    })
  })

  it('rejects unknown module slugs', () => {
    expect(isChartModuleSlug('most-active-artists')).toBe(true)
    expect(isChartModuleSlug('unknown')).toBe(false)
  })
})
