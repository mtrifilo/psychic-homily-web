import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VenueList } from './VenueList'
import type { VenueWithShowCount } from '../types'

// Mock next/navigation
const mockPush = vi.fn()
const mockSearchParams = vi.fn(() => ({
  get: vi.fn((_key: string): string | null => null),
}))
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => mockSearchParams(),
}))

// Mock venue hooks
const mockUseVenues = vi.fn()
const mockUseVenueCities = vi.fn()
vi.mock('../hooks/useVenues', () => ({
  useVenues: (opts: unknown) => mockUseVenues(opts),
  useVenueCities: () => mockUseVenueCities(),
}))

// PSY-1003: mock the tag facet components. The TagFacetPanel mock echoes the
// `layout` prop into `data-layout` so the test can assert the bar layout via
// the same `data-layout="bar|rail"` seam the real component exposes.
vi.mock('@/features/tags', () => ({
  TagFacetPanel: ({ layout = 'rail' }: { layout?: 'rail' | 'bar' }) => (
    <div data-testid="tag-facet-panel" data-layout={layout} />
  ),
  TagFacetSheet: () => <div data-testid="tag-facet-sheet" />,
  parseTagsParam: (s: string | null) => (s ? s.split(',').filter(Boolean) : []),
  buildTagsParam: (slugs: string[]) => slugs.join(','),
}))

// Mock child components
vi.mock('./VenueCard', () => ({
  VenueCard: ({ venue }: { venue: VenueWithShowCount }) => (
    <article data-testid={`venue-card-${venue.id}`}>{venue.name}</article>
  ),
}))

vi.mock('./VenueSearch', () => ({
  VenueSearch: () => <div data-testid="venue-search" />,
}))

vi.mock('@/components/filters', () => ({
  CityFilters: () => <div data-testid="city-filters" />,
}))

vi.mock('@/components/shared', () => ({
  LoadingSpinner: () => <div data-testid="loading-spinner" />,
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    onClick,
    disabled,
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
  }) => (
    <button onClick={onClick} disabled={disabled}>
      {children}
    </button>
  ),
}))

function makeVenue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 1,
    slug: 'test-venue',
    name: 'Test Venue',
    address: null,
    city: 'Phoenix',
    state: 'AZ',
    verified: true,
    upcoming_show_count: 0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('VenueList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams.mockReturnValue({
      get: vi.fn((_key: string): string | null => null),
    })
    mockUseVenueCities.mockReturnValue({
      data: { cities: [] },
      isLoading: false,
      isFetching: false,
    })
  })

  function mockVenues(venues: VenueWithShowCount[], total = venues.length) {
    mockUseVenues.mockReturnValue({
      data: { venues, total, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
  }

  describe('top-bar tag filter layout (PSY-1003)', () => {
    it('renders the tag facet panel in bar layout, not the rail', () => {
      mockVenues([makeVenue()])
      render(<VenueList />)
      // Only the desktop top-bar panel exists (the Sheet is a separate stub).
      const panel = screen.getByTestId('tag-facet-panel')
      expect(panel).toHaveAttribute('data-layout', 'bar')
    })

    it('does not render a left-rail aside wrapper', () => {
      mockVenues([makeVenue()])
      const { container } = render(<VenueList />)
      // The pre-PSY-1003 layout wrapped the panel in `<aside class="lg:w-64">`.
      // The bar layout drops that rail entirely.
      expect(container.querySelector('aside.lg\\:w-64')).toBeNull()
      // And the list column no longer uses the 2-column `flex-1` rail child.
      expect(container.querySelector('.flex-1')).toBeNull()
    })
  })

  describe('list rendering', () => {
    it('renders a card per venue', () => {
      mockVenues([
        makeVenue({ id: 1, name: 'Venue One' }),
        makeVenue({ id: 2, name: 'Venue Two' }),
      ])
      render(<VenueList />)
      expect(screen.getByTestId('venue-card-1')).toBeInTheDocument()
      expect(screen.getByTestId('venue-card-2')).toBeInTheDocument()
    })

    it('shows the venue count', () => {
      mockVenues([makeVenue()], 1)
      render(<VenueList />)
      expect(screen.getByTestId('venue-count')).toHaveTextContent('1 of 1 venue')
    })
  })

  describe('empty state', () => {
    it('shows the default empty message with no filters', () => {
      mockVenues([], 0)
      render(<VenueList />)
      expect(
        screen.getByText('No venues available at this time.')
      ).toBeInTheDocument()
    })

    it('shows the filtered empty message when a tag is selected', () => {
      mockSearchParams.mockReturnValue({
        get: vi.fn((key: string): string | null =>
          key === 'tags' ? 'punk' : null
        ),
      })
      mockVenues([], 0)
      render(<VenueList />)
      expect(
        screen.getByText('No venues match the current filters.')
      ).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message and retry on failure', async () => {
      const mockRefetch = vi.fn()
      mockUseVenues.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
        error: new Error('Network error'),
        refetch: mockRefetch,
      })
      const user = userEvent.setup()
      render(<VenueList />)
      expect(
        screen.getByText('Failed to load venues. Please try again later.')
      ).toBeInTheDocument()
      await user.click(screen.getByText('Retry'))
      expect(mockRefetch).toHaveBeenCalled()
    })
  })
})
