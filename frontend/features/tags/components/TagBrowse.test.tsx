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

import { TagBrowse } from './TagBrowse'

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

  it('shows "No tags found" when results are empty', () => {
    mockUseTags.mockReturnValue({
      data: { tags: [], total: 0 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    })

    renderWithProviders(<TagBrowse />)

    expect(screen.getByText('No tags found.')).toBeInTheDocument()
  })

  // ── Tag rendering ──

  it('renders tag cards as links', () => {
    const tags = [
      makeTag({ id: 1, name: 'rock', slug: 'rock' }),
      makeTag({ id: 2, name: 'punk', slug: 'punk', category: 'style' }),
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
    expect(screen.getByText('Mood')).toBeInTheDocument()
    expect(screen.getByText('Era')).toBeInTheDocument()
    expect(screen.getByText('Style')).toBeInTheDocument()
    expect(screen.getByText('Instrument')).toBeInTheDocument()
    expect(screen.getByText('Locale')).toBeInTheDocument()
    expect(screen.getByText('Other')).toBeInTheDocument()
  })

  it('calls useTags with category when a category button is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<TagBrowse />)

    await user.click(screen.getByText('Genre'))

    // Check that useTags was called with category: 'genre'
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
