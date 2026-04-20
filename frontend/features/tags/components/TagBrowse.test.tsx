import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { TagListItem } from '../types'

// ── Mocks ──────────────────────────────────────────

const mockUseTags = vi.fn()
vi.mock('../hooks', () => ({
  useTags: (...args: unknown[]) => mockUseTags(...args),
}))

// next/navigation — mutable URLSearchParams so tests can simulate different URLs.
const mockReplace = vi.fn()
const mockPush = vi.fn()
let mockSearchParamsStore = new URLSearchParams()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: mockReplace, push: mockPush }),
  useSearchParams: () => mockSearchParamsStore,
}))

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

import { TagBrowse, cloudFontSizePx } from './TagBrowse'

function setUrlParams(params: Record<string, string> = {}) {
  mockSearchParamsStore = new URLSearchParams(params)
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
}

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = createQueryClient()
  return render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
  )
}

function makeTag(overrides: Partial<TagListItem> = {}): TagListItem {
  return {
    id: 1,
    name: 'rock',
    slug: 'rock',
    category: 'genre',
    is_official: false,
    usage_count: 42,
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('TagBrowse', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setUrlParams()
    mockUseTags.mockReturnValue({
      data: { tags: [], total: 0 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })
  })

  // ── Loading state ──

  it('shows loading spinner on initial load', () => {
    mockUseTags.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  // ── Error state ──

  it('shows error message on error', () => {
    mockUseTags.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Server error'),
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(
      screen.getByText('Failed to load tags. Please try again later.')
    ).toBeInTheDocument()
  })

  it('shows Retry button on error', async () => {
    const mockRefetch = vi.fn()
    mockUseTags.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Server error'),
      refetch: mockRefetch,
    })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mockRefetch).toHaveBeenCalled()
  })

  // ── Empty state ──

  it('shows generic empty copy when no filter is active', () => {
    mockUseTags.mockReturnValue({
      data: { tags: [], total: 0 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('No tags found.')).toBeInTheDocument()
  })

  it('shows filter-aware empty copy when a category pill is active', async () => {
    mockUseTags.mockReturnValue({
      data: { tags: [], total: 0 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByRole('button', { name: 'Locale' }))

    expect(screen.getByText('No locale tags yet.')).toBeInTheDocument()
  })

  // ── Tag rendering (grid) ──

  it('renders tag cards as links in grid view (default)', () => {
    const tags = [
      makeTag({ id: 1, name: 'rock', slug: 'rock' }),
      makeTag({ id: 2, name: 'punk', slug: 'punk', category: 'other' }),
    ]
    mockUseTags.mockReturnValue({
      data: { tags, total: 2 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    const rockLink = screen.getByRole('link', { name: /rock/ })
    expect(rockLink).toHaveAttribute('href', '/tags/rock')

    const punkLink = screen.getByRole('link', { name: /punk/ })
    expect(punkLink).toHaveAttribute('href', '/tags/punk')

    // Default view is grid — tag cloud must not be rendered.
    expect(screen.queryByTestId('tag-cloud')).not.toBeInTheDocument()
  })

  it('renders usage count on tag cards', () => {
    mockUseTags.mockReturnValue({
      data: { tags: [makeTag({ usage_count: 42 })], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('42 uses')).toBeInTheDocument()
  })

  it('renders singular usage count', () => {
    mockUseTags.mockReturnValue({
      data: { tags: [makeTag({ usage_count: 1 })], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('1 use')).toBeInTheDocument()
  })

  it('renders Official badge on official tags', () => {
    mockUseTags.mockReturnValue({
      data: { tags: [makeTag({ is_official: true })], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('Official')).toBeInTheDocument()
  })

  it('renders total count', () => {
    mockUseTags.mockReturnValue({
      data: { tags: [makeTag()], total: 15 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('15 tags found')).toBeInTheDocument()
  })

  it('renders singular total count', () => {
    mockUseTags.mockReturnValue({
      data: { tags: [makeTag()], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('1 tag found')).toBeInTheDocument()
  })

  // ── Category filter tabs ──

  it('renders all category filter buttons plus "All"', () => {
    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('All')).toBeInTheDocument()
    expect(screen.getByText('Genre')).toBeInTheDocument()
    expect(screen.getByText('Locale')).toBeInTheDocument()
    expect(screen.getByText('Other')).toBeInTheDocument()
  })

  it('calls useTags with category when a category button is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByText('Genre'))

    const lastCall = mockUseTags.mock.calls[mockUseTags.mock.calls.length - 1]
    expect(lastCall[0]).toEqual(
      expect.objectContaining({ category: 'genre' })
    )
  })

  // ── Search ──

  it('renders search input', () => {
    renderWithProviders(<TagBrowse />)

    expect(screen.getByPlaceholderText('Search tags...')).toBeInTheDocument()
  })

  // ── Sort dropdown ──

  it('renders all sort options', () => {
    renderWithProviders(<TagBrowse />)

    const select = screen.getByLabelText('Sort tags') as HTMLSelectElement
    const values = Array.from(select.options).map(o => o.value)
    expect(values).toEqual(['popularity', 'alphabetical', 'newest'])
  })

  it('defaults to popularity sort → calls useTags with backend sort "usage"', () => {
    renderWithProviders(<TagBrowse />)

    const lastCall = mockUseTags.mock.calls[mockUseTags.mock.calls.length - 1]
    expect(lastCall[0]).toEqual(
      expect.objectContaining({ sort: 'usage' })
    )
  })

  it('reflects URL sort param ?sort=alphabetical in the select and in the useTags call', () => {
    setUrlParams({ sort: 'alphabetical' })

    renderWithProviders(<TagBrowse />)

    const select = screen.getByLabelText('Sort tags') as HTMLSelectElement
    expect(select.value).toBe('alphabetical')

    const lastCall = mockUseTags.mock.calls[mockUseTags.mock.calls.length - 1]
    expect(lastCall[0]).toEqual(expect.objectContaining({ sort: 'name' }))
  })

  it('reflects URL sort param ?sort=newest and maps to backend "created"', () => {
    setUrlParams({ sort: 'newest' })

    renderWithProviders(<TagBrowse />)

    const select = screen.getByLabelText('Sort tags') as HTMLSelectElement
    expect(select.value).toBe('newest')

    const lastCall = mockUseTags.mock.calls[mockUseTags.mock.calls.length - 1]
    expect(lastCall[0]).toEqual(expect.objectContaining({ sort: 'created' }))
  })

  it('selecting a non-default sort pushes it into the URL via router.replace', async () => {
    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.selectOptions(screen.getByLabelText('Sort tags'), 'alphabetical')

    expect(mockReplace).toHaveBeenCalled()
    const call = mockReplace.mock.calls[mockReplace.mock.calls.length - 1]
    expect(call[0]).toBe('/tags?sort=alphabetical')
  })

  it('selecting the default sort strips ?sort from the URL', async () => {
    setUrlParams({ sort: 'alphabetical' })
    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.selectOptions(screen.getByLabelText('Sort tags'), 'popularity')

    const call = mockReplace.mock.calls[mockReplace.mock.calls.length - 1]
    expect(call[0]).toBe('/tags')
  })

  // ── View toggle (grid / cloud) ──

  it('renders the view toggle with grid selected by default', () => {
    renderWithProviders(<TagBrowse />)

    const grid = screen.getByTestId('view-grid')
    const cloud = screen.getByTestId('view-cloud')

    expect(grid).toHaveAttribute('aria-checked', 'true')
    expect(cloud).toHaveAttribute('aria-checked', 'false')
  })

  it('clicking the cloud button pushes ?view=cloud into the URL', async () => {
    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByTestId('view-cloud'))

    const call = mockReplace.mock.calls[mockReplace.mock.calls.length - 1]
    expect(call[0]).toBe('/tags?view=cloud')
  })

  it('clicking back to grid strips ?view from the URL', async () => {
    setUrlParams({ view: 'cloud' })
    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByTestId('view-grid'))

    const call = mockReplace.mock.calls[mockReplace.mock.calls.length - 1]
    expect(call[0]).toBe('/tags')
  })

  it('toggling cloud view with a non-default sort already in the URL preserves both params', async () => {
    // Simulate an incoming URL of /tags?sort=newest, then user clicks Cloud.
    setUrlParams({ sort: 'newest' })
    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByTestId('view-cloud'))

    const call = mockReplace.mock.calls[mockReplace.mock.calls.length - 1]
    const url = new URL(call[0], 'http://t')
    expect(url.pathname).toBe('/tags')
    expect(url.searchParams.get('sort')).toBe('newest')
    expect(url.searchParams.get('view')).toBe('cloud')
  })

  // ── Cloud view rendering & log-scale font sizing ──

  it('renders cloud view when ?view=cloud with usage-count-driven font sizes', () => {
    setUrlParams({ view: 'cloud' })
    const tags = [
      makeTag({ id: 1, name: 'tiny', slug: 'tiny', usage_count: 1 }),
      makeTag({ id: 2, name: 'medium', slug: 'medium', usage_count: 25 }),
      makeTag({ id: 3, name: 'huge', slug: 'huge', usage_count: 500 }),
    ]
    mockUseTags.mockReturnValue({
      data: { tags, total: 3 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByTestId('tag-cloud')).toBeInTheDocument()

    const tiny = screen.getByTestId('tag-cloud-item-tiny')
    const medium = screen.getByTestId('tag-cloud-item-medium')
    const huge = screen.getByTestId('tag-cloud-item-huge')

    const sizePx = (el: HTMLElement) =>
      parseFloat((el as HTMLElement).style.fontSize.replace('px', ''))

    expect(sizePx(tiny)).toBeLessThan(sizePx(medium))
    expect(sizePx(medium)).toBeLessThan(sizePx(huge))
  })

  it('cloudFontSizePx uses a log scale (compressed relative to linear)', () => {
    const minU = 1
    const maxU = 1000
    const mid = Math.sqrt(minU * maxU) // geometric mid ≈ 31
    const size = cloudFontSizePx(mid, minU, maxU)
    // With log scaling, the geometric mean lands near the midpoint of the
    // font-size range (≈21.5px), not near the low end (≈13px) as linear
    // scaling would give. Assert it's comfortably above halfway-low.
    expect(size).toBeGreaterThan(19)
    expect(size).toBeLessThan(24)
  })

  it('cloudFontSizePx clamps min and max to the configured range', () => {
    expect(cloudFontSizePx(1, 1, 1000)).toBeCloseTo(13, 0)
    expect(cloudFontSizePx(1000, 1, 1000)).toBeCloseTo(30, 0)
  })

  // ── Pagination ──

  it('shows pagination when there are more results', () => {
    const tags = Array.from({ length: 50 }, (_, i) =>
      makeTag({ id: i + 1, name: `tag-${i}`, slug: `tag-${i}` })
    )
    mockUseTags.mockReturnValue({
      data: { tags, total: 100 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('Next')).toBeInTheDocument()
    expect(screen.getByText('Previous')).toBeInTheDocument()
    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
  })

  it('Previous button is disabled on first page', () => {
    const tags = Array.from({ length: 50 }, (_, i) =>
      makeTag({ id: i + 1, name: `tag-${i}`, slug: `tag-${i}` })
    )
    mockUseTags.mockReturnValue({
      data: { tags, total: 100 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('Previous')).toBeDisabled()
  })

  it('does not show pagination when all results fit on one page', () => {
    mockUseTags.mockReturnValue({
      data: { tags: [makeTag()], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.queryByText('Next')).not.toBeInTheDocument()
    expect(screen.queryByText('Previous')).not.toBeInTheDocument()
  })
})
