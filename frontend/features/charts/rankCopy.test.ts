import { describe, expect, it } from 'vitest'
import {
  CHART_RANK_MODULE_COPY,
  chartRankHref,
  chartRankLineCopy,
  chartRankWindowLabel,
} from './rankCopy'

describe('rankCopy', () => {
  it('maps approved Figma module phrases', () => {
    expect(CHART_RANK_MODULE_COPY['most-anticipated']).toBe(
      'most-saved upcoming show'
    )
    expect(CHART_RANK_MODULE_COPY['most-active-artists']).toBe(
      'hardest-working artists'
    )
    expect(CHART_RANK_MODULE_COPY['busiest-venues']).toBe('busiest venues')
    expect(CHART_RANK_MODULE_COPY['new-releases']).toBe('new releases')
  })

  it('labels rolling windows', () => {
    expect(chartRankWindowLabel('quarter')).toBe('this quarter')
    expect(chartRankWindowLabel('month')).toBe('this month')
    expect(chartRankWindowLabel('all_time')).toBe('all time')
  })

  it('builds drill-down hrefs with window preserved', () => {
    expect(chartRankHref('most-anticipated', 'quarter')).toBe(
      '/charts/most-anticipated?window=quarter'
    )
    expect(chartRankHref('most-active-artists', 'month')).toBe(
      '/charts/most-active-artists?window=month'
    )
  })

  it('formats the badge line with em-dash and arrow', () => {
    expect(chartRankLineCopy('most-anticipated', 'quarter')).toBe(
      'most-saved upcoming show — this quarter →'
    )
  })
})
