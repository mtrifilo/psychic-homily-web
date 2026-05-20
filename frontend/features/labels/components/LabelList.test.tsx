import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { LabelListItem } from '../types'

// Mock next/navigation
const mockPush = vi.fn()
const mockGet = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => ({ get: mockGet }),
}))

// Mock data hook
const mockUseLabels = vi.fn()
vi.mock('../hooks/useLabels', () => ({
  useLabels: (opts: unknown) => mockUseLabels(opts),
}))

const mockUseDensity = vi.fn()
vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: (key: string) => mockUseDensity(key),
}))

// Mock the search child (its own suite covers it).
vi.mock('./LabelSearch', () => ({
  LabelSearch: () => <div data-testid="label-search">LabelSearch</div>,
}))

vi.mock('@/components/shared', () => ({
  LoadingSpinner: () => <div data-testid="loading-spinner">Loading...</div>,
  DensityToggle: ({ density }: { density: string; onDensityChange: (v: string) => void }) => (
    <div data-testid="density-toggle">{density}</div>
  ),
  EntityCardTitle: ({ name, href }: { name: string; href: string }) => (
    <a href={href}>
      <h3 title={name}>{name}</h3>
    </a>
  ),
}))

vi.mock('@/features/tags', () => ({
  TagFacetPanel: () => <div data-testid="tag-facet-panel" />,
  TagFacetSheet: () => <div data-testid="tag-facet-sheet" />,
  parseTagsParam: (s: string | null) => (s ? s.split(',').filter(Boolean) : []),
  buildTagsParam: (slugs: string[]) => slugs.join(','),
}))

import { LabelList } from './LabelList'

function makeLabel(overrides: Partial<LabelListItem> = {}): LabelListItem {
  return {
    id: 1,
    name: 'Sub Pop',
    slug: 'sub-pop',
    city: 'Seattle',
    state: 'WA',
    status: 'active',
    artist_count: 12,
    release_count: 340,
    ...overrides,
  }
}

describe('LabelList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockReturnValue(null)
    mockUseDensity.mockReturnValue({ density: 'comfortable', setDensity: vi.fn() })
    mockUseLabels.mockReturnValue({
      data: { labels: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
  })

  it('shows the loading spinner on initial load (no data yet)', () => {
    mockUseLabels.mockReturnValue({
      data: undefined,
      isLoading: true,
      isFetching: true,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<LabelList />)
    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
  })

  it('renders the search component and density toggle', () => {
    renderWithProviders(<LabelList />)
    expect(screen.getByTestId('label-search')).toBeInTheDocument()
    expect(screen.getByTestId('density-toggle')).toHaveTextContent('comfortable')
  })

  it('renders the unfiltered empty state when there are no labels', () => {
    renderWithProviders(<LabelList />)
    expect(
      screen.getByText('No labels available at this time.')
    ).toBeInTheDocument()
  })

  it('renders the filtered empty state when a status filter is applied', () => {
    mockGet.mockImplementation((key: string) =>
      key === 'status' ? 'defunct' : null
    )

    renderWithProviders(<LabelList />)
    expect(
      screen.getByText('No labels found matching your filters.')
    ).toBeInTheDocument()
    expect(screen.getByText('View all labels')).toBeInTheDocument()
  })

  it('renders label cards and the count line when data is present', () => {
    mockUseLabels.mockReturnValue({
      data: {
        labels: [
          makeLabel({ id: 1, name: 'Sub Pop', slug: 'sub-pop' }),
          makeLabel({ id: 2, name: 'Merge Records', slug: 'merge-records' }),
        ],
        count: 2,
      },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<LabelList />)
    expect(screen.getByText('Sub Pop')).toBeInTheDocument()
    expect(screen.getByText('Merge Records')).toBeInTheDocument()
    expect(screen.getByTestId('label-count')).toHaveTextContent('2 labels')
  })

  it('uses the singular noun for a single label', () => {
    mockUseLabels.mockReturnValue({
      data: { labels: [makeLabel()], count: 1 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<LabelList />)
    expect(screen.getByTestId('label-count')).toHaveTextContent('1 label')
  })

  it('shows an error state with a retry button', () => {
    mockUseLabels.mockReturnValue({
      data: { labels: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: new Error('boom'),
      refetch: vi.fn(),
    })

    renderWithProviders(<LabelList />)
    expect(
      screen.getByText('Failed to load labels. Please try again later.')
    ).toBeInTheDocument()
    expect(screen.getByText('Retry')).toBeInTheDocument()
  })

  it('calls refetch when the retry button is clicked', async () => {
    const user = userEvent.setup()
    const refetch = vi.fn()
    mockUseLabels.mockReturnValue({
      data: { labels: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: new Error('boom'),
      refetch,
    })

    renderWithProviders(<LabelList />)
    await user.click(screen.getByText('Retry'))
    expect(refetch).toHaveBeenCalledOnce()
  })

  it('passes the status URL param through to useLabels', () => {
    mockGet.mockImplementation((key: string) =>
      key === 'status' ? 'inactive' : null
    )

    renderWithProviders(<LabelList />)
    expect(mockUseLabels).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'inactive' })
    )
  })

  it('parses tags + tag_match from the URL and forwards them', () => {
    mockGet.mockImplementation((key: string) => {
      if (key === 'tags') return 'punk,noise'
      if (key === 'tag_match') return 'any'
      return null
    })

    renderWithProviders(<LabelList />)
    expect(mockUseLabels).toHaveBeenCalledWith(
      expect.objectContaining({
        tags: ['punk', 'noise'],
        tagMatch: 'any',
      })
    )
  })

  it('navigates to a status-filtered URL when a status chip is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LabelList />)

    // The status row renders an "Active" chip among others.
    await user.click(screen.getByRole('button', { name: 'Active' }))
    expect(mockPush).toHaveBeenCalledWith('/labels?status=active', {
      scroll: false,
    })
  })
})
