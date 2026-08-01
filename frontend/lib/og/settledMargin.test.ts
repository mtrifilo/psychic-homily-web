import { describe, it, expect } from 'vitest'
import { OG_SETTLED_MARGIN_MS, ogCacheControl } from './response'
import { isShowPast } from '@/lib/utils/showTiming'

/**
 * The show card's cache classification, pinned at the seam.
 *
 * The route itself cannot assert this: under vitest the brand fonts do not
 * resolve, so every card there renders `degraded` and takes the short window
 * before the settled/live branch is ever reached (see `opengraph-image.test.tsx`).
 * These pin the composition the route performs — the shared venue-local
 * derivation, read a margin into the past — so the branch stops being untested.
 */

/** What `renderCard` computes, with the clock made injectable. */
const settled = (
  show: { eventDate: string; state?: string | null; timezone?: string | null },
  now: Date
) =>
  isShowPast(
    { eventDate: show.eventDate, state: show.state, timezone: show.timezone },
    new Date(now.getTime() - OG_SETTLED_MARGIN_MS)
  )

const PHOENIX_8PM = {
  eventDate: '2026-03-15T03:00:00Z', // 20:00 Mar 14 Phoenix
  state: 'AZ',
  timezone: 'America/Phoenix',
}

describe('when a show card is allowed to cache hard', () => {
  it('is still live during the show', () => {
    expect(settled(PHOENIX_8PM, new Date('2026-03-15T05:00:00Z'))).toBe(false)
  })

  it('is still live at venue-local midnight, because the margin has not elapsed', () => {
    // The boundary WITHOUT the margin. A card frozen here would miss every
    // correction an admin makes the morning after the show.
    expect(settled(PHOENIX_8PM, new Date('2026-03-15T07:01:00Z'))).toBe(false)
  })

  it('is still live most of the following day', () => {
    expect(settled(PHOENIX_8PM, new Date('2026-03-16T06:59:00Z'))).toBe(false)
  })

  it('settles a full day after the venue-local day ended', () => {
    expect(settled(PHOENIX_8PM, new Date('2026-03-16T07:01:00Z'))).toBe(true)
  })

  it('never settles a show that has not happened', () => {
    expect(settled(PHOENIX_8PM, new Date('2026-03-01T00:00:00Z'))).toBe(false)
  })

  it('holds an after-midnight show through its own venue-local day plus the margin', () => {
    // 00:30 Mar 15 Phoenix: the case where a UTC-day rule settles a day early.
    const afterMidnight = { ...PHOENIX_8PM, eventDate: '2026-03-15T07:30:00Z' }
    expect(settled(afterMidnight, new Date('2026-03-16T07:01:00Z'))).toBe(false)
    expect(settled(afterMidnight, new Date('2026-03-17T07:01:00Z'))).toBe(true)
  })

  it('reads the VENUE clock, not the reader s', () => {
    // 20:00 Mar 14 Auckland (UTC+13). Phoenix is still Mar 14 when Auckland has
    // long rolled over, so a server-local rule would settle this a day late.
    const auckland = {
      eventDate: '2026-03-14T07:00:00Z',
      state: null,
      timezone: 'Pacific/Auckland',
    }
    expect(settled(auckland, new Date('2026-03-15T10:59:00Z'))).toBe(false)
    expect(settled(auckland, new Date('2026-03-15T11:01:00Z'))).toBe(true)
  })
})

describe('the margin itself', () => {
  it('is one day', () => {
    // Pinned because the route subtracts it from the clock rather than adding
    // it to a boundary, which is easy to mis-sign in a refactor.
    expect(OG_SETTLED_MARGIN_MS).toBe(24 * 60 * 60 * 1000)
  })

  it('matters because stale-while-revalidate doubles whatever window is chosen', () => {
    // The reason a settled card is expensive to get wrong: the CDN may serve it
    // for s-maxage AND then again through the revalidation window.
    expect(ogCacheControl(86400)).toBe(
      'public, max-age=0, s-maxage=86400, stale-while-revalidate=86400'
    )
  })
})
