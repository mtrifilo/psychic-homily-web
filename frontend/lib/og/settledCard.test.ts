import { describe, it, expect } from 'vitest'
import { isShowCardSettled, ogCacheControl } from './response'

/**
 * Tests the symbol the show-card route actually calls, not a copy of it.
 *
 * The route cannot assert this itself: the brand fonts are route assets that do
 * not resolve under vitest, so every card rendered there is `degraded` and takes
 * the short window before the settled/live branch is reached (see
 * `app/shows/[slug]/opengraph-image.test.tsx`). Reverting the route's rule
 * breaks these.
 */

const PHOENIX_8PM = {
  eventDate: '2026-03-15T03:00:00Z', // 20:00 Mar 14 Phoenix
  state: 'AZ',
  timezone: 'America/Phoenix',
}

describe('isShowCardSettled', () => {
  it('is live during the show', () => {
    expect(isShowCardSettled(PHOENIX_8PM, new Date('2026-03-15T05:00:00Z'))).toBe(false)
  })

  it('is still live at venue-local midnight, because the margin has not elapsed', () => {
    // The venue-local boundary WITHOUT the margin. A card frozen here would
    // miss every correction an admin makes the morning after the show, and
    // `ogCacheControl` would hold it for up to twice the window.
    expect(isShowCardSettled(PHOENIX_8PM, new Date('2026-03-15T07:01:00Z'))).toBe(false)
  })

  it('is still live most of the following day', () => {
    expect(isShowCardSettled(PHOENIX_8PM, new Date('2026-03-16T06:59:00Z'))).toBe(false)
  })

  it('settles a full day after the venue-local day ended', () => {
    expect(isShowCardSettled(PHOENIX_8PM, new Date('2026-03-16T07:01:00Z'))).toBe(true)
  })

  it('never settles a show that has not happened', () => {
    expect(isShowCardSettled(PHOENIX_8PM, new Date('2026-03-01T00:00:00Z'))).toBe(false)
  })

  it('is never earlier than the start-plus-a-day rule it replaced', () => {
    // The old rule settled at start + 24h. The venue-local day containing the
    // start always ends at or after the start, so the new instant is always
    // later. Pinned because a sign error here is a 48-hour stale share card.
    const startPlusADay = new Date(Date.parse(PHOENIX_8PM.eventDate) + 24 * 60 * 60 * 1000)
    expect(isShowCardSettled(PHOENIX_8PM, startPlusADay)).toBe(false)
  })

  it('holds an after-midnight show through its own venue-local day plus the margin', () => {
    // 00:30 Mar 15 Phoenix: the case a UTC-day rule settles a day early.
    const afterMidnight = { ...PHOENIX_8PM, eventDate: '2026-03-15T07:30:00Z' }
    expect(isShowCardSettled(afterMidnight, new Date('2026-03-16T07:01:00Z'))).toBe(false)
    expect(isShowCardSettled(afterMidnight, new Date('2026-03-17T07:01:00Z'))).toBe(true)
  })

  it('reads the VENUE clock, not the server s', () => {
    // 20:00 Mar 14 Auckland (UTC+13). Phoenix is still Mar 14 long after
    // Auckland has rolled over, so a server-local rule settles this late.
    const auckland = {
      eventDate: '2026-03-14T07:00:00Z',
      state: null,
      timezone: 'Pacific/Auckland',
    }
    expect(isShowCardSettled(auckland, new Date('2026-03-15T10:59:00Z'))).toBe(false)
    expect(isShowCardSettled(auckland, new Date('2026-03-15T11:01:00Z'))).toBe(true)
  })

  it('matters because stale-while-revalidate doubles whatever window is chosen', () => {
    // Why a settled card is expensive to get wrong: the CDN may serve it for
    // s-maxage and then again through the revalidation window.
    expect(ogCacheControl(86400)).toBe(
      'public, max-age=0, s-maxage=86400, stale-while-revalidate=86400'
    )
  })
})
