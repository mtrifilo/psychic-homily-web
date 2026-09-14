import { describe, it, expect, vi, beforeEach } from 'vitest'

// Throws, like the real one — which is typed `never`. A non-throwing mock lets
// the page body carry on past `notFound()` and return its Suspense element with
// a null window, so a test could assert "rejected the segments" while the route
// in fact rendered.
const NOT_FOUND = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND)
  }),
}))

// These tests exercise the route's OWN job — segment validation and the head —
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

describe('/shows/{yyyy}/{mm} — segment validation', () => {
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

describe('/shows/{yyyy}/{mm}/{dd} — segment validation', () => {
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
    })

    expect(metadata.title).toBe('Shows on November 14, 2026')
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/11/14'
    )
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
