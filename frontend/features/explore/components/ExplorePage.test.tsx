import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ExplorePage } from './ExplorePage'
import type { ExploreFeaturedResponse } from '../types'

const mockUseExploreFeatured = vi.fn<() => {
  data: ExploreFeaturedResponse | undefined
}>(() => ({ data: undefined }))

vi.mock('../hooks', () => ({
  useExploreFeatured: () => mockUseExploreFeatured(),
}))

// Stub the heavy children so we can assert layout assembly without their
// internals.
vi.mock('./UpcomingShowsList', () => ({
  UpcomingShowsList: () => <div data-testid="upcoming-shows-list" />,
}))
vi.mock('./FeaturedBillCard', () => ({
  FeaturedBillCard: ({ bill }: { bill: { title: string } }) => (
    <div data-testid="featured-bill-card">{bill.title}</div>
  ),
}))
vi.mock('./FeaturedCollectionCard', () => ({
  FeaturedCollectionCard: ({
    collection,
  }: {
    collection: { title: string }
  }) => <div data-testid="featured-collection-card">{collection.title}</div>,
}))
vi.mock('./InlineGraph', () => ({
  InlineGraph: () => <div data-testid="inline-graph" />,
}))
vi.mock('./ShuffleCta', () => ({
  ShuffleCta: () => <div data-testid="shuffle-cta" />,
}))

const sampleFeatured: ExploreFeaturedResponse = {
  bill: {
    id: 1,
    slug: 'bill-slug',
    title: 'Big Bill',
    event_date: '2026-06-15T03:00:00Z',
    headliner_name: 'Cool Band',
    venue_name: 'The Trunk Space',
    venue_city: 'Phoenix',
    venue_state: 'AZ',
  },
  collection: {
    id: 2,
    slug: 'arizona-noise',
    title: 'Arizona Noise',
  },
}

beforeEach(() => {
  mockUseExploreFeatured.mockReset()
})

describe('ExplorePage', () => {
  it('renders the heading + upcoming shows + shuffle CTA when featured slots are empty', () => {
    mockUseExploreFeatured.mockReturnValue({
      data: { bill: null, collection: null },
    })
    render(<ExplorePage />)
    expect(
      screen.getByRole('heading', { level: 1, name: /explore/i }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('upcoming-shows-list')).toBeInTheDocument()
    expect(screen.getByTestId('shuffle-cta')).toBeInTheDocument()
    // Featured + graph + collection sections collapse when null.
    expect(screen.queryByTestId('featured-bill-card')).toBeNull()
    expect(screen.queryByTestId('inline-graph')).toBeNull()
    expect(screen.queryByTestId('featured-collection-card')).toBeNull()
  })

  it('renders the bill + graph + collection sections when featured returns both', () => {
    mockUseExploreFeatured.mockReturnValue({ data: sampleFeatured })
    render(<ExplorePage />)
    expect(screen.getByTestId('featured-bill-card')).toHaveTextContent(
      'Big Bill',
    )
    expect(screen.getByTestId('inline-graph')).toBeInTheDocument()
    expect(screen.getByTestId('featured-collection-card')).toHaveTextContent(
      'Arizona Noise',
    )
  })

  it('renders the bill + graph but no collection when only the bill is set', () => {
    mockUseExploreFeatured.mockReturnValue({
      data: { bill: sampleFeatured.bill, collection: null },
    })
    render(<ExplorePage />)
    expect(screen.getByTestId('featured-bill-card')).toBeInTheDocument()
    expect(screen.getByTestId('inline-graph')).toBeInTheDocument()
    expect(screen.queryByTestId('featured-collection-card')).toBeNull()
  })

  it('renders the collection but no bill/graph when only the collection is set', () => {
    mockUseExploreFeatured.mockReturnValue({
      data: { bill: null, collection: sampleFeatured.collection },
    })
    render(<ExplorePage />)
    expect(screen.queryByTestId('featured-bill-card')).toBeNull()
    expect(screen.queryByTestId('inline-graph')).toBeNull()
    expect(screen.getByTestId('featured-collection-card')).toBeInTheDocument()
  })
})
