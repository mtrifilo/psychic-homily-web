import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ReleaseListItem } from '../types'

// next/navigation: URL params drive every filter in this component.
const mockPush = vi.fn()
const mockGet = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => ({ get: mockGet }),
}))

const mockUseReleases = vi.fn()
vi.mock('../hooks/useReleases', () => ({
  useReleases: (opts: unknown) => mockUseReleases(opts),
}))

const mockUseLabels = vi.fn()
vi.mock('@/features/labels/hooks/useLabels', () => ({
  useLabels: () => mockUseLabels(),
}))

const mockUseDensity = vi.fn()
vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: (key: string) => mockUseDensity(key),
}))

// ReleaseCard is exercised by its own sibling test; stub it to a title marker.
vi.mock('./ReleaseCard', () => ({
  ReleaseCard: ({ release }: { release: ReleaseListItem }) => (
    <div data-testid="release-card">{release.title}</div>
  ),
}))

vi.mock('@/components/shared', () => ({
  LoadingSpinner: () => <div data-testid="loading-spinner">Loading...</div>,
  DensityToggle: ({ density }: { density: string }) => (
    <div data-testid="density-toggle">{density}</div>
  ),
}))

vi.mock('@/features/tags', () => ({
  TagFacetPanel: () => <div data-testid="tag-facet-panel" />,
  TagFacetSheet: () => <div data-testid="tag-facet-sheet" />,
  parseTagsParam: (s: string | null) => (s ? s.split(',').filter(Boolean) : []),
  buildTagsParam: (slugs: string[]) => slugs.join(','),
}))

import { ReleaseList } from './ReleaseList'

function makeRelease(overrides: Partial<ReleaseListItem> = {}): ReleaseListItem {
  return {
    id: 1,
    title: 'In Rainbows',
    slug: 'in-rainbows',
    release_type: 'lp',
    release_year: 2007,
    cover_art_url: null,
    artist_count: 1,
    artists: [{ id: 1, name: 'Radiohead', slug: 'radiohead' }],
    label_name: 'XL Recordings',
    label_slug: 'xl-recordings',
    ...overrides,
  }
}

describe('ReleaseList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockReturnValue(null)
    mockUseDensity.mockReturnValue({
      density: 'comfortable',
      setDensity: vi.fn(),
    })
    mockUseLabels.mockReturnValue({ data: { labels: [] } })
    mockUseReleases.mockReturnValue({
      data: { releases: [], total: 0, limit: 50, offset: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
  })

  it('shows the loading spinner on initial load', () => {
    mockUseReleases.mockReturnValue({
      data: undefined,
      isLoading: true,
      isFetching: true,
      error: null,
      refetch: vi.fn(),
    })
    renderWithProviders(<ReleaseList />)
    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
  })

  it('renders the density toggle with the current density', () => {
    renderWithProviders(<ReleaseList />)
    expect(screen.getByTestId('density-toggle')).toHaveTextContent(
      'comfortable'
    )
  })

  it('renders the unfiltered empty state', () => {
    renderWithProviders(<ReleaseList />)
    expect(
      screen.getByText('No releases available at this time.')
    ).toBeInTheDocument()
  })

  it('renders the filtered empty state when a type filter is set', () => {
    mockGet.mockImplementation((key: string) => (key === 'type' ? 'lp' : null))
    renderWithProviders(<ReleaseList />)
    expect(
      screen.getByText('No releases found matching your filters.')
    ).toBeInTheDocument()
    expect(screen.getByText('View all releases')).toBeInTheDocument()
  })

  it('renders release cards when data is available', () => {
    mockUseReleases.mockReturnValue({
      data: {
        releases: [
          makeRelease({ id: 1, title: 'In Rainbows' }),
          makeRelease({ id: 2, title: 'Kid A', slug: 'kid-a' }),
        ],
        total: 2,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
    renderWithProviders(<ReleaseList />)
    expect(screen.getByText('In Rainbows')).toBeInTheDocument()
    expect(screen.getByText('Kid A')).toBeInTheDocument()
  })

  it('renders the result count with correct pluralization', () => {
    mockUseReleases.mockReturnValue({
      data: {
        releases: [makeRelease()],
        total: 1,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
    renderWithProviders(<ReleaseList />)
    expect(screen.getByTestId('release-count')).toHaveTextContent('1 release')
  })

  it('shows the error state with a retry button', () => {
    mockUseReleases.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      error: new Error('Network error'),
      refetch: vi.fn(),
    })
    renderWithProviders(<ReleaseList />)
    expect(
      screen.getByText('Failed to load releases. Please try again later.')
    ).toBeInTheDocument()
    expect(screen.getByText('Retry')).toBeInTheDocument()
  })

  it('calls refetch when the retry button is clicked', async () => {
    const user = userEvent.setup()
    const refetch = vi.fn()
    mockUseReleases.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      error: new Error('Network error'),
      refetch,
    })
    renderWithProviders(<ReleaseList />)
    await user.click(screen.getByText('Retry'))
    expect(refetch).toHaveBeenCalledOnce()
  })

  it('renders the label dropdown when labels are available', () => {
    mockUseLabels.mockReturnValue({
      data: {
        labels: [
          { id: 1, name: 'XL Recordings' },
          { id: 2, name: 'Sub Pop' },
        ],
      },
    })
    renderWithProviders(<ReleaseList />)
    expect(screen.getByText('All Labels')).toBeInTheDocument()
    expect(screen.getByText('Sub Pop')).toBeInTheDocument()
  })

  it('does not render the label dropdown when there are no labels', () => {
    mockUseLabels.mockReturnValue({ data: { labels: [] } })
    renderWithProviders(<ReleaseList />)
    expect(screen.queryByText('All Labels')).not.toBeInTheDocument()
  })

  it('passes URL filters through to useReleases', () => {
    mockGet.mockImplementation((key: string) => {
      const params: Record<string, string> = {
        type: 'ep',
        year: '2020',
        search: 'rainbows',
        sort: 'oldest',
        label_id: '3',
      }
      return params[key] ?? null
    })
    renderWithProviders(<ReleaseList />)
    expect(mockUseReleases).toHaveBeenCalledWith(
      expect.objectContaining({
        releaseType: 'ep',
        year: 2020,
        search: 'rainbows',
        sort: 'oldest',
        labelId: 3,
      })
    )
  })

  it('parses tags from the URL and passes them with AND semantics by default', () => {
    mockGet.mockImplementation((key: string) =>
      key === 'tags' ? 'post-punk,shoegaze' : null
    )
    renderWithProviders(<ReleaseList />)
    expect(mockUseReleases).toHaveBeenCalledWith(
      expect.objectContaining({
        tags: ['post-punk', 'shoegaze'],
        tagMatch: 'all',
      })
    )
  })

  it('honors tag_match=any from the URL', () => {
    mockGet.mockImplementation((key: string) => {
      if (key === 'tags') return 'post-punk'
      if (key === 'tag_match') return 'any'
      return null
    })
    renderWithProviders(<ReleaseList />)
    expect(mockUseReleases).toHaveBeenCalledWith(
      expect.objectContaining({ tagMatch: 'any' })
    )
  })

  it('computes the page offset from the page URL param', () => {
    mockGet.mockImplementation((key: string) => (key === 'page' ? '3' : null))
    renderWithProviders(<ReleaseList />)
    // Page 3 with PAGE_SIZE 50 → offset 100.
    expect(mockUseReleases).toHaveBeenCalledWith(
      expect.objectContaining({ offset: 100, limit: 50 })
    )
  })

  it('renders pagination controls when there is more than one page', () => {
    mockUseReleases.mockReturnValue({
      data: {
        releases: [makeRelease()],
        total: 120,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
    renderWithProviders(<ReleaseList />)
    expect(screen.getByText('Page 1 of 3')).toBeInTheDocument()
    expect(screen.getByText('Next')).toBeInTheDocument()
  })

  it('does not render pagination for a single page of results', () => {
    mockUseReleases.mockReturnValue({
      data: {
        releases: [makeRelease()],
        total: 10,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
    renderWithProviders(<ReleaseList />)
    expect(screen.queryByText(/Page \d+ of/)).not.toBeInTheDocument()
  })
})
