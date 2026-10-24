import { describe, expect, it } from 'vitest'
import { PASSED_SHOW_GRACE_MS, withoutPassedShows } from './withoutPassedShows'

const NOW = Date.parse('2026-10-23T12:00:00Z')
const at = (offsetMs: number) => new Date(NOW + offsetMs).toISOString()

describe('withoutPassedShows', () => {
  it('returns the payload itself when every row can still be upcoming', () => {
    const payload = {
      shows: [{ event_date: at(-PASSED_SHOW_GRACE_MS + 1) }, { event_date: at(60_000) }],
      total: 2,
      artist_id: 42,
    }

    expect(withoutPassedShows(payload, NOW)).toBe(payload)
  })

  it('drops rows older than the grace window and reduces total by the same count', () => {
    const kept = { event_date: at(3_600_000), id: 2 }
    const payload = {
      shows: [{ event_date: at(-PASSED_SHOW_GRACE_MS - 1), id: 1 }, kept],
      total: 9,
      venue_id: 77,
    }

    expect(withoutPassedShows(payload, NOW)).toEqual({ shows: [kept], total: 8, venue_id: 77 })
  })

  it('keeps a row whose date does not parse', () => {
    const payload = { shows: [{ event_date: 'not a date' }], total: 1 }

    expect(withoutPassedShows(payload, NOW)).toBe(payload)
  })

  it('never reports a negative total', () => {
    const payload = { shows: [{ event_date: at(-PASSED_SHOW_GRACE_MS * 2) }], total: 0 }

    expect(withoutPassedShows(payload, NOW).total).toBe(0)
  })
})
