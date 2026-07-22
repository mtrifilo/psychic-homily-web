import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { FeaturedArchivePage } from './FeaturedArchivePage'
import type { FeaturedCollectionRun } from '../types'

const mockHistory = vi.fn()

vi.mock('../hooks', () => ({
  useFeaturedCollectionHistory: () => mockHistory(),
}))

function run(overrides: Partial<FeaturedCollectionRun> = {}): FeaturedCollectionRun {
  return {
    run_id: 1,
    collection_id: 10,
    title: 'Desert Punk Starter Pack',
    slug: 'desert-punk-starter-pack',
    description: 'The Phoenix/Tucson underground since 2020.',
    cover_image_url: null,
    creator_id: 5,
    creator_name: 'mtrifilo',
    creator_username: 'mtrifilo',
    item_count: 24,
    featured_at: '2026-05-30T00:00:00Z',
    unfeatured_at: null,
    featured_at_estimated: false,
    ...overrides,
  }
}

function success(runs: FeaturedCollectionRun[]) {
  return {
    data: { total: runs.length, runs },
    isLoading: false,
    isError: false,
  }
}

describe('FeaturedArchivePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('always renders the header + back link, even while loading', () => {
    mockHistory.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    })

    render(<FeaturedArchivePage />)

    expect(
      screen.getByRole('heading', { name: 'Previously Featured', level: 1 })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '← Charts' })).toHaveAttribute(
      'href',
      '/charts'
    )
  })

  it('peels the newest run into the lead card with a "still live" label', () => {
    mockHistory.mockReturnValue(success([run()]))

    render(<FeaturedArchivePage />)

    expect(screen.getByText('FEATURED COLLECTION · MOST RECENT')).toBeInTheDocument()
    // Meta line reports the live run as still holding the slot.
    expect(
      screen.getByText(/curated by/i).textContent?.replace(/\s+/g, ' ').trim()
    ).toContain('featured May 30, 2026 — still live')
    // Lead discuss link reaches the collection comment thread.
    const discuss = screen.getByRole('link', { name: 'discuss →' })
    expect(discuss).toHaveAttribute(
      'href',
      '/collections/desert-punk-starter-pack#discussion'
    )
    // A single run means no closed-run ledger.
    expect(screen.queryByText('PREVIOUSLY FEATURED')).not.toBeInTheDocument()
  })

  it('renders closed runs as a newest-first ledger with a run count', () => {
    mockHistory.mockReturnValue(
      success([
        run(),
        run({
          run_id: 2,
          collection_id: 11,
          title: 'Winter Basement Tapes',
          slug: 'winter-basement-tapes',
          item_count: 18,
          featured_at: '2026-04-12T00:00:00Z',
          unfeatured_at: '2026-05-30T00:00:00Z',
        }),
        run({
          run_id: 3,
          collection_id: 12,
          title: 'The Flenser Primer',
          slug: 'the-flenser-primer',
          creator_name: 'kate.h',
          creator_username: 'kate-h',
          item_count: 12,
          featured_at: '2026-01-08T00:00:00Z',
          unfeatured_at: '2026-03-04T00:00:00Z',
        }),
      ])
    )

    render(<FeaturedArchivePage />)

    expect(screen.getByText('PREVIOUSLY FEATURED')).toBeInTheDocument()
    expect(screen.getByText('2 closed runs')).toBeInTheDocument()
    expect(screen.getByText('Apr 12 – May 30, 2026')).toBeInTheDocument()
    expect(screen.getByText('Jan 08 – Mar 04, 2026')).toBeInTheDocument()

    // Ledger discuss link resolves to the collection's discussion anchor.
    const flenserRow = screen
      .getByText('The Flenser Primer')
      .closest('div[class*="border-b"]') as HTMLElement
    expect(
      within(flenserRow).getByRole('link', { name: 'discuss →' })
    ).toHaveAttribute('href', '/collections/the-flenser-primer#discussion')
  })

  it('keeps secondary still-open picks out of the previously-featured ledger', () => {
    mockHistory.mockReturnValue(
      success([
        run({ title: 'Newest Open', slug: 'newest-open' }),
        run({
          run_id: 2,
          collection_id: 11,
          title: 'Older Still Open',
          slug: 'older-still-open',
          featured_at: '2026-04-01T00:00:00Z',
          unfeatured_at: null,
        }),
        run({
          run_id: 3,
          collection_id: 12,
          title: 'Actually Closed',
          slug: 'actually-closed',
          featured_at: '2026-01-08T00:00:00Z',
          unfeatured_at: '2026-03-04T00:00:00Z',
        }),
      ])
    )

    render(<FeaturedArchivePage />)

    expect(screen.getByText('Newest Open')).toBeInTheDocument()
    expect(screen.queryByText('Older Still Open')).not.toBeInTheDocument()
    expect(screen.getByText('Actually Closed')).toBeInTheDocument()
    expect(screen.getByText('1 closed run')).toBeInTheDocument()
  })

  it('renders an estimated start as "before <date>", never a fabricated date', () => {
    mockHistory.mockReturnValue(
      success([
        run(),
        run({
          run_id: 9,
          title: 'Old Migration Pick',
          slug: 'old-migration-pick',
          item_count: 24,
          featured_at: '2025-01-01T00:00:00Z',
          unfeatured_at: '2026-01-08T00:00:00Z',
          featured_at_estimated: true,
        }),
      ])
    )

    render(<FeaturedArchivePage />)

    expect(screen.getByText('before Jan 08, 2026')).toBeInTheDocument()
    expect(
      screen.getByText(/start date reconstructed at migration/)
    ).toBeInTheDocument()
    // The estimated start is never rendered as a precise range.
    expect(screen.queryByText(/Jan 01, 2025 –/)).not.toBeInTheDocument()
  })

  it('shows the single-line empty state (and does not throw / 404) with no picks', () => {
    mockHistory.mockReturnValue(success([]))

    render(<FeaturedArchivePage />)

    expect(
      screen.getByText('No collection has been featured yet.')
    ).toBeInTheDocument()
    expect(screen.queryByText('FEATURED COLLECTION · MOST RECENT')).not.toBeInTheDocument()
    // Header still renders — the route is valid, just early.
    expect(
      screen.getByRole('heading', { name: 'Previously Featured', level: 1 })
    ).toBeInTheDocument()
  })

  it('surfaces a load error inline without collapsing the page', () => {
    mockHistory.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })

    render(<FeaturedArchivePage />)

    expect(
      screen.getByText('Unable to load the featured-collection archive.')
    ).toBeInTheDocument()
  })
})
