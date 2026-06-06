import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
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

vi.mock('next/link', () => {
  const MockLink = React.forwardRef<
    HTMLAnchorElement,
    React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }
  >(({ href, children, ...props }, ref) => (
    <a href={href} ref={ref} {...props}>
      {children}
    </a>
  ))
  MockLink.displayName = 'MockLink'
  return { default: MockLink }
})

import { TagBrowse } from './TagBrowse'

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

/**
 * Helper to drive the multi-query `useTags` mock. The component issues:
 *   1. main list query (category/search/sort/offset)
 *   2-4. one count query per category ({genre|locale|other}, limit:1)
 * We key the count responses off the `category` param so facet counts and the
 * main list are independently controllable.
 */
function mockTags({
  list,
  total = list.length,
  counts = { genre: 0, locale: 0, other: 0 },
  isLoading = false,
  error = null as Error | null,
  refetch = vi.fn(),
}: {
  list: TagListItem[]
  total?: number
  counts?: { genre: number; locale: number; other: number }
  isLoading?: boolean
  error?: Error | null
  refetch?: () => void
}) {
  mockUseTags.mockImplementation((params?: { category?: string; limit?: number }) => {
    // Count queries are the limit:1, category-scoped calls.
    if (params?.limit === 1 && params.category) {
      const cat = params.category as 'genre' | 'locale' | 'other'
      return {
        data: { tags: [], total: counts[cat] ?? 0 },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      }
    }
    // Main list query.
    return {
      data: error ? undefined : { tags: list, total },
      isLoading,
      error,
      refetch,
    }
  })
}

describe('TagBrowse', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setUrlParams()
    mockTags({ list: [], total: 0 })
  })

  // ── Loading state ──

  it('shows loading spinner on initial load', () => {
    mockUseTags.mockImplementation((params?: { limit?: number; category?: string }) => {
      if (params?.limit === 1 && params.category) {
        return { data: { tags: [], total: 0 }, isLoading: false, error: null, refetch: vi.fn() }
      }
      return { data: undefined, isLoading: true, error: null, refetch: vi.fn() }
    })

    renderWithProviders(<TagBrowse />)

    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  // ── Error state ──

  it('shows error message on error', () => {
    mockTags({ list: [], error: new Error('Server error') })

    renderWithProviders(<TagBrowse />)

    expect(
      screen.getByText('Failed to load tags. Please try again later.')
    ).toBeInTheDocument()
  })

  it('shows Retry button on error', async () => {
    const mockRefetch = vi.fn()
    mockTags({ list: [], error: new Error('Server error'), refetch: mockRefetch })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mockRefetch).toHaveBeenCalled()
  })

  // ── Empty state ──

  it('shows generic empty copy when no filter is active', () => {
    mockTags({ list: [], total: 0 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('No tags found.')).toBeInTheDocument()
  })

  it('shows filter-aware empty copy when a category facet is active', async () => {
    mockTags({ list: [], total: 0, counts: { genre: 0, locale: 3, other: 0 } })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByTestId('facet-locale'))

    expect(screen.getByText('No locale tags yet.')).toBeInTheDocument()
  })

  // ── Dense table rendering ──

  it('renders a directory table with Tag / Category / Uses column headers', () => {
    mockTags({ list: [makeTag()], total: 1 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByRole('columnheader', { name: 'Tag' })).toBeInTheDocument()
    expect(
      screen.getByRole('columnheader', { name: 'Category' })
    ).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Uses' })).toBeInTheDocument()
  })

  it('renders each tag as a row link to its detail page', () => {
    mockTags({
      list: [
        makeTag({ id: 1, name: 'rock', slug: 'rock' }),
        makeTag({ id: 2, name: 'punk', slug: 'punk', category: 'other' }),
      ],
      total: 2,
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByRole('link', { name: /rock/ })).toHaveAttribute(
      'href',
      '/tags/rock'
    )
    expect(screen.getByRole('link', { name: /punk/ })).toHaveAttribute(
      'href',
      '/tags/punk'
    )
  })

  it('renders the category as a tinted text label (not a pill)', () => {
    mockTags({ list: [makeTag({ category: 'genre' })], total: 1 })

    renderWithProviders(<TagBrowse />)

    const rows = screen.getAllByRole('row')
    // Header row + 1 data row.
    const dataRow = rows[1]
    const label = within(dataRow).getByText('Genre')
    // chart-6 = denim genre tint, no bg/border pill classes.
    expect(label.className).toContain('text-chart-6')
    expect(label.className).not.toContain('rounded-full')
  })

  it('renders the usage count in a right-aligned cell', () => {
    mockTags({ list: [makeTag({ usage_count: 142 })], total: 1 })

    renderWithProviders(<TagBrowse />)

    const cell = screen.getByText('142')
    expect(cell).toHaveClass('text-right')
    expect(cell).toHaveClass('tabular-nums')
  })

  it('renders Official indicator on official tags', () => {
    mockTags({ list: [makeTag({ is_official: true })], total: 1 })

    renderWithProviders(<TagBrowse />)

    // Dense rows use the compact icon-only indicator (aria-label, no visible text).
    expect(screen.getByRole('img', { name: 'Official tag' })).toBeInTheDocument()
  })

  it('renders total count', () => {
    mockTags({ list: [makeTag()], total: 15 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('15 tags')).toBeInTheDocument()
  })

  it('renders singular total count', () => {
    mockTags({ list: [makeTag()], total: 1 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('1 tag')).toBeInTheDocument()
  })

  // ── Category facet chips with counts ──

  it('renders All + per-category facet chips', () => {
    mockTags({ list: [makeTag()], total: 1, counts: { genre: 18, locale: 4, other: 2 } })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByTestId('facet-all')).toBeInTheDocument()
    expect(screen.getByTestId('facet-genre')).toBeInTheDocument()
    expect(screen.getByTestId('facet-locale')).toBeInTheDocument()
    expect(screen.getByTestId('facet-other')).toBeInTheDocument()
  })

  it('shows live per-category counts (and All = sum) on the facet chips', () => {
    mockTags({ list: [makeTag()], total: 24, counts: { genre: 18, locale: 4, other: 2 } })

    renderWithProviders(<TagBrowse />)

    expect(within(screen.getByTestId('facet-all')).getByText('24')).toBeInTheDocument()
    expect(within(screen.getByTestId('facet-genre')).getByText('18')).toBeInTheDocument()
    expect(within(screen.getByTestId('facet-locale')).getByText('4')).toBeInTheDocument()
    expect(within(screen.getByTestId('facet-other')).getByText('2')).toBeInTheDocument()
  })

  it('disables a facet chip with zero results', () => {
    mockTags({ list: [makeTag()], total: 22, counts: { genre: 18, locale: 4, other: 0 } })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByTestId('facet-other')).toBeDisabled()
    expect(screen.getByTestId('facet-genre')).not.toBeDisabled()
  })

  it('calls useTags with category when a facet chip is clicked', async () => {
    mockTags({ list: [makeTag()], total: 18, counts: { genre: 18, locale: 4, other: 2 } })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByTestId('facet-genre'))

    const listCalls = mockUseTags.mock.calls.filter(
      c => (c[0] as { limit?: number })?.limit !== 1
    )
    expect(listCalls[listCalls.length - 1][0]).toEqual(
      expect.objectContaining({ category: 'genre' })
    )
  })

  // ── Search ──

  it('renders an autofocused search input', () => {
    mockTags({ list: [], total: 0 })

    renderWithProviders(<TagBrowse />)

    const input = screen.getByPlaceholderText('Search tags...')
    expect(input).toBeInTheDocument()
    expect(input).toHaveFocus()
  })

  // ── Sort segmented control ──

  it('renders the three sort options as a segmented control', () => {
    mockTags({ list: [makeTag()], total: 1 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByTestId('sort-popularity')).toBeInTheDocument()
    expect(screen.getByTestId('sort-alphabetical')).toBeInTheDocument()
    expect(screen.getByTestId('sort-newest')).toBeInTheDocument()
  })

  it('marks popularity as the default checked sort', () => {
    mockTags({ list: [makeTag()], total: 1 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByTestId('sort-popularity')).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByTestId('sort-alphabetical')).toHaveAttribute('aria-checked', 'false')
  })

  it('defaults to popularity sort → calls useTags with backend sort "usage"', () => {
    mockTags({ list: [makeTag()], total: 1 })

    renderWithProviders(<TagBrowse />)

    const listCalls = mockUseTags.mock.calls.filter(
      c => (c[0] as { limit?: number })?.limit !== 1
    )
    expect(listCalls[listCalls.length - 1][0]).toEqual(
      expect.objectContaining({ sort: 'usage' })
    )
  })

  it('reflects URL ?sort=alphabetical in the control and the useTags call', () => {
    setUrlParams({ sort: 'alphabetical' })
    mockTags({ list: [makeTag()], total: 1 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByTestId('sort-alphabetical')).toHaveAttribute('aria-checked', 'true')

    const listCalls = mockUseTags.mock.calls.filter(
      c => (c[0] as { limit?: number })?.limit !== 1
    )
    expect(listCalls[listCalls.length - 1][0]).toEqual(
      expect.objectContaining({ sort: 'name' })
    )
  })

  it('reflects URL ?sort=newest and maps to backend "created"', () => {
    setUrlParams({ sort: 'newest' })
    mockTags({ list: [makeTag()], total: 1 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByTestId('sort-newest')).toHaveAttribute('aria-checked', 'true')

    const listCalls = mockUseTags.mock.calls.filter(
      c => (c[0] as { limit?: number })?.limit !== 1
    )
    expect(listCalls[listCalls.length - 1][0]).toEqual(
      expect.objectContaining({ sort: 'created' })
    )
  })

  it('selecting a non-default sort pushes it into the URL', async () => {
    mockTags({ list: [makeTag()], total: 1 })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByTestId('sort-alphabetical'))

    expect(mockReplace).toHaveBeenCalled()
    const call = mockReplace.mock.calls[mockReplace.mock.calls.length - 1]
    expect(call[0]).toBe('/tags?sort=alphabetical')
  })

  it('selecting the default sort strips ?sort from the URL', async () => {
    setUrlParams({ sort: 'alphabetical' })
    mockTags({ list: [makeTag()], total: 1 })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByTestId('sort-popularity'))

    const call = mockReplace.mock.calls[mockReplace.mock.calls.length - 1]
    expect(call[0]).toBe('/tags')
  })

  // ── Keyboard nav (↑/↓ between rows; Enter is native on the anchor) ──

  it('moves row focus with ArrowDown / ArrowUp', async () => {
    mockTags({
      list: [
        makeTag({ id: 1, name: 'rock', slug: 'rock' }),
        makeTag({ id: 2, name: 'punk', slug: 'punk' }),
        makeTag({ id: 3, name: 'jazz', slug: 'jazz' }),
      ],
      total: 3,
    })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    const rock = screen.getByRole('link', { name: /rock/ })
    const punk = screen.getByRole('link', { name: /punk/ })

    rock.focus()
    expect(rock).toHaveFocus()

    await user.keyboard('{ArrowDown}')
    expect(punk).toHaveFocus()

    await user.keyboard('{ArrowUp}')
    expect(rock).toHaveFocus()
  })

  it('ArrowUp on the first row stays on the first row', async () => {
    mockTags({
      list: [
        makeTag({ id: 1, name: 'rock', slug: 'rock' }),
        makeTag({ id: 2, name: 'punk', slug: 'punk' }),
      ],
      total: 2,
    })

    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    const rock = screen.getByRole('link', { name: /rock/ })
    rock.focus()

    await user.keyboard('{ArrowUp}')
    expect(rock).toHaveFocus()
  })

  // ── Pagination ──

  it('shows pagination when there are more results', () => {
    const tags = Array.from({ length: 50 }, (_, i) =>
      makeTag({ id: i + 1, name: `tag-${i}`, slug: `tag-${i}` })
    )
    mockTags({ list: tags, total: 100 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('Next')).toBeInTheDocument()
    expect(screen.getByText('Previous')).toBeInTheDocument()
    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
    expect(screen.getByText('Showing 1–50 of 100')).toBeInTheDocument()
  })

  it('Previous button is disabled on first page', () => {
    const tags = Array.from({ length: 50 }, (_, i) =>
      makeTag({ id: i + 1, name: `tag-${i}`, slug: `tag-${i}` })
    )
    mockTags({ list: tags, total: 100 })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('Previous')).toBeDisabled()
  })

  it('does not show pagination when all results fit on one page', () => {
    mockTags({ list: [makeTag()], total: 1 })

    renderWithProviders(<TagBrowse />)

    expect(screen.queryByText('Next')).not.toBeInTheDocument()
    expect(screen.queryByText('Previous')).not.toBeInTheDocument()
  })
})
