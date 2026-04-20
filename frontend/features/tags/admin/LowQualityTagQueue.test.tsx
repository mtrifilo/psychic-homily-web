import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { LowQualityTagQueueItem } from '../types'

const mockUseLowQualityTagQueue = vi.fn()
const mockSnooze = vi.fn()
const mockMarkOfficial = vi.fn()
const mockDelete = vi.fn()

vi.mock('./useAdminTags', () => ({
  useLowQualityTagQueue: (...args: unknown[]) =>
    mockUseLowQualityTagQueue(...args),
  useSnoozeTag: () => ({
    mutate: mockSnooze,
    isPending: false,
    variables: undefined,
  }),
  useMarkTagOfficial: () => ({
    mutate: mockMarkOfficial,
    isPending: false,
    variables: undefined,
  }),
  useDeleteTag: () => ({
    mutate: mockDelete,
    isPending: false,
  }),
  // MergeTagDialog transitively needs these.
  useMergeTags: () => ({ mutate: vi.fn(), isPending: false }),
  useMergeTagsPreview: () => ({ data: null, isLoading: false, error: null }),
  useTagAliases: () => ({ data: { aliases: [] }, isLoading: false }),
}))

vi.mock('../hooks', () => ({
  useSearchTags: () => ({ data: { tags: [] }, isLoading: false }),
}))

import { LowQualityTagQueue } from './LowQualityTagQueue'

function makeItem(
  overrides: Partial<LowQualityTagQueueItem> = {}
): LowQualityTagQueueItem {
  return {
    id: 1,
    name: 'rock',
    slug: 'rock',
    category: 'genre',
    is_official: false,
    usage_count: 0,
    created_at: '2025-01-01T00:00:00Z',
    upvotes: 0,
    downvotes: 0,
    reasons: ['orphaned'],
    ...overrides,
  }
}

describe('LowQualityTagQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the empty state when no tags are flagged', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: { tags: [], total: 0 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    expect(screen.getByText(/nothing to review/i)).toBeInTheDocument()
  })

  it('renders each flagged tag with its reason pills', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [
          makeItem({
            id: 1,
            name: 'orphan-tag',
            slug: 'orphan-tag',
            reasons: ['orphaned', 'short_name'],
          }),
          makeItem({
            id: 2,
            name: 'downvoted-tag',
            slug: 'downvoted-tag',
            usage_count: 3,
            upvotes: 1,
            downvotes: 4,
            reasons: ['downvoted'],
          }),
        ],
        total: 2,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    expect(screen.getByText('orphan-tag')).toBeInTheDocument()
    expect(screen.getByText('downvoted-tag')).toBeInTheDocument()

    const orphanReasons = screen.getByTestId('reasons-1')
    expect(orphanReasons).toHaveTextContent('Orphaned')
    expect(orphanReasons).toHaveTextContent('Short name')

    const downvotedReasons = screen.getByTestId('reasons-2')
    expect(downvotedReasons).toHaveTextContent('Downvoted')
  })

  it('fires the snooze mutation when Ignore is clicked', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [makeItem({ id: 42, name: 'mystery' })],
        total: 1,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    fireEvent.click(
      screen.getByRole('button', { name: /ignore mystery for 30 days/i })
    )
    expect(mockSnooze).toHaveBeenCalledWith(42)
  })

  it('fires the mark-official mutation when Official is clicked', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [makeItem({ id: 7, name: 'goodbadtag' })],
        total: 1,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    fireEvent.click(
      screen.getByRole('button', { name: /mark goodbadtag official/i })
    )
    expect(mockMarkOfficial).toHaveBeenCalledWith(7)
  })

  it('shows loading state while fetching', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    const { container } = renderWithProviders(<LowQualityTagQueue />)

    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('surfaces errors', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })

    renderWithProviders(<LowQualityTagQueue />)

    expect(screen.getByText(/boom/)).toBeInTheDocument()
  })

  it('paginates with Previous/Next', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [makeItem()],
        total: 50,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    const prevBtn = screen.getByRole('button', { name: /^previous$/i })
    const nextBtn = screen.getByRole('button', { name: /^next$/i })
    expect(prevBtn).toBeDisabled() // offset=0
    expect(nextBtn).toBeEnabled() // 50 total, 20 per page

    fireEvent.click(nextBtn)
    // After click, the hook is called again with offset=20. We can verify
    // via the latest call args.
    const callArgs = mockUseLowQualityTagQueue.mock.calls.at(-1)?.[0]
    expect(callArgs).toMatchObject({ limit: 20, offset: 20 })
  })
})
