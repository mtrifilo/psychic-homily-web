import { describe, it, expect } from 'vitest'
import { toEpisodeDate } from './episodeDate'

describe('toEpisodeDate', () => {
  it('takes the written date from a UTC-midnight timestamp', () => {
    expect(toEpisodeDate('2025-01-15T00:00:00Z')).toBe('2025-01-15')
  })

  it('passes a bare YYYY-MM-DD through', () => {
    expect(toEpisodeDate('2026-10-28')).toBe('2026-10-28')
  })

  it('keeps the written date of a timestamp with an offset', () => {
    expect(toEpisodeDate('2026-10-28T23:30:00-05:00')).toBe('2026-10-28')
  })

  it.each([undefined, null, '', 'not a date', '2026-1-5', '2026-10-28 00:00:00', '12026-10-28'])(
    'answers null for %j',
    value => {
      expect(toEpisodeDate(value)).toBeNull()
    }
  )
})
