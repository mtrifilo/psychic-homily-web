import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FeaturedCollectionCard } from './FeaturedCollectionCard'
import type { FeaturedCollectionRun } from '../types'

let featured: FeaturedCollectionRun | null = null
let featuredLoading = false
let featuredError = false
let historyRuns: FeaturedCollectionRun[] = []
let historyLoading = false
let historyError = false
let historyEnabled: boolean | undefined

vi.mock('@/components/shared', () => ({
  UserAttribution: ({ name }: { name: string }) => (
    <span data-testid="curator">{name}</span>
  ),
}))

vi.mock('@/features/collections/components/CollectionCoverImage', () => ({
  CollectionCoverImage: ({
    alt,
    fallback,
  }: {
    alt: string
    fallback: ReactNode
  }) => <div data-testid="cover" aria-label={alt}>{fallback}</div>,
}))

vi.mock('../hooks', () => ({
  useFeaturedCollection: () => ({
    data: featuredLoading || featuredError ? undefined : { featured },
    isLoading: featuredLoading,
    isError: featuredError,
  }),
  useFeaturedCollectionHistory: (
    _limit?: number,
    _offset?: number,
    opts?: { enabled?: boolean }
  ) => {
    historyEnabled = opts?.enabled
    return {
      data:
        historyLoading || historyError
          ? undefined
          : { total: historyRuns.length, runs: historyRuns },
      isLoading: historyLoading,
      isError: historyError,
      isSuccess: !historyLoading && !historyError,
    }
  },
}))

function makeRun(
  overrides: Partial<FeaturedCollectionRun> = {}
): FeaturedCollectionRun {
  return {
    run_id: 1,
    collection_id: 10,
    title: 'Desert Psych Comp',
    slug: 'desert-psych-comp',
    description: 'A desert-psych shortlist.',
    cover_image_url: null,
    creator_id: 3,
    creator_name: 'Ada',
    creator_username: 'ada',
    item_count: 12,
    subscriber_count: 4,
    featured_at: '2026-07-01T00:00:00Z',
    unfeatured_at: null,
    featured_at_estimated: false,
    ...overrides,
  }
}

describe('FeaturedCollectionCard', () => {
  beforeEach(() => {
    featured = null
    featuredLoading = false
    featuredError = false
    historyRuns = []
    historyLoading = false
    historyError = false
    historyEnabled = undefined
  })

  it('renders nothing while loading, on error, or when nothing is featured', () => {
    featuredLoading = true
    const { container: loading } = render(<FeaturedCollectionCard />)
    expect(loading).toBeEmptyDOMElement()

    featuredLoading = false
    featuredError = true
    const { container: errored } = render(<FeaturedCollectionCard />)
    expect(errored).toBeEmptyDOMElement()

    featuredError = false
    featured = null
    const { container: empty } = render(<FeaturedCollectionCard />)
    expect(empty).toBeEmptyDOMElement()
    expect(historyEnabled).toBe(false)
  })

  it('renders the Almanac card with discuss → into the collection thread', () => {
    featured = makeRun()
    render(<FeaturedCollectionCard />)

    expect(screen.getByTestId('featured-collection')).toBeInTheDocument()
    expect(screen.getByText('Featured Collection')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Desert Psych Comp' })
    ).toHaveAttribute('href', '/collections/desert-psych-comp')
    expect(screen.getByText('A desert-psych shortlist.')).toBeInTheDocument()
    expect(screen.getByTestId('curator')).toHaveTextContent('Ada')
    expect(screen.getByText(/12 items/)).toBeInTheDocument()
    expect(screen.getByText(/4 subscribers/)).toBeInTheDocument()
    expect(screen.getByText(/featured July 1, 2026/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'discuss →' })).toHaveAttribute(
      'href',
      '/collections/desert-psych-comp#discussion'
    )
    expect(historyEnabled).toBe(true)
  })

  it('renders estimated starts as "featured before <date>"', () => {
    featured = makeRun({ featured_at_estimated: true })
    render(<FeaturedCollectionCard />)
    expect(screen.getByText(/featured before July 1, 2026/)).toBeInTheDocument()
  })

  it('hides previously featured → until history settles, and when no closed run exists', () => {
    featured = makeRun()
    historyLoading = true
    const { rerender } = render(<FeaturedCollectionCard />)
    expect(
      screen.queryByRole('link', { name: 'previously featured →' })
    ).not.toBeInTheDocument()

    historyLoading = false
    historyRuns = [makeRun()] // open only
    rerender(<FeaturedCollectionCard />)
    expect(
      screen.queryByRole('link', { name: 'previously featured →' })
    ).not.toBeInTheDocument()
  })

  it('shows previously featured → only when a closed run exists', () => {
    featured = makeRun()
    historyRuns = [
      makeRun(),
      makeRun({
        run_id: 2,
        collection_id: 11,
        title: 'Older Pick',
        slug: 'older-pick',
        unfeatured_at: '2026-06-01T00:00:00Z',
      }),
    ]
    render(<FeaturedCollectionCard />)

    expect(
      screen.getByRole('link', { name: 'previously featured →' })
    ).toHaveAttribute('href', '/charts/featured')
  })

  it('suppresses previously featured → when history errors', () => {
    featured = makeRun()
    historyError = true
    render(<FeaturedCollectionCard />)
    expect(
      screen.queryByRole('link', { name: 'previously featured →' })
    ).not.toBeInTheDocument()
  })
})
