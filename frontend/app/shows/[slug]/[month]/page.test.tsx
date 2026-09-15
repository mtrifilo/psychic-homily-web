import { describe, it, expect, vi, beforeEach } from 'vitest'

// Throws, like the real one, which is typed `never`. A non-throwing mock lets
// the page body carry on past `notFound()` and return its Suspense element with
// a null window, so a test could assert "rejected the segments" while the route
// in fact rendered.
const NOT_FOUND = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND)
  }),
}))

// These tests exercise the route's OWN job, segment validation and the head,
// so the body, which fetches, is stubbed out.
vi.mock('@/features/shows/calendarPage', async importOriginal => {
  const actual = await importOriginal<
    typeof import('@/features/shows/calendarPage')
  >()
  return {
    ...actual,
    ShowsCalendarRoute: (): null => null,
  }
})

import ShowsMonthPage, { generateMetadata } from './page'
import ShowsDayPage, { generateMetadata as dayMetadata } from './[day]/page'

const monthParams = (slug: string, month: string) =>
  Promise.resolve({ slug, month })
const dayParams = (slug: string, month: string, day: string) =>
  Promise.resolve({ slug, month, day })
const searchParams = Promise.resolve({})

beforeEach(() => {
  vi.clearAllMocks()
})

describe('/shows/{yyyy}/{mm}, segment validation', () => {
  it('renders a well-formed month', async () => {
    await expect(
      ShowsMonthPage({ params: monthParams('2026', '11'), searchParams })
    ).resolves.toBeTruthy()
  })

  it.each([
    ['2026', '9'],
    ['2026', '13'],
    ['2026', '00'],
    ['26', '11'],
    ['a-show-slug', '11'],
    ['2026', 'november'],
  ])('404s /shows/%s/%s', async (year, month) => {
    await expect(
      ShowsMonthPage({ params: monthParams(year, month), searchParams })
    ).rejects.toThrow(NOT_FOUND)
  })
})

describe('/shows/{yyyy}/{mm}/{dd}, segment validation', () => {
  it('renders a well-formed day', async () => {
    await expect(
      ShowsDayPage({ params: dayParams('2026', '11', '14'), searchParams })
    ).resolves.toBeTruthy()
  })

  // The leap day is the case a range check alone gets wrong: both spellings
  // pass `01-31`, and only the calendar can say which one is a document.
  it('renders a leap day in a leap year', async () => {
    await expect(
      ShowsDayPage({ params: dayParams('2028', '02', '29'), searchParams })
    ).resolves.toBeTruthy()
  })

  it.each([
    ['2027', '02', '29'],
    ['2026', '11', '31'],
    ['2026', '11', '32'],
    ['2026', '11', '4'],
    ['2026', '13', '14'],
  ])('404s /shows/%s/%s/%s', async (year, month, day) => {
    await expect(
      ShowsDayPage({ params: dayParams(year, month, day), searchParams })
    ).rejects.toThrow(NOT_FOUND)
  })
})

describe('/shows/{yyyy}/{mm}/{dd}?days=N, the run', () => {
  const withDays = (days: string) => Promise.resolve({ days })

  it.each(['2', '3', '7', '14'])('renders a run of %s days', async days => {
    await expect(
      ShowsDayPage({
        params: dayParams('2026', '11', '14'),
        searchParams: withDays(days),
      })
    ).resolves.toBeTruthy()
  })

  // A run this route will not serve is a NOT-FOUND rather than the nearest run
  // it would: a window names the days it lists, and answering `?days=99` with a
  // fortnight puts a span on screen that the address contradicts.
  it.each(['15', '99', '0', '-3', '3.5', '+3', '03', 'three'])(
    '404s ?days=%s',
    async days => {
      await expect(
        ShowsDayPage({
          params: dayParams('2026', '11', '14'),
          searchParams: withDays(days),
        })
      ).rejects.toThrow(NOT_FOUND)
    }
  )

  // A key with no value names no run. Query-string builders write `days=` when
  // they clear the key, and refusing it would 404 a real day over a URL that
  // said nothing.
  it('renders the day for a valueless ?days=', async () => {
    const metadata = await dayMetadata({
      params: dayParams('2026', '11', '14'),
      searchParams: withDays(''),
    })

    expect(metadata.title).toBe('Shows on November 14, 2026')
    await expect(
      ShowsDayPage({
        params: dayParams('2026', '11', '14'),
        searchParams: withDays(''),
      })
    ).resolves.toBeTruthy()
  })

  // Two spellings of one window would be two addresses for one page, so the
  // one-day run is the day itself, under the day's own title and canonical.
  it('reads ?days=1 as the day itself', async () => {
    const metadata = await dayMetadata({
      params: dayParams('2026', '11', '14'),
      searchParams: withDays('1'),
    })

    expect(metadata.title).toBe('Shows on November 14, 2026')
    expect(metadata.robots).toBeUndefined()
  })

  it('names the span, canonicalizes to the day root, and noindexes it', async () => {
    const metadata = await dayMetadata({
      params: dayParams('2026', '11', '14'),
      searchParams: withDays('3'),
    })

    expect(metadata.title).toBe('Shows from Nov 14 to Nov 16, 2026')
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/11/14'
    )
    expect(metadata.robots).toEqual({ index: false, follow: true })
  })

  // The run rolls over a month, a year and a leap day on the calendar rather
  // than on the anchor month's length.
  it.each([
    [['2026', '11', '29'], '7', 'Shows from Nov 29 to Dec 5, 2026'],
    [['2026', '12', '29'], '7', 'Shows from Dec 29, 2026 to Jan 4, 2027'],
    [['2028', '02', '27'], '3', 'Shows from Feb 27 to Feb 29, 2028'],
    [['2027', '02', '27'], '3', 'Shows from Feb 27 to Mar 1, 2027'],
  ])('names the span of %s over %s days', async (date, days, want) => {
    const [year, month, day] = date
    const metadata = await dayMetadata({
      params: dayParams(year, month, day),
      searchParams: withDays(days),
    })

    expect(metadata.title).toBe(want)
  })

  /**
   * The head reads `?days=` and NOTHING else from the query. A page number is a
   * slice of the run, not another window, so it carries the same canonical the
   * run does, which is the site's canonicalize-to-root pagination policy.
   */
  it('cannot be reached by a page number', async () => {
    const metadata = await dayMetadata({
      params: dayParams('2026', '11', '14'),
      searchParams: Promise.resolve({ days: '3', page: '2' }),
    })

    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/11/14'
    )
  })

  // `?days=` names a run, and a month is not anchored on a date, so there is no
  // run for it to name there and nothing for it to break.
  it('ignores ?days= on a month', async () => {
    const metadata = await generateMetadata({ params: monthParams('2026', '11') })
    expect(metadata.title).toBe('Shows in November 2026')

    await expect(
      ShowsMonthPage({
        params: monthParams('2026', '11'),
        searchParams: withDays('99'),
      })
    ).resolves.toBeTruthy()
  })
})

describe('generateMetadata', () => {
  it('names the month and self-canonicalizes', async () => {
    const metadata = await generateMetadata({ params: monthParams('2026', '11') })

    expect(metadata.title).toBe('Shows in November 2026')
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/11'
    )
  })

  it('names the day and self-canonicalizes', async () => {
    const metadata = await dayMetadata({
      params: dayParams('2026', '11', '14'),
      searchParams,
    })

    expect(metadata.title).toBe('Shows on November 14, 2026')
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/11/14'
    )
    expect(metadata.robots).toBeUndefined()
  })

  /**
   * The load-bearing property of the params-only signature: the function cannot
   * see `?page=`, so every page of a month carries the month root's canonical
   * whatever the URL said. That is the site's canonicalize-to-root pagination
   * policy (`listRootCanonical`, PSY-1767), and here it holds structurally
   * rather than by remembering to apply it.
   */
  it('cannot be reached by a page number', async () => {
    expect(generateMetadata.length).toBe(1)
    const metadata = await generateMetadata({
      params: monthParams('2026', '11'),
      // A search-param argument is not part of the signature; passing one
      // changes nothing about the canonical.
      ...({ searchParams: Promise.resolve({ page: '2' }) } as object),
    })
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/11'
    )
  })

  it('zero-pads a single-digit month in the canonical', async () => {
    const metadata = await generateMetadata({ params: monthParams('2026', '09') })

    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/09'
    )
  })

  it('declines to name a month it cannot parse, and noindexes it', async () => {
    const metadata = await generateMetadata({ params: monthParams('2026', '13') })

    expect(metadata.title).toBe('Shows not found')
    expect(metadata.robots).toEqual({ index: false, follow: false })
    expect(metadata.alternates?.canonical).toBeUndefined()
  })
})
