import { describe, it, expect } from 'vitest'
import {
  DIMENSION_LABELS,
  PERIOD_LABELS,
  type LeaderboardDimension,
  type LeaderboardPeriod,
} from './types'

const DIMENSIONS: LeaderboardDimension[] = [
  'overall',
  'shows',
  'venues',
  'tags',
  'edits',
  'requests',
]

const PERIODS: LeaderboardPeriod[] = ['all_time', 'month', 'week']

describe('DIMENSION_LABELS', () => {
  it('maps every leaderboard dimension to a non-empty label', () => {
    for (const dimension of DIMENSIONS) {
      expect(DIMENSION_LABELS[dimension]).toBeTruthy()
    }
  })

  it('uses curated copy for representative dimensions', () => {
    expect(DIMENSION_LABELS.overall).toBe('Overall')
    expect(DIMENSION_LABELS.shows).toBe('Shows')
  })

  it('has exactly one label per known dimension', () => {
    expect(Object.keys(DIMENSION_LABELS).sort()).toEqual([...DIMENSIONS].sort())
  })
})

describe('PERIOD_LABELS', () => {
  it('maps every leaderboard period to a non-empty label', () => {
    for (const period of PERIODS) {
      expect(PERIOD_LABELS[period]).toBeTruthy()
    }
  })

  it('expands snake_case keys into human copy', () => {
    expect(PERIOD_LABELS.all_time).toBe('All Time')
    expect(PERIOD_LABELS.month).toBe('This Month')
    expect(PERIOD_LABELS.week).toBe('This Week')
  })
})
