import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSceneDay } = vi.hoisted(() => ({ fetchSceneDay: vi.fn() }))
vi.mock('./sceneDayApi', () => ({ fetchSceneDay }))

import type { SceneDayResponse } from './sceneDay'
import { buildSceneDayMetadata } from './sceneDayPage'

const day = (over: Partial<SceneDayResponse> = {}): SceneDayResponse =>
  ({
    slug: 'phoenix-az',
    scene_name: 'Phoenix, AZ',
    city: 'Phoenix',
    state: 'AZ',
    date: '2026-07-31',
    timezone: 'America/Phoenix',
    iso_week: '2026-W31',
    show_count: 4,
    prev_date: '2026-07-30',
    next_date: '2026-08-01',
    is_tonight: true,
    is_past_day: false,
    shows: [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }],
    tracked_venues: [],
    ...over,
  }) as SceneDayResponse

/**
 * The `<head>` decisions that live nowhere else: the query-language title, the
 * split canonical (rolling /tonight names the WEEK permalink, a dated permalink
 * names itself), the week key coming from the payload rather than a clock, the
 * empty-payload fallbacks, og:url held apart from the canonical, and the
 * route-conditional noindex. Nothing else in the suite touches any of them, so
 * without this file each one can be reworded silently.
 */
describe('buildSceneDayMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    // Unconditional: one test below pins the clock, and a leaked fake clock
    // would follow it into every test after it in this file.
    vi.useRealTimers()
  })

  it('titles the live night in the words someone would type', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.title).toBe('Phoenix Shows Tonight — Friday, July 31, 2026')
  })

  // A dated permalink is permanent. Calling an archived Tuesday "tonight" would
  // be false the day after it was written, and it is the indexed URL.
  it('drops the word from a date that is not the live night', async () => {
    fetchSceneDay.mockResolvedValue(day({ is_tonight: false }))

    const meta = await buildSceneDayMetadata('phoenix-az', '2026-07-31')

    expect(meta.title).toBe('Phoenix Shows — Friday, July 31, 2026')
  })

  it('names the count and the city in the description', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.description).toContain('4 shows')
    expect(meta.description).toContain('Phoenix')
  })

  // Data-honest even in a snippet read out of context: never "no shows in
  // Phoenix", always our own calendar — and still carrying the count.
  it('says 0 shows LISTED for a quiet night, and does not speak for the city', async () => {
    fetchSceneDay.mockResolvedValue(day({ shows: [], show_count: 0 }))

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.description).toContain('0 shows listed')
    expect(meta.description).toContain('Phoenix')
    expect(meta.description).not.toMatch(/no shows in/i)
  })

  // Why the week rather than the night is argued once, in buildSceneDayMetadata.
  it('points the rolling /tonight canonical at the WEEK permalink', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-W31'
    )
  })

  // og:url is a SHARE IDENTITY, not a second canonical. The week page declares
  // the week permalink as its own og:url with its own generated card, so if
  // /tonight declared it too the two would collide in every scraper cache that
  // keys on og:url. It stays on the night these tags actually describe.
  it('keeps og:url on the DAY permalink, not on the week canonical', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const rolling = await buildSceneDayMetadata('phoenix-az')
    const dated = await buildSceneDayMetadata('phoenix-az', '2026-07-31')

    for (const meta of [rolling, dated]) {
      expect(meta.openGraph?.url).toBe(
        'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
      )
    }
  })

  // An empty `iso_week` passes the payload guard (`asPayload` accepts any
  // string, and prev_date/next_date in that same required list are empty by
  // design at the window edges). Unguarded, the canonical would collapse to
  // `/scenes/phoenix-az/` for every scene at once, silently.
  it('falls back to the day permalink when iso_week is empty', async () => {
    fetchSceneDay.mockResolvedValue(day({ iso_week: '' }))

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
    )
    expect(JSON.stringify(meta.openGraph?.images)).toContain('/og-image.jpg')
  })

  // The fallback above is only worth anything if the URL it falls back TO is
  // itself well formed. `slug` and `date` come through the same guard that lets
  // an empty string past, so a payload missing them must produce NO canonical
  // rather than a `/scenes//` shape offered to crawlers as this page's identity.
  it.each([
    ['slug', { slug: '' }],
    ['date', { date: '' }],
  ])('declares no canonical at all when %s is empty', async (_field, over) => {
    fetchSceneDay.mockResolvedValue(day(over))

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.alternates?.canonical).toBeUndefined()
    expect(meta.openGraph?.url).toBeUndefined()
    expect(meta.robots).toEqual({ index: false, follow: false })
  })

  // The fixture is a live night (`is_tonight: true`), so this also pins the
  // DISCRIMINATOR: an implementation keying off `day.is_tonight` instead of the
  // absent `date` argument would collapse every same-day permalink into the
  // week, and only this shape catches it.
  it('self-canonicalizes a dated permalink even on the night it names', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az', '2026-07-31')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
    )
  })

  // THE BOUNDARY CASE, and the reason `iso_week` is read from the payload.
  //
  // The scenario: it is 01:30 on Monday 2026-08-03. The backend's 6am night
  // boundary still answers Sunday 2026-08-02, the LAST day of 2026-W31, while
  // that same Monday already opens 2026-W32. So the night's week and "the week
  // it is right now" genuinely disagree, and only the payload knows which is
  // which.
  //
  // The pinned clock is what gives this test teeth. The payload alone cannot
  // fail it, because `iso_week` and `date` agree (both W31), so an
  // implementation deriving the week from `date` would also pass. The clock is
  // the counterfactual: `buildSceneDayMetadata` reads NO clock today, and this
  // test exists so that the day one is introduced it answers W32 and goes red.
  //
  // Noon UTC rather than the literal 01:30 local. The hour is immaterial to
  // what is guarded; the WEEK a local clock would name is the counterfactual,
  // and from this instant every real offset (UTC-12 through UTC+14) reads
  // Monday the 3rd or Tuesday the 4th, so all of them answer W32. There is no
  // runner timezone in which this test passes for the wrong reason.
  it('uses the payload iso_week when tonight falls in the PREVIOUS ISO week', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-03T12:00:00Z')) // Monday, opening W32
    fetchSceneDay.mockResolvedValue(
      day({
        date: '2026-08-02', // Sunday, closing W31
        iso_week: '2026-W31',
        prev_date: '2026-08-01',
        next_date: '2026-08-03',
      })
    )

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-W31'
    )
    const images = meta.openGraph?.images
    const image = Array.isArray(images) ? images[0] : images
    expect(image).toMatchObject({
      url: 'https://psychichomily.com/scenes/phoenix-az/2026-W31/opengraph-image',
    })
  })

  // Thin content — real, worth serving, worth linking out of, not worth an
  // index entry. `follow` stays ON precisely because the page's job in that
  // state is to point at the week and the rooms.
  //
  // ONLY on the dated permalink, which is its own canonical. See the rolling
  // case below: a noindex sitting beside a canonical that names a different URL
  // is a contradiction, and the URL it would be aimed at is the week page the
  // sitemap pushes.
  it('noindexes a quiet night on the dated permalink, but keeps following', async () => {
    fetchSceneDay.mockResolvedValue(day({ shows: [], show_count: 0 }))

    const meta = await buildSceneDayMetadata('phoenix-az', '2026-07-31')

    expect(meta.robots).toEqual({ index: false, follow: true })
    // The noindex and the canonical must be talking about the SAME URL.
    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
    )
  })

  // The pairing this change must never emit: `index: false` next to a canonical
  // naming a different, sitemap-announced page. Search engines are documented to
  // consolidate the suppression onto the canonical target, which here is the
  // scene's week page. /tonight needs no noindex regardless: naming another URL
  // as its canonical is already what keeps it out of the index as itself.
  it('never pairs a noindex with a foreign canonical on the rolling route', async () => {
    fetchSceneDay.mockResolvedValue(day({ shows: [], show_count: 0 }))

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-W31'
    )
    expect(meta.robots).toBeUndefined()
  })

  it('leaves a night that has shows indexable', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.robots).toBeUndefined()
  })

  // The sibling [period] route contributes an opengraph-image that renders the
  // WEEK card and 404s a date key. The rolling /tonight URL is itself a
  // constant, so the image we advertise is the archived week card — the URL
  // that both varies (it carries the night's iso_week) and actually serves a
  // card. Setting images explicitly is also what stops Next injecting the
  // dated permalink's 404 convention image.
  it('points the rolling route at the archived week card', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')
    const images = meta.openGraph?.images
    const image = Array.isArray(images) ? images[0] : images

    expect(image).toEqual({
      url: 'https://psychichomily.com/scenes/phoenix-az/2026-W31/opengraph-image',
      width: 1200,
      height: 630,
      type: 'image/png',
      alt: meta.description,
    })
    expect(meta.twitter).not.toHaveProperty('images')
  })

  // Dated permalinks still suppress the [period] convention 404 with the
  // site-wide card. A generated card per night is a different ticket.
  it('keeps the site-wide card on a dated permalink', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az', '2026-07-31')

    expect(meta.openGraph?.images).toBeDefined()
    expect(JSON.stringify(meta.openGraph?.images)).toContain('/og-image.jpg')
  })

  it('does not index a scene that does not resolve', async () => {
    fetchSceneDay.mockResolvedValue(null)

    const meta = await buildSceneDayMetadata('nowhere-zz')

    expect(meta.robots).toEqual({ index: false, follow: false })
    expect(meta.alternates?.canonical).toBeUndefined()
  })
})
